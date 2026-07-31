# syntax=docker/dockerfile:1

# --- Build stage ---
FROM golang:1.25-alpine AS build

WORKDIR /src

# Copy manifests first so dependency download is cached independently of source
# changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binary: CGO off so it runs on a distroless/scratch-style base.
# -trimpath and the ldflags keep the binary small and reproducible.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/app .

# --- Runtime stage ---
FROM alpine:3.22

# curl is used by the container healthcheck below.
RUN apk add --no-cache ca-certificates curl \
    && addgroup -S app \
    && adduser -S -G app app

COPY --from=build /out/app /usr/local/bin/app

USER app

EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsS http://localhost:8090/health || exit 1

ENTRYPOINT ["/usr/local/bin/app"]
