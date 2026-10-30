// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-SVC-05 check 1: the catalogue offers the top-level types whose form opens the portal's door,
// that the collection offers and that are not archived; the fields the portal hides are not shown.
func TestTheCatalogueOffersTheTypesThePortalOpens(t *testing.T) {
	archived := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	portal := func(closed bool, hidden ...string) work.Form {
		return work.Form{
			Fields:   []work.FormField{{Key: "title"}, {Key: "custom_fields.room"}, {Key: "custom_fields.cost_centre"}},
			Channels: []work.FormChannelRule{{Channel: work.ChannelPortal, Closed: closed, Hidden: hidden}},
		}
	}
	types := []work.EntryType{
		{Key: "incident", Level: work.ItemTask, Name: "Incident", Form: portal(false, "custom_fields.cost_centre")},
		{Key: "access", Level: work.ItemTask, Name: "Access", Form: portal(false)},
		{Key: "change", Level: work.ItemTask, Form: work.Form{}},
		{Key: "closed", Level: work.ItemTask, Form: portal(true)},
		{Key: "old", Level: work.ItemTask, Form: portal(false), ArchivedAt: &archived},
		{Key: "step", Level: work.ItemActivity, Form: portal(false)},
	}

	all := servicedesk.RequestCatalogue(types, nil, nil)
	if len(all) != 2 || all[0].Key != "incident" || all[1].Key != "access" {
		t.Fatalf("the catalogue is %+v", all)
	}
	if len(all[0].Form.Fields) != 2 || all[0].Form.Fields[1].Key != "custom_fields.room" || len(all[0].Form.Channels) != 0 {
		t.Errorf("the portal's form is %+v", all[0].Form)
	}
	narrowed := servicedesk.RequestCatalogue(types, []string{"access", "change"}, nil)
	if len(narrowed) != 1 || narrowed[0].Key != "access" || !servicedesk.InCatalogue(narrowed, "access") ||
		servicedesk.InCatalogue(narrowed, "incident") {
		t.Errorf("a collection offering access and change answers %+v", narrowed)
	}
}

// UC-SVC-05 check 1: the portal asks the custom fields its form shows with their kind and options,
// and not one the portal hides or no definition backs.
func TestThePortalAsksTheShownCustomFieldsWithTheirKind(t *testing.T) {
	types := []work.EntryType{{Key: "incident", Level: work.ItemTask, Form: work.Form{
		Fields: []work.FormField{
			{Key: "title"}, {Key: "custom_fields.room"}, {Key: "custom_fields.cost_centre"}, {Key: "custom_fields.gone"},
		},
		Channels: []work.FormChannelRule{{Channel: work.ChannelPortal, Hidden: []string{"custom_fields.cost_centre"}}},
	}}}
	definitions := []work.CustomFieldDefinition{
		{Key: "room", Kind: work.CustomFieldSelect, Options: []string{"floor_1", "floor_2"}},
		{Key: "cost_centre", Kind: work.CustomFieldText},
	}
	questions := servicedesk.RequestCatalogue(types, nil, definitions)[0].Questions
	if len(questions) != 1 || questions[0].Key != "room" || questions[0].Kind != work.CustomFieldSelect ||
		len(questions[0].Options) != 2 {
		t.Errorf("the portal asks %+v", questions)
	}
}
