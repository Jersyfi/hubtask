// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	media "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	attachedItem = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	receiptID    = shared.MustParseID("0192f000-0000-7000-8000-0000000000d2")
)

// attachProfiles is the shared fixture with ATTACHMENTS where the capability matrix grants it: a
// task and a work package carry files, an activity does not (domain-model.md §2).
func attachProfiles() []domain.CapabilityProfile {
	rows := systemProfiles()
	for i, row := range rows {
		if row.Type == domain.ItemTask || row.Type == domain.ItemWorkPackage {
			rows[i].Capabilities = append(row.Capabilities, domain.CapabilityAttachments)
		}
	}
	return rows
}

// attachments is the fake link table and its OR-set tags. It keeps both, because writing one
// without the other is the failure the port exists to prevent: a link with no tag merges as last
// writer wins and loses a concurrent change.
type attachments struct {
	carried  map[shared.ID][]shared.ID
	elements map[shared.ID]map[shared.ID]domain.SetElement
}

func newAttachments() *attachments {
	return &attachments{
		carried:  map[shared.ID][]shared.ID{},
		elements: map[shared.ID]map[shared.ID]domain.SetElement{},
	}
}

func (a *attachments) Add(
	_ context.Context, itemID, mediaID shared.ID, tag shared.HLC,
) (bool, error) {
	fresh := !slices.Contains(a.carried[itemID], mediaID)
	if fresh {
		a.carried[itemID] = append(a.carried[itemID], mediaID)
	}
	a.tag(itemID, mediaID, func(element *domain.SetElement) {
		element.AddedAt, element.RemovedAt = tag, shared.HLC{}
	})
	return fresh, nil
}

func (a *attachments) Remove(
	_ context.Context, itemID, mediaID shared.ID, tag shared.HLC,
) (bool, error) {
	carried := slices.Contains(a.carried[itemID], mediaID)
	a.carried[itemID] = slices.DeleteFunc(a.carried[itemID], func(id shared.ID) bool {
		return id == mediaID
	})
	a.tag(itemID, mediaID, func(element *domain.SetElement) { element.RemovedAt = tag })
	return carried, nil
}

func (a *attachments) MediaIDs(_ context.Context, itemID shared.ID) ([]shared.ID, error) {
	return slices.Clone(a.carried[itemID]), nil
}

func (a *attachments) Elements(
	_ context.Context, itemID shared.ID,
) ([]domain.SetElement, error) {
	elements := make([]domain.SetElement, 0, len(a.elements[itemID]))
	for _, element := range a.elements[itemID] {
		elements = append(elements, element)
	}
	return elements, nil
}

func (a *attachments) tag(itemID, mediaID shared.ID, apply func(*domain.SetElement)) {
	if a.elements[itemID] == nil {
		a.elements[itemID] = map[shared.ID]domain.SetElement{}
	}
	element := a.elements[itemID][mediaID]
	element.ElementID = mediaID
	apply(&element)
	a.elements[itemID][mediaID] = element
}

type attachmentHarness struct {
	attach      AttachMedia
	detach      DetachMedia
	items       *items
	containers  *containers
	attachments *attachments
	media       *mediaObjects
	events      *events
	changes     *changes
	audit       *sink
	history     *journal
}

func newAttachmentHarness(t *testing.T) *attachmentHarness {
	t.Helper()

	h := &attachmentHarness{
		items:       &items{stored: map[shared.ID]domain.WorkItem{}},
		containers:  &containers{stored: map[shared.ID]domain.Container{}},
		attachments: newAttachments(),
		media:       newMediaObjects(),
		events:      &events{}, changes: &changes{}, audit: &sink{}, history: &journal{},
	}

	writer := ItemAttachmentWriter{
		Items: h.items, Containers: h.containers, Profiles: &profiles{rows: attachProfiles()},
		Attachments: h.attachments, Media: h.media, Authorizer: &authorizer{},
		Events: h.events, Changes: h.changes, Audit: h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.attach = AttachMedia{Writer: writer}
	h.detach = DetachMedia{Writer: writer}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.media.stored[receiptID] = readyAttachment(receiptID)
	return h
}

func (h *attachmentHarness) withItem(itemType domain.ItemType) domain.WorkItem {
	item := domain.WorkItem{
		ID: attachedItem, TenantID: tenantID, CollectionID: collectionID, Type: itemType,
		Path: domain.RootPath(attachedItem), Depth: 1, Title: "File the expenses",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[attachedItem] = item
	return item
}

// readyAttachment is a media object that may join an entry.
func readyAttachment(id shared.ID) media.Object {
	object := readyCover(id)
	object.Usage = media.UsageAttachment
	return object
}

func attachCmd() AttachmentCommand {
	return AttachmentCommand{ItemID: attachedItem, MediaID: receiptID}
}

// One change owes five things, and this is the test that says so.
func TestAttachingWritesTheLinkTheCountTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newAttachmentHarness(t)
	h.withItem(domain.ItemTask)

	set, err := h.attach.Execute(t.Context(), actor(), attachCmd())
	if err != nil {
		t.Fatalf("attaching failed: %v", err)
	}

	if len(set.MediaIDs) != 1 || set.MediaIDs[0] != receiptID {
		t.Fatalf("the entry carries %v", set.MediaIDs)
	}
	// The bytes are shared, never copied: the count is what says how many entries point at them.
	if h.media.deltas[receiptID] != 1 {
		t.Errorf("the reference count moved by %d, want 1", h.media.deltas[receiptID])
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.AttachmentAdded {
		t.Fatalf("the announcement is %+v", h.events.appended)
	}
	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d change log entries, want 1", len(h.changes.recorded))
	}
	// The one element that moved, not the whole set: that is the OR-set merge rule written down
	// (offline-sync.md §4.2).
	payload := h.changes.recorded[0].Payload
	if payload["set"] != string(domain.SetAttachments) || payload["op"] != "add" {
		t.Errorf("the change log payload is %+v", payload)
	}
	if payload["element_id"] != receiptID.String() {
		t.Errorf("the change log names %v", payload["element_id"])
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ItemAttachmentAddedAction {
		t.Fatalf("the audit trail is %+v", h.audit.entries)
	}
	if len(h.history.entries) != 1 || h.history.entries[0].Verb != activity.ItemAttachmentAdded {
		t.Fatalf("the history is %+v", h.history.entries)
	}
	// The tag travels with the link: without it a file attached on one device is lost the moment
	// another detaches a different one.
	elements, _ := h.attachments.Elements(t.Context(), attachedItem)
	if len(elements) != 1 || elements[0].AddedAt.IsZero() {
		t.Errorf("the merge tags are %+v", elements)
	}
}

func TestDetachingRemovesTheLinkAndLowersTheCount(t *testing.T) {
	h := newAttachmentHarness(t)
	h.withItem(domain.ItemTask)
	if _, err := h.attach.Execute(t.Context(), actor(), attachCmd()); err != nil {
		t.Fatalf("attaching failed: %v", err)
	}

	set, err := h.detach.Execute(t.Context(), actor(), attachCmd())
	if err != nil {
		t.Fatalf("detaching failed: %v", err)
	}

	if len(set.MediaIDs) != 0 {
		t.Errorf("the entry still carries %v", set.MediaIDs)
	}
	// Back to nothing, and the object itself untouched: what decides its life is the count and the
	// reconciliation job, never this call.
	if h.media.deltas[receiptID] != 0 {
		t.Errorf("the reference count stands at %d, want 0", h.media.deltas[receiptID])
	}
	last := h.events.appended[len(h.events.appended)-1]
	if last.Type != event.AttachmentRemoved {
		t.Errorf("the announcement is %s", last.Type)
	}
	if verb := h.history.entries[len(h.history.entries)-1].Verb; verb != activity.ItemAttachmentRemoved {
		t.Errorf("the history verb is %s", verb)
	}
}

func TestAttachingWhatIsAlreadyAttachedAnnouncesNothingAndCountsOnce(t *testing.T) {
	h := newAttachmentHarness(t)
	h.withItem(domain.ItemTask)
	if _, err := h.attach.Execute(t.Context(), actor(), attachCmd()); err != nil {
		t.Fatalf("attaching failed: %v", err)
	}

	set, err := h.attach.Execute(t.Context(), actor(), attachCmd())
	if err != nil {
		t.Fatalf("attaching again failed: %v", err)
	}

	if len(set.MediaIDs) != 1 {
		t.Errorf("the entry carries %v", set.MediaIDs)
	}
	// The whole point of the port reporting whether the link was new: a repeat must not raise the
	// count a second time, or the object would never reach zero.
	if h.media.deltas[receiptID] != 1 {
		t.Errorf("the reference count moved to %d, want 1", h.media.deltas[receiptID])
	}
	if len(h.events.appended) != 1 {
		t.Errorf("%d announcements, want 1", len(h.events.appended))
	}
}

func TestDetachingWhatIsNotAttachedAnnouncesNothing(t *testing.T) {
	h := newAttachmentHarness(t)
	h.withItem(domain.ItemTask)

	if _, err := h.detach.Execute(t.Context(), actor(), attachCmd()); err != nil {
		t.Fatalf("detaching failed: %v", err)
	}

	if len(h.events.appended) != 0 || h.media.deltas[receiptID] != 0 {
		t.Error("a detachment of nothing announced or counted something")
	}
	// The tag is written all the same: a device that decided this has made a decision another
	// replica has to merge against.
	elements, _ := h.attachments.Elements(t.Context(), attachedItem)
	if len(elements) != 1 || elements[0].RemovedAt.IsZero() {
		t.Errorf("the merge tags are %+v", elements)
	}
}

func TestAnActivityCarriesNoAttachments(t *testing.T) {
	h := newAttachmentHarness(t)
	h.withItem(domain.ItemActivity)

	_, err := h.attach.Execute(t.Context(), actor(), attachCmd())
	if detail := shared.AsError(err).DetailCode; detail != "items.capability_not_supported" {
		t.Fatalf("detail %q, want items.capability_not_supported", detail)
	}
	if h.media.deltas[receiptID] != 0 {
		t.Error("a reference was counted despite the refusal")
	}
}

func TestAFileTheMediaContextWillNotStandBehindIsRefused(t *testing.T) {
	cases := []struct {
		name   string
		object media.Object
		detail string
	}{
		{name: "still pending", object: pendingAttachment(), detail: "media.not_ready"},
		{name: "staged as a cover", object: readyCover(receiptID), detail: "media.usage_mismatch"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newAttachmentHarness(t)
			h.withItem(domain.ItemTask)
			h.media.stored[receiptID] = c.object

			_, err := h.attach.Execute(t.Context(), actor(), attachCmd())
			if detail := shared.AsError(err).DetailCode; detail != c.detail {
				t.Fatalf("detail %q, want %s", detail, c.detail)
			}
			if len(h.attachments.carried[attachedItem]) != 0 {
				t.Error("a link was written despite the refusal")
			}
		})
	}
}

// A detachment asks nothing of the media context: the object may be anything by now, and refusing
// to let go of a file because it is no longer READY would be a trap.
func TestDetachingDoesNotAskTheMediaContext(t *testing.T) {
	h := newAttachmentHarness(t)
	h.withItem(domain.ItemTask)
	if _, err := h.attach.Execute(t.Context(), actor(), attachCmd()); err != nil {
		t.Fatalf("attaching failed: %v", err)
	}
	h.media.findErr = shared.ErrNotFound

	if _, err := h.detach.Execute(t.Context(), actor(), attachCmd()); err != nil {
		t.Fatalf("detaching failed: %v", err)
	}
}

func pendingAttachment() media.Object {
	object := readyAttachment(receiptID)
	object.Status = media.StatusPending
	return object
}
