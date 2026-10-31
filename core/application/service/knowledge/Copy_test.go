// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"
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
