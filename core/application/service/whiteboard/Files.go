// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"
	"errors"
	"strconv"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// AttachWhiteboardMediaName is the use case's name (UC-CNV-08).
const AttachWhiteboardMediaName = "AttachWhiteboardMedia"

// BoardMediaPlacedAction is the audit code of a file placed on a board.
const BoardMediaPlacedAction audit.Action = "whiteboard.media_placed"

// MaxMediaPerBoard bounds a board's files; the bytes are bounded by the workspace's storage
// ceiling at the upload (multi-tenancy.md §4).
const MaxMediaPerBoard = 500

// MediaObjects reads the uploads a board's files are.
type MediaObjects interface {
	Find(ctx context.Context, id shared.ID) (media.Object, error)
	// ReferencingBoards answers the hubs of the boards that already hold an object.
	ReferencingBoards(ctx context.Context, mediaID shared.ID) ([]shared.ID, error)
}

// AttachWhiteboardMedia places a confirmed upload on a board, so that its pictures and files draw
// it and every reader of the board reads it (UC-CNV-08).
//
// Placing is what grants reading, so it is judged here and not by the scene: the object is one
// the writer uploaded, or one a board they may read already holds - a picture copied from one
// board to another. Anything else is not found, as GetMedia answers it (T-04).
type AttachWhiteboardMedia struct{ Studio Studio }

// Execute places the object.
func (h AttachWhiteboardMedia) Execute(ctx context.Context, actor appshared.ActorContext, boardID, mediaID shared.ID) error {
	if h.Studio.Media == nil {
		return shared.ErrNotFound.WithDetail("whiteboards.media_not_found")
	}
	if err := actor.RequireScope(boardsWrite); err != nil {
		return err
	}
	board, err := h.Studio.placeOf(ctx, actor, boardID, false)
	if err != nil {
		return err
	}
	if err := h.Studio.authorizeWrite(ctx, actor, board.HubID, board.ID, BoardMediaPlacedAction); err != nil {
		return err
	}
	var object media.Object
	var holders []shared.ID
	err = h.Studio.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if object, err = h.Studio.Media.Find(ctx, mediaID); err != nil {
			return err
		}
		holders, err = h.Studio.Media.ReferencingBoards(ctx, mediaID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && (object.Status != media.StatusReady || object.DeletedAt != nil)) {
		return shared.ErrNotFound.WithDetail("whiteboards.media_not_found")
	}
	if err != nil {
		return err
	}
	if object.CreatedBy != actor.AccountID {
		mayRead, err := h.anyReadable(ctx, actor, holders)
		if err != nil {
			return err
		}
		if !mayRead {
			return shared.ErrNotFound.WithDetail("whiteboards.media_not_found")
		}
	}
	return h.Studio.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Studio.find(ctx, board.ID, true, false); err != nil {
			return err
		}
		count, err := h.Studio.Boards.CountMedia(ctx, board.ID)
		if err != nil {
			return err
		}
		if count >= MaxMediaPerBoard {
			return shared.ErrConflict.WithDetail("whiteboards.media_full").
				WithParams(map[string]string{"maximum": strconv.Itoa(MaxMediaPerBoard)})
		}
		if err := h.Studio.Boards.AttachMedia(ctx, board.ID, mediaID); err != nil {
			return err
		}
		return h.Studio.record(ctx, actor, BoardMediaPlacedAction, board, nil)
	})
}

func (h AttachWhiteboardMedia) anyReadable(ctx context.Context, actor appshared.ActorContext, hubs []shared.ID) (bool, error) {
	readable, err := h.Studio.readableHubs(ctx, actor, hubs)
	if err != nil {
		return false, err
	}
	for _, may := range readable {
		if may {
			return true, nil
		}
	}
	return false, nil
}

// Descriptor is the catalogue entry.
func (h AttachWhiteboardMedia) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: AttachWhiteboardMediaName, Permission: service.PermissionWriteItems,
		Summary: "Places a confirmed upload on a whiteboard, so that its pictures and file objects draw it and " +
			"every reader of the board may read it: one the caller uploaded, or one a board they may read holds.",
		SideEffects: "Writes the placement and an audit entry.",
		TokenScope:  boardsWrite,
		Input: []usecase.Field{
			{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."},
			{Name: "media_id", Kind: usecase.KindID, Required: true, Description: "A confirmed upload."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardMediaPlacedAction, TargetType: boardTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			board, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			object, err := in.ID("media_id")
			if err != nil {
				return nil, err
			}
			if err := h.Execute(ctx, actor, board, object); err != nil {
				return nil, err
			}
			return usecase.Output{"media_id": object.String()}, nil
		}),
	})
}
