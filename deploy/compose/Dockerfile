# Build context is the repository root (see compose.yaml). The Dockerfile lives
# here, next to the only file that uses it, so the repository root stays free of
# deployment artifacts.
# The Go version must match "go" in go.mod.
FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# The plugins are separate modules; their dependencies are fetched before the sources are
# copied so source changes do not invalidate the download layer.
COPY plugins/pushover/go.mod plugins/pushover/go.sum plugins/pushover/
COPY plugins/webhook/go.mod plugins/webhook/go.sum plugins/webhook/
RUN for plugin in pushover webhook; do (cd plugins/$plugin && go mod download) || exit 1; done
COPY . .
# Static binary: the runtime image needs no libc. Migrations are embedded.
RUN CGO_ENABLED=0 go build -trimpath -o /out/gopager ./cmd/gopager
# First-party plugins with their manifests (executable digest filled in).
RUN plugins/build.sh /out/plugins && chmod -R a+rX,go-w /out/plugins

FROM alpine:3.24
# Fixed non-root user; busybox wget (already in alpine) serves the healthcheck.
RUN adduser -D -u 10001 gopager
COPY --from=build /out/gopager /usr/local/bin/gopager
# Plugin directory: read-only for the gopager user, executables 0755, owned by root.
COPY --from=build --chown=root:root /out/plugins /usr/lib/gopager/plugins
USER gopager
ENTRYPOINT ["gopager"]
