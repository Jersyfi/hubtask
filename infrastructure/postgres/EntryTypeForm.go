// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// formDocument is a type's form as the `entry_type.form` column holds it (migration 0221). The
// names are the contract's (api/openapi.yaml, EntryForm), so that the archive and the sync replica
// carry the document a client already reads.
type formDocument struct {
	Sections    []formSectionDocument    `json:"sections,omitempty"`
	Fields      []formFieldDocument      `json:"fields,omitempty"`
	Validations []formValidationDocument `json:"validations,omitempty"`
	Channels    []formChannelDocument    `json:"channels,omitempty"`
}

type formSectionDocument struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type formFieldDocument struct {
	Key          string `json:"key"`
	Section      string `json:"section,omitempty"`
	Required     bool   `json:"required,omitempty"`
	RequiredWhen string `json:"required_when,omitempty"`
	ReadOnlyWhen string `json:"read_only_when,omitempty"`
	HiddenWhen   string `json:"hidden_when,omitempty"`
	Default      any    `json:"default,omitempty"`
	Help         string `json:"help,omitempty"`
}

type formValidationDocument struct {
	Key     string `json:"key"`
	Field   string `json:"field,omitempty"`
	Holds   string `json:"holds"`
	Message string `json:"message,omitempty"`
}

type formChannelDocument struct {
	Channel string         `json:"channel"`
	Closed  bool           `json:"closed,omitempty"`
	Hidden  []string       `json:"hidden,omitempty"`
	Presets map[string]any `json:"presets,omitempty"`
}

func formColumn(form work.Form) ([]byte, error) {
	document := formDocument{}
	for _, section := range form.Sections {
		document.Sections = append(document.Sections, formSectionDocument(section))
	}
	for _, field := range form.Fields {
		document.Fields = append(document.Fields, formFieldDocument(field))
	}
	for _, validation := range form.Validations {
		document.Validations = append(document.Validations, formValidationDocument(validation))
	}
	for _, rule := range form.Channels {
		document.Channels = append(document.Channels, formChannelDocument{
			Channel: string(rule.Channel), Closed: rule.Closed, Hidden: rule.Hidden, Presets: rule.Presets,
		})
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("entry_types.form_unwritable").WithCause(fmt.Errorf("encoding a form: %w", err))
	}
	return raw, nil
}

func formFrom(raw []byte) (work.Form, error) {
	if len(raw) == 0 {
		return work.Form{}, nil
	}
	var document formDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return work.Form{}, shared.ErrInternal.WithDetail("entry_types.form_unreadable").WithCause(fmt.Errorf("decoding a form: %w", err))
	}
	form := work.Form{}
	for _, section := range document.Sections {
		form.Sections = append(form.Sections, work.FormSection(section))
	}
	for _, field := range document.Fields {
		form.Fields = append(form.Fields, work.FormField(field))
	}
	for _, validation := range document.Validations {
		form.Validations = append(form.Validations, work.FormValidation(validation))
	}
	for _, rule := range document.Channels {
		form.Channels = append(form.Channels, work.FormChannelRule{
			Channel: work.FormChannel(rule.Channel), Closed: rule.Closed, Hidden: rule.Hidden, Presets: rule.Presets,
		})
	}
	return form, nil
}

// translationDocument is one language's texts as the `entry_type.translations` column holds them
// (migration 0318), under the contract's names (EntryTypeTranslations).
type translationDocument struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// translationsColumn is a type's texts per language as the column holds them; none is `{}`.
func translationsColumn(translations map[string]work.EntryTypeText) ([]byte, error) {
	document := make(map[string]translationDocument, len(translations))
	for tag, words := range translations {
		document[tag] = translationDocument(words)
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("entry_types.translations_unwritable").WithCause(fmt.Errorf("encoding translations: %w", err))
	}
	return raw, nil
}

// translationsFrom reads the column back; an empty one names no language.
func translationsFrom(raw []byte) (map[string]work.EntryTypeText, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var document map[string]translationDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, shared.ErrInternal.WithDetail("entry_types.translations_unreadable").WithCause(fmt.Errorf("decoding translations: %w", err))
	}
	if len(document) == 0 {
		return nil, nil
	}
	out := make(map[string]work.EntryTypeText, len(document))
	for tag, words := range document {
		out[tag] = work.EntryTypeText(words)
	}
	return out, nil
}
