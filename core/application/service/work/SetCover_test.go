// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	media "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	coveredItem  = shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")
	pictureID    = shared.MustParseID("0192f000-0000-7000-8000-0000000000c2")
	otherPicture = shared.MustParseID("0192f000-0000-7000-8000-0000000000c3")
)

// coverProfiles is the shared fixture with COVER where the capability matrix grants it: a task has
// one, and neither a work package nor an activity does (domain-model.md §2, migration 0002).
func coverProfiles() []domain.CapabilityProfile {
	rows := systemProfiles()
	for i, row := range rows {
		if row.Type == domain.ItemTask {
			rows[i].Capabilities = append(row.Capabilities, domain.CapabilityCover)
		}
	}
	return rows
}

// mediaObjects is the media record store as this package sees it: read one object, move its
// counter. The deltas are kept so a test can say what the reference counting did rather than only
// that it happened.
type mediaObjects struct {
	stored  map[shared.ID]media.Object
	deltas  map[shared.ID]int
	order   []shared.ID
	findErr error
}

func newMediaObjects() *mediaObjects {
	return &mediaObjects{
		stored: map[shared.ID]media.Object{}, deltas: map[shared.ID]int{},
	}
}

func (m *mediaObjects) Find(_ context.Context, id shared.ID) (media.Object, error) {
	if m.findErr != nil {
		return media.Object{}, m.findErr
	}
	object, ok := m.stored[id]
	if !ok {
		return media.Object{}, shared.ErrNotFound
	}
	return object, nil
}

func (m *mediaObjects) AdjustRefCount(_ context.Context, id shared.ID, delta int) error {
	m.deltas[id] += delta
	m.order = append(m.order, id)
	return nil
}

// readyCover is a media object that may stand behind a cover.
func readyCover(id shared.ID) media.Object {
	return media.Object{
		ID: id, TenantID: tenantID, StorageKey: "media/" + tenantID.String() + "/" + id.String(),
		FileName: "beach.png", ContentType: "image/png", ByteSize: 32,
		Usage: media.UsageCover, Status: media.StatusReady, CreatedBy: accountID, CreatedAt: now,
	}
}

type coverHarness struct {
	set        SetCover
	clear      ClearCover
	items      *items
	containers *containers
	media      *mediaObjects
	events     *events
	changes    *changes
	audit      *sink
	history    *journal
	authorizer *authorizer
}

func newCoverHarness(t *testing.T) *coverHarness {
	t.Helper()

	h := &coverHarness{
		items:      &items{stored: map[shared.ID]domain.WorkItem{}},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		media:      newMediaObjects(),
		events:     &events{}, changes: &changes{}, audit: &sink{}, history: &journal{},
		authorizer: &authorizer{},
	}

	writer := CoverWriter{
		Items: h.items, Containers: h.containers, Profiles: &profiles{rows: coverProfiles()},
		Media: h.media, Authorizer: h.authorizer, Events: h.events, Changes: h.changes,
		Audit:      h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.set = SetCover{Cover: writer}
	h.clear = ClearCover{Cover: writer}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.media.stored[pictureID] = readyCover(pictureID)
	h.media.stored[otherPicture] = readyCover(otherPicture)
	return h
}

func (h *coverHarness) withItem(itemType domain.ItemType, cover *domain.Cover) domain.WorkItem {
	item := domain.WorkItem{
		ID: coveredItem, TenantID: tenantID, CollectionID: collectionID, Type: itemType,
		Path: domain.RootPath(coveredItem), Depth: 1, Title: "Plan the trip", Cover: cover,
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[coveredItem] = item
	return item
}

func colourCmd(token string) CoverCommand {
	return CoverCommand{ItemID: coveredItem, Kind: domain.CoverColor, ColorToken: token}
}

func imageCmd(mediaID shared.ID) CoverCommand {
	return CoverCommand{ItemID: coveredItem, Kind: domain.CoverImage, MediaID: mediaID}
}

// One change owes five things, and this is the test that says so.
func TestSettingAColourCoverWritesTheFieldTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newCoverHarness(t)
	h.withItem(domain.ItemTask, nil)

	item, err := h.set.Execute(t.Context(), actor(), colourCmd("surface.sand"))
	if err != nil {
		t.Fatalf("setting the cover failed: %v", err)
	}

	if item.Cover == nil || item.Cover.ColorToken != "surface.sand" {
		t.Fatalf("the cover is %+v", item.Cover)
	}
	if len(h.items.covers) != 1 || h.items.covers[0].expectedVersion != 1 {
		t.Errorf("the write is %+v", h.items.covers)
	}
	if item.Version != 2 {
		t.Errorf("the version is %d, want 2", item.Version)
	}
	// The event is an item.updated carrying the field, because the cover is a scalar on the row
	// and the catalogue names no cover event (domain-model.md §4).
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ItemUpdated {
		t.Fatalf("the announcement is %+v", h.events.appended)
	}
	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d change log entries, want 1", len(h.changes.recorded))
	}
	if h.changes.recorded[0].Payload[domain.FieldCover] != "surface.sand" {
		t.Errorf("the change log payload is %+v", h.changes.recorded[0].Payload)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ItemCoverSetAction {
		t.Fatalf("the audit trail is %+v", h.audit.entries)
	}
	if len(h.history.entries) != 1 || h.history.entries[0].Verb != activity.ItemCoverSet {
		t.Fatalf("the history is %+v", h.history.entries)
	}
	// A colour is not a media object, so no counter moved.
	if len(h.media.order) != 0 {
		t.Errorf("a reference counter moved for a colour cover: %v", h.media.deltas)
	}
}

func TestAnImageCoverRaisesTheReferenceCount(t *testing.T) {
	h := newCoverHarness(t)
	h.withItem(domain.ItemTask, nil)

	if _, err := h.set.Execute(t.Context(), actor(), imageCmd(pictureID)); err != nil {
		t.Fatalf("setting the cover failed: %v", err)
	}

	if h.media.deltas[pictureID] != 1 {
		t.Errorf("the reference count moved by %d, want 1", h.media.deltas[pictureID])
	}
}

func TestReplacingAnImageCoverMovesBothCounters(t *testing.T) {
	h := newCoverHarness(t)
	h.withItem(domain.ItemTask, &domain.Cover{Kind: domain.CoverImage, MediaID: pictureID})

	if _, err := h.set.Execute(t.Context(), actor(), imageCmd(otherPicture)); err != nil {
		t.Fatalf("replacing the cover failed: %v", err)
	}

	if h.media.deltas[otherPicture] != 1 {
		t.Errorf("the arriving image moved by %d, want 1", h.media.deltas[otherPicture])
	}
	// The displaced image is not deleted here: it is left at zero for the reconciliation job,
	// which is the ordering ON DELETE RESTRICT makes impossible to get wrong.
	if h.media.deltas[pictureID] != -1 {
		t.Errorf("the displaced image moved by %d, want -1", h.media.deltas[pictureID])
	}
}

func TestClearingAnImageCoverLowersTheReferenceCount(t *testing.T) {
	h := newCoverHarness(t)
	h.withItem(domain.ItemTask, &domain.Cover{Kind: domain.CoverImage, MediaID: pictureID})

	item, err := h.clear.Execute(t.Context(), actor(), CoverCommand{ItemID: coveredItem})
	if err != nil {
		t.Fatalf("clearing the cover failed: %v", err)
	}

	if item.Cover != nil {
		t.Errorf("the cover is still %+v", item.Cover)
	}
	if h.media.deltas[pictureID] != -1 {
		t.Errorf("the reference count moved by %d, want -1", h.media.deltas[pictureID])
	}
	if len(h.history.entries) != 1 || h.history.entries[0].Verb != activity.ItemCoverCleared {
		t.Fatalf("the history is %+v", h.history.entries)
	}
	// The clearing names the field as empty rather than leaving it out - an absent field means
	// "not touched", and a device reading it that way would keep a cover somebody removed.
	if _, named := h.changes.recorded[0].Payload[domain.FieldCover]; !named {
		t.Errorf("the change log payload is %+v", h.changes.recorded[0].Payload)
	}
}

// The acceptance C-06 names by hand: a cover on a work package is refused, and with the code that
// says why rather than being quietly dropped.
func TestATypeWithoutTheCapabilityIsRefused(t *testing.T) {
	for _, itemType := range []domain.ItemType{domain.ItemWorkPackage, domain.ItemActivity} {
		t.Run(string(itemType), func(t *testing.T) {
			h := newCoverHarness(t)
			h.withItem(itemType, nil)

			_, err := h.set.Execute(t.Context(), actor(), colourCmd("surface.sand"))
			if detail := shared.AsError(err).DetailCode; detail != "items.capability_not_supported" {
				t.Fatalf("detail %q, want items.capability_not_supported", detail)
			}
			if len(h.items.covers) != 0 {
				t.Error("a cover was written despite the refusal")
			}
		})
	}
}

func TestTheSameCoverAgainWritesNothing(t *testing.T) {
	h := newCoverHarness(t)
	h.withItem(domain.ItemTask, &domain.Cover{Kind: domain.CoverColor, ColorToken: "surface.sand"})

	item, err := h.set.Execute(t.Context(), actor(), colourCmd("surface.sand"))
	if err != nil {
		t.Fatalf("setting the cover failed: %v", err)
	}

	if item.Version != 1 {
		t.Errorf("the version is %d, want 1 - no version is spent on a no-op", item.Version)
	}
	if len(h.items.covers) != 0 || len(h.events.appended) != 0 || len(h.audit.entries) != 0 {
		t.Error("a repeat wrote something")
	}
}

func TestClearingAnEntryWithNoCoverWritesNothing(t *testing.T) {
	h := newCoverHarness(t)
	h.withItem(domain.ItemTask, nil)

	if _, err := h.clear.Execute(
		t.Context(), actor(), CoverCommand{ItemID: coveredItem},
	); err != nil {
		t.Fatalf("clearing the cover failed: %v", err)
	}
	if len(h.items.covers) != 0 || len(h.events.appended) != 0 {
		t.Error("a clearing of nothing wrote something")
	}
}

func TestAnImageTheMediaContextWillNotStandBehindIsRefused(t *testing.T) {
	cases := []struct {
		name   string
		object media.Object
		detail string
	}{
		{
			name:   "still pending",
			object: pendingObject(),
			detail: "media.not_ready",
		},
		{
			name:   "staged as an attachment",
			object: attachmentObject(),
			detail: "media.usage_mismatch",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newCoverHarness(t)
			h.withItem(domain.ItemTask, nil)
			h.media.stored[pictureID] = c.object

			_, err := h.set.Execute(t.Context(), actor(), imageCmd(pictureID))
			if detail := shared.AsError(err).DetailCode; detail != c.detail {
				t.Fatalf("detail %q, want %s", detail, c.detail)
			}
			if len(h.items.covers) != 0 {
				t.Error("a cover was written despite the refusal")
			}
		})
	}
}

func TestAnImageThatIsNotThereIsRefused(t *testing.T) {
	h := newCoverHarness(t)
	h.withItem(domain.ItemTask, nil)
	delete(h.media.stored, pictureID)

	_, err := h.set.Execute(t.Context(), actor(), imageCmd(pictureID))
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
}

func TestACoverIsRefusedAgainstAVersionThatHasMovedOn(t *testing.T) {
	h := newCoverHarness(t)
	h.withItem(domain.ItemTask, nil)

	cmd := colourCmd("surface.sand")
	cmd.ExpectedVersion = 7

	_, err := h.set.Execute(t.Context(), actor(), cmd)
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("error %v, want a version conflict", err)
	}
}

func pendingObject() media.Object {
	object := readyCover(pictureID)
	object.Status = media.StatusPending
	return object
}

func attachmentObject() media.Object {
	object := readyCover(pictureID)
	object.Usage = media.UsageAttachment
	return object
}
