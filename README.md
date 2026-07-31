# Sematext OTel Onboarding

Everything you need to instrument applications with OpenTelemetry and send telemetry to Sematext Cloud Apps (Tracing App, Monitoring App, Logs App): per-language reference examples, an end-to-end multi-service demo, and an AI-powered onboarding skill.

## Overview

This repository covers both sides of the OpenTelemetry onboarding story — runnable code you can copy from, and an AI skill that walks you through the setup conversationally. Two flows are demonstrated:

- **Sematext Agent** — the service ships OTLP locally to a running Sematext Agent which forwards to Sematext Cloud. Covered by the per-language examples below. See [Sematext Agent OpenTelemetry docs](https://sematext.com/docs/agents/sematext-agent/opentelemetry/).
- **Managed OTLP endpoint** — the service ships OTLP directly to Sematext's hosted receiver. Used by the end-to-end multi-service example, where it's the natural fit for browser-side telemetry.

## Quick Start

1. **Pick a language** from the table below (click the link)
2. **Read the language README** - it contains all auto/manual instrumentation options and deployment environments
3. **Follow the instructions** in your chosen example

Or, for distributed-tracing across multiple services, jump to the [end-to-end example](e2e/).

## Examples by Language

| Language | Framework | Flow |
|----------|-----------|------|
| **[Node.js](nodejs/)** | Express | Sematext Agent |
| **[Java](java/)** | Spring Boot | Sematext Agent |
| **[Python](python/)** | Flask | Sematext Agent |
| **[.NET](dotnet/)** | ASP.NET Core | Sematext Agent |
| **[PHP](php/)** | Laravel | Sematext Agent |
| **[Go](go/)** | Gin | Sematext Agent, Managed OTLP endpoint |
| **[Ruby](https://sematext.com/docs/tracing/sdks/ruby/)** | Rails, Sinatra | Sematext Agent |

**Note**: Node.js, Java, Python, .NET, PHP, and Go examples are currently implemented. Go currently covers Docker only; other languages and environments will follow the same structure (baremetal/docker/kubernetes with auto and manual instrumentation).

## End-to-End Examples

Multi-service examples showing W3C trace context propagation across the full request path (frontend → backend → "database") with a single distributed trace per user action.

| Stack | Path | Flow |
|---|---|---|
| React + Express | [`e2e/react-express/`](e2e/react-express/) | Managed OTLP endpoint |

## AI-powered onboarding

For an AI-driven walkthrough of the instrumentation choices (region, App type, flow, language, env, auto vs manual), invoke the [`sematext-otel` skill](skills/SKILL.md) in Claude Code or any agent that can read markdown.

## Telemetry Data

- **Traces** - Distributed tracing shows request flow across services
- **Metrics** - Application and infrastructure metrics (latency, throughput, errors)
- **Logs** - Application logs correlated with traces

## Sematext Cloud

These examples send telemetry to Sematext Cloud Apps:

- **Sematext Tracing App** - Distributed tracing and application performance monitoring
- **Sematext Monitoring App** - Metrics and infrastructure monitoring
- **Sematext Logs App** - Log management and analysis

[Sign up for free](https://sematext.com/cloud/) to get started.

## Resources

- [Sematext Documentation](https://sematext.com/docs/)
- [Sematext Agent OpenTelemetry](https://sematext.com/docs/agents/sematext-agent/opentelemetry/)
- [Sematext Tracing](https://sematext.com/docs/tracing/)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [OpenTelemetry Zero-code Instrumentation](https://opentelemetry.io/docs/zero-code/)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
