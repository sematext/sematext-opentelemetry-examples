---
name: sematext-otel
description: Wire a service's OpenTelemetry (OTel) output to Sematext Cloud for observability and monitoring. Walks through region, App-type, instrumentation flow (managed OTLP endpoint vs Sematext Agent), and signal selection (traces/metrics/logs), then produces the exact env-var block and points at a runnable reference example in this repo. Invoke when instrumenting a new app for Sematext or sending telemetry to Sematext.
user-invocable: true
---

# Sematext OTel onboarding

Use this skill to wire a service that emits OpenTelemetry data into Sematext Cloud. It is parameter-driven; work through these in order:

1. **Triage**: ask the six questions below to fix region, Apps, flow, protocol, language/env, and instrumentation style.
2. **Assemble**: build the env-var block from Flow A (managed OTLP) or Flow B (Sematext Agent), placeholders intact.
3. **Point**: send the user to the matching reference example in this repo.
4. **Verify**: confirm data lands within 60s, looping through Troubleshooting until it does.

## Agent constraints

Two hard rules when running this skill:

**Never handle real token values.** Do not ask the user to paste an App token, and do not accept one if offered. Every env-var block you produce keeps the literal placeholders (`<tracing-app-token>`, etc.); the user substitutes real values themselves, outside the conversation. A token that appears in chat is in conversation history and agent context for good, and must be rotated in Sematext Cloud. If a user pastes one anyway, tell them to rotate it and continue with placeholders.

**Never run the privileged commands.** The `sudo st-agent otel enable` commands in Flow B are for the user to run in their own shell. Print them for the user to copy; do not execute them, and do not offer to.

## Triage

Ask the user, in order:

1. **Which Sematext region?** US or EU.
2. **Which App types are you wiring up?** Tracing, Logs, Monitoring — any combination. Each App has its own token; the user must have created the App(s) already in Sematext Cloud.
3. **Which flow?**
   - **Managed OTLP endpoint** — service ships directly to `otlp-receiver.sematext.com` (or EU). Simpler. Default for new users.
   - **Sematext Agent** — service ships to a local Sematext Agent which forwards. Required if the agent is already deployed for infra monitoring and you want one collector for everything.
4. **HTTP or gRPC?** Default HTTP (`http/protobuf`). gRPC only if the user has a specific reason.
5. **Language and deployment env?** Pick from the supported matrix below. Determines which reference example to point at and which instrumentation style (auto vs manual) to recommend.
6. **Auto or manual instrumentation?** Auto = traces + metrics, zero code changes. Manual = traces + metrics + logs, requires SDK init code. **OTel logs only ship via manual instrumentation.** If the user wants Logs App data and is reaching for auto, flag this tradeoff.

## Sematext fundamentals

- **One token per App.** Each Tracing / Logs / Monitoring App has its own token, wired as a separate signal-specific header. Signals with no header are skipped.
- **Custom auth header.** Sematext uses `X-API-TOKEN=<token>`, **not** `Authorization: Bearer …`. Hand-coded exporters that assume Bearer need overriding; the env-var path below works uniformly across SDKs.
- **Region-bound tokens.** US and EU have different endpoint hostnames, and a token belongs to one region. A US token against the EU endpoint drops data silently, with no error.

## Flow A — Managed OTLP endpoint

### Endpoint matrix

| Region | Protocol | `OTEL_EXPORTER_OTLP_ENDPOINT` | `OTEL_EXPORTER_OTLP_PROTOCOL` |
|---|---|---|---|
| US | HTTP (default) | `https://otlp-receiver.sematext.com` | `http/protobuf` |
| US | gRPC | `https://otlp-receiver-grpc.sematext.com:443` | `grpc` |
| EU | HTTP (default) | `https://otlp-receiver.eu.sematext.com` | `http/protobuf` |
| EU | gRPC | `https://otlp-receiver-grpc.eu.sematext.com:443` | `grpc` |

### Env-var block

Set the headers only for the signals the user is wiring up. Each `<token>` is the token of the corresponding Sematext App.

Emit this block **with the placeholders intact**; the user fills in real tokens themselves. See Agent constraints above. Point the user at their platform's secret store rather than a literal `export`: `--env-file` for Docker (never `ENV` in a Dockerfile, since it persists in the image layer), a Secret for Kubernetes, encrypted variables in CI. A plain `export` also writes the token to shell history.

```bash
# Endpoint + protocol — pick one row from the matrix above
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp-receiver.sematext.com
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf

# Per-signal token. Omit a line if the user doesn't have that App type.
export OTEL_EXPORTER_OTLP_TRACES_HEADERS=X-API-TOKEN=<tracing-app-token>
export OTEL_EXPORTER_OTLP_LOGS_HEADERS=X-API-TOKEN=<logs-app-token>
export OTEL_EXPORTER_OTLP_METRICS_HEADERS=X-API-TOKEN=<monitoring-app-token>

# Resource attributes — service.name is what shows up in the UI
export OTEL_SERVICE_NAME=my-service
export OTEL_SERVICE_VERSION=1.0.0
```

If the user is on auto-instrumentation, this env block plus the SDK's auto-instrumentation hook is all they need. If manual, they additionally need the SDK init code from the reference example.

## Flow B — Sematext Agent

The service ships to the locally-running Sematext Agent, which forwards to Sematext Cloud. No token in the service config — the agent already has one.

### Default ports

| Signal | Port |
|---|---|
| Traces | `4338` |
| Metrics | `4318` |
| Logs | `4328` (manual instrumentation only) |

These are signal-specific, so the umbrella `OTEL_EXPORTER_OTLP_ENDPOINT` is not used here — the per-signal `OTEL_EXPORTER_OTLP_*_ENDPOINT` env vars are.

### Env-var block

```bash
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://localhost:4338
export OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=http://localhost:4318
export OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=http://localhost:4328   # manual only

export OTEL_SERVICE_NAME=my-service
export OTEL_SERVICE_VERSION=1.0.0
```

**For the user to run**, once per signal type they want. These reconfigure a system service and need root, so the user runs them in their own shell. Present them for copying and do not execute them:

```bash
sudo /opt/spm/spm-monitor/bin/st-agent otel enable --type traces
sudo /opt/spm/spm-monitor/bin/st-agent otel enable --type metrics
sudo /opt/spm/spm-monitor/bin/st-agent otel enable --type logs
```

Each opens a local OTLP listener on the port above. Those ports are plaintext HTTP and unauthenticated, meant for same-host traffic only, so the agent should be reachable from localhost and not exposed across the network. Verify against the [Sematext Agent OpenTelemetry docs](https://sematext.com/docs/agents/sematext-agent/opentelemetry/) before running, since paths and flags vary by agent version.

## Reference examples in this repo

Once the user has picked language + env + instrumentation, send them to the corresponding directory. The READMEs there have language-specific build and run commands.

### Single-service (one App at a time)

| Language | Framework | Path | Flow |
|---|---|---|---|
| Node.js | Express | [`nodejs/`](../nodejs/) | Sematext Agent |
| Java | Spring Boot | [`java/`](../java/) | Sematext Agent |
| Python | Flask | [`python/`](../python/) | Sematext Agent |
| .NET | ASP.NET Core | [`dotnet/`](../dotnet/) | Sematext Agent |
| PHP | Laravel | [`php/`](../php/) | Sematext Agent |

Each language directory has the same structure:

```
{lang}/
├── README.md
├── baremetal/
│   ├── auto-instrumentation/{framework}/
│   └── manual-instrumentation/{framework}/
├── docker/
│   ├── auto-instrumentation/{framework}/
│   └── manual-instrumentation/{framework}/
└── kubernetes/
    ├── auto-instrumentation/{framework}/
    └── manual-instrumentation/{framework}/
```

The per-language examples target the Sematext Agent flow. If the user picked the managed OTLP flow instead, the SDK init code is identical — only the endpoint + auth headers differ (follow the env-var block in Flow A above).

### End-to-end (multi-service trace with W3C context propagation)

| Stack | Path | Flow |
|---|---|---|
| React + Express | [`e2e/react-express/`](../e2e/react-express/) | Managed OTLP endpoint (via backend-as-proxy for the browser-side spans) |

The e2e example is the natural reference for any setup that includes browser-side OpenTelemetry — browsers can't ship OTLP directly to a remote receiver (CORS), so the frontend POSTs spans to a same-origin endpoint on its own backend, which forwards to Sematext.

## Verify the data is landing

Within 60 seconds of starting the instrumented service:

| Signal | Where to look |
|---|---|
| Traces | Tracing App → Services → look for the `service.name` you set |
| Metrics | Monitoring App → look for the OTel metric names emitted by your SDK |
| Logs | Logs App → filter by `service.name` |

If nothing arrives, loop until it does: match the symptom in Troubleshooting below, apply the fix, restart the instrumented service, then re-check the table above after 60s. If two passes produce no data, drop to the narrowest test you can (one signal, `OTEL_LOG_LEVEL=debug` for exporter errors) before changing anything else.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| No data in any App within 60s | Token mismatch (region mismatch counts here too — US token on EU endpoint silently fails) |
| Traces but no metrics | Auto-instrumentation doesn't enable metrics in all SDKs by default; check SDK-specific flag |
| Auto-instrumented but no logs | Expected — auto only covers traces + metrics. Switch to manual for logs. |
| `Connection refused` on agent ports | Agent not running, or `st-agent otel enable --type <signal>` not run for that signal |
| `Connection refused` on managed endpoint | Wrong protocol (gRPC URL with HTTP protocol setting or vice versa) |
| Traces dropped intermittently | Batch size or queue full — bump `OTEL_BSP_MAX_QUEUE_SIZE` |
| TLS errors against managed endpoint | Old SDK / system CA bundle missing — update OS certs or SDK. Do not "fix" this by disabling certificate verification or setting the exporter to insecure; that sends the App token over an unverified connection. |
| `X-API-TOKEN` header rejected | Hand-coded exporter that forces `Authorization: Bearer`; remove that and use the `OTEL_EXPORTER_OTLP_*_HEADERS` env var path instead |
| CORS errors (browser/RUM) | Managed OTLP endpoint is server-to-server; browser-side instrumentation needs a different surface |

## Next steps after the user has data flowing

- Tracing App → set up a few starter alert rules (the platform now ships defaults: high response time, error count, HTTP 5xx, slow DB ops, volume anomaly, error rate anomaly).
- Monitoring App → if the user also runs the Sematext Agent for infrastructure, the OTel metrics will correlate with infra metrics automatically.
- Logs App → if the user wants logs correlated with traces, ensure `traceId` / `spanId` are emitted with each log record (manual instrumentation gives full control over this).

## Resources

- [Sematext Docs](https://sematext.com/docs/)
- [Sematext Agent — OpenTelemetry](https://sematext.com/docs/agents/sematext-agent/opentelemetry/)
- [OpenTelemetry SDK documentation](https://opentelemetry.io/docs/)
- This repo's per-language READMEs (linked above)
