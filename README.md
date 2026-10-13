# AC Project

Air-conditioner operations platform: a customer app, a partner (contractor) app, a technician app and the HQ admin
app on a Go Core API split into business-domain services. The specification lives in [docs/](docs/README.md).

| Path | What |
|---|---|
| `service/api` | Go (Echo): gateway, five domain services (identity, equipment, maintenance, billing, energy), workers, migrations, IoT bridge and device simulator |
| `service/web` | Next.js: `customer`, `partner`, `technician`, `admin` apps and the shared package; Playwright E2E in `e2e/` |
| `docs` | requirements, design, catalogs, validators (`docs/tools`) |
| `docker` | Keycloak realm, Mosquitto, IoT demo script |
| `compose.yaml` | the local stack (profiles below) |

## Run locally

```sh
docker compose --env-file .env.local --profile full up -d --build   # API, workers, the four apps, Keycloak, Mailpit
sh docker/iot-demo.sh                                                 # optional: the IoT demo (mock fleet, broker, 30 days of history)
```

| URL | |
|---|---|
| http://localhost:3000 · 3001 · 3002 · 3003 | customer · partner · technician · HQ admin apps |
| http://localhost:8080/docs | Core API (Swagger) |
| http://localhost:8081 | Keycloak (realm `ac`) |
| http://localhost:8025 | Mailpit: every e-mail the stack sends (nothing leaves the machine) |

Sign in as `customer-a` / `customer-b`, `contractor-a`, `tech-external-a`, `hq-operator`, …; the local passwords and
the HQ one-time-code secrets are in `docker/keycloak/realm-ac.json` (local values only). `.env.local` (git-ignored,
from `.env.local.example`) holds local keys; `docker/iot-demo.sh` generates the broker keys on its first run.

Profiles: `infra` (PostgreSQL, Valkey, LocalStack, Mosquitto, Keycloak, Mailpit, mocks), `backend` (+ API services and
workers), `full` (+ the four apps), `iot` (bridge, simulator, mock history), `obs`, `stripe`.

## Checks

```sh
sh scripts/check.sh                       # before a push: web lint/typecheck/Vitest, gofmt/vet/staticcheck, docs validators
(cd service/api && make test-all)         # Go unit + integration suites on fresh test databases (both service layouts)
(cd service/web && npm run e2e)           # Playwright against the running stack
```

GitHub Actions run the same: `.github/workflows/ci.yml` (seven parallel jobs) and `.github/workflows/e2e.yml` (the
compose stack + Playwright) on every push to `main` and on pull requests.
