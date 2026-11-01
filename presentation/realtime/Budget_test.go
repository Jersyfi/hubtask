// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package realtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A writer streaming updates past the byte budget is closed with closeTooFast, and the update
// past it is not stored: the budget bounds what one connection and one workspace write.
func TestAnUpdatePastTheByteBudgetIsRefusedAndNotStored(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		handler func(*documents) Handler
	}{
		{"the connection's", func(docs *documents) Handler {
			return Handler{Documents: docs, Relay: NewRelay(), ConnectionBytes: BudgetRate{PerMinute: 10, Burst: 10}}
		}},
		{"the workspace's", func(docs *documents) Handler {
			return Handler{Documents: docs, Relay: NewRelay(), Workspaces: NewBudget(BudgetRate{PerMinute: 10, Burst: 10})}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			docs := &documents{writable: true}
			handler := testCase.handler(docs)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actor := appshared.ActorContext{TenantID: tenant, Kind: shared.ActorUser}
				_ = handler.Serve(w, r, actor, doc, nil, Credential{})
			}))
			t.Cleanup(server.Close)
			conn := dial(t, server)
			next(t, conn) // the snapshot
			next(t, conn) // step one

			send(t, conn, SyncUpdate([]byte("eight by")))
			send(t, conn, SyncUpdate([]byte("past the budget")))

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			_, _, err := conn.Read(ctx)
			if websocket.CloseStatus(err) != closeTooFast {
				t.Fatalf("closed with %v, want %d", err, closeTooFast)
			}
			docs.mu.Lock()
			defer docs.mu.Unlock()
			if len(docs.updates) != 1 || string(docs.updates[0].Data) != "eight by" {
				t.Errorf("stored %v, want only the update within the budget", docs.updates)
			}
		})
	}
}

func TestABudgetRefillsOverTime(t *testing.T) {
	budget := NewBudget(BudgetRate{PerMinute: 60, Burst: 60})
	start := time.Unix(0, 0)
	if wait := budget.Spend("w", 60, start); wait != 0 {
		t.Fatalf("the burst was refused, wait %v", wait)
	}
	if wait := budget.Spend("w", 30, start); wait != 30*time.Second {
		t.Fatalf("an empty bucket answered wait %v, want 30s", wait)
	}
	if wait := budget.Spend("w", 30, start.Add(30*time.Second)); wait != 0 {
		t.Fatalf("half a minute later the half was refused, wait %v", wait)
	}
	if wait := budget.Spend("other", 60, start); wait != 0 {
		t.Fatalf("another workspace shared the bucket, wait %v", wait)
	}
}
