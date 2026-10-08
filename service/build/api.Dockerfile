# syntax=docker/dockerfile:1.7
# AC Project api — Core API (Go + Echo): HTTP operations at /v1/ops/* (backend architecture §4).
# Build context: service/ (one Go module: cmd/api + internal/).
ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd cmd
COPY internal internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api

# dev: toolchain image with the sources (local tools such as the IoT bridge build here)
FROM build AS dev

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/ /app/
USER nonroot
EXPOSE 8080
CMD ["/app/api"]
