// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-DOC-01 check 8: a copy is a new page of the same space with the page's body, beside it or
// where asked, never a template, with the title given or the page's; its versions start afresh,
// and a page the reader may not read is not copied.
func TestAPageIsCopiedWithinItsSpace(t *testing.T) {
	f := newFixture()
	root := f.create(t, anna, hub, "", "Handbook", "")
	page := f.create(t, anna, hub, root.ID, "Onboarding", "Day one: laptop")

	copied, err := (CopyPage{Space: f.space}).Execute(context.Background(), as(bert), CopyPageCommand{PageID: page.ID})
	if err != nil {
		t.Fatal(err)
	}
	if copied.ID == page.ID || copied.HubID != hub || copied.ParentID != root.ID || copied.Title != "Onboarding" ||
		copied.Body != page.Body || copied.Template || copied.CreatedBy != bert {
		t.Errorf("the copy %+v", copied)
	}
	if versions := f.store.versions[copied.ID]; len(versions) != 1 {
		t.Errorf("the copy has %d versions, want its first only", len(versions))
	}

	top, err := (CopyPage{Space: f.space}).Execute(context.Background(), as(bert), CopyPageCommand{
		PageID: page.ID, Title: "Onboarding (Berlin)", ParentGiven: true,
	})
	if err != nil || top.ParentID != "" || top.Title != "Onboarding (Berlin)" {
		t.Errorf("a copy at the top with its own title: %+v, %v", top, err)
	}

	hidden := f.create(t, anna, private, "", "Notes", "mine")
	if _, err := (CopyPage{Space: f.space}).Execute(context.Background(), as(bert), CopyPageCommand{PageID: hidden.ID}); err == nil {
		t.Error("Bert copied a page of Anna's private hub")
	}
}

// UC-DOC-01 check 8: a restricted page's copy keeps the people the source admits - its own
// restriction and those above it - wherever it is put, and the copier may lift it.
func TestACopyOfARestrictedPageKeepsItsRestriction(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	root := f.create(t, anna, hub, "", "Handbook", "")
	page := f.create(t, anna, hub, root.ID, "Salaries", "Band A")
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: root.ID, AccountIDs: []shared.ID{anna, bert}}); err != nil {
		t.Fatal(err)
	}

	copied, err := (CopyPage{Space: f.space}).Execute(ctx, as(bert), CopyPageCommand{PageID: page.ID, ParentGiven: true})
	if err != nil {
		t.Fatal(err)
	}
	readers, _ := f.store.Readers(ctx, []shared.ID{copied.ID})
	if !copied.Restricted || !readers[copied.ID].Restricted || readers[copied.ID].Admits(carla) ||
		!readers[copied.ID].Admits(anna) || !readers[copied.ID].Admits(bert) {
		t.Errorf("a copy at the top of the space reads to %+v", readers[copied.ID])
	}

	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(bert), RestrictPageCommand{
		PageID: copied.ID, Lift: true,
	}); err != nil {
		t.Errorf("the copier could not lift the copy's restriction: %v", err)
	}
	if readers, _ := f.store.Readers(ctx, []shared.ID{copied.ID}); readers[copied.ID].Restricted {
		t.Error("the lifted copy is still restricted")
	}
}
