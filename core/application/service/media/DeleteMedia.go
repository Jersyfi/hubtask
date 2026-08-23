// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	DeleteMediaName = "DeleteMedia"

	// MediaDeletedAction is the audit code. Stable: an auditor filters on it and a SIEM rule
	// matches on it (audit.md §2).
	MediaDeletedAction audit.Action = "media.deleted"
)

// Authorizer is the slice of the authorisation service this package needs for a single path.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// DeleteMedia removes a media object nothing points at.
//
// The record is marked here and the bytes go with the reconciliation job, which is the deletion
// path data-protection.md §5 promises rather than a shortcut around it: a request has no business
// waiting on a bucket, and the journal entry the retention model requires is written where the row
// finally goes. What this call does is decide that the object may go at all.
//
// Refused while anything references it. `ON DELETE RESTRICT` on `item_attachment` and on the
// cover's foreign key would refuse it anyway, and being refused by a constraint is being refused
// with a message nobody can act on - so the counter is asked first and the answer names the fix.
type DeleteMedia struct {
	Objects    repository.Objects
	Authorizer Authorizer
	Audit      audit.Sink
	// Jobs is where the tenant's reconciliation is pulled forward: this call marks a row and the
	// pass is what takes its bytes, so a deletion that scheduled nothing would wait for the
	// interval to come round (queue.KindMediaReconcile).
	Jobs       queue.Queue
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// DeleteCommand is the input, typed.
type DeleteCommand struct{ MediaID shared.ID }

// Execute marks the object for removal.
func (h DeleteMedia) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DeleteCommand,
) error {
	if err := actor.RequireScope(mediaWrite); err != nil {
		return err
	}
	if cmd.MediaID.IsZero() {
		return mediaIDRequired()
	}

	object, err := h.find(ctx, actor, cmd.MediaID)
	if err != nil {
		return err
	}
	if err := h.ensureMayRemove(ctx, actor, object); err != nil {
		return err
	}

	now := h.Clock.Now()
	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		marked, err := h.Objects.MarkDeleted(ctx, object.ID, now)
		if err != nil {
			return err
		}
		if !marked {
			// Nothing matched: the object gained a reference between the read and here, or lost
			// the race to another deletion. Either way the state is one the caller can act on -
			// the counter says what is in the way, and the answer says to clear it first.
			return shared.ErrConflict.
				WithDetail("media.still_referenced").
				WithParams(map[string]string{"media_id": object.ID.String()})
		}
		if err := scheduleReconciliation(ctx, h.Jobs, actor.TenantID); err != nil {
			return err
		}
		return h.recordAudit(ctx, object, actor, now)
	})
}

// find reads the object, and reports one that is already on its way out as missing.
func (h DeleteMedia) find(
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
	if object.DeletedAt != nil {
		// Already marked. Not an error worth its own answer: the object is gone as far as anything
		// outside the reconciliation job is concerned, and a second deletion of a deleted object
		// is a client retrying (T-04 keeps the answer the same as an unknown identifier).
		return media.Object{}, mediaNotFound(mediaID)
	}
	return object, nil
}

// ensureMayRemove answers who may: whoever staged it, or an administrator of the workspace.
//
// The uploader passes without an authorisation question, for the reason the confirmation does -
// a staged object serves nothing and sits under nobody's container, so there is no path to resolve
// a membership along. Everybody else is asked at the tenant scope, which is what "an
// administrator" means here: a role held on one hub says nothing about a file that may be attached
// to entries in another.
func (h DeleteMedia) ensureMayRemove(
	ctx context.Context, actor appshared.ActorContext, object media.Object,
) error {
	if object.CreatedBy == actor.AccountID {
		return nil
	}

	// Before any transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside the write transaction would be rolled back together with the refusal (audit.md §7).
	return h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     MediaDeletedAction,
		TokenScope: mediaWrite,
		TargetType: mediaTarget,
		TargetID:   object.ID,
	})
}

// recordAudit writes the evidence.
//
// The file name is not in it, for the reason the staging keeps it out: it is user content, and
// rule 10 keeps user content out of the trail. What an auditor needs is who removed how many bytes
// of what kind, and whose upload it was.
func (h DeleteMedia) recordAudit(
	ctx context.Context, object media.Object, actor appshared.ActorContext, now time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   object.TenantID,
		OccurredAt: now,
		Action:     MediaDeletedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: mediaTarget,
		TargetID:   object.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "usage", Classification: audit.Open, To: string(object.Usage)},
			audit.Change{Field: "size", Classification: audit.Open, To: sizeString(object.ByteSize)},
			// The uploader by identifier and in clear text, exactly as the assignment trail
			// records an account: an identifier is not content, and "whose file was removed, by
			// whom" is not answerable without it (audit.md §4, rule 10).
			audit.Change{
				Field: "uploaded_by", Classification: audit.Open, To: object.CreatedBy.String(),
			},
		),
	})
}

// Descriptor is the catalogue entry.
func (h DeleteMedia) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteMediaName,
		Summary: "Removes a media object nothing points at: the record now, the bytes through the " +
			"reconciliation job, which is what writes the deletion journal entry. Refused while " +
			"anything references it - detach it or clear the cover first. Only the account that " +
			"staged it, or an administrator of the workspace, may.",
		SideEffects: "Marks the record for removal and writes an audit entry. The bytes are " +
			"removed by the reconciliation job, outside this request.",
		TokenScope: mediaWrite,
		Input: []usecase.Field{
			{
				Name: "media_id", Kind: usecase.KindID, Required: true,
				Description: "The media object to remove. It has to be unreferenced: no cover and " +
					"no attachment may point at it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MediaDeletedAction, TargetType: mediaTarget,
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

func (h DeleteMedia) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	mediaID, err := in.ID("media_id")
	if err != nil {
		return nil, err
	}

	if err := h.Execute(ctx, actor, DeleteCommand{MediaID: mediaID}); err != nil {
		return nil, err
	}
	// Nothing to project: the record is on its way out, and a body describing what was just
	// removed would be a state no read will confirm. The contract answers 204.
	return usecase.Output{}, nil
}
