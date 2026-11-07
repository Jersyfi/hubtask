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
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The history's use case names (UC-CNV-06).
const (
	ListWhiteboardVersionsName = "ListWhiteboardVersions"
	ReadWhiteboardVersionName  = "ReadWhiteboardVersion"
	NameWhiteboardVersionName  = "NameWhiteboardVersion"
)

// BoardVersionNamedAction is the audit code of a version named or its name taken away.
const BoardVersionNamedAction audit.Action = "whiteboard.version_named"

const maxVersionsPage = 100

// snapshot leaves the version a save of the board's scene or title behind, folding it into the
// last one in the same drawing session (domain.Snapshot).
func (s Studio) snapshot(ctx context.Context, board domain.Board, author shared.ID) error {
	last, found, err := s.Boards.LastVersion(ctx, board.ID)
	if err != nil {
		return err
	}
	var previous *domain.Version
	if found {
		previous = &last
	}
	version, fold := domain.Snapshot(previous, board, author, s.Clock.Now())
	if fold {
		return s.Boards.FoldVersion(ctx, version)
	}
	return s.Boards.InsertVersion(ctx, version)
}

// VersionOutput is a version as a history lists it (schema WhiteboardVersion); its objects only
// when it is read on its own.
func VersionOutput(seq int, title string, author shared.ID, name string, at any, elements int) usecase.Output {
	out := usecase.Output{"seq": seq, "title": title, "author_id": nil, "name": nilIfEmpty(name), "created_at": at, "element_count": elements}
	if !author.IsZero() {
		out["author_id"] = author.String()
	}
	return out
}

// ListWhiteboardVersions reads a board's history, newest first (UC-CNV-06).
type ListWhiteboardVersions struct{ Studio Studio }

// Execute reads the history.
func (h ListWhiteboardVersions) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, before, limit int) ([]usecase.Output, error) {
	board, err := h.Studio.readable(ctx, actor, id, BoardReadAction)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxVersionsPage {
		limit = maxVersionsPage
	}
	out := []usecase.Output{}
	err = h.Studio.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rows, err := h.Studio.Boards.Versions(ctx, board.ID, before, limit)
		for _, row := range rows {
			out = append(out, VersionOutput(row.Seq, row.Title, row.AuthorID, row.Name, row.CreatedAt, row.Elements))
		}
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListWhiteboardVersions) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListWhiteboardVersionsName, Permission: service.PermissionRead,
		Summary: "Lists a whiteboard's history newest first: one version per drawing session and every " +
			"named one, each with its title, who saved it last, its name and how many objects it held.",
		SideEffects: "None. Reads only.",
		TokenScope:  boardsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."},
			{Name: "before", Kind: usecase.KindInt, Description: "Only the versions older than this one; the newest without."},
			{Name: "limit", Kind: usecase.KindInt, Description: "At most this many, 100 at most."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardReadAction, TargetType: boardTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			data, err := h.Execute(ctx, actor, id, in.Int("before"), in.Int("limit"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{"data": data}, nil
		}),
	})
}

// ReadWhiteboardVersion reads one version with its objects: what is compared and brought back.
// Bringing it back is the client's: it writes the objects into the live document, and the save
// that follows is a version of its own, so nothing in between is lost (P-03).
type ReadWhiteboardVersion struct{ Studio Studio }

// Execute reads the version.
func (h ReadWhiteboardVersion) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, seq int) (domain.Version, error) {
	board, err := h.Studio.readable(ctx, actor, id, BoardReadAction)
	if err != nil {
		return domain.Version{}, err
	}
	var version domain.Version
	err = h.Studio.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		version, err = h.Studio.Boards.Version(ctx, board.ID, seq)
		return err
	})
	return version, err
}

// Descriptor is the catalogue entry.
func (h ReadWhiteboardVersion) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadWhiteboardVersionName, Permission: service.PermissionRead,
		Summary:     "Reads one version of a whiteboard with its objects, to compare it or bring it back.",
		SideEffects: "None. Reads only.",
		TokenScope:  boardsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."},
			{Name: "seq", Kind: usecase.KindInt, Required: true, Description: "The version's place in the history."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardReadAction, TargetType: boardTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			version, err := h.Execute(ctx, actor, id, in.Int("seq"))
			if err != nil {
				return nil, err
			}
			out := VersionOutput(version.Seq, version.Title, version.AuthorID, version.Name, version.CreatedAt, len(version.Scene.Elements))
			out["elements"] = ElementsOutput(version.Scene)
			return out, nil
		}),
	})
}

// NameWhiteboardVersion names a version ("Before the workshop"), or takes its name away. A named
// version is never folded into by a later save of the same session.
type NameWhiteboardVersion struct{ Studio Studio }

// Execute names the version.
func (h NameWhiteboardVersion) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, seq int, raw string) (string, error) {
	place, err := h.Studio.placeOf(ctx, actor, id, false)
	if err != nil {
		return "", err
	}
	if err := h.Studio.authorizeWrite(ctx, actor, place.HubID, place.ID, BoardVersionNamedAction); err != nil {
		return "", err
	}
	name, err := domain.CheckVersionName(raw, h.Studio.Text)
	if err != nil {
		return "", err
	}
	return name, h.Studio.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Studio.Boards.NameVersion(ctx, place.ID, seq, name); err != nil {
			return err
		}
		return h.Studio.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: h.Studio.Clock.Now(), Action: BoardVersionNamedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: boardTarget, TargetID: place.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "seq", Classification: audit.Open, To: seq},
				audit.Change{Field: "name", Classification: audit.Sensitive, To: name},
			),
		})
	})
}

// Descriptor is the catalogue entry.
func (h NameWhiteboardVersion) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: NameWhiteboardVersionName, Permission: service.PermissionWriteItems,
		Summary: "Names a version of a whiteboard, or takes its name away with an empty name. A named " +
			"version stays as it is when the drawing goes on.",
		SideEffects: "Writes the version's name and an audit entry.",
		TokenScope:  boardsWrite,
		Input: []usecase.Field{
			{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board."},
			{Name: "seq", Kind: usecase.KindInt, Required: true, Description: "The version's place in the history."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "The name, one line of at most 120 characters; empty takes it away."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardVersionNamedAction, TargetType: boardTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("whiteboard_id")
			if err != nil {
				return nil, err
			}
			seq := in.Int("seq")
			name, err := h.Execute(ctx, actor, id, seq, in.String("name"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{"seq": seq, "name": nilIfEmpty(name)}, nil
		}),
	})
}
