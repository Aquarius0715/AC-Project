# syntax=docker/dockerfile:1.7
# AC Project Core API services (IR180): one image per business-domain service or the gateway, from one Go module.
# Build with --build-arg SERVICE=gateway | identity-api | equipment-api | maintenance-api | billing-api | energy-api;
# context service/api/.
ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-bookworm AS build
ARG SERVICE
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd cmd
COPY internal internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/${SERVICE}
RUN mkdir -p /out/blobs

# dev: toolchain image with the sources (local tools such as the IoT bridge build here)
FROM build AS dev

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/server /app/server
# the local blob store (BLOB_DIR) of development: owned by nonroot so a named volume mounted here is writable
COPY --from=build --chown=65532:65532 /out/blobs /var/lib/ac/blobs
USER nonroot
EXPOSE 8080
CMD ["/app/server"]
