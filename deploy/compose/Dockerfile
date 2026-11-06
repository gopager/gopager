# Build context is the repository root (see compose.yaml). The Dockerfile lives
# here, next to the only file that uses it, so the repository root stays free of
# deployment artifacts.
# The Go version must match "go" in go.mod.
FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Static binary: the runtime image needs no libc. Migrations are embedded.
RUN CGO_ENABLED=0 go build -trimpath -o /out/gopager ./cmd/gopager

FROM alpine:3.22
# Fixed non-root user; busybox wget (already in alpine) serves the healthcheck.
RUN adduser -D -u 10001 gopager
COPY --from=build /out/gopager /usr/local/bin/gopager
USER gopager
ENTRYPOINT ["gopager"]
