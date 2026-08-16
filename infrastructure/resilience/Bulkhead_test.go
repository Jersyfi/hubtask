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

func TestBulkheadAdmitsUpToItsCapacity(t *testing.T) {
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{Name: "automation", Capacity: 2})

	admitted := 0
	// The nested calls are the concurrent ones: each outer call still holds its slot while the
	// inner one asks for another.
	err := bulkhead.Do(context.Background(), func(ctx context.Context) error {
		admitted++
		return bulkhead.Do(ctx, func(ctx context.Context) error {
			admitted++
			return bulkhead.Do(ctx, func(context.Context) error {
				admitted++
				return nil
			})
		})
	})

	if admitted != 2 {
		t.Errorf("admitted = %d, want 2", admitted)
	}
	domainErr := shared.AsError(err)
	if domainErr.DetailCode != "dependency.saturated" {
		t.Errorf("detail code = %q, want dependency.saturated", domainErr.DetailCode)
	}
	if domainErr.Category != shared.CategoryUnavailable {
		t.Errorf("category = %s, want %s", domainErr.Category, shared.CategoryUnavailable)
	}
	if domainErr.Params["dependency"] != "automation" {
		t.Errorf("dependency parameter = %q, want automation", domainErr.Params["dependency"])
	}
}

func TestBulkheadReleasesTheSlotAfterTheCall(t *testing.T) {
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{Name: "s3", Capacity: 1})

	for i := range 5 {
		if err := bulkhead.Do(context.Background(), func(context.Context) error { return nil }); err != nil {
			t.Fatalf("call %d was rejected: %v", i, err)
		}
	}
	if got := bulkhead.InFlight(); got != 0 {
		t.Errorf("in flight = %d after the calls returned, want 0", got)
	}
}

// A compartment that leaks a slot per panic quietly shrinks to nothing - the kind of failure
// that shows up as "it gets slower every week".
func TestBulkheadReleasesTheSlotWhenTheCallPanics(t *testing.T) {
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{Name: "worker", Capacity: 1})

	func() {
		defer func() { _ = recover() }()
		_ = bulkhead.Do(context.Background(), func(context.Context) error {
			panic("in the call")
		})
	}()

	if got := bulkhead.InFlight(); got != 0 {
		t.Fatalf("in flight = %d after a panic, want 0", got)
	}
	if err := bulkhead.Do(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Errorf("the compartment stayed blocked after a panic: %v", err)
	}
}

func TestBulkheadRejectsImmediatelyWithoutAQueue(t *testing.T) {
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{Name: "api", Capacity: 1})

	start := time.Now()
	err := bulkhead.Do(context.Background(), func(ctx context.Context) error {
		return bulkhead.Do(ctx, func(context.Context) error { return nil })
	})
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("the rejection took %v - a caller waited without a queue", elapsed)
	}
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("error = %v, want the saturation error", err)
	}
}

func TestBulkheadWaitsForTheConfiguredTimeBeforeRejecting(t *testing.T) {
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{
		Name: "worker", Capacity: 1, WaitFor: 30 * time.Millisecond,
	})

	start := time.Now()
	err := bulkhead.Do(context.Background(), func(ctx context.Context) error {
		return bulkhead.Do(ctx, func(context.Context) error { return nil })
	})
	elapsed := time.Since(start)

	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("error = %v, want the saturation error", err)
	}
	if elapsed < 30*time.Millisecond {
		t.Errorf("the caller queued for %v, want at least 30ms", elapsed)
	}
}

// Somebody else's deadline is not our saturation.
func TestBulkheadReportsTheCallersCancellationAsItsOwn(t *testing.T) {
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{
		Name: "worker", Capacity: 1, WaitFor: time.Hour,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := bulkhead.Do(ctx, func(inner context.Context) error {
		return bulkhead.Do(inner, func(context.Context) error { return nil })
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want the caller's deadline", err)
	}
	if shared.AsError(err).DetailCode == "dependency.saturated" {
		t.Error("the caller's deadline was reported as saturation")
	}
}

func TestBulkheadCountsRejections(t *testing.T) {
	rejected := 0
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{
		Name: "automation", Capacity: 1,
		OnRejected: func(name string) {
			if name != "automation" {
				t.Errorf("name = %q, want automation", name)
			}
			rejected++
		},
	})

	_ = bulkhead.Do(context.Background(), func(ctx context.Context) error {
		_ = bulkhead.Do(ctx, func(context.Context) error { return nil })
		return bulkhead.Do(ctx, func(context.Context) error { return nil })
	})

	if rejected != 2 {
		t.Errorf("rejections = %d, want 2", rejected)
	}
}

// A compartment that admits nobody is a defect, and failing closed would take out the work it
// exists to protect.
func TestBulkheadWithoutACapacityStillAdmitsOne(t *testing.T) {
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{Name: "worker"})

	if got := bulkhead.Capacity(); got != 1 {
		t.Errorf("capacity = %d, want 1", got)
	}
	if err := bulkhead.Do(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Errorf("the compartment admitted nobody: %v", err)
	}
}
