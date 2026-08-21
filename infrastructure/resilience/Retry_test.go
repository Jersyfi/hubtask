// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// fast is a policy whose delays are short enough for a unit test and whose jitter is fixed, so
// that the test measures the policy rather than the dice.
func fast(attempts int) resilience.Backoff {
	return resilience.Backoff{
		Attempts: attempts,
		Base:     time.Millisecond,
		Max:      4 * time.Millisecond,
		Random:   func() float64 { return 1 },
	}
}

func TestBackoffStopsAtTheFirstSuccess(t *testing.T) {
	calls := 0
	err := fast(5).Do(context.Background(), "s3", func(context.Context) error {
		calls++
		if calls < 3 {
			return shared.ErrUnavailable
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

// The last error is what the caller gets: it says why the operation finally failed, not why it
// failed the first time.
func TestBackoffGivesUpAfterTheLastAttempt(t *testing.T) {
	calls := 0
	last := shared.ErrUnavailable.WithDetail("dependency.timeout")

	err := fast(3).Do(context.Background(), "s3", func(context.Context) error {
		calls++
		if calls < 3 {
			return shared.ErrUnavailable
		}
		return last
	})

	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
	if shared.AsError(err).DetailCode != "dependency.timeout" {
		t.Errorf("error = %v, want the error of the last attempt", err)
	}
}

func TestBackoffDoesNotRepeatAnAnswerThatWillNotChange(t *testing.T) {
	cases := map[string]error{
		"validation":     shared.ErrValidation,
		"not found":      shared.ErrNotFound,
		"conflict":       shared.ErrConflict,
		"forbidden":      shared.ErrForbidden,
		"internal":       shared.ErrInternal,
		"caller left":    context.Canceled,
		"gone":           shared.ErrGone,
		"unauthenticate": shared.ErrUnauthenticated,
	}

	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			err := fast(4).Do(context.Background(), "s3", func(context.Context) error {
				calls++
				return failure
			})
			if calls != 1 {
				t.Errorf("calls = %d, want 1 - %v was retried", calls, err)
			}
		})
	}
}

// An unclassified error is transport trouble far more often than it is a defect, and the
// attempt count bounds how wrong that guess can be.
func TestBackoffRetriesAnUntypedError(t *testing.T) {
	calls := 0
	err := fast(3).Do(context.Background(), "s3", func(context.Context) error {
		calls++
		return errors.New("connection reset by peer")
	})

	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
	if err == nil {
		t.Error("the failure was swallowed")
	}
}

// A backoff that outlives its request is a request that hangs. The dependency's error is what
// comes back, not the fact that the wait was cut short.
func TestBackoffStopsWaitingWhenTheContextEnds(t *testing.T) {
	policy := resilience.Backoff{
		Attempts: 5,
		Base:     time.Hour,
		Max:      time.Hour,
		Random:   func() float64 { return 1 },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	calls := 0
	start := time.Now()
	err := policy.Do(ctx, "s3", func(context.Context) error {
		calls++
		return shared.ErrUnavailable
	})

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("the retry waited %v past the deadline", elapsed)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("error = %v, want the dependency's error", err)
	}
}

func TestBackoffRejectsANonsensicalPolicy(t *testing.T) {
	cases := map[string]resilience.Backoff{
		"no attempts":       {Attempts: 0, Base: time.Second, Max: time.Minute},
		"no base":           {Attempts: 3, Base: 0, Max: time.Minute},
		"cap below base":    {Attempts: 3, Base: time.Minute, Max: time.Second},
		"negative base":     {Attempts: 3, Base: -time.Second, Max: time.Minute},
		"the zero policy":   {},
		"cap without base":  {Attempts: 3, Max: time.Minute},
		"base without cap":  {Attempts: 3, Base: time.Second},
		"attempts only set": {Attempts: 3},
	}

	for name, policy := range cases {
		t.Run(name, func(t *testing.T) {
			called := false
			err := policy.Do(context.Background(), "s3", func(context.Context) error {
				called = true
				return nil
			})
			if called {
				t.Error("the call ran under an invalid policy")
			}
			if got := shared.AsError(err).Category; got != shared.CategoryInternal {
				t.Errorf("category = %s, want %s", got, shared.CategoryInternal)
			}
		})
	}
}

func TestDelayGrowsExponentiallyAndIsCapped(t *testing.T) {
	policy := resilience.Backoff{
		Attempts: 6,
		Base:     100 * time.Millisecond,
		Max:      time.Second,
		Random:   func() float64 { return 1 }, // the upper end of the window
	}

	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		time.Second, // capped
		time.Second,
	}
	for i, expected := range want {
		if got := policy.Delay(i + 1); got != expected {
			t.Errorf("delay of attempt %d = %v, want %v", i+1, got, expected)
		}
	}
}

// Full jitter means the delay is drawn from the window, not equal to it - two clients that
// failed together must not retry together.
func TestDelayIsJitteredWithinTheWindow(t *testing.T) {
	policy := resilience.Backoff{Attempts: 3, Base: time.Second, Max: time.Minute}

	seen := map[time.Duration]bool{}
	for range 20 {
		d := policy.Delay(2)
		if d < 0 || d > 2*time.Second {
			t.Fatalf("delay %v is outside the window [0, 2s]", d)
		}
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Error("every draw produced the same delay - the jitter is not random")
	}
}
