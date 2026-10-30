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
| `AC_Project_Overview.pptx` | the overview deck as of docs 0.21.0 (2026-10-01): historical — it predates the implementation; the current state is in [docs/README.md](docs/README.md) |

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

An environment without the demo fixture starts from one HQ administrator (DEC-84). Create them once per tenant; the
identity service opens their sign-in account, which sets up a one-time-code device at the first sign-in, and e-mails
the set-up link. HQ adds everyone else in Access & roles; a tenant that already has an administrator is refused.

```sh
docker compose --env-file .env.local --profile full run --rm migrate \
  /app/migrate bootstrap-admin -tenant "AC Project" -email first.admin@example.com -name "First Admin"
```

## Checks

```sh
sh scripts/check.sh                       # before a push: web lint/typecheck/Vitest, gofmt/vet/staticcheck, generated files, docs validators
(cd service/api && make test-all)         # Go unit + integration suites on fresh test databases (both service layouts)
(cd service/web && npm run e2e)           # Playwright against the running stack
```

GitHub Actions run the same: `.github/workflows/ci.yml` (seven parallel jobs) and `.github/workflows/e2e.yml` (the
compose stack + Playwright) on every push to `main` and on pull requests. Actions are pinned by commit SHA, images by
digest and Go/npm dependencies by lock file; `.github/dependabot.yml` proposes their updates as weekly pull requests.
