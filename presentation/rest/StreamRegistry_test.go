// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"sync"
	"testing"

	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

func registry(limits StreamLimits) *StreamRegistry { return NewStreamRegistry(limits) }

func admit(t *testing.T, r *StreamRegistry, credential, tenant string) StreamSlot {
	t.Helper()
	slot, refusal := r.Admit(credential, tenant)
	if refusal != RefusedNone {
		t.Fatalf("refused with %q", refusal)
	}
	return slot
}

// One client reconnecting in a loop must not hold a hundred sockets it never reads.
func TestOneCredentialIsCappedOnItsOwn(t *testing.T) {
	r := registry(StreamLimits{PerCredential: 2, PerTenant: 10, PerProcess: 10})

	first := admit(t, r, "anna", "workspace")
	second := admit(t, r, "anna", "workspace")

	if _, refusal := r.Admit("anna", "workspace"); refusal != RefusedCredential {
		t.Errorf("the third stream was refused with %q, want the credential cap", refusal)
	}
	// Somebody else in the same workspace is unaffected: the cap is per credential.
	if _, refusal := r.Admit("bert", "workspace"); refusal != RefusedNone {
		t.Errorf("another client was refused with %q", refusal)
	}

	// And giving one back makes room again.
	first.Release()
	if _, refusal := r.Admit("anna", "workspace"); refusal != RefusedNone {
		t.Errorf("a released slot did not make room: %q", refusal)
	}
	second.Release()
}

// One busy workspace must not take a whole pod's capacity from everybody else on it.
func TestOneWorkspaceIsCappedAcrossItsClients(t *testing.T) {
	r := registry(StreamLimits{PerCredential: 10, PerTenant: 2, PerProcess: 10})

	admit(t, r, "anna", "workspace")
	admit(t, r, "bert", "workspace")

	if _, refusal := r.Admit("carla", "workspace"); refusal != RefusedTenant {
		t.Errorf("the third client was refused with %q, want the tenant cap", refusal)
	}
	if _, refusal := r.Admit("carla", "another"); refusal != RefusedNone {
		t.Errorf("another workspace was refused with %q", refusal)
	}
}

// The load shedding of observability-reliability.md §6, applied to the resource a stream consumes:
// above the threshold new connections are refused, and the ones already open are never dropped to
// make room - shedding is about not accepting more work, not about abandoning work in hand.
func TestTheProcessCapRefusesRatherThanDropping(t *testing.T) {
	r := registry(StreamLimits{PerCredential: 10, PerTenant: 10, PerProcess: 2})

	held := admit(t, r, "anna", "workspace")
	admit(t, r, "bert", "another")

	if _, refusal := r.Admit("carla", "a third"); refusal != RefusedProcess {
		t.Errorf("refused with %q, want the process cap", refusal)
	}
	select {
	case <-held.Closing:
		t.Error("an open stream was asked to close to make room for a new one")
	default:
	}
	if r.Open() != 2 {
		t.Errorf("%d streams open, want the two that were admitted", r.Open())
	}
}

// A limit of zero is no limit: an installation that does not want one says so by not setting it.
func TestALimitOfZeroIsNoLimit(t *testing.T) {
	r := registry(StreamLimits{})

	for range 50 {
		if _, refusal := r.Admit("anna", "workspace"); refusal != RefusedNone {
			t.Fatalf("refused with %q where no limit is configured", refusal)
		}
	}
}

// SIGTERM: every stream is asked to end, and nothing new is accepted. A stream admitted after the
// signal and closed a moment later would look to a client like a flapping server rather than one
// going away.
func TestClosingAllEndsEveryStreamAndRefusesNewOnes(t *testing.T) {
	r := registry(StreamLimits{PerCredential: 10, PerTenant: 10, PerProcess: 10})

	first := admit(t, r, "anna", "workspace")
	second := admit(t, r, "bert", "another")

	r.CloseAll()

	for name, slot := range map[string]StreamSlot{"first": first, "second": second} {
		select {
		case <-slot.Closing:
		default:
			t.Errorf("the %s stream was not asked to close", name)
		}
	}
	if _, refusal := r.Admit("carla", "a third"); refusal != RefusedDraining {
		t.Errorf("a stream opened during the drain, refused with %q", refusal)
	}
	if !r.Draining() {
		t.Error("the registry does not report itself draining")
	}

	// The gauge stays honest: a stream is counted until its handler has actually returned. One
	// that dropped to zero on the signal would say the drain was over while connections were
	// still finishing, which is the number an operator watches during a rollout.
	if r.Open() != 2 {
		t.Errorf("%d streams open right after the signal, want the two still finishing", r.Open())
	}
	first.Release()
	second.Release()
	if r.Open() != 0 {
		t.Errorf("%d streams open after both returned", r.Open())
	}
}

// A shutdown path can run on two signals, and closing a closed channel panics.
func TestClosingAllTwiceIsHarmless(t *testing.T) {
	r := registry(StreamLimits{PerProcess: 4})
	admit(t, r, "anna", "workspace")

	r.CloseAll()
	r.CloseAll()
}

// A slot that outlives its connection is a stream this process refuses to open for the rest of its
// life, so releasing twice must not free two.
func TestReleasingTwiceFreesOneSlot(t *testing.T) {
	r := registry(StreamLimits{PerCredential: 1, PerTenant: 4, PerProcess: 4})

	slot := admit(t, r, "anna", "workspace")
	slot.Release()
	slot.Release()

	admit(t, r, "anna", "workspace")
	if _, refusal := r.Admit("anna", "workspace"); refusal != RefusedCredential {
		t.Errorf("releasing twice freed two slots: %q", refusal)
	}
}

// The counters go back to absent rather than to zero: a pod that served one stream for every
// tenant it has ever seen would otherwise keep a map entry per tenant for its whole life.
func TestTheCountersDoNotGrowForever(t *testing.T) {
	r := registry(StreamLimits{PerCredential: 4, PerTenant: 4, PerProcess: 8})

	for i := range 100 {
		tenant := string(rune('a' + i%26))
		slot := admit(t, r, "anna-"+tenant, tenant)
		slot.Release()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.perTenant) != 0 || len(r.perCredential) != 0 {
		t.Errorf("%d tenant and %d credential counters left behind",
			len(r.perTenant), len(r.perCredential))
	}
}

// Admit and Release run on request goroutines and CloseAll on the shutdown path. The race detector
// is the assertion here.
func TestTheRegistryIsSafeUnderConcurrentUse(t *testing.T) {
	r := registry(StreamLimits{PerCredential: 100, PerTenant: 100, PerProcess: 100})

	// Through the guard like every other goroutine in this codebase (ADR-0016, rule 5).
	ctx := t.Context()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		concurrency.Go(ctx, "test.stream_admit", func(context.Context) {
			defer wg.Done()
			slot, refusal := r.Admit("anna", "workspace")
			if refusal == RefusedNone {
				_ = r.Open()
				slot.Release()
			}
		})
		if i == 25 {
			wg.Add(1)
			concurrency.Go(ctx, "test.stream_close_all", func(context.Context) {
				defer wg.Done()
				r.CloseAll()
			})
		}
	}
	wg.Wait()
}
