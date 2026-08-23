// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"log/slog"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// ConfirmMediaUploadName is the catalogue name.
const ConfirmMediaUploadName = "ConfirmMediaUpload"

// ConfirmMediaUpload seals a staged upload: it reads the bytes back, judges them, and marks the
// object READY with what they turned out to be.
//
// The judgement happens here and not on the way in, and that is the price of not carrying the
// bytes. A presigned upload goes to the bucket without passing this server at all, so the only
// moment the content can be held against its claim is on the way to READY - which is why nothing
// may use an object that is not READY (media.Object.Attachable, T-11).
//
// Idempotent, and idempotent by state rather than by an idempotency key: confirming an object that
// is already READY answers with what is there. That is what makes "an upload confirmed twice
// yields one media object" true even for a client that retried without a key.
type ConfirmMediaUpload struct {
	Objects    repository.Objects
	Store      storage.ObjectStore
	Guard      storage.Guard
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Config     env.Config
}

// ConfirmCommand is the input, typed.
type ConfirmCommand struct{ MediaID shared.ID }

// Execute seals the object and returns it.
func (h ConfirmMediaUpload) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ConfirmCommand,
) (media.Object, error) {
	if err := actor.RequireScope(mediaWrite); err != nil {
		return media.Object{}, err
	}
	if cmd.MediaID.IsZero() {
		return media.Object{}, shared.ErrValidation.
			WithDetail("media.media_id_required").
			WithFields(shared.FieldError{Path: "/media_id", Code: "media.media_id_required"})
	}

	object, err := h.stagedBy(ctx, actor, cmd.MediaID)
	if err != nil {
		return media.Object{}, err
	}
	if object.Status == media.StatusReady {
		// Already sealed. Nothing is read back, nothing is written and nothing is recorded a
		// second time - which is what makes a retry harmless rather than merely accepted.
		return object, nil
	}

	judged, err := h.judge(ctx, object)
	if err != nil {
		// The staged bytes are what the refusal was about, and leaving them would leave a file
		// this installation has judged unacceptable sitting in its bucket until the reconciliation
		// job comes past. Removing them is best effort: the refusal stands either way, and the job
		// is the backstop that makes it eventual rather than lost.
		h.discard(ctx, object)
		return media.Object{}, err
	}

	sealed, err := object.Sealed(judged.contentType, judged.size)
	if err != nil {
		return media.Object{}, err
	}

	now := h.Clock.Now()
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Objects.Seal(ctx, sealed); err != nil {
			return err
		}
		return h.recordAudit(ctx, object, sealed, actor, now)
	})
	if errors.Is(err, shared.ErrConflict) {
		// Somebody confirmed it between the read and the write. The answer is the state that is
		// there rather than a conflict the client can do nothing about - two confirmations of one
		// upload are one media object, whichever of them arrives second.
		return h.stagedBy(ctx, actor, cmd.MediaID)
	}
	if err != nil {
		return media.Object{}, err
	}
	return sealed, nil
}

// judgement is what reading the bytes back decided.
type judgement struct {
	contentType string
	size        int64
}

// judge reads the object's head back from storage and holds it against what was declared.
//
// Outside any transaction, deliberately: storage is an external dependency, and a transaction
// waiting on a bucket holds a connection for as long as somebody else's server feels like taking
// (observability-reliability.md §8).
//
// The head is all that is read. The type is decided by the WHATWG sniffing algorithm, which never
// looks past 512 bytes, and the size comes from the store rather than from counting - counting
// would mean streaming 64 MiB back through this server to learn a number the store already knows,
// which is the traffic the presigned flow exists to avoid.
func (h ConfirmMediaUpload) judge(ctx context.Context, object media.Object) (judgement, error) {
	stored, err := h.Store.Get(ctx, object.StorageKey)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The record exists and its bytes do not: the upload never happened, or it failed
			// halfway. Its own code, because the fix is to upload again rather than to stage again.
			return judgement{}, shared.ErrValidation.
				WithDetail("media.content_missing").
				WithFields(shared.FieldError{Path: "/media_id", Code: "media.content_missing"})
		}
		return judgement{}, err
	}
	defer func() { _ = stored.Content.Close() }()

	limit := h.Config.Request.MaxUploadBytes
	if limit > 0 && stored.Size > limit {
		// Bounded again here, because a presigned PUT went to the bucket without passing this
		// server: the declaration was checked at staging and the bucket checked nothing (T-17).
		return judgement{}, media.TooLarge(limit)
	}
	if stored.Size != object.ByteSize {
		return judgement{}, shared.ErrValidation.
			WithDetail("media.size_mismatch").
			WithParams(map[string]string{
				"declared": sizeString(object.ByteSize), "written": sizeString(stored.Size),
			}).
			WithFields(shared.FieldError{Path: "/media_id", Code: "media.size_mismatch"})
	}

	// The claim on the record is the one made at staging. Holding it against the sniff is the
	// whole of T-11: a lie about a renderable type is refused, and what is stored is what the
	// bytes are.
	inspection, err := h.Guard.Inspect(stored.Content, object.ContentType, limit)
	if err != nil {
		return judgement{}, err
	}
	return judgement{contentType: inspection.ContentType, size: stored.Size}, nil
}

// discard removes the bytes a refused confirmation staged.
func (h ConfirmMediaUpload) discard(ctx context.Context, object media.Object) {
	if err := h.Store.Delete(ctx, object.StorageKey); err != nil {
		slog.WarnContext(ctx, "removing the bytes of a refused upload failed",
			slog.String("media_id", object.ID.String()),
			slog.String("error", err.Error()))
	}
}

// stagedBy reads the object, and reports it missing to anybody but the person who staged it.
//
// Not forbidden: an object nobody else may confirm is one whose existence they have no business
// learning, and the answer is the same one another tenant's identifier produces (T-04). An
// administrator is not an exception here - confirming is finishing an upload somebody else began,
// and there is nothing to administer about it.
func (h ConfirmMediaUpload) stagedBy(
	ctx context.Context, actor appshared.ActorContext, mediaID shared.ID,
) (media.Object, error) {
	var object media.Object

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Objects.Find(ctx, mediaID)
		if err != nil {
			return err
		}
		object = found
		return nil
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return media.Object{}, mediaNotFound(mediaID)
		}
		return media.Object{}, err
	}
	if object.CreatedBy != actor.AccountID || object.DeletedAt != nil {
		return media.Object{}, mediaNotFound(mediaID)
	}
	return object, nil
}

// mediaNotFound is the one answer for an object the actor cannot reach, whatever the reason - the
// row is another tenant's, it is somebody else's, or it is not there at all. One answer, because
// telling them apart is an oracle for which identifiers exist (T-04, multi-tenancy.md §2).
func mediaNotFound(mediaID shared.ID) error {
	return shared.ErrNotFound.
		WithDetail("media.not_found").
		WithParams(map[string]string{"media_id": mediaID.String()})
}

// recordAudit writes the evidence: what the bytes turned out to be, beside what was claimed.
func (h ConfirmMediaUpload) recordAudit(
	ctx context.Context, before, after media.Object, actor appshared.ActorContext, now time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   after.TenantID,
		OccurredAt: now,
		Action:     MediaConfirmedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: mediaTarget,
		TargetID:   after.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{
				Field: "status", Classification: audit.Open,
				From: string(before.Status), To: string(after.Status),
			},
			// Both halves of the judgement, because "what did this server decide these bytes were,
			// and what was it told" is the question an incident asks of this entry.
			audit.Change{
				Field: "content_type", Classification: audit.Open,
				From: before.ContentType, To: after.ContentType,
			},
			audit.Change{
				Field: "size", Classification: audit.Open,
				From: sizeString(before.ByteSize), To: sizeString(after.ByteSize),
			},
		),
	})
}

// Descriptor is the catalogue entry.
func (h ConfirmMediaUpload) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfirmMediaUploadName,
		Summary: "Seals a staged upload: reads the bytes back from storage, sniffs them, checks the " +
			"size against what was declared and against the installation's limit, and marks the " +
			"object READY with the judged content type - never the claim. A refusal removes the " +
			"staged bytes. Idempotent: confirming a READY object succeeds and changes nothing.",
		SideEffects: "Reads the object back from storage, writes the sealed record and an audit " +
			"entry, and on a refusal removes the staged bytes.",
		TokenScope: mediaWrite,
		Input: []usecase.Field{
			{
				Name: "media_id", Kind: usecase.KindID, Required: true,
				Description: "The staged object to seal. Only the account that staged it may.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MediaConfirmedAction, TargetType: mediaTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A media object is not a work item and has no history of its own; what a person " +
				"reads is the entry it ends up covering or attached to, and that entry's history " +
				"records the attachment rather than the upload.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ConfirmMediaUpload) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	mediaID, err := in.ID("media_id")
	if err != nil {
		return nil, err
	}

	object, err := h.Execute(ctx, actor, ConfirmCommand{MediaID: mediaID})
	if err != nil {
		return nil, err
	}
	return mediaOutput(object), nil
}
