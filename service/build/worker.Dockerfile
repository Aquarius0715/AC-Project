# syntax=docker/dockerfile:1.7
# AC Project worker — background worker roles (scheduler ticks the clock-driven transitions; other roles follow).
# Build context: service/ (one Go module: cmd/worker + internal/).
ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd cmd
COPY internal internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/worker ./cmd/worker

# dev: toolchain image with the sources (local tools such as the IoT bridge build here)
FROM build AS dev

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/ /app/
USER nonroot
CMD ["/app/worker", "--role=scheduler"]
