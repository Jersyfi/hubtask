// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"testing"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

type brandedDesk struct{ workrepo.Containers }

func (brandedDesk) Find(_ context.Context, id shared.ID) (work.Container, error) {
	return work.Container{
		ID: id, TenantID: tenantID, Type: work.ContainerCollection, ParentID: hubID,
		Name: "IT help", Description: "Printers, laptops and access", Icon: "printer", ColorToken: "blue",
	}, nil
}

type translatedTypes struct{ workrepo.EntryTypes }

func (translatedTypes) List(context.Context) ([]work.EntryType, error) {
	return []work.EntryType{{
		Key: "incident", Level: work.ItemTask, Name: "Incident", Description: "Something is broken",
		Translations: map[string]work.EntryTypeText{"de": {Name: "Störung", Description: "Etwas ist kaputt"}},
		Form:         work.Form{Channels: []work.FormChannelRule{{Channel: work.ChannelPortal}}},
	}}, nil
}

// UC-SVC-05 checks 7 and 8: the catalogue names its kinds in the reader's language and says
// which service it is - the collection's name, description, icon and colour.
func TestTheCatalogueSpeaksTheReadersLanguageAndNamesItsDesk(t *testing.T) {
	portal := Portal{
		Containers: brandedDesk{}, EntryTypes: translatedTypes{},
		Desk: Desk{Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: &seqIDs{}},
	}
	actor := reader()
	actor.Locale = "de-AT"
	out, err := ListRequestCatalogue{Portal: portal}.Descriptor().Handler.Invoke(context.Background(), actor,
		usecase.Input{"collection_id": collection.String()})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := out["data"].([]usecase.Output)
	if len(data) != 1 || data[0]["name"] != "Störung" || data[0]["description"] != "Etwas ist kaputt" {
		t.Errorf("the catalogue reads %+v", data)
	}
	desk, _ := out["desk"].(usecase.Output)
	if desk["name"] != "IT help" || desk["description"] != "Printers, laptops and access" ||
		desk["icon"] != "printer" || desk["color_token"] != "blue" {
		t.Errorf("the desk is %+v", desk)
	}
}

// UC-SVC-05 check 9: the catalogue names the desk's logo, or none for a desk drawn by its icon.
func TestTheCatalogueNamesTheDesksLogo(t *testing.T) {
	for _, logo := range []shared.ID{logoID, ""} {
		portal := Portal{
			Containers: brandedDesk{}, EntryTypes: translatedTypes{},
			Logos: &logoStore{logos: map[shared.ID]shared.ID{collection: logo}},
			Desk:  Desk{Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: &seqIDs{}},
		}
		out, err := ListRequestCatalogue{Portal: portal}.Descriptor().Handler.Invoke(context.Background(), reader(),
			usecase.Input{"collection_id": collection.String()})
		if err != nil {
			t.Fatal(err)
		}
		desk, _ := out["desk"].(usecase.Output)
		if desk["logo_media_id"] != idText(logo) {
			t.Errorf("the desk's logo reads %v, want %v", desk["logo_media_id"], idText(logo))
		}
	}
}
