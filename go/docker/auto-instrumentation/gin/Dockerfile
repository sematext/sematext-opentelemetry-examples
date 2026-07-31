# syntax=docker/dockerfile:1

# --- Build stage ---
FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Note: unlike a typical Go build, symbols are NOT stripped (-s -w is omitted).
# OBI's eBPF probes rely on the Go symbol table to locate functions inside the
# binary, and stripping it degrades or breaks instrumentation.
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/app .

# --- Runtime stage ---
FROM alpine:3.22

RUN apk add --no-cache ca-certificates curl \
    && addgroup -S app \
    && adduser -S -G app app

COPY --from=build /out/app /usr/local/bin/app

USER app

EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsS http://localhost:8090/health || exit 1

ENTRYPOINT ["/usr/local/bin/app"]
