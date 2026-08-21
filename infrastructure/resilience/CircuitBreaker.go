// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package resilience

import (
	"context"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// BreakerState is the state of one breaker. The numbers are the values of
// hubtask_circuit_breaker_state (observability-reliability.md §4), the names are what
// /meta/health reports as CircuitState - one type owns both, so the two cannot drift.
type BreakerState int

const (
	// BreakerClosed lets everything through. The normal state.
	BreakerClosed BreakerState = iota
	// BreakerHalfOpen lets a probe through to find out whether the dependency is back.
	BreakerHalfOpen
	// BreakerOpen rejects immediately, without a call and without a thread waiting for one.
	BreakerOpen
)

func (s BreakerState) String() string {
	switch s {
	case BreakerClosed:
		return "closed"
	case BreakerHalfOpen:
		return "half_open"
	case BreakerOpen:
		return "open"
	default:
		return "unknown"
	}
}

// Level is the numeric value for the gauge: 0 closed, 1 half-open, 2 open.
func (s BreakerState) Level() int64 { return int64(s) }

// BreakerConfig configures one breaker. One per external dependency
// (observability-reliability.md §6) - object storage, SMTP, an AI provider, a webhook target -
// because a breaker shared between two of them reports the wrong one as broken.
type BreakerConfig struct {
	// Dependency names the guarded dependency. It becomes a metric label and a health entry,
	// so it is short, stable, and free of identifiers (rule 10).
	Dependency string
	// FailureThreshold is how many consecutive failures open the breaker.
	FailureThreshold int
	// SuccessThreshold is how many consecutive successful probes close it again. More than one
	// protects against a dependency that answers the first request after a restart and then
	// falls over again.
	SuccessThreshold int
	// OpenFor is the cool-down before the first probe is allowed through.
	OpenFor time.Duration
	// HalfOpenProbes is how many calls may run at once while probing. One is almost always
	// right: the point of the probe is to ask, not to load-test a recovering dependency.
	HalfOpenProbes int
	// Failure decides which errors count against the dependency. Nil means Retryable - what is
	// worth retrying is exactly what a breaker exists to stop retrying forever. A 422 from a
	// webhook target is the target working correctly, and it must not open anything.
	Failure func(error) bool
	// Now is the clock. Nil means time.Now; a test injects its own rather than sleeping.
	Now func() time.Time
	// OnStateChange is called after every transition, outside the lock. The metric hangs off
	// this, and so does the health report.
	OnStateChange func(dependency string, state BreakerState)
}

// Breaker guards one dependency. The zero value is not usable; build one with NewBreaker.
type Breaker struct {
	cfg BreakerConfig

	mu        sync.Mutex
	state     BreakerState
	failures  int
	successes int
	openedAt  time.Time
	probes    int
	changedAt time.Time
}

// NewBreaker builds a breaker, filling in the defaults for anything left at zero. It never
// returns an error: a misconfigured breaker that refuses to be built would take the dependency
// down at startup, which is the outage it exists to prevent.
func NewBreaker(cfg BreakerConfig) *Breaker {
	if cfg.FailureThreshold < 1 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold < 1 {
		cfg.SuccessThreshold = 2
	}
	if cfg.OpenFor <= 0 {
		cfg.OpenFor = 30 * time.Second
	}
	if cfg.HalfOpenProbes < 1 {
		cfg.HalfOpenProbes = 1
	}
	if cfg.Failure == nil {
		cfg.Failure = Retryable
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Breaker{cfg: cfg, changedAt: cfg.Now()}
}

// Do runs fn unless the breaker is open. An open breaker returns immediately - that is the whole
// point: a caller waiting on a dependency that is known to be down is a thread doing nothing,
// and enough of them are an outage of their own.
func (b *Breaker) Do(ctx context.Context, fn func(context.Context) error) error {
	if err := b.enter(); err != nil {
		return err
	}
	err := fn(ctx)
	b.leave(err)
	return err
}

// State reports the current state, moving an expired cool-down to half-open on the way. The
// health report and the metric read it, and neither of them makes a call - so the transition
// cannot wait for the next caller to notice it.
func (b *Breaker) State() BreakerState {
	b.mu.Lock()
	changed := b.expireCoolDown()
	state := b.state
	b.mu.Unlock()

	if changed {
		b.notify(state)
	}
	return state
}

// Since is when the current state began. /meta/health reports it, so that an operator sees how
// long a dependency has been out rather than only that it is.
func (b *Breaker) Since() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.changedAt
}

// Dependency is the guarded dependency's name.
func (b *Breaker) Dependency() string { return b.cfg.Dependency }

// enter decides whether this call may run.
func (b *Breaker) enter() error {
	b.mu.Lock()
	changed := b.expireCoolDown()
	state := b.state

	var err error
	switch state {
	case BreakerOpen:
		err = OpenCircuitError(b.cfg.Dependency)
	case BreakerHalfOpen:
		if b.probes >= b.cfg.HalfOpenProbes {
			// Somebody else is already asking. A second probe tells us nothing new and puts
			// load on a dependency that has just come back.
			err = OpenCircuitError(b.cfg.Dependency)
		} else {
			b.probes++
		}
	case BreakerClosed:
		// Nothing to decide.
	}
	b.mu.Unlock()

	if changed {
		b.notify(state)
	}
	return err
}

// leave records the outcome of a call that was let through.
func (b *Breaker) leave(err error) {
	b.mu.Lock()
	if b.state == BreakerHalfOpen && b.probes > 0 {
		b.probes--
	}

	failed := err != nil && b.cfg.Failure(err)
	before := b.state

	switch {
	case failed && b.state == BreakerHalfOpen:
		// The dependency is not back after all. Straight to open, and the cool-down starts
		// again from now - stepping back to closed here would let the next caller in
		// immediately and turn every request into a probe.
		b.trip()
	case failed:
		b.failures++
		b.successes = 0
		if b.failures >= b.cfg.FailureThreshold {
			b.trip()
		}
	case b.state == BreakerHalfOpen:
		b.successes++
		if b.successes >= b.cfg.SuccessThreshold {
			b.transition(BreakerClosed)
			b.failures, b.successes = 0, 0
		}
	default:
		// A success in the closed state. The failure count is consecutive, so it resets.
		b.failures = 0
	}

	after := b.state
	b.mu.Unlock()

	if before != after {
		b.notify(after)
	}
}

// expireCoolDown moves an open breaker to half-open once the cool-down has passed. The caller
// holds the lock; the return value says whether a notification is owed.
func (b *Breaker) expireCoolDown() bool {
	if b.state != BreakerOpen || b.cfg.Now().Sub(b.openedAt) < b.cfg.OpenFor {
		return false
	}
	b.transition(BreakerHalfOpen)
	b.successes, b.probes = 0, 0
	return true
}

// trip opens the breaker and restarts the cool-down. The caller holds the lock.
func (b *Breaker) trip() {
	b.transition(BreakerOpen)
	b.openedAt = b.cfg.Now()
	b.failures, b.successes, b.probes = 0, 0, 0
}

// transition sets the state and stamps it. The caller holds the lock.
func (b *Breaker) transition(to BreakerState) {
	if b.state == to {
		return
	}
	b.state = to
	b.changedAt = b.cfg.Now()
}

// notify runs the observer outside the lock: it publishes a metric, and a metric export that
// blocks must not take the breaker with it.
func (b *Breaker) notify(state BreakerState) {
	if b.cfg.OnStateChange != nil {
		b.cfg.OnStateChange(b.cfg.Dependency, state)
	}
}

// OpenCircuitError is what a rejected call gets: UNAVAILABLE, so the REST layer answers 503 and
// the client knows to come back later rather than to fix its request (api-guidelines.md §6).
func OpenCircuitError(dependency string) *shared.Error {
	return shared.ErrUnavailable.
		WithDetail("dependency.circuit_open").
		WithParams(map[string]string{"dependency": dependency})
}
