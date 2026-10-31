// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// FormChannel is a door an entry arrives through, as the server tells them apart (domain-model.md
// §2.4). The web app's capture and panel and hubctl all speak REST, so they are one door: a
// channel the server could not tell apart from another would be a narrowing anybody could step
// around by claiming the wider one.
type FormChannel string

const (
	ChannelApp    FormChannel = "APP"
	ChannelMCP    FormChannel = "MCP"
	ChannelRule   FormChannel = "RULE"
	ChannelSync   FormChannel = "SYNC"
	ChannelImport FormChannel = "IMPORT"
	ChannelMail   FormChannel = "MAIL"
	// ChannelPortal is the requester portal's catalogue (UC-SVC-05): a type the portal offers is
	// one whose form leaves this door open.
	ChannelPortal FormChannel = "PORTAL"
)

var formChannels = [...]FormChannel{
	ChannelApp, ChannelMCP, ChannelRule, ChannelSync, ChannelImport, ChannelMail, ChannelPortal,
}

// FormChannels answers every channel, in a fixed order.
func FormChannels() []FormChannel { return formChannels[:] }

// Valid reports whether the channel is one of the closed set.
func (c FormChannel) Valid() bool { return slices.Contains(formChannels[:], c) }

// The keys a form field may name besides a custom field's: the entry's own values that an editor
// shows as fields.
const (
	FormKeyTitle   = "title"
	FormKeyNotes   = "notes"
	FormKeyDueDate = "due_date"
	FormKeyStartAt = "start_at"
	// FormCustomPrefix starts a custom field's key: `custom_fields.severity`, the query language's
	// spelling too.
	FormCustomPrefix = "custom_fields."
)

var formCoreKeys = []string{FormKeyTitle, FormKeyNotes, FormKeyDueDate, FormKeyStartAt}

// Bounds of a form. Generous for a form somebody designs, and far short of a document somebody
// pastes.
const (
	MaxFormSections    = 20
	MaxFormFields      = MaxCustomFieldsPerItem + 4
	MaxFormValidations = 50
	MaxFormHelpLength  = 500
	// MaxFormExpressionLength is the expression port's MaxLength.
	MaxFormExpressionLength = 4096
)

var formKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// Form is a type's one layout and logic (P-18): which fields an editor shows, in which section and
// order, which are required, read-only or hidden and when, what they start with, and what must
// hold. Every door that creates or edits an entry of the type is held to it by the server.
//
// A condition is CEL over the entry, the actor's role, the channel and the step, in the type's own
// environment; the domain keeps its text, the application compiles and answers it.
type Form struct {
	Sections    []FormSection
	Fields      []FormField
	Validations []FormValidation
	Channels    []FormChannelRule
}

// FormSection groups fields under a heading, in the slice's order. Name is a person's words.
type FormSection struct {
	Key  string
	Name string
}

// FormField is one field of the form, in the slice's order.
type FormField struct {
	Key     string
	Section string
	// Required always; RequiredWhen adds a condition under which it is.
	Required     bool
	RequiredWhen string
	ReadOnlyWhen string
	HiddenWhen   string
	// Default is what a new entry starts with where the caller sends nothing; only a custom field
	// takes one, judged by its definition when it is written.
	Default any
	// Help is a person's words shown under the field.
	Help string
}

// FormValidation is a condition that must hold, with the person's words shown when it does not.
// The refusal names its key, never the words (rule 8, rule 10).
type FormValidation struct {
	Key     string
	Field   string
	Holds   string
	Message string
}

// FormChannelRule narrows the form for one channel (model.md §6): it may close the channel for
// creating the type, hide fields and preset them, and never add a field or a rule of its own.
type FormChannelRule struct {
	Channel FormChannel
	// Closed refuses creating an entry of the type through the channel.
	Closed  bool
	Hidden  []string
	Presets map[string]any
}

// IsEmpty reports whether the form asks for nothing: what every type has until somebody designs one.
func (f Form) IsEmpty() bool {
	return len(f.Sections) == 0 && len(f.Fields) == 0 && len(f.Validations) == 0 && len(f.Channels) == 0
}

// Field answers the form's field of a key.
func (f Form) Field(key string) (FormField, bool) {
	for _, field := range f.Fields {
		if field.Key == key {
			return field, true
		}
	}
	return FormField{}, false
}

// Channel answers the narrowing of a channel; a channel without one narrows nothing.
func (f Form) Channel(channel FormChannel) FormChannelRule {
	for _, rule := range f.Channels {
		if rule.Channel == channel {
			return rule
		}
	}
	return FormChannelRule{Channel: channel}
}

// CustomKeys answers the custom field keys the form names, in its order.
func (f Form) CustomKeys() []string {
	keys := []string{}
	for _, field := range f.Fields {
		if key, custom := strings.CutPrefix(field.Key, FormCustomPrefix); custom {
			keys = append(keys, key)
		}
	}
	return keys
}

// Expressions answers every condition the form holds with the path a refusal of it names, so that
// the application compiles each once and points a refusal at the one that failed.
func (f Form) Expressions() []FormExpression {
	var out []FormExpression
	add := func(path, expression string) {
		if strings.TrimSpace(expression) != "" {
			out = append(out, FormExpression{Path: path, Text: expression})
		}
	}
	for i, field := range f.Fields {
		at := "/fields/" + strconv.Itoa(i)
		add(at+"/required_when", field.RequiredWhen)
		add(at+"/read_only_when", field.ReadOnlyWhen)
		add(at+"/hidden_when", field.HiddenWhen)
	}
	for i, validation := range f.Validations {
		add("/validations/"+strconv.Itoa(i)+"/holds", validation.Holds)
	}
	return out
}

// FormExpression is one condition of a form and where it sits.
type FormExpression struct {
	Path string
	Text string
}

// NewForm checks a form as a person designed it and brings its words to normal form C. Whether a
// custom field exists and whether a condition compiles are the application's questions: the one
// reads the registry, the other the expression port.
func NewForm(in Form, form text.Normalizer) (Form, error) {
	if len(in.Sections) > MaxFormSections {
		return Form{}, formInvalid("/sections", "forms.too_many_sections", map[string]string{"maximum": strconv.Itoa(MaxFormSections)})
	}
	if len(in.Fields) > MaxFormFields {
		return Form{}, formInvalid("/fields", "forms.too_many_fields", map[string]string{"maximum": strconv.Itoa(MaxFormFields)})
	}
	if len(in.Validations) > MaxFormValidations {
		return Form{}, formInvalid("/validations", "forms.too_many_validations", map[string]string{"maximum": strconv.Itoa(MaxFormValidations)})
	}
	out := Form{}
	sections := []string{}
	for i, section := range in.Sections {
		at := "/sections/" + strconv.Itoa(i)
		key := strings.TrimSpace(section.Key)
		if !formKeyPattern.MatchString(key) {
			return Form{}, formInvalid(at+"/key", "forms.key_malformed", nil)
		}
		if slices.Contains(sections, key) {
			return Form{}, formInvalid(at+"/key", "forms.key_taken", map[string]string{"key": key})
		}
		name, err := structureName(section.Name, formSectionNameCodes, form)
		if err != nil {
			return Form{}, atFormPath(err, at+"/name")
		}
		sections = append(sections, key)
		out.Sections = append(out.Sections, FormSection{Key: key, Name: name})
	}
	for i, field := range in.Fields {
		checked, err := newFormField(field, sections, out.Fields, form, "/fields/"+strconv.Itoa(i))
		if err != nil {
			return Form{}, err
		}
		out.Fields = append(out.Fields, checked)
	}
	for i, validation := range in.Validations {
		checked, err := newFormValidation(validation, out, form, "/validations/"+strconv.Itoa(i))
		if err != nil {
			return Form{}, err
		}
		out.Validations = append(out.Validations, checked)
	}
	for i, rule := range in.Channels {
		checked, err := newFormChannelRule(rule, out, "/channels/"+strconv.Itoa(i))
		if err != nil {
			return Form{}, err
		}
		out.Channels = append(out.Channels, checked)
	}
	return out, nil
}

var formSectionNameCodes = nameCodes{
	empty: "forms.section_name_empty", tooLong: "forms.section_name_too_long", malformed: "forms.section_name_malformed",
}

func newFormField(field FormField, sections []string, before []FormField, form text.Normalizer, at string) (FormField, error) {
	field.Key = strings.TrimSpace(field.Key)
	if !validFormFieldKey(field.Key) {
		return FormField{}, formInvalid(at+"/key", "forms.field_key_unknown", map[string]string{"key": field.Key})
	}
	for _, other := range before {
		if other.Key == field.Key {
			return FormField{}, formInvalid(at+"/key", "forms.key_taken", map[string]string{"key": field.Key})
		}
	}
	if field.Section != "" && !slices.Contains(sections, field.Section) {
		return FormField{}, formInvalid(at+"/section", "forms.section_unknown", map[string]string{"section": field.Section})
	}
	if field.Key == FormKeyTitle && strings.TrimSpace(field.HiddenWhen) != "" {
		// Every entry has a title, and a form that hid it would make an entry nobody can name.
		return FormField{}, formInvalid(at+"/hidden_when", "forms.title_not_hideable", nil)
	}
	for path, expression := range map[string]string{
		"/required_when": field.RequiredWhen, "/read_only_when": field.ReadOnlyWhen, "/hidden_when": field.HiddenWhen,
	} {
		if len(expression) > MaxFormExpressionLength {
			return FormField{}, formInvalid(at+path, "expression.too_long", nil)
		}
	}
	if field.Default != nil && !strings.HasPrefix(field.Key, FormCustomPrefix) {
		return FormField{}, formInvalid(at+"/default", "forms.default_not_custom", nil)
	}
	help, err := formHelp(field.Help, form, at+"/help")
	if err != nil {
		return FormField{}, err
	}
	field.Help = help
	return field, nil
}

func newFormValidation(validation FormValidation, f Form, form text.Normalizer, at string) (FormValidation, error) {
	validation.Key = strings.TrimSpace(validation.Key)
	if !formKeyPattern.MatchString(validation.Key) {
		return FormValidation{}, formInvalid(at+"/key", "forms.key_malformed", nil)
	}
	for _, other := range f.Validations {
		if other.Key == validation.Key {
			return FormValidation{}, formInvalid(at+"/key", "forms.key_taken", map[string]string{"key": validation.Key})
		}
	}
	if validation.Field != "" {
		if _, found := f.Field(validation.Field); !found {
			return FormValidation{}, formInvalid(at+"/field", "forms.field_not_in_form", map[string]string{"key": validation.Field})
		}
	}
	if strings.TrimSpace(validation.Holds) == "" {
		return FormValidation{}, formInvalid(at+"/holds", "forms.validation_empty", nil)
	}
	if len(validation.Holds) > MaxFormExpressionLength {
		return FormValidation{}, formInvalid(at+"/holds", "expression.too_long", nil)
	}
	message, err := formHelp(validation.Message, form, at+"/message")
	if err != nil {
		return FormValidation{}, err
	}
	validation.Message = message
	return validation, nil
}

// newFormChannelRule holds a channel to narrowing: what it hides or presets is a field of the
// form, and a required field it hides it also fills, or nothing could create the entry through it.
func newFormChannelRule(rule FormChannelRule, f Form, at string) (FormChannelRule, error) {
	if !rule.Channel.Valid() {
		return FormChannelRule{}, formInvalid(at+"/channel", "forms.channel_unknown", map[string]string{"channel": string(rule.Channel)})
	}
	for _, other := range f.Channels {
		if other.Channel == rule.Channel {
			return FormChannelRule{}, formInvalid(at+"/channel", "forms.key_taken", map[string]string{"key": string(rule.Channel)})
		}
	}
	hidden := []string{}
	for _, key := range rule.Hidden {
		field, found := f.Field(key)
		if !found {
			return FormChannelRule{}, formInvalid(at+"/hidden", "forms.field_not_in_form", map[string]string{"key": key})
		}
		if key == FormKeyTitle {
			return FormChannelRule{}, formInvalid(at+"/hidden", "forms.title_not_hideable", nil)
		}
		if _, preset := rule.Presets[key]; field.Required && !preset && field.Default == nil {
			return FormChannelRule{}, formInvalid(at+"/hidden", "forms.required_hidden_unfilled", map[string]string{"key": key})
		}
		if !slices.Contains(hidden, key) {
			hidden = append(hidden, key)
		}
	}
	presets := map[string]any{}
	for key, value := range rule.Presets {
		if _, found := f.Field(key); !found || !strings.HasPrefix(key, FormCustomPrefix) {
			return FormChannelRule{}, formInvalid(at+"/presets/"+key, "forms.field_not_in_form", map[string]string{"key": key})
		}
		presets[key] = value
	}
	rule.Hidden, rule.Presets = hidden, presets
	return rule, nil
}

func validFormFieldKey(key string) bool {
	if slices.Contains(formCoreKeys, key) {
		return true
	}
	custom, found := strings.CutPrefix(key, FormCustomPrefix)
	return found && validCustomFieldKey(custom)
}

func formHelp(raw string, form text.Normalizer, at string) (string, error) {
	help, err := shared.NFC(strings.TrimSpace(raw), form)
	if err != nil {
		return "", err
	}
	if len([]rune(help)) > MaxFormHelpLength {
		return "", formInvalid(at, "forms.help_too_long", map[string]string{"maximum": strconv.Itoa(MaxFormHelpLength)})
	}
	return help, nil
}

func formInvalid(path, code string, params map[string]string) error {
	return shared.ErrValidation.WithDetail(code).WithParams(params).
		WithFields(shared.FieldError{Path: path, Code: code, Params: params})
}

func atFormPath(err error, path string) error {
	var coded *shared.Error
	if !errors.As(err, &coded) {
		return err
	}
	return shared.ErrValidation.WithDetail(coded.DetailCode).WithParams(coded.Params).
		WithFields(shared.FieldError{Path: path, Code: coded.DetailCode, Params: coded.Params})
}

// FormFieldState is how the form stands for one field of one entry, through one channel.
type FormFieldState struct {
	Required bool
	ReadOnly bool
	Hidden   bool
}

// FormState is the form answered for one entry: each field's state and the validations that do
// not hold. The application answers the conditions; State puts them together.
type FormState struct {
	Fields map[string]FormFieldState
	Failed []string
	Closed bool
}

// FormAnswer answers one condition of the form - true where its text is empty is never asked.
type FormAnswer func(expression string) (bool, error)

// State answers the form for an entry through a channel. A field the channel hides is hidden; a
// hidden field is never required, because nobody could fill it; a validation of a hidden field is
// not asked, for the same reason.
func (f Form) State(channel FormChannel, answer FormAnswer) (FormState, error) {
	rule := f.Channel(channel)
	state := FormState{Fields: make(map[string]FormFieldState, len(f.Fields)), Closed: rule.Closed}
	ask := func(expression string) (bool, error) {
		if strings.TrimSpace(expression) == "" {
			return false, nil
		}
		return answer(expression)
	}
	for _, field := range f.Fields {
		hidden, err := ask(field.HiddenWhen)
		if err != nil {
			return FormState{}, err
		}
		hidden = hidden || slices.Contains(rule.Hidden, field.Key)
		readOnly, err := ask(field.ReadOnlyWhen)
		if err != nil {
			return FormState{}, err
		}
		required := field.Required
		if !required {
			if required, err = ask(field.RequiredWhen); err != nil {
				return FormState{}, err
			}
		}
		state.Fields[field.Key] = FormFieldState{Required: required && !hidden, ReadOnly: readOnly, Hidden: hidden}
	}
	for _, validation := range f.Validations {
		if validation.Field != "" && state.Fields[validation.Field].Hidden {
			continue
		}
		holds, err := answer(validation.Holds)
		if err != nil {
			return FormState{}, err
		}
		if !holds {
			state.Failed = append(state.Failed, validation.Key)
		}
	}
	return state, nil
}

// Starting answers what a new entry starts with through a channel: the channel's presets, then the
// fields' defaults, for each custom field the caller sent nothing for. A field the channel hides
// takes its preset whatever was sent: nobody on that channel could have seen it. Keyed by the
// custom key.
func (f Form) Starting(channel FormChannel, sent map[string]any) map[string]any {
	rule := f.Channel(channel)
	out := map[string]any{}
	for _, field := range f.Fields {
		key, custom := strings.CutPrefix(field.Key, FormCustomPrefix)
		if !custom {
			continue
		}
		_, preset := rule.Presets[field.Key]
		if _, given := sent[key]; given && (!preset || !slices.Contains(rule.Hidden, field.Key)) {
			continue
		}
		if preset, found := rule.Presets[field.Key]; found {
			out[key] = preset
		} else if field.Default != nil {
			out[key] = field.Default
		}
	}
	return out
}

// FormValue answers the value of a form key on an entry, nil for none.
func FormValue(item WorkItem, key string) any {
	switch key {
	case FormKeyTitle:
		return item.Title
	case FormKeyNotes:
		return item.Notes
	case FormKeyDueDate:
		if item.Due == nil {
			return nil
		}
		return item.Due.At.UTC().Format(time.RFC3339)
	case FormKeyStartAt:
		if item.StartAt == nil {
			return nil
		}
		return item.StartAt.UTC().Format(time.RFC3339)
	}
	if custom, found := strings.CutPrefix(key, FormCustomPrefix); found {
		return item.CustomFields[custom]
	}
	return nil
}

// CheckFormWrite refuses a write that moves an entry further from its form (P-18): a required
// field left empty that was filled before - on a creation, any required field empty -, a
// validation failing that held before, and a change to a field that was read-only or hidden as the
// entry stood. An entry created before its form is not refused for what it already lacked: only
// the write that makes it worse is.
//
// before is nil on a creation; its state is then the empty one.
func CheckFormWrite(f Form, beforeItem *WorkItem, before FormState, after WorkItem, afterState FormState) error {
	var problems []shared.FieldError
	for _, field := range f.Fields {
		path := "/" + strings.ReplaceAll(field.Key, ".", "/")
		now := FormValue(after, field.Key)
		if beforeItem != nil {
			was := FormValue(*beforeItem, field.Key)
			if !sameFormValue(was, now) {
				if before.Fields[field.Key].Hidden {
					problems = append(problems, formProblem(path, "forms.field_hidden", field.Key))
					continue
				}
				if before.Fields[field.Key].ReadOnly {
					problems = append(problems, formProblem(path, "forms.field_read_only", field.Key))
					continue
				}
			}
			if afterState.Fields[field.Key].Required && !hasValue(now) && hasValue(was) {
				problems = append(problems, formProblem(path, "fields.value_required", field.Key))
			}
			continue
		}
		if afterState.Fields[field.Key].Required && !hasValue(now) {
			problems = append(problems, formProblem(path, "fields.value_required", field.Key))
		}
	}
	for _, key := range afterState.Failed {
		if slices.Contains(before.Failed, key) {
			continue
		}
		validation := f.validation(key)
		path := ""
		if validation.Field != "" {
			path = "/" + strings.ReplaceAll(validation.Field, ".", "/")
		}
		problems = append(problems, shared.FieldError{
			Path: path, Code: "forms.validation_failed", Params: map[string]string{"validation": key},
		})
	}
	if len(problems) == 0 {
		return nil
	}
	return shared.ErrValidation.WithDetail("forms.refused").
		WithParams(map[string]string{"count": strconv.Itoa(len(problems))}).
		WithFields(problems...)
}

// RefuseClosedChannel refuses creating an entry of a type through a channel its form closes.
func RefuseClosedChannel(state FormState, entryType string, channel FormChannel) error {
	if !state.Closed {
		return nil
	}
	return shared.ErrForbidden.WithDetail("forms.channel_closed").
		WithParams(map[string]string{"entry_type": entryType, "channel": string(channel)})
}

func (f Form) validation(key string) FormValidation {
	for _, validation := range f.Validations {
		if validation.Key == key {
			return validation
		}
	}
	return FormValidation{}
}

func formProblem(path, code, key string) shared.FieldError {
	return shared.FieldError{Path: path, Code: code, Params: map[string]string{"key": key}}
}

// sameFormValue compares two stored values; a value is a decoded document, so deep equality is the
// comparison a person would make.
func sameFormValue(a, b any) bool {
	if !hasValue(a) && !hasValue(b) {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// CheckFormStep holds an entry taking a workflow step to what its form asks of that step: a field
// required only while the step is taken must hold a value, and a validation that fails only for the
// step refuses it. What the form asks regardless of the step is the edit's to hold, not the step's,
// so an entry already short of a field still moves on (UC-FLW-04 check 2).
func CheckFormStep(f Form, item WorkItem, without, with FormState) error {
	var problems []shared.FieldError
	for _, field := range f.Fields {
		if !with.Fields[field.Key].Required || without.Fields[field.Key].Required || hasValue(FormValue(item, field.Key)) {
			continue
		}
		problems = append(problems, formProblem("/"+strings.ReplaceAll(field.Key, ".", "/"), "fields.value_required", field.Key))
	}
	for _, key := range with.Failed {
		if slices.Contains(without.Failed, key) {
			continue
		}
		path := ""
		if validation := f.validation(key); validation.Field != "" {
			path = "/" + strings.ReplaceAll(validation.Field, ".", "/")
		}
		problems = append(problems, shared.FieldError{
			Path: path, Code: "forms.validation_failed", Params: map[string]string{"validation": key},
		})
	}
	if len(problems) == 0 {
		return nil
	}
	return shared.ErrValidation.WithDetail("forms.refused").
		WithParams(map[string]string{"count": strconv.Itoa(len(problems))}).
		WithFields(problems...)
}
