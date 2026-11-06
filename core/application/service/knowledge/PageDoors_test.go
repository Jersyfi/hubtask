// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// pageDoors is the page use cases as REST, MCP and hubctl reach them: through the registry.
func pageDoors(t *testing.T, s Space) *usecase.Registry {
	t.Helper()
	doors, err := usecase.NewRegistry(nil,
		CreatePage{s}.Descriptor(), ReadPage{s}.Descriptor(), ListPages{s}.Descriptor(), UpdatePage{s}.Descriptor(),
		MovePage{s}.Descriptor(), TrashPage{s}.Descriptor(), RestorePage{s}.Descriptor(), ListTrashedPages{s}.Descriptor(),
		VerifyPage{s}.Descriptor(), SearchPages{Space: s}.Descriptor(), ListPageBacklinks{s}.Descriptor(), ExportPage{s}.Descriptor(),
		ListPageVersions{s}.Descriptor(), ReadPageVersion{s}.Descriptor(), NamePageVersion{s}.Descriptor(),
		RestorePageVersion{s}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return doors
}

// UC-DOC-01 checks 1, 2, 4, 5 and 7 through the doors: a page and its subpages are written, read
// with their breadcrumb, listed, changed at the version read, moved, trashed with what lies under
// them and brought back; a page of a hub out of reach answers as a missing one; malformed input
// writes nothing; the trail never holds the words.
func TestAPageIsWrittenAndFoundThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := pageDoors(t, f.space)

	for name, in := range map[string]usecase.Input{
		"a subpage that is no object": {"hub_id": hub.String(), "title": "Handbook", "subpages": []any{"Onboarding"}},
		"a parent that is no id":      {"hub_id": hub.String(), "title": "Handbook", "parent_id": "the top"},
	} {
		if _, err := doors.Invoke(ctx, CreatePageName, as(anna), in); err == nil {
			t.Errorf("%s was written", name)
		}
	}
	if _, err := doors.Invoke(ctx, CreatePageName, as(bert), usecase.Input{"hub_id": private.String(), "title": "Notes"}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert wrote in Anna's private hub: %v", err)
	}
	if len(f.store.pages) != 0 {
		t.Fatalf("a refused page was written: %d pages", len(f.store.pages))
	}

	root, err := doors.Invoke(ctx, CreatePageName, as(anna), usecase.Input{
		"hub_id": hub.String(), "title": "Handbook", "body": "Our travel budget", "icon": "📘",
		"subpages": []any{map[string]any{"title": "Onboarding", "body": "Day one", "subpages": []any{map[string]any{"title": "Laptop"}}}},
	})
	if err != nil || root["body"] != "Our travel budget" || root["icon"] != "📘" || len(f.store.pages) != 3 {
		t.Fatalf("writing a page with its subpages: %v %v (%d pages)", root, err, len(f.store.pages))
	}
	rootID := root.String("id")
	var child shared.ID
	for id, page := range f.store.pages {
		if page.Title == "Onboarding" {
			child = id
		}
	}

	read, err := doors.Invoke(ctx, ReadPageName, as(bert), usecase.Input{"page_id": child.String()})
	if err != nil || read["writable"] != true || len(read["ancestors"].([]usecase.Output)) != 1 || read["draft"] != nil {
		t.Fatalf("reading the subpage: %v %v", read, err)
	}
	listed, err := doors.Invoke(ctx, ListPagesName, as(bert), usecase.Input{"hub_id": hub.String()})
	if err != nil || len(listed["data"].([]usecase.Output)) != 3 {
		t.Fatalf("listing the space: %v %v", listed, err)
	}
	if _, err := doors.Invoke(ctx, ListPagesName, as(bert), usecase.Input{"hub_id": private.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert listed Anna's private hub: %v", err)
	}

	update := usecase.Input{"page_id": rootID, "title": "Travel handbook", "expected_version": 9}
	if _, err := doors.Invoke(ctx, UpdatePageName, as(bert), update); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale write: %v", err)
	}
	update["expected_version"] = root.Int("version")
	changed, err := doors.Invoke(ctx, UpdatePageName, as(bert), update)
	if err != nil || changed["title"] != "Travel handbook" {
		t.Fatalf("changing the title: %v %v", changed, err)
	}

	if _, err := doors.Invoke(ctx, MovePageName, as(anna), usecase.Input{"page_id": rootID, "parent_id": child.String()}); code(err) != "pages.parent_cycle" {
		t.Fatalf("a page moved under its own: %v", err)
	}
	if _, err := doors.Invoke(ctx, MovePageName, as(anna), usecase.Input{"page_id": child.String(), "after_id": "first"}); err == nil {
		t.Fatal("a sibling that is no id was accepted")
	}
	moved, err := doors.Invoke(ctx, MovePageName, as(anna), usecase.Input{"page_id": child.String()})
	if err != nil || moved["parent_id"] != nil {
		t.Fatalf("moving to the top: %v %v", moved, err)
	}

	found, err := doors.Invoke(ctx, SearchPagesName, as(bert), usecase.Input{"text": "budget", "limit": 10})
	if err != nil || len(found["data"].([]usecase.Output)) != 1 {
		t.Fatalf("finding by a word: %v %v", found, err)
	}
	if _, err := doors.Invoke(ctx, SearchPagesName, as(bert), usecase.Input{"text": "budget", "updated_from": "last week"}); code(err) != "pages.search_moment_malformed" {
		t.Fatalf("a moment of no shape: %v", err)
	}

	if _, err := doors.Invoke(ctx, TrashPageName, as(bert), usecase.Input{"page_id": "a page"}); err == nil {
		t.Fatal("a page that is no id was trashed")
	}
	trashed, err := doors.Invoke(ctx, TrashPageName, as(anna), usecase.Input{"page_id": rootID})
	if err != nil || trashed.Int("trashed") < 1 {
		t.Fatalf("trashing: %v %v", trashed, err)
	}
	if out, err := doors.Invoke(ctx, ListTrashedPagesName, as(bert), usecase.Input{"hub_id": hub.String()}); err != nil || out["data"] == nil {
		t.Fatalf("the trash: %v %v", out, err)
	}
	if _, err := doors.Invoke(ctx, ListTrashedPagesName, as(bert), usecase.Input{"hub_id": private.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert read the private hub's trash: %v", err)
	}
	restored, err := doors.Invoke(ctx, RestorePageName, as(anna), usecase.Input{"page_id": rootID})
	if err != nil || restored["trashed_at"] != nil {
		t.Fatalf("restoring: %v %v", restored, err)
	}

	for _, entry := range f.trail.entries {
		if body, ok := entry.Changes[domain.FieldBody].(map[string]any); ok {
			if _, open := body["to"]; open {
				t.Fatalf("%s carried a body in clear text", entry.Action)
			}
		}
	}
}

// UC-DOC-12 checks 1 and 3 through the doors: a writer verifies a page until a day with its owner;
// a day of no shape and a day already past are refused and write nothing; an empty day takes the
// verification away.
func TestAPageIsVerifiedThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := pageDoors(t, f.space)
	page := f.create(t, anna, hub, "", "Expenses", "Receipts within a month")
	hidden := f.create(t, anna, private, "", "Mine", "x")

	if _, err := doors.Invoke(ctx, VerifyPageName, as(anna), usecase.Input{"page_id": page.ID.String(), "verified_until": "soon"}); code(err) != "pages.verified_until_malformed" {
		t.Fatalf("a day of no shape: %v", err)
	}
	if _, err := doors.Invoke(ctx, VerifyPageName, as(anna), usecase.Input{
		"page_id": page.ID.String(), "verified_until": start.Add(-time.Hour).Format(time.RFC3339),
	}); err == nil {
		t.Fatal("a verification already past was accepted")
	}
	if _, err := doors.Invoke(ctx, VerifyPageName, as(bert), usecase.Input{"page_id": hidden.ID.String(), "verified_until": start.Add(72 * time.Hour).Format(time.RFC3339)}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert verified a page of the private hub: %v", err)
	}
	if f.store.pages[page.ID].VerifiedUntil != nil {
		t.Fatal("a refused verification was written")
	}
	until := start.Add(30 * 24 * time.Hour)
	out, err := doors.Invoke(ctx, VerifyPageName, as(bert), usecase.Input{
		"page_id": page.ID.String(), "verified_until": until.Format(time.RFC3339), "owner_id": anna.String(),
	})
	if err != nil || out["verified_by"] != bert.String() || out["owner_id"] != anna.String() || out["verified_until"] == nil {
		t.Fatalf("verifying: %v %v", out, err)
	}
	cleared, err := doors.Invoke(ctx, VerifyPageName, as(bert), usecase.Input{"page_id": page.ID.String()})
	if err != nil || cleared["verified_until"] != nil || cleared["owner_id"] != anna.String() {
		t.Fatalf("taking the verification away: %v %v", cleared, err)
	}
}

// linkedPages answers backlinks to every page: one from a page of the shared hub, one from a page
// of the private hub, and one from an entry.
type linkedPages struct {
	*store
	entry shared.ID
}

func (l linkedPages) Backlinks(context.Context, string, shared.ID, int) ([]repository.Backlink, error) {
	return []repository.Backlink{
		{SourceKind: "PAGE", SourceID: "p-shared", HubID: hub, Strength: "MENTION"},
		{SourceKind: "PAGE", SourceID: "p-private", HubID: private, Strength: "MENTION"},
		{SourceKind: "ENTRY", SourceID: l.entry, EntryID: l.entry, Strength: "MENTION"},
	}, nil
}

func (l linkedPages) Summaries(context.Context, []shared.ID) ([]repository.Summary, error) {
	return []repository.Summary{{ID: "p-shared", HubID: hub, Title: "Travel"}, {ID: "p-private", HubID: private, Title: "Diary"}}, nil
}

type linkingEntries struct{ workrepo.References }

func (linkingEntries) Entries(_ context.Context, ids []shared.ID) ([]workrepo.EntrySummary, error) {
	out := make([]workrepo.EntrySummary, 0, len(ids))
	for _, id := range ids {
		out = append(out, workrepo.EntrySummary{ID: id, HubID: hub, CollectionID: "c-1", Title: "Book the flights"})
	}
	return out, nil
}

// UC-DOC-04 check 3 through the doors: what links here names a page and an entry the reader may
// see and leaves out a page of a hub out of their reach.
func TestWhatLinksHereShowsOnlyWhatTheReaderMaySee(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	page := f.create(t, anna, hub, "", "Budget", "x")
	entry := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b0e1")
	f.space.Pages, f.space.Entries = linkedPages{store: f.store, entry: entry}, linkingEntries{}
	doors := pageDoors(t, f.space)

	out, err := doors.Invoke(ctx, ListPageBacklinksName, as(bert), usecase.Input{"page_id": page.ID.String()})
	if err != nil {
		t.Fatalf("what links here: %v", err)
	}
	data := out["data"].([]usecase.Output)
	if len(data) != 2 || data[0]["title"] != "Travel" || data[1]["entry_id"] != entry.String() {
		t.Fatalf("Bert sees %v", data)
	}
	if all, _ := doors.Invoke(ctx, ListPageBacklinksName, as(anna), usecase.Input{"page_id": page.ID.String()}); len(all["data"].([]usecase.Output)) != 3 {
		t.Fatalf("Anna sees %v", all["data"])
	}
	if _, err := doors.Invoke(ctx, ListPageBacklinksName, as(bert), usecase.Input{"page_id": "it"}); err == nil {
		t.Fatal("a page that is no id was read")
	}
}

// UC-DOC-06 checks 2-4 through the doors: a page's history is listed, one version read and named,
// and an older one brought back as a version of its own; a page out of reach answers missing.
func TestAPagesHistoryIsReadAndRestoredThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := pageDoors(t, f.space)
	page := f.create(t, anna, hub, "", "Plan", "one")
	f.clock.at = f.clock.at.Add(time.Minute)
	if _, err := doors.Invoke(ctx, UpdatePageName, as(bert), usecase.Input{"page_id": page.ID.String(), "body": "two"}); err != nil {
		t.Fatal(err)
	}
	hidden := f.create(t, anna, private, "", "Mine", "x")

	if _, err := doors.Invoke(ctx, ListPageVersionsName, as(bert), usecase.Input{"page_id": hidden.ID.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert read a private page's history: %v", err)
	}
	if _, err := doors.Invoke(ctx, ListPageVersionsName, as(bert), usecase.Input{"page_id": page.ID.String()}); err != nil {
		t.Fatalf("listing the history: %v", err)
	}
	version, err := doors.Invoke(ctx, ReadPageVersionName, as(bert), usecase.Input{"page_id": page.ID.String(), "seq": 2})
	if err != nil || version["body"] != "two" {
		t.Fatalf("reading a version: %v %v", version, err)
	}
	if _, err := doors.Invoke(ctx, NamePageVersionName, as(bert), usecase.Input{"page_id": page.ID.String(), "seq": 1, "name": "Signed off"}); err != nil {
		t.Fatalf("naming a version: %v", err)
	}
	if f.store.versions[page.ID][0].Name != "Signed off" {
		t.Fatalf("the version is named %q", f.store.versions[page.ID][0].Name)
	}
	if _, err := doors.Invoke(ctx, NamePageVersionName, as(bert), usecase.Input{"page_id": hidden.ID.String(), "seq": 1, "name": "x"}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert named a private page's version: %v", err)
	}
	f.clock.at = f.clock.at.Add(time.Minute)
	if _, err := doors.Invoke(ctx, UpdatePageName, as(anna), usecase.Input{"page_id": page.ID.String(), "body": "three"}); err != nil {
		t.Fatal(err)
	}
	restored, err := doors.Invoke(ctx, RestorePageVersionName, as(anna), usecase.Input{"page_id": page.ID.String(), "seq": 1})
	if err != nil || restored["body"] != "one" {
		t.Fatalf("restoring: %v %v", restored, err)
	}
	if versions := f.store.versions[page.ID]; versions[len(versions)-1].Restored != 1 {
		t.Fatalf("the restore is not a version of its own: %+v", versions)
	}
}
