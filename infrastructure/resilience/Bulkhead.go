// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package resilience

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// BulkheadConfig configures one compartment.
type BulkheadConfig struct {
	// Name is the compartment: a work class (automation, worker) or the pool in front of one
	// dependency (s3, smtp). It becomes a metric label, so it stays short and identifier-free
	// (rule 10).
	Name string
	// Capacity is how many calls may run at once. Below one it would admit nobody, so the
	// constructor raises it to one - a compartment that lets nothing through is a defect, and
	// failing closed here would take out the work it is meant to protect.
	Capacity int
	// WaitFor is how long a caller may queue for a slot. Zero means fail fast, which is the
	// right answer on the interactive path: a queue is latency the user pays for without being
	// told. Background work can afford to wait.
	WaitFor time.Duration
	// OnRejected is called for each rejected call, for the metric.
	OnRejected func(name string)
}

// Bulkhead bounds how many calls run at once, so that one kind of work cannot consume the
// capacity of another (observability-reliability.md §6). A runaway automation rule must not
// starve the interactive path; separate compartments are what stops it.
//
// It is a counter of slots, not a worker pool: the caller's own goroutine does the work. The
// bulkhead only decides whether it may start.
type Bulkhead struct {
	cfg   BulkheadConfig
	slots chan struct{}
}

// NewBulkhead builds a compartment of the given capacity.
func NewBulkhead(cfg BulkheadConfig) *Bulkhead {
	if cfg.Capacity < 1 {
		cfg.Capacity = 1
	}
	return &Bulkhead{cfg: cfg, slots: make(chan struct{}, cfg.Capacity)}
}

// Do runs fn if a slot is free, waits for one for as long as WaitFor allows, and otherwise
// rejects the call.
func (b *Bulkhead) Do(ctx context.Context, fn func(context.Context) error) error {
	if err := b.acquire(ctx); err != nil {
		return err
	}
	// The slot is released even if fn panics. A panic is recovered further up (SafeGo, the
	// recovery middleware), and a compartment that leaks a slot per panic quietly shrinks to
	// nothing - the kind of failure that shows up as "it gets slower every week".
	defer func() { <-b.slots }()

	return fn(ctx)
}

// InFlight is how many calls are running. The saturation metric reads it.
func (b *Bulkhead) InFlight() int { return len(b.slots) }

// Capacity is how many may run at once.
func (b *Bulkhead) Capacity() int { return b.cfg.Capacity }

// Name is the compartment's name.
func (b *Bulkhead) Name() string { return b.cfg.Name }

func (b *Bulkhead) acquire(ctx context.Context) error {
	// The non-blocking attempt first: with a free slot, neither a timer nor a second select
	// is needed, and that is the ordinary case.
	select {
	case b.slots <- struct{}{}:
		return nil
	default:
	}

	if b.cfg.WaitFor <= 0 {
		return b.reject()
	}

	timer := time.NewTimer(b.cfg.WaitFor)
	defer timer.Stop()

	select {
	case b.slots <- struct{}{}:
		return nil
	case <-timer.C:
		return b.reject()
	case <-ctx.Done():
		// The caller's deadline or cancellation. That is not saturation, and reporting it as
		// such would put our own compartment in the dock for somebody else's timeout.
		return ctx.Err()
	}
}

func (b *Bulkhead) reject() error {
	if b.cfg.OnRejected != nil {
		b.cfg.OnRejected(b.cfg.Name)
	}
	return SaturatedError(b.cfg.Name)
}

// SaturatedError is what a rejected call gets: UNAVAILABLE, hence a 503, because the same call
// a moment later may well succeed (api-guidelines.md §6).
func SaturatedError(name string) *shared.Error {
	return shared.ErrUnavailable.
		WithDetail("dependency.saturated").
		WithParams(map[string]string{"dependency": name})
}
