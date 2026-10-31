// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"strconv"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListSlaPoliciesName = "ListSlaPolicies"
	DefineSlaPolicyName = "DefineSlaPolicy"
	DeleteSlaPolicyName = "DeleteSlaPolicy"
)

// GoalsFromInput reads a policy's goals from a door's input (schema SlaGoal).
func GoalsFromInput(raw any) ([]domain.SlaGoal, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, malformed("/goals", "sla_policies.goals_malformed")
	}
	goals := make([]domain.SlaGoal, 0, len(list))
	for i, entry := range list {
		at := "/goals/" + strconv.Itoa(i)
		member, ok := entry.(map[string]any)
		if !ok {
			return nil, malformed(at, "sla_policies.goals_malformed")
		}
		kind, _ := member["kind"].(string)
		goal := domain.SlaGoal{
			Kind: domain.GoalKind(kind), Minutes: intOf(member["minutes"]), WarnPercent: intOf(member["warn_percent"]),
		}
		if targets, ok := member["by_priority"].(map[string]any); ok {
			goal.ByPriority = make(map[string]int, len(targets))
			for priority, minutes := range targets {
				goal.ByPriority[priority] = intOf(minutes)
			}
		}
		if pauses, ok := member["pause_statuses"].([]any); ok {
			for _, status := range pauses {
				text, _ := status.(string)
				goal.PauseStatuses = append(goal.PauseStatuses, text)
			}
		}
		goals = append(goals, goal)
	}
	return goals, nil
}

func intOf(raw any) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}

// PolicyOutput is the shape every door returns (api/openapi.yaml, schema SlaPolicy).
func PolicyOutput(p domain.SlaPolicy) usecase.Output {
	goals := make([]usecase.Output, 0, len(p.Goals))
	for _, goal := range p.Goals {
		byPriority := usecase.Output{}
		for priority, minutes := range goal.ByPriority {
			byPriority[priority] = minutes
		}
		pauses := goal.PauseStatuses
		if pauses == nil {
			pauses = []string{}
		}
		goals = append(goals, usecase.Output{
			"kind": string(goal.Kind), "minutes": goal.Minutes, "by_priority": byPriority,
			"pause_statuses": pauses, "warn_percent": goal.WarnPercent,
		})
	}
	out := usecase.Output{
		"key": p.Key, "name": p.Name, "calendar": nil, "entry_types": p.EntryTypes,
		"organisations": keysOf(p.Organisations), "priority_field": nil, "goals": goals, "position": p.Position, "enabled": p.Enabled,
		"version": p.Version,
	}
	if p.CalendarKey != "" {
		out["calendar"] = p.CalendarKey
	}
	if p.PriorityField != "" {
		out["priority_field"] = p.PriorityField
	}
	return out
}

// ListSlaPolicies reads the workspace's SLA policies (UC-SVC-02).
type ListSlaPolicies struct{ Desk Desk }

// Execute answers every policy by position and key, to any member of the workspace.
func (h ListSlaPolicies) Execute(ctx context.Context, actor appshared.ActorContext) ([]domain.SlaPolicy, error) {
	if err := h.Desk.authorizeRead(ctx, actor, PoliciesReadAction, policyTarget); err != nil {
		return nil, err
	}
	var out []domain.SlaPolicy
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Desk.Policies.List(ctx)
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListSlaPolicies) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListSlaPoliciesName, Permission: service.PermissionRead,
		Summary: "Lists the workspace's SLA policies: the types each measures, its goals with targets per " +
			"priority, the statuses that pause them and the share at which they are at risk, and its calendar.",
		SideEffects: "None. Reads only.",
		TokenScope:  deskRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: PoliciesReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			policies, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(policies))
			for _, p := range policies {
				data = append(data, PolicyOutput(p))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, policyTarget)
}

// DefineSlaPolicyCommand is the input, typed.
type DefineSlaPolicyCommand struct {
	Key             string
	Name            string
	CalendarKey     string
	EntryTypes      []string
	Organisations   []string
	PriorityField   string
	Goals           []domain.SlaGoal
	Position        int
	Enabled         bool
	ExpectedVersion int
}

// DefineSlaPolicy creates a policy under a key, or replaces the one there (UC-SVC-02). A change
// measures what is armed after it; a clock already running keeps its target.
type DefineSlaPolicy struct{ Desk Desk }

// Execute defines the policy.
func (h DefineSlaPolicy) Execute(ctx context.Context, actor appshared.ActorContext, cmd DefineSlaPolicyCommand) (domain.SlaPolicy, error) {
	if err := h.Desk.authorizeWrite(ctx, actor, PolicyDefinedAction, policyTarget); err != nil {
		return domain.SlaPolicy{}, err
	}
	now := h.Desk.Clock.Now()
	input := domain.NewSlaPolicyInput{
		ID: h.Desk.IDs.NewID(), TenantID: actor.TenantID, Key: cmd.Key, Name: cmd.Name,
		CalendarKey: cmd.CalendarKey, EntryTypes: cmd.EntryTypes, Organisations: cmd.Organisations,
		PriorityField: cmd.PriorityField, Goals: cmd.Goals, Position: cmd.Position, Enabled: cmd.Enabled, Now: now, Text: h.Desk.Text,
	}
	var out domain.SlaPolicy
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Desk.calendarHeld(ctx, strings.TrimSpace(cmd.CalendarKey)); err != nil {
			return err
		}
		if err := h.Desk.organisationsHeld(ctx, cmd.Organisations); err != nil {
			return err
		}
		current, err := h.Desk.Policies.Find(ctx, strings.TrimSpace(cmd.Key))
		switch {
		case errors.Is(err, shared.ErrNotFound):
			if out, err = domain.NewSlaPolicy(input); err != nil {
				return err
			}
			if err := h.Desk.Policies.Insert(ctx, out); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
				return shared.ErrVersionConflict.WithDetail("sla_policies.version_conflict")
			}
			if out, err = current.Change(input); err != nil {
				return err
			}
			if err := h.Desk.Policies.Update(ctx, out, current.Version); err != nil {
				return err
			}
			out.Version = current.Version + 1
		}
		return h.Desk.record(ctx, actor, PolicyDefinedAction, policyTarget, out.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, To: out.Key},
			audit.Change{Field: "name", Classification: audit.Sensitive, To: out.Name},
			audit.Change{Field: "calendar", Classification: audit.Open, To: out.CalendarKey},
			audit.Change{Field: "entry_types", Classification: audit.Open, To: strings.Join(out.EntryTypes, ",")},
			audit.Change{Field: "organisations", Classification: audit.Open, To: strings.Join(out.Organisations, ",")},
			audit.Change{Field: "enabled", Classification: audit.Open, To: strconv.FormatBool(out.Enabled)})
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h DefineSlaPolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineSlaPolicyName,
		Summary: "Creates an SLA policy under a key, or replaces the one there: the entry types it measures, a " +
			"first-response and a resolution goal with targets in working minutes per priority, the statuses " +
			"that pause a goal, the share of the target at which it is at risk, and the business calendar its " +
			"time is counted on - around the clock when none is named. An entry is measured by the first " +
			"enabled policy, by position, that covers its type and, where it names organisations, the requester's.",
		SideEffects: "Writes the policy and an audit entry. Entries already measured keep their clocks.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Small letters, digits and underscores, starting with a letter."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
			{Name: "calendar", Kind: usecase.KindString, Description: "A business calendar's key; absent counts around the clock."},
			{Name: "entry_types", Kind: usecase.KindList, Required: true, Description: "The entry type keys the policy measures."},
			{Name: "organisations", Kind: usecase.KindList, Description: "The requester organisations' keys whose requests the policy measures; absent or empty measures every requester's."},
			{Name: "priority_field", Kind: usecase.KindString, Description: "The custom field whose option value picks a goal's target."},
			{Name: "goals", Kind: usecase.KindList, Required: true, Description: "{kind: FIRST_RESPONSE, NEXT_RESPONSE or RESOLUTION, minutes, by_priority, pause_statuses, warn_percent}, each kind once."},
			{Name: "position", Kind: usecase.KindInt, Description: "The order in which policies are tried; lower first."},
			{Name: "enabled", Kind: usecase.KindBool, Description: "Whether the policy measures new entries; true when absent."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; a replacement of a newer one is refused."},
		},
		Audit: usecase.AuditDeclaration{Action: PolicyDefinedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			types, err := in.StringList("entry_types")
			if err != nil {
				return nil, err
			}
			organisations, err := in.StringList("organisations")
			if err != nil {
				return nil, err
			}
			goals, err := GoalsFromInput(in["goals"])
			if err != nil {
				return nil, err
			}
			enabled := true
			if in.Present("enabled") {
				enabled = in.Bool("enabled")
			}
			defined, err := h.Execute(ctx, actor, DefineSlaPolicyCommand{
				Key: in.String("key"), Name: in.String("name"), CalendarKey: in.String("calendar"),
				EntryTypes: types, Organisations: organisations, PriorityField: in.String("priority_field"), Goals: goals,
				Position: in.Int("position"), Enabled: enabled, ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return PolicyOutput(defined), nil
		}),
	}, policyTarget)
}

// DeleteSlaPolicy removes a policy (UC-SVC-02). The entries it measured keep their records; their
// clocks stop being moved by it.
type DeleteSlaPolicy struct{ Desk Desk }

// Execute deletes the policy.
func (h DeleteSlaPolicy) Execute(ctx context.Context, actor appshared.ActorContext, key string) error {
	if err := h.Desk.authorizeWrite(ctx, actor, PolicyDeletedAction, policyTarget); err != nil {
		return err
	}
	now := h.Desk.Clock.Now()
	return h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Desk.Policies.Find(ctx, key)
		if err != nil {
			return err
		}
		if err := h.Desk.Policies.Delete(ctx, key); err != nil {
			return err
		}
		return h.Desk.record(ctx, actor, PolicyDeletedAction, policyTarget, current.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, From: current.Key})
	})
}

// Descriptor is the catalogue entry.
func (h DeleteSlaPolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name:        DeleteSlaPolicyName,
		Summary:     "Deletes an SLA policy. The entries it measured keep the record of how their goals stood.",
		SideEffects: "Deletes the policy and writes an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The policy's key."},
		},
		Audit: usecase.AuditDeclaration{Action: PolicyDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Execute(ctx, actor, in.String("key")); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}, policyTarget)
}

// calendarHeld refuses a policy's calendar that neither the workspace nor the installation holds
// (UC-SVC-01 check 6); no key counts around the clock.
// organisationsHeld refuses a policy for an organisation the workspace does not hold: it would
// measure nobody's requests without saying so.
func (d Desk) organisationsHeld(ctx context.Context, keys []string) error {
	for i, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || d.Organisations == nil {
			continue
		}
		if _, err := d.Organisations.Find(ctx, key); errors.Is(err, shared.ErrNotFound) {
			at := "/organisations/" + strconv.Itoa(i)
			return shared.ErrValidation.WithDetail("sla_policies.organisation_unknown").
				WithFields(shared.FieldError{Path: at, Code: "sla_policies.organisation_unknown"})
		} else if err != nil {
			return err
		}
	}
	return nil
}

// keysOf answers keys for an output, never null.
func keysOf(keys []string) []string {
	if keys == nil {
		return []string{}
	}
	return keys
}

func (d Desk) calendarHeld(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	if _, offered := d.Offered[key]; offered {
		return nil
	}
	_, err := d.Calendars.Find(ctx, key)
	if errors.Is(err, shared.ErrNotFound) {
		return shared.ErrValidation.WithDetail("sla_policies.calendar_unknown").
			WithFields(shared.FieldError{Path: "/calendar", Code: "sla_policies.calendar_unknown"})
	}
	return err
}
