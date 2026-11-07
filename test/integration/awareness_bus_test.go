// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// SY-E across two processes - two listeners on one database: one process's awareness reaches the
// other's relay and not its own, and a document's log wakes that document's connections alone.
func TestAwarenessAndADocumentsWakeCrossProcesses(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	pool := appPool(ctx, t)
	listening, stop := context.WithCancel(ctx)
	defer stop()

	type heard struct {
		tenant, document shared.ID
		message          []byte
	}
	var mu sync.Mutex
	got := map[string][]heard{}
	start := func(name string) *postgres.ChangeListener {
		listener := postgres.NewChangeListener(pool)
		listener.OnAwareness(func(tenantID shared.ID, _ string, documentID shared.ID, message []byte) {
			mu.Lock()
			defer mu.Unlock()
			got[name] = append(got[name], heard{tenantID, documentID, message})
		})
		concurrency.Go(listening, "test.change_listener", listener.Run)
		waitFor(t, 5*time.Second, "the listener to connect", listener.Connected)
		return listener
	}
	first, second := start("first"), start("second")

	document, other := freshID(t), freshID(t)
	ownWake, stopOwn := second.SubscribeDocument(tenantA, "page", document)
	defer stopOwn()
	otherWake, stopOther := second.SubscribeDocument(tenantA, "page", other)
	defer stopOther()

	// Each process announces itself; the first relays only once it has heard the second.
	work := postgres.NewUnitOfWork(pool)
	bus := &postgres.AwarenessBus{Work: work, Listener: first}
	concurrency.Go(listening, "test.awareness_first", bus.Run)
	concurrency.Go(listening, "test.awareness_second", (&postgres.AwarenessBus{Work: work, Listener: second}).Run)
	waitFor(t, 5*time.Second, "the processes to hear each other", func() bool { return first.HasPeers(time.Minute) })
	bus.Publish(tenantA, "page", document, 1, []byte("cursor"))
	if err := bus.Flush(ctx); err != nil {
		t.Fatalf("publishing: %v", err)
	}
	waitFor(t, 5*time.Second, "the other process to hear the awareness", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got["second"]) == 1
	})
	mu.Lock()
	if h := got["second"][0]; h.tenant != tenantA || h.document != document || !bytes.Equal(h.message, []byte("cursor")) {
		t.Errorf("heard %+v", h)
	}
	if len(got["first"]) != 0 {
		t.Errorf("the sending process heard its own awareness: %v", got["first"])
	}
	mu.Unlock()

	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO realtime_document (tenant_id, kind, document_id, head_seq, created_at, updated_at)
		VALUES ($1, 'page', $2, 1, now(), now())`, tenantA.String(), document.String()); err != nil {
		t.Fatalf("opening the document: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO realtime_update (tenant_id, kind, document_id, seq, data, created_at)
		VALUES ($1, 'page', $2, 1, '\x01', now())`, tenantA.String(), document.String()); err != nil {
		t.Fatalf("appending to the log: %v", err)
	}
	select {
	case <-ownWake:
	case <-time.After(5 * time.Second):
		t.Fatal("the document's connections were not woken by its log")
	}
	select {
	case <-otherWake:
		t.Error("another document's connections were woken")
	case <-time.After(300 * time.Millisecond):
	}
}
