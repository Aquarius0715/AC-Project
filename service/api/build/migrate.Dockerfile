# syntax=docker/dockerfile:1.7
# AC Project migrate — one-off schema migration task (database design §9); runs before the API and workers start.
# Build context: service/api/ (one Go module: cmd/migrate + internal/).
ARG GO_VERSION=1.26.10

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd cmd
COPY internal internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS runtime
COPY --from=build /out/ /app/
USER nonroot
CMD ["/app/migrate", "up"]
