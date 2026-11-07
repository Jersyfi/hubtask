// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// CatalogueEntry is one kind of request the portal offers a requester (UC-SVC-05): a type, and
// its form as the portal's door narrows it.
type CatalogueEntry struct {
	Key string
	// Name and Description are in the reader's language where the type says them there
	// (EntryType.TextIn, UC-SVC-05 check 7).
	Name        string
	Description string
	Icon        string
	// Form is the type's form without the fields the portal hides; their presets still apply
	// when the request is raised.
	Form work.Form
	// Questions are the custom fields the shown form asks, with what a requester's answer must
	// be: a portal reads no field definitions of its own, as a requester reads no team fields.
	Questions []Question
}

// Question is one custom field a portal form asks: its key, its kind and, for a choice, the
// values it offers. A client names it in the reader's words (rule 8).
type Question struct {
	Key     string
	Kind    work.CustomFieldKind
	Options []string
}

// RequestCatalogue answers the types a collection offers through the portal, in the registry's
// order: a top-level type, not archived, offered by the collection (offered nil offers every
// type), whose form names the PORTAL channel and leaves it open. The portal is opt-in per type,
// so a workspace's internal types never reach a requester by default.
//
// definitions are the custom fields in force at the collection; a field of the form that none
// defines is asked of nobody. locale is the reader's, whose words the entries are named in.
func RequestCatalogue(
	types []work.EntryType, offered []string, definitions []work.CustomFieldDefinition, locale string,
) []CatalogueEntry {
	return CatalogueThrough(work.ChannelPortal, types, offered, definitions, locale)
}

// CatalogueThrough is RequestCatalogue for another opt-in door: a public form offers the types
// whose form names PUBLIC and leaves it open (UC-SVC-13).
func CatalogueThrough(
	channel work.FormChannel, types []work.EntryType, offered []string, definitions []work.CustomFieldDefinition,
	locale string,
) []CatalogueEntry {
	out := []CatalogueEntry{}
	for _, entryType := range types {
		if entryType.Level != work.ItemTask || entryType.ArchivedAt != nil {
			continue
		}
		if offered != nil && !slices.Contains(offered, entryType.Key) {
			continue
		}
		rule, named := channelRule(entryType.Form, channel)
		if !named || rule.Closed {
			continue
		}
		form := portalForm(entryType.Form, rule.Hidden)
		words := entryType.TextIn(locale)
		out = append(out, CatalogueEntry{
			Key: entryType.Key, Name: words.Name, Description: words.Description, Icon: entryType.Icon,
			Form: form, Questions: questionsOf(form, definitions),
		})
	}
	return out
}

// InCatalogue reports whether the catalogue offers a type.
func InCatalogue(catalogue []CatalogueEntry, key string) bool {
	return slices.ContainsFunc(catalogue, func(entry CatalogueEntry) bool { return entry.Key == key })
}

func channelRule(form work.Form, channel work.FormChannel) (work.FormChannelRule, bool) {
	for _, rule := range form.Channels {
		if rule.Channel == channel {
			return rule, true
		}
	}
	return work.FormChannelRule{}, false
}

// portalForm is what a requester is shown: the fields the door hides taken out with their
// validations, which are never asked there (work.Form.State), and no channel rules - both are the
// team's configuration.
func portalForm(form work.Form, hidden []string) work.Form {
	shown := work.Form{Sections: slices.Clone(form.Sections)}
	for _, field := range form.Fields {
		if !slices.Contains(hidden, field.Key) {
			shown.Fields = append(shown.Fields, field)
		}
	}
	for _, validation := range form.Validations {
		if validation.Field == "" || !slices.Contains(hidden, validation.Field) {
			shown.Validations = append(shown.Validations, validation)
		}
	}
	return shown
}

// questionsOf answers the custom fields a form asks, in the form's order.
func questionsOf(form work.Form, definitions []work.CustomFieldDefinition) []Question {
	out := []Question{}
	for _, field := range form.Fields {
		key, custom := strings.CutPrefix(field.Key, work.FormCustomPrefix)
		if !custom {
			continue
		}
		for _, definition := range definitions {
			if definition.Key == key {
				out = append(out, Question{Key: key, Kind: definition.Kind, Options: slices.Clone(definition.Options)})
				break
			}
		}
	}
	return out
}
