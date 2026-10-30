// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// CatalogueEntry is one kind of request the portal offers a requester (UC-SVC-05): a type, and
// its form as the portal's door narrows it.
type CatalogueEntry struct {
	Key  string
	Name string
	Icon string
	// Form is the type's form without the fields the portal hides; their presets still apply
	// when the request is raised.
	Form work.Form
}

// RequestCatalogue answers the types a collection offers through the portal, in the registry's
// order: a top-level type, not archived, offered by the collection (offered nil offers every
// type), whose form names the PORTAL channel and leaves it open. The portal is opt-in per type,
// so a workspace's internal types never reach a requester by default.
func RequestCatalogue(types []work.EntryType, offered []string) []CatalogueEntry {
	out := []CatalogueEntry{}
	for _, entryType := range types {
		if entryType.Level != work.ItemTask || entryType.ArchivedAt != nil {
			continue
		}
		if offered != nil && !slices.Contains(offered, entryType.Key) {
			continue
		}
		rule, named := portalRule(entryType.Form)
		if !named || rule.Closed {
			continue
		}
		out = append(out, CatalogueEntry{
			Key: entryType.Key, Name: entryType.Name, Icon: entryType.Icon,
			Form: portalForm(entryType.Form, rule.Hidden),
		})
	}
	return out
}

// InCatalogue reports whether the catalogue offers a type.
func InCatalogue(catalogue []CatalogueEntry, key string) bool {
	return slices.ContainsFunc(catalogue, func(entry CatalogueEntry) bool { return entry.Key == key })
}

func portalRule(form work.Form) (work.FormChannelRule, bool) {
	for _, rule := range form.Channels {
		if rule.Channel == work.ChannelPortal {
			return rule, true
		}
	}
	return work.FormChannelRule{}, false
}

// portalForm is what a requester is shown: the fields the portal hides taken out, and no
// channel rules - they are the team's configuration.
func portalForm(form work.Form, hidden []string) work.Form {
	shown := work.Form{Sections: slices.Clone(form.Sections), Validations: slices.Clone(form.Validations)}
	for _, field := range form.Fields {
		if !slices.Contains(hidden, field.Key) {
			shown.Fields = append(shown.Fields, field)
		}
	}
	return shown
}
