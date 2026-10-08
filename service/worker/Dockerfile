# syntax=docker/dockerfile:1.7
# AC Project service/worker — Background worker roles (scheduler ticks the clock-driven transitions; other roles follow).
# Build context: service/ (the go.work workspace with the shared module service/core).
ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY go.work ./
COPY core/go.mod core/go.sum core/
COPY api/go.mod api/go.sum api/
COPY worker/go.mod worker/go.sum worker/
RUN --mount=type=cache,target=/go/pkg/mod cd worker && go mod download
COPY core core
COPY worker worker
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    cd worker && CGO_ENABLED=0 go build -trimpath -o /out/worker ./cmd/worker

# dev: toolchain image with the sources (local tools such as the IoT bridge build here)
FROM build AS dev

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/ /app/
USER nonroot

CMD ["/app/worker", "--role=scheduler"]
