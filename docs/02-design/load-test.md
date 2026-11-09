# Load test

The load test measures the Core API, and the web apps in front of it, against the launch size of [backend architecture §13](backend-architecture.md#13-non-functional-targets-proposed): read p95 ≤ 300 ms, write p95 ≤ 600 ms, errors below 1%, with 600 customers, 3,000 units and 300 concurrent users. Those targets are PROPOSED; a run produces **measurements on the machine it ran on** (the lab server for a real run), not a guarantee of production performance. IR691.

## What exists, and what does not

| Phase | Scope | State |
|---|---|---|
| 1 | The Core API through the gateway: `smoke`, `load`, `stress`; the launch-size data (`cmd/loadseed`); the throwaway stack `ac-load`; the manual workflow `load.yml` | built (IR691) |
| 2 | The web layer: the customer, partner and technician apps (OIDC code flow with PKCE, the BFF session, the server-rendered pages) with `web-smoke`, `web`, `spike` and the 2-hour `soak` | built (IR692) |
| 3 | The IoT path: telemetry (target 330 batches a second, freshness p95 ≤ 10 s) and the command round trip (p95 ≤ 5 s) through the stack's own broker | planned |

## Running it

- By hand on a machine with Docker: `sh deploy/load/run.sh smoke` (also `load`, `stress`, and the web scenarios `web-smoke`, `web`, `spike`, `soak`). It builds the Go images tagged `load`, starts the compose project `ac-load`, fills it, runs k6 inside its network and removes everything. The results are in `load-out/`: `summary.json`, `k6.log`, `postgres.csv` (connections and CPU every 10 seconds), `compose-logs.txt` after a failure. `KEEP_STACK=1` leaves the stack up and `REUSE_STACK=1` runs only k6 against it, to try a scenario again.
- On the lab server: Actions > Load test > Run workflow (`.github/workflows/load.yml`, manual only, never on a schedule). It shares the concurrency group `e2e-lab-server` with the end-to-end tests, so the two never overlap. The shared test environment (`ac-test`) and the server's own services still share its CPU and memory: run it when they are idle.
- k6 exits with status 99 when a threshold is breached, and the script exits with it.
- `TIME_SCALE` and `VU_SCALE` (for example `TIME_SCALE=0.05 VU_SCALE=0.2`) shorten or shrink a scenario to try the script. The report then says "SHORTENED RUN ... not a measurement"; a real run leaves both at 1.

## What a run does

1. **Stack.** `deploy/load/compose.load.yaml` on top of `compose.yaml`: PostgreSQL (database `ac_load`), Valkey, LocalStack, Keycloak with a realm copy, a broker nobody connects to, the gateway, the five domain services and the workers; for the web scenarios also the customer, partner and technician apps (images built in the run, one at a time). No admin app, no IoT bridge or simulator. Nothing is published on the host and the script fails if a container publishes a port or names the real MQTT broker.
2. **Accounts and realm.** `loadseed -manifest-only` writes the account manifest before the stack starts. `deploy/load/realm.py` copies `docker/keycloak/realm-ac.json` (which is never changed) into a realm with the client `ac-load` (password grant; the Core API takes it as its app client) and one user per account. HQ accounts sign in through `ac-admin-web`, whose password grant is switched on in the copy only, because the Core API takes HQ's memberships from that client alone (IR510). The accounts have no one-time code device. The three web apps sign in with `ac-load` too (the authorization code flow with PKCE, redirect addresses of the load stack), so the tokens they get are the ones the Core API accepts.
3. **Data.** `loadseed` applies, on top of the demo seed, 600 customers with a property, two spaces, five units, a contract, three invoices and two maintenance requests each (3,000 units, 1,200 jobs, 300 of them offered to one of 10 contractor companies), and 300 accounts. It reuses `internal/seed`, so the IDs, tables and constraints are the seed's own. It refuses every database but `ac_load`, before connecting and again by `current_database()` after.
4. **Scenarios** (`service/web/load`, TypeScript run by k6; each virtual user is one account, so the 60-writes-a-minute limit per membership stays on and is met per person):
   - `smoke`: 4 virtual users for 1 minute, one account of each kind; no error at all is allowed.
   - `load`: 300 virtual users for 15 minutes (ramp 2 minutes), one account each; the §13 thresholds.
   - `stress`: 100 up to 1,500 virtual users in steps; it only records. It reads only, because accounts would be shared between virtual users and one membership's write limit could not be told from a slowdown.
5. **Web scenarios** (285 virtual users, one account each: the 300 accounts less HQ's 15, because HQ's sign-in needs a one-time code and the admin app is not part of the load stack):
   - `web-smoke`: 3 virtual users (a customer, a partner, a technician) for 1 minute; no error at all is allowed.
   - `web`: 285 virtual users for 15 minutes (ramp 2 minutes).
   - `spike`: 60 people at work, and at minute 3, 225 more sign in at the same moment, then work like the others (11 minutes). The report has a row for before, during and after the spike, so the time to come back to the earlier speed can be read.
   - `soak`: 285 virtual users for 2 hours, reported in windows of a quarter of an hour; a drift in page time or errors from the first window to the last shows in the table, and `postgres.csv` has the connections and the CPU and memory of every container every 10 seconds, to see a leak.
   Each virtual user is one browser: it signs in through the app's own sign-in step and the identity provider's page (the BFF's session cookie is kept for all its visits), then opens the app's main pages one after the other with 2–5 seconds of think time. Only each page's HTML is fetched (the scripts and styles are static files a CDN serves in production, and no script runs), so what is measured is the BFF and the server-side rendering with the Core API calls behind them. The scenarios read only: a Server Action cannot be called without a browser, so the writes stay in the API scenarios. Section 13 names no target for the web layer: the only threshold is the error rate (below 1%; the spike allows 5%, `web-smoke` none), and page and sign-in times are recorded to be read and to decide a target from.
6. **Calls (API scenarios).** Each visit is the reads of a role's main screens (the primary queries in `docs/03-uiux/screen-catalog.csv`) with 1–3 seconds of think time, and one write: `preferences.update` for everyone, `jobs.create` for a customer about every fifth visit.

## Reading the result

- The latency trends (`ac_read_duration`, `ac_write_duration`) hold **successful answers only**. A refusal takes a few milliseconds, so counting it in would make percentiles look better the more the system fails. Errors are counted on their own (`ac_errors`, `ac_status{class}`, `ac_rate_limited`); a 429 fails the run, because the accounts are separate and no virtual user should meet the limit.
- The report prints, per kind (read, write, sign-in), the requests, the error percentage and the p50, p95, p99 and max of the successful answers, then the status classes seen and the median speed of the error answers. A run with errors is read from that table, not from the percentiles.
- A page that sends the person back to the sign-in page (a lost session) counts as an error, as does a sign-in whose home page does not come back.
- The run also fails if any service logged `write limit not checked`: with Valkey unreachable the Core API lets the write through without checking the limit (it fails open) after about 1.8 seconds, which would make a run look as if there were no limit.
- Postgres connections (`pg_stat_activity` of `ac_load`) and the CPU of each container are in `postgres.csv`. `max_connections` is 100 and `newPool` sets no `MaxConns`, so with six services the connection count is the first limit to watch.

## Assumptions to confirm

- The mix of users (customer 70%, partner 15%, technician 10%, HQ 5%) is a placeholder: there is no usage data. The 300 accounts follow it, and the work of a visit is the same for every account of a kind.
- HQ has no web scenario, and the admin app is not measured.
- The data has no telemetry history, no alerts and no devices of its own (the demo seed's few units have some), so telemetry and energy queries are not part of the mix yet.
- The lab server shares its CPU with k6 and with its other services, and its IoT path is the bridge, not IoT Core.

## Results

| Run | Where | Result |
|---|---|---|
| `smoke` (IR691, 2026-11-09) | a developer's Mac, Docker Desktop, the Go images built there | 0 errors in 139 answers; read p95 43 ms, write p95 11 ms. This checks that the script and the stack work; it is **not a measurement** of the system |
| `web-smoke` (IR692, 2026-11-09) | a developer's Mac | 0 errors in 63 pages and 3 sign-ins; page p95 99 ms, sign-in p95 266 ms. A check of the script and the stack, not a measurement |
| `spike`, `soak`, shortened (`TIME_SCALE` 0.3 / 0.03, `VU_SCALE` 0.3 / 0.2) | a developer's Mac | They ran to the end with 0 errors and printed the window tables; the numbers are of a shortened run on a laptop and are not given here |
| `load`, `stress`, `web`, `spike`, `soak` | the lab server | not run yet |

§13 stays PROPOSED until `load` has been run on the lab server and its numbers have been read.
