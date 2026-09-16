// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
	"github.com/Jersyfi/hubtask/presentation/stream"
)

// The initial synchronisation as one response (SY-C, P-12): what `:pull` with no cursor answers
// in pages, written as newline-delimited JSON while it is read, the cursor last.
//
// It is a long-lived response like the stream and is bounded the way the stream is: admitted by
// the same registry - so that a snapshot counts against the same complement per credential, per
// workspace and per pod, and `hubtask_stream_connections` counts it - and ended by a deadline and
// a byte budget of its own. A device whose connection ended before the last line has no cursor
// and starts again, which is what a page sequence offers it too.

// Snapshotter is the slice of the pull service the snapshot drives: the whole walk, handed out
// record by record, and how the cursor at its end is spelled.
type Snapshotter interface {
	WalkAll(ctx context.Context, actor appshared.ActorContext, request syncservice.SnapshotRequest,
		emit func(syncservice.Record) error) (syncservice.Position, error)
	Encode(position syncservice.Position) string
}

const (
	// snapshotDeadline bounds one snapshot. Long enough for a workspace of a few hundred thousand
	// rows over a slow link, short enough that a connection nobody reads from is not a slot held
	// for the afternoon.
	snapshotDeadline = 15 * time.Minute
	// snapshotByteBudget bounds what one snapshot writes. Above it the connection ends without
	// the cursor line, and the device starts again - with a scope, or in pages.
	snapshotByteBudget = 512 << 20
	// snapshotContentType is the line-delimited JSON the contract promises.
	snapshotContentType = "application/x-ndjson"
	// snapshotFlushEvery bounds how long a record waits in the buffer before it is on the wire:
	// the first byte arrives before the last row is counted, which is the whole point of a stream.
	snapshotFlushEvery = 32 << 10
)

// SyncSnapshot answers POST /sync:snapshot.
func (c SyncController) SyncSnapshot(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	actor, _ := appshared.ActorFrom(r.Context())
	if !actor.IsAuthenticated() {
		WriteProblem(w, shared.ErrUnauthenticated.WithDetail("access.credential_required"), requestID)
		return
	}
	snapshotter, held := c.Pull.(Snapshotter)
	if !held || c.Registry == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.SyncSnapshotRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	// The same registry, the same key, the same refusal as the stream's: a client could not
	// double its allowance by asking for snapshots instead.
	slot, refusal := c.Registry.Admit(stream.Credential(r), actor.TenantID.String())
	if refusal != stream.RefusedNone {
		if c.StreamSignals != nil {
			c.StreamSignals.StreamRefused(r.Context(), refusal.String())
		}
		w.Header().Set("Retry-After", strconv.Itoa(streamRefusalRetryAfter))
		WriteProblem(w, shared.ErrUnavailable.
			WithDetail("sync.stream_unavailable").
			WithParams(map[string]string{"reason": refusal.String()}), requestID)
		return
	}
	defer slot.Release()

	ctx, cancel := context.WithTimeout(r.Context(), snapshotDeadline)
	defer cancel()
	// A process draining ends the snapshot as it ends a stream: the client starts again against
	// another pod rather than waiting on a socket nobody will write to.
	concurrency.Go(ctx, "rest.snapshot_drain", func(ctx context.Context) {
		select {
		case <-slot.Closing:
			cancel()
		case <-ctx.Done():
		}
	})

	started := time.Now()
	if c.StreamSignals != nil {
		c.StreamSignals.StreamOpened(ctx)
		defer func() { c.StreamSignals.StreamClosed(ctx, time.Since(started).Seconds()) }()
	}

	// The headers go before the first record, so the first failure past this point is the
	// connection ending rather than a problem document - the same shape the stream has, and the
	// reason the device is validated before a byte is written.
	sink := &snapshotSink{
		buffer: bufio.NewWriterSize(w, snapshotFlushEvery), budget: snapshotByteBudget,
	}
	sink.flusher, _ = w.(http.Flusher)
	headed := false
	head := func() {
		if headed {
			return
		}
		headed = true
		w.Header().Set("Content-Type", snapshotContentType)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
	}

	count := 0
	cursor, err := snapshotter.WalkAll(ctx, actor, snapshotRequest(body), func(record syncservice.Record) error {
		head()
		count++
		return sink.line(syncChange(record))
	})
	if err != nil {
		if !headed {
			// Nothing written yet: the refusal is an ordinary problem document - the device, the
			// scope, a workspace whose installation serves no initial synchronisation.
			WriteProblem(w, err, requestID)
			return
		}
		// Mid-stream there is no status left to send. What is buffered goes out whole, and the
		// connection ends without the cursor line - which is exactly what tells the device it
		// did not read the snapshot whole.
		_ = sink.flush()
		c.count(ctx, count)
		return
	}
	head()
	if err := sink.line(map[string]any{"cursor": c.Pull.Encode(cursor)}); err != nil {
		c.count(ctx, count)
		return
	}
	_ = sink.flush()
	c.count(ctx, count)
}

// count reports the records delivered, beside the pull's, through the same counter: how many
// records left by which door.
func (c SyncController) count(ctx context.Context, records int) {
	if c.Signals != nil && records > 0 {
		c.Signals.PullRecords(ctx, records)
	}
}

// snapshotRequest maps the body onto the service's request, as pullRequest does.
func snapshotRequest(body openapi.SyncSnapshotRequest) syncservice.SnapshotRequest {
	request := syncservice.SnapshotRequest{DeviceID: shared.ID(body.DeviceId.String())}
	if body.Platform != nil {
		request.Platform = *body.Platform
	}
	if body.DisplayName != nil {
		request.DisplayName = *body.DisplayName
	}
	if body.Scopes != nil {
		for _, scope := range *body.Scopes {
			mapped := syncservice.Scope{}
			if scope.ContainerId != nil {
				mapped.ContainerID = shared.ID(scope.ContainerId.String())
			}
			if scope.Depth != nil {
				mapped.Depth = syncservice.Depth(*scope.Depth)
			}
			request.Scopes = append(request.Scopes, mapped)
		}
	}
	return request
}

// errSnapshotBudget ends a snapshot that grew past its byte budget.
var errSnapshotBudget = errors.New("the snapshot exceeded its byte budget")

// snapshotSink writes one JSON document per line, buffered, and flushes the buffer to the wire as
// it fills so that the first byte does not wait for the last row.
type snapshotSink struct {
	buffer  *bufio.Writer
	flusher http.Flusher
	budget  int64
	written int64
}

func (s *snapshotSink) line(document any) error {
	encoded, err := json.Marshal(document)
	if err != nil {
		return err
	}
	s.written += int64(len(encoded)) + 1
	if s.written > s.budget {
		return errSnapshotBudget
	}
	if len(encoded)+1 > s.buffer.Available() {
		// The buffer would overflow: what it holds goes to the wire first, whole lines only, so
		// that a connection cut mid-line is cut between lines.
		if err := s.flush(); err != nil {
			return err
		}
	}
	if _, err := s.buffer.Write(encoded); err != nil {
		return err
	}
	return s.buffer.WriteByte('\n')
}

func (s *snapshotSink) flush() error {
	if err := s.buffer.Flush(); err != nil {
		return err
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}
