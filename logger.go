package main

// logger.go — Structured logging setup using log/slog (stdlib, Go 1.21+).
// Provides setupLogger() for initializing the global slog logger,
// and timeOp() for measuring and logging operation durations.

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// setupLogger configures the global slog logger.
// In dev mode (wails dev), log at Debug level for full per-file timing.
// In production builds, log at Info level (scan summaries only).
func setupLogger(debug bool) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

// timeOp returns a closure that, when called, logs the elapsed time since
// timeOp was invoked. Designed for use with defer:
//
//	defer timeOp("scan_walk", slog.String("folder", path))()
//
// Note the trailing () — timeOp returns a closure which must be invoked by defer.
func timeOp(operation string, attrs ...slog.Attr) func() {
	start := time.Now()
	return func() {
		elapsed := time.Since(start)
		allAttrs := append(attrs, slog.Int64("duration_ms", elapsed.Milliseconds()))
		slog.LogAttrs(context.Background(), slog.LevelInfo, operation, allAttrs...)
	}
}
