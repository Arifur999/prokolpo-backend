# syntax=docker/dockerfile:1
# Static binaries on a distroless, non-root base. The image carries the API,
# the migration runner and a healthcheck probe; it has no shell.
ARG GO_VERSION=1.27
FROM golang:${GO_VERSION} AS build
WORKDIR /src

# Dependencies first, so this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/api ./cmd/migrate ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /usr/local/bin/
USER nonroot:nonroot
EXPOSE 5000
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 CMD ["healthcheck"]
ENTRYPOINT ["api"]
