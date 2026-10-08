---
document_id: DD-CONTAINER
version: 0.30.0
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
| `ac-web` | `service/web/Dockerfile` | Next.js standalone server (`output: "standalone"`); `DATA_SOURCE` (`mock` or `api`) selects the Repository adapter once the API adapter exists — today `service/web/` always uses the in-browser mock and ignores the variable | Compose `web` (demo, mock) and `web-api` (BFF) / ECS service `web` behind the ALB (app and admin host names) |
| `ac-api` | `service/build/api.Dockerfile` (context `service/`, `./cmd/api`) | Static Go binary `/app/api` (Core API); `/app/webhook`, `/app/iotbridge`, `/app/devicesim` are planned as their own `service/<name>` | Compose `api` / ECS service `api` |
| `ac-migrate` | `service/build/migrate.Dockerfile` (context `service/`, `./cmd/migrate`) | Static Go binary `/app/migrate up` with the embedded migrations (IR167); locally `APP_LOGIN_PASSWORD` creates `ac_app_login` and `SEED_FIXTURE` loads the demo fixture once | Compose `migrate` (one-off; `api` and the workers wait for it) / ECS one-off task before the deploy |
| `ac-worker` | `service/build/worker.Dockerfile` (context `service/`, `./cmd/worker`) | Static Go binary `/app/worker --role=<role>` (scheduler implemented; other roles idle until built) | Compose `worker-*` / ECS services per role |

One backend image with several entry commands keeps every service on the same build and version; each ECS service overrides the command (`/app/worker --role=telemetry`, …). The `iotbridge` and `devicesim` binaries are built only into the `dev` target and never pushed to the production repository.

## 3. Dockerfile standards

| Rule | Web (Node) | Backend (Go) |
|---|---|---|
| Build | Multi-stage: `npm ci` → `npm run typecheck && npm run build` | Multi-stage: `go mod download` → `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`; cross-compiled with `$BUILDPLATFORM` / `$TARGETARCH` |
| Base | `gcr.io/distroless/nodejs22-debian12:nonroot` | `gcr.io/distroless/static-debian12:nonroot` |
| Toolchain | Node 22 LTS | Go 1.25 (current pgx v5 and golang.org/x modules require Go ≥ 1.25) |
| User | `nonroot` (UID 65532) | `nonroot:nonroot` |
| Filesystem | Read-only root; tmpfs `/tmp`, `/app/.next/cache` | Read-only root; tmpfs `/tmp` |
| Health | ALB / compose HTTP check on the app port | Binary subcommand `healthcheck` (no shell or curl in distroless) for `/healthz`; ALB uses `/readyz` |
| Shutdown | Next.js handles SIGTERM | Echo `Shutdown` with a 25 s drain inside ECS `stopTimeout` 30 s; workers stop polling and finish in-flight messages |
| Architecture | `linux/arm64` for Fargate Graviton; `linux/amd64` also built for x86 developer machines and CI | same |
| Pinning | Base images pinned by digest in CI builds (Renovate updates) | same |
| Contents | `.dockerignore` excludes `node_modules`, `.next`, `.env*` | `.dockerignore` excludes `bin/`, test data, `.env*` |
| Secrets | Never in images or build args; injected at run time (ECS secrets from Secrets Manager, compose `.env.local`) | same |

Backend Dockerfile (pattern verified with the Core API entry point):

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

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
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
| `demo` | `web` (mock mode, port 3000) | Phase 1A clickable demo |
| `infra` | PostgreSQL 16, Valkey, LocalStack, Mosquitto, Keycloak, stripe-mock, weather mock | Stand-ins for AWS services |
| `schema` | one-off `migrate` (cmd/migrate): schema, local `ac_app_login`, demo fixture | Database only, without the Go services |
| `backend` | `migrate`, `api` (8080), `webhook` (8082), eight `worker-*` services, `iot-bridge`, `device-sim` | Go backend against the stand-ins |
| `full` | `web-api` (BFF mode) + backend + infra | Production-like end-to-end runs and acceptance tests |
| `obs` | OpenTelemetry collector, Jaeger (16686) | Traces |
| `stripe` | Stripe CLI forwarding test-mode webhooks to `webhook:8080/stripe` | Needs `STRIPE_TEST_SECRET_KEY` in `.env.local` |

Local stand-ins for the AWS managed services:

| Production (AWS) | Local container | Notes |
|---|---|---|
| Aurora PostgreSQL 16 | `postgres:16` | Same schema and RLS; only DEFAULT partitions (no pg_partman) |
| ElastiCache (Valkey) | `valkey/valkey:8` | Sessions, idempotency fast path |
| SQS / SNS FIFO, Kinesis, Firehose, S3, SES, Secrets Manager, KMS, EventBridge Scheduler | `localstack/localstack:4` | Resources created by `docker/localstack/init-aws.sh`; services use `AWS_ENDPOINT_URL` |
| AWS IoT Core (MQTT, rules) | `eclipse-mosquitto:2` + `iot-bridge` | Same topic layout; the bridge forwards topics to Kinesis / SQS like the IoT rules. Mutual TLS and IoT policies are verified in staging against IoT Core |
| Amazon Cognito | `keycloak:26` realm `ac` (`docker/keycloak/realm-ac.json`) | Two OIDC clients `ac-web` and `ac-admin-web` (HQ network rule), TOTP (required for HQ users); one local user per fixture actor (username = `membershipId` in `docs/04-agentic-sdlc/fixture-contract.json`, password `local-pass`); the Go verifier is configured by issuer and JWKS URL only |
| Stripe API / webhooks | `stripe/stripe-mock` (offline) and Stripe CLI (test mode) | Live mode never outside production |
| Weather source | `wiremock` (`docker/wiremock`) | Fixed demo responses |
| CloudWatch / X-Ray | OpenTelemetry collector + Jaeger | Same OTLP exporter configuration |

Compose networks mirror the network zones: `edge` (browser-facing web, webhook, Keycloak), `app` (application services), `data` (data stand-ins). Only ports needed for development are published to the host.

Configuration is twelve-factor: every service reads environment variables only. The variable names are the same locally (compose) and on ECS (task definitions); only the values differ.

| Variable | Used by | Local value (compose) | Production value |
|---|---|---|---|
| `APP_ENV` | backend | `local` | `staging` / `production` |
| `DATABASE_URL` / `DATABASE_READER_URL` | backend | `postgres` container, login `ac_app_login` | Aurora writer / reader endpoints with IAM auth or a Secrets Manager secret |
| `AWS_REGION` | backend | `ap-southeast-5` | `ap-southeast-5` |
| `AWS_ENDPOINT_URL` | backend | `http://localstack:4566` | not set (real AWS endpoints, VPC endpoints) |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | backend | `test` / `test` (LocalStack) | not set (ECS task role) |
| `OIDC_ISSUER` | backend, web | `http://localhost:8081/realms/ac` (Keycloak `KC_HOSTNAME` fixes the `iss` claim) | Cognito user pool issuer |
| `OIDC_JWKS_URL` | backend | `http://keycloak:8080/realms/ac/protocol/openid-connect/certs` (container network) | not set (derived from the issuer discovery document) |
| `OIDC_INTERNAL_ISSUER` | web | `http://keycloak:8080/realms/ac` (container network) | not set (same as `OIDC_ISSUER`) |
| `OIDC_APP_CLIENT_ID` / `OIDC_ADMIN_CLIENT_ID` | backend, web | `ac-web` / `ac-admin-web` | Cognito app clients for `app.<domain>` / `admin.<domain>` (HQ network rule, IR117) |
| `MQTT_URL` | backend (iot worker, iot-bridge, device-sim) | `tcp://mosquitto:1883` | not set; the iot worker publishes through the AWS IoT data plane endpoint (`IOT_DATA_ENDPOINT`) |
| `IOT_DATA_ENDPOINT` | backend (iot worker) | not set | AWS IoT Core data endpoint / custom domain |
| `STRIPE_API_BASE` | backend | `http://stripe-mock:12111` | not set (api.stripe.com); keys come from Secrets Manager |
| `WEATHER_API_BASE` | backend (automation worker) | `http://weather-mock:8080` (WireMock, `docker/wiremock/mappings/weather.json`) | weather provider base URL (PROPOSED, provider not yet chosen); key from Secrets Manager |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | backend, web | `http://otel-collector:4318` | ADOT sidecar `http://localhost:4318` |
| `DATA_SOURCE` | web | `mock` (demo) / `api` (full) | `api` |
| `CORE_API_URL` | web (BFF) | `http://api:8080` | internal Core API name (service discovery) |
| `SESSION_STORE_URL` | web (BFF) | `redis://valkey:6379` | ElastiCache endpoint (TLS) |

Container-only variables of the stand-ins (`POSTGRES_*`, `KC_BOOTSTRAP_ADMIN_*`, LocalStack `SERVICES` / `AWS_DEFAULT_REGION`) exist only in compose. Local values come from compose and an optional git-ignored `.env.local` (`.env.local.example`).

## 5. Production on ECS Fargate

| Item | Design |
|---|---|
| Registry | Amazon ECR per account (dev, staging, prod), immutable tags, scan on push, cross-region replication to ap-southeast-1 |
| Tags | `git-<sha>` (immutable) plus release tags; deployments reference digests |
| Services | `web`, `api`, `webhook`, `worker-outbox`, `worker-scheduler`, `worker-iot`, `worker-telemetry`, `worker-rollup`, `worker-automation`, `worker-notification`, `worker-importexport`; task `migrate` |
| Task settings | `readonlyRootFilesystem: true`, non-root user, no privileged mode, `stopTimeout: 30`, awsvpc networking in the private subnets of network architecture §2a, one security group per service |
| Configuration | Environment variables in the task definition; secrets from Secrets Manager via the `secrets` field; IAM task role per service (least privilege: e.g. only `worker-iot` may publish to IoT Core) |
| Scaling | `web` and `api` on CPU and ALB request count; workers on SQS backlog per task and Kinesis iterator age; minimum two tasks across two AZs for `web`, `api`, `webhook`, `worker-outbox`, `worker-iot` |
| Logs and traces | `awslogs` driver to CloudWatch (JSON), ADOT collector sidecar for OTLP traces and metrics |
| Deployment | Rolling with deployment circuit breaker and automatic rollback; `migrate` runs first as a one-off task (expand step); `web` last |
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

Additional contracts for current version 0.30.0: Read IR01–139 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.
