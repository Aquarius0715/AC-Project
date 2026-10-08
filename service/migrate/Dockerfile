# syntax=docker/dockerfile:1.7
# AC Project service/migrate — one-off schema migration task (database design §9); runs before the API and workers start.
# Build context: service/ (the go.work workspace with the shared module service/core).
ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY go.work ./
COPY core/go.mod core/go.sum core/
COPY api/go.mod api/go.sum api/
COPY worker/go.mod worker/go.sum worker/
COPY migrate/go.mod migrate/go.sum migrate/
RUN --mount=type=cache,target=/go/pkg/mod cd migrate && go mod download
COPY core core
COPY migrate migrate
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    cd migrate && CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/ /app/
USER nonroot

CMD ["/app/migrate", "up"]
