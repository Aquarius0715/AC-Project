# Load test

The load test measures the Core API against the launch size of [backend architecture §13](backend-architecture.md#13-non-functional-targets-proposed): read p95 ≤ 300 ms, write p95 ≤ 600 ms, errors below 1%, with 600 customers, 3,000 units and 300 concurrent users. Those targets are PROPOSED; a run produces **measurements on the machine it ran on** (the lab server for a real run), not a guarantee of production performance. IR691.

## What exists, and what does not

| Phase | Scope | State |
|---|---|---|
| 1 | The Core API through the gateway: `smoke`, `load`, `stress`; the launch-size data (`cmd/loadseed`); the throwaway stack `ac-load`; the manual workflow `load.yml` | built (IR691) |
| 2 | The web layer (OIDC code flow, the BFF and the server-rendered pages); the sign-in spike and the soak (2 hours) | planned |
| 3 | The IoT path: telemetry (target 330 batches a second, freshness p95 ≤ 10 s) and the command round trip (p95 ≤ 5 s) through the stack's own broker | planned |

## Running it

- By hand on a machine with Docker: `sh deploy/load/run.sh smoke` (also `load`, `stress`). It builds the Go images tagged `load`, starts the compose project `ac-load`, fills it, runs k6 inside its network and removes everything. The results are in `load-out/`: `summary.json`, `k6.log`, `postgres.csv` (connections and CPU every 10 seconds), `compose-logs.txt` after a failure. `KEEP_STACK=1` leaves the stack up and `REUSE_STACK=1` runs only k6 against it, to try a scenario again.
- On the lab server: Actions > Load test > Run workflow (`.github/workflows/load.yml`, manual only, never on a schedule). It shares the concurrency group `e2e-lab-server` with the end-to-end tests, so the two never overlap. The shared test environment (`ac-test`) and the server's own services still share its CPU and memory: run it when they are idle.
- k6 exits with status 99 when a threshold is breached, and the script exits with it.

## What a run does

1. **Stack.** `deploy/load/compose.load.yaml` on top of `compose.yaml`: PostgreSQL (database `ac_load`), Valkey, LocalStack, Keycloak with a realm copy, a broker nobody connects to, the gateway, the five domain services and the workers. No web app, no IoT bridge or simulator. Nothing is published on the host and the script fails if a container publishes a port or names the real MQTT broker.
2. **Accounts and realm.** `loadseed -manifest-only` writes the account manifest before the stack starts. `deploy/load/realm.py` copies `docker/keycloak/realm-ac.json` (which is never changed) into a realm with the client `ac-load` (password grant; the Core API takes it as its app client) and one user per account. HQ accounts sign in through `ac-admin-web`, whose password grant is switched on in the copy only, because the Core API takes HQ's memberships from that client alone (IR510). The accounts have no one-time code device.
3. **Data.** `loadseed` applies, on top of the demo seed, 600 customers with a property, two spaces, five units, a contract, three invoices and two maintenance requests each (3,000 units, 1,200 jobs, 300 of them offered to one of 10 contractor companies), and 300 accounts. It reuses `internal/seed`, so the IDs, tables and constraints are the seed's own. It refuses every database but `ac_load`, before connecting and again by `current_database()` after.
4. **Scenarios** (`service/web/load`, TypeScript run by k6; each virtual user is one account, so the 60-writes-a-minute limit per membership stays on and is met per person):
   - `smoke`: 4 virtual users for 1 minute, one account of each kind; no error at all is allowed.
   - `load`: 300 virtual users for 15 minutes (ramp 2 minutes), one account each; the §13 thresholds.
   - `stress`: 100 up to 1,500 virtual users in steps; it only records. It reads only, because accounts would be shared between virtual users and one membership's write limit could not be told from a slowdown.
5. **Calls.** Each visit is the reads of a role's main screens (the primary queries in `docs/03-uiux/screen-catalog.csv`) with 1–3 seconds of think time, and one write: `preferences.update` for everyone, `jobs.create` for a customer about every fifth visit.

## Reading the result

- The latency trends (`ac_read_duration`, `ac_write_duration`) hold **successful answers only**. A refusal takes a few milliseconds, so counting it in would make percentiles look better the more the system fails. Errors are counted on their own (`ac_errors`, `ac_status{class}`, `ac_rate_limited`); a 429 fails the run, because the accounts are separate and no virtual user should meet the limit.
- The report prints, per kind (read, write, sign-in), the requests, the error percentage and the p50, p95, p99 and max of the successful answers, then the status classes seen and the median speed of the error answers. A run with errors is read from that table, not from the percentiles.
- The run also fails if any service logged `write limit not checked`: with Valkey unreachable the Core API lets the write through without checking the limit (it fails open) after about 1.8 seconds, which would make a run look as if there were no limit.
- Postgres connections (`pg_stat_activity` of `ac_load`) and the CPU of each container are in `postgres.csv`. `max_connections` is 100 and `newPool` sets no `MaxConns`, so with six services the connection count is the first limit to watch.

## Assumptions to confirm

- The mix of users (customer 70%, partner 15%, technician 10%, HQ 5%) is a placeholder: there is no usage data. The 300 accounts follow it, and the work of a visit is the same for every account of a kind.
- The data has no telemetry history, no alerts and no devices of its own (the demo seed's few units have some), so telemetry and energy queries are not part of the mix yet.
- The lab server shares its CPU with k6 and with its other services, and its IoT path is the bridge, not IoT Core.

## Results

| Run | Where | Result |
|---|---|---|
| `smoke` (IR691, 2026-11-09) | a developer's Mac, Docker Desktop, the Go images built there | 0 errors in 139 answers; read p95 43 ms, write p95 11 ms. This checks that the script and the stack work; it is **not a measurement** of the system |
| `load`, `stress` | the lab server | not run yet |

§13 stays PROPOSED until `load` has been run on the lab server and its numbers have been read.
