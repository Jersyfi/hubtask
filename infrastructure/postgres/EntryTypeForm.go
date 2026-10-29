// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// formDocument is a type's form as the `entry_type.form` column holds it (migration 0214). The
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
