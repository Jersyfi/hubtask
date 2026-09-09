// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/stream"
)

// The server-initiated half of the streamable transport (J-13).
//
// `GET /mcp` used to answer `405` with a comment saying this server initiates nothing and that
// saying so is better than holding a connection that never speaks. J-11 and J-12 ended that: there
// are lists that can change, and a client that has to poll to find out is a client holding a stale
// picture of a workspace somebody else is editing.
//
// **It is the same kind of connection as `GET /stream`, and it is held to the same limits.** The
// caps, the draining, the refusal reasons and the framing are `presentation/stream`'s, shared with
// the browser's stream: a pod's capacity is one number whoever is holding it, and an agent that
// opened a hundred sockets would be refused by the same rule and counted in the same metric.

// The stream's own timings, matching the change stream's for the same reasons: a heartbeat well
// under any proxy's idle timeout, and a reconnection delay long enough that a client does not spend
// a rollout in a tight loop.
const (
	agentHeartbeat  = 20 * time.Second
	agentRetry      = 3000
	agentRetryAfter = 5
)

// StreamSignals is the slice of the metrics adapter this stream reports through - the same one the
// change stream uses, so an operator reads one set of series for both.
type StreamSignals interface {
	StreamOpened(ctx context.Context)
	StreamClosed(ctx context.Context, seconds float64)
	StreamRefused(ctx context.Context, reason string)
}

// Wakeups tells the stream that a workspace has changed, so a notification is sent because
// something happened rather than because a timer fired (ADR-0007).
//
// The same subscription the change stream uses. What differs is what it produces: a browser is told
// *what* changed, record by record; an agent is told only *that* its resource list may have moved,
// because MCP's `listChanged` carries no payload and an agent re-reads what it needs.
type Wakeups interface {
	Subscribe(tenantID shared.ID) (<-chan struct{}, func())
}

// stream opens the server-initiated stream.
func (s Server) stream(w http.ResponseWriter, r *http.Request) {
	if s.Streams == nil {
		// No registry wired, so nothing bounds a connection here. Refusing is the honest answer:
		// an unbounded stream is exactly what the caps exist to prevent.
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	actor, held := actorOf(r)
	if !held {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	// The stream belongs to a session, not merely to a credential: a client opens it after the
	// handshake, with the identifier the handshake answered. One that presents none is asking for a
	// stream that is part of no conversation.
	if !s.sessionAccepted(r, actor) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	slot, refusal := s.Streams.Admit(credentialOf(r), actor.TenantID.String())
	if refusal != stream.RefusedNone {
		s.report(r.Context(), func(ctx context.Context, signals StreamSignals) {
			signals.StreamRefused(ctx, refusal.String())
		})
		w.Header().Set("Retry-After", strconv.Itoa(agentRetryAfter))
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	defer slot.Release()

	// Subscribed before the first frame, not after: a wake-up that arrived between opening the
	// stream and subscribing would be a change nobody is told about until the next one.
	woken, unsubscribe := s.subscribe(actor.TenantID)
	defer unsubscribe()

	s.hold(w, r, slot, woken)
}

// hold writes the stream until the client leaves, the process drains, or writing fails.
//
// Nothing here starts a goroutine, and that is deliberate rather than incidental: the handler *is*
// the connection, it ends by returning, and that is what lets the HTTP server's own drain wait for
// it. A stream that spawned a writer would need the writer told about every way a connection can
// end, and net/http already owns three of them.
func (s Server) hold(
	w http.ResponseWriter, r *http.Request, slot stream.Slot, woken <-chan struct{},
) {
	started := s.now()
	stream.Headers(w.Header())
	w.WriteHeader(http.StatusOK)

	events := stream.NewWriter(w)
	if err := events.Retry(agentRetry); err != nil {
		return
	}
	s.report(r.Context(), func(ctx context.Context, signals StreamSignals) {
		signals.StreamOpened(ctx)
	})
	defer s.report(r.Context(), func(ctx context.Context, signals StreamSignals) {
		signals.StreamClosed(ctx, s.now().Sub(started).Seconds())
	})

	heartbeat := time.NewTicker(agentHeartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-slot.Closing:
			// The process is going away. The client is told rather than having its socket cut: it
			// reconnects to another pod, and the difference between the two is a visible error in
			// somebody's console.
			_ = events.Comment("closing")
			return
		case <-woken:
			// Something in this workspace changed, so the lists this server publishes may have
			// moved. MCP's notification carries no payload by design - the client re-reads what it
			// cares about - which is also what keeps a workspace's content out of a frame that
			// nothing has authorised the reading of yet.
			if err := events.Event("", "message", listChanged()); err != nil {
				return
			}
		case <-heartbeat.C:
			// The one thing that notices a connection nobody has told us about. A client that
			// vanished without a FIN leaves a socket that reads as open until something is written.
			if err := events.Comment("heartbeat"); err != nil {
				return
			}
		}
	}
}

// listChanged is the JSON-RPC notification a wake-up produces.
//
// Resources only. The tool list is generated from the use case registry, which does not change
// while a process runs, and the prompt list is compiled in - so notifying about either would be a
// message that is never true. `initialize` says the same thing in its capabilities, and the two
// have to agree or a client waits for a notification that will not come.
func listChanged() string {
	frame, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "method": "notifications/resources/list_changed",
	})
	if err != nil {
		// A constant map that will not serialise is not a thing that happens; an empty frame is
		// still better than a panic on a connection somebody is holding.
		return "{}"
	}
	return string(frame)
}

func (s Server) subscribe(tenantID shared.ID) (<-chan struct{}, func()) {
	if s.Wakeups == nil {
		// No listener wired. The stream still works - it heartbeats and it drains - and it simply
		// never notifies, which is a client polling as it did before J-13 rather than a broken one.
		return nil, func() {}
	}
	return s.Wakeups.Subscribe(tenantID)
}

func (s Server) report(ctx context.Context, to func(context.Context, StreamSignals)) {
	if s.Signals == nil {
		return
	}
	to(ctx, s.Signals)
}

func (s Server) now() time.Time {
	if s.Clock == nil {
		return time.Now()
	}
	return s.Clock()
}
