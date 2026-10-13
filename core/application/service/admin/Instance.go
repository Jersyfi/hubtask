// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"strings"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/quota"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The level above the workspaces, as the control plane writes it (ADR-0070 §2, §5).
//
// **One API, and this is it.** `hubctl admin`, the instance dashboard and the file an operator
// checks into a repository are three clients of these use cases rather than three implementations
// of them - which is what keeps "configured by code" and "configured by clicking" the same
// installation rather than two that drift.
//
// Everything here is behind `admin:tenants` **and** the operator register, both checked: the scope
// says what a credential may reach and the register says whose credential it may be, and either
// alone is a hole (ADR-0070 §1).

const (
	ReadInstanceSettingsName  = "ReadInstanceSettings"
	WriteInstanceSettingsName = "WriteInstanceSettings"
	ListOperatorsName         = "ListOperators"
	AddOperatorName           = "AddOperator"
	RemoveOperatorName        = "RemoveOperator"

	instanceTarget = "instance"
)

// The audit actions. The installation's configuration is not any one workspace's event, so these
// are recorded in the instance journal - but a *refused* read is recorded against the action that
// was refused, and that action has to have a name (audit.md §4).
const (
	instanceSettingsReadAction    audit.Action = "instance.settings_read"
	instanceSettingsChangedAction audit.Action = "instance.settings_changed"
	instanceOperatorsReadAction   audit.Action = "instance.operators_read"
	instanceOperatorAddedAction   audit.Action = "instance.operator_added"
	instanceOperatorRemovedAction audit.Action = "instance.operator_removed"
)

// The journal's actions. The installation's own evidence, where no tenant's trail can hold it:
// changing a value that applies to every workspace is not any one workspace's event (audit.md §6).
const (
	journalSettingsChanged = string(instanceSettingsChangedAction)
	journalOperatorAdded   = string(instanceOperatorAddedAction)
	journalOperatorRemoved = string(instanceOperatorRemovedAction)
)

// InstanceWriter is what the control plane's instance use cases share.
type InstanceWriter struct {
	Settings   identityrepo.InstanceSettings
	Operators  identityrepo.Operators
	Journal    adminrepo.Journal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// authorize is the pair every operation here demands: the scope, then the register.
//
// The register is checked again although the credential could only carry the scope by passing it at
// minting time, and that is the point: a token minted last month by somebody who has since been
// removed still carries the scope, and the second check is what makes the removal mean something
// (ADR-0070 §1).
func (w InstanceWriter) authorize(ctx context.Context, actor appshared.ActorContext) error {
	if err := actor.RequireScope(adminTenantsScope); err != nil {
		return err
	}
	if w.Operators == nil {
		return nil
	}

	var held bool
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := w.Operators.Holds(ctx, actor.AccountID)
			held = read
			return err
		})
	if err != nil {
		return err
	}
	if !held {
		return shared.ErrForbidden.WithDetail("admin.operator_required")
	}
	return nil
}

// ReadInstanceSettings answers what the installation has decided.
type ReadInstanceSettings struct{ Writer InstanceWriter }

// Execute reads the level.
func (h ReadInstanceSettings) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (identityrepo.InstanceLevel, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return identityrepo.InstanceLevel{}, err
	}

	var level identityrepo.InstanceLevel
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := w.Settings.Read(ctx)
			level = read
			return err
		})
	if err != nil {
		return identityrepo.InstanceLevel{}, err
	}
	return level, nil
}

// WriteInstanceSettings replaces the level whole.
type WriteInstanceSettings struct{ Writer InstanceWriter }

// Execute writes it.
//
// A `PUT` rather than a merge, because a merge over eighteen switches that can each be absent has no
// way to say "unset this one" - and "the operator decided nothing here" is a value the resolver acts
// on, so it has to be expressible.
// ExecuteAsFile writes the level on behalf of the instance file, at start-up.
//
// **The one caller that is not a person**, and the one that may write while a file enforces —
// because it *is* the file. It carries no actor: there is nobody to authorise, nothing has a
// session yet, and the journal records the source rather than a name. Everything else about it is
// the same write, including the entry in the journal, which is what makes a change that arrived
// from a repository as visible as one somebody clicked.
func (h WriteInstanceSettings) ExecuteAsFile(
	ctx context.Context, level identityrepo.InstanceLevel, source string,
) (identityrepo.InstanceLevel, error) {
	return h.write(ctx, appshared.ActorContext{AccountName: source}, level)
}

func (h WriteInstanceSettings) Execute(
	ctx context.Context, actor appshared.ActorContext, level identityrepo.InstanceLevel,
) (identityrepo.InstanceLevel, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return identityrepo.InstanceLevel{}, err
	}
	return h.write(ctx, actor, level)
}

func (h WriteInstanceSettings) write(
	ctx context.Context, actor appshared.ActorContext, level identityrepo.InstanceLevel,
) (identityrepo.InstanceLevel, error) {
	w := h.Writer

	var written identityrepo.InstanceLevel
	// The system scope: no tenant, and write access to the tables the schema deliberately left
	// without a policy. Every table with a tenant column stays invisible and unwritable under it,
	// which is what bounds this write to the one table it is for.
	err := w.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		before, err := w.Settings.Read(ctx)
		if err != nil {
			return err
		}
		// A file in `enforce` mode is the source, and it is rewritten at every start. A save
		// accepted here would be a save the next restart silently undoes, which is worse than a
		// refusal: the operator would believe their change took (ADR-0070 §5, the concept's §5.4 —
		// "ein Ort je Betriebsart, nie zwei").
		if before.IsEnforcedFromFile {
			return shared.ErrConflict.
				WithDetail("admin.instance_enforced_from_file").
				WithParams(map[string]string{"source": before.Source})
		}
		if err := w.Settings.Write(ctx, level, actor.AccountID, w.Clock.Now()); err != nil {
			return err
		}
		read, err := w.Settings.Read(ctx)
		if err != nil {
			return err
		}
		written = read

		return w.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.IDs.NewID(), OccurredAt: w.Clock.Now(), Action: journalSettingsChanged,
			ActorLabel: actor.AccountName,
			// The switches that moved, by name. Never their values: a journal that recorded the
			// blocklist's path or a legal URL would be a journal carrying configuration into a
			// place nothing ever deletes from.
			Details: map[string]any{"switches": movedSwitches(before, read)},
		})
	})
	if err != nil {
		return identityrepo.InstanceLevel{}, err
	}
	return written, nil
}

// movedSwitches names what changed, in the domain's own order.
func movedSwitches(before, after identityrepo.InstanceLevel) []any {
	moved := make([]any, 0, len(identity.PolicySwitches()))
	earlier := identity.Effective(before.Policy, identity.PolicyLayer{}, identity.PolicyLayer{})
	later := identity.Effective(after.Policy, identity.PolicyLayer{}, identity.PolicyLayer{})

	for _, name := range identity.PolicySwitches() {
		// Whether the installation decides a switch is a change of its own, even where the value
		// in force stays the product's default (UC-INS-12 check 3).
		if identity.SwitchText(earlier.Policy, name) != identity.SwitchText(later.Policy, name) ||
			earlier.LockOf(name) != later.LockOf(name) ||
			before.Policy.Patch.Decides(name) != after.Policy.Patch.Decides(name) {
			moved = append(moved, string(name))
		}
	}
	for _, link := range identity.LegalSettingNames() {
		if before.Legal.Links.Of(link) != after.Legal.Links.Of(link) ||
			before.Legal.Locks[link] != after.Legal.Locks[link] {
			moved = append(moved, string(link))
		}
	}
	for _, name := range identity.LocalisationKeys() {
		if localisationValueOf(before.Localisation, name) != localisationValueOf(after.Localisation, name) {
			moved = append(moved, "localisation."+name)
		}
	}
	for _, name := range quota.OperatorNames() {
		earlierCeiling, earlierSet := before.Quotas.Of(name)
		laterCeiling, laterSet := after.Quotas.Of(name)
		if earlierCeiling != laterCeiling || earlierSet != laterSet ||
			before.Quotas.Locked(name) != after.Quotas.Locked(name) {
			moved = append(moved, "quotas."+name)
		}
	}
	for _, name := range integration.AiSwitches() {
		if aiValueOf(before.Ai, name) != aiValueOf(after.Ai, name) ||
			before.Ai.Locks[name] != after.Ai.Locks[name] {
			moved = append(moved, "ai."+string(name))
		}
	}
	if before.BlocklistFile != after.BlocklistFile {
		moved = append(moved, "blocklist_file")
	}
	return moved
}

// aiValueOf is one key of the AI area as text, empty where the level decided nothing.
func aiValueOf(layer integration.AiLayer, name integration.AiSwitch) string {
	switch name {
	case integration.AiSwitchSources:
		if layer.Sources != nil {
			return string(*layer.Sources)
		}
	case integration.AiSwitchMinJurisdiction:
		if layer.MinJurisdiction != nil {
			return string(*layer.MinJurisdiction)
		}
	case integration.AiSwitchPersonOptOut:
		if layer.PersonOptOut != nil {
			return string(*layer.PersonOptOut)
		}
	}
	return ""
}

// ListOperators answers the register.
type ListOperators struct{ Writer InstanceWriter }

// Execute lists it.
func (h ListOperators) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]identityrepo.Operator, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return nil, err
	}

	var register []identityrepo.Operator
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := w.Operators.Roster(ctx, actor.AccountID)
			register = read
			return err
		})
	if err != nil {
		return nil, err
	}
	return register, nil
}

// AddOperator puts an account in the register.
type AddOperator struct{ Writer InstanceWriter }

// Execute adds. An account that is already an operator is not an error: the caller asked for
// somebody to be one, and they are.
func (h AddOperator) Execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID,
) error {
	return h.execute(ctx, actor, accountID)
}

// ExecuteByAddress names the account the way a person can: the workspace it is in, and the address
// it signs in with.
//
// The identifier form stays — a script that already has one keeps working — and this is the form a
// screen uses, because an account id is not something an operator can look up at all: `account` is
// behind row level security, so the control plane cannot list accounts across workspaces.
// Resolution happens through the register's own narrow door and answers one identifier or nothing.
func (h AddOperator) ExecuteByAddress(
	ctx context.Context, actor appshared.ActorContext, slug, email string,
) error {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return err
	}
	if strings.TrimSpace(slug) == "" || strings.TrimSpace(email) == "" {
		return shared.ErrValidation.WithDetail("admin.operator_incomplete")
	}

	var found shared.ID
	if err := w.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		resolved, err := w.Operators.Resolve(ctx, slug, email)
		found = resolved
		return err
	}); err != nil {
		return err
	}
	if found.IsZero() {
		// The same refusal a wrong identifier produced, and deliberately the same: whether an
		// address exists in a workspace is exactly what somebody probing would want to learn, and
		// the caller may already learn as much by trying the identifier form.
		return shared.ErrValidation.WithDetail("admin.operator_unknown_account")
	}
	return h.execute(ctx, actor, found)
}

func (h AddOperator) execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID,
) error {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return err
	}
	if accountID.IsZero() {
		return shared.ErrValidation.WithDetail("admin.operator_incomplete")
	}

	// The workspace is read from the account by the register's own function rather than named by
	// the caller: a pair that could disagree is a pair somebody eventually gets wrong, and only
	// that function can read the account row across the tenant boundary at all.
	return w.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		outcome, err := w.Operators.Enlist(ctx, accountID, actor.AccountID)
		if err != nil {
			return err
		}
		switch outcome {
		case identityrepo.EnlistPresent:
			// The caller asked for somebody to be an operator, and they are.
			return nil
		case identityrepo.EnlistNotActive:
			return shared.ErrValidation.WithDetail("admin.operator_not_active")
		case identityrepo.EnlistUnknown:
			return shared.ErrValidation.WithDetail("admin.operator_unknown_account")
		case identityrepo.EnlistAddedWithCaller:
			// The register was empty, so the caller was an operator only because it was; the
			// statement put them in too, and that is as much an addition as the one asked for.
			if err := w.recordAdded(ctx, actor, actor.AccountID); err != nil {
				return err
			}
		case identityrepo.EnlistAdded:
		default:
			return shared.ErrInternal.WithDetail("admin.operator_outcome_unknown")
		}
		return w.recordAdded(ctx, actor, accountID)
	})
}

func (w InstanceWriter) recordAdded(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID,
) error {
	return w.Journal.Record(ctx, adminrepo.InstanceEvent{
		ID: w.IDs.NewID(), OccurredAt: w.Clock.Now(), Action: journalOperatorAdded,
		ActorLabel: actor.AccountName,
		Details:    map[string]any{"account_id": accountID.String()},
	})
}

// RemoveOperator takes one out.
type RemoveOperator struct{ Writer InstanceWriter }

// Execute removes, unless it would empty the register.
func (h RemoveOperator) Execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID,
) error {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return err
	}

	return w.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		outcome, err := w.Operators.Dismiss(ctx, accountID)
		if err != nil {
			return err
		}
		switch outcome {
		case identityrepo.DismissRemoved:
		case identityrepo.DismissLast:
			// An installation with no operators is an installation nobody can operate.
			return shared.ErrConflict.WithDetail("admin.operator_last")
		case identityrepo.DismissAbsent:
			return shared.ErrNotFound.WithDetail("admin.operator_not_registered")
		default:
			return shared.ErrInternal.WithDetail("admin.operator_outcome_unknown")
		}
		return w.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.IDs.NewID(), OccurredAt: w.Clock.Now(), Action: journalOperatorRemoved,
			ActorLabel: actor.AccountName,
			Details:    map[string]any{"account_id": accountID.String()},
		})
	})
}

func operatorOutput(operator identityrepo.Operator) usecase.Output {
	out := usecase.Output{
		"tenant_id":      operator.TenantID.String(),
		"account_id":     operator.AccountID.String(),
		"added_at":       operator.AddedAt.UTC(),
		"display_name":   operator.DisplayName,
		"kind":           string(operator.Kind),
		"workspace_slug": operator.WorkspaceSlug,
		"workspace_name": operator.WorkspaceName,
	}
	if !operator.AddedBy.IsZero() {
		out["added_by"] = operator.AddedBy.String()
	}
	if operator.Email != "" {
		out["email"] = operator.Email
	}
	if operator.SignInName != "" {
		out["sign_in_name"] = operator.SignInName
	}
	if operator.AddedByName != "" {
		out["added_by_name"] = operator.AddedByName
	}
	return out
}

// Descriptor is the catalogue entry.
func (h ReadInstanceSettings) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadInstanceSettingsName,
		Summary: "Answers what this installation has decided for every workspace on it: the " +
			"sign-in switches, which of them are locked, the legal links, the localisation " +
			"defaults, the quota ceilings and which AI workspaces may use. Every key of every area, decided or not. Only " +
			"what was decided - a level that answered the product's defaults for everything " +
			"else would be one nobody could tell apart from an operator who had chosen them.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: instanceSettingsReadAction, TargetType: instanceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the installation's configuration is not an entry's history.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadInstanceSettings) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	level, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return instanceLevelOutput(level), nil
}

// Descriptor is the catalogue entry.
func (h WriteInstanceSettings) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: WriteInstanceSettingsName,
		Summary: "Replaces the installation's level whole. A `PUT` rather than a merge, because a " +
			"merge over eighteen switches that can each be absent has no way to say 'unset this " +
			"one' - and 'the operator decided nothing here' is a value the resolver acts on, so " +
			"it has to be expressible. A locked switch applies to every workspace and switches " +
			"its control off, with the reason and with who set it, rather than hiding it.",
		SideEffects: "Writes the installation's settings and one journal entry naming the " +
			"switches that moved - never their values.",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{
				Name: "sign_in", Kind: usecase.KindObject,
				Description: "The switches, by name, each `{value, locked}`. A switch the body " +
					"does not name is not decided at this level.",
			},
			{
				Name: "legal", Kind: usecase.KindObject,
				Description: "The four links, each `{value, locked}`.",
			},
			{
				Name: "localisation", Kind: usecase.KindObject,
				Description: "`locale`, `time_zone` and `week_start` - what a workspace that set " +
					"none of its own inherits. A default and never a lock: a write that carries " +
					"one is refused rather than partly obeyed.",
			},
			{
				Name: "quotas", Kind: usecase.KindObject,
				Description: "One ceiling per quota, each `{value, locked}` - the middle level " +
					"of Effective(product, instance, plan, workspace). A workspace's own still " +
					"wins unless the ceiling is locked; 0 is unlimited and absent is undecided.",
			},
			{
				Name: "ai", Kind: usecase.KindObject,
				Description: "`sources` (NONE, OFFERED, OWN, EITHER) - which AI a workspace may " +
					"use - and `min_jurisdiction` (ANY, ADEQUACY, EEA, SELF_HOSTED) - the least a " +
					"workspace's own model must declare - each `{value, locked}`. A lock holds a " +
					"workspace out of a source and never in one: no AI is always its own choice, " +
					"and no value here switches processing on.",
			},
			{
				Name: "blocklist_file", Kind: usecase.KindString,
				Description: "The path to the operator's own list of refused passwords, read " +
					"offline. Instance-only: the file is on the operator's disk, so there is " +
					"nothing for a workspace to point at.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: instanceSettingsChangedAction, TargetType: instanceTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the installation's configuration is not an entry's history.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h WriteInstanceSettings) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	level, err := instanceLevelFrom(in)
	if err != nil {
		return nil, err
	}
	written, err := h.Execute(ctx, actor, level)
	if err != nil {
		return nil, err
	}
	return instanceLevelOutput(written), nil
}

// Descriptor is the catalogue entry.
func (h ListOperators) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListOperatorsName,
		Summary: "Answers who operates this installation. An empty register is the private " +
			"installation: nothing was configured, and the owner is the operator exactly as " +
			"they were before the register existed.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: instanceOperatorsReadAction, TargetType: instanceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the operator register is not an entry's history.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListOperators) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	register, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(register))
	for _, operator := range register {
		rows = append(rows, operatorOutput(operator))
	}
	return usecase.Output{"data": rows}, nil
}

// Descriptor is the catalogue entry.
func (h AddOperator) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AddOperatorName,
		Summary: "Puts an account in the operator register. A service account may be one: a " +
			"purchase platform that provisions workspaces needs a credential that does not " +
			"belong to a person who may leave. An account that is already an operator is not " +
			"an error - the caller asked for somebody to be one, and they are.",
		SideEffects: "Writes the register and one journal entry.",
		TokenScope:  adminTenantsScope,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindString,
				Description: "The account, where the caller has its identifier. The workspace it " +
					"lives in is read from it rather than named - a pair that could disagree is " +
					"a pair somebody eventually gets wrong.",
			},
			{
				Name: "workspace", Kind: usecase.KindString,
				Description: "The workspace's address, with `email`, where the caller does not " +
					"have an identifier - which is every screen, because the control plane " +
					"cannot list accounts across workspaces and so cannot show one.",
			},
			{
				Name: "email", Kind: usecase.KindString,
				Description: "The address the account signs in with, inside `workspace`.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: instanceOperatorAddedAction, TargetType: instanceTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the operator register is not an entry's history.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AddOperator) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	// Two ways of naming one account, and neither is required on its own: a script has the
	// identifier, a person has the address. Both absent is the incomplete refusal, which is the
	// same one an empty identifier already produced.
	if in.Present("workspace") || in.Present("email") {
		if err := h.ExecuteByAddress(ctx, actor, in.String("workspace"), in.String("email")); err != nil {
			return nil, err
		}
		return usecase.Output{}, nil
	}
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, accountID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// Descriptor is the catalogue entry.
func (h RemoveOperator) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemoveOperatorName,
		Summary: "Takes an account out of the operator register. The last one cannot be removed: " +
			"an installation with no operators is an installation nobody can operate, and the " +
			"refusal is in the statement rather than in a read two operators could race.",
		SideEffects: "Writes the register and one journal entry.",
		TokenScope:  adminTenantsScope,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindString, Required: true,
				Description: "The account to remove. An identifier is unique across the " +
					"installation, so the workspace does not have to be named.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: instanceOperatorRemovedAction, TargetType: instanceTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the operator register is not an entry's history.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RemoveOperator) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, accountID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
