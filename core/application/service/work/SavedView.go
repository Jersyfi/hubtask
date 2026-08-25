// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	viewrepo "github.com/Jersyfi/hubtask/core/application/repository/view"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateSavedViewName = "CreateSavedView"
	ListSavedViewsName  = "ListSavedViews"
	GetSavedViewName    = "GetSavedView"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	ViewCreatedAction audit.Action = "view.created"
	ViewReadAction    audit.Action = "view.read"

	viewTarget = "saved_view"
	// The token scopes alias the container pair, the precedent buckets and custom fields set: a
	// view is workspace furniture, and a scope the token issuer does not mint would be one no
	// client could ever hold.
	viewsWrite = containersWrite
	viewsRead  = containersRead
)

// Permitting is the slice of the authorisation service the view reads need beyond Authorizer: the
// same question, answered without an audited refusal - a view the actor may not see is not found,
// and a DENIED entry for every invisible bookmark would make the trail unreadable (T-04).
type Permitting interface {
	Permits(ctx context.Context, actor appshared.ActorContext, request access.Request) (bool, error)
}

// CreateSavedView saves a view: a query in the DSL, validated here against the same catalogue an
// ad-hoc one passes, and the layout the client draws it in, stored and never consulted (D-07,
// api-guidelines.md §3).
type CreateSavedView struct {
	Views      viewrepo.SavedViews
	Containers repository.Containers
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// CreateSavedViewCommand is the input, typed.
type CreateSavedViewCommand struct {
	ScopeType view.ViewScope
	ScopeID   shared.ID
	Name      string
	Layout    string
	Query     map[string]any
	Grouping  map[string]any
	// VisibleFields distinguishes nil - not sent, stored empty - from an empty list a client
	// chose, though the two store alike.
	VisibleFields []string
	Sharing       view.Sharing
}

// Execute creates the view and returns it.
//
// The permission is the scope's, and it depends on what is asked. A view is a bookmark, so
// creating one needs READ on its scope's path - anyone who may read a collection may save a query
// on it. Creating it already shared publishes workspace furniture, which is STRUCTURE, exactly as
// :share asks. An ACCOUNT-scoped view is self-service, the way changing one's own preferences is:
// no matrix cell describes saving a private bookmark, and requiring one would mean a viewer could
// not keep their own (identity.UpdateAccountPreferences is the precedent).
func (h CreateSavedView) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateSavedViewCommand,
) (view.SavedView, error) {
	scopeID, path, err := h.resolveScope(ctx, actor, cmd.ScopeType, cmd.ScopeID)
	if err != nil {
		return view.SavedView{}, err
	}

	if cmd.ScopeType == view.ViewScopeAccount {
		if err := actor.RequireScope(viewsWrite); err != nil {
			return view.SavedView{}, err
		}
	} else {
		permission := service.PermissionRead
		if cmd.Sharing == view.SharingScope {
			permission = service.PermissionStructure
		}
		// Before the transaction, deliberately: a refusal writes an audit entry, and an entry
		// written inside this transaction would be rolled back with the refusal (audit.md §7).
		if err := h.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: permission,
			Path:       path,
			Action:     ViewCreatedAction,
			TokenScope: viewsWrite,
			TargetType: viewTarget,
			TargetID:   scopeID,
		}); err != nil {
			return view.SavedView{}, err
		}
	}

	var created view.SavedView
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Clock.Now()

		built, err := view.NewSavedView(view.NewSavedViewInput{
			ID: h.IDs.NewID(), TenantID: actor.TenantID,
			ScopeType: cmd.ScopeType, ScopeID: scopeID, OwnerID: actor.AccountID,
			Name: cmd.Name, Layout: cmd.Layout, Query: cmd.Query, Grouping: cmd.Grouping,
			VisibleFields: cmd.VisibleFields, Sharing: cmd.Sharing,
			Now: now,
		})
		if err != nil {
			return err
		}
		if built, _, err = built.Shared(built.Sharing); err != nil {
			// The one rule sharing adds at creation: an ACCOUNT scope names no audience.
			return err
		}
		if err := h.Views.Insert(ctx, built); err != nil {
			return err
		}
		if err := recordViewAudit(ctx, h.Audit, actor, built, ViewCreatedAction, now); err != nil {
			return err
		}
		created = built
		return nil
	})
	if err != nil {
		return view.SavedView{}, err
	}
	return created, nil
}

// resolveScope normalises the scope and answers the path the permission question is about.
//
// The container is read before the question, because the answer depends on the path - a
// membership held at the hub applies downwards (domain-model.md §3.2) - and its type has to be
// what the scope claims: a COLLECTION view over a hub would be a lie every reader inherits.
func (h CreateSavedView) resolveScope(
	ctx context.Context, actor appshared.ActorContext, scopeType view.ViewScope, scopeID shared.ID,
) (shared.ID, []identity.Scope, error) {
	switch scopeType {
	case view.ViewScopeAccount:
		if !scopeID.IsZero() && scopeID != actor.AccountID {
			// A personal view is always the caller's own; naming somebody else is refused rather
			// than quietly corrected.
			return noID, nil, shared.ErrValidation.
				WithDetail("views.scope_id_not_allowed").
				WithFields(shared.FieldError{Path: "/scope_id", Code: "views.scope_id_not_allowed"})
		}
		return actor.AccountID, nil, nil
	case view.ViewScopeTenant:
		return noID, []identity.Scope{identity.TenantScope()}, nil
	case view.ViewScopeHub, view.ViewScopeCollection:
		container, err := findViewScopeContainer(ctx, h.UnitOfWork, h.Containers, actor, scopeType, scopeID)
		if err != nil {
			return noID, nil, err
		}
		return container.ID, containerPath(container), nil
	default:
		// The constructor refuses the unknown spelling with the field error; this only keeps the
		// switch total.
		return scopeID, nil, nil
	}
}

// findViewScopeContainer reads the scope's container and holds it to the claimed type.
func findViewScopeContainer(
	ctx context.Context, uow persistence.UnitOfWork, containers repository.Containers,
	actor appshared.ActorContext, scopeType view.ViewScope, scopeID shared.ID,
) (domain.Container, error) {
	if scopeID.IsZero() {
		return domain.Container{}, shared.ErrValidation.
			WithDetail("views.scope_id_required").
			WithFields(shared.FieldError{Path: "/scope_id", Code: "views.scope_id_required"})
	}

	var container domain.Container
	err := uow.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := containers.Find(ctx, scopeID)
		container = found
		return err
	})
	if err != nil {
		return domain.Container{}, err
	}

	want := domain.ContainerCollection
	if scopeType == view.ViewScopeHub {
		want = domain.ContainerHub
	}
	if container.Type != want {
		return domain.Container{}, shared.ErrValidation.
			WithDetail("views.scope_container_mismatched").
			WithParams(map[string]string{"scope_type": string(scopeType)}).
			WithFields(shared.FieldError{
				Path: "/scope_id", Code: "views.scope_container_mismatched",
				Params: map[string]string{"scope_type": string(scopeType)},
			})
	}
	return container, nil
}

// ListSavedViews answers the views the caller may see.
type ListSavedViews struct {
	Views      viewrepo.SavedViews
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// ListSavedViewsQuery is the input, typed.
type ListSavedViewsQuery struct {
	// ContainerID widens the answer to what is shared along that container's path. Zero answers
	// the caller's own views alone.
	ContainerID shared.ID
}

// Execute returns the views.
//
// Unpaged, deliberately, the way the custom field list is: a shelf of bookmarks is small and
// bounded by what a person saves, and a cursor over a list nobody scrolls would be machinery for
// its own sake. What is shared along the path is computed in the query, never filtered after a
// page (C-04's rule).
func (h ListSavedViews) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListSavedViewsQuery,
) ([]view.SavedView, error) {
	if query.ContainerID.IsZero() {
		// The caller's own shelf: self-service, like reading one's own preferences. The token
		// scope is still the door.
		if err := actor.RequireScope(viewsRead); err != nil {
			return nil, err
		}
		var views []view.SavedView
		err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			views, err = h.Views.ListOwned(ctx, actor.AccountID)
			return err
		})
		return views, err
	}

	var container domain.Container
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Containers.Find(ctx, query.ContainerID)
		container = found
		return err
	})
	if err != nil {
		return nil, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(container),
		Action:     ViewReadAction,
		TokenScope: viewsRead,
		TargetType: viewTarget,
		TargetID:   container.ID,
	}); err != nil {
		return nil, err
	}

	// The scopes that container's path names: itself and, for a collection, the hub above it.
	// TENANT-wide shares match by their type alone and need no identifier here.
	scopes := []shared.ID{container.ID}
	if !container.ParentID.IsZero() {
		scopes = append(scopes, container.ParentID)
	}

	var views []view.SavedView
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		views, err = h.Views.ListReachable(ctx, actor.AccountID, scopes)
		return err
	})
	return views, err
}

// GetSavedView answers one view.
type GetSavedView struct {
	Views      viewrepo.SavedViews
	Containers repository.Containers
	Permits    Permitting
	UnitOfWork persistence.UnitOfWork
}

// Execute returns the view, or that there is none.
//
// A view the caller may not see answers exactly what a missing one answers (T-04): another
// account's private bookmark, and a shared view over a scope the caller holds nothing on, are
// both worlds this caller cannot distinguish from empty ones.
func (h GetSavedView) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (view.SavedView, error) {
	if err := actor.RequireScope(viewsRead); err != nil {
		return view.SavedView{}, err
	}
	if id.IsZero() {
		return view.SavedView{}, shared.ErrValidation.
			WithDetail("views.view_id_required").
			WithFields(shared.FieldError{Path: "/view_id", Code: "views.view_id_required"})
	}

	var found view.SavedView
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		saved, err := h.Views.Find(ctx, id)
		found = saved
		return err
	})
	if err != nil {
		return view.SavedView{}, err
	}

	visible, err := viewVisibleTo(ctx, h.UnitOfWork, h.Containers, h.Permits, actor, found)
	if err != nil {
		return view.SavedView{}, err
	}
	if !visible {
		return view.SavedView{}, viewNotFound(id)
	}
	return found, nil
}

// viewNotFound is the one answer both an absent view and an invisible one produce, built in one
// place so the two cannot drift apart (T-04).
func viewNotFound(id shared.ID) error {
	return shared.ErrNotFound.
		WithDetail("views.not_found").
		WithParams(map[string]string{"view_id": id.String()})
}

// viewVisibleTo decides whether the actor sees the view: the owner always, everyone else exactly
// when it is shared into a scope they may read. Asked without an audited refusal - the caller is
// told not-found, and the trail is not filled with denials nobody acted on.
func viewVisibleTo(
	ctx context.Context, uow persistence.UnitOfWork, containers repository.Containers,
	permits Permitting, actor appshared.ActorContext, saved view.SavedView,
) (bool, error) {
	if saved.OwnerID == actor.AccountID {
		return true, nil
	}
	if saved.Sharing != view.SharingScope {
		return false, nil
	}

	var path []identity.Scope
	switch saved.ScopeType {
	case view.ViewScopeTenant:
		path = []identity.Scope{identity.TenantScope()}
	case view.ViewScopeHub, view.ViewScopeCollection:
		var container domain.Container
		err := uow.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			found, err := containers.Find(ctx, saved.ScopeID)
			container = found
			return err
		})
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// The scope's container is gone; a share into nowhere reaches nobody.
				return false, nil
			}
			return false, err
		}
		path = containerPath(container)
	default:
		// An ACCOUNT-scoped view is never shared; the constructor refuses the combination.
		return false, nil
	}

	return permits.Permits(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       path,
		Action:     ViewReadAction,
		TokenScope: viewsRead,
		TargetType: viewTarget,
		TargetID:   saved.ID,
	})
}

// recordViewAudit writes the evidence. The name is the owner's free text and is classified
// SENSITIVE - hashed, not kept in clear - while the scope and the sharing are the workspace's
// shape and stay readable (audit.md §4, rule 10).
func recordViewAudit(
	ctx context.Context, sink audit.Sink, actor appshared.ActorContext, saved view.SavedView,
	action audit.Action, now time.Time,
) error {
	return sink.Append(ctx, audit.Entry{
		TenantID:   saved.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: viewTarget,
		TargetID:   saved.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "name", Classification: audit.Sensitive, To: saved.Name},
			audit.Change{Field: "scope_type", Classification: audit.Open, To: string(saved.ScopeType)},
			audit.Change{Field: "scope_id", Classification: audit.Open, To: idOrEmptyText(saved.ScopeID)},
			audit.Change{Field: "sharing", Classification: audit.Open, To: string(saved.Sharing)},
			audit.Change{Field: "layout", Classification: audit.Open, To: string(saved.Layout)},
		),
	})
}

func idOrEmptyText(id shared.ID) string {
	if id.IsZero() {
		return ""
	}
	return id.String()
}

// viewOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema SavedView).
func viewOutput(saved view.SavedView) usecase.Output {
	out := usecase.Output{
		"id":         saved.ID.String(),
		"scope_type": string(saved.ScopeType),
		"scope_id":   nil,
		"owner_id":   saved.OwnerID.String(),
		"name":       saved.Name,
		"layout":     string(saved.Layout),
		"query":      saved.Query,
		"grouping":   saved.Grouping,
		// Always present, empty for a view that chose nothing: a client renders its columns from
		// here, and null would make it special-case the ordinary case.
		"visible_fields": saved.VisibleFields,
		"sharing":        string(saved.Sharing),
		"created_at":     saved.CreatedAt,
		"version":        saved.Version,
	}
	if !saved.ScopeID.IsZero() {
		out["scope_id"] = saved.ScopeID.String()
	}
	return out
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h CreateSavedView) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateSavedViewName,
		Summary: "Saves a view: a query in the DSL of queryItems, the layout it is drawn in, and " +
			"the fields the client shows. The query is validated here against the same catalogue " +
			"and bounds as an ad-hoc one, with the same codes. The layout is one of the declared " +
			"set (view_layouts in the capability manifest), stored and echoed and never " +
			"consulted. The creator owns the view; it starts PRIVATE unless sharing says " +
			"otherwise, under the same rules as :share. PUBLIC_LINK is refused by name.",
		SideEffects: "Writes the view and an audit entry. Announces nothing: a view is " +
			"configuration, and no event in the catalogue is about one.",
		TokenScope: viewsWrite,
		Input: []usecase.Field{
			{
				Name: "scope_type", Kind: usecase.KindString, Required: true,
				Enum: []string{"TENANT", "HUB", "COLLECTION", "ACCOUNT"},
				Description: "Where the view lives, which is who it can be shared with: a " +
					"container's members, the workspace, or - for ACCOUNT - nobody but its owner.",
			},
			{
				Name: "scope_id", Kind: usecase.KindID,
				Description: "The container for HUB and COLLECTION. Omitted for TENANT, and for " +
					"ACCOUNT - a personal view is always the caller's own.",
			},
			{
				Name: "name", Kind: usecase.KindString, Required: true,
				Description: "The view's name: one line, at most 200 characters.",
			},
			{
				Name: "layout", Kind: usecase.KindString, Required: true,
				Description: "How a client draws the view, from the declared set the capability " +
					"manifest publishes as view_layouts. Stored and never consulted.",
			},
			{
				Name: "query", Kind: usecase.KindObject, Required: true,
				Description: "The query document of queryItems, stored as sent. Validated " +
					"against the same catalogue, depth and node bounds as an ad-hoc query.",
			},
			{
				Name: "grouping", Kind: usecase.KindObject,
				Description: "The client's grouping hint, stored whole. A grouped field is held " +
					"to the catalogue; everything else is the client's.",
			},
			{
				Name: "visible_fields", Kind: usecase.KindList,
				Description: "The columns the client shows, at most 50 short names. Stored and " +
					"never resolved: a custom field's key is a legitimate spelling this " +
					"catalogue cannot enumerate.",
			},
			{
				Name: "sharing", Kind: usecase.KindString,
				Enum: []string{"PRIVATE", "SCOPE"},
				Description: "Who sees the view. Omitted is PRIVATE. SCOPE at creation asks the " +
					"same STRUCTURE permission :share does.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ViewCreatedAction, TargetType: viewTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A view is a bookmark over the work rather than a piece of it, and the item " +
				"history is keyed on an entry. Saving a query changes no entry anybody reads a " +
				"history of.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateSavedView) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	scopeID, err := in.ID("scope_id")
	if err != nil {
		return nil, err
	}
	sharing := view.SharingPrivate
	if raw := in.String("sharing"); raw != "" {
		if sharing, err = view.NewSharing(raw); err != nil {
			return nil, err
		}
	}

	cmd := CreateSavedViewCommand{
		ScopeType: view.ViewScope(in.String("scope_type")),
		ScopeID:   scopeID,
		Name:      in.String("name"),
		Layout:    in.String("layout"),
		Sharing:   sharing,
	}
	cmd.Query, _ = in["query"].(map[string]any)
	cmd.Grouping, _ = in["grouping"].(map[string]any)
	if raw, sent := in["visible_fields"].([]any); sent {
		fields := make([]string, 0, len(raw))
		for _, entry := range raw {
			text, _ := entry.(string)
			fields = append(fields, text)
		}
		cmd.VisibleFields = fields
	}

	created, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return viewOutput(created), nil
}

// Descriptor is the catalogue entry.
func (h ListSavedViews) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListSavedViewsName,
		Summary: "The saved views the caller may see. Naming a container answers the caller's " +
			"own views plus what is shared into that container's path - the collection's, its " +
			"hub's and the workspace-wide ones. Naming none answers the caller's own alone.",
		SideEffects: "None. Reads only.",
		TokenScope:  viewsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID,
				Description: "The container whose shared views are wanted, together with the " +
					"caller's own. Omitted answers the caller's own alone.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ViewReadAction, TargetType: viewTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListSavedViews) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}

	views, err := h.Execute(ctx, actor, ListSavedViewsQuery{ContainerID: containerID})
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(views))
	for _, saved := range views {
		rows = append(rows, viewOutput(saved))
	}
	// A bare list rather than a page, like the custom field vocabulary and for the same reason.
	return usecase.Output{"data": rows}, nil
}

// Descriptor is the catalogue entry.
func (h GetSavedView) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetSavedViewName,
		Summary: "One saved view. A view the caller may not see - another account's private " +
			"view, or one shared into a scope the caller holds nothing on - is not found, in " +
			"exactly the words a genuinely missing one produces.",
		SideEffects: "None. Reads only.",
		TokenScope:  viewsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "view_id", Kind: usecase.KindID, Required: true,
				Description: "The view to read.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ViewReadAction, TargetType: viewTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetSavedView) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("view_id")
	if err != nil {
		return nil, err
	}

	found, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return viewOutput(found), nil
}
