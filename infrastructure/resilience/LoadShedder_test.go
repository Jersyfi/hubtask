// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package resilience_test

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

func TestShedderAdmitsDeferrableWorkBelowTheLimit(t *testing.T) {
	shedder := resilience.NewLoadShedder(resilience.LoadShedderConfig{Limit: 2})

	first, err := shedder.Admit(resilience.ClassDeferrable)
	if err != nil {
		t.Fatalf("the first export was shed: %v", err)
	}
	second, err := shedder.Admit(resilience.ClassDeferrable)
	if err != nil {
		t.Fatalf("the second export was shed: %v", err)
	}

	if _, err := shedder.Admit(resilience.ClassDeferrable); err == nil {
		t.Error("the third export was admitted above the limit")
	}

	first()
	second()
	if got := shedder.InFlight(); got != 0 {
		t.Errorf("in flight = %d after release, want 0", got)
	}
	if _, err := shedder.Admit(resilience.ClassDeferrable); err != nil {
		t.Errorf("nothing is running and an export was still shed: %v", err)
	}
}

// A person who cannot tick off a task retries by hand, which adds load rather than removing it.
func TestShedderNeverShedsInteractiveWork(t *testing.T) {
	shedder := resilience.NewLoadShedder(resilience.LoadShedderConfig{Limit: 1})

	for i := range 50 {
		if _, err := shedder.Admit(resilience.ClassInteractive); err != nil {
			t.Fatalf("interactive request %d was shed: %v", i, err)
		}
	}
}

// The load an export competes with is the whole load, not just the load from other exports.
func TestShedderCountsInteractiveWorkTowardsTheThreshold(t *testing.T) {
	shedder := resilience.NewLoadShedder(resilience.LoadShedderConfig{Limit: 2})

	if _, err := shedder.Admit(resilience.ClassInteractive); err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if _, err := shedder.Admit(resilience.ClassInteractive); err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if _, err := shedder.Admit(resilience.ClassDeferrable); err == nil {
		t.Error("the export was admitted although the process was already at its limit")
	}
}

func TestShedErrorCarriesTheWaitForTheClient(t *testing.T) {
	shedder := resilience.NewLoadShedder(resilience.LoadShedderConfig{
		Limit: 1, RetryAfter: 12 * time.Second,
	})

	_, _ = shedder.Admit(resilience.ClassDeferrable)
	_, err := shedder.Admit(resilience.ClassDeferrable)

	domainErr := shared.AsError(err)
	if domainErr.Category != shared.CategoryUnavailable {
		t.Errorf("category = %s, want %s - the client did nothing wrong", domainErr.Category, shared.CategoryUnavailable)
	}
	if domainErr.DetailCode != "capacity.shed" {
		t.Errorf("detail code = %q, want capacity.shed", domainErr.DetailCode)
	}
	if got := domainErr.Params["retry_after_seconds"]; got != "12" {
		t.Errorf("retry_after_seconds = %q, want 12", got)
	}
	if got := domainErr.Params["class"]; got != string(resilience.ClassDeferrable) {
		t.Errorf("class = %q, want %s", got, resilience.ClassDeferrable)
	}
}

// Retry-After: 0 invites the client to come straight back, which is the opposite of shedding.
func TestShedErrorNeverAsksForAnImmediateRetry(t *testing.T) {
	err := resilience.ShedError(resilience.ClassDeferrable, 100*time.Millisecond)
	if got := err.Params["retry_after_seconds"]; got != "1" {
		t.Errorf("retry_after_seconds = %q, want 1", got)
	}
}

// A middleware that both defers the release and calls it on an early return would otherwise
// drive the counter below zero, and from then on nothing is ever shed.
func TestShedderIgnoresADoubleRelease(t *testing.T) {
	shedder := resilience.NewLoadShedder(resilience.LoadShedderConfig{Limit: 1})

	release, err := shedder.Admit(resilience.ClassDeferrable)
	if err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	release()
	release()
	release()

	if got := shedder.InFlight(); got != 0 {
		t.Fatalf("in flight = %d, want 0", got)
	}
	if _, err := shedder.Admit(resilience.ClassDeferrable); err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if _, err := shedder.Admit(resilience.ClassDeferrable); err == nil {
		t.Error("the counter drifted below zero - the limit no longer bites")
	}
}

// The release of a shed call is a no-op rather than nil, so a caller can defer it on every path.
func TestShedderReturnsAUsableReleaseOnRejection(t *testing.T) {
	shedder := resilience.NewLoadShedder(resilience.LoadShedderConfig{Limit: 1})

	first, _ := shedder.Admit(resilience.ClassDeferrable)
	release, err := shedder.Admit(resilience.ClassDeferrable)
	if err == nil {
		t.Fatal("the call was admitted above the limit")
	}
	if release == nil {
		t.Fatal("the release is nil - every caller would need a guard around its defer")
	}
	release()
	first()

	if got := shedder.InFlight(); got != 0 {
		t.Errorf("in flight = %d, want 0 - a shed call still occupied a slot", got)
	}
}

func TestShedderCountsWhatItSheds(t *testing.T) {
	var shed []resilience.Class
	shedder := resilience.NewLoadShedder(resilience.LoadShedderConfig{
		Limit:  1,
		OnShed: func(class resilience.Class) { shed = append(shed, class) },
	})

	_, _ = shedder.Admit(resilience.ClassDeferrable)
	_, _ = shedder.Admit(resilience.ClassDeferrable)
	_, _ = shedder.Admit(resilience.ClassInteractive)

	if len(shed) != 1 || shed[0] != resilience.ClassDeferrable {
		t.Errorf("shed = %v, want one deferrable call", shed)
	}
}

func TestShedderDoReleasesAfterTheCall(t *testing.T) {
	shedder := resilience.NewLoadShedder(resilience.LoadShedderConfig{Limit: 1})

	ran := false
	err := shedder.Do(context.Background(), resilience.ClassDeferrable, func(context.Context) error {
		ran = true
		if got := shedder.InFlight(); got != 1 {
			t.Errorf("in flight = %d during the call, want 1", got)
		}
		return nil
	})

	if err != nil || !ran {
		t.Fatalf("got (ran=%v, err=%v), want (true, nil)", ran, err)
	}
	if got := shedder.InFlight(); got != 0 {
		t.Errorf("in flight = %d after the call, want 0", got)
	}
}
