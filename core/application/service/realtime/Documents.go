// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package realtime serves live documents (F-8, offline-sync.md §13): who may open and write one,
// the order its updates are handed out in, and the snapshot that compacts its log.
//
// Not catalogue use cases, for the sync pull's reason: a connection is a stream of small acts on
// one document, and each is authorised here against the place the document's kind names. What a
// document belongs to - a page, a whiteboard - is the kind's; this package knows only that a kind
// can say where a document sits.
package realtime

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/realtime"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The actions a refusal is recorded under.
const (
	DocumentReadAction  audit.Action = "realtime.document.read"
	DocumentWriteAction audit.Action = "realtime.document.write"
)

// The token scopes a document needs: the entries' own, because a token's vocabulary is the
// catalogue's (catalogue.Scopes) and a live document is work like an entry is.
const (
	documentsRead  = "items:read"
	documentsWrite = "items:write"
)

// documentTarget is what a refusal names.
const documentTarget = "realtime_document"

// Place is where a document sits, as its kind says: the scope path its reader and writer are judged
// on, and the permissions each needs there.
type Place struct {
	Path  []identity.Scope
	Read  service.Permission
	Write service.Permission
	// On names the entry a document belongs to, so a role reading only its own entries is
	// judged on it as on the entry (an entry's presence).
	On access.ItemSubject
	// PresenceOnly is a document that carries awareness alone - who is here, who is replying -
	// and never an update or a snapshot: nothing of it is stored.
	PresenceOnly bool
}

// Locator is a kind's answer to "where does this document sit". It runs inside a read-only
// transaction the service opens; a document whose owner is gone answers shared.ErrNotFound.
type Locator interface {
	Locate(ctx context.Context, actor appshared.ActorContext, id shared.ID) (Place, error)
}

// Authorizer records a refusal and answers it as an error; Permits answers without a record, for
// a question that is not anybody trying anything (whether to offer writing, a connection's
// re-check after a wake).
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
	Permits(ctx context.Context, actor appshared.ActorContext, request access.Request) (bool, error)
}

// Documents is the service.
type Documents struct {
	Store      repository.Documents
	Kinds      map[domain.Kind]Locator
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Opened is what a connection starts from: the snapshot, the updates after it, where the log
// stands, and whether the reader may write.
type Opened struct {
	Head     domain.Head
	Snapshot *domain.Snapshot
	Updates  []domain.Update
	// More is a log longer past its snapshot than one opening reads; the rest follows with Since.
	More     bool
	Writable bool
}

// Open authorises the reader and reads the document.
func (d Documents) Open(ctx context.Context, actor appshared.ActorContext, ref domain.Ref) (Opened, error) {
	place, err := d.locate(ctx, actor, ref)
	if err != nil {
		return Opened{}, err
	}
	if err := d.Authorizer.Authorize(ctx, actor, d.request(ref, place, place.Read, DocumentReadAction, documentsRead)); err != nil {
		return Opened{}, err
	}
	writable, err := d.mayWrite(ctx, actor, ref, place)
	if err != nil {
		return Opened{}, err
	}
	out := Opened{Writable: writable}
	err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if out.Head, err = d.Store.Head(ctx, ref); err != nil {
			return err
		}
		snapshot, found, err := d.Store.Snapshot(ctx, ref)
		if err != nil {
			return err
		}
		after := int64(0)
		if found {
			out.Snapshot, after = &snapshot, snapshot.UpToSeq
		}
		out.Updates, err = d.Store.Since(ctx, ref, after, domain.MaxOpenUpdates)
		return err
	})
	if err != nil {
		return Opened{}, err
	}
	out.More = len(out.Updates) == domain.MaxOpenUpdates
	return out, nil
}

// Since answers the updates after a place, for a connection woken by a write or catching up. The
// reader is asked again, without a record: access ended while connected ends the connection on
// the next update rather than at the next opening.
func (d Documents) Since(
	ctx context.Context, actor appshared.ActorContext, ref domain.Ref, after int64,
) ([]domain.Update, error) {
	place, err := d.locate(ctx, actor, ref)
	if err != nil {
		return nil, err
	}
	may, err := d.Authorizer.Permits(ctx, actor, d.request(ref, place, place.Read, DocumentReadAction, documentsRead))
	if err != nil {
		return nil, err
	}
	if !may {
		return nil, shared.ErrForbidden.WithDetail("realtime.access_ended")
	}
	var updates []domain.Update
	err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		updates, err = d.Store.Since(ctx, ref, after, domain.MaxOpenUpdates)
		return err
	})
	return updates, err
}

// Appended is where an update landed, and whether the writer should send a snapshot.
type Appended struct {
	Seq           int64
	CompactionDue bool
}

// Append stores one update from a writer.
func (d Documents) Append(
	ctx context.Context, actor appshared.ActorContext, ref domain.Ref, data []byte,
) (Appended, error) {
	if err := domain.CheckUpdate(data); err != nil {
		return Appended{}, err
	}
	if err := d.authorizeWrite(ctx, actor, ref); err != nil {
		return Appended{}, err
	}
	var out Appended
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		seq, err := d.Store.Append(ctx, ref, data, actor.AccountID, d.Clock.Now())
		if err != nil {
			return err
		}
		head, err := d.Store.Head(ctx, ref)
		if err != nil {
			return err
		}
		out = Appended{Seq: seq, CompactionDue: head.CompactionDue()}
		return nil
	})
	return out, err
}

// Compact stores a writer's snapshot of the document up to an update. The updates it covers stay
// until the history's retention period has passed (DOCUMENT_HISTORY).
func (d Documents) Compact(
	ctx context.Context, actor appshared.ActorContext, ref domain.Ref, upTo int64, state []byte,
) error {
	if err := d.authorizeWrite(ctx, actor, ref); err != nil {
		return err
	}
	snapshot := domain.Snapshot{UpToSeq: upTo, State: state, At: d.Clock.Now()}
	return d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		head, err := d.Store.Head(ctx, ref)
		if err != nil {
			return err
		}
		if err := domain.CheckSnapshot(snapshot, head.Seq, head.SnapshotSeq); err != nil {
			return err
		}
		stored, err := d.Store.StoreSnapshot(ctx, ref, snapshot)
		if err != nil {
			return err
		}
		if !stored {
			return shared.ErrVersionConflict.WithDetail("realtime.snapshot_superseded")
		}
		return nil
	})
}

// Relayable answers whether an awareness message may be relayed: a reader's cursor and name go to
// the other readers of the document, bounded, never stored.
func (d Documents) Relayable(data []byte) error { return domain.CheckAwareness(data) }

func (d Documents) authorizeWrite(ctx context.Context, actor appshared.ActorContext, ref domain.Ref) error {
	place, err := d.locate(ctx, actor, ref)
	if err != nil {
		return err
	}
	if place.PresenceOnly {
		return shared.ErrForbidden.WithDetail("realtime.read_only")
	}
	return d.Authorizer.Authorize(ctx, actor, d.request(ref, place, place.Write, DocumentWriteAction, documentsWrite))
}

func (d Documents) mayWrite(ctx context.Context, actor appshared.ActorContext, ref domain.Ref, place Place) (bool, error) {
	if place.PresenceOnly || actor.RequireScope(documentsWrite) != nil {
		return false, nil
	}
	return d.Authorizer.Permits(ctx, actor, d.request(ref, place, place.Write, DocumentWriteAction, documentsWrite))
}

// locate asks the document's kind where it sits. An unknown kind is not found: a client cannot
// tell a kind this build does not serve from a document that is not there.
func (d Documents) locate(ctx context.Context, actor appshared.ActorContext, ref domain.Ref) (Place, error) {
	locator, known := d.Kinds[ref.Kind]
	if !known {
		return Place{}, shared.ErrNotFound.WithDetail("realtime.document_not_found")
	}
	var place Place
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		place, err = locator.Locate(ctx, actor, ref.ID)
		return err
	})
	return place, err
}

func (d Documents) request(
	ref domain.Ref, place Place, permission service.Permission, action audit.Action, scope string,
) access.Request {
	return access.Request{
		Permission: permission, Path: place.Path, Action: action, TokenScope: scope,
		TargetType: documentTarget, TargetID: ref.ID, On: place.On,
	}
}
