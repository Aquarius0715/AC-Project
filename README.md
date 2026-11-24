# AC Project

Air-conditioner operations platform: a customer app, a partner (contractor) app, a technician app and the HQ admin
app on a Go Core API split into business-domain services. The specification lives in [docs/](docs/README.md).

| Path | What |
|---|---|
| `service/api` | Go (Echo): gateway, five domain services (identity, equipment, maintenance, billing, energy), workers, migrations, IoT bridge and device simulator |
| `service/web` | Next.js: `customer`, `partner`, `technician`, `admin` apps, the `portal` (every service address, IR791) and the shared package; Playwright E2E in `e2e/`, k6 load scenarios in `load/` |
| `service/device` | the ESP32 firmware of the real sensor nodes ([device firmware requirements](docs/02-design/device-firmware.md)) |
| `docs` | requirements, design, catalogs, validators (`docs/tools`) |
| `docker` | Keycloak realm and themes, Mosquitto, LocalStack, WireMock, the OpenTelemetry collector, IoT demo script |
| `deploy` | the shared test environment (`deploy/test`) and the load test (`deploy/load`) |
| `compose.yaml` | the local stack (profiles below) |
| `AC_Project_Overview.pptx` | the overview deck as of docs 0.21.0 (2026-10-01): historical — it predates the implementation; the current state is in [docs/README.md](docs/README.md) |

## Run locally

```sh
docker compose --env-file .env.local --profile full up -d --build   # API, workers, the four apps, the portal, Keycloak, Mailpit
sh docker/iot-demo.sh                                                 # optional: the IoT demo (mock fleet, broker, 30 days of history)
```

| URL | |
|---|---|
| http://localhost:3000 · 3001 · 3002 · 3003 | customer · partner · technician · HQ admin apps |
| http://localhost:3004 | the portal: every service address |
| http://localhost:8080/docs | Core API (Swagger) |
| http://localhost:8081 | Keycloak (realm `ac`) |
| http://localhost:8025 | Mailpit: every e-mail the stack sends (nothing leaves the machine) |

Sign in as `customer-a` / `customer-b`, `contractor-a`, `tech-external-a`, `hq-operator`, …; the local passwords and
the HQ one-time-code secrets are in `docker/keycloak/realm-ac.json` (local values only). `.env.local` (git-ignored,
from `.env.local.example`) holds local keys; `docker/iot-demo.sh` generates the broker keys on its first run.

`customer-a` sees the demo data only, the mock fleet's simulated air conditioners included; `customer-b` sees the real
sensor nodes' devices when the stack takes them in (DEC-92).

Real sensor nodes (optional, development only): with `REAL_MQTT_URL` and its login in `.env.local`, `migrate` adds a
live site to Demo Customer B and the IoT bridge records the nodes' readings there once a minute; `REAL_MQTT_COMMANDS=1`
also lets the app set the real air conditioner (power, setpoint, mode and fan). `customer-b` keeps its access past the
demo data's end of access (DEC-86, DEC-89, DEC-92, DEC-104; what the nodes must send: backend architecture §9 and the
[device firmware requirements](docs/02-design/device-firmware.md)).

Shared test environment (DEC-93, DEC-94): pushing a tag such as `test1.0.0r2` on a commit of `main` runs the workflow
"Test deployment" on the lab server, the real nodes' broker's machine, which is the repository's self-hosted runner. Once
the commit's CI passed it builds the images there and starts them, with the build and deploy logs on the Actions tab. CI
runs on GitHub-hosted runners (IR732); the lab server's runners run E2E every night and by hand, the load test by hand,
and the deployments (DEC-95).
The apps open at `http://<server>:3200` (customer), `:3201` (partner), `:3202` (technician), `:3203` (HQ) and the portal
at `:3204`, behind `web-edge` (IR855, IR857), and Keycloak at `:8281`; Mailpit (`:8225`) is on the server's loopback only,
read through an SSH tunnel (IR720). Its data are the default users and the real devices only, on the real clock: `customer-a`
sees the two real units, and there is no demo data or mock fleet (DEC-96). The server's one-time set-up is `bash deploy/test/setup.sh` from a checkout, then the
runner's registration (container design §6).

Profiles: `infra` (PostgreSQL, Valkey, LocalStack, Mosquitto, Keycloak, Mailpit, mocks), `backend` (+ API services and
workers), `full` (+ the four apps and the portal), `iot` (bridge, simulator, mock history), `obs`, `stripe`, `planned` (the webhook
receiver, not built), `demo` (the four apps in mock mode), `schema` (migrate only).

An environment without the demo fixture starts from one HQ administrator (DEC-84). Create them once per tenant; the
identity service opens their sign-in account, which sets up a one-time-code device at the first sign-in, and e-mails
the set-up link. HQ adds everyone else in Access & roles; a tenant that already has an administrator is refused.

```sh
docker compose --env-file .env.local --profile full run --rm migrate \
  /app/migrate bootstrap-admin -tenant "AC Project" -email first.admin@example.com -name "First Admin"
```

## Checks

```sh
sh scripts/check.sh                       # before a push: web lint/typecheck/Vitest, gofmt/vet/golangci-lint, generated files, docs validators
(cd service/api && make test-all)         # Go unit + integration suites on fresh test databases (both service layouts)
(cd service/web && npm run e2e)           # Playwright against the running stack
```

GitHub Actions run the same: `.github/workflows/ci.yml` (nine parallel jobs, 15 checks) on every push to `main` and on
pull requests, and `.github/workflows/e2e.yml` (the compose stack + Playwright) every night on `main` and by hand. Actions are pinned by commit SHA, images by
digest and Go/npm dependencies by lock file; `.github/dependabot.yml` proposes their updates as weekly pull requests.
