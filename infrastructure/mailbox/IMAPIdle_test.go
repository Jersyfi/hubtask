// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mailbox

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-JUM-11 check 10.
func TestAnArrivalEndsTheIdleAtOnce(t *testing.T) {
	server := newServer("pw")
	server.idle, server.announce = true, true
	started := time.Now()
	arrived, err := server.reader().Watch(context.Background(), connectionFor("pw", ""), time.Minute)
	if err != nil || !arrived {
		t.Fatalf("watch answered %v, %v", arrived, err)
	}
	if time.Since(started) > 5*time.Second {
		t.Errorf("an announced message waited %s for the hold", time.Since(started))
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	for _, want := range []string{"CAPABILITY", "IDLE", "DONE", "LOGOUT"} {
		if !slices.Contains(server.commands, want) {
			t.Errorf("the watch sent no %s: %v", want, server.commands)
		}
	}
	for _, command := range server.commands {
		if len(command) > 3 && command[:3] == "UID" {
			t.Errorf("the watch read the mailbox itself: %s", command)
		}
	}
}

func TestAQuietMailboxIsHeldForTheHoldAndLetGo(t *testing.T) {
	server := newServer("pw")
	server.idle = true
	arrived, err := server.reader().Watch(context.Background(), connectionFor("pw", ""), 100*time.Millisecond)
	if err != nil || arrived {
		t.Fatalf("a quiet hold answered %v, %v", arrived, err)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if !slices.Contains(server.commands, "DONE") {
		t.Errorf("the hold was not ended with DONE: %v", server.commands)
	}
}

// UC-JUM-11 check 10.
func TestAServerWithoutIdleIsSaidSo(t *testing.T) {
	_, err := newServer("pw").reader().Watch(context.Background(), connectionFor("pw", ""), time.Minute)
	if shared.AsError(err).DetailCode != "mail.mailbox_idle_unsupported" {
		t.Errorf("a server without IDLE answered %v", err)
	}
	_, err = newServer("right").reader().Watch(context.Background(), connectionFor("wrong", ""), time.Minute)
	if shared.AsError(err).DetailCode != "mail.mailbox_refused" {
		t.Errorf("a wrong password answered %v", err)
	}
}

func TestAnEndedContextEndsTheHold(t *testing.T) {
	server := newServer("pw")
	server.idle = true
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := server.reader().Watch(ctx, connectionFor("pw", ""), 10*time.Minute)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("an ended context answered %v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Errorf("the hold outlived its context by %s", time.Since(started))
	}
}

func TestOnlyAnExistsLineAnnouncesMail(t *testing.T) {
	for line, want := range map[string]bool{
		"* 4 EXISTS": true, "* 12 exists": true, "* 3 EXPUNGE": false, "* OK still here": false, "* 2 RECENT": false,
	} {
		if announcesArrival(line) != want {
			t.Errorf("%q announces %v", line, !want)
		}
	}
}

// A context that ends after the IDLE is accepted but before the hold's deadline is set still ends
// the hold at once: the hold's deadline never replaces the cancellation's.
func TestAContextEndedAsTheIdleStartsEndsTheHold(t *testing.T) {
	server := newServer("pw")
	server.idle = true
	ctx, cancel := context.WithCancel(context.Background())
	idleAccepted = func() {
		cancel()
		time.Sleep(20 * time.Millisecond) // the cancellation's deadline is set by now
	}
	defer func() { idleAccepted = func() {} }()
	started := time.Now()
	_, err := server.reader().Watch(ctx, connectionFor("pw", ""), 3*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("an ended context answered %v", err)
	}
	if took := time.Since(started); took > time.Second {
		t.Errorf("the hold outlived its context by %s", took)
	}
}
