// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package resilience_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// clock is the injected time source. A breaker test that slept through its own cool-down would
// take half a minute and still be flaky.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock {
	return &clock{now: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func fail(context.Context) error    { return shared.ErrUnavailable }
func succeed(context.Context) error { return nil }

func TestBreakerOpensAfterTheThresholdAndRejectsWithoutCalling(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: "s3", FailureThreshold: 3, OpenFor: time.Minute, Now: newClock().Now,
	})

	for i := range 3 {
		if err := breaker.Do(context.Background(), fail); err == nil {
			t.Fatalf("attempt %d: expected the dependency's error", i)
		}
	}
	if got := breaker.State(); got != resilience.BreakerOpen {
		t.Fatalf("state = %s, want open", got)
	}

	called := false
	err := breaker.Do(context.Background(), func(context.Context) error {
		called = true
		return nil
	})
	if called {
		t.Error("an open breaker let the call through")
	}

	domainErr := shared.AsError(err)
	if domainErr.Category != shared.CategoryUnavailable {
		t.Errorf("category = %s, want %s", domainErr.Category, shared.CategoryUnavailable)
	}
	if domainErr.DetailCode != "dependency.circuit_open" {
		t.Errorf("detail code = %q, want dependency.circuit_open", domainErr.DetailCode)
	}
	if domainErr.Params["dependency"] != "s3" {
		t.Errorf("dependency parameter = %q, want s3", domainErr.Params["dependency"])
	}
}

// The count is of consecutive failures. A dependency that fails one call in three is unpleasant,
// but it is not down, and cutting it off would remove a feature that mostly works.
func TestBreakerCountsConsecutiveFailuresOnly(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: "s3", FailureThreshold: 3, OpenFor: time.Minute, Now: newClock().Now,
	})

	for range 5 {
		_ = breaker.Do(context.Background(), fail)
		_ = breaker.Do(context.Background(), fail)
		_ = breaker.Do(context.Background(), succeed)
	}
	if got := breaker.State(); got != resilience.BreakerClosed {
		t.Errorf("state = %s, want closed", got)
	}
}

// A 422 from a webhook target is the target working correctly. Opening on it would report our
// own malformed payload as the target being down.
func TestBreakerIgnoresErrorsThatAreNotTheDependencysFault(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: "webhook", FailureThreshold: 2, OpenFor: time.Minute, Now: newClock().Now,
	})

	for _, failure := range []error{shared.ErrValidation, shared.ErrNotFound, shared.ErrForbidden, context.Canceled} {
		for range 5 {
			_ = breaker.Do(context.Background(), func(context.Context) error { return failure })
		}
		if got := breaker.State(); got != resilience.BreakerClosed {
			t.Fatalf("%v opened the breaker: state = %s", failure, got)
		}
	}
}

func TestBreakerProbesAfterTheCoolDownAndClosesOnSuccess(t *testing.T) {
	c := newClock()
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: "s3", FailureThreshold: 1, SuccessThreshold: 2, OpenFor: 30 * time.Second, Now: c.Now,
	})

	_ = breaker.Do(context.Background(), fail)
	if got := breaker.State(); got != resilience.BreakerOpen {
		t.Fatalf("state = %s, want open", got)
	}

	c.advance(29 * time.Second)
	if got := breaker.State(); got != resilience.BreakerOpen {
		t.Errorf("state = %s before the cool-down elapsed, want open", got)
	}

	c.advance(2 * time.Second)
	if got := breaker.State(); got != resilience.BreakerHalfOpen {
		t.Fatalf("state = %s after the cool-down, want half_open", got)
	}

	// One probe is not enough: a dependency that answers the first request after a restart and
	// then falls over again would close the breaker on that one answer.
	if err := breaker.Do(context.Background(), succeed); err != nil {
		t.Fatalf("the probe was rejected: %v", err)
	}
	if got := breaker.State(); got != resilience.BreakerHalfOpen {
		t.Errorf("state = %s after one probe, want half_open", got)
	}

	if err := breaker.Do(context.Background(), succeed); err != nil {
		t.Fatalf("the second probe was rejected: %v", err)
	}
	if got := breaker.State(); got != resilience.BreakerClosed {
		t.Errorf("state = %s after two probes, want closed", got)
	}
}

func TestBreakerReopensWithAFreshCoolDownWhenTheProbeFails(t *testing.T) {
	c := newClock()
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: "s3", FailureThreshold: 1, SuccessThreshold: 1, OpenFor: 30 * time.Second, Now: c.Now,
	})

	_ = breaker.Do(context.Background(), fail)
	c.advance(31 * time.Second)
	if got := breaker.State(); got != resilience.BreakerHalfOpen {
		t.Fatalf("state = %s, want half_open", got)
	}

	_ = breaker.Do(context.Background(), fail)
	if got := breaker.State(); got != resilience.BreakerOpen {
		t.Fatalf("state = %s, want open", got)
	}

	// The cool-down starts again from the failed probe, not from the original outage.
	c.advance(29 * time.Second)
	if got := breaker.State(); got != resilience.BreakerOpen {
		t.Errorf("state = %s, want open - the cool-down was not restarted", got)
	}
}

// While a probe is in flight, a second caller learns nothing by asking as well and only puts
// load on a dependency that has just come back.
func TestBreakerLetsOnlyOneProbeThroughAtATime(t *testing.T) {
	c := newClock()
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: "s3", FailureThreshold: 1, SuccessThreshold: 1, OpenFor: time.Second, Now: c.Now,
	})

	_ = breaker.Do(context.Background(), fail)
	c.advance(2 * time.Second)

	var secondCallRan bool
	var secondErr error
	err := breaker.Do(context.Background(), func(ctx context.Context) error {
		// The nested call is the second caller: it arrives while this probe is still running.
		secondErr = breaker.Do(ctx, func(context.Context) error {
			secondCallRan = true
			return nil
		})
		return nil
	})
	if err != nil {
		t.Fatalf("the probe was rejected: %v", err)
	}
	if secondCallRan {
		t.Error("a second probe ran while the first was still in flight")
	}
	if !errors.Is(secondErr, shared.ErrUnavailable) {
		t.Errorf("the second caller got %v, want the open-circuit error", secondErr)
	}
}

func TestBreakerReportsEveryTransition(t *testing.T) {
	c := newClock()
	var states []string
	var mu sync.Mutex

	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: "s3", FailureThreshold: 1, SuccessThreshold: 1, OpenFor: time.Second, Now: c.Now,
		OnStateChange: func(dependency string, state resilience.BreakerState) {
			mu.Lock()
			defer mu.Unlock()
			if dependency != "s3" {
				t.Errorf("dependency = %q, want s3", dependency)
			}
			states = append(states, state.String())
		},
	})

	_ = breaker.Do(context.Background(), fail) // closed -> open
	c.advance(2 * time.Second)
	_ = breaker.Do(context.Background(), succeed) // open -> half_open -> closed

	mu.Lock()
	defer mu.Unlock()
	want := []string{"open", "half_open", "closed"}
	if len(states) != len(want) {
		t.Fatalf("states = %v, want %v", states, want)
	}
	for i, state := range want {
		if states[i] != state {
			t.Errorf("states = %v, want %v", states, want)
			break
		}
	}
}

// The gauge values are part of the contract with the dashboard
// (observability-reliability.md §4: 0 closed / 1 half / 2 open).
func TestBreakerStateLevelsMatchTheMetricContract(t *testing.T) {
	cases := map[resilience.BreakerState]struct {
		level int64
		name  string
	}{
		resilience.BreakerClosed:   {0, "closed"},
		resilience.BreakerHalfOpen: {1, "half_open"},
		resilience.BreakerOpen:     {2, "open"},
	}
	for state, want := range cases {
		if state.Level() != want.level {
			t.Errorf("%s: level = %d, want %d", want.name, state.Level(), want.level)
		}
		if state.String() != want.name {
			t.Errorf("String() = %q, want %q", state.String(), want.name)
		}
	}
}

// The cool-down has to advance for a reader that never makes a call - the health report and the
// metric sampler are exactly that.
func TestBreakerSinceTracksTheCurrentState(t *testing.T) {
	c := newClock()
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: "s3", FailureThreshold: 1, OpenFor: time.Minute, Now: c.Now,
	})
	start := breaker.Since()

	c.advance(time.Minute)
	_ = breaker.Do(context.Background(), fail)

	if !breaker.Since().After(start) {
		t.Error("the timestamp of the state did not move when the breaker opened")
	}
}
