// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"strconv"
	"time"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The stream's own timings. Constants rather than configuration, because none of them is a
// decision an operator makes: they are properties of how SSE behaves in front of a proxy.
const (
	// streamHeartbeat is how often a comment line is sent down an idle stream. Well under the
	// idle timeout of every proxy anybody puts in front of this - nginx and most cloud load
	// balancers default to a minute - because a stream a proxy closed looks to a client like a
	// server that went away.
	streamHeartbeat = 20 * time.Second
	// streamRetry is the reconnection delay the stream suggests to a browser's EventSource, in
	// milliseconds. Longer than the default of a few hundred milliseconds: a client that
	// reconnects to a pod that is shutting down should not spend the rollout in a tight loop.
	streamRetry = 3000
	// streamRefusalRetryAfter is what a refused connection is told to wait, in seconds. A cap is
	// reached because something else is holding the slots, and the shortest useful answer is one
	// that does not have every refused client come back together.
	streamRefusalRetryAfter = 5
	// streamIdlePoll is the longest a round waits before reading the log again without having
	// been woken. The safety net under LISTEN/NOTIFY rather than the mechanism: when the listener
	// is connected this timer almost never fires, and when it is not, the stream is correct and
	// merely slow (ADR-0007).
	streamIdlePoll = 30 * time.Second
)

// StreamSignals is the slice of the metrics adapter the stream reports through
// (observability-reliability.md §3.2). The labels are closed sets: a refusal reason, never an
// identifier.
type StreamSignals interface {
	StreamOpened(ctx context.Context)
	StreamClosed(ctx context.Context, seconds float64)
	StreamRefused(ctx context.Context, reason string)
	StreamRecords(ctx context.Context, count int)
}

// Wakeups tells the stream when a workspace has changed, so that a round happens because something
// happened rather than because a timer fired (ADR-0007).
type Wakeups interface {
	Subscribe(tenantID shared.ID) (<-chan struct{}, func())
}

// Changes is the slice of the stream service this controller drives: where a client resumes, what
// it may see next, and how a position is spelled on the wire.
//
// An interface rather than the service itself, for the reason UseCaseRegistry is one - this is the
// whole of what the transport is allowed to do with the stream, and a controller test needs no
// database to prove framing.
type Changes interface {
	Resume(ctx context.Context, actor appshared.ActorContext, cursor string) (
		syncservice.Position, error)
	Next(ctx context.Context, actor appshared.ActorContext, from syncservice.Position) (
		syncservice.Batch, error)
	Encode(position syncservice.Position) string
}

// StreamController serves `GET /stream` as server-sent events (C-10).
//
// The handler is the connection: it runs for as long as the stream lives, and it ends by returning
// - which is what lets the HTTP server's own drain wait for it. Nothing here starts a goroutine,
// and that is deliberate rather than incidental: a stream that spawned a writer would need the
// writer to be told about every way a connection can end, and net/http already owns three of them.
type StreamController struct {
	Stream   Changes
	Registry *StreamRegistry
	Wakeups  Wakeups
	Signals  StreamSignals
	// Clock is injectable so the tests do not have to wait. Nil means the system clock.
	Clock func() time.Time
}

// StreamChanges opens the stream.
func (c StreamController) StreamChanges(
	w http.ResponseWriter, r *http.Request, params openapi.StreamChangesParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	actor, _ := appshared.ActorFrom(r.Context())
	if !actor.IsAuthenticated() {
		WriteProblem(w, shared.ErrUnauthenticated.WithDetail("access.credential_required"), requestID)
		return
	}

	slot, refusal := c.admit(r, actor)
	if refusal != RefusedNone {
		c.report(r.Context(), func(ctx context.Context, s StreamSignals) {
			s.StreamRefused(ctx, refusal.String())
		})
		w.Header().Set("Retry-After", strconv.Itoa(streamRefusalRetryAfter))
		WriteProblem(w, shared.ErrUnavailable.
			WithDetail("sync.stream_unavailable").
			WithParams(map[string]string{"reason": refusal.String()}), requestID)
		return
	}
	defer slot.Release()

	// Resolved before a byte of the stream is written, because both of its failures are ordinary
	// problem documents and a `200` already sent cannot become a `410`.
	from, err := c.Stream.Resume(r.Context(), actor, cursorOf(params))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	// Subscribed before the first read, not after. A wake-up that arrived between reading the log
	// and subscribing would be a change nobody is told about until the next one - which is the
	// race that makes a stream look like it loses records under load.
	woken, unsubscribe := c.subscribe(actor.TenantID)
	defer unsubscribe()

	c.serve(w, r, actor, from, slot, woken)
}

// admit asks the registry for the right to hold a connection.
func (c StreamController) admit(
	r *http.Request, actor appshared.ActorContext,
) (StreamSlot, StreamRefusal) {
	// The credential is fingerprinted rather than used as the key, for the reason the rate
	// limiter's is: the map ends up in a heap dump, and a heap dump with live tokens in it is a
	// second incident on top of the first (rule 10).
	credential := ""
	if presented, err := bearerCredential(r); err == nil && presented != "" {
		credential = fingerprint(presented)
	}
	return c.Registry.Admit(credential, actor.TenantID.String())
}

func (c StreamController) subscribe(tenantID shared.ID) (<-chan struct{}, func()) {
	if c.Wakeups == nil {
		// No listener wired. The stream still works, at the idle poll interval - correct and
		// slow, which is the right way round for a degradation.
		return nil, func() {}
	}
	return c.Wakeups.Subscribe(tenantID)
}

// serve writes the stream until the client leaves, the process drains, or writing fails.
func (c StreamController) serve(
	w http.ResponseWriter, r *http.Request, actor appshared.ActorContext,
	from syncservice.Position, slot StreamSlot, woken <-chan struct{},
) {
	started := c.now()
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	// No store and no transform: an intermediary that cached this would serve one client's
	// records to another, and one that buffered it would hold every event until the connection
	// ended - which is the whole point of the connection.
	header.Set("Cache-Control", "no-store")
	header.Set("Connection", "keep-alive")
	// nginx buffers proxied responses by default and this is the header that turns it off. Sent
	// unconditionally: it means nothing to anything else.
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	stream := &sseWriter{w: w, controller: http.NewResponseController(w)}
	if err := stream.retry(streamRetry); err != nil {
		return
	}
	c.report(r.Context(), func(ctx context.Context, s StreamSignals) { s.StreamOpened(ctx) })
	defer c.report(r.Context(), func(ctx context.Context, s StreamSignals) {
		s.StreamClosed(ctx, c.now().Sub(started).Seconds())
	})

	heartbeat := time.NewTicker(streamHeartbeat)
	defer heartbeat.Stop()
	idle := time.NewTimer(streamIdlePoll)
	defer idle.Stop()

	cursor := from
	for {
		batch, err := c.Stream.Next(r.Context(), actor, cursor)
		if err != nil {
			// Mid-stream there is no status left to send. The connection ends and the client
			// reconnects with its last cursor, which is exactly the recovery the protocol has -
			// and the error is already counted where it happened.
			return
		}
		cursor = batch.Cursor

		for _, record := range batch.Records {
			if err := stream.event(c.Stream.Encode(record.Cursor), record); err != nil {
				return
			}
		}
		if len(batch.Records) > 0 {
			c.report(r.Context(), func(ctx context.Context, s StreamSignals) {
				s.StreamRecords(ctx, len(batch.Records))
			})
		}
		if batch.More {
			// Known work left. Straight back rather than waiting to be woken, because the wake-up
			// for these records has already been consumed.
			continue
		}

		if !c.wait(r.Context(), slot, woken, heartbeat, idle, stream) {
			return
		}
	}
}

// wait blocks until there is a reason to read the log again, and reports whether the stream
// continues.
//
// Every way a stream ends is one of these cases, which is why they are in one select: the client
// went, the process is draining, or a heartbeat could not be written - and that last one is how a
// connection that died without telling anybody is noticed at all.
func (c StreamController) wait(
	ctx context.Context, slot StreamSlot, woken <-chan struct{},
	heartbeat *time.Ticker, idle *time.Timer, stream *sseWriter,
) bool {
	if !idle.Stop() {
		select {
		case <-idle.C:
		default:
		}
	}
	idle.Reset(streamIdlePoll)

	for {
		select {
		case <-ctx.Done():
			return false
		case <-slot.Closing:
			// The process is going away. The client is told so rather than having the socket cut:
			// it reconnects to another pod and resumes from its cursor, and the difference between
			// the two is a visible error in somebody's console.
			_ = stream.comment("closing")
			return false
		case <-woken:
			return true
		case <-idle.C:
			return true
		case <-heartbeat.C:
			// The one thing that notices a connection nobody has told us about. A client that
			// vanished without a FIN leaves a socket that reads as open until something is written
			// to it.
			if err := stream.comment("heartbeat"); err != nil {
				return false
			}
		}
	}
}

// report hands the signals to the adapter, if one is wired. The context is passed through rather
// than captured, so that what a metric is recorded against is the call's context and not whichever
// one the closure happened to close over.
func (c StreamController) report(ctx context.Context, to func(context.Context, StreamSignals)) {
	if c.Signals == nil {
		return
	}
	to(ctx, c.Signals)
}

func (c StreamController) now() time.Time {
	if c.Clock == nil {
		return time.Now()
	}
	return c.Clock()
}

// cursorOf is where the client says it stands. The header is the only place it is read from: a
// query parameter would be a second way to say one thing, and this one is the way `EventSource`
// resumes by itself.
func cursorOf(params openapi.StreamChangesParams) string {
	if params.LastEventID == nil {
		return ""
	}
	return *params.LastEventID
}
