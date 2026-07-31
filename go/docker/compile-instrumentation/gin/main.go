// Command gin-otelc is a plain Gin HTTP service with NO OpenTelemetry code.
//
// There are no OTel imports, no SDK setup and no spans in this file — compare it
// with ../../manual-instrumentation/gin/main.go, which does all of that by hand.
// Instrumentation is injected at build time by otelc (see Dockerfile), which
// rewrites the Go build to weave in:
//
//   - net/http server + client spans
//   - gin route enrichment (span renamed to "GET /users/:id", http.route set)
//   - Go runtime metrics
//   - log/slog trace-context enrichment
//
// The OTel SDK is also initialised automatically and configured entirely from
// the standard OTEL_* environment variables in docker-compose.yaml.
//
// This file is deliberately ordinary Go. Keeping it free of telemetry concerns
// is the whole point of compile-time instrumentation.
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
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
			"instrumentation": "compile-time (otelc)",
		})
	})

	r.GET("/users/:id", func(c *gin.Context) {
		id := c.Param("id")
		logger.InfoContext(c.Request.Context(), "fetching user", slog.String("user.id", id))

		// Stands in for a database call. otelc instruments supported libraries
		// (net/http, database/sql, redis, kafka, mongo, ...) automatically; a
		// plain sleep like this has nothing to hook, so it produces no span.
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
		err := errors.New("intentional error for testing traces")

		// c.Error records the error on the gin context. otelc's gin hook reads
		// these after the handler chain returns and marks the span failed —
		// without this call the 500 would still be traced, but without the
		// error detail attached.
		_ = c.Error(err)

		logger.ErrorContext(c.Request.Context(), "error endpoint called", slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	})

	srv := &http.Server{
		Addr:              ":" + port(),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		logger.Error("server failed", slog.Any("error", err))
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
	}

	// otelc registers its own shutdown hook to flush telemetry, so there is no
	// provider to shut down here — only the HTTP server needs draining.
	drainCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(drainCtx); err != nil {
		logger.Error("graceful shutdown failed", slog.Any("error", err))
	}
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8090"
}
