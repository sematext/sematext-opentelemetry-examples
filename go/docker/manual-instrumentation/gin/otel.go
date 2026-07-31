package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// shutdownFunc releases the SDK providers. Call it before the process exits so
// buffered telemetry is flushed rather than dropped.
type shutdownFunc func(context.Context) error

// setupOTel wires up the three signals (traces, metrics, logs) against the
// Sematext Agent's OTLP/HTTP endpoints.
//
// Endpoints, headers and protocol are read from the standard OTEL_* environment
// variables by each exporter, so there are no hardcoded URLs or tokens here. The
// Sematext Agent listens on a separate port per signal, which is why the
// per-signal OTEL_EXPORTER_OTLP_<SIGNAL>_ENDPOINT variables are set in
// docker-compose.yaml rather than a single shared OTEL_EXPORTER_OTLP_ENDPOINT.
func setupOTel(ctx context.Context) (shutdownFunc, error) {
	res, err := newResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	// Collect every shutdown hook so a failure partway through setup can still
	// tear down whatever was already started.
	var shutdownFuncs []shutdownFunc
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// Propagate W3C trace context so spans join a distributed trace that starts
	// upstream (a browser, a gateway, another service).
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// --- Traces ---
	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create trace exporter: %w", err), shutdown(ctx))
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// --- Metrics ---
	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create metric exporter: %w", err), shutdown(ctx))
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			metricExporter,
			sdkmetric.WithInterval(30*time.Second),
		)),
	)
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// --- Logs ---
	logExporter, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create log exporter: %w", err), shutdown(ctx))
	}
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	// Go runtime metrics (GC, goroutines, heap) — cheap and useful in production.
	if err := runtime.Start(runtime.WithMeterProvider(meterProvider)); err != nil {
		return nil, errors.Join(fmt.Errorf("start runtime metrics: %w", err), shutdown(ctx))
	}

	return shutdown, nil
}

// newResource describes this service. service.name is what groups telemetry in
// Sematext, so it must be set consistently across all three signals.
//
// resource.New merges in the OTEL_RESOURCE_ATTRIBUTES and OTEL_SERVICE_NAME
// environment variables via WithFromEnv, and those take precedence over the
// defaults supplied here.
func newResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithProcess(),
		resource.WithContainer(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
}
