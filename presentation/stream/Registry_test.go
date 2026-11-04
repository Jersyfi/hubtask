// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package stream

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

func registry(limits Limits) *Registry { return NewRegistry(limits) }

func admit(t *testing.T, r *Registry, credential, tenant string) Slot {
	t.Helper()
	slot, refusal := r.Admit(credential, tenant)
	if refusal != RefusedNone {
		t.Fatalf("refused with %q", refusal)
	}
	return slot
}

// One client reconnecting in a loop must not hold a hundred sockets it never reads.
func TestOneCredentialIsCappedOnItsOwn(t *testing.T) {
	r := registry(Limits{PerCredential: 2, PerTenant: 10, PerProcess: 10})

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
	r := registry(Limits{PerCredential: 10, PerTenant: 2, PerProcess: 10})

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
	r := registry(Limits{PerCredential: 10, PerTenant: 10, PerProcess: 2})

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
	r := registry(Limits{})

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
	r := registry(Limits{PerCredential: 10, PerTenant: 10, PerProcess: 10})

	first := admit(t, r, "anna", "workspace")
	second := admit(t, r, "bert", "another")

	r.CloseAll()

	for name, slot := range map[string]Slot{"first": first, "second": second} {
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
	r := registry(Limits{PerProcess: 4})
	admit(t, r, "anna", "workspace")

	r.CloseAll()
	r.CloseAll()
}

// A slot that outlives its connection is a stream this process refuses to open for the rest of its
// life, so releasing twice must not free two.
func TestReleasingTwiceFreesOneSlot(t *testing.T) {
	r := registry(Limits{PerCredential: 1, PerTenant: 4, PerProcess: 4})

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
	r := registry(Limits{PerCredential: 4, PerTenant: 4, PerProcess: 8})

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
	r := registry(Limits{PerCredential: 100, PerTenant: 100, PerProcess: 100})

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

func rung(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// A credential ended on any process asks the streams of its account to ask again at once, and
// those of a workspace whose status changed; nobody else's.
func TestARevocationAsksTheAccountsStreamsAgain(t *testing.T) {
	r := registry(Limits{})
	anna, _ := r.Admit("a", "workspace", "anna")
	viewer, _ := r.Admit("v", "workspace", "bert", "admin")
	other, _ := r.Admit("c", "another", "anna")
	defer anna.Release()
	defer viewer.Release()
	defer other.Release()

	r.Recheck("workspace", "anna")
	if !rung(anna.Recheck) || rung(viewer.Recheck) || rung(other.Recheck) {
		t.Fatal("the account's revocation rang the wrong streams")
	}
	r.Recheck("workspace", "admin")
	if !rung(viewer.Recheck) {
		t.Error("the viewer's revocation did not reach the view as somebody")
	}
	r.Recheck("workspace", "")
	if !rung(anna.Recheck) || !rung(viewer.Recheck) || rung(other.Recheck) {
		t.Error("the workspace's change did not ring exactly its streams")
	}

	anna.Release()
	r.Recheck("workspace", "anna") // a released slot is not rung, and nothing blocks
}

// Side connections - snapshots, live documents - hold at most half of the workspace's and the
// process's slots, so a sign-in wave leaves room for the streams; a credential's side connections
// are counted apart from its streams, so a tab with boards open still gets its stream.
func TestSideConnectionsCannotStarveTheStreams(t *testing.T) {
	r := registry(Limits{PerCredential: 2, PerTenant: 4, PerProcess: 100})
	for _, who := range []string{"anna", "bert"} {
		if _, refusal := r.AdmitSide(who, "workspace"); refusal != RefusedNone {
			t.Fatalf("a side connection of %s was refused with %q", who, refusal)
		}
	}
	if _, refusal := r.AdmitSide("carla", "workspace"); refusal != RefusedTenant {
		t.Errorf("a third side connection was refused with %q, want the side share of the tenant cap", refusal)
	}
	admit(t, r, "anna", "workspace")
	admit(t, r, "carla", "workspace")
	if _, refusal := r.Admit("dora", "workspace"); refusal != RefusedTenant {
		t.Errorf("past the workspace's cap a stream was answered %q", refusal)
	}

	// Per credential: two side connections and two streams for one client.
	own := registry(Limits{PerCredential: 2, PerTenant: 100, PerProcess: 100})
	for range 2 {
		if _, refusal := own.AdmitSide("anna", "workspace"); refusal != RefusedNone {
			t.Fatalf("a side connection was refused with %q", refusal)
		}
	}
	if _, refusal := own.AdmitSide("anna", "workspace"); refusal != RefusedCredential {
		t.Errorf("a third side connection of one client was answered %q", refusal)
	}
	admit(t, own, "anna", "workspace")
}

// A refusal at the workspace's or the process's cap raises the health report's warning; a client
// over its own cap does not - that is the client's loop, not the installation's capacity.
func TestACapacityRefusalWarns(t *testing.T) {
	r := registry(Limits{PerCredential: 1, PerTenant: 2, PerProcess: 100})
	admit(t, r, "anna", "workspace")
	if _, refusal := r.Admit("anna", "workspace"); refusal != RefusedCredential {
		t.Fatalf("answered %q", refusal)
	}
	if warnings := r.Warnings(context.Background()); len(warnings) != 0 {
		t.Errorf("a credential refusal warned: %+v", warnings)
	}
	admit(t, r, "bert", "workspace")
	if _, refusal := r.Admit("carla", "workspace"); refusal != RefusedTenant {
		t.Fatalf("answered %q", refusal)
	}
	warnings := r.Warnings(context.Background())
	if len(warnings) != 1 || warnings[0].Code != "stream.capacity_reached" || warnings[0].Params["per_tenant"] != "2" {
		t.Errorf("the warning %+v", warnings)
	}
}

// A pod's streams end in waves over the window rather than all at once, so its clients do not
// reconnect to the remaining pods in the same second (deployment.md §5): every stream is asked
// exactly once, no wave holds more than a tenth plus one, the waves come in order and the last
// lands inside the window.
func TestTheDrainClosesInWavesOverTheWindow(t *testing.T) {
	r := registry(Limits{PerCredential: 100, PerTenant: 100, PerProcess: 100})
	var slots []Slot
	for i := range 95 {
		slots = append(slots, admit(t, r, "client-"+strconv.Itoa(i), "workspace"))
	}
	closed := func() int {
		n := 0
		for _, s := range slots {
			select {
			case <-s.Closing:
				n++
			default:
			}
		}
		return n
	}

	const window = 10 * time.Second
	var now time.Duration
	var waves []int
	before := 0
	// The jitter takes the largest value it may, the worst case for the window's end.
	r.closeInWaves(window, func(step time.Duration) time.Duration { return step - 1 }, func(d time.Duration) {
		if d <= 0 {
			t.Fatalf("slept %v", d)
		}
		if c := closed(); c > before {
			waves = append(waves, c-before)
			before = c
		}
		now += d
	})
	waves = append(waves, closed()-before)

	if closed() != len(slots) {
		t.Fatalf("%d of %d streams asked to close", closed(), len(slots))
	}
	if len(waves) != drainWaves {
		t.Errorf("%d waves %v, want %d", len(waves), waves, drainWaves)
	}
	for _, w := range waves {
		if w > len(slots)/drainWaves+1 {
			t.Errorf("a wave closed %d streams at once, waves %v", w, waves)
		}
	}
	if now >= window {
		t.Errorf("the last wave fell at %v, past the %v window", now, window)
	}
	if now < window-window/drainWaves {
		t.Errorf("the last wave fell at %v, the waves are not spread over the %v window", now, window)
	}
	if _, refusal := r.Admit("late", "workspace"); refusal != RefusedDraining {
		t.Errorf("a stream opened during the drain, refused with %q", refusal)
	}
}

// Without a window (nothing routes the traffic) or with one stream there is nothing to spread.
func TestTheDrainWithoutAWindowIsOneWave(t *testing.T) {
	held := []*openStream{{}, {}, {}}
	never := func(time.Duration) time.Duration { t.Fatal("jitter drawn"); return 0 }
	if got := drainSchedule(held, 0, never); len(got) != 1 || len(got[0].streams) != 3 || got[0].at != 0 {
		t.Errorf("no window: %+v", got)
	}
	if got := drainSchedule(held[:1], time.Minute, never); len(got) != 1 || got[0].at != 0 {
		t.Errorf("one stream: %+v", got)
	}
	if got := drainSchedule(nil, time.Minute, never); got != nil {
		t.Errorf("nothing open: %+v", got)
	}
}
