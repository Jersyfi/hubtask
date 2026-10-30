// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"slices"
	"strconv"
	"strings"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/quota"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/port/audit"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The installation's document has a plan and a history (accepted.md D, ADR-0070 §5): a plan says
// what a level would change, key by key, and writes nothing; the history is the journal's
// `instance.settings_changed` entries, each naming the keys that moved and the version it made.
// Applying stays the instance file's in enforce mode and PUT /admin/settings' otherwise - no
// second write path.
const (
	PlanInstanceSettingsName = "PlanInstanceSettings"

	instanceSettingsPlannedAction audit.Action = "instance.settings_planned"

	// How a planned level would be applied.
	AppliedThroughAPI  = "API"
	AppliedThroughFile = "FILE"
)

// keyState is one key of a level as a plan compares it.
type keyState struct {
	Key     string
	Value   string
	Decided bool
	Locked  bool
}

// keyStates is every key of a level in the domain's order, the same keys movedSwitches names.
func keyStates(level identityrepo.InstanceLevel) []keyState {
	states := make([]keyState, 0, 64)
	effective := identity.Effective(level.Policy, identity.PolicyLayer{}, identity.PolicyLayer{})
	for _, name := range identity.PolicySwitches() {
		states = append(states, keyState{
			Key: string(name), Value: identity.SwitchText(effective.Policy, name),
			Decided: level.Policy.Patch.Decides(name), Locked: effective.LockOf(name) != "",
		})
	}
	for _, link := range identity.LevelLegalSettingNames() {
		value := level.Legal.Links.Of(link)
		states = append(states, keyState{Key: string(link), Value: value, Decided: value != "", Locked: level.Legal.Locks[link]})
	}
	for _, name := range identity.LocalisationKeys() {
		value := localisationValueOf(level.Localisation, name)
		states = append(states, keyState{Key: "localisation." + name, Value: value, Decided: value != ""})
	}
	for _, name := range quota.OperatorNames() {
		ceiling, set := level.Quotas.Of(name)
		value := ""
		if set {
			value = strconv.FormatInt(ceiling, 10)
		}
		states = append(states, keyState{Key: "quotas." + name, Value: value, Decided: set, Locked: level.Quotas.Locked(name)})
	}
	for _, name := range integration.AiSwitches() {
		value := aiValueOf(level.Ai, name)
		states = append(states, keyState{Key: "ai." + string(name), Value: value, Decided: value != "", Locked: level.Ai.Locks[name]})
	}
	for _, key := range module.Keys() {
		decision, decided := level.Modules[key]
		value := ""
		if decided {
			value = strconv.FormatBool(decision.On)
		}
		states = append(states, keyState{Key: "module." + string(key), Value: value, Decided: decided, Locked: decision.Locked})
	}
	states = append(states,
		keyState{Key: "blocklist_file", Value: level.BlocklistFile, Decided: level.BlocklistFile != ""},
		keyState{Key: "metering", Value: strconv.FormatBool(level.Metering), Decided: level.Metering},
		keyState{Key: "reserved_short_names", Value: strings.Join(level.ReservedShortNames, ","), Decided: len(level.ReservedShortNames) > 0},
	)
	return states
}

// InstanceChange is one key a planned level moves.
type InstanceChange struct {
	Key                  string
	From, To             string
	FromLocked, ToLocked bool
}

// InstancePlan is what a planned level would do.
type InstancePlan struct {
	// Version is the level's version the plan was made on; a save on it presents it in If-Match.
	Version int
	Changes []InstanceChange
	// AppliedThrough is API, or FILE where a file enforces the level and only it applies.
	AppliedThrough string
	Source         string
}

// planOf compares two levels key by key.
func planOf(before, after identityrepo.InstanceLevel) []InstanceChange {
	earlier, later := keyStates(before), keyStates(after)
	changes := make([]InstanceChange, 0)
	for i, was := range earlier {
		now := later[i]
		if was == now {
			continue
		}
		changes = append(changes, InstanceChange{Key: was.Key, From: was.Value, To: now.Value, FromLocked: was.Locked, ToLocked: now.Locked})
	}
	return changes
}

// PlanInstanceSettings answers what replacing the installation's level would change, writing
// nothing.
type PlanInstanceSettings struct{ Writer InstanceWriter }

// Execute plans the level against the one in force.
func (h PlanInstanceSettings) Execute(
	ctx context.Context, actor appshared.ActorContext, level identityrepo.InstanceLevel,
) (InstancePlan, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return InstancePlan{}, err
	}
	var before identityrepo.InstanceLevel
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		read, err := w.Settings.Read(ctx)
		before = read
		return err
	})
	if err != nil {
		return InstancePlan{}, err
	}
	plan := InstancePlan{Version: before.Version, Changes: planOf(before, level), AppliedThrough: AppliedThroughAPI}
	if before.IsEnforcedFromFile {
		plan.AppliedThrough, plan.Source = AppliedThroughFile, before.Source
	}
	return plan, nil
}

// planInputIgnored are the read-only members of the level's answer: a plan takes the document the
// read answered unchanged.
var planInputIgnored = []string{"source", "is_enforced_from_file", "version"}

// Descriptor is the catalogue entry.
func (h PlanInstanceSettings) Descriptor() usecase.Descriptor {
	input := make([]usecase.Field, 0, 16)
	for _, field := range (WriteInstanceSettings{}).Descriptor().Input {
		if field.Name != "expected_version" {
			input = append(input, field)
		}
	}
	if !slices.ContainsFunc(input, func(f usecase.Field) bool { return f.Name == module.Area }) {
		input = append(input, usecase.Field{
			Name: module.Area, Kind: usecase.KindObject,
			Description: "The modules, by key, each `{value, locked}`: on or off for every workspace.",
		})
	}
	for _, name := range planInputIgnored {
		input = append(input, usecase.Field{Name: name, Kind: usecase.KindAny, CallerOnly: true,
			Description: "Read-only in the level's answer; ignored."})
	}
	return usecase.Descriptor{
		Name: PlanInstanceSettingsName,
		Summary: "Answers what replacing the installation's level with this one would change, key by key " +
			"with the value before and after and whether it is locked, on the version in force. Writes " +
			"nothing. Where a file enforces the level only the file applies it (FILE); otherwise " +
			"PUT /admin/settings does (API).",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Input:       input,
		Audit: usecase.AuditDeclaration{
			Action: instanceSettingsPlannedAction, TargetType: instanceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "the installation's configuration is not an entry's history."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			level, err := instanceLevelFrom(in)
			if err != nil {
				return nil, err
			}
			if h.Writer.Tenancy != env.TenancyMulti {
				delete(level.Legal.Locks, identity.LinkAccount)
			}
			plan, err := h.Execute(ctx, actor, level)
			if err != nil {
				return nil, err
			}
			changes := make([]usecase.Output, 0, len(plan.Changes))
			for _, c := range plan.Changes {
				changes = append(changes, usecase.Output{
					"key": c.Key, "from": c.From, "to": c.To, "from_locked": c.FromLocked, "to_locked": c.ToLocked,
				})
			}
			out := usecase.Output{
				"version": plan.Version, "changes": changes, "applied_through": plan.AppliedThrough, "source": nil,
			}
			if plan.Source != "" {
				out["source"] = plan.Source
			}
			return out, nil
		}),
	}
}
