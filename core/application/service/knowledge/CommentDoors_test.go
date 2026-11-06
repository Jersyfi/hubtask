// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func commentDoors(t *testing.T, s Space) *usecase.Registry {
	t.Helper()
	doors, err := usecase.NewRegistry(nil,
		AddPageComment{s}.Descriptor(), ListPageComments{s}.Descriptor(), ChangePageComment{s}.Descriptor(),
		DeletePageComment{s}.Descriptor(), ListPageLabels{s}.Descriptor(), SetPageLabels{s}.Descriptor(),
		ListSpaceLabels{s}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return doors
}

// UC-DOC-11 checks 1-3 through the doors: a comment stands inline with the words it is about and
// a reply joins its thread; only its author changes it or takes it back, whoever writes the page
// resolves the thread; taken back it answers no words; a page out of reach takes none.
func TestAPageIsDiscussedThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := commentDoors(t, f.space)
	page := f.create(t, anna, hub, "", "Plan", "the budget")
	hidden := f.create(t, anna, private, "", "Mine", "x")

	if _, err := doors.Invoke(ctx, AddPageCommentName, as(bert), usecase.Input{"page_id": hidden.ID.String(), "body": "Hello"}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert commented on a page out of reach: %v", err)
	}
	if _, err := doors.Invoke(ctx, AddPageCommentName, as(bert), usecase.Input{"page_id": page.ID.String(), "body": "x", "parent_id": "the first"}); err == nil {
		t.Fatal("a parent that is no id was accepted")
	}
	if len(f.store.comments) != 0 {
		t.Fatal("a refused comment was written")
	}
	thread, err := doors.Invoke(ctx, AddPageCommentName, as(anna), usecase.Input{
		"page_id": page.ID.String(), "body": "Why so high?", "anchor": "block-1", "quote": "the budget",
	})
	if err != nil || thread["anchor"] != "block-1" || thread["quote"] != "the budget" || thread["resolved"] != false {
		t.Fatalf("an inline comment: %v %v", thread, err)
	}
	reply, err := doors.Invoke(ctx, AddPageCommentName, as(bert), usecase.Input{"page_id": page.ID.String(), "body": "Flights", "parent_id": thread.String("id")})
	if err != nil || reply["parent_id"] != thread.String("id") {
		t.Fatalf("a reply: %v %v", reply, err)
	}
	if _, err := doors.Invoke(ctx, ListPageCommentsName, as(bert), usecase.Input{"page_id": page.ID.String()}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if _, err := doors.Invoke(ctx, ListPageCommentsName, as(bert), usecase.Input{"page_id": hidden.ID.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Bert listed comments out of reach: %v", err)
	}

	if _, err := doors.Invoke(ctx, ChangePageCommentName, as(bert), usecase.Input{"comment_id": thread.String("id"), "body": "Mine now"}); code(err) != "pages.comment_not_own" {
		t.Fatalf("Bert rewrote Anna's comment: %v", err)
	}
	resolved, err := doors.Invoke(ctx, ChangePageCommentName, as(bert), usecase.Input{"comment_id": thread.String("id"), "resolved": true})
	if err != nil || resolved["resolved"] != true || resolved["resolved_by"] != bert.String() {
		t.Fatalf("resolving: %v %v", resolved, err)
	}
	if _, err := doors.Invoke(ctx, DeletePageCommentName, as(bert), usecase.Input{"comment_id": thread.String("id")}); code(err) != "pages.comment_not_own" {
		t.Fatalf("Bert took Anna's comment back: %v", err)
	}
	if _, err := doors.Invoke(ctx, DeletePageCommentName, as(anna), usecase.Input{"comment_id": thread.String("id")}); err != nil {
		t.Fatalf("taking it back: %v", err)
	}
	if c := f.store.comments[shared.ID(thread.String("id"))]; !c.IsDeleted() || CommentOutput(c)["body"] != "" {
		t.Fatalf("the comment taken back still speaks: %+v", c)
	}
}

// UC-DOC-01 check 9 through the doors: a writer labels a page, the labels are read back lower
// case without a new version, the space counts them, and a label of two words writes nothing.
func TestAPageIsLabelledThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := commentDoors(t, f.space)
	page := f.create(t, anna, hub, "", "Travel guide", "x")

	if _, err := doors.Invoke(ctx, SetPageLabelsName, as(bert), usecase.Input{"page_id": page.ID.String(), "labels": []any{"two words"}}); code(err) != "pages.label_malformed" {
		t.Fatalf("a label of two words: %v", err)
	}
	if _, err := doors.Invoke(ctx, SetPageLabelsName, as(bert), usecase.Input{"page_id": page.ID.String(), "labels": []any{7.0}}); err == nil {
		t.Fatal("a label that is no text was accepted")
	}
	if len(f.store.labels[page.ID]) != 0 {
		t.Fatal("a refused label was written")
	}
	set, err := doors.Invoke(ctx, SetPageLabelsName, as(bert), usecase.Input{"page_id": page.ID.String(), "labels": []any{"HR", "how-to"}})
	if err != nil || !slices.Equal(set["labels"].([]string), []string{"how-to", "hr"}) {
		t.Fatalf("labelling: %v %v", set, err)
	}
	if read, err := doors.Invoke(ctx, ListPageLabelsName, as(anna), usecase.Input{"page_id": page.ID.String()}); err != nil || len(read["labels"].([]string)) != 2 {
		t.Fatalf("reading the labels: %v %v", read, err)
	}
	if f.store.pages[page.ID].Version != page.Version {
		t.Fatal("labelling made a new version")
	}
	facet, err := doors.Invoke(ctx, ListSpaceLabelsName, as(bert), usecase.Input{"hub_id": hub.String()})
	if err != nil || len(facet["data"].([]usecase.Output)) != 2 {
		t.Fatalf("the space's labels: %v %v", facet, err)
	}
	if _, err := doors.Invoke(ctx, ListSpaceLabelsName, as(bert), usecase.Input{"hub_id": private.String()}); err == nil {
		t.Fatal("Bert counted the private hub's labels")
	}
}
