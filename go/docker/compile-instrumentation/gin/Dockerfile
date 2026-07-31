# syntax=docker/dockerfile:1

# --- Stage 1: build the otelc tool ---
#
# otelc is distributed as source, so it is compiled here and then used to build
# the application in stage 2. Pinned to a release tag for reproducible builds.
FROM golang:1.25-alpine AS otelc-build

ARG OTELC_VERSION=v1.0.1

RUN apk add --no-cache git make

WORKDIR /otelc
RUN git clone --depth 1 --branch ${OTELC_VERSION} \
        https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation.git . \
    && make build

# --- Stage 2: build the application WITH instrumentation woven in ---
FROM golang:1.25-alpine AS build

RUN apk add --no-cache git

WORKDIR /src

COPY --from=otelc-build /otelc/otelc /usr/local/bin/otelc

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# The only difference from a normal build is the `otelc` prefix. It rewrites the
# build to inject instrumentation into the application and its dependencies.
#
# otelc resolves its own instrumentation modules during the build, so unlike a
# typical Dockerfile this stage needs network access.
#
# Note: CGO_ENABLED=0 for a static binary, and symbols are NOT stripped —
# otelc's toolchain needs them while rewriting.
RUN CGO_ENABLED=0 GOOS=linux otelc go build -o /out/app .

# --- Stage 3: runtime ---
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
