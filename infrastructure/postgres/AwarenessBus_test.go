// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

type countedOutcomes []string

func (c *countedOutcomes) AwarenessRelayed(_ context.Context, outcome string) {
	*c = append(*c, outcome)
}

// countingWork counts the transactions it is asked for and answers err without running them.
type countingWork struct {
	persistence.UnitOfWork
	err          error
	transactions *int
}

func (w countingWork) Within(context.Context, persistence.Scope, func(context.Context) error) error {
	*w.transactions++
	return w.err
}

var (
	busTenant   = shared.ID("0192a7c0-0000-7000-8000-000000000001")
	busDocument = shared.ID("0192a7c0-0000-7000-8000-000000000002")
)

func listenerWithPeer(peer bool) *ChangeListener {
	l := &ChangeListener{origin: "a1", peers: map[string]time.Time{}}
	if peer {
		l.peers["b2"] = time.Now()
	}
	return l
}

// A single process has nobody to relay to: thirty people moving their cursors open no transaction.
func TestAwarenessOnASingleProcessOpensNoTransaction(t *testing.T) {
	transactions := 0
	var counted countedOutcomes
	bus := &AwarenessBus{Work: countingWork{transactions: &transactions}, Listener: listenerWithPeer(false), Signals: &counted}
	for from := int64(1); from <= 30; from++ {
		bus.Publish(busTenant, "whiteboard", busDocument, from, []byte("cursor"))
	}
	if err := bus.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if transactions != 0 {
		t.Fatalf("%d transactions without a peer, want none", transactions)
	}
	if len(counted) != 30 || counted[0] != "local" {
		t.Fatalf("counted %v, want 30 x local", counted)
	}
}

// With a peer, a tick's awareness of every connection goes in one transaction, and a connection's
// newer cursor replaces its older one.
func TestAwarenessIsSentOncePerTickForAllConnections(t *testing.T) {
	transactions := 0
	var counted countedOutcomes
	bus := &AwarenessBus{Work: countingWork{transactions: &transactions}, Listener: listenerWithPeer(true), Signals: &counted}
	for round := 0; round < 3; round++ {
		for from := int64(1); from <= 30; from++ {
			bus.Publish(busTenant, "whiteboard", busDocument, from, []byte("cursor"))
		}
	}
	if err := bus.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if transactions != 1 {
		t.Fatalf("%d transactions for one tick, want 1", transactions)
	}
	if len(counted) != 30 {
		t.Fatalf("counted %d messages, want the newest of each of 30 connections", len(counted))
	}
	if err := bus.Flush(context.Background()); err != nil || transactions != 1 {
		t.Fatalf("an empty tick opened a transaction (%d, %v)", transactions, err)
	}
}

// Awareness is best effort, so a message that does not cross to the other replicas is never the
// connection's error - which is why each outcome is counted (offline-sync.md §13).
func TestEveryAwarenessPublicationIsCountedByOutcome(t *testing.T) {
	transactions := 0
	var counted countedOutcomes
	down := errors.New("connection refused")
	bus := &AwarenessBus{Work: countingWork{err: down, transactions: &transactions}, Listener: listenerWithPeer(true), Signals: &counted}

	bus.Publish(busTenant, "page", busDocument, 1, bytes.Repeat([]byte{7}, 6000))
	bus.Publish(busTenant, "page", busDocument, 2, []byte("cursor"))
	if err := bus.Flush(context.Background()); !errors.Is(err, down) {
		t.Fatalf("a failed flush answers %v", err)
	}
	bus.Work = countingWork{transactions: &transactions}
	bus.Publish(busTenant, "page", busDocument, 2, []byte("cursor"))
	if err := bus.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"too_large", "failed", "sent"}; strings.Join(counted, ",") != strings.Join(want, ",") {
		t.Fatalf("counted %v, want %v", counted, want)
	}
}

// Entries pack into notifications under PostgreSQL's limit, and the listener hands each one out.
func TestAwarenessPacksAndIsHeardEntryByEntry(t *testing.T) {
	entry := busTenant.String() + "/page/" + busDocument.String() + "|" + strings.Repeat("Y3Vyc29y", 100)
	entries := make([]string, 40)
	for i := range entries {
		entries[i] = entry
	}
	payloads := pack("a1", entries)
	if len(payloads) < 2 {
		t.Fatalf("40 entries of %d bytes packed into %d payloads", len(entry), len(payloads))
	}
	heard := 0
	listener := &ChangeListener{origin: "b2"}
	listener.heard = func(shared.ID, string, shared.ID, []byte) { heard++ }
	for _, payload := range payloads {
		if len(payload) > maxAwarenessPayload {
			t.Fatalf("a payload of %d bytes", len(payload))
		}
		listener.hear(payload)
	}
	if heard != 40 {
		t.Fatalf("heard %d entries, want 40", heard)
	}
}

// A process learns of a peer from its announcement, and forgets one that went quiet.
func TestAwarenessPeersComeFromAnnouncements(t *testing.T) {
	listener := &ChangeListener{origin: "a1", newPeer: make(chan struct{}, 1)}
	if listener.HasPeers(time.Minute) {
		t.Fatal("a peer before any announcement")
	}
	listener.hear("a1/hello")
	if listener.HasPeers(time.Minute) {
		t.Fatal("its own announcement made a peer")
	}
	listener.hear("b2/hello")
	if !listener.HasPeers(time.Minute) {
		t.Fatal("an announcement made no peer")
	}
	select {
	case <-listener.NewPeer():
	default:
		t.Fatal("a new peer did not ring")
	}
	listener.peers["b2"] = time.Now().Add(-time.Hour)
	if listener.HasPeers(time.Minute) {
		t.Fatal("a quiet peer is still counted")
	}
}
