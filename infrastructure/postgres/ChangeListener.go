// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ChangeChannel is the `LISTEN` channel the change log's trigger announces on
// (db/migrations/0022_change_notify.sql). A constant on both sides of a name that has to agree, and
// the reason it is here rather than in a configuration file: a channel nobody is listening on is a
// stream that never wakes up, and nothing would say so.
const ChangeChannel = "hubtask_change"

// listenerReconnectDelay is how long a listener waits before dialling again after its connection
// went. Short, because the cost of being disconnected is that every stream on this process falls
// back to its poll interval - correct, but slow.
const listenerReconnectDelay = time.Second

// ChangeListener is the wake-up for the change stream: one connection per process holding a
// `LISTEN`, and an in-process fan-out from it (ADR-0007).
//
// # Why this holds a connection, and why that is not a hole in rule 3
//
// The reasoning is the Leader's, and it is worth writing out again because it is the second
// exception in this package rather than a precedent being widened. Rule 3 exists because the
// transaction wrapper is the only place that sets the tenant context, so a query reaching the
// database another way would run without one. Nothing here is such a query: the only statement is
// `LISTEN hubtask_change`, it touches no table, and no other code can reach this connection - it is
// unexported and this type offers no way to run anything on it. What arrives back is a tenant
// identifier and nothing else; every read of the log still goes through the wrapper, under row
// level security, exactly as a poll would.
//
// It has to be a held connection because `LISTEN` belongs to a session. A pooled connection handed
// back after the statement would take the subscription with it.
//
// One connection of the API role's pool is therefore occupied for as long as the process serves
// streams. That is the cost, and it is why the pool is never sized at one.
type ChangeListener struct {
	pool *pgxpool.Pool

	// mu guards waiters. Subscribe and Unsubscribe are called from request goroutines, and the
	// delivery from the listening loop, which is three by construction.
	mu      sync.Mutex
	waiters map[shared.ID]map[int64]chan struct{}
	nextID  int64
	// connected says whether the loop currently holds a listening connection. A stream that
	// starts while it is false is not broken - it falls back to its own poll interval - but it is
	// worth reporting, because a process that never reconnects is a process where every stream is
	// silently slow.
	connected bool
	// documents are the waiters of one live document, woken by its own log (migration 0296).
	documents map[DocumentKey]map[int64]chan struct{}
	// origin names this process on the awareness channel, so that it skips what it sent itself;
	// heard is where another process's awareness goes.
	origin string
	heard  func(tenantID shared.ID, kind string, documentID shared.ID, message []byte)
}

// DocumentKey names one live document of a workspace.
type DocumentKey struct {
	TenantID   shared.ID
	Kind       string
	DocumentID shared.ID
}

// The channels besides ChangeChannel: a document's own log (migration 0296) and the awareness
// relayed between processes (AwarenessBus).
const (
	DocumentChannel  = "hubtask_document"
	AwarenessChannel = "hubtask_awareness"
)

func NewChangeListener(pool *pgxpool.Pool) *ChangeListener {
	origin := make([]byte, 8)
	_, _ = rand.Read(origin)
	return &ChangeListener{
		pool: pool, waiters: map[shared.ID]map[int64]chan struct{}{},
		documents: map[DocumentKey]map[int64]chan struct{}{}, origin: hex.EncodeToString(origin),
	}
}

// SubscribeDocument asks to be woken when the document's log moves, as Subscribe for a workspace.
func (l *ChangeListener) SubscribeDocument(tenantID shared.ID, kind string, documentID shared.ID) (<-chan struct{}, func()) {
	key := DocumentKey{TenantID: tenantID, Kind: kind, DocumentID: documentID}
	woken := make(chan struct{}, 1)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextID++
	id := l.nextID
	if l.documents[key] == nil {
		l.documents[key] = map[int64]chan struct{}{}
	}
	l.documents[key][id] = woken
	var once sync.Once
	return woken, func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			delete(l.documents[key], id)
			if len(l.documents[key]) == 0 {
				delete(l.documents, key)
			}
		})
	}
}

// OnAwareness names where another process's awareness goes; set before Run.
func (l *ChangeListener) OnAwareness(heard func(tenantID shared.ID, kind string, documentID shared.ID, message []byte)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.heard = heard
}

// Subscribe asks to be woken when the workspace changes, and returns the channel and the way to
// stop.
//
// The channel has room for one, and a send that would block is dropped. That is the correct
// semantics for a doorbell: a subscriber with an unread wake-up already knows there is something to
// read, and a second one tells it nothing new. It is also what stops a slow reader from blocking
// the listening loop, which is the one goroutine every stream on this process depends on.
//
// The cancel is idempotent and must be called, or the waiter outlives the connection it belongs to.
func (l *ChangeListener) Subscribe(tenantID shared.ID) (<-chan struct{}, func()) {
	woken := make(chan struct{}, 1)

	l.mu.Lock()
	defer l.mu.Unlock()

	l.nextID++
	id := l.nextID
	if l.waiters[tenantID] == nil {
		l.waiters[tenantID] = map[int64]chan struct{}{}
	}
	l.waiters[tenantID][id] = woken

	var once sync.Once
	return woken, func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			delete(l.waiters[tenantID], id)
			if len(l.waiters[tenantID]) == 0 {
				// The map of a workspace nobody is watching goes with the last watcher: a process
				// that served a stream for every tenant it has ever seen would otherwise keep a
				// map entry per tenant forever.
				delete(l.waiters, tenantID)
			}
		})
	}
}

// Connected reports whether a listening connection is currently held. What the health report reads:
// a process without one still serves streams, and serves them at the poll interval.
func (l *ChangeListener) Connected() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.connected
}

// Run holds the listening connection for as long as the context lives, reconnecting when it drops.
//
// Started through SafeGo by the composition root like every other loop (rule 5). It returns when
// the context is cancelled and not before: a listener that gave up after a failure would leave
// every stream on the process at its poll interval, which is a degradation nothing reports.
func (l *ChangeListener) Run(ctx context.Context) {
	slog.InfoContext(ctx, "change listener ready", slog.String("channel", ChangeChannel))

	for ctx.Err() == nil {
		if err := l.listen(ctx); err != nil && ctx.Err() == nil {
			// Logged at warning rather than error: the streams keep working, more slowly. An
			// error would page somebody for a degradation the health report already carries.
			slog.WarnContext(ctx, "the change listener lost its connection",
				slog.String("error", err.Error()))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(listenerReconnectDelay):
		}
	}
}

// listen holds one connection until it fails or the context ends.
func (l *ChangeListener) listen(ctx context.Context) error {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	for _, channel := range []string{ChangeChannel, DocumentChannel, AwarenessChannel} {
		if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
			return err
		}
	}

	l.setConnected(true)
	defer l.setConnected(false)

	for {
		// No deadline of its own, deliberately, and it is not the hole in rule 7 it looks like:
		// waiting for a notification is the loop's whole purpose, and a timeout here would mean
		// dropping and re-establishing the subscription on a schedule for no reason. The bound is
		// the context, which the shutdown cancels.
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return nil
			}
			return err
		}

		switch notification.Channel {
		case DocumentChannel:
			if key, ok := documentKeyOf(notification.Payload); ok {
				l.wakeDocument(key)
			}
			continue
		case AwarenessChannel:
			l.hear(notification.Payload)
			continue
		}
		tenantID, err := shared.ParseID(notification.Payload)
		if err != nil {
			// Something else is announcing on this channel. Ignored rather than fatal: the
			// channel is a database-wide name and this process does not own it.
			continue
		}
		l.wake(tenantID)
	}
}

func (l *ChangeListener) setConnected(connected bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.connected = connected
}

// wake rings the doorbell for everybody watching one workspace.
func (l *ChangeListener) wake(tenantID shared.ID) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, woken := range l.waiters[tenantID] {
		select {
		case woken <- struct{}{}:
		default:
			// Already has an unread wake-up. Dropping the second is the point: it says nothing the
			// first does not, and blocking here would let one slow stream stop every other.
		}
	}
}

func (l *ChangeListener) wakeDocument(key DocumentKey) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, woken := range l.documents[key] {
		select {
		case woken <- struct{}{}:
		default:
		}
	}
}

// hear hands another process's awareness to this one's relay; its own, and a payload that is not
// the bus's framing, are skipped.
func (l *ChangeListener) hear(payload string) {
	origin, rest, _ := strings.Cut(payload, "/")
	if origin == l.origin {
		return
	}
	keyText, encoded, found := strings.Cut(rest, "|")
	key, ok := documentKeyOf(keyText)
	if !found || !ok {
		return
	}
	message, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(message) == 0 {
		return
	}
	l.mu.Lock()
	heard := l.heard
	l.mu.Unlock()
	if heard != nil {
		heard(key.TenantID, key.Kind, key.DocumentID, message)
	}
}

// documentKeyOf reads `tenant/kind/document`, as migration 0296 writes it.
func documentKeyOf(payload string) (DocumentKey, bool) {
	parts := strings.Split(payload, "/")
	if len(parts) != 3 || parts[1] == "" {
		return DocumentKey{}, false
	}
	tenantID, err := shared.ParseID(parts[0])
	if err != nil {
		return DocumentKey{}, false
	}
	documentID, err := shared.ParseID(parts[2])
	if err != nil {
		return DocumentKey{}, false
	}
	return DocumentKey{TenantID: tenantID, Kind: parts[1], DocumentID: documentID}, true
}
