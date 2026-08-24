// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package sync is the application half of the change stream: what a client is allowed to be told
// about, and where in the log it stands (C-10, ADR-0021).
//
// It is not a use case and is deliberately absent from the catalogue in domain-model.md §5, for the
// reason ReconcileMedia and RunRetention are absent: the catalogue is the list of things a person,
// an agent or a rule can *ask for*, and a stream is a connection being served rather than an action
// being performed. It has no MCP tool and no automation action, because "open a stream" is not
// something a rule could usefully do.
package sync

import (
	"context"
	"errors"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// streamScope is the token scope a stream needs. Reading, because that is what it does: every
// record it carries is a change the caller could have read through the API.
const streamScope = "items:read"

// Cursors turns a position in the change log into an opaque string and back.
//
// An interface here and a keyed HMAC in infrastructure/security, because the installation secret
// has no business above the adapters (security.md §8) - and the application never looks inside the
// string, which is what "opaque" means from this side.
type Cursors interface {
	Encode(position Position) string
	Decode(cursor string) (Position, error)
}

// Position is where a client stands in the log, and when it was told so. The moment is what decides
// whether the cursor is still usable; the sequence is what orders the walk (ADR-0021).
type Position struct {
	Seq      int64
	IssuedAt time.Time
}

// Containers is the slice of the container repository this package reads: one lookup, to resolve
// the permission path of a change. Narrow rather than the whole port, for the reason every slice in
// this codebase is - a stream that held the container repository could create one.
type Containers interface {
	Find(ctx context.Context, id shared.ID) (work.Container, error)
}

// Authorizer answers whether the actor may read a container.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Batch is one round of the stream: the records the caller may see, and where the walk now stands.
type Batch struct {
	// Records are the entries to send, oldest first. Empty is an ordinary answer - the log moved
	// and none of it concerned this caller.
	Records []Record
	// Cursor is where the walk stands *after* this batch, whether or not anything survived the
	// filter. Advancing past records the caller may not see is the point: a cursor that stalled on
	// a container somebody lost access to would re-read it forever.
	Cursor Position
	// More says the log had at least a full batch left. The caller comes straight back rather than
	// waiting to be woken.
	More bool
}

// Record is one change as a client receives it: the log entry, and the cursor that follows it.
type Record struct {
	repository.Recorded
	// Cursor is the position after this record. What the stream sends as the event's `id`, and what
	// a client sends back as `Last-Event-ID` - so a client that received half a batch resumes in
	// the middle of it rather than at its start.
	Cursor Position
}

// StreamChanges reads the change log on behalf of one connection.
//
// The authorisation is applied per record and at the moment the record is read, never by trusting
// what a subscription stated (ADR-0005, and the acceptance criterion of C-10). Permission lost
// while a connection is open therefore stops the records for that container without the client
// having to do anything, and without this package having to be told.
type StreamChanges struct {
	Changes    repository.Changes
	Containers Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
	Cursors    Cursors
	Clock      clock.Clock
	// Window is how far back a cursor may reach: the maximum offline window, which is also the
	// minimum tombstone period (offline-sync.md §7). Beyond it the log no longer holds everything
	// that happened, so a delta would be silently wrong.
	Window time.Duration
	// Batch is how many entries one round reads. A bound rather than a tuning knob: a round is a
	// read inside one transaction, and an unbounded one would hold it open across a backlog.
	Batch int
}

// Resume decides where a connection starts.
//
// An empty cursor means "from now": a client with no position is starting fresh, and the stream is
// an accelerator rather than a history - what it missed before it connected is a pull's business.
func (s StreamChanges) Resume(
	ctx context.Context, actor appshared.ActorContext, cursor string,
) (Position, error) {
	if cursor == "" {
		var latest int64
		err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
			func(ctx context.Context) error {
				var err error
				latest, err = s.Changes.Latest(ctx)
				return err
			})
		if err != nil {
			return Position{}, err
		}
		return Position{Seq: latest, IssuedAt: s.Clock.Now()}, nil
	}

	position, err := s.Cursors.Decode(cursor)
	if err != nil {
		return Position{}, err
	}
	if s.Clock.Now().Sub(position.IssuedAt) > s.Window {
		// The only safe answer. The log keeps the offline window and no longer; a delta across the
		// gap would silently omit whatever was pruned, and a client applying it would keep objects
		// that are gone (offline-sync.md §7).
		return Position{}, shared.ErrGone.
			WithDetail("sync.cursor_too_old").
			WithParams(map[string]string{"window_days": days(s.Window)})
	}
	return position, nil
}

// Next reads one round and returns what this actor may see.
func (s StreamChanges) Next(
	ctx context.Context, actor appshared.ActorContext, from Position,
) (Batch, error) {
	var entries []repository.Recorded
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		entries, err = s.Changes.After(ctx, from.Seq, s.Batch)
		return err
	})
	if err != nil {
		return Batch{}, err
	}
	if len(entries) == 0 {
		return Batch{Cursor: s.at(from.Seq)}, nil
	}

	// One decision per container rather than one per record. A batch is usually a handful of
	// changes in one or two collections, and asking the same question ten times would be ten
	// membership resolutions for one answer. The judgement is still per record: every record is
	// checked, and what is cached is the answer to the question it asks.
	permitted := map[shared.ID]bool{}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		allowed, err := s.mayRead(ctx, actor, entry.ContainerID, permitted)
		if err != nil {
			return Batch{}, err
		}
		if !allowed {
			continue
		}
		records = append(records, Record{Recorded: entry, Cursor: s.at(entry.Seq)})
	}

	last := entries[len(entries)-1].Seq
	return Batch{
		Records: records,
		// The cursor of the last entry *read*, not of the last one sent. A cursor that stalled on
		// a container somebody lost access to would re-read it on every round forever.
		Cursor: s.at(last),
		More:   len(entries) == s.Batch,
	}, nil
}

// mayRead answers whether the actor may see changes in a container, remembering the answer for the
// rest of the batch.
func (s StreamChanges) mayRead(
	ctx context.Context, actor appshared.ActorContext, containerID shared.ID,
	permitted map[shared.ID]bool,
) (bool, error) {
	if containerID.IsZero() {
		// A change that names no container is one whose visibility nothing here can decide.
		// Withheld rather than sent: nothing writes such an entry today, and the day something
		// does, the safe answer is the one that does not leak it.
		return false, nil
	}
	if allowed, decided := permitted[containerID]; decided {
		return allowed, nil
	}

	var container work.Container
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		container, findErr = s.Containers.Find(ctx, containerID)
		return findErr
	})
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// The container is gone - purged, or in another tenant and therefore invisible. Its
		// records go with it: a client cannot be told about a change in something it can no longer
		// be shown.
		permitted[containerID] = false
		return false, nil
	case err != nil:
		return false, err
	}

	authErr := s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       service.ContainerScopes(container),
		TokenScope: streamScope,
	})
	switch {
	case authErr == nil:
		permitted[containerID] = true
		return true, nil
	case errors.Is(authErr, shared.ErrForbidden), errors.Is(authErr, shared.ErrNotFound):
		permitted[containerID] = false
		return false, nil
	default:
		// Not a refusal: nobody was denied anything, the question could not be answered. Reporting
		// it as "may not see" would silently shorten the stream on a database blip.
		return false, authErr
	}
}

func (s StreamChanges) at(seq int64) Position {
	return Position{Seq: seq, IssuedAt: s.Clock.Now()}
}

// Encode is how the presentation layer turns a position into the event's `id`, without knowing what
// a cursor is made of.
func (s StreamChanges) Encode(position Position) string { return s.Cursors.Encode(position) }

// days renders a window for a message parameter. Whole days, because that is the unit the offline
// window is documented and configured in.
func days(window time.Duration) string {
	return strconv.Itoa(int(window / (24 * time.Hour)))
}
