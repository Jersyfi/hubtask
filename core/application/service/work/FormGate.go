// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

// FormRoles answers the role the actor holds along a path, for a form's `actor.role`. The
// authorisation service's RoleAlong; inside a writer's transaction it joins it.
type FormRoles interface {
	RoleAlong(ctx context.Context, actor appshared.ActorContext, path []identity.Scope) (identity.Role, bool, error)
	// HeldAlong answers every role key (built-in and custom) and every permission the actor
	// holds along a path, for a step limited by role and a field or step permission.
	HeldAlong(ctx context.Context, actor appshared.ActorContext, path []identity.Scope) (access.Held, error)
}

// FormGate holds every write of an entry to its type's form (P-18, D-13): asked inside each
// writing use case's transaction with the entry as it would be written, so that REST, MCP, hubctl,
// a rule, a device's push, an import and mail meet the same refusal from one place. A build
// without it - a test of something else, a type without a form - asks nothing.
type FormGate struct {
	Types    repository.EntryTypes
	Fields   repository.CustomFields
	Compiler expression.Compiler
	// Roles is optional: without it `actor.role` is empty.
	Roles FormRoles
}

// Wired reports whether the gate has what it needs.
func (g FormGate) Wired() bool { return g.Types != nil && g.Compiler != nil }

// FormChannelOf is the door an act arrived through: what its adapter said, a rule's run where the
// rule engine acts, the REST door otherwise.
func FormChannelOf(actor appshared.ActorContext) domain.FormChannel {
	if !actor.RunsForRule.IsZero() {
		return domain.ChannelRule
	}
	if channel := domain.FormChannel(actor.Channel); channel.Valid() {
		return channel
	}
	return domain.ChannelApp
}

// typeOf answers the entry's type as the registry holds it, with its form.
func (g FormGate) typeOf(ctx context.Context, item domain.WorkItem) (domain.EntryType, error) {
	rows, err := g.Types.List(ctx)
	if err != nil {
		return domain.EntryType{}, err
	}
	return domain.NewEntryTypes(rows).Of(item.Type, item.EntryType), nil
}

// formCheck is one answered form: the type, its definitions in force and how to answer it.
type formCheck struct {
	gate       FormGate
	actor      appshared.ActorContext
	collection domain.Container
	entryType  domain.EntryType
	keys       []string
	channel    domain.FormChannel
	role       *string
	programs   map[string]expression.Program
	// step is the workflow step the write takes, empty for none: what a form's `step` reads.
	step string
}

func (g FormGate) open(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, item domain.WorkItem,
) (*formCheck, error) {
	if !g.Wired() {
		return nil, nil
	}
	entryType, err := g.typeOf(ctx, item)
	if err != nil || entryType.Form.IsEmpty() {
		return nil, err
	}
	keys := entryType.Form.CustomKeys()
	if g.Fields != nil {
		definitions, err := g.Fields.ListInScope(ctx, collection.ID)
		if err != nil {
			return nil, err
		}
		for _, definition := range definitions {
			if !slices.Contains(keys, definition.Key) {
				keys = append(keys, definition.Key)
			}
		}
	}
	return &formCheck{
		gate: g, actor: actor, collection: collection, entryType: entryType, keys: keys,
		channel: FormChannelOf(actor), programs: map[string]expression.Program{},
	}, nil
}

// state answers the form for an entry.
func (c *formCheck) state(ctx context.Context, item domain.WorkItem) (domain.FormState, error) {
	activation, err := c.activation(ctx, item)
	if err != nil {
		return domain.FormState{}, err
	}
	return c.entryType.Form.State(c.channel, func(text string) (bool, error) {
		program, compiled := c.programs[text]
		if !compiled {
			// Open over the custom fields at run time: the form was typed when it was saved, and a
			// definition deleted since must not make every write of the type fail to compile.
			program, err = c.gate.Compiler.Compile(text, FormEnvironment(nil), expression.Boolean)
			if err != nil {
				return false, shared.ErrConflict.WithDetail("forms.condition_broken").
					WithParams(map[string]string{"entry_type": c.entryType.Key})
			}
			c.programs[text] = program
		}
		value, err := program.Evaluate(ctx, activation)
		if err != nil {
			return false, err
		}
		return value.Bool, nil
	})
}

func (c *formCheck) activation(ctx context.Context, item domain.WorkItem) (formActivation, error) {
	custom := make(map[string]any, len(c.keys))
	for _, key := range c.keys {
		custom[key] = item.CustomFields[key]
	}
	entry := map[string]any{
		"title": item.Title, "notes": item.Notes, "level": string(item.Type),
		"entry_type": c.entryType.Key, "completed": item.Completion.IsCompleted,
		"due_date":      domain.FormValue(item, domain.FormKeyDueDate),
		"start_at":      domain.FormValue(item, domain.FormKeyStartAt),
		"custom_fields": custom,
	}
	role, err := c.roleFor(ctx)
	if err != nil {
		return formActivation{}, err
	}
	return formActivation{values: map[string]any{
		"entry":   entry,
		"actor":   map[string]any{"role": role, "kind": string(c.actor.Kind)},
		"channel": string(c.channel),
		"step":    c.step,
	}}, nil
}

// roleFor reads the actor's role only where a condition of the form asks for it.
func (c *formCheck) roleFor(ctx context.Context) (string, error) {
	if c.role != nil {
		return *c.role, nil
	}
	role := ""
	asks := false
	for _, condition := range c.entryType.Form.Expressions() {
		asks = asks || strings.Contains(condition.Text, "actor")
	}
	if asks && c.gate.Roles != nil {
		held, found, err := c.gate.Roles.RoleAlong(ctx, c.actor, containerPath(c.collection))
		if err != nil {
			return "", err
		}
		if found {
			role = string(held)
		}
	}
	c.role = &role
	return role, nil
}

type formActivation struct{ values map[string]any }

func (a formActivation) Resolve(_ context.Context, name string) (any, bool, error) {
	value, found := a.values[name]
	return value, found, nil
}

// checkCreate holds a new entry, with every value the request carried already on it, to its form:
// a channel the form closes, a required field left empty, a validation that does not hold.
func (g FormGate) checkCreate(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, created domain.WorkItem,
) error {
	check, err := g.open(ctx, actor, collection, created)
	if err != nil || check == nil {
		return err
	}
	state, err := check.state(ctx, created)
	if err != nil {
		return err
	}
	if err := domain.RefuseClosedChannel(state, check.entryType.Key, check.channel); err != nil {
		return err
	}
	return domain.CheckFormWrite(check.entryType.Form, nil, domain.FormState{}, created, state)
}

// checkEdit holds a change of an entry to its form: what was read-only or hidden as it stood is
// not changed, and the change leaves it no further from the form than it was.
func (g FormGate) checkEdit(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, before, after domain.WorkItem,
) error {
	check, err := g.open(ctx, actor, collection, after)
	if err != nil || check == nil {
		return err
	}
	was, err := check.state(ctx, before)
	if err != nil {
		return err
	}
	now, err := check.state(ctx, after)
	if err != nil {
		return err
	}
	return domain.CheckFormWrite(check.entryType.Form, &before, was, after, now)
}

// checkStep holds an entry taking a step to what its form asks of that step (domain.CheckFormStep):
// the form answered once without the step and once with it.
func (g FormGate) checkStep(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, item domain.WorkItem, step string,
) error {
	check, err := g.open(ctx, actor, collection, item)
	if err != nil || check == nil {
		return err
	}
	without, err := check.state(ctx, item)
	if err != nil {
		return err
	}
	check.step = step
	with, err := check.state(ctx, item)
	if err != nil {
		return err
	}
	return domain.CheckFormStep(check.entryType.Form, item, without, with)
}

// starting answers the presets and defaults a new entry takes for the custom fields the caller
// sent nothing for, limited to the fields in force where it is created.
func (g FormGate) starting(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, item domain.WorkItem,
	sent map[string]any,
) (map[string]any, error) {
	check, err := g.open(ctx, actor, collection, item)
	if err != nil || check == nil {
		return nil, err
	}
	starting := check.entryType.Form.Starting(check.channel, sent)
	for key := range starting {
		if !slices.Contains(check.keys, key) {
			delete(starting, key)
		}
	}
	return starting, nil
}

// Evaluate answers the form for a draft of an entry - the state an editor renders before it
// sends anything (UC-FLW-01 check 2). Nil where the type has no form.
func (g FormGate) Evaluate(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, draft domain.WorkItem,
) (*domain.FormState, domain.EntryType, error) {
	check, err := g.open(ctx, actor, collection, draft)
	if err != nil || check == nil {
		return nil, domain.EntryType{}, err
	}
	state, err := check.state(ctx, draft)
	if err != nil {
		return nil, domain.EntryType{}, err
	}
	return &state, check.entryType, nil
}

// FormEnvironment is a form's CEL environment (domain-model.md §2, "Forms"): the entry with its own
// values and its custom fields, the actor's role and kind, the channel and the step. Typed by the
// custom field keys given; nil keeps the custom fields open.
func FormEnvironment(customKeys []string) expression.Environment {
	var custom []expression.Variable
	for _, key := range customKeys {
		custom = append(custom, expression.Variable{Name: key, Kind: expression.KindString})
	}
	customField := expression.Variable{Name: "custom_fields", Kind: expression.KindMap, Fields: custom}
	if customKeys != nil && len(custom) == 0 {
		// A typed environment with no custom field: any selection of one is a mistake.
		customField.Fields = []expression.Variable{{Name: "\x00", Kind: expression.KindString}}
	}
	return expression.Environment{Variables: []expression.Variable{
		{Name: "entry", Kind: expression.KindMap, Description: "The entry as it would be written.", Fields: []expression.Variable{
			{Name: "title", Kind: expression.KindString}, {Name: "notes", Kind: expression.KindString},
			{Name: "level", Kind: expression.KindString}, {Name: "entry_type", Kind: expression.KindString},
			{Name: "completed", Kind: expression.KindBool}, {Name: "due_date", Kind: expression.KindString},
			{Name: "start_at", Kind: expression.KindString}, customField,
		}},
		{Name: "actor", Kind: expression.KindMap, Description: "Who writes: role, kind.", Fields: []expression.Variable{
			{Name: "role", Kind: expression.KindString}, {Name: "kind", Kind: expression.KindString},
		}},
		{Name: "channel", Kind: expression.KindString, Description: "The door: APP, MCP, RULE, SYNC, IMPORT, MAIL."},
		{Name: "step", Kind: expression.KindString, Description: "The workflow step the write takes, empty for none."},
	}}
}
