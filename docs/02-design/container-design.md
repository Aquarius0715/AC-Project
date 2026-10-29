---
document_id: DD-CONTAINER
version: 0.42.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent, operations]
scope: all-environments
---

# Container Design (everything runs in Docker)

## 1. Purpose and decision

Every application component runs as a Docker image in every environment (IR119, DEC-70):

- **Application components** — the Next.js web app / BFF, the Go Core API, the webhook receiver, every worker role, the migration runner, and the local-only IoT bridge and device simulator — are built once as images and run unchanged from a laptop to production.
- **Local and CI environments** run the whole system with Docker Compose (`compose.yaml` at the repository root), including containers that stand in for the AWS managed services.
- **Staging and production** run the same images on Amazon ECS on AWS Fargate. Data and messaging stay AWS managed services there (Aurora PostgreSQL, ElastiCache, AWS IoT Core, SQS / SNS, Kinesis, S3, Cognito, SES), as designed in the [backend architecture §3a](backend-architecture.md#3a-aws-service-mapping). Compose is never used in production.

The Phase 1A demo is also a container: the `web` image in mock mode (`docker compose --profile demo up`).

Checked on 2026-10-07: the `web` image built from `service/web/Dockerfile` serves `/`, `/login`, `/customer`, `/admin`, `/partner`, `/technician`, `/customer/alerts`, and `/demo` with HTTP 200 from a read-only root filesystem; `docker compose --profile demo --profile infra --profile schema up` started web, PostgreSQL 16 (93 tables from [db/schema.sql](db/schema.sql), local `ac_app_login` role), LocalStack (10 SQS queues, the telemetry stream, buckets, SES identity, secret), Keycloak (realm `ac`, OIDC discovery), Mosquitto (publish on `t/<tenant>/d/<device>/telemetry`), Valkey, stripe-mock, and the weather mock. A Go Echo image built with the §3 Dockerfile was 15.2 MB, ran read-only, answered its own `healthcheck` subcommand, and exited 0 on SIGTERM.

## 2. Image catalogue

| Image | Source | Contains | Runs as (local / production) |
|---|---|---|---|
| `ac-customer-web` / `ac-partner-web` / `ac-technician-web` / `ac-admin-web` | `service/web/Dockerfile` with `--build-arg APP=<name>` (context `service/web/`, npm workspace with the shared package `service/web/shared`) | One Next.js standalone server per entry point (IR178); `DATA_SOURCE=mock` keeps the Phase 1A fixtures, `DATA_SOURCE=api` renders through the server-only DAL and Server Actions | Compose `<name>-web` (demo, ports 3000–3003) and `<name>-web-api` (full) / one ECS service per app behind the ALB (customer, partner, technician host names; admin host on the HQ network, IR117) |
| `ac-gateway` / `ac-identity-api` / `ac-equipment-api` / `ac-maintenance-api` / `ac-billing-api` / `ac-energy-api` | `service/api/build/service.Dockerfile --build-arg SERVICE=<name>` (context `service/api/`, `./cmd/<name>`) | Static Go binary `/app/server` (IR180): the gateway routes the catalog's REST routes (IR222) to the business-domain service that owns each route's operation; each domain service serves only its operations (≈ 36 MB, gateway ≈ 22 MB); `/app/server healthcheck` | Compose `gateway` (port 8080) and five internal domain services / one ECS service each (the ALB can route the catalog's paths directly) |
| `ac-migrate` | `service/api/build/migrate.Dockerfile` (context `service/api/`, `./cmd/migrate`) | Static Go binary `/app/migrate up` with the embedded migrations (IR167); locally `APP_LOGIN_PASSWORD` creates `ac_app_login` and the domain logins `ac_<domain>_login` (IR194) and `SEED_FIXTURE` loads the demo fixture once | Compose `migrate` (one-off; `api` and the workers wait for it) / ECS one-off task before the deploy |
| `ac-worker` | `service/api/build/worker.Dockerfile` (context `service/api/`, `./cmd/worker`) | Static Go binary `/app/worker --role=<role>`; the scheduler runs per business domain (`--role=scheduler --domain=<equipment, maintenance or billing>`, IR195, IR341) on the domain's database login, asking other domains through internal queries; other roles idle until built | Compose `worker-scheduler-<domain>` and `worker-*` / ECS services per role and domain |

One backend image with several entry commands keeps every service on the same build and version; each ECS service overrides the command (`/app/worker --role=telemetry`, …). The `iotbridge` and `devicesim` binaries are built only into the `dev` target and never pushed to the production repository.

## 3. Dockerfile standards

| Rule | Web (Node) | Backend (Go) |
|---|---|---|
| Build | Multi-stage: `npm ci` → `npm run typecheck && npm run build` | Multi-stage: `go mod download` → `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`; cross-compiled with `$BUILDPLATFORM` / `$TARGETARCH` |
| Base | `node:26.12.1-bookworm-slim` (as built, IR441; the official image since IR409, as the distroless Node 22 line stopped at 22.22.0 without the fixes of 22.23.x). npm, npx and corepack are removed from the runtime, and the Debian packages are upgraded to their security fixes | `gcr.io/distroless/static-debian12:nonroot` |
| Toolchain | Node 26 (26.12.1 or later; CI `setup-node` 26, `engines` >=26, IR441) | Go 1.26.9 (`go.mod` toolchain line, IR408: docker scout found standard-library CVEs fixed only from 1.26.9) |
| User | `node` (UID 1000, as built, IR409) | `nonroot:nonroot` |
| Filesystem | Read-only root; tmpfs `/tmp`, `/app/.next/cache` | Read-only root; tmpfs `/tmp` |
| Privileges | No Linux capability (`cap_drop: [ALL]`) and `no-new-privileges` (as built, IR373) | same |
| Health | ALB / compose HTTP check on the app port | Binary subcommand `healthcheck` (no shell or curl in distroless) for `/healthz`; ALB uses `/readyz` |
| Shutdown | Next.js handles SIGTERM | Echo `Shutdown` with a 25 s drain inside ECS `stopTimeout` 30 s; workers stop polling and finish in-flight messages |
| Architecture | `linux/arm64` for Fargate Graviton; `linux/amd64` also built for x86 developer machines and CI | same |
| Pinning | Base and third-party images pinned by digest, GitHub Actions by commit SHA; Dependabot proposes the updates weekly (`.github/dependabot.yml`) | same |
| Contents | `.dockerignore` excludes `node_modules`, `.next`, `.env*` (as built, IR369: also the E2E sign-in state, test results, reports and coverage; Next.js copies a loaded `.env` / `.env.production` into the standalone output) | `.dockerignore` excludes `bin/`, test data, `.env*` (as built, IR369: also the test-run logs) |
| Secrets | Never in images or build args; injected at run time (ECS secrets from Secrets Manager, compose `.env.local`) | same |

Backend Dockerfile (the original pattern; as built the Go images come from three files in `service/api/build/`: `service.Dockerfile` builds one service per image from `cmd/${SERVICE}` (gateway and the five domain APIs; its `dev` target adds `devicesim`, `iotbridge` and `mockhistory`), `worker.Dockerfile` the worker and `migrate.Dockerfile` the migrator, all on `golang:1.26.9` and the digest-pinned distroless runtime; `cmd/webhook` is not built yet, IR197):

```dockerfile
# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/api ./cmd/webhook ./cmd/worker ./cmd/migrate

FROM build AS build-dev
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/ ./cmd/iotbridge ./cmd/devicesim

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872…   # pinned by digest in build/*.Dockerfile
COPY --from=build /out/ /app/
USER nonroot:nonroot
EXPOSE 8080

FROM runtime AS dev
COPY --from=build-dev /out/ /app/
```

## 4. Local and CI stack (Docker Compose)

`compose.yaml` uses profiles so that each task starts only what it needs:

| Profile | Starts | Use |
|---|---|---|
| `demo` | `customer-web`, `partner-web`, `technician-web`, `admin-web` (mock mode, ports 3000–3003) | Phase 1A clickable demo |
| `infra` | PostgreSQL 18, Valkey, LocalStack, Mosquitto, Keycloak, Mailpit, stripe-mock, weather mock | Stand-ins for AWS services |
| `schema` | one-off `migrate` (cmd/migrate): schema, local `ac_app_login` and the domain logins `ac_<domain>_login`, demo fixture | Database only, without the Go services |
| `backend` | `migrate`, `gateway` (8080), `identity-api`, `equipment-api`, `maintenance-api`, `billing-api`, `energy-api`, `worker-scheduler-equipment`, `worker-scheduler-maintenance`, `worker-scheduler-billing`, `worker-notification` (e-mails, IR418), `worker-outbox` (table retention, IR428) and the idle `worker-*` roles, with the `infra` stand-ins | Go backend against the stand-ins (IR180, IR194, IR195, IR341) |
| `full` | `customer-web-api`, `partner-web-api`, `technician-web-api`, `admin-web-api` (BFF mode, ports 3000–3003) + backend + infra | Production-like end-to-end runs and acceptance tests |
| `planned` | `webhook` (8082) | Services whose commands are not built yet; not started by the other profiles (IR197) |
| `iot` | `iot-bridge`, `device-sim`, one-off `mock-history` (with `backend` or `full`) | IoT demo: the mock fleet (`service/api/internal/devicesim/fleet.json`, one air conditioner and IoT module per state pattern, added by `migrate` with `SEED_MOCK_FLEET=1`) publishes telemetry and heartbeats and answers commands on the broker, and keeps each module's last applied setpoint as a retained message on its own `t/<tenant>/d/<device>/sim/kept` topic, so a simulator restart does not reset it; the bridge carries the telemetry and acknowledgements to the demo trigger and sends the requested commands; `mock-history` writes 30 days of past readings up to the scenario clock for the charts. Each client logs in with its own API key from `.env.local` (`docker/mosquitto/passwd.sh` writes the broker password file). Start everything with `sh docker/iot-demo.sh` and sign in as `customer-b` (customer app) or `hq-operator` (HQ app) |
| `obs` | OpenTelemetry collector, Jaeger (16686) | Traces |
| `stripe` | Stripe CLI forwarding test-mode webhooks to `webhook:8080/stripe` | Needs `STRIPE_TEST_SECRET_KEY` in `.env.local` |

Local stand-ins for the AWS managed services:

| Production (AWS) | Local container | Notes |
|---|---|---|
| Aurora PostgreSQL 18 (confirm that ap-southeast-5 offers the engine version before provisioning, IR441) | `postgres:18` (data in the `pgdata18` volume at `/var/lib/postgresql`) | Same schema and RLS; only DEFAULT partitions (no pg_partman) |
| ElastiCache (Valkey) | `valkey/valkey:9` | The BFF session store (IR382) and the write limit's counters (IR375) |
| SQS / SNS FIFO, Kinesis, Firehose, S3, SES, Secrets Manager, KMS, EventBridge Scheduler | `localstack/localstack:4` | Resources created by `docker/localstack/init-aws.sh`; the AWS clients will use `AWS_ENDPOINT_URL` (none is built yet, IR334) |
| AWS IoT Core (MQTT, rules) | `eclipse-mosquitto:2` + `iot-bridge` | Same topic layout; the bridge forwards topics to Kinesis / SQS like the IoT rules. Mutual TLS and IoT policies are verified in staging against IoT Core |
| Amazon Cognito | `keycloak:26` realm `ac` (`docker/keycloak/realm-ac.json`) | Two OIDC clients `ac-web` and `ac-admin-web` (HQ network rule), and identity-api's service-account client `ac-identity-admin` (no sign-in flow; its service account holds realm-management `manage-users` only, to end a user's sessions when HQ signs them out everywhere, IR386, and to open, point, e-mail and turn on or off the accounts of the people HQ, contractors and customer owners add, IR461–IR463); the user profile declares the claim attributes `tenant_id`, `membership_id` and `role` as admin-only (the user neither sees nor changes them; undeclared attributes would be dropped, IR463), TOTP (required for HQ users; the local realm pre-provisions a test TOTP credential per HQ user for end-to-end runs, IR177; an HQ account the identity service opens sets up its device in the set-up e-mail's steps and as a required action, IR468); one local user per fixture actor (username = `membershipId` in `docs/04-agentic-sdlc/fixture-contract.json`, password `AcDemoLocal-2026`); the Go verifier is configured by issuer and JWKS URL only; brute-force protection like Cognito's lockout (10 failures lock an account for 60 s, growing to 15 min, IR370) |
| Amazon SES | `axllent/mailpit:v1.31.6` (pinned by digest) | Catches every e-mail on SMTP `mailpit:1025` and shows it at http://localhost:8025; nothing leaves the machine. `worker-notification` sends the e-mail notifications and the invitations there (IR418); Keycloak's realm uses it as its SMTP server |
| Stripe API / webhooks | `stripe/stripe-mock` (offline) and Stripe CLI (test mode), each pinned by version and digest (IR373) | Live mode never outside production |
| Weather source | `wiremock` (`docker/wiremock`) | Fixed demo responses |
| CloudWatch / X-Ray | OpenTelemetry collector + Jaeger | Same OTLP exporter configuration |

Compose networks mirror the network zones: `edge` (browser-facing web, webhook, Keycloak), `app` (application services), `data` (data stand-ins). Only ports needed for development are published to the host. As built (IR373, IR382), the ports of PostgreSQL, Valkey, LocalStack, Mosquitto, Keycloak, the gateway, the webhook receiver and Jaeger are bound to `127.0.0.1`, so only this machine reaches them. They have local passwords or none: the database superuser, the identity provider's admin console, the anonymous broker, the session store without a password. The web apps' ports stay open to the network for showing the demo on other devices. Signing in from another device does not work in any case, because the issuer is `localhost`.

Configuration is twelve-factor: every service reads environment variables only. The variable names are the same locally (compose) and on ECS (task definitions); only the values differ.

| Variable | Used by | Local value (compose) | Production value |
|---|---|---|---|
| `APP_ENV` | backend | `local` | `staging` / `production` |
| `DATABASE_URL` / `DATABASE_READER_URL` | backend | `postgres` container: each domain service with its own login `ac_<domain>_login` (IR194), the worker with `ac_app_login` | Aurora writer / reader endpoints with IAM auth or a Secrets Manager secret |
| `AWS_REGION` | backend | `ap-southeast-5` | `ap-southeast-5` |
| `AWS_ENDPOINT_URL` | backend | `http://localstack:4566` | not set (real AWS endpoints, VPC endpoints) |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | backend | `test` / `test` (LocalStack) | not set (ECS task role) |
| `BLOB_DIR` | backend (maintenance-api) | `/var/lib/ac/blobs` on the named volume `blobs` (the image creates the directory owned by `nonroot`, so the read-only container keeps report photos, certificates and problem attachments across restarts, IR229) | not set — the S3 store for bucket `ac-files` is not implemented yet (IR229, open) |
| `OIDC_ISSUER` | backend, web | `http://localhost:8081/realms/ac` (Keycloak `KC_HOSTNAME` fixes the `iss` claim) | Cognito user pool issuer |
| `RATE_LIMIT_URL` | backend | `redis://valkey:6379` | ElastiCache endpoint (TLS, `rediss://`) — the shared counters of the write limit per membership (IR375); unset = no limit |
| `WRITE_LIMIT_PER_MINUTE` | backend | not set (60) | not set (60) — writes per membership and minute (network architecture §6) |
| `ACCOUNTS_PER_ORG_PER_DAY` | backend (identity-api) | `500` (repeated end-to-end runs add many accounts to the demo companies) | not set (30) — accounts a contractor or customer owner adds to their organization in 24 hours (IR464) |
| `OIDC_JWKS_URL` | backend | `http://keycloak:8080/realms/ac/protocol/openid-connect/certs` (container network) | not set (derived from the issuer discovery document) |
| `OIDC_INTERNAL_ISSUER` | web | `http://keycloak:8080/realms/ac` (container network) | not set (same as `OIDC_ISSUER`) |
| `OIDC_APP_CLIENT_ID` / `OIDC_ADMIN_CLIENT_ID` | backend, web | `ac-web` / `ac-admin-web` | Cognito app clients for `app.<domain>` / `admin.<domain>` (HQ network rule, IR117) |
| `MQTT_URL` | backend (iot worker, iot-bridge, device-sim) | `tcp://mosquitto:1883` | not set; the iot worker publishes through the AWS IoT data plane endpoint (`IOT_DATA_ENDPOINT`) |
| `IOT_DATA_ENDPOINT` | backend (iot worker) | not set | AWS IoT Core data endpoint / custom domain |
| `STRIPE_API_BASE` | backend | `http://stripe-mock:12111` | not set (api.stripe.com); keys come from Secrets Manager |
| `WEATHER_API_BASE` | backend (automation worker) | `http://weather-mock:8080` (WireMock, `docker/wiremock/mappings/weather.json`) | weather provider base URL (PROPOSED, provider not yet chosen); key from Secrets Manager |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | backend, web | `http://otel-collector:4318` | ADOT sidecar `http://localhost:4318` |
| `DATA_SOURCE` | web | `mock` (demo) / `api` (full) | `api` |
| `CORE_API_URL` | web (BFF / DAL) | `http://gateway:8080` | Core API entry (gateway, IR180) |
| `IDENTITY_API_URL` / `EQUIPMENT_API_URL` / `MAINTENANCE_API_URL` / `BILLING_API_URL` / `ENERGY_API_URL` | gateway, domain services | `http://<domain>-api:8080` | internal service discovery names — The gateway routes each REST route to the service that owns its operation; the services call each other's internal queries (IR180, IR190) |
| `DEMO_OPS` / `DEMO_CLOCK_START` | domain services, workers | `1` / not set (the fixture clock) | not set (demo operations off) — `demo.*` operations and the shared scenario clock (IR154, IR36, IR168) |
| `APP_LOGIN_PASSWORD` / `SEED_FIXTURE` | migrate | a local password / the fixture path | not set (logins from infrastructure as code; no fixture) — Local logins `ac_app_login` and `ac_<domain>_login`, and the demo fixture (IR167, IR194) |
| `INTERNAL_API_TOKEN` / `IDENTITY_INTERNAL_URL` | Core API services | secret / `http://identity-api:8080` | service-to-service calls; principals from identity-api (IR182) |
| `IDP_TOKEN_URL` / `IDP_ADMIN_URL` / `IDP_ADMIN_CLIENT_ID` / `IDP_ADMIN_CLIENT_SECRET` | identity-api only | `http://keycloak:8080/realms/ac/protocol/openid-connect/token` / `http://keycloak:8080/admin/realms/ac` / `ac-identity-admin` / a local value in `compose.yaml` | not set: a Cognito adapter calls AdminUserGlobalSignOut with identity-api's task role (`cognito-idp:AdminUserGlobalSignOut` on the user pool; planned with the AWS target) — Ends a user's sessions at the identity provider when HQ signs them out everywhere (IR386). All four or none, and the URLs are http(s), or the service does not start; unset = the Core API's cutoff only (IR384) |
| `MAIL_SMTP_ADDR` / `MAIL_FROM` | worker-notification | `mailpit:1025` / `AC Project <no-reply@ac-project.local>` | not set: an SES adapter sends with the worker's task role (`ses:SendEmail`, planned with the AWS target) — Unset locally = the worker idles and e-mails stay previews (IR418) |
| `APP_URL_CUSTOMER` / `APP_URL_PARTNER` / `APP_URL_TECHNICIAN` / `APP_URL_ADMIN` | worker-notification | `http://localhost:3000` … `3003` | the apps' public URLs — The sign-in links of the e-mails |
| `SESSION_SECRET` | web (BFF) | a local value in `compose.yaml` | Secrets Manager — Key material of the sealed session cookie (AES-256-GCM, key derived with HKDF, IR333). Required on https, at least 32 characters: without it the app seals and opens no session (IR367) |
| `APP_URL` / `OIDC_APP_CLIENT_SECRET` | web (BFF) | `http://localhost:3000`–`3003` / the Keycloak client secret | the app's host name / Secrets Manager — The OIDC redirect URI and the confidential client |
| `CUSTOMER_WEB_URL` / `PARTNER_WEB_URL` / `TECHNICIAN_WEB_URL` / `ADMIN_WEB_URL` | web | `http://localhost:3000`–`3003` | the four host names — Links between the four apps (IR178) |
| `SESSION_STORE_URL` | web (BFF) | `redis://valkey:6379` (web apps run on the host: `redis://127.0.0.1:6379`) | ElastiCache endpoint (TLS, `rediss://`) — The server-side session store (IR382): the record of each sign-in, revoked at sign-out; unset = the session sealed into the cookie (IR333) |

As built (IR334), the code reads the database, OIDC, demo, blob, internal-call and web-session variables above. `APP_ENV`, the `AWS_*` variables, `MQTT_URL`, `IOT_DATA_ENDPOINT`, `STRIPE_API_BASE`, `WEATHER_API_BASE` and `OTEL_EXPORTER_OTLP_ENDPOINT` are set for integrations that are not built yet, and nothing reads them:

- the outbox, the domain events and the notifications run inside the services on PostgreSQL;
- uploaded files stay in the local blob store (OPEN-CT-02);
- payments, weather and devices are simulated (DEC-68 open items, the `planned` profile);
- no service exports traces yet, so the `obs` profile receives nothing (OPEN-CT-04).

`docker/localstack/init-aws.sh` already creates the production resource names for those integrations: the domain-events FIFO topic, five FIFO queues with their dead-letter queues, the telemetry stream, four buckets, the SES identity and the Stripe secret.

The resource names, created locally by that script, are the ones the production infrastructure code creates (IR381):

| Resource | Name | Used for |
|---|---|---|
| SNS FIFO topic | `ac-domain-events.fifo` | Domain events from the outbox relay (backend architecture §8) |
| SQS FIFO queues | `command-requested`, `device-ack`, `device-event`, `notification-requested`, `importexport-jobs` (each `<name>.fifo`) | The work queues of the iot, notification and importexport workers (backend Go design §7) |
| Dead-letter queues | `<name>-dlq.fifo` for each queue, after 5 receives | Replays reuse the event ID (backend architecture §8) |
| Kinesis stream | `ac-telemetry` | Device telemetry from the IoT rules (2 shards locally) |
| S3 buckets | `ac-files`, `ac-reports`, `ac-telemetry-archive`, `ac-audit-export` | Uploads (OPEN-CT-02), generated reports and exports, raw measurement history in Parquet, the monthly audit export with Object Lock (backend architecture §3a, §7) |
| SES identity | `no-reply@ac.local` (LocalStack) | Kept for the planned SES adapter; the e-mails the stack sends today go over SMTP to Mailpit from `MAIL_FROM` (`no-reply@ac-project.local`, IR418). Production verifies the real domain |
| Secret | `ac/local/stripe` | The Stripe test key; production keeps one secret per environment in Secrets Manager |

Container-only variables of the stand-ins (`POSTGRES_*`, `KC_BOOTSTRAP_ADMIN_*`, LocalStack `SERVICES` / `AWS_DEFAULT_REGION`) exist only in compose. Local values come from compose and an optional git-ignored `.env.local` (`.env.local.example`).

## 5. Production on ECS Fargate

| Item | Design |
|---|---|
| Registry | Amazon ECR per account (dev, staging, prod), immutable tags, scan on push, cross-region replication to ap-southeast-1 |
| Tags | `git-<sha>` (immutable) plus release tags; deployments reference digests |
| Services | the four web apps `customer-web`, `partner-web`, `technician-web` and `admin-web` (IR178); `gateway` and the five domain services `identity-api`, `equipment-api`, `maintenance-api`, `billing-api` and `energy-api` (IR180); `webhook`, `worker-outbox`, `worker-scheduler-equipment`, `worker-scheduler-maintenance`, `worker-scheduler-billing`, `worker-iot`, `worker-telemetry`, `worker-rollup`, `worker-automation`, `worker-notification` and `worker-importexport`; task `migrate` |
| Task settings | `readonlyRootFilesystem: true`, non-root user, no privileged mode, all Linux capabilities dropped (`linuxParameters.capabilities.drop: ["ALL"]`, as in Compose, IR373), `stopTimeout: 30`, awsvpc networking in the private subnets of network architecture §2a, one security group per service |
| Configuration | Environment variables in the task definition; secrets from Secrets Manager via the `secrets` field; IAM task role per service (least privilege: e.g. only `worker-iot` may publish to IoT Core) |
| Scaling | the web apps, the gateway and the domain services on CPU and ALB request count; workers on SQS backlog per task and Kinesis iterator age; minimum two tasks across two AZs for the web apps, the gateway, the domain services, `webhook`, `worker-outbox` and `worker-iot` |
| Logs and traces | `awslogs` driver to CloudWatch (JSON), ADOT collector sidecar for OTLP traces and metrics |
| Deployment | Rolling with deployment circuit breaker and automatic rollback; `migrate` runs first as a one-off task (expand step), then the domain services and the workers, then the gateway, and the web apps last |
| Images allowed | Only images signed in CI (AWS Signer / cosign) and pushed by the pipeline role |

## 6. CI pipeline

1. Build both images with `docker buildx` for `linux/arm64` and `linux/amd64`; run unit tests inside the build stage.
2. Start `docker compose --profile full up -d` (or `infra` + backend) and run contract, store, API, and acceptance tests against it; tear down.
3. Scan images (ECR scan / Trivy), generate an SBOM, sign, push to the dev ECR.
4. Promote the same digest to staging, run the IoT Core / Stripe test-mode integration suite, then promote to production.

## 7. Open items

| ID | Open item |
|---|---|
| OPEN-CT-01 | Whether developers on x86 laptops need the `amd64` variant in the shared registry or build locally only |
| OPEN-CT-02 | The S3 blob store (bucket `ac-files`, LocalStack locally) for staging and production; until then the maintenance service keeps uploaded files in the local directory store on the `blobs` volume (IR229) |
| OPEN-CT-03 | Closed by IR382: the BFF keeps each sign-in in the session store (ElastiCache in production, Valkey locally) and deletes it at sign-out (backend architecture §6) |
| OPEN-CT-04 | OpenTelemetry instrumentation of the Go services and the web apps. The collector, Jaeger and `OTEL_EXPORTER_OTLP_ENDPOINT` are ready, but nothing exports traces or metrics yet (IR334) |

Additional contracts: every IR in the [Re-review Correction Contracts](review-resolution-contracts.md) applies, the newest included. They take priority over older text on the same topic; follow IR72 for conflict precedence.
