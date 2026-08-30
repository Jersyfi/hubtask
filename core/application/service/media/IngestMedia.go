// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"bytes"
	"context"
	"log/slog"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

// IngestMedia is the pipeline's other end (G-11): the three steps, run by this server, for bytes
// that arrived here rather than from a client that could be told where to put them.
//
// The three-step flow exists because the server does not carry a client's bytes (arc42 §8.4). A
// mail intake is the case where it does: the message came in whole, over somebody else's bridge,
// and there is nobody to hand a presigned URL to. So the same three steps run here in order -
// staged, written, sealed - through the same repository, the same store and the same guard.
// Nothing about what an object has to survive changes; what changes is who performs the steps.
//
// Not a catalogue entry, for IntakeJumbleEntry's reason: there is no actor to authorise. The
// tenant comes from the intake's token and from nowhere else, and the object records no uploader -
// naming an account would invent a person who did this.
//
// An ingest that stages and then fails leaves bytes and a PENDING record behind, and that is what
// the reconciliation is for (C-06). It is seeded here for the same reason the staging seeds it:
// nothing in this system may enumerate tenants, so a tenant whose only uploads arrive by mail
// would otherwise have a reclaimer nobody ever started (multi-tenancy.md §2.1).
type IngestMedia struct {
	Objects repository.Objects
	Store   storage.ObjectStore
	Guard   storage.Guard
	// Jobs is the half of the queue this needs: an ingest asks for the reclaimer to run and never
	// claims any work itself, which is the same cut every other service that seeds a job makes.
	Jobs Enqueuer
	// UnitOfWork opens the two short transactions this needs. Two rather than one, and the byte
	// write between them rather than inside them: an object store is somebody else's machine, and
	// a transaction waiting on one holds a connection for as long as they feel like taking
	// (observability-reliability.md §8).
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Config     env.Config
}

// Enqueuer is the half of the queue an ingest needs: it asks for work and never claims any.
type Enqueuer interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// seedReconciliation is scheduleReconciliation for a caller with no actor. The same request, and
// the same tolerance of a build wired without a queue.
func seedReconciliation(ctx context.Context, jobs Enqueuer, tenantID shared.ID) error {
	if jobs == nil {
		return nil
	}
	_, enqueued := jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindMediaReconcile,
		TenantID:  tenantID,
		DedupeKey: tenantID.String(),
	})
	return enqueued
}

// IngestedFile is one file the server already holds.
type IngestedFile struct {
	FileName string
	// ClaimedType is what the sender called it. Held against the sniff by the guard and never
	// trusted on its own, exactly as a client's claim is (T-11).
	ClaimedType string
	Content     []byte
}

// Execute stores each file and answers the identifiers, in the order they were given.
//
// One file that cannot be stored does not lose the rest: it is skipped and the ones that worked
// are answered, because what this serves is an inbox entry, and an entry with three of its four
// attachments is worth more than no entry at all. What it does not do is answer an identifier for
// something that is not READY - a caller attaching one of these is attaching a sealed object.
func (h IngestMedia) Execute(
	ctx context.Context, tenantID shared.ID, files []IngestedFile,
) ([]shared.ID, error) {
	if tenantID.IsZero() {
		return nil, shared.ErrInternal.WithDetail("media.ingest_without_tenant")
	}

	stored := make([]shared.ID, 0, len(files))
	for _, file := range files {
		id, err := h.one(ctx, tenantID, file)
		if err != nil {
			// Logged with the identifier and the reason, never with the file name: a name off a
			// mail is user content, and rule 10 keeps it out of every log.
			slog.WarnContext(ctx, "an inbound attachment was not stored",
				slog.String("tenant_id", tenantID.String()),
				slog.String("error", err.Error()))
			continue
		}
		stored = append(stored, id)
	}
	return stored, nil
}

// one runs the three steps for a single file.
func (h IngestMedia) one(
	ctx context.Context, tenantID shared.ID, file IngestedFile,
) (shared.ID, error) {
	now := h.Clock.Now()
	scope := persistence.Scope{TenantID: tenantID}

	object, err := media.NewPendingObject(media.NewObjectInput{
		ID:           h.IDs.NewID(),
		TenantID:     tenantID,
		FileName:     file.FileName,
		ClaimedType:  file.ClaimedType,
		DeclaredSize: int64(len(file.Content)),
		SizeLimit:    h.Config.Request.MaxUploadBytes,
		Usage:        media.UsageAttachment,
		// No uploader. The intake authenticates the tenant and nobody else, and an account here
		// would be an author this system invented (G-10's reasoning for the entry's own actor).
		CreatedBy: "",
		Now:       now,
	})
	if err != nil {
		return "", err
	}

	if err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := h.Objects.Insert(ctx, object); err != nil {
			return err
		}
		return seedReconciliation(ctx, h.Jobs, tenantID)
	}); err != nil {
		return "", err
	}

	sealed, err := h.write(ctx, object, file.Content)
	if err != nil {
		// The bytes are removed where they got as far as being written, and the PENDING record is
		// left to the reconciliation. Removing the record here would be a second deletion path for
		// media, which is the one thing C-06 keeps in one place.
		h.discard(ctx, object)
		return "", err
	}

	if err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		return h.Objects.Seal(ctx, sealed)
	}); err != nil {
		return "", err
	}
	return sealed.ID, nil
}

// write judges the bytes and puts them in the store, and answers the object as it will be sealed.
//
// The guard runs before the write rather than after it, which is what a local-storage installation
// already does on its content route: the bytes pass through this process either way, so judging
// them on the way in means unacceptable bytes never reach the store at all (T-11, T-17). What the
// confirmation does for a presigned upload - read back and judge - has nothing left to add here,
// because nothing between the judgement and the write could have changed them.
func (h IngestMedia) write(
	ctx context.Context, object media.Object, content []byte,
) (media.Object, error) {
	inspection, err := h.Guard.Inspect(
		bytes.NewReader(content), object.ContentType, h.Config.Request.MaxUploadBytes)
	if err != nil {
		return media.Object{}, err
	}

	if err := h.Store.Put(ctx, storage.Upload{
		Key:         object.StorageKey,
		ContentType: inspection.ContentType,
		Size:        object.ByteSize,
		Content:     inspection.Content,
	}); err != nil {
		return media.Object{}, err
	}
	return object.Sealed(inspection.ContentType, object.ByteSize)
}

// discard removes the bytes of an ingest that failed after writing them.
func (h IngestMedia) discard(ctx context.Context, object media.Object) {
	if err := h.Store.Delete(ctx, object.StorageKey); err != nil {
		slog.WarnContext(ctx, "removing the bytes of a failed ingest failed",
			slog.String("media_id", object.ID.String()),
			slog.String("error", err.Error()))
	}
}
