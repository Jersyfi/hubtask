// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	mailservice "github.com/Jersyfi/hubtask/core/application/service/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	boxOne = mailservice.WatchTarget{TenantID: "t1", MailboxID: "m1"}
	boxTwo = mailservice.WatchTarget{TenantID: "t2", MailboxID: "m2"}
)

// holds answers each target's scripted outcomes in turn, then holds quietly until let go.
type holds struct {
	mu       sync.Mutex
	targets  []mailservice.WatchTarget
	scripted map[mailservice.WatchTarget][]string
	active   map[mailservice.WatchTarget]int
	most     map[mailservice.WatchTarget]int
	calls    map[mailservice.WatchTarget]int
	// wanted is how many reads live; zero is as many as the targets.
	wanted int64
}

func newHolds(targets ...mailservice.WatchTarget) *holds {
	return &holds{
		targets: targets, scripted: map[mailservice.WatchTarget][]string{},
		active: map[mailservice.WatchTarget]int{}, most: map[mailservice.WatchTarget]int{},
		calls: map[mailservice.WatchTarget]int{},
	}
}

func (h *holds) Targets(context.Context, int) ([]mailservice.WatchTarget, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]mailservice.WatchTarget(nil), h.targets...), nil
}

func (h *holds) Wanted(context.Context) (int64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return max(h.wanted, int64(len(h.targets))), nil
}

func (h *holds) Hold(ctx context.Context, target mailservice.WatchTarget, _ time.Duration) (string, error) {
	h.mu.Lock()
	h.calls[target]++
	h.active[target]++
	h.most[target] = max(h.most[target], h.active[target])
	var outcome string
	if script := h.scripted[target]; len(script) > 0 {
		outcome, h.scripted[target] = script[0], script[1:]
	}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.active[target]--
		h.mu.Unlock()
	}()
	if outcome == mailservice.WatchFailed {
		return outcome, shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable")
	}
	if outcome != "" {
		return outcome, nil
	}
	<-ctx.Done()
	return mailservice.WatchQuiet, nil
}

func (h *holds) count(target mailservice.WatchTarget) (calls, active, most int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls[target], h.active[target], h.most[target]
}

type watchSignals struct {
	mu     sync.Mutex
	held   []int64
	unheld []int64
	ended  map[string]int
}

func (s *watchSignals) MailboxWatchesUnheld(_ context.Context, unheld int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unheld = append(s.unheld, unheld)
}

func (s *watchSignals) MailboxWatchesHeld(_ context.Context, held int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held = append(s.held, held)
}

func (s *watchSignals) MailboxWatchEnded(_ context.Context, outcome string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended == nil {
		s.ended = map[string]int{}
	}
	s.ended[outcome]++
}

func (s *watchSignals) last() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.held[len(s.held)-1]
}

// eventually waits for a condition the holds' goroutines make true.
func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func fastWatch(h *holds, l *leadership, signals *watchSignals) MailboxWatch {
	return MailboxWatch{
		Holds: h, Leadership: l, Signals: signals,
		Rest: time.Millisecond, Backoff: time.Millisecond, Ceiling: 4 * time.Millisecond, Retry: time.Hour,
	}.withDefaults()
}

func TestOneInstanceHoldsEachMailboxAndAStandbyTakesOver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	shared := &lock{}
	h := newHolds(boxOne, boxTwo)
	signalsA, signalsB := &watchSignals{}, &watchSignals{}
	a := fastWatch(h, &leadership{lock: shared, name: "a"}, signalsA)
	b := fastWatch(h, &leadership{lock: shared, name: "b"}, signalsB)
	heldByA := map[mailservice.WatchTarget]*mailboxHolder{}
	heldByB := map[mailservice.WatchTarget]*mailboxHolder{}

	a.tick(ctx, heldByA)
	b.tick(ctx, heldByB)
	eventually(t, "both mailboxes held", func() bool {
		_, one, _ := h.count(boxOne)
		_, two, _ := h.count(boxTwo)
		return one == 1 && two == 1
	})
	if len(heldByB) != 0 || signalsB.last() != 0 || signalsA.last() != 2 {
		t.Errorf("the standby holds %d, signals a=%d b=%d", len(heldByB), signalsA.last(), signalsB.last())
	}
	a.tick(ctx, heldByA)
	if calls, _, _ := h.count(boxOne); calls != 1 {
		t.Errorf("a second tick held a held mailbox again: %d holds", calls)
	}

	for _, target := range []mailservice.WatchTarget{boxOne, boxTwo} {
		if _, _, most := h.count(target); most != 1 {
			t.Errorf("%v was held %d times at once", target, most)
		}
	}

	// The lock goes with A's connection: B takes it, and A lets go on its next tick.
	shared.cut()
	b.tick(ctx, heldByB)
	a.tick(ctx, heldByA)
	if len(heldByA) != 0 {
		t.Errorf("the former holder kept %d mailboxes", len(heldByA))
	}
	eventually(t, "the standby holds both", func() bool {
		one, _, _ := h.count(boxOne)
		two, _, _ := h.count(boxTwo)
		return one == 2 && two == 2
	})
	eventually(t, "the former holder let go", func() bool {
		_, one, _ := h.count(boxOne)
		_, two, _ := h.count(boxTwo)
		return one == 1 && two == 1
	})
}

func TestAHoldComesBackAfterAnArrivalAndBacksOffAfterAFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := newHolds(boxOne, boxTwo)
	h.scripted[boxOne] = []string{mailservice.WatchArrived, mailservice.WatchFailed, mailservice.WatchFailed, mailservice.WatchQuiet}
	h.scripted[boxTwo] = []string{mailservice.WatchUnsupported}
	signals := &watchSignals{}
	watch := fastWatch(h, &leadership{lock: &lock{}, name: "a"}, signals)
	held := map[mailservice.WatchTarget]*mailboxHolder{}

	watch.tick(ctx, held)
	eventually(t, "the first mailbox held again after its arrival and failures", func() bool {
		calls, active, _ := h.count(boxOne)
		return calls == 5 && active == 1
	})
	eventually(t, "the second mailbox let go", func() bool {
		select {
		case <-held[boxTwo].done:
			return true
		default:
			return false
		}
	})
	watch.tick(ctx, held)
	if calls, _, _ := h.count(boxTwo); calls != 1 {
		t.Errorf("a server without IDLE was asked again within the hour: %d", calls)
	}
	if signals.last() != 1 {
		t.Errorf("held gauge %d, want 1", signals.last())
	}
	signals.mu.Lock()
	ended := signals.ended
	signals.mu.Unlock()
	if ended[mailservice.WatchArrived] != 1 || ended[mailservice.WatchFailed] != 2 || ended[mailservice.WatchUnsupported] != 1 {
		t.Errorf("ended %v", ended)
	}
	if watch.backoff(1) != time.Millisecond || watch.backoff(2) != 2*time.Millisecond || watch.backoff(10) != 4*time.Millisecond {
		t.Errorf("backoff %s %s %s", watch.backoff(1), watch.backoff(2), watch.backoff(10))
	}

	// A mailbox whose read ended (removed) is let go on the next tick.
	h.mu.Lock()
	h.targets = []mailservice.WatchTarget{boxTwo}
	h.mu.Unlock()
	watch.tick(ctx, held)
	if _, active, _ := h.count(boxOne); active != 0 || held[boxOne] != nil {
		t.Errorf("a removed mailbox is still held (%d)", active)
	}
}

// The mailboxes the limit leaves to the minute's read are counted, so an operator hears that some
// workspaces lost the arrival in seconds (A-38).
func TestTheMailboxesBeyondTheLimitAreCounted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := newHolds(boxOne, boxTwo)
	h.wanted = 5
	signals := &watchSignals{}
	watch := fastWatch(h, &leadership{lock: &lock{}, name: "a"}, signals)
	holders := map[mailservice.WatchTarget]*mailboxHolder{}
	watch.tick(ctx, holders)
	defer watch.stopAll(holders)
	signals.mu.Lock()
	defer signals.mu.Unlock()
	if len(signals.unheld) != 1 || signals.unheld[0] != 3 {
		t.Fatalf("unheld %v, want 3", signals.unheld)
	}
}

// stuckHolds is a hold that does not hear its cancellation until let go.
type stuckHolds struct {
	*holds
	entered chan struct{}
	release chan struct{}
}

func (s stuckHolds) Hold(context.Context, mailservice.WatchTarget, time.Duration) (string, error) {
	s.entered <- struct{}{}
	<-s.release
	return mailservice.WatchQuiet, nil
}

// A hold that ignores its cancellation costs the loop a bounded wait, not the hold's 25 minutes:
// a lost lock, a removed mailbox and a shutdown go on.
func TestAStuckHoldDoesNotHoldUpTheWatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stuck := stuckHolds{holds: newHolds(boxOne), entered: make(chan struct{}, 4), release: make(chan struct{})}
	defer close(stuck.release)
	watch := MailboxWatch{
		Holds: stuck, Leadership: &leadership{lock: &lock{}, name: "a"}, Release: 50 * time.Millisecond,
	}.withDefaults()
	holders := map[mailservice.WatchTarget]*mailboxHolder{}
	watch.tick(ctx, holders)
	if len(holders) != 1 {
		t.Fatalf("%d mailboxes held, want one", len(holders))
	}
	<-stuck.entered

	started := time.Now()
	watch.stopAll(holders)
	if took := time.Since(started); took > time.Second {
		t.Errorf("letting go of a stuck hold took %s", took)
	}
	if len(holders) != 0 {
		t.Errorf("%d holds left after letting go", len(holders))
	}

	watch.tick(ctx, holders)
	<-stuck.entered
	stuck.mu.Lock()
	stuck.targets = nil
	stuck.mu.Unlock()
	started = time.Now()
	watch.tick(ctx, holders)
	if took := time.Since(started); took > time.Second {
		t.Errorf("a removed mailbox's stuck hold held up the tick for %s", took)
	}
}
