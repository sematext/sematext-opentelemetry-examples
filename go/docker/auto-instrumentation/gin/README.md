# Go Gin - eBPF Auto-Instrumentation with OBI (Docker)

Zero-code tracing and metrics for a Go Gin service using [OpenTelemetry eBPF Instrumentation (OBI)](https://github.com/open-telemetry/opentelemetry-ebpf-instrumentation) — the OpenTelemetry project's eBPF agent, which instruments the process from outside with no instrumentation code in the request path.

> **⚠️ OBI is v0 / in development.** Upstream documents breaking changes between minor releases and advises pinning exact versions. This example pins `v0.10.0`. Review the [OBI release notes](https://github.com/open-telemetry/opentelemetry-ebpf-instrumentation/releases) before upgrading, and treat this path as experimental for production use.

> **Linux only.** OBI loads eBPF programs into the host kernel and requires a privileged container. It does **not** work on Docker Desktop for macOS or Windows. If you are on either, or cannot run privileged containers, use the [manual instrumentation example](../../manual-instrumentation/gin/) instead — it runs anywhere and produces richer telemetry.

## Telemetry Data

| Type | Supported | Notes |
|--------|-----------|-------|
| **Traces** | ✅ | HTTP-boundary spans from OBI (method, route, status, duration) |
| **Metrics** | ✅ | RED metrics (rate, errors, duration) and span metrics from OBI |
| **Logs** | ⚠️ | Not from OBI. The app exports its own logs via the OTel SDK — see the caveat below |

### Trade-offs versus manual instrumentation

| | eBPF/OBI (this example) | [otelc](../../compile-instrumentation/gin/) | [Manual](../../manual-instrumentation/gin/) |
|---|---|---|---|
| Code changes | None | None | SDK setup + spans |
| Runs on macOS/Windows | ❌ | ✅ | ✅ |
| Needs `privileged: true` | ✅ | ❌ | ❌ |
| Requires rebuilding the binary | ❌ | ✅ | ✅ |
| Span depth | HTTP boundary only | Route-aware + library spans | Nested spans, DB calls, internal steps |
| Log↔trace correlation | ❌ (see below) | ⚠️ partial | ✅ |
| Upstream stability | v0, breaking changes expected | v1.x | Stable (OTel Go SDK v1.x) |

If you can rebuild the application, [otelc](../../compile-instrumentation/gin/) is usually the better zero-code option — richer spans, no privileged container, and it runs on macOS and Windows. OBI's advantage is that it needs no rebuild at all.

**The log correlation caveat**: OBI builds spans inside the kernel, so they never appear in the application's Go context. Log records exported by the app therefore have no `trace_id` and **cannot be correlated with OBI's traces**. This is structural to mixing kernel-side tracing with application-side logging, not a configuration mistake.

## How It Works

OBI attaches eBPF probes to the Go binary via a shared PID namespace, decodes HTTP traffic at the kernel level, and exports OTLP directly to Sematext.

```
┌──────────────────────────────────────────┐
│  Docker Compose                          │
│                                          │
│  ┌─────────────┐                         │
│  │  go-app     │ ◀── HTTP ────────────── │ ◀── curl localhost:8090
│  │  (port 8090)│ ──── logs (OTLP) ─────▶ │ ──▶ Sematext Logs App
│  └──────┬──────┘                         │
│         │ shared PID namespace           │
│  ┌──────▼──────┐                         │
│  │  obi        │ ─ traces+metrics ─────▶ │ ──▶ Sematext Tracing +
│  │  (eBPF)     │      (OTLP/HTTP)        │     Monitoring Apps
│  └─────────────┘                         │
└──────────────────────────────────────────┘
```

The app **does** include the OpenTelemetry log SDK (see [`main.go`](main.go)) — only traces and metrics require no code.

## Prerequisites

- Linux host with a recent kernel (5.8+ recommended for BPF ring buffer support)
- Docker and Docker Compose
- Ability to run privileged containers
- Sematext Cloud account with Tracing, Monitoring and Logs Apps

Check your kernel:

```bash
uname -r
```

## Quick Start

### 1. Configure tokens

```bash
cp .env.example .env
```

Edit `.env` with your region endpoint and the three App tokens. Compose reads it automatically; it is git-ignored and must never be committed. The stack refuses to start if a token is missing.

### 2. Start the stack

```bash
docker compose up -d --build
```

Two containers start:
- **go-app** — the Gin server (no tracing/metrics code; log SDK only)
- **obi** — the eBPF agent instrumenting go-app

### 3. Generate traffic

```bash
curl http://localhost:8090/
curl http://localhost:8090/users/123
curl http://localhost:8090/slow
curl http://localhost:8090/error
```

### 4. View in Sematext Cloud

- **Traces** → Sematext Tracing App
- **Metrics** → Sematext Monitoring App
- **Logs** → Sematext Logs App

## Configuration

OBI splits configuration into two families, which is easy to trip over:

- **`OTEL_EBPF_*`** — OBI's own behaviour (target selection, log level, config path)
- **`OTEL_EXPORTER_OTLP_*`** — export destination, using the *standard* OpenTelemetry variables

Set in [`docker-compose.yaml`](docker-compose.yaml), sourced from `.env`:

| Variable | Applies to | Description |
|----------|-----------|-------------|
| `OTEL_EBPF_OPEN_PORT` | obi | Target app's listening port — how OBI finds the process |
| `OTEL_EBPF_CONFIG_PATH` | obi | Path to [`obi-config.yaml`](obi-config.yaml) |
| `OTEL_EBPF_LOG_LEVEL` | obi | `info` by default; set `debug` when troubleshooting |
| `OTEL_SERVICE_NAME` | both | Service identity in Sematext — same value for both containers |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | obi | Sematext managed OTLP receiver for your region |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | obi | `http/protobuf` |
| `OTEL_EXPORTER_OTLP_TRACES_HEADERS` | obi | Tracing App token |
| `OTEL_EXPORTER_OTLP_METRICS_HEADERS` | obi | Monitoring App token |
| `OTEL_EXPORTER_OTLP_LOGS_*` | go-app | Logs endpoint and token for the app's own log export |

### Why `pid: "service:go-app"`?

OBI's probes must be attached inside the target process's namespace. Sharing the PID namespace lets OBI see the `go-app` process; `privileged: true` and the `/sys/kernel/debug` + `/sys/fs/bpf` mounts grant the kernel access needed to load eBPF programs.

### Why the binary is not stripped

The [`Dockerfile`](Dockerfile) deliberately omits `-ldflags="-s -w"`. eBPF instrumentation resolves Go functions through the binary's symbol table, so stripping symbols degrades or breaks instrumentation. This is the opposite of normal Go production practice and specific to this approach.

## Troubleshooting

**OBI exits immediately**
- Confirm you are on a real Linux host, not Docker Desktop's VM
- Check `/sys/kernel/debug` is mounted and accessible
- Set `OTEL_EBPF_LOG_LEVEL=debug` in `.env` and re-check logs

**No traces in Sematext**
1. `docker compose logs obi` — look for export errors
2. Confirm `OTEL_EBPF_OPEN_PORT` matches the app's port (8090)
3. Verify each token matches its App type and that the endpoint region is right

**Config file appears to be ignored**
Target selection uses `discovery.instrument`. If the key is wrong, OBI silently instruments nothing rather than reporting an error — check [`obi-config.yaml`](obi-config.yaml) against the [configuration reference](https://opentelemetry.io/docs/zero-code/obi/configure/options/).

**Traces appear but logs do not**
Logs come from the app, not OBI. Check `docker compose logs go-app` and confirm `SEMATEXT_LOGS_TOKEN` is a Logs App token.

**Logs have no trace IDs**
Expected — see the caveat above. Use the [manual example](../../manual-instrumentation/gin/) if you need correlation.

## Production Considerations

- **Version pinning**: pinned to `v0.10.0`. Given OBI's v0 status, test explicitly before bumping.
- **Tokens**: use Docker/Kubernetes secrets instead of `.env`.
- **Privilege**: OBI needs elevated capabilities; confirm this is acceptable in your environment.
- **Sampling**: configure sampling in `obi-config.yaml` for high-traffic services.

## Resources

- [OBI repository](https://github.com/open-telemetry/opentelemetry-ebpf-instrumentation)
- [OBI documentation (OpenTelemetry zero-code)](https://opentelemetry.io/docs/zero-code/obi/)
- [OBI Docker setup](https://opentelemetry.io/docs/zero-code/obi/setup/docker/)
- [Sematext OTLP Ingestion](https://sematext.com/docs/agents/sematext-agent/opentelemetry/)
