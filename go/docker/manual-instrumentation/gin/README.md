# Go Gin - Manual Instrumentation (Docker)

Manual OpenTelemetry instrumentation for a Go Gin service, exporting **traces, metrics and logs** to Sematext Cloud through a local Sematext Agent.

This is the recommended starting point for Go: it runs anywhere Docker runs (including macOS and Windows), needs no special kernel privileges for the app itself, and produces trace-correlated logs.

## Telemetry Data

| Type | Supported | Notes |
|--------|-----------|-------|
| **Traces** | ✅ | Server spans via `otelgin`, plus custom child spans |
| **Metrics** | ✅ | HTTP metrics, custom app metrics, Go runtime metrics |
| **Logs** | ✅ | `slog` via the OTel bridge, automatically correlated to traces |

## Prerequisites

- Docker and Docker Compose
- A Sematext Cloud account with a Tracing App, a Monitoring App and a Logs App
- Go 1.25+ (only if you want to run outside Docker)

## Quick Start

### 1. Configure tokens

```bash
cp .env.example .env
```

Edit `.env` and fill in your Infrastructure token, region, and the three App tokens. Compose reads this file automatically; it is git-ignored and must never be committed.

The service refuses to start if a token is missing, rather than silently sending telemetry nowhere.

### 2. Start the stack

```bash
docker compose up -d --build
```

### 3. Generate traffic

```bash
curl http://localhost:8090/
curl http://localhost:8090/users/123
curl http://localhost:8090/slow
curl http://localhost:8090/error
```

### 4. View in Sematext Cloud

- **Traces** → Sematext Tracing App. Look for service `go-gin-docker-manual`.
- **Metrics** → Sematext Monitoring App.
- **Logs** → Sematext Logs App.

Open a trace for `GET /users/:id` and you will see the span tree:

```
GET /users/:id            (otelgin server span)
└── lookupUser            (custom span, app.user.id=123)
    └── SELECT users      (client span, db.system.name=postgresql)
```

## How It Works

Two files, split by responsibility:

| File | Responsibility |
|---|---|
| [`otel.go`](otel.go) | SDK setup: resource, exporters, providers, propagator, shutdown |
| [`main.go`](main.go) | The Gin app: routes, custom spans, custom metrics, logging |

### Endpoints

| Endpoint | What it demonstrates |
|---|---|
| `/` | Baseline traced request with a correlated log line |
| `/users/:id` | Nested spans, DB semantic conventions, custom metrics |
| `/slow` | Latency breakdown across two sequential child spans |
| `/error` | Error recording — `RecordError` + `SetStatus(codes.Error, …)` |
| `/health`, `/ready` | Probes, excluded from tracing to avoid noise |

### Configuration

All configuration is via standard `OTEL_*` environment variables — there are no endpoints or tokens in the Go source. Set in [`docker-compose.yaml`](docker-compose.yaml):

| Variable | Purpose |
|---|---|
| `OTEL_SERVICE_NAME` | Service identity that groups telemetry in Sematext |
| `OTEL_RESOURCE_ATTRIBUTES` | Extra resource attributes (version, environment) |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Agent traces port (`4338`) |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | Agent metrics port (`4318`) |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | Agent logs port (`4328`) |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` |

> The Sematext Agent uses a **separate port per signal**, so each signal needs its own endpoint variable rather than a single shared `OTEL_EXPORTER_OTLP_ENDPOINT`. These are treated as absolute URLs — the SDK posts directly to them and does not append `/v1/traces`.

## Instrumentation Patterns

### Trace-correlated logs

The key detail: pass `ctx` to the logger. The OTel `slog` bridge reads the active span from the context and stamps each record with its `trace_id` and `span_id`, which is what lets Sematext jump from a log line to its trace.

```go
logger.InfoContext(ctx, "fetching user", slog.String("user.id", id))  // correlated
slog.Info("fetching user")                                            // NOT correlated
```

### Custom spans

```go
ctx, span := tracer.Start(ctx, "lookupUser",
    trace.WithAttributes(attribute.String("app.user.id", id)),
)
defer span.End()
```

Reassigning `ctx` is what makes subsequent spans children of this one.

### Error recording

```go
span.RecordError(err)                        // attaches an exception event
span.SetStatus(codes.Error, "lookup failed") // marks the span failed
```

### Custom metrics

```go
userLookups, _ := meter.Int64Counter("app.user.lookups",
    metric.WithDescription("Number of user lookups performed"),
    metric.WithUnit("{lookup}"),
)
userLookups.Add(ctx, 1, metric.WithAttributes(attribute.Bool("app.lookup.error", false)))
```

### Graceful shutdown

Telemetry is batched, so an abrupt exit drops whatever is buffered. `main` traps `SIGTERM`, drains in-flight requests, then flushes the providers using a **fresh** context — the signal context is already cancelled at that point and would abort the flush.

## Running Without Docker

```bash
export OTEL_SERVICE_NAME=go-gin-manual
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://localhost:4338
export OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=http://localhost:4318
export OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=http://localhost:4328
go run .
```

## Troubleshooting

**No telemetry in Sematext**

1. Check the agent is receiving data: `docker compose logs sematext-agent`
2. Confirm each token belongs to the matching App type — a Logs token in the traces slot fails silently.
3. Confirm `REGION` matches your account (`US` vs `EU`).

**Logs arrive but have no trace correlation**

Use `logger.InfoContext(ctx, …)`, not `logger.Info(…)`. Without the context there is no active span to correlate against.

**Telemetry missing right after shutdown**

Batched telemetry flushes on shutdown. If you `docker kill` instead of `docker compose stop`, the flush is skipped.

**Spans not nested**

Use the `ctx` returned by `tracer.Start`, and pass it down. Reusing the parent `ctx` produces sibling spans instead of children.

## Production Considerations

- **Tokens**: use Docker/Kubernetes secrets rather than a `.env` file.
- **Sampling**: this example exports every span. Under real traffic, set `OTEL_TRACES_SAMPLER=parentbased_traceidratio` and `OTEL_TRACES_SAMPLER_ARG=0.1`.
- **Resource limits**: add `deploy.resources.limits` for the app and agent.
- **Metric interval**: 30s here; raise it to reduce data volume.
- **`deployment.environment`**: set it per environment so staging and production telemetry stay separable.

## Resources

- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/languages/go/)
- [otelgin instrumentation](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin)
- [otelslog bridge](https://pkg.go.dev/go.opentelemetry.io/contrib/bridges/otelslog)
- [Sematext Agent OpenTelemetry](https://sematext.com/docs/agents/sematext-agent/opentelemetry/)
