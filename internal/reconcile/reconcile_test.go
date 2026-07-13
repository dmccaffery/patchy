// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package reconcile

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunExecutesImmediatelyAndRepeats(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())

	var passes atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, log, "test", time.Millisecond, func(context.Context) error {
			if passes.Add(1) >= 3 {
				cancel()
			}
			return nil
		})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
	if got := passes.Load(); got < 3 {
		t.Errorf("passes = %d, want >= 3", got)
	}
}

func TestRunSurvivesErrors(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())

	var passes atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, log, "test", time.Millisecond, func(context.Context) error {
			if passes.Add(1) >= 2 {
				cancel()
			}
			return errors.New("boom")
		})
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run stopped retrying after an error")
	}
	if got := passes.Load(); got < 2 {
		t.Errorf("passes = %d, want >= 2 (loop must survive fn errors)", got)
	}
}

func TestJitterBounds(t *testing.T) {
	interval := time.Second
	for range 100 {
		got := jitter(interval)
		if got < 900*time.Millisecond || got > 1100*time.Millisecond {
			t.Fatalf("jitter(%v) = %v, want within ±10%%", interval, got)
		}
	}
}
