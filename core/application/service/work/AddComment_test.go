// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// comments is the in-memory fake of the comment store. Like the others, it records what it was
// given, because what the use case owes is not only a return value.
type comments struct {
	stored   map[shared.ID]domain.Comment
	inserted []domain.Comment
	// bodies and tombstones record every SetBody and SetDeleted with the version they were
	// written against; both also honour the optimistic lock the statements behind them take.
	bodies     []attributeVersion
	tombstones []attributeVersion
}

// attributeVersion is one comment write: what was stored, against which version.
type attributeVersion struct {
	comment         domain.Comment
	expectedVersion int
}

func newComments() *comments {
	return &comments{stored: map[shared.ID]domain.Comment{}}
}

func (c *comments) Find(_ context.Context, id shared.ID) (domain.Comment, error) {
	comment, found := c.stored[id]
	if !found {
		return domain.Comment{}, shared.ErrNotFound
	}
	return comment, nil
}

func (c *comments) List(
	_ context.Context, itemID shared.ID, _ repository.Page,
) (repository.CommentPage, error) {
	page := repository.CommentPage{}
	for _, comment := range c.stored {
		if comment.ItemID == itemID {
			page.Comments = append(page.Comments, comment)
		}
	}
	return page, nil
}

func (c *comments) Insert(_ context.Context, comment domain.Comment) error {
	c.inserted = append(c.inserted, comment)
	c.stored[comment.ID] = comment
	return nil
}

func (c *comments) SetBody(_ context.Context, comment domain.Comment, expectedVersion int) error {
	stored, found := c.stored[comment.ID]
	if !found || stored.Version != expectedVersion || stored.DeletedAt != nil {
		return shared.ErrVersionConflict.WithDetail("comments.version_conflict")
	}
	c.bodies = append(c.bodies, attributeVersion{comment: comment, expectedVersion: expectedVersion})
	written := comment
	written.Version = expectedVersion + 1
	c.stored[comment.ID] = written
	return nil
}

func (c *comments) SetDeleted(_ context.Context, comment domain.Comment, expectedVersion int) error {
	stored, found := c.stored[comment.ID]
	if !found || stored.Version != expectedVersion || stored.DeletedAt != nil {
		return shared.ErrVersionConflict.WithDetail("comments.version_conflict")
	}
	c.tombstones = append(c.tombstones, attributeVersion{comment: comment, expectedVersion: expectedVersion})
	written := comment
	written.Version = expectedVersion + 1
	c.stored[comment.ID] = written
	return nil
}

// commentProfiles is the fixture with COMMENTS where the capability matrix grants it: not on an
// activity (domain-model.md §2).
func commentProfiles() []domain.CapabilityProfile {
	rows := systemProfiles()
	for i, row := range rows {
		if row.Type != domain.ItemActivity {
			rows[i].Capabilities = append(row.Capabilities, domain.CapabilityComments)
		}
	}
	return rows
}

type commentHarness struct {
	writer     CommentWriter
	comments   *comments
	items      *items
	containers *containers
	events     *events
	changes    *changes
	audit      *sink
	history    *journal
	authorizer *authorizer
}

func newCommentHarness(t *testing.T) *commentHarness {
	t.Helper()

	h := &commentHarness{
		comments:   newComments(),
		items:      &items{stored: map[shared.ID]domain.WorkItem{}},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		events:     &events{}, changes: &changes{}, audit: &sink{}, history: &journal{},
		authorizer: &authorizer{},
	}
	h.writer = CommentWriter{
		Comments: h.comments, Items: h.items, Containers: h.containers,
		Profiles: &profiles{rows: commentProfiles()}, Authorizer: h.authorizer,
		Events: h.events, Changes: h.changes, Audit: h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	return h
}

func (h *commentHarness) withItem(itemType domain.ItemType) domain.WorkItem {
	item := domain.WorkItem{
		ID: assignedItem, TenantID: tenantID, CollectionID: collectionID, Type: itemType,
		Path: domain.RootPath(assignedItem), Depth: 1, Title: "Buy oat milk",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[assignedItem] = item
	return item
}

func (h *commentHarness) withComment(id shared.ID, author shared.ID, body string) domain.Comment {
	comment := domain.Comment{
		ID: id, TenantID: tenantID, ItemID: assignedItem, AuthorID: author,
		Body: body, CreatedAt: now, Version: 1,
	}
	h.comments.stored[id] = comment
	return comment
}

func addCmd(body string) AddCommentCommand {
	return AddCommentCommand{ItemID: assignedItem, Body: body}
}

// One write owes four things - the row, the event, the change log entry and the audit entry -
// plus the one comment verb of the item's history.
func TestCommentingWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)

	comment, err := AddComment{Writer: h.writer}.Execute(t.Context(), actor(), addCmd("Looks good."))
	if err != nil {
		t.Fatalf("commenting failed: %v", err)
	}

	if comment.Body != "Looks good." || comment.ItemID != assignedItem ||
		comment.AuthorID != accountID || comment.Version != 1 {
		t.Errorf("created %+v", comment)
	}
	if len(h.comments.inserted) != 1 {
		t.Fatalf("rows written: %d", len(h.comments.inserted))
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.CommentCreated {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	payload := h.events.appended[0].Payload
	if payload["body"] != "Looks good." || payload["item_id"] != assignedItem.String() ||
		payload["collection_id"] != collectionID.String() {
		t.Errorf("the event payload is %+v", payload)
	}
	if len(h.changes.recorded) != 1 {
		t.Fatalf("changes recorded: %d", len(h.changes.recorded))
	}
	change := h.changes.recorded[0]
	if change.Entity != "comment" || change.EntityID != comment.ID ||
		change.ContainerID != collectionID {
		t.Errorf("the change entry is %+v", change)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != CommentCreatedAction {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}
	if len(h.history.entries) != 1 || h.history.entries[0].Verb != activity.ItemCommented {
		t.Fatalf("history steps: %+v", h.history.entries)
	}
}

// Rule 10: the body is user content. The audit entry records a fingerprint of it, never the text.
func TestTheCommentAuditEntryCarriesNoText(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)

	_, err := AddComment{Writer: h.writer}.Execute(t.Context(), actor(), addCmd("A secret plan"))
	if err != nil {
		t.Fatal(err)
	}

	recorded, ok := h.audit.entries[0].Changes["body"].(map[string]any)
	if !ok {
		t.Fatalf("the entry has no body change: %+v", h.audit.entries[0].Changes)
	}
	for _, value := range recorded {
		if value == "A secret plan" {
			t.Fatal("the audit trail carries the comment's text in the open")
		}
	}
}

func TestAReplyThreadsOneLevel(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	top := h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "First")

	cmd := addCmd("Second")
	cmd.ParentCommentID = top.ID
	reply, err := AddComment{Writer: h.writer}.Execute(t.Context(), actor(), cmd)
	if err != nil {
		t.Fatalf("replying failed: %v", err)
	}
	if reply.ParentCommentID != top.ID {
		t.Errorf("the reply hangs from %q", reply.ParentCommentID)
	}

	nested := addCmd("Third")
	nested.ParentCommentID = reply.ID
	h.comments.stored[reply.ID] = reply
	_, err = AddComment{Writer: h.writer}.Execute(t.Context(), actor(), nested)
	assertValidation(t, err, "comments.reply_to_reply")
}

func TestAReplyToAMissingParentIsRefusedByName(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)

	cmd := addCmd("Answering nothing")
	cmd.ParentCommentID = "0192f000-0000-7000-8000-0000000000b9"
	_, err := AddComment{Writer: h.writer}.Execute(t.Context(), actor(), cmd)
	if got := shared.AsError(err).DetailCode; got != "comments.parent_not_found" {
		t.Fatalf("refused as %q", got)
	}
	if len(h.comments.inserted) != 0 {
		t.Error("the refused reply still wrote a row")
	}
}

// The acceptance criterion of C-03: an activity carries no discussion.
func TestACommentOnAnActivityIsRefused(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemActivity)

	_, err := AddComment{Writer: h.writer}.Execute(t.Context(), actor(), addCmd("Chatty"))
	assertValidation(t, err, "items.capability_not_supported")
	if len(h.comments.inserted)+len(h.events.appended) != 0 {
		t.Error("the refusal still wrote something")
	}
}

func TestCommentingAsksThePermissionQuestionFirst(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := AddComment{Writer: h.writer}.Execute(t.Context(), actor(), addCmd("Denied"))
	if err == nil {
		t.Fatal("a refused actor commented anyway")
	}
	if len(h.authorizer.requests) != 1 ||
		h.authorizer.requests[0].TokenScope != "comments:write" {
		t.Fatalf("the permission question was %+v", h.authorizer.requests)
	}
	if len(h.comments.inserted) != 0 {
		t.Error("the refusal still wrote a row")
	}
}

// The channel adapter: the output is the contract's shape, in the contract's words.
func TestTheCommentChannelSpeaksTheContract(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)

	out, err := AddComment{Writer: h.writer}.Descriptor().Handler.Invoke(
		t.Context(), actor(), map[string]any{
			"item_id": assignedItem.String(), "body": "Looks good.",
		})
	if err != nil {
		t.Fatalf("the channel refused: %v", err)
	}
	for _, field := range []string{
		"id", "item_id", "author_id", "parent_comment_id", "body",
		"created_at", "edited_at", "deleted_at", "version",
	} {
		if _, present := out[field]; !present {
			t.Errorf("%s is missing from the output", field)
		}
	}
	if out["body"] != "Looks good." {
		t.Errorf("body = %v", out["body"])
	}
}

// The reader: the right to read the entry is the right to read its discussion, and a tombstone
// travels without a body rather than being filtered out.
func TestListingCommentsSpeaksTheContractTombstonesIncluded(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "Still here")
	gone := h.withComment("0192f000-0000-7000-8000-0000000000b2", accountID, "Removed")
	h.comments.stored[gone.ID] = gone.Removed(now)

	reader := ListComments{
		Comments: h.comments, Items: h.items, Containers: h.containers,
		Authorizer: h.authorizer, UnitOfWork: &unitOfWork{},
	}
	out, err := reader.Descriptor().Handler.Invoke(t.Context(), actor(), map[string]any{
		"item_id": assignedItem.String(),
	})
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}

	if len(h.authorizer.requests) != 1 ||
		h.authorizer.requests[0].TokenScope != "items:read" {
		t.Fatalf("the permission question was %+v", h.authorizer.requests)
	}

	rows, ok := out["data"].([]usecase.Output)
	if !ok || len(rows) != 2 {
		t.Fatalf("the page carries %+v", out["data"])
	}
	for _, row := range rows {
		if row["id"] == gone.ID.String() {
			if row["body"] != nil || row["deleted_at"] == nil {
				t.Errorf("the tombstone travels as %+v", row)
			}
		} else if row["body"] == nil {
			t.Errorf("a living comment lost its body: %+v", row)
		}
	}
}
