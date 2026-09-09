// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ReadWorkspaceName   = "ReadWorkspace"
	UpdateWorkspaceName = "UpdateWorkspace"

	// workspaceRead and workspaceManage are the registry's scopes (api-guidelines.md §7). Two
	// rather than one, because the read is what an auditor and an ordinary member do and the
	// write is what somebody who administers the workspace does - a token that may read how the
	// workspace is set up must not thereby be able to change it.
	workspaceRead   = "workspace:read"
	workspaceManage = "workspace:manage"

	workspaceTarget = "workspace"
)

// The workspace's own configuration, as the trail names it (F4-01).
const (
	WorkspaceReadAction    audit.Action = "workspace.read"
	WorkspaceChangedAction audit.Action = "workspace.changed"
)

// WorkspaceWriter is what the two use cases share.
type WorkspaceWriter struct {
	Workspaces repository.Workspaces
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// ReadWorkspace answers the workspace the caller is in (F4-01).
//
// `READ` or the auditor's read-only configuration permission: how a workspace is set up is
// configuration, and G-12 split `READ_CONFIGURATION` out precisely so that an auditor can read it
// without holding `READ` and without gaining the right to change anything.
type ReadWorkspace struct{ Writer WorkspaceWriter }

// Execute reads it.
func (h ReadWorkspace) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (domain.Workspace, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission:  service.PermissionRead,
		Alternative: service.PermissionReadConfiguration,
		Path:        []domain.Scope{domain.TenantScope()},
		Action:      WorkspaceReadAction,
		TokenScope:  workspaceRead,
		TargetType:  workspaceTarget,
		TargetID:    actor.TenantID,
	}); err != nil {
		return domain.Workspace{}, err
	}

	var found domain.Workspace
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := w.Workspaces.Find(ctx)
		found = read
		return err
	})
	if err != nil {
		return domain.Workspace{}, err
	}
	return found, nil
}

// UpdateWorkspace changes how the workspace is set up.
type UpdateWorkspace struct{ Writer WorkspaceWriter }

// UpdateWorkspaceCommand is the merge-patch, typed. `ExpectedVersion` is what an `If-Match`
// carried, and zero is "the caller named none".
type UpdateWorkspaceCommand struct {
	Change          domain.WorkspaceChange
	ExpectedVersion int
}

// Execute reads, applies and writes inside one transaction.
//
// The read is inside the write's transaction rather than before it, so that the version the guard
// compares is the version the change was computed from. A caller who named one through `If-Match`
// is checked against what is actually stored first, which is what turns a stale form into a
// precondition failure rather than into a silent overwrite (ADR-0025).
//
// A patch that moves nothing writes nothing: no row, no version bump, no audit entry. Answering
// the workspace unchanged is the honest result, and an audit trail with one entry per save of an
// untouched form would be a trail nobody reads.
func (h UpdateWorkspace) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateWorkspaceCommand,
) (domain.Workspace, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     WorkspaceChangedAction,
		TokenScope: workspaceManage,
		TargetType: workspaceTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return domain.Workspace{}, err
	}

	var answer domain.Workspace
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := w.Workspaces.Find(ctx)
		if err != nil {
			return err
		}
		changed, moved, err := stored.With(cmd.Change)
		if err != nil {
			return err
		}
		if len(moved) == 0 {
			answer = stored
			return nil
		}

		// What the caller named through `If-Match` if they named one, and what was just read
		// otherwise - `UpdateWebhookSubscription`'s shape, for its reason.
		expected := stored.Version
		if cmd.ExpectedVersion > 0 {
			expected = cmd.ExpectedVersion
		}

		now := w.Clock.Now()
		written, err := w.Workspaces.Update(ctx, changed, expected, now)
		if err != nil {
			return err
		}
		if !written {
			// Either the caller's form was stale, or somebody committed between the read and
			// the write. Both are the same fact to the caller: what you changed is not what
			// stands (ADR-0025).
			return shared.ErrConflict.WithDetail("workspace.version_conflict")
		}

		changed.UpdatedAt = now
		changed.Version = expected + 1
		answer = changed
		return w.record(ctx, actor, moved, now)
	})
	if err != nil {
		return domain.Workspace{}, err
	}
	return answer, nil
}

// record writes the trail entry: which fields moved, from what, to what.
//
// Every one of them is `audit.Open`. A workspace's name, its two defaults and whether it demands
// a second factor of its administrators are configuration rather than anybody's personal data,
// and an entry that redacted them would be an entry that cannot answer "who turned enforcement
// off, and when".
func (w WorkspaceWriter) record(
	ctx context.Context, actor appshared.ActorContext, moved []domain.FieldChange, now time.Time,
) error {
	changes := make([]audit.Change, 0, len(moved))
	for _, change := range moved {
		changes = append(changes, audit.Change{
			Field: change.Field, Classification: audit.Open,
			From: change.From, To: change.To,
		})
	}
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: now,
		Action:     WorkspaceChangedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: workspaceTarget,
		TargetID:   actor.TenantID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// workspaceOutput is the read shape, shared by both use cases so that the answer after a write is
// the answer a read gives.
func workspaceOutput(workspace domain.Workspace) usecase.Output {
	out := usecase.Output{
		"id":                 workspace.ID.String(),
		"slug":               workspace.Slug,
		"display_name":       workspace.DisplayName,
		"status":             string(workspace.Status),
		"default_locale":     workspace.DefaultLocale,
		"default_time_zone":  workspace.DefaultTimeZone,
		"require_admin_totp": workspace.Settings.RequireAdminTotp,
		"created_at":         workspace.CreatedAt,
		"version":            workspace.Version,
	}
	if !workspace.UpdatedAt.IsZero() {
		out["updated_at"] = workspace.UpdatedAt
	}
	return out
}

func (h ReadWorkspace) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadWorkspaceName,
		Summary: "The workspace the caller is in: the slug it is reached by, its display name, " +
			"its standing, the locale and time zone every member without a preference of their " +
			"own falls back to, and whether it demands a second factor of its administrators. " +
			"Not the installation's listing of workspaces, which is the operator's.",
		SideEffects: "None. Reads only.",
		TokenScope:  workspaceRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: WorkspaceReadAction, TargetType: workspaceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadWorkspace) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	workspace, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return workspaceOutput(workspace), nil
}

func (h UpdateWorkspace) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateWorkspaceName,
		Summary: "Changes how the workspace is set up: its display name, the locale and time " +
			"zone its members fall back to, and whether it demands a second factor of its " +
			"OWNER and ADMIN role holders. A field the caller does not send does not move. The " +
			"slug does not move here at all - it is the hostname in multi mode and the base of " +
			"the registered OIDC redirect, so renaming it is the operator's operation.",
		SideEffects: "Writes the workspace and an audit entry naming every field that moved.",
		TokenScope:  workspaceManage,
		Input: []usecase.Field{
			{Name: "display_name", Kind: usecase.KindString,
				Description: "What the workspace is called, 1 to 200 characters."},
			{Name: "default_locale", Kind: usecase.KindString,
				Description: "A BCP-47 tag. The fallback for a member who has set none."},
			{Name: "default_time_zone", Kind: usecase.KindString,
				Description: "An IANA zone, for the same position in the same chain."},
			{Name: "require_admin_totp", Kind: usecase.KindBool,
				Description: "Whether an OWNER or ADMIN has to hold a second factor."},
			{Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read. Omitted means the caller named none."},
		},
		Audit: usecase.AuditDeclaration{
			Action: WorkspaceChangedAction, TargetType: workspaceTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a workspace's own configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateWorkspace) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	change := domain.WorkspaceChange{
		DisplayName:     in.OptionalString("display_name"),
		DefaultLocale:   in.OptionalString("default_locale"),
		DefaultTimeZone: in.OptionalString("default_time_zone"),
	}
	if in.Present("require_admin_totp") {
		wanted := in.Bool("require_admin_totp")
		change.RequireAdminTotp = &wanted
	}

	workspace, err := h.Execute(ctx, actor, UpdateWorkspaceCommand{
		Change: change, ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return workspaceOutput(workspace), nil
}
