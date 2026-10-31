// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-DOC-01 check 9: a page carries labels in their one written form, without a new version; the
// space's facet counts only the pages the reader may read; a search narrows by label, first author
// and last change, and finds by label alone; the labels reach the trail as a sensitive change.
func TestAPagesLabelsNarrowASearchAndCountAsTheSpacesFacet(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	guide := f.create(t, anna, hub, "", "Onboarding", "budget for laptops")
	policy := f.create(t, bert, hub, "", "Travel", "budget for trips")
	salaries := f.create(t, anna, hub, "", "Salaries", "budget lines")

	labels, err := (SetPageLabels{Space: f.space}).Execute(ctx, as(bert), guide.ID, []string{"HR", "how-to", "hr"})
	if err != nil || !slices.Equal(labels, []string{"how-to", "hr"}) {
		t.Fatalf("SetPageLabels answered %q, %v", labels, err)
	}
	if _, err := (SetPageLabels{Space: f.space}).Execute(ctx, as(bert), policy.ID, []string{"hr"}); err != nil {
		t.Fatal(err)
	}
	if _, err := (SetPageLabels{Space: f.space}).Execute(ctx, as(anna), salaries.ID, []string{"hr", "secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := (SetPageLabels{Space: f.space}).Execute(ctx, as(bert), guide.ID, []string{"two words"}); code(err) != "pages.label_malformed" {
		t.Errorf("a label of two words answered %v", err)
	}
	if read, _ := (ReadPage{Space: f.space}).Execute(ctx, as(anna), guide.ID); read.Page.Version != guide.Version ||
		len(f.store.versions[guide.ID]) != 1 {
		t.Errorf("labelling changed the page's version to %d or its history", read.Page.Version)
	}
	if got, err := (ListPageLabels{Space: f.space}).Execute(ctx, as(anna), guide.ID); err != nil || !slices.Equal(got, []string{"how-to", "hr"}) {
		t.Errorf("ListPageLabels answered %q, %v", got, err)
	}
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: salaries.ID, AccountIDs: []shared.ID{anna}}); err != nil {
		t.Fatal(err)
	}

	facet, err := (ListSpaceLabels{Space: f.space}).Execute(ctx, as(bert), hub)
	want := []LabelCount{{Label: "hr", Pages: 2}, {Label: "how-to", Pages: 1}}
	if err != nil || !slices.Equal(facet, want) {
		t.Errorf("Bert's facet is %+v, %v; want %+v - the restricted page uncounted", facet, err, want)
	}
	if _, err := (ListSpaceLabels{Space: f.space}).Execute(ctx, as(bert), private); err == nil {
		t.Error("Bert counted the labels of Anna's private hub")
	}

	search := SearchPages{Space: f.space}
	ids := func(q SearchPagesQuery) []shared.ID {
		t.Helper()
		hits, err := search.Execute(ctx, as(bert), q)
		if err != nil {
			t.Fatalf("SearchPages(%+v): %v", q, err)
		}
		out := []shared.ID{}
		for _, hit := range hits {
			out = append(out, hit.ID)
		}
		slices.Sort(out)
		return out
	}
	both := []shared.ID{guide.ID, policy.ID}
	slices.Sort(both)
	if got := ids(SearchPagesQuery{Label: "HR"}); !slices.Equal(got, both) {
		t.Errorf("by label alone Bert found %v, want %v", got, both)
	}
	if got := ids(SearchPagesQuery{Text: "budget", Label: "how-to"}); !slices.Equal(got, []shared.ID{guide.ID}) {
		t.Errorf("by text and label Bert found %v", got)
	}
	if got := ids(SearchPagesQuery{Text: "budget", AuthorID: bert}); !slices.Equal(got, []shared.ID{policy.ID}) {
		t.Errorf("by author Bert found %v", got)
	}
	later := policy.UpdatedAt.Add(time.Nanosecond)
	if got := ids(SearchPagesQuery{Label: "hr", UpdatedFrom: &later}); len(got) != 0 {
		t.Errorf("pages changed after the last write: %v", got)
	}
	if _, err := search.Execute(ctx, as(bert), SearchPagesQuery{}); code(err) != "pages.search_empty" {
		t.Errorf("a search with nothing to look for answered %v", err)
	}
	if _, err := search.Execute(ctx, as(bert), SearchPagesQuery{Label: "hr", UpdatedFrom: &later, UpdatedTo: &later}); code(err) != "pages.search_window_empty" {
		t.Errorf("an empty window answered %v", err)
	}

	for _, entry := range f.trail.entries {
		if change, ok := entry.Changes["labels"].(map[string]any); ok {
			if _, open := change["to"]; open {
				t.Fatal("a page's labels reached the trail as open text")
			}
		}
	}
}
