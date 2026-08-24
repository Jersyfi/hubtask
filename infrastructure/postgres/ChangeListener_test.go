// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The fan-out, without a database. What needs proving here is the bookkeeping - who is woken, who
// is not, and what a cancelled subscription leaves behind; that the notification arrives at all is
// the integration suite's, because it is a property of the trigger.

var (
	workspaceA = shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	workspaceB = shared.ID("01936f2a-7c1e-7000-8000-0000000000a2")
)

func listener() *ChangeListener { return NewChangeListener(nil) }

func woken(t *testing.T, ch <-chan struct{}) bool {
	t.Helper()
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestOnlyTheWorkspaceThatChangedIsWoken(t *testing.T) {
	l := listener()

	inA, stopA := l.Subscribe(workspaceA)
	inB, stopB := l.Subscribe(workspaceB)
	defer stopA()
	defer stopB()

	l.wake(workspaceA)

	if !woken(t, inA) {
		t.Error("the workspace that changed was not woken")
	}
	if woken(t, inB) {
		t.Error("a workspace that did not change was woken")
	}
}

func TestEverybodyWatchingOneWorkspaceIsWoken(t *testing.T) {
	l := listener()

	first, stopFirst := l.Subscribe(workspaceA)
	second, stopSecond := l.Subscribe(workspaceA)
	defer stopFirst()
	defer stopSecond()

	l.wake(workspaceA)

	if !woken(t, first) || !woken(t, second) {
		t.Error("a second stream on the same workspace was not woken")
	}
}

// A doorbell, not a queue: a subscriber with an unread wake-up already knows there is something to
// read, and a second one tells it nothing new. Dropping it is also what stops one slow stream from
// blocking the loop every other stream depends on.
func TestASecondWakeUpIsDroppedRatherThanQueued(t *testing.T) {
	l := listener()

	in, stop := l.Subscribe(workspaceA)
	defer stop()

	for range 100 {
		// None of these may block. A test that hangs here is the failure.
		l.wake(workspaceA)
	}

	if !woken(t, in) {
		t.Fatal("no wake-up arrived at all")
	}
	if woken(t, in) {
		t.Error("the wake-ups queued up - a stream would then read the log once per write")
	}
}

// A waiter that outlived its connection is a leak the process cannot see: the stream is gone and
// the map still names it.
func TestACancelledSubscriptionIsForgotten(t *testing.T) {
	l := listener()

	_, stop := l.Subscribe(workspaceA)
	stop()

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, present := l.waiters[workspaceA]; present {
		t.Error("the workspace still has watchers after the last one cancelled")
	}
}

func TestCancellingTwiceIsHarmless(t *testing.T) {
	l := listener()

	_, first := l.Subscribe(workspaceA)
	other, second := l.Subscribe(workspaceA)
	defer second()

	first()
	first()

	// The one that did not cancel is untouched by its neighbour cancelling twice.
	l.wake(workspaceA)
	if !woken(t, other) {
		t.Error("cancelling one subscription silenced another")
	}
}

func TestAProcessWithNoListeningConnectionSaysSo(t *testing.T) {
	l := listener()

	if l.Connected() {
		t.Error("a listener that has never dialled reports itself connected")
	}
	l.setConnected(true)
	if !l.Connected() {
		t.Error("a connected listener reports itself disconnected")
	}
}
