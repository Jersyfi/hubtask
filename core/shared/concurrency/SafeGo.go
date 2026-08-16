// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package concurrency is the only place in the project where a `go` statement may appear.
//
// Background: a panic in a bare goroutine kills the whole process, no matter how carefully
// the request path is guarded. That defeats the "should essentially never crash" goal
// (ADR-0016). The architecture test in test/architecture enforces that no `go` statement
// occurs outside this package.
package concurrency

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
)

// PanicObserver is called for every recovered panic. The metric
// hubtask_panics_recovered_total hangs off this - its target value is permanently 0.
type PanicObserver func(component string, recovered any)

var (
	mu       sync.RWMutex
	observer PanicObserver = func(string, any) {}
)

// SetPanicObserver is set once at startup by the observability layer.
func SetPanicObserver(o PanicObserver) {
	mu.Lock()
	defer mu.Unlock()
	if o != nil {
		observer = o
	}
}

// Go runs fn concurrently. A panic does not terminate the process; it is logged and
// reported instead. component names the location for the metric and the log.
func Go(ctx context.Context, component string, fn func(context.Context)) {
	go func() {
		defer Recover(ctx, component)
		fn(ctx)
	}()
}

// Recover is the reusable guard for request handlers and job execution.
// Usage: defer concurrency.Recover(ctx, "rest.handler")
func Recover(ctx context.Context, component string) {
	r := recover()
	if r == nil {
		return
	}
	mu.RLock()
	obs := observer
	mu.RUnlock()
	obs(component, r)

	slog.ErrorContext(ctx, "panic recovered",
		slog.String("component", component),
		slog.String("panic", fmt.Sprint(r)),
		slog.String("stack", string(debug.Stack())),
	)
}
