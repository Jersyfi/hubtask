// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
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
func (h WriteInstanceSettings) Execute(
	ctx context.Context, actor appshared.ActorContext, level identityrepo.InstanceLevel,
) (identityrepo.InstanceLevel, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return identityrepo.InstanceLevel{}, err
	}

	var written identityrepo.InstanceLevel
	// The system scope: no tenant, and write access to the tables the schema deliberately left
	// without a policy. Every table with a tenant column stays invisible and unwritable under it,
	// which is what bounds this write to the one table it is for.
	err := w.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		before, err := w.Settings.Read(ctx)
		if err != nil {
			return err
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
		if identity.SwitchText(earlier.Policy, name) != identity.SwitchText(later.Policy, name) ||
			earlier.LockOf(name) != later.LockOf(name) {
			moved = append(moved, string(name))
		}
	}
	for _, link := range identity.LegalLinkNames() {
		if before.Legal.Links.Of(link) != after.Legal.Links.Of(link) ||
			before.Legal.Locks[link] != after.Legal.Locks[link] {
			moved = append(moved, string(link))
		}
	}
	if before.BlocklistFile != after.BlocklistFile {
		moved = append(moved, "blocklist_file")
	}
	return moved
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
			read, err := w.Operators.List(ctx)
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
		added, err := w.Operators.Add(ctx, accountID, actor.AccountID)
		if err != nil {
			return err
		}
		if !added {
			// Already there, or no such account. Which of the two is answered by reading the
			// register, and the caller's question - "make this account an operator" - is answered
			// either way only when it now is.
			register, err := w.Operators.List(ctx)
			if err != nil {
				return err
			}
			for _, operator := range register {
				if operator.AccountID == accountID {
					return nil
				}
			}
			return shared.ErrValidation.WithDetail("admin.operator_unknown_account")
		}
		return w.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.IDs.NewID(), OccurredAt: w.Clock.Now(), Action: journalOperatorAdded,
			ActorLabel: actor.AccountName,
			Details:    map[string]any{"account_id": accountID.String()},
		})
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
		removed, err := w.Operators.Remove(ctx, accountID)
		if err != nil {
			return err
		}
		if !removed {
			// Either it was not there, or it is the last one. The second is the interesting case
			// and the one worth a sentence: an installation with no operators is an installation
			// nobody can operate.
			return shared.ErrConflict.WithDetail("admin.operator_last")
		}
		return w.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.IDs.NewID(), OccurredAt: w.Clock.Now(), Action: journalOperatorRemoved,
			ActorLabel: actor.AccountName,
			Details:    map[string]any{"account_id": accountID.String()},
		})
	})
}

// instanceLevelOutput is the projection the control plane reads and writes back.
//
// The switches the operator decided, and only those: a level that answered the product's defaults
// for everything it had not decided would be a level nobody could tell apart from one that had
// decided them, and "the operator chose nothing here" is a value the resolver acts on.
func instanceLevelOutput(level identityrepo.InstanceLevel) usecase.Output {
	patch := level.Policy.Patch
	settings := usecase.Output{}

	put := func(name identity.PolicySwitch, value any) {
		entry := usecase.Output{"value": value, "locked": level.Policy.Locks[name]}
		settings[string(name)] = entry
	}
	if patch.MinLength != nil {
		put(identity.SwitchMinLength, *patch.MinLength)
	}
	if patch.MinLowercase != nil {
		put(identity.SwitchMinLowercase, *patch.MinLowercase)
	}
	if patch.MinUppercase != nil {
		put(identity.SwitchMinUppercase, *patch.MinUppercase)
	}
	if patch.MinDigits != nil {
		put(identity.SwitchMinDigits, *patch.MinDigits)
	}
	if patch.MinSymbols != nil {
		put(identity.SwitchMinSymbols, *patch.MinSymbols)
	}
	if patch.MinClasses != nil {
		put(identity.SwitchMinClasses, *patch.MinClasses)
	}
	if patch.MaxRepeat != nil {
		put(identity.SwitchMaxRepeat, *patch.MaxRepeat)
	}
	if patch.CommonPasswords != nil {
		put(identity.SwitchCommonPasswords, *patch.CommonPasswords)
	}
	if patch.ContextWords != nil {
		put(identity.SwitchContextWords, *patch.ContextWords)
	}
	if patch.BreachCheck != nil {
		put(identity.SwitchBreachCheck, *patch.BreachCheck)
	}
	if patch.MaxAgeDays != nil {
		put(identity.SwitchMaxAgeDays, *patch.MaxAgeDays)
	}
	if patch.HistoryCount != nil {
		put(identity.SwitchHistoryCount, *patch.HistoryCount)
	}
	if patch.MinAgeHours != nil {
		put(identity.SwitchMinAgeHours, *patch.MinAgeHours)
	}
	if patch.MfaRequiredFor != nil {
		put(identity.SwitchMfaRequiredFor, string(*patch.MfaRequiredFor))
	}
	if patch.Methods != nil {
		methods := make([]any, 0, len(*patch.Methods))
		for _, method := range *patch.Methods {
			methods = append(methods, method)
		}
		put(identity.SwitchMethods, methods)
	}
	if patch.SessionMaxDays != nil {
		put(identity.SwitchSessionMaxDays, *patch.SessionMaxDays)
	}
	if patch.SessionIdleMinutes != nil {
		put(identity.SwitchSessionIdleMinutes, *patch.SessionIdleMinutes)
	}

	legal := usecase.Output{}
	for _, link := range identity.LegalLinkNames() {
		value := level.Legal.Links.Of(link)
		if value == "" && !level.Legal.Locks[link] {
			continue
		}
		legal[string(link)] = usecase.Output{"value": value, "locked": level.Legal.Locks[link]}
	}

	out := usecase.Output{
		"sign_in":               settings,
		"legal":                 legal,
		"source":                level.Source,
		"is_enforced_from_file": level.IsEnforcedFromFile,
	}
	if level.BlocklistFile != "" {
		out["blocklist_file"] = level.BlocklistFile
	}
	return out
}

// instanceLevelFrom reads the document a `PUT` carried. An unknown key is ignored rather than
// refused, for the reason the adapter ignores an unknown row: a newer client talking to an older
// server should lose the switch it does not have rather than the save.
//
//nolint:gocyclo,cyclop // one case per switch is the mapping; splitting it hides which key is which
func instanceLevelFrom(in usecase.Input) (identityrepo.InstanceLevel, error) {
	level := identityrepo.InstanceLevel{
		Policy: identity.PolicyLayer{Locks: map[identity.PolicySwitch]bool{}},
		Legal:  identity.LegalLayer{Locks: map[identity.LegalLink]bool{}},
	}

	sent, _ := in["sign_in"].(map[string]any)
	for name, raw := range sent {
		entry, isObject := raw.(map[string]any)
		if !isObject {
			return identityrepo.InstanceLevel{}, refusedInstanceValue(name)
		}
		locked, _ := entry["locked"].(bool)
		if locked {
			level.Policy.Locks[identity.PolicySwitch(name)] = true
		}
		if err := applyInstanceSwitch(&level.Policy.Patch, name, entry["value"]); err != nil {
			return identityrepo.InstanceLevel{}, err
		}
	}

	legal, _ := in["legal"].(map[string]any)
	for name, raw := range legal {
		entry, isObject := raw.(map[string]any)
		if !isObject {
			return identityrepo.InstanceLevel{}, refusedInstanceValue(name)
		}
		link := identity.LegalLink(name)
		value, _ := entry["value"].(string)
		checked, err := identity.ValidLegalURL(link, value)
		if err != nil {
			return identityrepo.InstanceLevel{}, err
		}
		level.Legal.Links = level.Legal.Links.With(link, checked)
		if locked, _ := entry["locked"].(bool); locked {
			level.Legal.Locks[link] = true
		}
	}

	level.BlocklistFile = in.String("blocklist_file")
	return level, nil
}

//nolint:gocyclo,cyclop // one case per switch is the mapping
func applyInstanceSwitch(patch *identity.PolicyPatch, name string, raw any) error {
	if raw == nil {
		return nil
	}
	number := func() (*int, error) {
		switch value := raw.(type) {
		case int:
			return &value, nil
		case int64:
			narrowed := int(value)
			return &narrowed, nil
		case float64:
			narrowed := int(value)
			return &narrowed, nil
		}
		return nil, refusedInstanceValue(name)
	}
	flag := func() (*bool, error) {
		value, isBool := raw.(bool)
		if !isBool {
			return nil, refusedInstanceValue(name)
		}
		return &value, nil
	}

	var err error
	switch identity.PolicySwitch(name) {
	case identity.SwitchMinLength:
		patch.MinLength, err = number()
	case identity.SwitchMinLowercase:
		patch.MinLowercase, err = number()
	case identity.SwitchMinUppercase:
		patch.MinUppercase, err = number()
	case identity.SwitchMinDigits:
		patch.MinDigits, err = number()
	case identity.SwitchMinSymbols:
		patch.MinSymbols, err = number()
	case identity.SwitchMinClasses:
		patch.MinClasses, err = number()
	case identity.SwitchMaxRepeat:
		patch.MaxRepeat, err = number()
	case identity.SwitchMaxAgeDays:
		patch.MaxAgeDays, err = number()
	case identity.SwitchHistoryCount:
		patch.HistoryCount, err = number()
	case identity.SwitchMinAgeHours:
		patch.MinAgeHours, err = number()
	case identity.SwitchSessionMaxDays:
		patch.SessionMaxDays, err = number()
	case identity.SwitchSessionIdleMinutes:
		patch.SessionIdleMinutes, err = number()
	case identity.SwitchCommonPasswords:
		patch.CommonPasswords, err = flag()
	case identity.SwitchContextWords:
		patch.ContextWords, err = flag()
	case identity.SwitchBreachCheck:
		patch.BreachCheck, err = flag()
	case identity.SwitchMfaRequiredFor:
		value, isString := raw.(string)
		requirement := identity.MfaRequirement(value)
		if !isString || !requirement.Valid() {
			return refusedInstanceValue(name)
		}
		patch.MfaRequiredFor = &requirement
	case identity.SwitchMethods:
		values, isList := raw.([]any)
		if !isList {
			return refusedInstanceValue(name)
		}
		methods := make([]string, 0, len(values))
		for _, entry := range values {
			method, isString := entry.(string)
			if !isString {
				return refusedInstanceValue(name)
			}
			methods = append(methods, method)
		}
		patch.Methods = &methods
	}
	return err
}

func refusedInstanceValue(name string) error {
	return shared.ErrValidation.
		WithDetail("auth.policy_value_invalid").
		WithParams(map[string]string{"switch": name}).
		WithFields(shared.FieldError{
			Path: "/sign_in/" + name, Code: "auth.policy_value_invalid",
		})
}

func operatorOutput(operator identityrepo.Operator) usecase.Output {
	out := usecase.Output{
		"tenant_id":  operator.TenantID.String(),
		"account_id": operator.AccountID.String(),
		"added_at":   operator.AddedAt.UTC(),
	}
	if !operator.AddedBy.IsZero() {
		out["added_by"] = operator.AddedBy.String()
	}
	return out
}

// Descriptor is the catalogue entry.
func (h ReadInstanceSettings) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadInstanceSettingsName,
		Summary: "Answers what this installation has decided for every workspace on it: the " +
			"sign-in switches, which of them are locked, and the operator's legal links. Only " +
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
				Name: "account_id", Kind: usecase.KindString, Required: true,
				Description: "The account. It has to exist, and the workspace it lives in is " +
					"read from it rather than named - a pair that could disagree is a pair " +
					"somebody eventually gets wrong.",
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
