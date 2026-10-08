# syntax=docker/dockerfile:1.7
# AC Project service/api — Core API (Go + Echo): HTTP operations at /v1/ops/* (backend architecture §4).
# Build context: service/ (the go.work workspace with the shared module service/core).
ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY go.work ./
COPY core/go.mod core/go.sum core/
COPY api/go.mod api/go.sum api/
COPY worker/go.mod worker/go.sum worker/
COPY migrate/go.mod migrate/go.sum migrate/
RUN --mount=type=cache,target=/go/pkg/mod cd api && go mod download
COPY core core
COPY api api
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    cd api && CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api

# dev: toolchain image with the sources (local tools such as the IoT bridge build here)
FROM build AS dev

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/ /app/
USER nonroot
EXPOSE 8080
CMD ["/app/api"]
