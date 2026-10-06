// SPDX-License-Identifier: Apache-2.0
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
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/port/text"
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
	// Text brings the display name to normal form C on the way in (i18n-l10n.md §5, M-07).
	Text text.Normalizer
	// Resolver is the three levels of the sign-in rule (ADR-0068 §2). Nil on an installation
	// wired before the instance layer, where a patch that touches the policy is refused rather
	// than written into a row nothing would read.
	Resolver SignInPolicyResolver
	// Hosts is the hosts this workspace answers at (SI-12). Optional: a build wired without it
	// answers none, which is what an installation whose workspaces predate migration 0104 has -
	// and nothing resolves a request through them, so an empty list costs nobody anything.
	Hosts repository.TenantHosts
	// Providers is the workspace's ways in beside the password, so that switching the password off
	// is refused where no provider is on (UC-ID-12 check 6). Nil counts none.
	Providers repository.IdentityProviders
	// StepUp is the proof a policy change demands (H-03). A workspace's sign-in rule is what
	// decides whether a stolen tab can weaken the way in, so the one patch that touches it asks
	// the person to prove themselves afresh - and the name, the locale and the zone do not.
	StepUp stepupport.Verifier
	// Permits decides who reads who asked for an operator's opening of the password and why
	// (ADR-0078 §3): the configuration readers - administrators, owners, the auditor - and nobody
	// else. Nil withholds both from everybody, which is the safe direction.
	Permits Permitter
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

// hostsOf answers the hosts this workspace answers at, in the same transaction shape every other
// read here uses. Empty where the store is not wired or the workspace predates migration 0104,
// which is the honest reading of "nothing here says otherwise".
func (w WorkspaceWriter) hostsOf(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.TenantHost, error) {
	if w.Hosts == nil {
		return nil, nil
	}
	var hosts []domain.TenantHost
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := w.Hosts.List(ctx)
		hosts = read
		return err
	})
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

// UpdateWorkspace changes how the workspace is set up.
type UpdateWorkspace struct{ Writer WorkspaceWriter }

// UpdateWorkspaceCommand is the merge-patch, typed. `ExpectedVersion` is what an `If-Match`
// carried, and zero is "the caller named none".
type UpdateWorkspaceCommand struct {
	Change          domain.WorkspaceChange
	ExpectedVersion int
	// SignIn is the sign-in half (SI-07), empty where the patch says nothing about it.
	SignIn WorkspacePolicyChange
	// RequireAdminTotp is the old boolean, where the caller sent it. It is not stored as itself:
	// it is translated into the rule's `mfa_required_for` and meets every check the rule has
	// (UC-ID-12 check 2). Nil is a caller that did not send it.
	RequireAdminTotp *bool
	// StepUpToken is demanded exactly when SignIn says something.
	StepUpToken string
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

	// The old boolean is the rule's switch under its old name, so it becomes that switch before
	// anything else is decided - and with it the proof, the lock and the direction the rule demands.
	// A flag sent as it stands moves nothing and so asks for nothing.
	if cmd.RequireAdminTotp != nil {
		translated, err := w.translateAdminFlag(ctx, actor.TenantID, *cmd.RequireAdminTotp, cmd.SignIn)
		if err != nil {
			return domain.Workspace{}, err
		}
		cmd.SignIn = translated
	}

	// The sign-in rule is the one part of this patch that needs a fresh proof. Checked before the
	// transaction opens, because a step-up consumes a credential and a refusal that had already
	// spent one would make the retry fail for a second reason.
	if !cmd.SignIn.IsEmpty() {
		if err := w.proveForPolicy(ctx, actor, cmd.StepUpToken); err != nil {
			return domain.Workspace{}, err
		}
	}

	var answer domain.Workspace
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := w.Workspaces.Find(ctx)
		if err != nil {
			return err
		}
		changed, moved, err := stored.With(cmd.Change, w.Text)
		if err != nil {
			return err
		}
		now := w.Clock.Now()
		changed, signInMoved, err := w.applyPolicy(ctx, actor.TenantID, changed, cmd.SignIn, now)
		if err != nil {
			return err
		}
		moved = append(moved, signInMoved...)
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

// proveForPolicy demands the step-up a policy change carries.
//
// Only a session can prove it, StepUp's reasoning: a personal access token has no person at the
// keyboard to ask. An installation with no verifier wired demands nothing, which is what an
// installation without the sign-in flow did before there was one.
func (w WorkspaceWriter) proveForPolicy(
	ctx context.Context, actor appshared.ActorContext, token string,
) error {
	if w.StepUp == nil {
		return nil
	}
	// Demand rather than the same three checks written out: it is what names the methods on the
	// refusal, which is what a client builds its prompt from.
	return stepupport.Demand(ctx, w.StepUp, actor.TenantID, actor.AccountID, token)
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

// standing keeps an operator's opening of the password only while it stands (ADR-0078 §3): an
// opening past its end is answered as none, whatever the row still holds until the job that records
// the end has run. Without a clock there is no telling, and an opening read as standing forever is
// the one mistake its bound exists to prevent.
//
// And who asked and why only for a reader of the workspace's configuration: every member is told
// that the password is open and until when - it concerns how they sign in - but the requester may
// name a person and the reason may say more than a member needs, so both are the administrators'.
// Asked without a record: withholding two fields is no refusal anybody attempted.
func (w WorkspaceWriter) standing(
	ctx context.Context, actor appshared.ActorContext, workspace domain.Workspace,
) (domain.Workspace, error) {
	if w.Clock == nil || !workspace.PasswordOpening.InForce(w.Clock.Now()) {
		workspace.PasswordOpening = domain.PasswordOpening{}
		return workspace, nil
	}
	attributed := false
	if w.Permits != nil {
		permitted, err := w.Permits.Permits(ctx, actor, access.Request{
			Permission: service.PermissionReadConfiguration,
			Path:       []domain.Scope{domain.TenantScope()},
			Action:     WorkspaceReadAction,
			TokenScope: workspaceRead,
			TargetType: workspaceTarget,
			TargetID:   actor.TenantID,
		})
		if err != nil {
			return domain.Workspace{}, err
		}
		attributed = permitted
	}
	if !attributed {
		workspace.PasswordOpening.Requester = ""
		workspace.PasswordOpening.Reason = ""
	}
	return workspace, nil
}

// workspaceOutput is the read shape, shared by both use cases so that the answer after a write is
// the answer a read gives.
//
// The sign-in rule rides along when it could be resolved, in its three-level shape: what is in
// force, what the level above set, and where a lock came from. It is absent rather than empty on an
// installation with no instance layer - a screen that drew eighteen rows of the product's defaults
// and could not save any of them would be a screen that lies about what it offers.
func workspaceOutput(
	workspace domain.Workspace, resolved *ResolvedPolicy, hosts []domain.TenantHost,
) usecase.Output {
	out := usecase.Output{
		"id":                 workspace.ID.String(),
		"slug":               workspace.Slug,
		"display_name":       workspace.DisplayName,
		"status":             string(workspace.Status),
		"default_locale":     workspace.DefaultLocale,
		"default_time_zone":  workspace.DefaultTimeZone,
		"require_admin_totp": adminFlagOf(workspace, resolved),
		"created_at":         workspace.CreatedAt,
		"version":            workspace.Version,
	}
	if !workspace.UpdatedAt.IsZero() {
		out["updated_at"] = workspace.UpdatedAt
	}
	// The anchoring target is read here and written by ConfigureAuditAnchoring (issue 774): a
	// screen that sets a value it cannot read back is guessing. Absent, not null, where anchoring
	// is off, so that the adapter answers the contract's null from one place.
	if !workspace.Settings.AuditAnchorTargetID.IsZero() {
		out["audit_anchor_target_id"] = workspace.Settings.AuditAnchorTargetID.String()
	}
	if resolved != nil {
		out["sign_in_policy"] = signInPolicyOutput(*resolved)
	}
	// The installation operator's opening of the password, while it stands (ADR-0078 §3): until
	// when for everybody, and for whom and why where the reader may read the configuration
	// (standing). Read-only - it is the control plane's, and the PATCH declares no field for it.
	if opening := workspace.PasswordOpening; !opening.Until.IsZero() {
		answer := usecase.Output{"until": opening.Until}
		if opening.Requester != "" {
			answer["requester"] = opening.Requester
		}
		if opening.Reason != "" {
			answer["reason"] = opening.Reason
		}
		out["password_opening"] = answer
	}
	// The hosts this workspace answers at (SI-12). Read-only and absent where there are none: the
	// canonical one is derived from the slug and nothing resolves a request through the table yet,
	// so an empty array would read as "this workspace is reachable nowhere".
	if len(hosts) > 0 {
		rows := make([]usecase.Output, 0, len(hosts))
		for _, host := range hosts {
			row := usecase.Output{
				"host":         host.Host,
				"state":        string(host.State),
				"is_canonical": host.Canonical,
				"created_at":   host.CreatedAt,
			}
			// The mark travels with a row whose zone still has to carry it - which is a claim that
			// is PENDING or one that BROKE and has to be proved again. Not for the canonical one:
			// it is ACTIVE by construction, and a value nobody has to publish is a value nobody
			// has to be shown.
			if !host.Canonical &&
				(host.State == domain.HostPending || host.State == domain.HostBroken) {
				row["verification"] = host.Verification
			}
			if !host.VerifiedAt.IsZero() {
				row["verified_at"] = host.VerifiedAt
			}
			rows = append(rows, row)
		}
		out["hosts"] = rows
	}
	return out
}

func (h ReadWorkspace) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadWorkspaceName,
		Summary: "The workspace the caller is in: the slug it is reached by, its display name, " +
			"its standing, the locale and time zone every member without a preference of their " +
			"own falls back to, and its sign-in rule - `require_admin_totp` among it, read from " +
			"the rule in force. Not the installation's listing of workspaces, which is the operator's.",
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
	hosts, err := h.Writer.hostsOf(ctx, actor)
	if err != nil {
		return nil, err
	}
	standing, err := h.Writer.standing(ctx, actor, workspace)
	if err != nil {
		return nil, err
	}
	return workspaceOutput(standing, h.Writer.resolvedPolicy(ctx, actor.TenantID), hosts), nil
}

func (h UpdateWorkspace) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateWorkspaceName,
		Summary: "Changes how the workspace is set up: its display name, the locale and time " +
			"zone its members fall back to, and its sign-in rule. A field the caller does not " +
			"send does not move. The " +
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
				Description: "The old name for `sign_in_policy.mfa_required_for`: true asks for " +
					"`ADMINS` where the rule demands less, false for `NONE`. Not stored as itself - " +
					"it is that change to the rule, with the rule's step-up, lock and direction."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The version last read. Omitted means the caller named none."},
			{Name: "sign_in_policy", Kind: usecase.KindObject,
				Description: "The sign-in switches this workspace is tightening, flat: the " +
					"thirteen password switches, `mfa_required_for`, `methods`, the two session " +
					"bounds, the four legal links, and `rotation_from` as the literal `now`. A " +
					"switch the caller does not send does not move; one the level above locked is " +
					"refused against its own field, and so is one that would loosen the rule."},
			{Name: "step_up_token", Kind: usecase.KindString,
				Description: "The proof from `/auth/step-up`, demanded exactly when " +
					"`sign_in_policy` says something."},
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
	var adminFlag *bool
	if in.Present("require_admin_totp") {
		wanted := in.Bool("require_admin_totp")
		adminFlag = &wanted
	}

	sent, _ := in["sign_in_policy"].(map[string]any)
	signIn, err := policyPatchFrom(sent)
	if err != nil {
		return nil, err
	}

	workspace, err := h.Execute(ctx, actor, UpdateWorkspaceCommand{
		Change: change, ExpectedVersion: in.Int("expected_version"),
		SignIn: signIn, StepUpToken: in.String("step_up_token"), RequireAdminTotp: adminFlag,
	})
	if err != nil {
		return nil, err
	}
	hosts, err := h.Writer.hostsOf(ctx, actor)
	if err != nil {
		return nil, err
	}
	standing, err := h.Writer.standing(ctx, actor, workspace)
	if err != nil {
		return nil, err
	}
	return workspaceOutput(standing, h.Writer.resolvedPolicy(ctx, actor.TenantID), hosts), nil
}

// resolvedPolicy answers the rule for the projection, or nil where there is no level above to
// resolve against. A read that could not resolve answers the workspace without its sign-in half
// rather than failing: the name, the locale and the zone are still true.
func (w WorkspaceWriter) resolvedPolicy(ctx context.Context, tenantID shared.ID) *ResolvedPolicy {
	if w.Resolver.Instance == nil {
		return nil
	}
	resolved, err := w.Resolver.Resolve(ctx, tenantID)
	if err != nil {
		return nil
	}
	return &resolved
}
