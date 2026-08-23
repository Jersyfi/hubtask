// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// moderation is the role question as the comment changes see it. It records whether it was asked,
// because the author path must not consult it at all - the author's right is not a role.
type moderation struct {
	role  identity.Role
	found bool
	asked int
}

func (m *moderation) RoleAlong(
	context.Context, appshared.ActorContext, []identity.Scope,
) (identity.Role, bool, error) {
	m.asked++
	return m.role, m.found, nil
}

// withModeration wires the harness for the author-or-administrator rule.
func (h *commentHarness) withModeration(role identity.Role, found bool) *moderation {
	m := &moderation{role: role, found: found}
	h.writer.Moderation = m
	return m
}

func editCmd(commentID shared.ID, body string) ChangeCommentCommand {
	return ChangeCommentCommand{ItemID: assignedItem, CommentID: commentID, Body: body}
}

func TestTheAuthorEditsTheirComment(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	moderator := h.withModeration("", false)
	comment := h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "Frist")

	edited, err := EditComment{Writer: h.writer}.Execute(
		t.Context(), actor(), editCmd(comment.ID, "First"))
	if err != nil {
		t.Fatalf("the author's edit was refused: %v", err)
	}

	if edited.Body != "First" || edited.EditedAt == nil || edited.Version != 2 {
		t.Errorf("edited %+v", edited)
	}
	if moderator.asked != 0 {
		t.Error("the author's own comment put the role question - the author's right is not a role")
	}
	if len(h.comments.bodies) != 1 || h.comments.bodies[0].expectedVersion != 1 {
		t.Fatalf("body writes: %+v", h.comments.bodies)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.CommentUpdated {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != CommentUpdatedAction {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}
	if len(h.history.entries) != 0 {
		t.Error("an edit wrote history - the comment's own edited_at is that record")
	}
}

// The acceptance criterion of C-03: a third party is refused with access.not_permitted - the same
// code and status as any other missing permission - and an administrator succeeds.
func TestOnlyTheAuthorOrAnAdministratorChangesAComment(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	moderator := h.withModeration(identity.RoleMember, true)
	comment := h.withComment("0192f000-0000-7000-8000-0000000000b1", strangerID, "Not yours")

	_, err := EditComment{Writer: h.writer}.Execute(
		t.Context(), actor(), editCmd(comment.ID, "Defaced"))
	if !errors.Is(err, shared.ErrForbidden) ||
		shared.AsError(err).DetailCode != "access.not_permitted" {
		t.Fatalf("refused as %v, want access.not_permitted", err)
	}
	if moderator.asked != 1 {
		t.Error("the role question was not put")
	}
	if len(h.comments.bodies)+len(h.events.appended) != 0 {
		t.Error("the refusal still wrote something")
	}
	// The refusal is in the trail, exactly as a permission refusal is (audit.md §4).
	if len(h.audit.entries) != 1 || h.audit.entries[0].Outcome != audit.OutcomeDenied {
		t.Fatalf("the trail carries %+v, want the DENIED entry", h.audit.entries)
	}

	h.withModeration(identity.RoleAdmin, true)
	edited, err := EditComment{Writer: h.writer}.Execute(
		t.Context(), actor(), editCmd(comment.ID, "Moderated"))
	if err != nil {
		t.Fatalf("the administrator's edit was refused: %v", err)
	}
	if edited.Body != "Moderated" {
		t.Errorf("edited %+v", edited)
	}
	// The author survives the moderation: an administrator edits the words, not the attribution.
	if edited.AuthorID != strangerID {
		t.Errorf("the edit reattributed the comment to %q", edited.AuthorID)
	}
}

func TestATombstoneCannotBeEdited(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	h.withModeration("", false)
	comment := h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "Gone soon")
	h.comments.stored[comment.ID] = comment.Removed(now)

	_, err := EditComment{Writer: h.writer}.Execute(
		t.Context(), actor(), editCmd(comment.ID, "Back again"))
	if got := shared.AsError(err).DetailCode; got != "comments.comment_deleted" {
		t.Fatalf("refused as %q", got)
	}
}

func TestDeletionAnnouncesTheTombstone(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	h.withModeration("", false)
	comment := h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "Take this down")

	deleted, err := DeleteComment{Writer: h.writer}.Execute(
		t.Context(), actor(), editCmd(comment.ID, ""))
	if err != nil {
		t.Fatalf("deleting failed: %v", err)
	}

	if deleted.Body != "" || deleted.DeletedAt == nil || deleted.Version != 2 {
		t.Errorf("the tombstone is %+v", deleted)
	}
	if len(h.comments.tombstones) != 1 {
		t.Fatalf("tombstone writes: %+v", h.comments.tombstones)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.CommentDeleted {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	if body := h.events.appended[0].Payload["body"]; body != nil {
		t.Errorf("the deletion event carries the text: %v", body)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != CommentDeletedAction {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}
}

func TestDeletingTwiceAnnouncesNothing(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	h.withModeration("", false)
	comment := h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "Once")
	h.comments.stored[comment.ID] = comment.Removed(now)

	again, err := DeleteComment{Writer: h.writer}.Execute(
		t.Context(), actor(), editCmd(comment.ID, ""))
	if err != nil {
		t.Fatalf("the repeat was refused: %v", err)
	}
	if len(h.comments.tombstones)+len(h.events.appended)+len(h.audit.entries) != 0 {
		t.Error("a no-op wrote or announced something")
	}
	if again.DeletedAt == nil {
		t.Error("the repeat answered with a living comment")
	}

	// The If-Match is still honoured on the no-op.
	stale := editCmd(comment.ID, "")
	stale.ExpectedVersion = 7
	_, err = DeleteComment{Writer: h.writer}.Execute(t.Context(), actor(), stale)
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale If-Match on a no-op was accepted: %v", err)
	}
}

// T-04, one level down: a comment reached through another entry's route is the same answer as a
// comment that does not exist.
func TestACommentReachedThroughAnotherEntryIsNotFound(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	h.withModeration("", false)
	comment := h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "Here")

	cmd := editCmd(comment.ID, "There")
	cmd.ItemID = "0192f000-0000-7000-8000-0000000000ff"
	_, err := EditComment{Writer: h.writer}.Execute(t.Context(), actor(), cmd)
	if got := shared.AsError(err).DetailCode; got != "comments.not_found" {
		t.Fatalf("answered %q, want the same answer as a missing comment", got)
	}
}

// Both changes reach every channel through the same untyped input.
func TestTheCommentChangeChannelsReachTheSameCommand(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	h.withModeration("", false)
	comment := h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "Frist")

	out, err := EditComment{Writer: h.writer}.Descriptor().Handler.Invoke(
		t.Context(), actor(), map[string]any{
			"item_id": assignedItem.String(), "comment_id": comment.ID.String(), "body": "First",
		})
	if err != nil {
		t.Fatalf("the edit channel refused: %v", err)
	}
	if out["body"] != "First" || out["version"] != 2 {
		t.Errorf("the channel answered %+v", out)
	}

	out, err = DeleteComment{Writer: h.writer}.Descriptor().Handler.Invoke(
		t.Context(), actor(), map[string]any{
			"item_id": assignedItem.String(), "comment_id": comment.ID.String(),
		})
	if err != nil {
		t.Fatalf("the delete channel refused: %v", err)
	}
	if out["body"] != nil || out["deleted_at"] == nil {
		t.Errorf("the tombstone travels as %+v", out)
	}
}
