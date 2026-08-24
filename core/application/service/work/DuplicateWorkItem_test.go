// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"strconv"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// What a copy carries, what it leaves behind, and what it says about the difference (C-11).

var (
	copiedTaskID       = shared.MustParseID("0192f000-0000-7000-8000-0000000c1101")
	copiedPackID       = shared.MustParseID("0192f000-0000-7000-8000-0000000c1102")
	copiedActivityID   = shared.MustParseID("0192f000-0000-7000-8000-0000000c1103")
	farCollectionID    = shared.MustParseID("0192f000-0000-7000-8000-0000000c1104")
	homeLabelID        = shared.MustParseID("0192f000-0000-7000-8000-0000000c1105")
	homeBucketID       = shared.MustParseID("0192f000-0000-7000-8000-0000000c1106")
	colleagueAccountID = shared.MustParseID("0192f000-0000-7000-8000-0000000c1107")
	attachedFileID     = shared.MustParseID("0192f000-0000-7000-8000-0000000c1108")
)

type duplicateHarness struct {
	handler     DuplicateWorkItem
	items       *items
	containers  *containers
	labels      *labels
	buckets     *buckets
	itemLabels  *itemLabels
	itemMembers *itemMembers
	fields      *customFieldStore
	attachments *attachments
	media       *mediaObjects
	visibility  *visibility
	events      *events
	changes     *changes
	audit       *sink
	history     *journal
}

// copyProfiles is the system topology with everything a copy can carry switched on for a task and a
// work package, and an activity left as it is: the type that carries an assignee and no member list
// is what makes the capability half of this testable at all (domain-model.md §2).
func copyProfiles() []domain.CapabilityProfile {
	rows := systemProfiles()
	for i, row := range rows {
		if row.Type == domain.ItemActivity {
			continue
		}
		rows[i].Capabilities = append(row.Capabilities,
			domain.CapabilityLabels, domain.CapabilityMembers, domain.CapabilityAssignment,
			domain.CapabilityAttachments, domain.CapabilityCover, domain.CapabilityCustomFields)
	}
	return rows
}

func newDuplicateHarness(t *testing.T) *duplicateHarness {
	t.Helper()

	h := &duplicateHarness{
		items:       &items{stored: map[shared.ID]domain.WorkItem{}, lastKey: "a0"},
		containers:  &containers{stored: map[shared.ID]domain.Container{}},
		labels:      &labels{stored: map[shared.ID]domain.Label{}},
		buckets:     &buckets{stored: map[shared.ID]domain.Bucket{}},
		itemLabels:  newItemLabels(),
		itemMembers: newItemMembers(),
		fields:      newCustomFieldStore(),
		attachments: newAttachments(),
		media:       newMediaObjects(),
		visibility:  newVisibility(accountID, colleagueAccountID),
		events:      &events{}, changes: &changes{}, audit: &sink{}, history: &journal{},
	}

	h.handler = DuplicateWorkItem{
		Items: h.items, ItemLabels: h.itemLabels, ItemMembers: h.itemMembers, Labels: h.labels,
		Buckets: h.buckets, Fields: h.fields, Containers: h.containers,
		Attachments: h.attachments, Media: h.media, Profiles: &profiles{rows: copyProfiles()},
		Authorizer: &authorizer{}, Visibility: h.visibility,
		Events: h.events, Changes: h.changes, Audit: h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	for _, id := range []shared.ID{collectionID, farCollectionID} {
		h.containers.stored[id] = domain.Container{
			ID: id, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
			Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
		}
	}
	return h
}

// withTask stores one task, filled in with everything a copy can carry.
func (h *duplicateHarness) withTask() domain.WorkItem {
	task := domain.WorkItem{
		ID: copiedTaskID, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemTask,
		Path: domain.RootPath(copiedTaskID), Depth: 1, Title: "Plan the move", Notes: "Boxes first",
		OrderKey: "a0", BucketID: homeBucketID, AssigneeID: colleagueAccountID,
		Cover:        &domain.Cover{Kind: domain.CoverColor, ColorToken: "blue"},
		CustomFields: map[string]any{"priority": "high"},
		CreatedBy:    accountID, CreatedAt: now, UpdatedAt: now, Version: 3,
	}
	h.items.stored[task.ID] = task
	h.itemLabels.carried[task.ID] = []shared.ID{homeLabelID}
	h.itemMembers.carried[task.ID] = []shared.ID{colleagueAccountID}
	h.attachments.carried[task.ID] = []shared.ID{attachedFileID}

	h.labels.stored[homeLabelID] = domain.Label{
		ID: homeLabelID, TenantID: tenantID, CollectionID: collectionID, Name: "Home",
		ColorToken: "blue", Version: 1,
	}
	h.buckets.stored[homeBucketID] = domain.Bucket{
		ID: homeBucketID, TenantID: tenantID, CollectionID: collectionID, Name: "Doing",
		OrderKey: "a0", Version: 1,
	}
	h.withField(collectionID, "priority")
	return task
}

// withField defines one text field in a collection's scope.
func (h *duplicateHarness) withField(collection shared.ID, key string) domain.CustomFieldDefinition {
	definition, err := domain.NewCustomFieldDefinition(domain.NewCustomFieldInput{
		ID:       shared.MustParseID("0192f000-0000-7000-8000-0000000c11" + collectionKey(collection)),
		TenantID: tenantID, CollectionID: collection, Key: key, Kind: domain.CustomFieldText,
		AppliesTo: []domain.ItemType{domain.ItemTask, domain.ItemWorkPackage}, Now: now,
	})
	if err != nil {
		panic(err)
	}
	h.fields.stored[definition.ID] = definition
	return definition
}

// collectionKey keeps the two definitions of one key apart by the collection they belong to, which
// is the whole point of the pair: the same key, two definitions, one per collection.
func collectionKey(collection shared.ID) string {
	if collection == collectionID {
		return "10"
	}
	return "11"
}

// withSubtree hangs a work package and an activity under the task, three levels deep.
func (h *duplicateHarness) withSubtree(task domain.WorkItem) (pack, act domain.WorkItem) {
	pack = domain.WorkItem{
		ID: copiedPackID, TenantID: tenantID, CollectionID: collectionID,
		Type: domain.ItemWorkPackage, ParentID: task.ID, Path: task.ChildPath(copiedPackID),
		Depth: 2, Title: "Pack the kitchen", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	act = domain.WorkItem{
		ID: copiedActivityID, TenantID: tenantID, CollectionID: collectionID,
		Type: domain.ItemActivity, ParentID: pack.ID, Path: pack.ChildPath(copiedActivityID),
		Depth: 3, Title: "Wrap the glasses", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[pack.ID], h.items.stored[act.ID] = pack, act
	return pack, act
}

// One copy owes four things per entry, and this is the test that says so.
func TestACopyWritesTheRowTheEventTheChangeAndTheHistory(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{
		ItemID: original.ID,
	})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}

	if result.Copied != 1 || len(h.items.copies) != 1 {
		t.Fatalf("the copy is %d entries and %d rows", result.Copied, len(h.items.copies))
	}
	copied := h.items.copies[0].Item
	if copied.ID == original.ID {
		t.Error("the copy carries the original's identifier")
	}
	if copied.Version != 1 || copied.CreatedAt != now {
		t.Errorf("the copy was born at version %d, %v", copied.Version, copied.CreatedAt)
	}
	if copied.Title != original.Title || copied.Notes != original.Notes {
		t.Errorf("the copy reads %q / %q", copied.Title, copied.Notes)
	}
	// A fresh rank at the end of the level, rather than the one the original holds: two entries
	// with one rank are two entries in one place.
	if copied.OrderKey == original.OrderKey || copied.OrderKey == "" {
		t.Errorf("the copy is ranked %q, the original %q", copied.OrderKey, original.OrderKey)
	}

	// One event and one change record for the entry, and one more record per element of its three
	// sets: a set lives beside the row, so the row's snapshot does not carry it.
	if len(h.events.appended) != 1 || len(h.changes.recorded) != 4 {
		t.Errorf("%d events and %d change records", len(h.events.appended), len(h.changes.recorded))
	}
	if len(h.history.entries) != 1 || h.history.entries[0].Verb != activity.ItemDuplicated {
		t.Errorf("the history is %+v", h.history.entries)
	}
	// One act, one audit entry - whatever the copy turned out to consist of (audit.md §2).
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ItemDuplicatedAction {
		t.Errorf("the trail is %+v", h.audit.entries)
	}
}

// The completion is the one piece of state a copy deliberately does not take over: it names a
// person and a moment, and the copy was never completed by anybody.
func TestACopyOfACompletedEntryIsOpen(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()
	completed := original
	stamp := now
	completed.Completion = domain.Completion{IsCompleted: true, CompletedAt: &stamp, CompletedBy: accountID}
	h.items.stored[original.ID] = completed

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{ItemID: original.ID})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}
	if result.Item.Completion.IsCompleted || !result.Item.Completion.CompletedBy.IsZero() {
		t.Errorf("the copy claims a completion nobody performed: %+v", result.Item.Completion)
	}
}

// The sets travel: the labels, the members and the files, each written with a tag of its own so
// that the addition survives a merge (offline-sync.md §4.2).
func TestACopyCarriesTheLabelsTheMembersAndTheFiles(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{ItemID: original.ID})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}

	copyID := result.Item.ID
	if labels := h.itemLabels.carried[copyID]; len(labels) != 1 || labels[0] != homeLabelID {
		t.Errorf("the copy carries the labels %v", labels)
	}
	if members := h.itemMembers.carried[copyID]; len(members) != 1 || members[0] != colleagueAccountID {
		t.Errorf("the copy carries the members %v", members)
	}
	if files := h.attachments.carried[copyID]; len(files) != 1 || files[0] != attachedFileID {
		t.Errorf("the copy carries the files %v", files)
	}
	// The bytes are shared rather than copied: the counter is what says how many entries point at
	// them (C-06).
	if h.media.deltas[attachedFileID] != 1 {
		t.Errorf("the file's reference count moved by %d", h.media.deltas[attachedFileID])
	}
	if result.Item.AssigneeID != colleagueAccountID {
		t.Errorf("the copy is on %s", result.Item.AssigneeID)
	}
	if result.Item.CustomFields["priority"] != "high" {
		t.Errorf("the copy reads the custom fields as %v", result.Item.CustomFields)
	}
	if len(result.DroppedReferences) != 0 {
		t.Errorf("a copy inside one collection lost %+v", result.DroppedReferences)
	}

	// And an offline client is told about each of them: the entry's own record is a snapshot of the
	// row, and a set lives beside the row, so a client that received only the entry would have a
	// copy with no labels, no members and no files.
	sets := map[string]string{}
	for _, change := range h.changes.recorded {
		if set, carried := change.Payload["set"].(string); carried {
			sets[set], _ = change.Payload["element_id"].(string)
		}
	}
	for set, wanted := range map[string]string{
		string(domain.SetLabels):      homeLabelID.String(),
		string(domain.SetMembers):     colleagueAccountID.String(),
		string(domain.SetAttachments): attachedFileID.String(),
	} {
		if sets[set] != wanted {
			t.Errorf("the change log records %s as %q, want %q", set, sets[set], wanted)
		}
	}
}

// The acceptance criterion: a subtree of depth 3 produces a subtree of depth 3, with new
// identifiers and fresh ranks.
func TestACopyOfASubtreeKeepsItsShapeAndTakesNewIdentifiers(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()
	pack, act := h.withSubtree(original)

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{
		ItemID: original.ID, IncludeSubtree: true,
	})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}

	if result.Copied != 3 || len(h.items.copies) != 3 {
		t.Fatalf("the copy is %d entries and %d rows", result.Copied, len(h.items.copies))
	}

	root, copiedPack, copiedActivity := h.items.copies[0].Item, h.items.copies[1].Item, h.items.copies[2].Item
	for _, pair := range [][2]shared.ID{
		{root.ID, original.ID}, {copiedPack.ID, pack.ID}, {copiedActivity.ID, act.ID},
	} {
		if pair[0] == pair[1] {
			t.Errorf("%s was copied onto itself", pair[1])
		}
	}
	// The shape: each copy hangs from the copy of its own parent, and the paths and depths follow.
	if copiedPack.ParentID != root.ID || copiedActivity.ParentID != copiedPack.ID {
		t.Errorf("the copies hang from %s and %s", copiedPack.ParentID, copiedActivity.ParentID)
	}
	if copiedPack.Depth != 2 || copiedActivity.Depth != 3 {
		t.Errorf("the copies sit at depth %d and %d", copiedPack.Depth, copiedActivity.Depth)
	}
	if copiedPack.Path != root.ChildPath(copiedPack.ID) ||
		copiedActivity.Path != copiedPack.ChildPath(copiedActivity.ID) {
		t.Errorf("the paths are %q and %q", copiedPack.Path, copiedActivity.Path)
	}
	// Every entry of the copy is announced: a client that heard only about the root would have a
	// subtree whose children it has never been told about.
	if len(h.events.appended) != 3 || len(h.history.entries) != 3 {
		t.Errorf("%d events, %d history entries", len(h.events.appended), len(h.history.entries))
	}
	// Three entry records, and one per element of the root's three sets - a set lives beside the
	// row, so the row's snapshot does not carry it (offline-sync.md §4.2).
	if len(h.changes.recorded) != 6 {
		t.Errorf("%d change records, want three entries and three set elements",
			len(h.changes.recorded))
	}
	if len(h.audit.entries) != 1 {
		t.Errorf("one copy wrote %d audit entries", len(h.audit.entries))
	}
}

// Without the subtree the entry is copied alone, and its children stay where they are.
func TestACopyWithoutTheSubtreeCopiesTheEntryAlone(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()
	h.withSubtree(original)

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{ItemID: original.ID})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}
	if result.Copied != 1 {
		t.Errorf("the copy is %d entries", result.Copied)
	}
}

// A title given replaces the original's; one omitted keeps it, because the server writes no display
// text of its own (ADR-0011).
func TestATitleGivenIsTheCopysAndOnlyTheRootTakesIt(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()
	pack, _ := h.withSubtree(original)

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{
		ItemID: original.ID, IncludeSubtree: true, Title: "Plan the second move",
	})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}
	if result.Item.Title != "Plan the second move" {
		t.Errorf("the copy is called %q", result.Item.Title)
	}
	if below := h.items.copies[1].Item; below.Title != pack.Title {
		t.Errorf("the copied work package is called %q", below.Title)
	}
}

// I-W6 through a copy: what the destination collection cannot resolve is reported rather than
// dropped in silence.
func TestACopyIntoAnotherCollectionReportsWhatItCouldNotResolve(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()
	// The destination defines neither the label nor the column, and knows the key `priority` as a
	// definition of its own - so the value travels and the two references do not.
	h.withField(farCollectionID, "priority")
	// And the colleague cannot see the destination, so neither the assignment nor the membership
	// travels: an entry is only ever on somebody who can see it (C-01).
	h.visibility.reachable[colleagueAccountID] = false

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{
		ItemID: original.ID, TargetCollectionID: farCollectionID, ParentGiven: true,
	})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}

	if result.Item.CollectionID != farCollectionID {
		t.Fatalf("the copy landed in %s", result.Item.CollectionID)
	}
	if result.Item.CustomFields["priority"] != "high" {
		t.Errorf("the value did not travel: %v", result.Item.CustomFields)
	}

	lost := map[domain.ReferenceKind]string{}
	for _, reference := range result.DroppedReferences {
		lost[reference.Kind] = reference.ID
	}
	for kind, wanted := range map[domain.ReferenceKind]string{
		domain.ReferenceLabel:    homeLabelID.String(),
		domain.ReferenceBucket:   homeBucketID.String(),
		domain.ReferenceMember:   colleagueAccountID.String(),
		domain.ReferenceAssignee: colleagueAccountID.String(),
	} {
		if lost[kind] != wanted {
			t.Errorf("the losses report %s as %q, want %q", kind, lost[kind], wanted)
		}
	}
	if !result.Item.BucketID.IsZero() || !result.Item.AssigneeID.IsZero() {
		t.Errorf("the copy kept what it reported as lost: %+v", result.Item)
	}
	if len(h.itemLabels.carried[result.Item.ID]) != 0 {
		t.Error("the copy carries a label of the collection it left")
	}
}

// A key the destination does not define at all is a loss of its own, and the value stays behind.
func TestACopyReportsACustomFieldTheDestinationDoesNotDefine(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{
		ItemID: original.ID, TargetCollectionID: farCollectionID, ParentGiven: true,
	})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}
	if len(result.Item.CustomFields) != 0 {
		t.Errorf("the copy carries %v", result.Item.CustomFields)
	}

	var reported bool
	for _, reference := range result.DroppedReferences {
		if reference.Kind == domain.ReferenceCustomField && reference.ID == "priority" &&
			reference.Code == "fields.not_in_collection" {
			reported = true
		}
	}
	if !reported {
		t.Errorf("the losses are %+v, want the custom field among them", result.DroppedReferences)
	}
}

// A copy takes only what the type's profile allows, and says what the profile took away: a
// workspace that has narrowed a type since the entry was written is the case this is for.
func TestACopyLeavesBehindWhatTheProfileNoLongerAllows(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()
	h.handler.Profiles = &profiles{rows: systemProfiles(), system: systemProfiles()}

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{ItemID: original.ID})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}

	if !result.Item.AssigneeID.IsZero() || result.Item.Cover != nil || len(result.Item.CustomFields) != 0 {
		t.Errorf("the copy carries what the profile does not allow: %+v", result.Item)
	}
	if len(h.itemLabels.carried[result.Item.ID]) != 0 || len(h.attachments.carried[result.Item.ID]) != 0 {
		t.Error("the copy carries a set the profile does not allow")
	}
	for _, reference := range result.DroppedReferences {
		if reference.Code != "items.capability_not_supported" {
			t.Errorf("unexpected loss: %+v", reference)
		}
	}
	if len(result.DroppedReferences) < 4 {
		t.Errorf("the losses are %+v, want one per reference the profile took away",
			result.DroppedReferences)
	}
}

// A trashed entry would put back what somebody deleted, and an archived one would take back out
// what somebody put away. Both are the answer I-W4 gives every other write.
func TestACopyOfATrashedOrArchivedEntryIsRefused(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		stamp  func(*domain.WorkItem)
		detail string
	}{
		{"trashed", func(item *domain.WorkItem) { stamp := now; item.DeletedAt = &stamp }, "items.trashed"},
		{"archived", func(item *domain.WorkItem) { stamp := now; item.ArchivedAt = &stamp }, "items.archived"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			h := newDuplicateHarness(t)
			original := h.withTask()
			stamped := original
			testCase.stamp(&stamped)
			h.items.stored[original.ID] = stamped

			_, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{ItemID: original.ID})
			if !errors.Is(err, shared.ErrConflict) ||
				shared.AsError(err).DetailCode != testCase.detail {
				t.Fatalf("copying a %s entry answered %v", testCase.name, err)
			}
			if len(h.items.copies) != 0 {
				t.Error("something was copied all the same")
			}
		})
	}
}

// A subtree over the bound is refused rather than copied halfway: one copy is one transaction, and
// its length is not decided by how much somebody happens to have below an entry.
func TestASubtreeOverTheBoundIsRefused(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()

	for i := range maxCopiedEntries {
		id := shared.MustParseID("0192f000-0000-7000-8000-" + strconv.Itoa(700000000000+i))
		h.items.stored[id] = domain.WorkItem{
			ID: id, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemWorkPackage,
			ParentID: original.ID, Path: original.ChildPath(id), Depth: 2, Title: "Below",
			OrderKey: "a" + strconv.Itoa(i), CreatedBy: accountID, CreatedAt: now, Version: 1,
		}
	}

	_, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{
		ItemID: original.ID, IncludeSubtree: true,
	})
	if !errors.Is(err, shared.ErrValidation) ||
		shared.AsError(err).DetailCode != "items.subtree_too_large" {
		t.Fatalf("a subtree over the bound answered %v", err)
	}
	if len(h.items.copies) != 0 {
		t.Error("something was copied all the same")
	}
}

// The entry that does not exist is a plain not-found, in the same words every other operation on an
// entry uses: two spellings would be an oracle for which identifiers exist (T-04).
func TestCopyingAnEntryThatIsNotThereIsNotFound(t *testing.T) {
	h := newDuplicateHarness(t)

	_, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{ItemID: copiedTaskID})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("copying a missing entry answered %v", err)
	}
}

// The command with no entry named is the caller's mistake, and says which field it is about.
func TestCopyingWithoutAnEntryIsRefused(t *testing.T) {
	h := newDuplicateHarness(t)

	_, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{})
	if !errors.Is(err, shared.ErrValidation) ||
		shared.AsError(err).DetailCode != "items.item_id_required" {
		t.Fatalf("a copy of nothing answered %v", err)
	}
}

// A copy asks two permission questions, and they are deliberately different: reading the entry it
// copies, and writing where the copy lands. Somebody who may only read a collection can therefore
// copy out of it into one they may write - and both questions carry the write scope, because a
// scope is the credential's licence for the operation and the operation is a write.
func TestACopyAsksToReadTheSourceAndToWriteTheDestination(t *testing.T) {
	h := newDuplicateHarness(t)
	permission := &authorizer{}
	h.handler.Authorizer = permission
	original := h.withTask()

	if _, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{
		ItemID: original.ID, TargetCollectionID: farCollectionID, ParentGiven: true,
	}); err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}

	if len(permission.requests) != 2 {
		t.Fatalf("the copy asked %d permission questions", len(permission.requests))
	}
	source, destination := permission.requests[0], permission.requests[1]
	if source.Permission != service.PermissionRead ||
		destination.Permission != service.PermissionWriteItems {
		t.Errorf("the questions were %s and %s", source.Permission, destination.Permission)
	}
	for _, request := range permission.requests {
		if request.TokenScope != itemsWrite {
			t.Errorf("a question asked for the scope %q", request.TokenScope)
		}
		if request.Action != ItemDuplicatedAction {
			t.Errorf("a refusal would be recorded as %q", request.Action)
		}
	}
}
