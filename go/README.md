# Go OpenTelemetry Examples

Go application examples with OpenTelemetry instrumentation for Sematext Cloud.

## Examples

| Environment | Manual (SDK) | Compile-time (otelc) | eBPF (OBI) |
|-------------|--------------|----------------------|------------|
| **Docker** | [Gin Manual](docker/manual-instrumentation/gin/) | [Gin otelc](docker/compile-instrumentation/gin/) | [Gin OBI](docker/auto-instrumentation/gin/) |

**Manual (SDK)**: Traces ✅ Metrics ✅ Logs ✅ — full control, nested spans, custom business spans, trace-correlated logs. Runs anywhere Docker runs.

**Compile-time (otelc)**: Traces ✅ Metrics ✅ Logs ⚠️ — zero code changes, zero runtime overhead, no privileged container. Instrumentation is woven in during `go build`. Route-aware spans and automatic library coverage (DB, Redis, Kafka, gRPC), but no custom business spans and a slower image build.

**eBPF (OBI)**: Traces ✅ Metrics ✅ Logs ⚠️ — zero code changes via [OpenTelemetry eBPF Instrumentation](https://github.com/open-telemetry/opentelemetry-ebpf-instrumentation). **Linux hosts only**, requires privileged containers, HTTP-boundary spans only, logs cannot be correlated with traces, and OBI is `v0` with breaking changes expected between releases.

## Which should I use?

Unlike most languages, Go has no runtime agent that can attach to a running process — it compiles to a static binary with no bytecode to rewrite. So "auto-instrumentation" for Go means either rewriting the build or watching the kernel, and the three examples above cover both.

| If you… | Use |
|---|---|
| Want spans around your own business logic | [Manual](docker/manual-instrumentation/gin/) |
| Need OTLP log export with trace correlation | [Manual](docker/manual-instrumentation/gin/) |
| Want broad coverage without touching source | [otelc](docker/compile-instrumentation/gin/) |
| Cannot add dependencies or telemetry code to the app | [otelc](docker/compile-instrumentation/gin/) |
| Cannot rebuild the binary at all | [OBI](docker/auto-instrumentation/gin/) |
| Are on macOS or Windows, or cannot run privileged containers | Manual or otelc (not OBI) |

**Starting fresh?** Use [manual instrumentation](docker/manual-instrumentation/gin/). It is the stable, mainstream Go path, gives all three signals with working log correlation, and is the only option that lets you trace your own logic rather than just library boundaries.

**Instrumenting an app you would rather not modify?** Use [otelc](docker/compile-instrumentation/gin/). It needs no source changes, no sidecar and no privileged container, and produces richer traces than eBPF.

The two approaches also compose: otelc can provide the automatic library coverage while you add manual spans for the business logic that matters most.

## Resources

- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/languages/go/)
- [OpenTelemetry Go Compile Instrumentation (otelc)](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation)
- [OpenTelemetry eBPF Instrumentation (OBI)](https://opentelemetry.io/docs/zero-code/obi/)
- [Sematext Agent Documentation](https://sematext.com/docs/agents/sematext-agent/opentelemetry/)
