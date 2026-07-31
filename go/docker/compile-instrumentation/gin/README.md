# Go Gin - Compile-Time Instrumentation with otelc (Docker)

Zero-code instrumentation for a Go Gin service using [OpenTelemetry Go Compile Instrumentation](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation) (`otelc`), which weaves instrumentation into the binary during `go build`.

Open [`main.go`](main.go) — it has **no OpenTelemetry imports at all**. The only direct dependency is Gin. Instrumentation and SDK setup are injected at build time, so telemetry costs nothing at runtime and nothing in the source.

> **Requires network access at build time.** `otelc` is distributed as source, so the [`Dockerfile`](Dockerfile) clones and compiles it, then resolves instrumentation modules while building the app. This makes the image build slower and unsuitable for fully air-gapped builds.

## Telemetry Data

| Type | Supported | Notes |
|--------|-----------|-------|
| **Traces** | ✅ | Route-aware server spans (`GET /users/:id`) with `http.route`, plus outbound HTTP client spans |
| **Metrics** | ✅ | Go runtime metrics (memory, goroutines, GC) |
| **Logs** | ⚠️ | A logger provider is initialised, but `slog` records are not exported by default — see [Logs](#logs) |

Verified against `otelc v1.0.1`: spans carry the matched route and resource attributes (`service.name`, `service.version`, `deployment.environment`), errors recorded via `c.Error()` appear on the span, and runtime metrics export on schedule.

### What gets instrumented

`otelc` detects supported libraries in the dependency graph and injects only what applies. For this app it wires up:

| Instrumentation | Effect |
|---|---|
| `net/http` server | Creates the per-request server span |
| `net/http` client | Spans for outbound requests |
| `gin` | Renames the span to `METHOD /route`, sets `http.route`, records `c.Error()` values |
| `runtime` | Go runtime metrics; also provides the goroutine-local storage other hooks rely on |
| `log/slog` | Adds `trace_id`/`span_id` attributes to log records |

Beyond this app, `otelc` also ships instrumentation for `database/sql`, Redis, Kafka, MongoDB, gRPC, logrus and others — so a service doing real database or queue work gets those spans without code changes. See the [instrumentation directory](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/tree/main/instrumentation) for the current list.

### How it compares

| | otelc (this example) | [eBPF/OBI](../../auto-instrumentation/gin/) | [Manual](../../manual-instrumentation/gin/) |
|---|---|---|---|
| Code changes | None | None | SDK setup + spans |
| Runs on macOS/Windows | ✅ | ❌ | ✅ |
| Needs `privileged: true` | ❌ | ✅ | ❌ |
| Runtime overhead | None (compile-time) | eBPF probes | SDK in-process |
| Route-aware span names | ✅ | ❌ (HTTP boundary) | ✅ |
| Library spans (DB, cache, queue) | ✅ for supported libraries | ❌ | ✅ where you add them |
| Custom business spans | ❌ | ❌ | ✅ |
| Build complexity | Higher (builds otelc) | None | None |

Use **otelc** when you want broad automatic coverage without touching source and without privileged containers. Use **manual** when you need spans around your own business logic. Use **OBI** when you cannot rebuild the binary at all.

## Prerequisites

- Docker and Docker Compose
- Sematext Cloud account with a Tracing App, a Monitoring App and a Logs App
- Network access during the image build

## Quick Start

### 1. Configure tokens

```bash
cp .env.example .env
```

Edit `.env` with your Infrastructure token, region, and the three App tokens. Compose reads it automatically; it is git-ignored and must never be committed. The stack refuses to start if a token is missing.

### 2. Start the stack

```bash
docker compose up -d --build
```

The first build takes a few minutes — it compiles `otelc` before compiling the app.

### 3. Generate traffic

```bash
curl http://localhost:8090/
curl http://localhost:8090/users/123
curl http://localhost:8090/slow
curl http://localhost:8090/error
```

### 4. View in Sematext Cloud

- **Traces** → Sematext Tracing App, service `go-gin-docker-otelc`
- **Metrics** → Sematext Monitoring App

Spans appear as `GET /users/:id` rather than a bare `GET`, because the Gin hook resolves the route pattern after the router matches.

## How It Works

The build is a normal `go build` with one prefix:

```dockerfile
RUN CGO_ENABLED=0 GOOS=linux otelc go build -o /out/app .
```

`otelc` intercepts the build, rewrites the relevant packages to add hooks, and links in an SDK bootstrap that runs before `main`. Everything else — endpoints, protocol, service name, sampling — is read from the standard `OTEL_*` environment variables at startup, so the same binary can be pointed anywhere without recompiling.

Because instrumentation is applied at build time rather than by an agent, there is no sidecar, no privileged container, and no runtime attach step.

### Configuration

Set in [`docker-compose.yaml`](docker-compose.yaml):

| Variable | Purpose |
|---|---|
| `OTEL_SERVICE_NAME` | Service identity that groups telemetry in Sematext |
| `OTEL_RESOURCE_ATTRIBUTES` | Extra resource attributes (version, environment) |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Agent traces port (`4338`) |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | Agent metrics port (`4318`) |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | Agent logs port (`4328`) |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` |
| `OTEL_LOG_LEVEL` | `otelc`'s own diagnostic verbosity |
| `OTELC_VERSION` (build arg) | `otelc` release used to instrument the build |

Per-signal endpoints are posted to verbatim — no `/v1/<signal>` path is appended. Only the generic `OTEL_EXPORTER_OTLP_ENDPOINT` gets a path appended.

Instrumentation can be narrowed with `OTEL_GO_ENABLED_INSTRUMENTATIONS` / `OTEL_GO_DISABLED_INSTRUMENTATIONS`, and the SDK disabled entirely with `OTEL_SDK_DISABLED=true`.

### Logs

This is the one area where compile-time instrumentation is less complete than the manual example, and it is worth being precise about.

`otelc` initialises a logger provider and hooks `log/slog` to add `trace_id`/`span_id` attributes. However, in this example the application's `slog` output goes to stdout and is **not** exported over OTLP, and the trace attributes are only attached when the hook can resolve an active span through goroutine-local storage.

Two practical options:

1. **Collect stdout** — let the Sematext Agent or Logs Discovery pick up container logs. Straightforward, and the standard approach for containerised apps.
2. **Use the manual example's approach** — wire the OTel `slog` bridge explicitly, as in [`../../manual-instrumentation/gin/otel.go`](../../manual-instrumentation/gin/otel.go), for guaranteed OTLP log export with trace correlation.

If correlated logs are a hard requirement, prefer the [manual example](../../manual-instrumentation/gin/).

## Building Locally (without Docker)

```bash
git clone --depth 1 --branch v1.0.1 \
    https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation.git /tmp/otelc
cd /tmp/otelc && make build

cd -                    # back to this directory
/tmp/otelc/otelc go build -o app .

OTEL_SERVICE_NAME=go-gin-otelc \
OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://localhost:4338 \
OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=http://localhost:4318 \
OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=http://localhost:4328 \
./app
```

On startup you should see `trace provider initialized with auto-export` — confirmation that instrumentation was woven in. A plain `go build` produces no such line.

## Troubleshooting

**No telemetry, and no "provider initialized" line at startup**

The binary was built without `otelc`. Confirm the build command is prefixed, and rebuild with `docker compose build --no-cache`.

**Build fails cloning or resolving modules**

`otelc` needs network access during the build to fetch its instrumentation modules. Check proxy settings and that `OTELC_VERSION` is a valid release tag.

**Spans named `GET` instead of `GET /users/:id`**

The Gin hook enriches the span after the router matches a route. A bare method name means no route matched (a genuine 404) or the gin instrumentation was excluded via `OTEL_GO_DISABLED_INSTRUMENTATIONS`.

**Binary is much larger than usual**

Expected. Instrumentation and the SDK are linked in, roughly doubling binary size. There is no runtime agent in exchange.

**Errors not showing on spans**

The Gin hook reads errors registered with `c.Error(err)`. Returning a 500 without that call produces a span with the right status code but no error detail.

## Production Considerations

- **Pin `OTELC_VERSION`**: pinned to `v1.0.1` so a new release cannot silently change instrumentation.
- **Build caching**: the otelc stage is a separate layer and caches well; keep it above the `COPY . .` step.
- **Tokens**: use Docker/Kubernetes secrets rather than a `.env` file.
- **Sampling**: exports every span by default. Set `OTEL_TRACES_SAMPLER=parentbased_traceidratio` and `OTEL_TRACES_SAMPLER_ARG=0.1` under real traffic.
- **Verify after dependency bumps**: instrumentation matches specific library versions, so re-check telemetry after upgrading Gin or other instrumented libraries.

## Resources

- [OpenTelemetry Go Compile Instrumentation](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation)
- [Supported instrumentations](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/tree/main/instrumentation)
- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/languages/go/)
- [Sematext Agent OpenTelemetry](https://sematext.com/docs/agents/sematext-agent/opentelemetry/)
