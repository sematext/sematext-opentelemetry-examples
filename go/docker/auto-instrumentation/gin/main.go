// Command gin-auto is a Gin HTTP service whose traces and metrics are produced
// entirely by OpenTelemetry eBPF Instrumentation (OBI) — this file contains no
// tracing or metrics code at all.
//
// The one thing OBI cannot do is logs, so the OTel log SDK is wired up here to
// export application logs via OTLP.
//
// Note on correlation: because OBI builds spans in the kernel, they are not
// visible in this process's context. Log records therefore carry no trace_id and
// cannot be correlated with OBI's traces in Sematext. If you need log-to-trace
// correlation, use the manual example instead:
// ../../manual-instrumentation/gin
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
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

const (
	serviceName    = "go-gin-docker-auto"
	serviceVersion = "1.0.0"
)

var logger *slog.Logger

// initLogging sets up OTLP log export. Endpoint, headers and protocol are read
// from the OTEL_EXPORTER_OTLP_LOGS_* environment variables, so no URLs or
// tokens appear in this source file.
func initLogging(ctx context.Context) (*sdklog.LoggerProvider, error) {
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithContainer(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, err
	}

	exporter, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, err
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)
	global.SetLoggerProvider(provider)
	return provider, nil
}

func main() {
	boot := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	provider, err := initLogging(ctx)
	if err != nil {
		boot.Error("failed to init OTel logging", "error", err)
		os.Exit(1)
	}

	// Flush buffered logs on exit, using a fresh context because ctx is already
	// cancelled by the time this runs.
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := provider.Shutdown(flushCtx); err != nil {
			boot.Error("error flushing logs on shutdown", "error", err)
		}
	}()

	logger = otelslog.NewLogger(serviceName)

	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) { c.String(http.StatusOK, "healthy") })
	r.GET("/ready", func(c *gin.Context) { c.String(http.StatusOK, "ready") })

	r.GET("/", func(c *gin.Context) {
		logger.InfoContext(c.Request.Context(), "root endpoint called")
		c.JSON(http.StatusOK, gin.H{
			"message":         "Hello from Go Gin with OpenTelemetry!",
			"service":         serviceName,
			"instrumentation": "auto (eBPF)",
		})
	})

	r.GET("/users/:id", func(c *gin.Context) {
		id := c.Param("id")
		logger.InfoContext(c.Request.Context(), "fetching user", slog.String("user.id", id))
		time.Sleep(100 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{
			"id":    id,
			"name":  "User " + id,
			"email": "user" + id + "@example.com",
		})
	})

	r.GET("/slow", func(c *gin.Context) {
		logger.InfoContext(c.Request.Context(), "slow endpoint called")
		time.Sleep(2 * time.Second)
		c.JSON(http.StatusOK, gin.H{"message": "slow operation completed"})
	})

	r.GET("/error", func(c *gin.Context) {
		logger.ErrorContext(c.Request.Context(), "error endpoint called - simulating error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
	})

	srv := &http.Server{
		Addr:              ":" + port(),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		boot.Info("server starting", "addr", srv.Addr)
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

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8090"
}
