// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func childrenOf(f fixture, parent shared.ID) []domain.Page {
	var out []domain.Page
	for _, page := range f.store.pages {
		if page.ParentID == parent && !page.IsTrashed() {
			out = append(out, page)
		}
	}
	return out
}

// UC-DOC-05 check 5: a template started with its subpages brings them along beneath the new page,
// none of them a template, and the template's own pages stay as they were.
func TestASpaceTemplateBringsItsSubpages(t *testing.T) {
	f := newFixture()
	template, err := (CreatePage{Space: f.space}).Execute(context.Background(), as(anna), CreatePageCommand{
		HubID: hub, Title: "Team space", Template: true, Body: ptr("Welcome"),
	})
	if err != nil {
		t.Fatal(err)
	}
	notes := f.create(t, anna, hub, template.ID, "Meeting notes", "## Agenda")
	f.create(t, anna, hub, notes.ID, "First meeting", "## Agenda\nnone")
	f.create(t, anna, hub, template.ID, "Decisions", "")

	page, err := (CreatePage{Space: f.space}).Execute(context.Background(), as(bert), CreatePageCommand{
		HubID: hub, Title: "Platform team", TemplateID: template.ID, WithSubpages: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	top := childrenOf(f, page.ID)
	if len(top) != 2 {
		t.Fatalf("subpages %+v", top)
	}
	var copiedNotes domain.Page
	for _, child := range top {
		if child.Template || child.CreatedBy != bert || child.HubID != hub {
			t.Fatalf("subpage %+v", child)
		}
		if child.Title == "Meeting notes" {
			copiedNotes = child
		}
	}
	if copiedNotes.Body != "## Agenda" || len(childrenOf(f, copiedNotes.ID)) != 1 {
		t.Fatalf("notes %+v under it %+v", copiedNotes, childrenOf(f, copiedNotes.ID))
	}
	if len(childrenOf(f, template.ID)) != 2 {
		t.Fatal("the template's own pages changed")
	}
	// Without the subpages asked for, a template gives its body alone (check 2).
	alone, err := (CreatePage{Space: f.space}).Execute(context.Background(), as(bert), CreatePageCommand{
		HubID: hub, Title: "Alone", TemplateID: template.ID,
	})
	if err != nil || len(childrenOf(f, alone.ID)) != 0 {
		t.Fatalf("alone %v, %v", childrenOf(f, alone.ID), err)
	}
}

// UC-DOC-05 check 6: Hubtask's own space templates arrive as a tree the client wrote in the
// reader's words, written in one go; a tree past the bound is refused whole.
func TestASpaceIsStartedWithSeveralPagesAtOnce(t *testing.T) {
	f := newFixture()
	page, err := (CreatePage{Space: f.space}).Execute(context.Background(), as(anna), CreatePageCommand{
		HubID: hub, Title: "Overview", Subpages: []Subpage{
			{Title: "How-tos", Subpages: []Subpage{{Title: "Set up a laptop", Body: "1. Unpack"}}},
			{Title: "Decisions", Icon: "D"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	top := childrenOf(f, page.ID)
	if len(top) != 2 || len(f.store.pages) != 4 {
		t.Fatalf("pages %d, top %+v", len(f.store.pages), top)
	}
	if top[0].OrderKey == top[1].OrderKey {
		t.Fatal("siblings share an order key")
	}

	many := make([]Subpage, domain.MaxPagesPerWrite)
	for i := range many {
		many[i] = Subpage{Title: "p"}
	}
	before := len(f.store.pages)
	if _, err := (CreatePage{Space: f.space}).Execute(context.Background(), as(anna), CreatePageCommand{
		HubID: hub, Title: "Too much", Subpages: many,
	}); code(err) != "pages.too_many_at_once" {
		t.Fatalf("got %v", err)
	}
	if len(f.store.pages) != before {
		t.Fatal("a refused tree left pages behind")
	}
	if _, err := (CreatePage{Space: f.space}).Execute(context.Background(), as(anna), CreatePageCommand{
		HubID: hub, Title: "Both", TemplateID: page.ID, WithSubpages: true, Subpages: []Subpage{{Title: "x"}},
	}); code(err) != "pages.subpages_twice" {
		t.Fatalf("got %v", err)
	}
}
