// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var commentedAt = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

func draftComment(body string, parent *work.Comment) work.NewCommentInput {
	return work.NewCommentInput{
		ID: "c1", TenantID: "t1", ItemID: "i1", AuthorID: "a1",
		Parent: parent, Body: body, Now: commentedAt, Text: text.Composing{},
	}
}

func TestACommentIsBuiltWithItsRules(t *testing.T) {
	comment, err := work.NewComment(draftComment("Looks good.\nShip it.", nil))
	if err != nil {
		t.Fatalf("building failed: %v", err)
	}
	if comment.Body != "Looks good.\nShip it." || comment.Version != 1 ||
		comment.CreatedAt != commentedAt {
		t.Errorf("built %+v", comment)
	}
	if comment.EditedAt != nil || comment.DeletedAt != nil {
		t.Error("a fresh comment carries stamps it has not earned")
	}
}

func TestTheBodyRulesCountCodePoints(t *testing.T) {
	// 20000 two-byte code points: precisely at the limit in code points and 40000 bytes long -
	// a limit that counted bytes would refuse it (I-W7).
	atLimit := strings.Repeat("ü", 20000)
	if _, err := work.NewComment(draftComment(atLimit, nil)); err != nil {
		t.Fatalf("a body at the limit was refused: %v", err)
	}

	over := strings.Repeat("a", 20001)
	_, err := work.NewComment(draftComment(over, nil))
	if got := shared.AsError(err).DetailCode; got != "comments.body_too_long" {
		t.Fatalf("refused as %q, want comments.body_too_long", got)
	}

	_, err = work.NewComment(draftComment("   \n  ", nil))
	if got := shared.AsError(err).DetailCode; got != "comments.body_required" {
		t.Fatalf("blank refused as %q, want comments.body_required", got)
	}
}

// The body is stored as sent apart from its form: normal form C, on creation and on edit
// (i18n-l10n.md §5, M-07), so the length is the length of what a person sees and two spellings
// of a word compare equal. Without the port, a body that is not ASCII is refused.
func TestTheBodyIsStoredInNormalFormC(t *testing.T) {
	comment, err := work.NewComment(draftComment("Scho\u0308n!\nDanke fu\u0308r alles.", nil))
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if comment.Body != "Sch\u00f6n!\nDanke f\u00fcr alles." {
		t.Errorf("body = %q, want it composed with its newline kept", comment.Body)
	}

	edited, err := comment.Edited("Gru\u0308\u00dfe", text.Composing{}, commentedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("editing: %v", err)
	}
	if edited.Body != "Gr\u00fc\u00dfe" {
		t.Errorf("edited body = %q, want it composed", edited.Body)
	}

	in := draftComment("Scho\u0308n", nil)
	in.Text = nil
	if _, err := work.NewComment(in); shared.AsError(err).DetailCode != "text.normalizer_missing" {
		t.Errorf("without a port the decomposed body was accepted: %v", err)
	}
}

func TestThreadingIsOneLevel(t *testing.T) {
	top := work.Comment{ID: "p1", ItemID: "i1", Body: "First"}

	reply, err := work.NewComment(draftComment("Second", &top))
	if err != nil || reply.ParentCommentID != "p1" {
		t.Fatalf("replying failed: %v (%+v)", err, reply)
	}

	nested := work.Comment{ID: "p2", ItemID: "i1", ParentCommentID: "p1", Body: "Second"}
	_, err = work.NewComment(draftComment("Third", &nested))
	if got := shared.AsError(err).DetailCode; got != "comments.reply_to_reply" {
		t.Fatalf("a reply to a reply was refused as %q", got)
	}

	elsewhere := work.Comment{ID: "p3", ItemID: "other", Body: "First"}
	_, err = work.NewComment(draftComment("Stray", &elsewhere))
	if got := shared.AsError(err).DetailCode; got != "comments.parent_not_on_item" {
		t.Fatalf("a cross-item reply was refused as %q", got)
	}

	gone := work.Comment{ID: "p4", ItemID: "i1", DeletedAt: &commentedAt}
	_, err = work.NewComment(draftComment("Answering silence", &gone))
	if got := shared.AsError(err).DetailCode; got != "comments.parent_deleted" {
		t.Fatalf("a reply to a tombstone was refused as %q", got)
	}
}

func TestAnEditRewritesTheBodyAndNothingElse(t *testing.T) {
	comment, err := work.NewComment(draftComment("Frist", nil))
	if err != nil {
		t.Fatal(err)
	}

	editedAt := commentedAt.Add(time.Minute)
	edited, err := comment.Edited("First", text.Composing{}, editedAt)
	if err != nil {
		t.Fatalf("editing failed: %v", err)
	}
	if edited.Body != "First" || edited.EditedAt == nil || !edited.EditedAt.Equal(editedAt) {
		t.Errorf("edited %+v", edited)
	}
	if edited.CreatedAt != comment.CreatedAt {
		t.Error("the edit moved the creation")
	}

	if _, err := comment.Edited("", text.Composing{}, editedAt); err == nil {
		t.Error("an empty body got through the edit")
	}
}

func TestDeletionLeavesAReadableTombstone(t *testing.T) {
	comment, err := work.NewComment(draftComment("Take this down", nil))
	if err != nil {
		t.Fatal(err)
	}
	editedAt := commentedAt.Add(time.Minute)
	comment, err = comment.Edited("Take this down, please", text.Composing{}, editedAt)
	if err != nil {
		t.Fatal(err)
	}

	deletedAt := commentedAt.Add(time.Hour)
	removed := comment.Removed(deletedAt)
	if removed.Body != "" {
		t.Error("the text survived its deletion")
	}
	if removed.DeletedAt == nil || !removed.DeletedAt.Equal(deletedAt) {
		t.Error("the tombstone carries no deletion stamp")
	}
	if removed.ID != comment.ID || removed.AuthorID != comment.AuthorID ||
		removed.CreatedAt != comment.CreatedAt || removed.EditedAt == nil {
		t.Errorf("the tombstone lost its identity: %+v", removed)
	}

	// Idempotent: deleting the tombstone is the state that is already there.
	again := removed.Removed(deletedAt.Add(time.Hour))
	if !again.DeletedAt.Equal(deletedAt) {
		t.Error("a second deletion moved the stamp")
	}

	// And the text cannot be edited back into existence.
	_, err = removed.Edited("I take it back", text.Composing{}, deletedAt.Add(time.Hour))
	if got := shared.AsError(err).DetailCode; got != "comments.comment_deleted" {
		t.Fatalf("editing the tombstone was refused as %q", got)
	}
}

func TestOnlyACommentableTypeCarriesADiscussion(t *testing.T) {
	item := work.WorkItem{ID: "i1"}

	chatty := work.CapabilityProfile{Capabilities: []work.Capability{work.CapabilityComments}}
	if err := item.EnsureCommentable(chatty); err != nil {
		t.Fatalf("a commentable type was refused: %v", err)
	}

	silent := work.CapabilityProfile{Type: work.ItemActivity}
	err := item.EnsureCommentable(silent)
	if got := shared.AsError(err).DetailCode; got != "items.capability_not_supported" {
		t.Fatalf("refused as %q, want items.capability_not_supported", got)
	}
}
