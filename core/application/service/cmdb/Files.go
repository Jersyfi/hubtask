// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use cases' names (UC-AST-01; F-1, IN-2).
const (
	AttachCiMediaName = "AttachCiMedia"
	DetachCiMediaName = "DetachCiMedia"
)

// The audit codes of a file put on and taken off an object; the file by identifier only (rule 10).
const (
	CIFileAttachedAction audit.Action = "cmdb.ci_file_attached"
	CIFileRemovedAction  audit.Action = "cmdb.ci_file_removed"
)

// MaxFilesPerCI bounds an object's files; the bytes are bounded by the workspace's storage ceiling
// at the upload (multi-tenancy.md §4).
const MaxFilesPerCI = 500

// Files is what an object's files are written through: the links, and the uploads they name with
// their reference count.
type Files interface {
	mediarepo.CiFiles
	Find(ctx context.Context, id shared.ID) (media.Object, error)
	AdjustRefCount(ctx context.Context, id shared.ID, delta int) error
}

// AttachCiMedia puts a confirmed upload on an object, so that every reader of its hub reads it.
//
// Holding is what grants reading, so it is judged here: the upload is one the writer made, or one
// an object of a hub they read already holds. Anything else is not found, as GetMedia answers it
// (T-04). The bytes are shared, never copied; the reference count moves with the link.
type AttachCiMedia struct{ Inventory Inventory }

// DetachCiMedia takes a file off an object. The upload stays - other things may hold it - and one
// that loses its last reference is removed by the media reconciliation, not by this call.
type DetachCiMedia struct{ Inventory Inventory }

func filesNotFound() error { return shared.ErrNotFound.WithDetail("media.not_found") }

// Execute attaches the upload and answers what the object now holds.
func (h AttachCiMedia) Execute(ctx context.Context, actor appshared.ActorContext, ciID, mediaID shared.ID) ([]shared.ID, error) {
	return h.Inventory.changeFiles(ctx, actor, ciID, mediaID, true)
}

// Execute detaches the upload and answers what the object now holds.
func (h DetachCiMedia) Execute(ctx context.Context, actor appshared.ActorContext, ciID, mediaID shared.ID) ([]shared.ID, error) {
	return h.Inventory.changeFiles(ctx, actor, ciID, mediaID, false)
}

func (s Inventory) changeFiles(ctx context.Context, actor appshared.ActorContext, ciID, mediaID shared.ID, attach bool) ([]shared.ID, error) {
	if s.Files == nil {
		return nil, filesNotFound()
	}
	action := CIFileAttachedAction
	if !attach {
		action = CIFileRemovedAction
	}
	placed, err := s.placeOf(ctx, actor, ciID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeHub(ctx, actor, placed.HubID, true, action, ciTarget, ciID); err != nil {
		return nil, err
	}
	if attach {
		if err := s.mayHold(ctx, actor, mediaID); err != nil {
			return nil, err
		}
	}
	var held []shared.ID
	err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := s.Store.FindCI(ctx, ciID, true); err != nil {
			return err
		}
		moved, err := s.linkFile(ctx, ciID, mediaID, attach)
		if err != nil {
			return err
		}
		if held, err = s.Files.CiMediaIDs(ctx, ciID); err != nil || !moved {
			return err
		}
		verb := domain.VerbFileAttached
		if !attach {
			verb = domain.VerbFileRemoved
		}
		if err := s.chronicle(ctx, actor, ciID, verb, map[string]domain.Moved{"media_id": fileSide(mediaID, attach)}); err != nil {
			return err
		}
		return s.record(ctx, actor, action, ciTarget, ciID,
			audit.Change{Field: "media_id", Classification: audit.Open, To: mediaID.String()})
	})
	if err != nil {
		return nil, err
	}
	return held, nil
}

// linkFile writes or removes the link and moves the reference count in the same transaction;
// it reports whether the set moved.
func (s Inventory) linkFile(ctx context.Context, ciID, mediaID shared.ID, attach bool) (bool, error) {
	if !attach {
		removed, err := s.Files.DetachFromCi(ctx, ciID, mediaID)
		if err != nil || !removed {
			return false, err
		}
		return true, s.Files.AdjustRefCount(ctx, mediaID, -1)
	}
	count, err := s.Files.CountCiMedia(ctx, ciID)
	if err != nil {
		return false, err
	}
	if count >= MaxFilesPerCI {
		ids, err := s.Files.CiMediaIDs(ctx, ciID)
		if err != nil {
			return false, err
		}
		for _, id := range ids {
			if id == mediaID {
				return false, nil
			}
		}
		return false, shared.ErrConflict.WithDetail("cmdb.files_full").WithParams(map[string]string{"maximum": itoa(MaxFilesPerCI)})
	}
	added, err := s.Files.AttachToCi(ctx, ciID, mediaID)
	if err != nil || !added {
		return false, err
	}
	return true, s.Files.AdjustRefCount(ctx, mediaID, 1)
}

// mayHold refuses an upload the media context does not stand behind, or one the writer neither
// made nor reads on an object of a hub they read.
func (s Inventory) mayHold(ctx context.Context, actor appshared.ActorContext, mediaID shared.ID) error {
	var object media.Object
	var hubs []shared.ID
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if object, err = s.Files.Find(ctx, mediaID); err != nil {
			return err
		}
		hubs, err = s.Files.ReferencingCis(ctx, mediaID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return filesNotFound()
	}
	if err != nil {
		return err
	}
	if object.DeletedAt != nil || (object.CreatedBy != actor.AccountID && !s.readsAnyHub(ctx, actor, hubs)) {
		return filesNotFound()
	}
	return object.Attachable(media.UsageAttachment)
}

// readsAnyHub answers whether the actor reads one of the hubs, without recording a refusal: the
// question is about another object's file, and its answer is the not-found above.
func (s Inventory) readsAnyHub(ctx context.Context, actor appshared.ActorContext, hubs []shared.ID) bool {
	if s.Reader == nil || len(hubs) == 0 {
		return false
	}
	readable, err := Pointers{Reader: s.Reader}.readableHubs(ctx, actor, hubs)
	if err != nil {
		return false
	}
	for _, may := range readable {
		if may {
			return true
		}
	}
	return false
}

func fileSide(mediaID shared.ID, attach bool) domain.Moved {
	if attach {
		return domain.Moved{To: mediaID.String()}
	}
	return domain.Moved{From: mediaID.String()}
}

// CiFilesOutput is the shape both directions answer (schema CiAttachments).
func CiFilesOutput(ciID shared.ID, ids []shared.ID) usecase.Output {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return usecase.Output{"ci_id": ciID.String(), "media_ids": out}
}

func filesInput(verb string) []usecase.Field {
	return []usecase.Field{
		{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
		{Name: "media_id", Kind: usecase.KindID, Required: true, Description: "The media object to " + verb + "."},
	}
}

func filesHandler(run func(context.Context, appshared.ActorContext, shared.ID, shared.ID) ([]shared.ID, error)) usecase.Handler {
	return usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
		ci, err := in.ID("ci_id")
		if err != nil {
			return nil, err
		}
		object, err := in.ID("media_id")
		if err != nil {
			return nil, err
		}
		held, err := run(ctx, actor, ci, object)
		if err != nil {
			return nil, err
		}
		return CiFilesOutput(ci, held), nil
	})
}

// Descriptor is the catalogue entry.
func (h AttachCiMedia) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: AttachCiMediaName, Permission: service.PermissionWriteItems,
		Summary: "Puts a confirmed upload on a configuration item, so that every reader of its hub may read it: one " +
			"the caller uploaded, or one an object of a hub they read holds. Idempotent. The bytes are shared, never " +
			"copied - attaching raises the upload's reference count.",
		SideEffects: "Writes the link, raises the reference count, a step of the object's history and an audit entry.",
		TokenScope:  cisWrite,
		Input:       filesInput("attach"),
		Audit:       usecase.AuditDeclaration{Action: CIFileAttachedAction, TargetType: ciTarget, Severity: audit.SeverityInfo, Required: true},
		Handler:     filesHandler(h.Execute),
	})
}

// Descriptor is the catalogue entry.
func (h DetachCiMedia) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DetachCiMediaName, Permission: service.PermissionWriteItems,
		Summary: "Takes a file off a configuration item. Idempotent. The upload itself stays - other things may hold " +
			"it - and one that loses its last reference is removed by the media reconciliation.",
		SideEffects: "Removes the link, lowers the reference count, writes a step of the object's history and an audit entry.",
		TokenScope:  cisWrite,
		Input:       filesInput("detach"),
		Audit:       usecase.AuditDeclaration{Action: CIFileRemovedAction, TargetType: ciTarget, Severity: audit.SeverityInfo, Required: true},
		Handler:     filesHandler(h.Execute),
	})
}
