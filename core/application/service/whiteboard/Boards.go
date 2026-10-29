// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	CreateWhiteboardName  = "CreateWhiteboard"
	ReadWhiteboardName    = "ReadWhiteboard"
	ListWhiteboardsName   = "ListWhiteboards"
	UpdateWhiteboardName  = "UpdateWhiteboard"
	TrashWhiteboardName   = "TrashWhiteboard"
	RestoreWhiteboardName = "RestoreWhiteboard"
	ExportWhiteboardName  = "ExportWhiteboard"
)

// ExportFormat names the native document ExportWhiteboard writes and CreateWhiteboard reads back.
const ExportFormat = "hubtask.whiteboard"

// ExportVersion is the native document's version; a later shape raises it.
const ExportVersion = 1

// CreateWhiteboardCommand is the input, typed.
type CreateWhiteboardCommand struct {
	// ID is an identifier the client minted; empty mints one.
	ID    shared.ID
	HubID shared.ID
	Title string
	// Scene is the board's first objects - a template's, or an exported board's read back.
	Scene *domain.Scene
}

// CreateWhiteboard makes a new board in a hub (UC-CNV-01), empty, or from a native document
// exported before (UC-CNV-03).
type CreateWhiteboard struct{ Studio Studio }

// Execute creates the board.
func (h CreateWhiteboard) Execute(ctx context.Context, actor appshared.ActorContext, cmd CreateWhiteboardCommand) (domain.Board, error) {
	if err := h.Studio.authorizeWrite(ctx, actor, cmd.HubID, cmd.HubID, BoardCreatedAction); err != nil {
		return domain.Board{}, err
	}
	id := cmd.ID
	if id.IsZero() {
		id = h.Studio.IDs.NewID()
	}
	scene := domain.Scene{}
	if cmd.Scene != nil {
		scene = *cmd.Scene
	}
	var board domain.Board
	err := h.Studio.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Studio.hub(ctx, cmd.HubID); err != nil {
			return err
		}
		count, err := h.Studio.Boards.Count(ctx, cmd.HubID)
		if err != nil {
			return err
		}
		if count >= domain.MaxBoardsPerHub {
			return shared.ErrConflict.WithDetail("whiteboards.hub_full").WithParams(map[string]string{"maximum": "1000"})
		}
		board, err = domain.NewBoard(domain.NewBoardInput{
			ID: id, TenantID: actor.TenantID, HubID: cmd.HubID, Title: cmd.Title, Scene: scene,
			CreatedBy: actor.AccountID, Now: h.Studio.Clock.Now(), Text: h.Studio.Text,
		})
		if err != nil {
			return err
		}
		if err := h.Studio.Boards.Insert(ctx, board); err != nil {
			return err
		}
		return h.Studio.record(ctx, actor, BoardCreatedAction, board, []string{domain.FieldTitle, domain.FieldScene})
	})
	return board, err
}

// Descriptor is the catalogue entry.
func (h CreateWhiteboard) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreateWhiteboardName, Permission: service.PermissionWriteItems,
		Summary: "Makes a new whiteboard in a hub, empty or with objects - a template's, or the elements " +
			"of a board exported before, which reads an export back.",
		SideEffects: "Writes the board and an audit entry.",
		TokenScope:  boardsWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub the board belongs to."},
			{Name: "title", Kind: usecase.KindString, Required: true, Description: "The board's title, one line."},
			elementsField,
			{Name: "id", Kind: usecase.KindID, CallerOnly: true, Description: "An identifier the client minted."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardCreatedAction, TargetType: boardTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			var id shared.ID
			if in.String("id") != "" {
				if id, err = in.ID("id"); err != nil {
					return nil, err
				}
			}
			scene, err := h.Studio.sceneOf(in)
			if err != nil {
				return nil, err
			}
			board, err := h.Execute(ctx, actor, CreateWhiteboardCommand{ID: id, HubID: hub, Title: in.String("title"), Scene: scene})
			if err != nil {
				return nil, err
			}
			out := BoardOutput(board, len(board.Scene.Elements), true)
			out["writable"] = true
			return out, nil
		}),
	})
}

// ReadWhiteboard reads one board with its objects and whether the reader may draw on it.
type ReadWhiteboard struct{ Studio Studio }

// Execute reads the board.
func (h ReadWhiteboard) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Board, bool, error) {
	board, err := h.Studio.readable(ctx, actor, id, BoardReadAction)
	if err != nil {
		return domain.Board{}, false, err
	}
	writable, err := h.Studio.mayWrite(ctx, actor, board.HubID)
	return board, writable, err
}

// Descriptor is the catalogue entry.
func (h ReadWhiteboard) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadWhiteboardName, Permission: service.PermissionRead,
		Summary: "Reads one whiteboard with its objects in drawing order and whether the reader may draw " +
			"on it. A card names its entry only; what the card shows is resolved for each reader " +
			"(ResolveReferences). A board in the trash is not found.",
		SideEffects: "None. Reads only.",
		TokenScope:  boardsRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."}},
		Audit:       usecase.AuditDeclaration{Action: BoardReadAction, TargetType: boardTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			board, writable, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			out := BoardOutput(board, len(board.Scene.Elements), true)
			out["writable"] = writable
			return out, nil
		}),
	})
}

// ListWhiteboards reads a hub's boards without their objects, the last changed first.
type ListWhiteboards struct{ Studio Studio }

// Execute reads the list.
func (h ListWhiteboards) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, trashed bool) ([]usecase.Output, error) {
	if err := actor.RequireScope(boardsRead); err != nil {
		return nil, err
	}
	if err := h.Studio.authorizeRead(ctx, actor, hubID, hubID, BoardReadAction); err != nil {
		return nil, err
	}
	out := []usecase.Output{}
	err := h.Studio.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Studio.hub(ctx, hubID); err != nil {
			return err
		}
		boards, err := h.Studio.Boards.List(ctx, hubID, trashed, domain.MaxBoardsPerHub)
		for _, summary := range boards {
			out = append(out, BoardOutput(summary.Board, summary.Elements, false))
		}
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListWhiteboards) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListWhiteboardsName, Permission: service.PermissionRead,
		Summary: "Lists a hub's whiteboards without their objects, the last changed first, with how " +
			"many objects each holds; with trashed, the boards in the hub's trash instead.",
		SideEffects: "None. Reads only.",
		TokenScope:  boardsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "trashed", Kind: usecase.KindBool, Description: "List the boards in the trash instead."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardReadAction, TargetType: boardTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			boards, err := h.Execute(ctx, actor, hub, in.Bool("trashed"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{"data": boards}, nil
		}),
	})
}

// UpdateWhiteboardCommand is the input, typed.
type UpdateWhiteboardCommand struct {
	ID              shared.ID
	Change          domain.Change
	ExpectedVersion int
}

// UpdateWhiteboard renames a board or saves its objects - the scene the live document's editors
// save beside it (UC-CNV-01, UC-CNV-02).
type UpdateWhiteboard struct{ Studio Studio }

// Execute writes the board.
func (h UpdateWhiteboard) Execute(ctx context.Context, actor appshared.ActorContext, cmd UpdateWhiteboardCommand) (domain.Board, error) {
	place, err := h.Studio.placeOf(ctx, actor, cmd.ID, false)
	if err != nil {
		return domain.Board{}, err
	}
	if err := h.Studio.authorizeWrite(ctx, actor, place.HubID, place.ID, BoardChangedAction); err != nil {
		return domain.Board{}, err
	}
	var board domain.Board
	err = h.Studio.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Studio.find(ctx, cmd.ID, true, false)
		if err != nil {
			return err
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("whiteboards.version_conflict")
		}
		next, fields, err := current.Changed(cmd.Change, actor.AccountID, h.Studio.Clock.Now(), h.Studio.Text)
		if err != nil {
			return err
		}
		if len(fields) == 0 {
			board = current
			return nil
		}
		if err := h.Studio.Boards.Update(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		board = next
		return h.Studio.record(ctx, actor, BoardChangedAction, next, fields)
	})
	return board, err
}

// Descriptor is the catalogue entry.
func (h UpdateWhiteboard) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UpdateWhiteboardName, Permission: service.PermissionWriteItems,
		Summary: "Renames a whiteboard or saves its objects, sent whole in drawing order. Without " +
			"expected_version the write lands on whatever stands - what live drawing saves beside the " +
			"live document.",
		SideEffects: "Writes the board and an audit entry.",
		TokenScope:  boardsWrite,
		Input: []usecase.Field{
			{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."},
			{Name: "title", Kind: usecase.KindString, Description: "The title, one line."},
			elementsField,
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; a board changed since is refused."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardChangedAction, TargetType: boardTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			scene, err := h.Studio.sceneOf(in)
			if err != nil {
				return nil, err
			}
			board, err := h.Execute(ctx, actor, UpdateWhiteboardCommand{
				ID: id, ExpectedVersion: in.Int("expected_version"),
				Change: domain.Change{Title: in.OptionalString("title"), Scene: scene},
			})
			if err != nil {
				return nil, err
			}
			out := BoardOutput(board, len(board.Scene.Elements), true)
			out["writable"] = true
			return out, nil
		}),
	})
}

// TrashWhiteboard puts a board in its hub's trash; RestoreWhiteboard brings it back. Nothing is
// removed: a board leaves the database only with its hub or its workspace.
type TrashWhiteboard struct{ Studio Studio }

// Execute trashes the board.
func (h TrashWhiteboard) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Board, error) {
	return h.Studio.toggle(ctx, actor, id, true)
}

// RestoreWhiteboard brings a trashed board back.
type RestoreWhiteboard struct{ Studio Studio }

// Execute restores the board.
func (h RestoreWhiteboard) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Board, error) {
	return h.Studio.toggle(ctx, actor, id, false)
}

func (s Studio) toggle(ctx context.Context, actor appshared.ActorContext, id shared.ID, trash bool) (domain.Board, error) {
	action := BoardRestoredAction
	if trash {
		action = BoardTrashedAction
	}
	place, err := s.placeOf(ctx, actor, id, !trash)
	if err != nil {
		return domain.Board{}, err
	}
	if err := s.authorizeWrite(ctx, actor, place.HubID, place.ID, action); err != nil {
		return domain.Board{}, err
	}
	var board domain.Board
	err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := s.find(ctx, id, true, !trash)
		if err != nil {
			return err
		}
		if !trash {
			// A board comes back only into a hub that is there to hold it.
			if err := s.hub(ctx, current.HubID); err != nil {
				return err
			}
		}
		now := s.Clock.Now()
		next := current
		next.DeletedAt, next.UpdatedBy, next.UpdatedAt = nil, actor.AccountID, now
		if trash {
			next.DeletedAt = &now
		}
		if err := s.Boards.Update(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		board = next
		return s.record(ctx, actor, action, next, nil)
	})
	return board, err
}

func toggleDescriptor(name string, trash bool, execute func(context.Context, appshared.ActorContext, shared.ID) (domain.Board, error)) usecase.Descriptor {
	summary, effects, action := "Brings a whiteboard back from its hub's trash, its objects as they were.",
		"Marks the board restored and writes an audit entry.", BoardRestoredAction
	if trash {
		summary, effects, action = "Puts a whiteboard in its hub's trash. Nothing is removed: "+
			"RestoreWhiteboard brings it back, and its cards stop counting as boards an entry is drawn on.",
			"Marks the board trashed and writes an audit entry.", BoardTrashedAction
	}
	return descriptor(usecase.Descriptor{
		Name: name, Permission: service.PermissionWriteItems, Summary: summary, SideEffects: effects,
		TokenScope: boardsWrite,
		Input:      []usecase.Field{{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."}},
		Audit:      usecase.AuditDeclaration{Action: action, TargetType: boardTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			board, err := execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return BoardOutput(board, len(board.Scene.Elements), false), nil
		}),
	})
}

// Descriptor is the catalogue entry.
func (h TrashWhiteboard) Descriptor() usecase.Descriptor {
	return toggleDescriptor(TrashWhiteboardName, true, h.Execute)
}

// Descriptor is the catalogue entry.
func (h RestoreWhiteboard) Descriptor() usecase.Descriptor {
	return toggleDescriptor(RestoreWhiteboardName, false, h.Execute)
}

// ExportWhiteboard writes a board, or one frame of it, as the native document CreateWhiteboard
// reads back (UC-CNV-03). The picture formats are drawn by the client, which holds the colours
// (rule 15).
type ExportWhiteboard struct{ Studio Studio }

// Execute exports the board.
func (h ExportWhiteboard) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, frame string) (domain.Board, domain.Scene, error) {
	board, err := h.Studio.readable(ctx, actor, id, BoardExportedAction)
	if err != nil {
		return domain.Board{}, domain.Scene{}, err
	}
	scene := board.Scene
	if frame != "" {
		found, ok := board.Scene.Frame(frame)
		if !ok {
			return domain.Board{}, domain.Scene{}, shared.ErrNotFound.WithDetail("whiteboards.frame_not_found")
		}
		scene = board.Scene.Within(found)
	}
	return board, scene, h.Studio.record(ctx, actor, BoardExportedAction, board, nil)
}

// Descriptor is the catalogue entry.
func (h ExportWhiteboard) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ExportWhiteboardName, Permission: service.PermissionRead,
		Summary: "Exports a whiteboard, or the objects inside one of its frames, as a native document " +
			"(format hubtask.whiteboard, version 1) whose title and elements CreateWhiteboard reads back " +
			"into a new board. A card exports the entry it names, not the entry's words.",
		SideEffects: "Writes an audit entry.",
		TokenScope:  boardsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."},
			{Name: "frame", Kind: usecase.KindString, Description: "The identifier of one frame to export alone."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardExportedAction, TargetType: boardTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			board, scene, err := h.Execute(ctx, actor, id, in.String("frame"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"format": ExportFormat, "version": ExportVersion, "title": board.Title,
				"exported_at": h.Studio.Clock.Now(), "elements": ElementsOutput(scene),
			}, nil
		}),
	})
}
