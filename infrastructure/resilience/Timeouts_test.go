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

func TestDoPassesADeadlineToTheCall(t *testing.T) {
	err := resilience.Do(context.Background(), "s3", time.Second, func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("the call received a context without a deadline")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The budget bounds this call; it never extends what the request already granted.
func TestDoDoesNotExtendAnEarlierDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	var left time.Duration
	err := resilience.Do(parent, "s3", time.Hour, func(ctx context.Context) error {
		deadline, _ := ctx.Deadline()
		left = time.Until(deadline)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if left > time.Second {
		t.Errorf("the budget extended the parent deadline: %v left", left)
	}
}

func TestDoReportsAnExceededDeadlineAsUnavailable(t *testing.T) {
	err := resilience.Do(context.Background(), "smtp", time.Millisecond, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	domainErr := shared.AsError(err)
	if domainErr.Category != shared.CategoryUnavailable {
		t.Errorf("category = %s, want %s", domainErr.Category, shared.CategoryUnavailable)
	}
	if domainErr.DetailCode != "dependency.timeout" {
		t.Errorf("detail code = %q, want dependency.timeout", domainErr.DetailCode)
	}
	if domainErr.Params["dependency"] != "smtp" {
		t.Errorf("dependency parameter = %q, want smtp", domainErr.Params["dependency"])
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("the technical cause was lost - the log needs it")
	}
}

// A client that hangs up is not a broken dependency. Counting it as one would page somebody for
// a closed browser tab.
func TestDoDoesNotBlameTheDependencyWhenTheCallerLeaves(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	err := resilience.Do(parent, "s3", time.Second, func(ctx context.Context) error {
		return ctx.Err()
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if domainErr := shared.AsError(err); domainErr.DetailCode == "dependency.timeout" {
		t.Error("the caller's cancellation was reported as a dependency timeout")
	}
}

// A missing budget is a defect at the call site, and it must not be reported as the
// dependency's fault.
func TestDoRejectsACallWithoutABudget(t *testing.T) {
	for _, budget := range []time.Duration{0, -time.Second} {
		called := false
		err := resilience.Do(context.Background(), "s3", budget, func(context.Context) error {
			called = true
			return nil
		})
		if called {
			t.Errorf("budget %v: the call ran anyway", budget)
		}
		if got := shared.AsError(err).Category; got != shared.CategoryInternal {
			t.Errorf("budget %v: category = %s, want %s", budget, got, shared.CategoryInternal)
		}
	}
}

func TestDoValueReturnsTheValueOrTheZeroValue(t *testing.T) {
	value, err := resilience.DoValue(context.Background(), "s3", time.Second,
		func(context.Context) (string, error) { return "payload", nil })
	if err != nil || value != "payload" {
		t.Fatalf("got (%q, %v), want (payload, nil)", value, err)
	}

	failure := errors.New("boom")
	value, err = resilience.DoValue(context.Background(), "s3", time.Second,
		func(context.Context) (string, error) { return "half a payload", failure })
	if !errors.Is(err, failure) {
		t.Errorf("error = %v, want %v", err, failure)
	}
	if value != "" {
		t.Errorf("value = %q, want the zero value - a failed call returns nothing usable", value)
	}
}

func TestRemainingIsTheSmallerOfTheTwo(t *testing.T) {
	t.Run("no deadline yields the full budget", func(t *testing.T) {
		if got := resilience.Remaining(context.Background(), time.Minute); got != time.Minute {
			t.Errorf("remaining = %v, want %v", got, time.Minute)
		}
	})

	t.Run("an earlier deadline wins", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if got := resilience.Remaining(ctx, time.Minute); got > 50*time.Millisecond {
			t.Errorf("remaining = %v, want at most 50ms", got)
		}
	})

	t.Run("a later deadline does not raise the budget", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		if got := resilience.Remaining(ctx, time.Second); got != time.Second {
			t.Errorf("remaining = %v, want %v", got, time.Second)
		}
	})
}
