// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var filledItem = shared.MustParseID("0192f000-0000-7000-8000-000000000a01")

type valueHarness struct {
	set        SetCustomField
	items      *items
	containers *containers
	fields     *customFieldStore
	events     *events
	changes    *changes
	audit      *sink
	history    *journal
	authorizer *authorizer
	visibility *visibility
}

func newValueHarness(t *testing.T) *valueHarness {
	t.Helper()

	h := &valueHarness{
		items:      &items{stored: map[shared.ID]domain.WorkItem{}},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		fields:     newCustomFieldStore(),
		events:     &events{}, changes: &changes{}, audit: &sink{}, history: &journal{},
		authorizer: &authorizer{}, visibility: newVisibility(accountID, strangerID),
	}

	h.set = SetCustomField{
		Items: h.items, Containers: h.containers,
		Profiles: &profiles{rows: fieldProfiles()}, Fields: h.fields,
		Authorizer: h.authorizer, Visibility: h.visibility,
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

func (h *valueHarness) withItem(itemType domain.ItemType) domain.WorkItem {
	item := domain.WorkItem{
		ID: filledItem, TenantID: tenantID, CollectionID: collectionID, Type: itemType,
		Path: domain.RootPath(filledItem), Depth: 1, Title: "Plan the trip",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[filledItem] = item
	return item
}

// withDefinition puts one definition in scope, at the collection or at the workspace.
func (h *valueHarness) withDefinition(
	t *testing.T, scope shared.ID, key string, kind domain.CustomFieldKind, options ...string,
) domain.CustomFieldDefinition {
	t.Helper()

	definition, err := domain.NewCustomFieldDefinition(domain.NewCustomFieldInput{
		ID:       shared.MustParseID("0192f000-0000-7000-8000-000000000a0" + string(rune('1'+len(h.fields.stored)))),
		TenantID: tenantID, CollectionID: scope, Key: key, Kind: kind, Options: options,
		AppliesTo: []domain.ItemType{domain.ItemTask}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.fields.stored[definition.ID] = definition
	return definition
}

// One change owes five things, and this is the test that says so.
func TestWritingAFieldWritesTheDocumentTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, collectionID, "priority", domain.CustomFieldSelect, "high", "low")

	item, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: "high",
	})
	if err != nil {
		t.Fatalf("writing the field failed: %v", err)
	}

	if item.CustomFields["priority"] != "high" || item.Version != 2 {
		t.Fatalf("the entry is %+v (version %d)", item.CustomFields, item.Version)
	}
	if len(h.items.customFields) != 1 || h.items.customFields[0].expectedVersion != 1 {
		t.Errorf("the write is %+v", h.items.customFields)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ItemUpdated {
		t.Fatalf("the announcement is %+v", h.events.appended)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ItemCustomFieldSetAction {
		t.Fatalf("the trail is %+v", h.audit.entries)
	}
	// The value is user content: recorded as a fingerprint rather than in clear text, because the
	// trail outlives the entry and a value kept here is a copy no deletion reaches (rule 10).
	if change, named := h.audit.entries[0].Changes[domain.FieldCustomFields]; !named {
		t.Error("the trail does not say that a value moved")
	} else if containsText(change, "high") {
		t.Errorf("the value reached the trail in clear text: %v", change)
	}
	if len(h.history.entries) != 1 || h.history.entries[0].Verb != activity.ItemCustomFieldSet {
		t.Fatalf("the history is %+v", h.history.entries)
	}
}

// The merge rule, written down where it is enforced: one change log entry naming one key, with an
// HLC of its own, so two devices setting two different keys converge to both.
func TestTheChangeLogNamesOneKeyAndTakesItsOwnClock(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, collectionID, "priority", domain.CustomFieldText)
	h.withDefinition(t, collectionID, "owner_note", domain.CustomFieldText)

	for _, pair := range []struct{ key, value string }{
		{"priority", "high"}, {"owner_note", "ask Bert"},
	} {
		if _, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
			ItemID: filledItem, Key: pair.key, Value: pair.value,
		}); err != nil {
			t.Fatalf("writing %s failed: %v", pair.key, err)
		}
	}

	if len(h.changes.recorded) != 2 {
		t.Fatalf("%d change log entries, want one per key", len(h.changes.recorded))
	}
	first, second := h.changes.recorded[0], h.changes.recorded[1]
	if len(first.Payload) != 1 || first.Payload["custom_fields.priority"] != "high" {
		t.Errorf("the first payload is %+v", first.Payload)
	}
	if len(second.Payload) != 1 || second.Payload["custom_fields.owner_note"] != "ask Bert" {
		t.Errorf("the second payload is %+v", second.Payload)
	}
	// Two clocks, not one. One entry covering both keys would let the later of two devices erase
	// the other's value, which is the loss the per-key rule exists to prevent.
	if first.HLC == second.HLC {
		t.Errorf("both keys were given the clock reading %v", first.HLC)
	}
	// And the entry carries both, which is what "converge to both" means once they have merged.
	stored := h.items.stored[filledItem]
	if len(stored.CustomFields) != 2 {
		t.Errorf("the entry carries %+v", stored.CustomFields)
	}
}

func TestClearingAKeyRemovesItAndSaysSo(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, collectionID, "priority", domain.CustomFieldText)

	if _, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: "high",
	}); err != nil {
		t.Fatal(err)
	}
	item, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: nil,
	})
	if err != nil {
		t.Fatalf("clearing failed: %v", err)
	}

	if _, held := item.CustomFields["priority"]; held {
		t.Errorf("the entry still carries %+v", item.CustomFields)
	}
	// The clearing travels as an explicit null rather than being left out: an absent key means
	// "not touched", and a device reading it that way would keep a value somebody removed.
	last := h.changes.recorded[len(h.changes.recorded)-1]
	value, named := last.Payload["custom_fields.priority"]
	if !named || value != nil {
		t.Errorf("the change log payload is %+v", last.Payload)
	}
}

func TestTheSameValueAgainWritesNothing(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, collectionID, "priority", domain.CustomFieldText)

	if _, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: "high",
	}); err != nil {
		t.Fatal(err)
	}
	writes, events := len(h.items.customFields), len(h.events.appended)

	item, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: "high",
	})
	if err != nil {
		t.Fatalf("the repeat failed: %v", err)
	}
	if item.Version != 2 {
		t.Errorf("the version is %d, want 2 - no version is spent on a no-op", item.Version)
	}
	if len(h.items.customFields) != writes || len(h.events.appended) != events {
		t.Error("a repeat wrote something")
	}
}

// A key nothing defines is refused rather than stored: `custom_fields` is a document, and one that
// accepted any key would be a place for a typo to live forever.
func TestAKeyNothingDefinesIsRefused(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)

	_, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: "high",
	})
	if detail := shared.AsError(err).DetailCode; detail != "fields.not_in_scope" {
		t.Fatalf("detail %q, want fields.not_in_scope", detail)
	}
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("error %v, want not found", err)
	}
}

// The acceptance C-07 names by hand: a collection-scoped field is refused on an entry in another
// collection. The definition exists; it is simply not in this entry's scope.
func TestACollectionScopedFieldIsOutOfScopeElsewhere(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-000000000a99")
	h.withDefinition(t, elsewhere, "priority", domain.CustomFieldText)

	_, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: "high",
	})
	if detail := shared.AsError(err).DetailCode; detail != "fields.not_in_scope" {
		t.Fatalf("detail %q, want fields.not_in_scope", detail)
	}
}

// A workspace-wide definition reaches every collection, which is the other half of the same rule.
func TestAWorkspaceWideFieldReachesEveryEntry(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, shared.ID(""), "cost_centre", domain.CustomFieldText)

	item, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "cost_centre", Value: "R&D",
	})
	if err != nil {
		t.Fatalf("writing the field failed: %v", err)
	}
	if item.CustomFields["cost_centre"] != "R&D" {
		t.Errorf("the entry is %+v", item.CustomFields)
	}
}

// The other acceptance sentence: a custom field on an ACTIVITY is capability_not_supported.
func TestAnActivityCarriesNoCustomFields(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemActivity)
	h.withDefinition(t, collectionID, "priority", domain.CustomFieldText)

	_, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: "high",
	})
	if detail := shared.AsError(err).DetailCode; detail != "items.capability_not_supported" {
		t.Fatalf("detail %q, want items.capability_not_supported", detail)
	}
	if len(h.items.customFields) != 0 {
		t.Error("a value was written despite the refusal")
	}
}

// The definition exists for this collection and does not apply to this entry's type.
func TestADefinitionThatDoesNotApplyToTheTypeIsRefused(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemWorkPackage)
	h.withDefinition(t, collectionID, "priority", domain.CustomFieldText)

	_, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: "high",
	})
	if detail := shared.AsError(err).DetailCode; detail != "fields.not_for_this_type" {
		t.Fatalf("detail %q, want fields.not_for_this_type", detail)
	}
}

func TestAValueTheDefinitionRefusesIsNotStored(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, collectionID, "priority", domain.CustomFieldSelect, "high")

	_, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: "medium",
	})
	if detail := shared.AsError(err).DetailCode; detail != "fields.value_not_an_option" {
		t.Fatalf("detail %q, want fields.value_not_an_option", detail)
	}
	if len(h.items.customFields) != 0 {
		t.Error("a refused value was written")
	}
}

// A USER value naming somebody who cannot reach the entry is refused, exactly as an assignment to
// them would be - and with the same one answer for all three ways of not being reachable (T-04).
func TestAUserValueNamingSomebodyOutOfReachIsRefused(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, collectionID, "reviewer", domain.CustomFieldUser)
	delete(h.visibility.reachable, strangerID)

	_, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "reviewer", Value: strangerID.String(),
	})
	if detail := shared.AsError(err).DetailCode; detail != "items.account_without_access" {
		t.Fatalf("detail %q, want items.account_without_access", detail)
	}
}

func TestAWriteAgainstAVersionThatHasMovedOnIsRefused(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, collectionID, "priority", domain.CustomFieldText)

	_, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "priority", Value: "high", ExpectedVersion: 7,
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("error %v, want a version conflict", err)
	}
}
