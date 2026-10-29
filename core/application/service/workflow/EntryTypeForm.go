// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow

import (
	"context"
	"errors"
	"strconv"
	"strings"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

const (
	SetEntryTypeFormName = "SetEntryTypeForm"

	// EntryTypeFormSetAction is the stable audit code of a form's change (audit.md §2).
	EntryTypeFormSetAction audit.Action = "entry_type.form_set"
)

// FormDesk is what designing a form needs beyond the registry: the custom fields its keys name and
// the compiler its conditions are checked with, in the type's own environment.
type FormDesk struct {
	Fields   repository.CustomFields
	Compiler expression.Compiler
}

// SetEntryTypeFormCommand is the input, typed.
type SetEntryTypeFormCommand struct {
	Key             string
	Form            domain.Form
	ExpectedVersion int
}

// SetEntryTypeForm gives a type its form - the one layout and logic of every door that creates or
// edits an entry of the type (UC-FLW-01 check 2, D-13). The whole form is sent and replaces the
// one before; an empty one gives the type back no form.
type SetEntryTypeForm struct {
	Registry Registry
	Desk     FormDesk
}

// Execute checks the form - its shape in the domain, its custom fields in the workspace, each
// condition compiled in the type's environment - and stores it.
func (h SetEntryTypeForm) Execute(ctx context.Context, actor appshared.ActorContext, cmd SetEntryTypeFormCommand) (domain.EntryType, error) {
	if cmd.Key == "" {
		return domain.EntryType{}, shared.ErrValidation.WithDetail("entry_types.key_required")
	}
	if err := h.Registry.authorizeWrite(ctx, actor, EntryTypeFormSetAction); err != nil {
		return domain.EntryType{}, err
	}
	form, err := domain.NewForm(cmd.Form, h.Registry.Text)
	if err != nil {
		return domain.EntryType{}, err
	}
	var changed domain.EntryType
	err = h.Registry.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		registry, err := h.Registry.Read(ctx)
		if err != nil {
			return err
		}
		current, found := registry.Find(cmd.Key)
		if !found {
			return shared.ErrNotFound.WithDetail("entry_types.not_found")
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("entry_types.version_conflict")
		}
		if err := h.Desk.check(ctx, form); err != nil {
			return err
		}
		now := h.Registry.Clock.Now()
		next, changes := current.WithForm(form, now)
		changed = next
		if current.Stored() {
			if err := h.Registry.Types.Update(ctx, next, current.Version); err != nil {
				return err
			}
			changed.Version = current.Version + 1
		} else {
			changed.ID, changed.TenantID, changed.CreatedAt, changed.Version = h.Registry.IDs.NewID(), actor.TenantID, now, 1
			if err := h.Registry.Types.Insert(ctx, changed); err != nil {
				return err
			}
		}
		return h.Registry.record(ctx, actor, changed, changelog.Upsert, EntryTypeFormSetAction, changes, now)
	})
	if err != nil {
		return domain.EntryType{}, err
	}
	return changed, nil
}

// check refuses a custom field no collection of the workspace defines, and a condition that does
// not compile in the type's environment: the entry's own values and the form's custom fields,
// typed, so that a misspelt key is answered when it is written (domain-model.md §2, "Forms").
func (d FormDesk) check(ctx context.Context, form domain.Form) error {
	keys := form.CustomKeys()
	if len(keys) > 0 && d.Fields != nil {
		defined, err := d.Fields.KindsByKey(ctx, keys, shared.ID(""))
		if err != nil {
			return err
		}
		for i, field := range form.Fields {
			key, custom := strings.CutPrefix(field.Key, domain.FormCustomPrefix)
			if _, held := defined[key]; custom && !held {
				return shared.ErrValidation.WithDetail("forms.field_unknown").
					WithParams(map[string]string{"key": key}).
					WithFields(shared.FieldError{Path: "/form/fields/" + strconv.Itoa(i) + "/key", Code: "forms.field_unknown", Params: map[string]string{"key": key}})
			}
		}
	}
	if d.Compiler == nil {
		return shared.ErrInternal.WithDetail("forms.compiler_unwired")
	}
	environment := workservice.FormEnvironment(keys)
	for _, condition := range form.Expressions() {
		if _, err := d.Compiler.Compile(condition.Text, environment, expression.Boolean); err != nil {
			var coded *shared.Error
			code := "expression.syntax"
			if errors.As(err, &coded) && coded.DetailCode != "" {
				code = coded.DetailCode
			}
			return shared.ErrValidation.WithDetail(code).
				WithFields(shared.FieldError{Path: "/form" + condition.Path, Code: code})
		}
	}
	return nil
}

// Descriptor is the catalogue entry.
func (h SetEntryTypeForm) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetEntryTypeFormName,
		Summary: "Sets an entry type's form: its sections, its fields in order with required, read-only " +
			"and hidden conditions (CEL over entry, actor, channel, step), defaults, validations and " +
			"channels that may only narrow it. Every door that creates or edits an entry of the type " +
			"is held to it. The whole form replaces the one before; an empty form asks nothing.",
		SideEffects: "Writes the type, records a change for offline clients, and writes an audit entry.",
		TokenScope:  typesWrite,
		Module:      module.Workflows,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The type whose form this is."},
			{Name: "form", Kind: usecase.KindObject, Required: true, Description: "The form: sections, fields, validations, channels (schema EntryForm)."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for optimistic locking."},
		},
		Audit: usecase.AuditDeclaration{
			Action: EntryTypeFormSetAction, TargetType: EntryTypeTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a form is the workspace's configuration, not an entry."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h SetEntryTypeForm) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	form, err := FormFromInput(in["form"])
	if err != nil {
		return nil, err
	}
	changed, err := h.Execute(ctx, actor, SetEntryTypeFormCommand{
		Key: in.String("key"), Form: form, ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return EntryTypeOutput(changed), nil
}

// FormFromInput reads a form from the contract's document (schema EntryForm). A member of the
// wrong shape is refused naming it, never dropped.
func FormFromInput(raw any) (domain.Form, error) {
	document, ok := raw.(map[string]any)
	if raw == nil {
		return domain.Form{}, nil
	}
	if !ok {
		return domain.Form{}, malformedForm("/form")
	}
	var form domain.Form
	for i, entry := range listOf(document["sections"]) {
		member, ok := entry.(map[string]any)
		if !ok {
			return domain.Form{}, malformedForm("/form/sections/" + strconv.Itoa(i))
		}
		form.Sections = append(form.Sections, domain.FormSection{Key: textOf(member["key"]), Name: textOf(member["name"])})
	}
	for i, entry := range listOf(document["fields"]) {
		member, ok := entry.(map[string]any)
		if !ok {
			return domain.Form{}, malformedForm("/form/fields/" + strconv.Itoa(i))
		}
		required, _ := member["required"].(bool)
		form.Fields = append(form.Fields, domain.FormField{
			Key: textOf(member["key"]), Section: textOf(member["section"]), Required: required,
			RequiredWhen: textOf(member["required_when"]), ReadOnlyWhen: textOf(member["read_only_when"]),
			HiddenWhen: textOf(member["hidden_when"]), Default: member["default"], Help: textOf(member["help"]),
		})
	}
	for i, entry := range listOf(document["validations"]) {
		member, ok := entry.(map[string]any)
		if !ok {
			return domain.Form{}, malformedForm("/form/validations/" + strconv.Itoa(i))
		}
		form.Validations = append(form.Validations, domain.FormValidation{
			Key: textOf(member["key"]), Field: textOf(member["field"]), Holds: textOf(member["holds"]), Message: textOf(member["message"]),
		})
	}
	for i, entry := range listOf(document["channels"]) {
		member, ok := entry.(map[string]any)
		if !ok {
			return domain.Form{}, malformedForm("/form/channels/" + strconv.Itoa(i))
		}
		closed, _ := member["closed"].(bool)
		rule := domain.FormChannelRule{Channel: domain.FormChannel(textOf(member["channel"])), Closed: closed}
		for _, key := range listOf(member["hidden"]) {
			rule.Hidden = append(rule.Hidden, textOf(key))
		}
		if presets, ok := member["presets"].(map[string]any); ok {
			rule.Presets = presets
		}
		form.Channels = append(form.Channels, rule)
	}
	return form, nil
}

func listOf(raw any) []any {
	list, _ := raw.([]any)
	return list
}

func textOf(raw any) string {
	value, _ := raw.(string)
	return value
}

func malformedForm(path string) error {
	return shared.ErrValidation.WithDetail("forms.malformed").WithFields(shared.FieldError{Path: path, Code: "forms.malformed"})
}

// FormOutput is a form as every door answers it (schema EntryForm).
func FormOutput(form domain.Form) map[string]any {
	sections := make([]any, 0, len(form.Sections))
	for _, section := range form.Sections {
		sections = append(sections, map[string]any{"key": section.Key, "name": section.Name})
	}
	fields := make([]any, 0, len(form.Fields))
	for _, field := range form.Fields {
		fields = append(fields, map[string]any{
			"key": field.Key, "section": field.Section, "required": field.Required,
			"required_when": field.RequiredWhen, "read_only_when": field.ReadOnlyWhen,
			"hidden_when": field.HiddenWhen, "default": field.Default, "help": field.Help,
		})
	}
	validations := make([]any, 0, len(form.Validations))
	for _, validation := range form.Validations {
		validations = append(validations, map[string]any{
			"key": validation.Key, "field": validation.Field, "holds": validation.Holds, "message": validation.Message,
		})
	}
	channels := make([]any, 0, len(form.Channels))
	for _, rule := range form.Channels {
		hidden := rule.Hidden
		if hidden == nil {
			hidden = []string{}
		}
		presets := rule.Presets
		if presets == nil {
			presets = map[string]any{}
		}
		channels = append(channels, map[string]any{
			"channel": string(rule.Channel), "closed": rule.Closed, "hidden": hidden, "presets": presets,
		})
	}
	return map[string]any{"sections": sections, "fields": fields, "validations": validations, "channels": channels}
}
