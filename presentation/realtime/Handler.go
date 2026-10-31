// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package realtime

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	realtimeservice "github.com/Jersyfi/hubtask/core/application/service/realtime"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// Subprotocol is what the server answers a connection with; BearerPrefix marks the subprotocol a
// browser carries its bearer in, because it cannot set a header on a WebSocket.
const (
	Subprotocol  = "hubtask.v1"
	BearerPrefix = "hubtask.bearer."
)

// The bounds of a connection (rule 7). A write that takes longer than writeTimeout is a reader
// that stopped reading; a connection silent for longer than idleTimeout is gone, and the pings
// every pingInterval are what keep a live one from being mistaken for it. pollInterval is the
// catch-up when no wake-up is wired.
const (
	writeTimeout = 10 * time.Second
	idleTimeout  = 90 * time.Second
	pingInterval = 30 * time.Second
	pollInterval = 2 * time.Second
	// With a document's own wake-up, a workspace's change only asks the reader's access again, at
	// most every accessRecheck; refreshInterval catches up regardless, should a wake-up be lost.
	// awarenessEvery bounds how often one connection's awareness goes to the other processes, the
	// latest kept and sent at the end of the interval.
	accessRecheck   = 5 * time.Second
	refreshInterval = 30 * time.Second
	awarenessEvery  = 100 * time.Millisecond
)

// The close codes a client is told why with: in the range RFC 6455 leaves to applications.
const (
	closeForbidden websocket.StatusCode = 4403
	closeNotFound  websocket.StatusCode = 4404
)

// Documents is the whole of what the adapter may do with a document: the realtime service.
type Documents interface {
	Open(ctx context.Context, actor appshared.ActorContext, ref domain.Ref) (realtimeservice.Opened, error)
	Since(ctx context.Context, actor appshared.ActorContext, ref domain.Ref, after int64) ([]domain.Update, error)
	Append(ctx context.Context, actor appshared.ActorContext, ref domain.Ref, data []byte) (realtimeservice.Appended, error)
	Compact(ctx context.Context, actor appshared.ActorContext, ref domain.Ref, upTo int64, state []byte) error
	Relayable(data []byte) error
}

// Wakeups rings when the tenant's change log or a document's log moved (migration 0230).
type Wakeups interface {
	Subscribe(tenantID shared.ID) (<-chan struct{}, func())
}

// DocumentWakeups rings when one document's log moved (migration 0296).
type DocumentWakeups interface {
	SubscribeDocument(tenantID shared.ID, kind string, documentID shared.ID) (<-chan struct{}, func())
}

// Handler serves one document's connection.
type Handler struct {
	Documents Documents
	Wakeups   Wakeups
	// DocumentWakeups, where wired, wakes a connection for its own document alone; a workspace's
	// change then re-asks access at most every accessRecheck instead of catching up each time.
	DocumentWakeups DocumentWakeups
	Relay           *Relay
}

// Serve opens the document and holds the connection until the client leaves, the process asks it
// to end (closing), access ends or a write stalls. An error is answered before the upgrade, as a
// problem document by the caller; after the upgrade every ending is a close code.
func (h Handler) Serve(
	w http.ResponseWriter, r *http.Request, actor appshared.ActorContext, ref domain.Ref, closing <-chan struct{},
) error {
	opened, err := h.Documents.Open(r.Context(), actor, ref)
	if err != nil {
		return err
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{Subprotocol}})
	if err != nil {
		// Accept has answered the client already.
		return nil
	}
	conn.SetReadLimit(domain.MaxSnapshotBytes + 64)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	c := &connection{h: h, conn: conn, actor: actor, ref: ref, writable: opened.Writable, own: map[int64]bool{}}
	status, reason := c.run(ctx, opened, closing)
	_ = conn.Close(status, reason)
	return nil
}

type connection struct {
	h        Handler
	conn     *websocket.Conn
	actor    appshared.ActorContext
	ref      domain.Ref
	writable bool
	// last is the newest update this connection has handed out or sent; own are the places of
	// its own updates, which the catch-up does not echo.
	last int64
	own  map[int64]bool
	// checked is when access was last asked (every catch-up asks it); recheck fires a deferred one.
	checked time.Time
	recheck <-chan time.Time
	// pending is the newest awareness not yet sent to the other processes, flush when it goes.
	pending []byte
	sentAt  time.Time
	flush   <-chan time.Time
}

func (c *connection) run(ctx context.Context, opened realtimeservice.Opened, closing <-chan struct{}) (websocket.StatusCode, string) {
	woken, unsubscribe := c.subscribe()
	defer unsubscribe()
	relayID, awareness, leave := c.h.Relay.Join(c.actor.TenantID, c.ref)
	defer leave()

	if err := c.start(ctx, opened); err != nil {
		return c.ending(err)
	}
	// Opening asked the reader's access.
	c.checked = time.Now()

	incoming := make(chan []byte)
	failed := make(chan error, 1)
	concurrency.Go(ctx, "realtime.read", func(ctx context.Context) { c.read(ctx, incoming, failed) })

	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	var poll <-chan time.Time
	if woken == nil {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		poll = ticker.C
	}
	own, unsubscribeOwn := c.subscribeDocument()
	defer unsubscribeOwn()
	var refresh <-chan time.Time
	if own != nil {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		refresh = ticker.C
	}
	for {
		select {
		case <-ctx.Done():
			return websocket.StatusGoingAway, ""
		case <-closing:
			return websocket.StatusGoingAway, "draining"
		case err := <-failed:
			return c.ending(err)
		case raw := <-incoming:
			if err := c.receive(ctx, raw, relayID); err != nil {
				return c.ending(err)
			}
		case <-woken:
			if own != nil {
				// Another change of the workspace: it may have taken the reader's access.
				if wait := accessRecheck - time.Since(c.checked); wait > 0 {
					if c.recheck == nil {
						c.recheck = time.After(wait)
					}
					continue
				}
			}
			if err := c.catchUp(ctx); err != nil {
				return c.ending(err)
			}
		case <-own:
			if err := c.catchUp(ctx); err != nil {
				return c.ending(err)
			}
		case <-c.recheck:
			c.recheck = nil
			if err := c.catchUp(ctx); err != nil {
				return c.ending(err)
			}
		case <-refresh:
			if err := c.catchUp(ctx); err != nil {
				return c.ending(err)
			}
		case <-c.flush:
			c.flush = nil
			c.broadcast(ctx)
		case <-poll:
			if err := c.catchUp(ctx); err != nil {
				return c.ending(err)
			}
		case message := <-awareness:
			if err := c.write(ctx, message); err != nil {
				return c.ending(err)
			}
		case <-ping.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return c.ending(err)
			}
		}
	}
}

// start hands out the snapshot and the log after it, then asks a writer for what it holds.
func (c *connection) start(ctx context.Context, opened realtimeservice.Opened) error {
	if opened.Snapshot != nil {
		if err := c.write(ctx, SyncStep2(opened.Snapshot.State)); err != nil {
			return err
		}
		c.last = opened.Snapshot.UpToSeq
	}
	if err := c.send(ctx, opened.Updates); err != nil {
		return err
	}
	if opened.More {
		if err := c.catchUp(ctx); err != nil {
			return err
		}
	}
	if !c.writable {
		return nil
	}
	if err := c.write(ctx, SyncStep1()); err != nil {
		return err
	}
	if opened.Head.CompactionDue() {
		return c.write(ctx, CompactionDue(opened.Head.Seq))
	}
	return nil
}

func (c *connection) read(ctx context.Context, incoming chan<- []byte, failed chan<- error) {
	for {
		readCtx, cancel := context.WithTimeout(ctx, idleTimeout)
		kind, data, err := c.conn.Read(readCtx)
		cancel()
		if err != nil {
			failed <- err
			return
		}
		if kind != websocket.MessageBinary {
			failed <- errMalformed
			return
		}
		select {
		case incoming <- data:
		case <-ctx.Done():
			return
		}
	}
}

// receive handles one message from the client.
func (c *connection) receive(ctx context.Context, raw []byte, relayID int64) error {
	message, err := Decode(raw)
	if err != nil {
		return err
	}
	switch {
	case message.Type == messageSync && message.Sync == syncStep1:
		// The client's state vector. The server keeps none to compare it with, and has handed out
		// the whole log already.
		return nil
	case message.Type == messageSync && (message.Sync == syncStep2 || message.Sync == syncUpdate):
		if !c.writable {
			return shared.ErrForbidden.WithDetail("realtime.read_only")
		}
		appended, err := c.h.Documents.Append(ctx, c.actor, c.ref, message.Payload)
		if err != nil {
			return err
		}
		c.own[appended.Seq] = true
		if appended.CompactionDue {
			return c.write(ctx, CompactionDue(appended.Seq))
		}
		return nil
	case message.Type == messageAwareness:
		if err := c.h.Documents.Relayable(message.Payload); err != nil {
			return err
		}
		relayed := Awareness(message.Payload)
		c.h.Relay.Publish(c.actor.TenantID, c.ref, relayID, relayed)
		if c.h.Relay.Bus != nil {
			c.pending = relayed
			if wait := awarenessEvery - time.Since(c.sentAt); wait > 0 {
				if c.flush == nil {
					c.flush = time.After(wait)
				}
				return nil
			}
			c.broadcast(ctx)
		}
		return nil
	case message.Type == messageCompact:
		err := c.h.Documents.Compact(ctx, c.actor, c.ref, int64(message.Seq), message.Payload) //nolint:gosec // bounded by the head
		if errors.Is(err, shared.ErrVersionConflict) {
			// Another writer's snapshot came first; this one is not needed.
			return nil
		}
		return err
	}
	return nil
}

// catchUp hands out what was appended since the last update this connection knows of, its own
// updates excepted. The service asks for read access again each time.
func (c *connection) catchUp(ctx context.Context) error {
	c.checked = time.Now()
	for {
		updates, err := c.h.Documents.Since(ctx, c.actor, c.ref, c.last)
		if err != nil {
			return err
		}
		if len(updates) == 0 {
			return nil
		}
		if err := c.send(ctx, updates); err != nil {
			return err
		}
		if len(updates) < domain.MaxOpenUpdates {
			return nil
		}
	}
}

func (c *connection) send(ctx context.Context, updates []domain.Update) error {
	for _, update := range updates {
		c.last = update.Seq
		if c.own[update.Seq] {
			delete(c.own, update.Seq)
			continue
		}
		if err := c.write(ctx, SyncUpdate(update.Data)); err != nil {
			return err
		}
	}
	return nil
}

func (c *connection) write(ctx context.Context, message []byte) error {
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return c.conn.Write(writeCtx, websocket.MessageBinary, message)
}

// broadcast sends the newest awareness to the other processes. A failure is not the connection's:
// awareness is best effort, and the next message replaces this one.
func (c *connection) broadcast(ctx context.Context) {
	if c.pending == nil {
		return
	}
	message := c.pending
	c.pending, c.sentAt = nil, time.Now()
	if err := c.h.Relay.Bus.Publish(ctx, c.actor.TenantID, string(c.ref.Kind), c.ref.ID, message); err != nil {
		slog.DebugContext(ctx, "awareness not relayed to the other processes", slog.String("error", err.Error()))
	}
}

func (c *connection) subscribeDocument() (<-chan struct{}, func()) {
	if c.h.DocumentWakeups == nil {
		return nil, func() {}
	}
	return c.h.DocumentWakeups.SubscribeDocument(c.actor.TenantID, string(c.ref.Kind), c.ref.ID)
}

func (c *connection) subscribe() (<-chan struct{}, func()) {
	if c.h.Wakeups == nil {
		return nil, func() {}
	}
	return c.h.Wakeups.Subscribe(c.actor.TenantID)
}

// ending turns why a connection ends into its close code: the client is told a refusal by its
// code, a malformed or oversized message as a policy violation, and anything else as an error on
// this side. The reason is a message code, never a sentence (rule 8).
func (c *connection) ending(err error) (websocket.StatusCode, string) {
	var coded *shared.Error
	switch {
	case websocket.CloseStatus(err) != -1:
		return websocket.StatusNormalClosure, ""
	case errors.Is(err, context.Canceled):
		return websocket.StatusGoingAway, ""
	case errors.Is(err, context.DeadlineExceeded):
		return websocket.StatusPolicyViolation, "realtime.idle"
	case errors.Is(err, shared.ErrForbidden):
		return closeForbidden, detailOf(err)
	case errors.Is(err, shared.ErrNotFound):
		return closeNotFound, detailOf(err)
	case errors.Is(err, errMalformed), errors.Is(err, shared.ErrValidation):
		return websocket.StatusPolicyViolation, detailOf(err)
	case errors.As(err, &coded):
		return websocket.StatusInternalError, coded.DetailCode
	}
	return websocket.StatusInternalError, ""
}

func detailOf(err error) string {
	var coded *shared.Error
	if errors.As(err, &coded) {
		return coded.DetailCode
	}
	return "realtime.message_malformed"
}
