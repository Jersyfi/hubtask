// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package resilience holds the building blocks from observability-reliability.md §6: timeouts,
// retry, a circuit breaker, a bulkhead, and a load shedder.
//
// They live here, once, so that every adapter composes them instead of reinventing them
// (ADR-0016). Each block is a plain value with no global state: two callers get two breakers,
// and a test gets its own without touching a package variable.
//
// What they all have in common is the error they produce. A blocked, timed-out, or shed call is
// UNAVAILABLE with a detail code from the `dependency.` family - "later", not "wrong" - which
// the REST layer turns into a 503 with a stable code (api-guidelines.md §6).
package resilience

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Do runs fn under a deadline. It is the answer to rule 7 of CLAUDE.md - no call without a
// timeout - in the form an adapter can actually use.
//
// budget bounds this call; an earlier deadline already on ctx wins, because a request that has
// 200 ms left must not be kept waiting for a second by an adapter's own generosity.
//
// Do relies on fn honouring the context it is handed. It deliberately does not race fn against
// a timer in a goroutine: that would return to the caller while the call is still running,
// leaving the resource it holds - a connection, a lock - behind with nobody waiting for it. A
// dependency that cannot be cancelled needs a bulkhead, not a fake deadline.
func Do(ctx context.Context, dependency string, budget time.Duration, fn func(context.Context) error) error {
	if budget <= 0 {
		// A non-positive budget is a defect at the call site, not a configuration to honour:
		// context.WithTimeout would produce a context that is already expired, and every call
		// through it would fail as a dependency timeout. That would report the wrong culprit.
		return shared.Internalf("resilience: %s called without a timeout budget", dependency)
	}

	callCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	err := fn(callCtx)
	if err == nil {
		return nil
	}
	return classify(ctx, dependency, err)
}

// DoValue is Do for a call that returns something. Two functions rather than one generic over
// the empty struct: an adapter that returns a value should not have to write `_, err :=`.
func DoValue[T any](ctx context.Context, dependency string, budget time.Duration, fn func(context.Context) (T, error)) (T, error) {
	var value T
	err := Do(ctx, dependency, budget, func(callCtx context.Context) error {
		var innerErr error
		value, innerErr = fn(callCtx)
		return innerErr
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return value, nil
}

// classify separates "the dependency was too slow" from "the caller went away". Both arrive as
// context.DeadlineExceeded or context.Canceled, and the difference decides whether this is a
// 503 for the client or nothing at all - a client that hung up needs no error mapped for it,
// and counting its departure as a dependency failure would page somebody for a closed browser
// tab.
func classify(parent context.Context, dependency string, err error) error {
	if parent.Err() != nil {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return TimeoutError(dependency, err)
	}
	return err
}

// TimeoutError is the typed error of an exceeded deadline. Exported because an adapter that
// enforces its own deadline - a driver with a connect timeout, say - should produce the same
// error as Do rather than a second dialect of "too slow".
func TimeoutError(dependency string, cause error) *shared.Error {
	return shared.ErrUnavailable.
		WithDetail("dependency.timeout").
		WithParams(map[string]string{"dependency": dependency}).
		WithCause(cause)
}

// Remaining reports how much of the context's deadline is left, capped at budget. An adapter
// that has to hand a duration to a library - a driver, a mail client - reads it here instead of
// passing its configured value and quietly outliving the request.
//
// A context without a deadline yields the full budget: that is the background path, where the
// budget is the only bound there is.
func Remaining(ctx context.Context, budget time.Duration) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return budget
	}
	left := time.Until(deadline)
	if left < budget {
		return left
	}
	return budget
}
