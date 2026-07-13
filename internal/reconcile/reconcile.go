// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package reconcile

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const scopeName = "github.com/bitwise-media-group/patchy/internal/reconcile"

var tracer = sync.OnceValue(func() trace.Tracer {
	return otel.Tracer(scopeName)
})

var passDuration = sync.OnceValue(func() metric.Float64Histogram {
	h, err := otel.Meter(scopeName).Float64Histogram("patchy.reconcile.duration",
		metric.WithDescription("duration of one reconcile pass"),
		metric.WithUnit("s"))
	if err != nil {
		// The no-op meter never errors; a real provider failing to build an
		// instrument still returns a usable no-op instrument.
		otel.Handle(err)
	}
	return h
})

// Run executes fn immediately, then every interval (±10% jitter so replicas
// and controllers don't thundering-herd the GitHub API) until ctx is
// cancelled. A failing pass is logged and retried on the next tick — errors
// never stop the loop. Run returns ctx.Err() once cancelled.
func Run(
	ctx context.Context, log *slog.Logger, name string, interval time.Duration, fn func(context.Context) error,
) error {
	for {
		pass(ctx, log, name, fn)

		timer := time.NewTimer(jitter(interval))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func pass(ctx context.Context, log *slog.Logger, name string, fn func(context.Context) error) {
	ctx, span := tracer().Start(ctx, "patchy.reconcile.pass",
		trace.WithAttributes(attribute.String("reconciler", name)))
	defer span.End()

	start := time.Now()
	err := fn(ctx)
	elapsed := time.Since(start)
	passDuration().Record(ctx, elapsed.Seconds(),
		metric.WithAttributes(attribute.String("reconciler", name), attribute.Bool("error", err != nil)))

	if err != nil && ctx.Err() == nil {
		span.SetStatus(codes.Error, err.Error())
		log.LogAttrs(ctx, slog.LevelError, "reconcile pass failed",
			slog.String("reconciler", name),
			slog.Duration("elapsed", elapsed),
			slog.Any("error", err))
		return
	}
	log.LogAttrs(ctx, slog.LevelDebug, "reconcile pass complete",
		slog.String("reconciler", name),
		slog.Duration("elapsed", elapsed))
}

// jitter spreads an interval to interval ± 10%.
func jitter(interval time.Duration) time.Duration {
	spread := 0.9 + 0.2*rand.Float64()
	return time.Duration(float64(interval) * spread)
}
