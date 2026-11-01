// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/base64"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// maxAwarenessPayload keeps a notification under PostgreSQL's 8000 bytes. An awareness message
// that does not fit - base64 of more than about 5.8 KB - stays in its process: a cursor and a name
// are a few hundred bytes, and a larger state is sent again by the editor soon enough.
const maxAwarenessPayload = 7900

// awarenessTimeout bounds one flush (rule 7); awareness is the latest cursor, and a late one is
// worth less than the next.
const awarenessTimeout = 2 * time.Second

const (
	// awarenessEvery is how often the process sends what its connections relayed: one transaction
	// per process per tick, whatever the number of connections (offline-sync.md §13).
	awarenessEvery = 100 * time.Millisecond
	// awarenessHelloEvery is how often a relaying process announces itself; a peer not heard
	// within awarenessPeerWindow is taken as gone.
	awarenessHelloEvery = 5 * time.Second
	awarenessPeerWindow = 3 * awarenessHelloEvery
)

// AwarenessBus relays a live document's awareness to the other processes (offline-sync.md §13,
// SY-E) on `hubtask_awareness`, through the transaction wrapper (rule 3). The listener's origin
// marks the sender, so a process does not hand its own message out twice.
//
// Publish only keeps the newest message per connection; Run sends what was kept every tick in one
// transaction, packed into as few notifications as fit, and only while another relaying process
// has announced itself. A single process therefore opens no transaction for a cursor at all.
type AwarenessBus struct {
	Work     persistence.UnitOfWork
	Listener *ChangeListener
	// Signals counts each message's outcome; nil counts nothing.
	Signals AwarenessSignals

	mu      sync.Mutex
	pending map[awarenessSlot]string
}

type awarenessSlot struct {
	document DocumentKey
	from     int64
}

// AwarenessSignals is where a message's outcome is counted (`sent`, `too_large`, `failed`, `local`).
// Awareness is best effort, so a lost one is never the connection's error; the counter is how an
// operator learns that co-editing across replicas degraded.
type AwarenessSignals interface {
	AwarenessRelayed(ctx context.Context, outcome string)
}

// Publish keeps the message for the next flush, in place of the connection's earlier one. It never
// blocks and never reaches the database.
func (b *AwarenessBus) Publish(tenantID shared.ID, kind string, documentID shared.ID, from int64, message []byte) {
	ctx := context.Background()
	if !b.Listener.HasPeers(awarenessPeerWindow) {
		b.count(ctx, "local")
		return
	}
	entry := tenantID.String() + "/" + kind + "/" + documentID.String() + "|" + base64.StdEncoding.EncodeToString(message)
	if len(b.Listener.origin)+1+len(entry) > maxAwarenessPayload {
		b.count(ctx, "too_large")
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending == nil {
		b.pending = map[awarenessSlot]string{}
	}
	b.pending[awarenessSlot{document: DocumentKey{TenantID: tenantID, Kind: kind, DocumentID: documentID}, from: from}] = entry
}

// Run flushes every tick and announces the process, at start, every few seconds and whenever a new
// peer is heard. Started through SafeGo (rule 5); it returns when the context ends.
func (b *AwarenessBus) Run(ctx context.Context) {
	tick := time.NewTicker(awarenessEvery)
	defer tick.Stop()
	hello := time.NewTicker(awarenessHelloEvery)
	defer hello.Stop()
	b.announce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := b.Flush(ctx); err != nil {
				slog.DebugContext(ctx, "awareness not relayed to the other processes", slog.String("error", err.Error()))
			}
		case <-hello.C:
			b.announce(ctx)
		case <-b.Listener.NewPeer():
			b.announce(ctx)
		}
	}
}

// Flush sends the kept messages in one transaction. A failure loses them: the next message of each
// connection replaces its cursor.
func (b *AwarenessBus) Flush(ctx context.Context) error {
	b.mu.Lock()
	entries := make([]string, 0, len(b.pending))
	for _, entry := range b.pending {
		entries = append(entries, entry)
	}
	b.pending = nil
	b.mu.Unlock()
	if len(entries) == 0 {
		return nil
	}
	payloads := pack(b.Listener.origin, entries)
	if err := b.notify(ctx, payloads); err != nil {
		for range entries {
			b.count(ctx, "failed")
		}
		return err
	}
	for range entries {
		b.count(ctx, "sent")
	}
	return nil
}

func (b *AwarenessBus) announce(ctx context.Context) {
	if err := b.notify(ctx, []string{b.Listener.origin + "/" + awarenessHello}); err != nil {
		slog.DebugContext(ctx, "awareness announcement not sent", slog.String("error", err.Error()))
	}
}

// notify sends the payloads in one transaction. The system scope reaches no tenant's row, and a
// notification touches none (rule 3).
func (b *AwarenessBus) notify(ctx context.Context, payloads []string) error {
	ctx, cancel := context.WithTimeout(ctx, awarenessTimeout)
	defer cancel()
	return b.Work.Within(ctx, persistence.Scope{System: true}, func(ctx context.Context) error {
		tx, err := FromContext(ctx)
		if err != nil {
			return err
		}
		queries := sqlc.New(tx)
		for _, payload := range payloads {
			if err := queries.NotifyAwareness(ctx, payload); err != nil {
				return err
			}
		}
		return nil
	})
}

// pack joins the entries into payloads of `origin/` and newline-separated entries, each under
// maxAwarenessPayload; Publish refused an entry that would not fit alone.
func pack(origin string, entries []string) []string {
	var payloads []string
	var current strings.Builder
	for _, entry := range entries {
		if current.Len() > 0 && current.Len()+1+len(entry) > maxAwarenessPayload {
			payloads = append(payloads, current.String())
			current.Reset()
		}
		if current.Len() == 0 {
			current.WriteString(origin + "/")
		} else {
			current.WriteString("\n")
		}
		current.WriteString(entry)
	}
	if current.Len() > 0 {
		payloads = append(payloads, current.String())
	}
	return payloads
}

func (b *AwarenessBus) count(ctx context.Context, outcome string) {
	if b.Signals != nil {
		b.Signals.AwarenessRelayed(ctx, outcome)
	}
}
