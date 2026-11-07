// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// searcher is an index other than the page store, recording the spaces it was handed.
type searcher struct {
	spaces  []shared.ID
	excerpt string
}

func (s *searcher) Search(_ context.Context, q repository.PageQuery) ([]repository.Hit, error) {
	s.spaces = q.HubIDs
	return []repository.Hit{{ID: "0192f000-0000-7000-8000-0000000009aa", Excerpt: s.excerpt}}, nil
}

// The search port answers a page search, handed only the spaces the reader may read (F-9).
func TestAPageSearchAsksTheSearchPortWithTheReadableSpaces(t *testing.T) {
	f := newFixture()
	index := &searcher{}
	hits, err := (SearchPages{Space: f.space, Searcher: index}).Execute(context.Background(), as(bert), SearchPagesQuery{Text: "budget"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("the search answered %+v, %v", hits, err)
	}
	if !slices.Equal(index.spaces, []shared.ID{hub}) {
		t.Errorf("the port was handed %v, want only the shared hub", index.spaces)
	}
}

// A hit's passage reaches the reader as words: the body's Markdown is not shown (UC-DOC-01/3).
func TestAPageHitsPassageIsWords(t *testing.T) {
	index := &searcher{excerpt: "**Teilnehmende:** Anna\n\n## «Beschlüsse»\n| 45 min |"}
	hits, err := (SearchPages{Space: newFixture().space, Searcher: index}).Execute(context.Background(), as(bert), SearchPagesQuery{Text: "budget"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("the search answered %+v, %v", hits, err)
	}
	if want := "Teilnehmende: Anna «Beschlüsse» 45 min"; hits[0].Excerpt != want {
		t.Errorf("the passage reads %q, want %q", hits[0].Excerpt, want)
	}
}

// A pages summary's reach is the readable spaces; a hub named alone is asked with its refusal.
func TestAPagesSummaryReachesTheReadableSpaces(t *testing.T) {
	reach := PageReach{Space: newFixture().space}
	readable, err := reach.Readable(context.Background(), as(bert), []shared.ID{hub, private}, false)
	if err != nil || !slices.Equal(readable, []shared.ID{hub}) {
		t.Fatalf("Bert reaches %v, %v; want the shared hub", readable, err)
	}
	if _, err := reach.Readable(context.Background(), as(bert), []shared.ID{private}, true); err == nil {
		t.Fatal("Bert counted the pages of Anna's private hub")
	}
}

// UC-DOC-09 checks 1 and 2: a page with every page under it leaves as Markdown files with front
// matter, each under its parent's folder, a reference kept as its address; the export is in the
// trail with how many pages left.
func TestAPageTreeIsExportedAndRecorded(t *testing.T) {
	f := newFixture()
	root := f.create(t, anna, hub, "", "Handbook", "See hubtask:item/0193a000-0000-7000-8000-000000000001")
	child := f.create(t, anna, hub, root.ID, "Travel", "Book early")
	files, err := ExportPage{Space: f.space}.Execute(context.Background(), as(anna), root.ID, true, domain.FormatMarkdown)
	if err != nil || len(files) != 2 {
		t.Fatalf("exported %+v %v", files, err)
	}
	if files[0].Path != "Handbook.md" || files[1].Path != "Handbook/Travel.md" {
		t.Errorf("paths %s and %s", files[0].Path, files[1].Path)
	}
	for _, want := range []string{"id: " + child.ID.String(), `title: "Travel"`, "parent: " + root.ID.String()} {
		if !strings.Contains(files[1].Content, want) {
			t.Errorf("the child's front matter lacks %q: %q", want, files[1].Content)
		}
	}
	if !strings.Contains(files[0].Content, "hubtask:item/0193a000-0000-7000-8000-000000000001") {
		t.Errorf("the reference did not stay an address: %q", files[0].Content)
	}
	last := f.trail.entries[len(f.trail.entries)-1]
	pages, _ := last.Changes["pages"].(map[string]any)
	if last.Action != PageExportedAction || fmt.Sprint(pages["to"]) != "2" {
		t.Errorf("the trail recorded %s with %v", last.Action, last.Changes)
	}
}

// A search typed in another Unicode form finds the page: the words are compared in normal form
// C, as the page's body is stored (i18n-l10n.md §5).
func TestASearchInDecomposedLettersFindsThePage(t *testing.T) {
	f := newFixture()
	f.space.Text = text.Composing{}
	page := f.create(t, anna, hub, "", "Menu", "Café opens at nine")
	hits, err := (SearchPages{Space: f.space}).Execute(context.Background(), as(bert), SearchPagesQuery{Text: "Café", Limit: 10})
	if err != nil || len(hits) != 1 || hits[0].ID != page.ID {
		t.Fatalf("hits %+v, %v", hits, err)
	}
}
