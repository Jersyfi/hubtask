// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	viewrepo "github.com/Jersyfi/hubtask/core/application/repository/view"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	UpdateSavedViewName = "UpdateSavedView"
	DeleteSavedViewName = "DeleteSavedView"
	ShareSavedViewName  = "ShareSavedView"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	ViewUpdatedAction audit.Action = "view.updated"
	ViewDeletedAction audit.Action = "view.deleted"
	ViewSharedAction  audit.Action = "view.shared"
)

// SavedViewWriter is what the three changes to an existing view need.
//
// One dependency set, for the reason CustomFieldWriter is one: the same find, the same
// visibility, the same ownership question, and only what is written at the end differs.
type SavedViewWriter struct {
	Views      viewrepo.SavedViews
	Containers repository.Containers
	Authorizer Authorizer
	Permits    Permitting
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// UpdateSavedView changes a view's own fields: its name, its layout, its query, its hints.
type UpdateSavedView struct {
	Writer SavedViewWriter
}

// DeleteSavedView removes a view. A calendar feed that served it keeps its token and serves
// nothing, saying why - the reference nulls rather than cascading (migration 0005, D-08).
type DeleteSavedView struct {
	Writer SavedViewWriter
}

// ShareSavedView decides who sees a view: the owner alone, or everyone who may read its scope.
type ShareSavedView struct {
	Writer SavedViewWriter
}

// SavedViewCommand is the shared input of the three: which view, and the version the caller read.
type SavedViewCommand struct {
	ViewID shared.ID
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read
	// none and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute applies the update and returns the view as it now stands.
func (h UpdateSavedView) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SavedViewCommand,
	attributes view.ViewAttributes,
) (view.SavedView, error) {
	updated, err := h.Writer.change(ctx, actor, cmd, ViewUpdatedAction,
		func(ctx context.Context, saved view.SavedView, expected int, now time.Time) (view.SavedView, error) {
			wanted, changed, err := saved.Updated(attributes)
			if err != nil {
				return view.SavedView{}, err
			}
			if !changed {
				// Nothing moved: nothing is written, no version is spent, nothing is recorded.
				// The If-Match was already honoured by the writer's find-and-check.
				return saved, nil
			}
			if err := h.Writer.Views.SetAttributes(ctx, wanted, expected); err != nil {
				return view.SavedView{}, err
			}
			wanted.Version = expected + 1
			if err := recordViewAudit(ctx, h.Writer.Audit, actor, wanted, ViewUpdatedAction, now); err != nil {
				return view.SavedView{}, err
			}
			return wanted, nil
		})
	if err != nil {
		return view.SavedView{}, err
	}
	return updated, nil
}

// Execute removes the view.
func (h DeleteSavedView) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SavedViewCommand,
) error {
	_, err := h.Writer.change(ctx, actor, cmd, ViewDeletedAction,
		func(ctx context.Context, saved view.SavedView, expected int, now time.Time) (view.SavedView, error) {
			if err := h.Writer.Views.Delete(ctx, saved, expected); err != nil {
				return view.SavedView{}, err
			}
			if err := recordViewAudit(ctx, h.Writer.Audit, actor, saved, ViewDeletedAction, now); err != nil {
				return view.SavedView{}, err
			}
			return saved, nil
		})
	return err
}

// Execute decides the sharing and returns the view.
//
// Publishing into the scope asks STRUCTURE on the scope's path, whoever asks - a view over a
// collection cannot be pushed onto people by somebody who may not shape that collection, the
// owner included. Taking a view back to PRIVATE is the owner's right without any permission, and
// a STRUCTURE holder's for a view that was published.
func (h ShareSavedView) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd SavedViewCommand, sharing view.Sharing,
) (view.SavedView, error) {
	return h.Writer.share(ctx, actor, cmd, sharing)
}

// change is the walk the update and the deletion share: find, decide visibility, decide the
// right to change, then hand the row to the one step that differs.
//
// The decision has two answers and the difference between them is T-04. Somebody who cannot see
// the view is told it does not exist, in exactly the words a missing one produces. Somebody who
// sees it and may not change it is refused: the owner changes their own; anyone else needs
// STRUCTURE on the scope's path, because a view shared into a scope is workspace furniture and
// cannot be hostage to one account.
func (w SavedViewWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd SavedViewCommand, action audit.Action,
	apply func(context.Context, view.SavedView, int, time.Time) (view.SavedView, error),
) (view.SavedView, error) {
	saved, err := w.found(ctx, actor, cmd.ViewID)
	if err != nil {
		return view.SavedView{}, err
	}

	if saved.OwnerID != actor.AccountID {
		// Not the owner's: the right comes from the scope, asked with the audited refusal an
		// authorisation question deserves (a visible view refused is a refusal, not a 404).
		path, reachable, err := w.scopePath(ctx, actor, saved)
		if err != nil {
			return view.SavedView{}, err
		}
		if !reachable {
			return view.SavedView{}, viewNotFound(saved.ID)
		}
		if err := w.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionStructure,
			Path:       path,
			Action:     action,
			TokenScope: viewsWrite,
			TargetType: viewTarget,
			TargetID:   saved.ID,
		}); err != nil {
			return view.SavedView{}, err
		}
	} else if err := actor.RequireScope(viewsWrite); err != nil {
		return view.SavedView{}, err
	}

	expected := cmd.ExpectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still the one the write matches on.
		expected = saved.Version
	} else if expected != saved.Version {
		return view.SavedView{}, shared.ErrVersionConflict.
			WithDetail("views.version_conflict").
			WithParams(map[string]string{"view_id": saved.ID.String()})
	}

	var written view.SavedView
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		written, err = apply(ctx, saved, expected, now)
		return err
	})
	if err != nil {
		return view.SavedView{}, err
	}
	return written, nil
}

// share is change with the direction-dependent permission the sharing decision carries.
func (w SavedViewWriter) share(
	ctx context.Context, actor appshared.ActorContext, cmd SavedViewCommand, sharing view.Sharing,
) (view.SavedView, error) {
	saved, err := w.found(ctx, actor, cmd.ViewID)
	if err != nil {
		return view.SavedView{}, err
	}

	needsStructure := sharing == view.SharingScope || saved.OwnerID != actor.AccountID
	if needsStructure {
		path, reachable, err := w.scopePath(ctx, actor, saved)
		if err != nil {
			return view.SavedView{}, err
		}
		if !reachable {
			return view.SavedView{}, viewNotFound(saved.ID)
		}
		if saved.ScopeType == view.ViewScopeAccount {
			// No audience, no path: the domain refuses SCOPE below; a non-owner never sees an
			// account view at all and was answered by found already.
			path = []identity.Scope{identity.TenantScope()}
		}
		if err := w.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionStructure,
			Path:       path,
			Action:     ViewSharedAction,
			TokenScope: viewsWrite,
			TargetType: viewTarget,
			TargetID:   saved.ID,
		}); err != nil {
			return view.SavedView{}, err
		}
	} else if err := actor.RequireScope(viewsWrite); err != nil {
		return view.SavedView{}, err
	}

	wanted, moved, err := saved.Shared(sharing)
	if err != nil {
		return view.SavedView{}, err
	}

	expected := cmd.ExpectedVersion
	if expected == 0 {
		expected = saved.Version
	} else if expected != saved.Version {
		return view.SavedView{}, shared.ErrVersionConflict.
			WithDetail("views.version_conflict").
			WithParams(map[string]string{"view_id": saved.ID.String()})
	}
	if !moved {
		// Already in the state asked for: nothing is written, no version is spent.
		return saved, nil
	}

	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		if err := w.Views.SetSharing(ctx, wanted, expected); err != nil {
			return err
		}
		wanted.Version = expected + 1
		return recordViewAudit(ctx, w.Audit, actor, wanted, ViewSharedAction, now)
	})
	if err != nil {
		return view.SavedView{}, err
	}
	return wanted, nil
}

// found reads the view and answers not-found for what the actor cannot see (T-04).
func (w SavedViewWriter) found(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (view.SavedView, error) {
	if id.IsZero() {
		return view.SavedView{}, shared.ErrValidation.
			WithDetail("views.view_id_required").
			WithFields(shared.FieldError{Path: "/view_id", Code: "views.view_id_required"})
	}

	var saved view.SavedView
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Views.Find(ctx, id)
		saved = found
		return err
	})
	if err != nil {
		return view.SavedView{}, err
	}

	visible, err := viewVisibleTo(ctx, w.UnitOfWork, w.Containers, w.Permits, actor, saved)
	if err != nil {
		return view.SavedView{}, err
	}
	if !visible {
		return view.SavedView{}, viewNotFound(id)
	}
	return saved, nil
}

// scopePath answers the path of the view's scope, and whether it still resolves.
func (w SavedViewWriter) scopePath(
	ctx context.Context, actor appshared.ActorContext, saved view.SavedView,
) ([]identity.Scope, bool, error) {
	switch saved.ScopeType {
	case view.ViewScopeTenant:
		return []identity.Scope{identity.TenantScope()}, true, nil
	case view.ViewScopeHub, view.ViewScopeCollection:
		var path []identity.Scope
		found := true
		err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			container, err := w.Containers.Find(ctx, saved.ScopeID)
			if err != nil {
				if shared.AsError(err) != nil && shared.AsError(err).Category == shared.CategoryNotFound {
					found = false
					return nil
				}
				return err
			}
			path = containerPath(container)
			return nil
		})
		return path, found, err
	default:
		return []identity.Scope{identity.TenantScope()}, true, nil
	}
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h UpdateSavedView) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateSavedViewName,
		Summary: "Changes a view's own fields: its name, its layout, its query, its grouping and " +
			"its visible fields. A field that is not sent is left alone; the query and the hints " +
			"replace whole - a query is one statement, and merging two half-queries would " +
			"produce one nobody wrote. The scope and the sharing are not here: where a view " +
			"lives is decided at creation, and sharing is shareSavedView. The owner edits their " +
			"own; anyone else needs STRUCTURE on the view's scope.",
		SideEffects: "Writes the changed fields and an audit entry. Announces nothing: a view is " +
			"configuration, and no event in the catalogue is about one.",
		TokenScope: viewsWrite,
		Input: append([]usecase.Field{
			{
				Name: "name", Kind: usecase.KindString,
				Description: "The new name: one line, at most 200 characters. Omitted leaves it.",
			},
			{
				Name: "layout", Kind: usecase.KindString,
				Description: "The new layout, from the declared set. Omitted leaves it.",
			},
			{
				Name: "query", Kind: usecase.KindObject,
				Description: "The new query document, replacing the stored one whole, validated " +
					"like an ad-hoc query. Omitted leaves it.",
			},
			{
				Name: "grouping", Kind: usecase.KindObject,
				Description: "The new grouping hint, replacing the stored one whole. Omitted leaves it.",
			},
			{
				Name: "visible_fields", Kind: usecase.KindList,
				Description: "The new column list, replacing the stored one whole. Omitted leaves it.",
			},
		}, savedViewInput("The view to change.")...),
		Audit: usecase.AuditDeclaration{
			Action: ViewUpdatedAction, TargetType: viewTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A view is a bookmark over the work rather than a piece of it, and the item " +
				"history is keyed on an entry. Editing a query changes no entry anybody reads a " +
				"history of.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h DeleteSavedView) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteSavedViewName,
		Summary: "Deletes the view. A calendar feed that served it keeps its token and serves " +
			"nothing, saying why - the feed's view reference nulls rather than cascading, " +
			"because a feed is the subscriber's and a view is the workspace's. The owner deletes " +
			"their own; anyone else needs STRUCTURE on the view's scope.",
		SideEffects: "Removes the view and writes an audit entry. Announces nothing: a view is " +
			"configuration, and no event in the catalogue is about one.",
		TokenScope:  viewsWrite,
		Destructive: true,
		Input:       savedViewInput("The view to delete."),
		Audit: usecase.AuditDeclaration{
			Action: ViewDeletedAction, TargetType: viewTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A view is a bookmark over the work rather than a piece of it, and the item " +
				"history is keyed on an entry. Deleting a bookmark changes no entry anybody " +
				"reads a history of.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h ShareSavedView) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ShareSavedViewName,
		Summary: "Decides who sees the view: PRIVATE keeps it the owner's, SCOPE shares it with " +
			"everyone who may read the view's scope. Publishing asks STRUCTURE on the scope's " +
			"path, whoever asks - a view over a collection cannot be pushed onto people by " +
			"somebody who may not shape that collection. An ACCOUNT-scoped view names no " +
			"audience and cannot be shared. PUBLIC_LINK is declared and refused by name. " +
			"Idempotent: asking for the sharing a view already has succeeds and writes nothing.",
		SideEffects: "Writes the sharing and an audit entry. Announces nothing: a view is " +
			"configuration, and no event in the catalogue is about one.",
		TokenScope: viewsWrite,
		Input: append([]usecase.Field{
			{
				Name: "sharing", Kind: usecase.KindString, Required: true,
				Enum:        []string{"PRIVATE", "SCOPE"},
				Description: "Who sees the view.",
			},
		}, savedViewInput("The view whose sharing is decided.")...),
		Audit: usecase.AuditDeclaration{
			Action: ViewSharedAction, TargetType: viewTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A view is a bookmark over the work rather than a piece of it, and the item " +
				"history is keyed on an entry. Sharing a bookmark changes no entry anybody " +
				"reads a history of.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// savedViewInput is what all three changes take beyond their own fields. One list, so that a
// client which learned one of them from /meta/capabilities does not find the others spelled
// differently.
func savedViewInput(viewDescription string) []usecase.Field {
	return []usecase.Field{
		{Name: "view_id", Kind: usecase.KindID, Required: true, Description: viewDescription},
		{
			Name: "expected_version", Kind: usecase.KindInt,
			Description: "The version last read, from the If-Match header over REST. Omitted " +
				"means the caller read none and accepts whatever is there; a version that has " +
				"moved on since is refused rather than overwritten.",
		},
	}
}

func (h UpdateSavedView) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := savedViewCommand(in)
	if err != nil {
		return nil, err
	}

	attributes := view.ViewAttributes{
		Name:   in.OptionalString("name"),
		Layout: in.OptionalString("layout"),
	}
	attributes.Query, _ = in["query"].(map[string]any)
	attributes.Grouping, _ = in["grouping"].(map[string]any)
	if raw, sent := in["visible_fields"].([]any); sent {
		fields := make([]string, 0, len(raw))
		for _, entry := range raw {
			text, _ := entry.(string)
			fields = append(fields, text)
		}
		attributes.VisibleFields = fields
	}
	if attributes.IsEmpty() {
		return nil, shared.ErrValidation.
			WithDetail("views.update_empty").
			WithFields(shared.FieldError{Path: "/", Code: "views.update_empty"})
	}

	updated, err := h.Execute(ctx, actor, cmd, attributes)
	if err != nil {
		return nil, err
	}
	return viewOutput(updated), nil
}

func (h DeleteSavedView) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := savedViewCommand(in)
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, cmd); err != nil {
		return nil, err
	}
	return usecase.Output{"deleted": true}, nil
}

func (h ShareSavedView) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := savedViewCommand(in)
	if err != nil {
		return nil, err
	}
	sharing, err := view.NewSharing(in.String("sharing"))
	if err != nil {
		return nil, err
	}

	updated, err := h.Execute(ctx, actor, cmd, sharing)
	if err != nil {
		return nil, err
	}
	return viewOutput(updated), nil
}

// savedViewCommand is the adapter between the catalogue's untyped input and the typed command,
// for all three changes and all three channels.
func savedViewCommand(in usecase.Input) (SavedViewCommand, error) {
	id, err := in.ID("view_id")
	if err != nil {
		return SavedViewCommand{}, err
	}
	return SavedViewCommand{ViewID: id, ExpectedVersion: in.Int("expected_version")}, nil
}
