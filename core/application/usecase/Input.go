// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package usecase

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Input is what a channel hands a use case, and Output what it gets back.
//
// Maps rather than a type per use case, because three channels have to speak the same shape: REST
// decodes a request body, MCP receives tool arguments, and an automation action carries
// parameters - all three arrive as untyped data, and a typed struct in the middle would need a
// mapper per use case per channel. The shape is not free-form for it: every use case declares its
// fields (see Field), and the registry checks the input against that declaration before a handler
// sees it.
type (
	Input  map[string]any
	Output map[string]any
)

// Kind is the type of a declared field. Deliberately few: these are the shapes a JSON Schema, an
// OpenAPI body and a CEL expression can all express without a translation table.
type Kind string

const (
	KindString Kind = "string"
	// KindID is a UUID. Its own kind rather than a string with a pattern, so that every channel
	// rejects a malformed identifier the same way.
	KindID   Kind = "id"
	KindBool Kind = "boolean"
	KindInt  Kind = "integer"
	// KindIDList is a set of identifiers - the members of a group, the items of a bulk operation.
	// Its own kind for the same reason KindID is: every channel has to reject a malformed member
	// of the list identically, and a JSON Schema, an OpenAPI array and a CEL list all express it
	// without a translation table.
	KindIDList Kind = "id_list"
	// KindObject and KindList are nested documents: the filter tree of a query and its ordering.
	//
	// They are the exception to "deliberately few", and only one use case has them. A filter is a
	// grammar rather than a field list - what may go in it depends on what this installation
	// serves, and is answered by /meta/capabilities rather than pinned by a schema - so the
	// catalogue checks that a document arrived and leaves what is in it to the grammar that owns
	// it (core/domain/model/view, ADR-0026).
	KindObject Kind = "object"
	KindList   Kind = "list"
	// KindAny is a value whose shape another declaration decides.
	//
	// The second exception, and one use case has it: a custom field's value is a text, a number, a
	// boolean, a date or a list depending on the definition the key names, and that definition is
	// data a tenant wrote rather than anything the catalogue can pin (C-07, domain-model.md §6).
	// Declaring it as a string would make every other kind a type error before the definition was
	// even read, and declaring five fields would be five ways to send one value. The catalogue
	// therefore checks that something arrived and leaves the judgement to the definition, exactly
	// as it leaves a filter tree to the grammar that owns it.
	KindAny Kind = "any"
)

// Field declares one input field of a use case.
//
// Description is English prose, and that is not a breach of the "no display text in the backend"
// rule (ADR-0011): it is protocol documentation, like the descriptions in api/openapi.yaml, read
// by an agent deciding whether to call the tool rather than rendered to a user. Nothing here ever
// reaches an end user's screen - what does is a message code.
type Field struct {
	Name        string
	Kind        Kind
	Required    bool
	Enum        []string
	Description string
}

// String returns a declared string field. Missing and empty are the same answer for an optional
// field: a client that sends `""` for a description means the same thing as one that omits it.
func (in Input) String(field string) string {
	value, _ := in[field].(string)
	return strings.TrimSpace(value)
}

// OptionalString distinguishes an absent field from an empty one, which String deliberately does
// not.
//
// For most fields the distinction is noise - a description sent as "" means what an omitted one
// means. For a preference it is the whole meaning: omitted is "leave my time zone alone", empty is
// "clear it, the workspace default applies again". A client that had only String would have to
// send every preference back to change one, and the day it got that wrong it would silently reset
// somebody's locale.
func (in Input) OptionalString(field string) *string {
	raw, present := in[field]
	if !present || raw == nil {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

// ID returns a declared identifier field, or the zero identifier when it is absent.
func (in Input) ID(field string) (shared.ID, error) {
	raw := in.String(field)
	if raw == "" {
		return "", nil
	}
	id, err := shared.ParseID(raw)
	if err != nil {
		return "", shared.ErrValidation.
			WithDetail("shared.id_malformed").
			WithParams(map[string]string{"value": raw}).
			WithFields(shared.FieldError{Path: "/" + field, Code: "shared.id_malformed"})
	}
	return id, nil
}

// IDList returns a declared list of identifiers, empty when absent. A malformed member fails the
// whole list: half a set of members is not a smaller request, it is a different one.
func (in Input) IDList(field string) ([]shared.ID, error) {
	raw, present := in[field]
	if !present || raw == nil {
		return nil, nil
	}

	values, ok := raw.([]any)
	if !ok {
		// A single identifier where a list belongs is the mistake worth being generous about:
		// every channel's caller writes it at least once.
		if text, isString := raw.(string); isString {
			values = []any{text}
		} else {
			return nil, shared.ErrValidation.
				WithDetail("usecase.field_type_invalid").
				WithFields(shared.FieldError{Path: "/" + field, Code: "usecase.field_type_invalid"})
		}
	}

	ids := make([]shared.ID, 0, len(values))
	for index, value := range values {
		text, isString := value.(string)
		if !isString {
			return nil, shared.ErrValidation.
				WithDetail("usecase.field_type_invalid").
				WithFields(shared.FieldError{Path: fieldPath(field, index), Code: "usecase.field_type_invalid"})
		}
		id, err := shared.ParseID(strings.TrimSpace(text))
		if err != nil {
			return nil, shared.ErrValidation.
				WithDetail("shared.id_malformed").
				WithParams(map[string]string{"value": text}).
				WithFields(shared.FieldError{Path: fieldPath(field, index), Code: "shared.id_malformed"})
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// StringList returns a declared list of strings, empty when absent. A member that is not a string
// fails the whole list, for the reason IDList's does: half a list is not a smaller request, it is
// a different one.
//
// The same generosity about a bare value: a single string where a list belongs is the mistake
// every channel's caller writes at least once, and it means exactly the one-element list.
func (in Input) StringList(field string) ([]string, error) {
	raw, present := in[field]
	if !present || raw == nil {
		return nil, nil
	}

	values, ok := raw.([]any)
	if !ok {
		if text, isString := raw.(string); isString {
			return []string{text}, nil
		}
		if texts, isStrings := raw.([]string); isStrings {
			// Already the shape, which is what an in-process caller hands over.
			return texts, nil
		}
		return nil, shared.ErrValidation.
			WithDetail("usecase.field_type_invalid").
			WithFields(shared.FieldError{Path: "/" + field, Code: "usecase.field_type_invalid"})
	}

	texts := make([]string, 0, len(values))
	for index, value := range values {
		text, isString := value.(string)
		if !isString {
			return nil, shared.ErrValidation.
				WithDetail("usecase.field_type_invalid").
				WithFields(shared.FieldError{
					Path: fieldPath(field, index), Code: "usecase.field_type_invalid",
				})
		}
		texts = append(texts, text)
	}
	return texts, nil
}

// Present reports whether the caller sent the field at all - the distinction OptionalString makes
// for strings, for the fields where the value itself carries no "absent" spelling. An empty list
// is a legitimate instruction (empty the group); a missing one is not an instruction.
func (in Input) Present(field string) bool {
	value, present := in[field]
	return present && value != nil
}

// fieldPath is the JSON pointer of one element, for a field error a client can point at.
func fieldPath(field string, index int) string {
	return "/" + field + "/" + strconv.Itoa(index)
}

// Bool returns a declared boolean field, false when absent.
func (in Input) Bool(field string) bool {
	value, _ := in[field].(bool)
	return value
}

// Int returns a declared integer field, 0 when absent. JSON numbers arrive as float64, which is
// why the conversion is not a single type assertion.
func (in Input) Int(field string) int {
	switch value := in[field].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}

// String returns a field of the result. Outputs are produced by a use case rather than by a
// client, so an absent field is the zero value rather than an error: what is not there was not
// set, and every channel maps the result the same way.
func (out Output) String(field string) string {
	value, _ := out[field].(string)
	return value
}

// Int returns a numeric field of the result.
func (out Output) Int(field string) int {
	switch value := out[field].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}

// validate checks an input against the declared fields.
//
// An unknown field is refused rather than ignored. A client that misspells `parent_id` and gets a
// 201 back has created something in the wrong place and has no way to find out; for an agent,
// which cannot see the result the way a person can, silent acceptance is worse still
// (ai-first.md §1.2, "machine-readable errors instead of interpreting free text").
func validate(fields []Field, in Input) error {
	declared := make(map[string]Field, len(fields))
	var findings []shared.FieldError

	for _, field := range fields {
		declared[field.Name] = field
	}
	for name := range in {
		if _, known := declared[name]; !known {
			findings = append(findings, shared.FieldError{
				Path: "/" + name, Code: "usecase.field_unknown",
			})
		}
	}

	for _, field := range fields {
		value, present := in[field.Name]
		if !present || value == nil {
			if field.Required {
				findings = append(findings, shared.FieldError{
					Path: "/" + field.Name, Code: "usecase.field_required",
				})
			}
			continue
		}
		if finding, ok := checkField(field, value); !ok {
			findings = append(findings, finding)
		}
	}

	if len(findings) > 0 {
		// Sorted, so that two identical requests produce identical answers - a client that
		// compares responses, and a test, both depend on it.
		slices.SortFunc(findings, func(a, b shared.FieldError) int { return strings.Compare(a.Path, b.Path) })
		return shared.ErrValidation.WithDetail("usecase.input_invalid").WithFields(findings...)
	}
	return nil
}

func checkField(field Field, value any) (shared.FieldError, bool) {
	wrongType := shared.FieldError{Path: "/" + field.Name, Code: "usecase.field_type_invalid"}

	switch field.Kind {
	case KindString, KindID:
		text, ok := value.(string)
		if !ok {
			return wrongType, false
		}
		if field.Required && strings.TrimSpace(text) == "" {
			return shared.FieldError{Path: "/" + field.Name, Code: "usecase.field_required"}, false
		}
		if len(field.Enum) > 0 && !slices.Contains(field.Enum, text) {
			return shared.FieldError{
				Path:   "/" + field.Name,
				Code:   "usecase.field_not_in_enum",
				Params: map[string]string{"allowed": strings.Join(field.Enum, ", ")},
			}, false
		}
	case KindBool:
		if _, ok := value.(bool); !ok {
			return wrongType, false
		}
	case KindInt:
		switch number := value.(type) {
		case int, int64:
		case float64:
			if number != float64(int(number)) {
				return wrongType, false
			}
		default:
			return wrongType, false
		}
	case KindObject:
		if _, ok := value.(map[string]any); !ok {
			return wrongType, false
		}
	case KindList:
		if _, ok := value.([]any); !ok {
			return wrongType, false
		}
	}
	return shared.FieldError{}, true
}
