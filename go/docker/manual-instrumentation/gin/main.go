// Command gin-manual is a Gin HTTP service instrumented manually with the
// OpenTelemetry Go SDK, exporting traces, metrics and logs to Sematext Cloud
// via a local Sematext Agent.
//
// "Manual" here means the SDK is wired up in code (see otel.go) and this file
// creates its own spans, metrics and log records. The otelgin middleware still
// handles the per-request server span so that HTTP semantic conventions are
// correct without hand-rolling them.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	serviceName    = "go-gin-docker-manual"
	serviceVersion = "1.0.0"
)

// Telemetry handles. Tracer and meter are cheap to hold as package-level values
// because they resolve through the global providers set up in otel.go.
var (
	tracer = otel.Tracer(serviceName)
	meter  = otel.Meter(serviceName)

	// logger writes through the OTel slog bridge, so every record is exported
	// via OTLP *and* automatically stamped with the trace_id/span_id of the
	// span active on the context passed to logger.InfoContext(ctx, ...).
	//
	// Passing ctx is what makes log-to-trace correlation work in Sematext.
	// slog.Info (no ctx) still exports, but arrives without trace correlation.
	logger = otelslog.NewLogger(serviceName)
)

// Custom application metrics. These complement the HTTP metrics that otelgin
// records automatically.
var (
	userLookups    metric.Int64Counter
	activeRequests metric.Int64UpDownCounter
	lookupDuration metric.Float64Histogram
)

func initMetrics() error {
	var err error

	userLookups, err = meter.Int64Counter(
		"app.user.lookups",
		metric.WithDescription("Number of user lookups performed"),
		metric.WithUnit("{lookup}"),
	)
	if err != nil {
		return err
	}

	activeRequests, err = meter.Int64UpDownCounter(
		"app.requests.active",
		metric.WithDescription("Number of in-flight requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return err
	}

	lookupDuration, err = meter.Float64Histogram(
		"app.user.lookup.duration",
		metric.WithDescription("Duration of the simulated user lookup"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	// Bootstrap logger, used only until the OTel logger is available.
	boot := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Trap SIGINT/SIGTERM so Docker's stop signal triggers a graceful shutdown
	// and buffered telemetry gets flushed instead of dropped.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdown, err := setupOTel(ctx)
	if err != nil {
		boot.Error("failed to initialise OpenTelemetry", "error", err)
		os.Exit(1)
	}

	// Shutdown uses a fresh context: ctx is already cancelled by the time we
	// get here, and a cancelled context would abort the final flush.
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := shutdown(flushCtx); err != nil {
			boot.Error("error during OpenTelemetry shutdown", "error", err)
		}
	}()

	if err := initMetrics(); err != nil {
		boot.Error("failed to create metrics", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              ":" + port(),
		Handler:           newRouter(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Run the server in the background so main can wait on the signal context.
	serverErr := make(chan error, 1)
	go func() {
		logger.InfoContext(ctx, "server starting",
			slog.String("addr", srv.Addr),
			slog.String("service.name", serviceName),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		boot.Error("server failed", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		boot.Info("shutdown signal received, draining connections")
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(drainCtx); err != nil {
		boot.Error("graceful shutdown failed", "error", err)
	}
}

func newRouter() *gin.Engine {
	// Default to release mode unless GIN_MODE explicitly says otherwise.
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	// otelgin creates the server span per request, applying HTTP semantic
	// conventions and extracting any incoming W3C traceparent header.
	// Health endpoints are filtered out to keep probe noise out of tracing.
	r.Use(otelgin.Middleware(serviceName,
		otelgin.WithFilter(func(req *http.Request) bool {
			return req.URL.Path != "/health" && req.URL.Path != "/ready"
		}),
	))
	r.Use(activeRequestsMiddleware())

	// Liveness/readiness probes: unwrapped and untraced.
	r.GET("/health", func(c *gin.Context) { c.String(http.StatusOK, "healthy") })
	r.GET("/ready", func(c *gin.Context) { c.String(http.StatusOK, "ready") })

	r.GET("/", handleRoot)
	r.GET("/users/:id", handleGetUser)
	r.GET("/slow", handleSlow)
	r.GET("/error", handleError)

	return r
}

// activeRequestsMiddleware tracks in-flight requests as an UpDownCounter.
func activeRequestsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		attrs := metric.WithAttributes(
			attribute.String("http.route", c.FullPath()),
			attribute.String("http.request.method", c.Request.Method),
		)
		activeRequests.Add(c.Request.Context(), 1, attrs)
		defer activeRequests.Add(c.Request.Context(), -1, attrs)
		c.Next()
	}
}

func handleRoot(c *gin.Context) {
	ctx := c.Request.Context()

	// ctx carries the otelgin server span, so this log is correlated with it.
	logger.InfoContext(ctx, "root endpoint called")

	c.JSON(http.StatusOK, gin.H{
		"message":         "Hello from Go Gin with OpenTelemetry!",
		"service":         serviceName,
		"instrumentation": "manual",
	})
}

// handleGetUser shows the typical manual-instrumentation shape: a child span
// per logical step, attributes describing the work, and a metric recorded on
// completion.
func handleGetUser(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	// Child span of the otelgin server span.
	ctx, span := tracer.Start(ctx, "lookupUser",
		trace.WithAttributes(attribute.String("app.user.id", id)),
	)
	defer span.End()

	logger.InfoContext(ctx, "fetching user", slog.String("user.id", id))

	start := time.Now()
	user, err := fetchUserFromDB(ctx, id)
	elapsed := time.Since(start).Seconds()

	lookupDuration.Record(ctx, elapsed,
		metric.WithAttributes(attribute.Bool("app.lookup.error", err != nil)),
	)
	userLookups.Add(ctx, 1,
		metric.WithAttributes(attribute.Bool("app.lookup.error", err != nil)),
	)

	if err != nil {
		// RecordError attaches an exception event; SetStatus marks the span
		// failed so it surfaces in Sematext's error views.
		span.RecordError(err)
		span.SetStatus(codes.Error, "user lookup failed")
		logger.ErrorContext(ctx, "user lookup failed",
			slog.String("user.id", id),
			slog.Any("error", err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch user"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// fetchUserFromDB stands in for a real database call. The span attributes follow
// the OTel database semantic conventions so Sematext renders it as a DB call.
func fetchUserFromDB(ctx context.Context, id string) (gin.H, error) {
	_, span := tracer.Start(ctx, "SELECT users",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.operation.name", "SELECT"),
			attribute.String("db.collection.name", "users"),
		),
	)
	defer span.End()

	// Simulated query latency.
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		span.SetStatus(codes.Error, "query cancelled")
		return nil, ctx.Err()
	}

	return gin.H{
		"id":    id,
		"name":  "User " + id,
		"email": "user" + id + "@example.com",
	}, nil
}

// handleSlow produces a deliberately slow trace with two sequential child spans,
// useful for seeing latency breakdown in the Sematext Tracing App.
func handleSlow(c *gin.Context) {
	ctx := c.Request.Context()
	logger.InfoContext(ctx, "slow endpoint called")

	for _, step := range []struct {
		name string
		dur  time.Duration
	}{
		{"queryDatabase", 1 * time.Second},
		{"callExternalAPI", 1 * time.Second},
	} {
		_, span := tracer.Start(ctx, step.name)
		span.SetAttributes(attribute.Float64("app.step.duration_s", step.dur.Seconds()))
		time.Sleep(step.dur)
		span.End()
	}

	c.JSON(http.StatusOK, gin.H{"message": "slow operation completed"})
}

// handleError demonstrates error recording on a span.
func handleError(c *gin.Context) {
	ctx := c.Request.Context()

	span := trace.SpanFromContext(ctx)
	err := errors.New("intentional error for testing traces")

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())

	logger.ErrorContext(ctx, "error endpoint called",
		slog.Any("error", err),
		slog.Bool("app.error.intentional", true),
	)

	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8090"
}
