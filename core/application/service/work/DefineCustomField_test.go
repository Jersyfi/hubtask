// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// fieldProfiles is the shared fixture with CUSTOM_FIELDS where the capability matrix grants it: a
// task and a work package carry them, an activity does not (domain-model.md §2, migration 0002).
func fieldProfiles() []domain.CapabilityProfile {
	rows := systemProfiles()
	for i, row := range rows {
		if row.Type == domain.ItemTask || row.Type == domain.ItemWorkPackage {
			rows[i].Capabilities = append(row.Capabilities, domain.CapabilityCustomFields)
		}
	}
	return rows
}

// customFieldStore is the definitions, as a fake. It keeps the scope so that the two-scope read
// can be tested without a database.
type customFieldStore struct {
	stored    map[shared.ID]domain.CustomFieldDefinition
	inserted  []domain.CustomFieldDefinition
	insertErr error
	updates   []attributeWrite
	deletes   []attributeWrite
}

func newCustomFieldStore() *customFieldStore {
	return &customFieldStore{stored: map[shared.ID]domain.CustomFieldDefinition{}}
}

func (s *customFieldStore) Insert(_ context.Context, definition domain.CustomFieldDefinition) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.inserted = append(s.inserted, definition)
	s.stored[definition.ID] = definition
	return nil
}

func (s *customFieldStore) Find(
	_ context.Context, id shared.ID,
) (domain.CustomFieldDefinition, error) {
	definition, held := s.stored[id]
	if !held {
		return domain.CustomFieldDefinition{}, shared.ErrNotFound
	}
	return definition, nil
}

func (s *customFieldStore) FindInScope(
	_ context.Context, collectionID shared.ID, key string,
) (domain.CustomFieldDefinition, error) {
	var found domain.CustomFieldDefinition
	for _, definition := range s.stored {
		if definition.IsDeleted() || definition.Key != key {
			continue
		}
		if definition.CollectionID != collectionID && !definition.IsTenantWide() {
			continue
		}
		// The collection's own wins over the workspace-wide one.
		if found.ID.IsZero() || !definition.IsTenantWide() {
			found = definition
		}
	}
	if found.ID.IsZero() {
		return domain.CustomFieldDefinition{}, shared.ErrNotFound
	}
	return found, nil
}

func (s *customFieldStore) ListInScope(
	_ context.Context, collectionID shared.ID,
) ([]domain.CustomFieldDefinition, error) {
	var listed []domain.CustomFieldDefinition
	for _, definition := range s.stored {
		if definition.IsDeleted() {
			continue
		}
		if definition.IsTenantWide() || definition.CollectionID == collectionID {
			listed = append(listed, definition)
		}
	}
	slices.SortFunc(listed, func(a, b domain.CustomFieldDefinition) int {
		if a.Key != b.Key {
			if a.Key < b.Key {
				return -1
			}
			return 1
		}
		return 0
	})
	return listed, nil
}

func (s *customFieldStore) SetAttributes(
	_ context.Context, definition domain.CustomFieldDefinition, expectedVersion int,
) error {
	if s.stored[definition.ID].Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	written := definition
	written.Version = expectedVersion + 1
	s.stored[definition.ID] = written
	s.updates = append(s.updates, attributeWrite{expectedVersion: expectedVersion})
	return nil
}

func (s *customFieldStore) SetDeleted(
	_ context.Context, definition domain.CustomFieldDefinition, expectedVersion int,
) error {
	if s.stored[definition.ID].Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("items.version_conflict")
	}
	written := definition
	written.Version = expectedVersion + 1
	s.stored[definition.ID] = written
	s.deletes = append(s.deletes, attributeWrite{expectedVersion: expectedVersion})
	return nil
}

var _ repository.CustomFields = (*customFieldStore)(nil)

type customFieldHarness struct {
	define     DefineCustomField
	list       ListCustomFields
	fields     *customFieldStore
	containers *containers
	audit      *sink
	authorizer *authorizer
}

func newCustomFieldHarness(t *testing.T) *customFieldHarness {
	t.Helper()

	h := &customFieldHarness{
		fields:     newCustomFieldStore(),
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		audit:      &sink{}, authorizer: &authorizer{},
	}

	h.define = DefineCustomField{
		Fields: h.fields, Containers: h.containers,
		Profiles:   &profiles{rows: fieldProfiles()},
		Authorizer: h.authorizer, Audit: h.audit, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now), IDs: &ids{},
	}
	h.list = ListCustomFields{
		Fields: h.fields, Containers: h.containers,
		Authorizer: h.authorizer, UnitOfWork: &unitOfWork{},
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

func TestACollectionScopedFieldIsAuthorisedAlongTheCollectionsPath(t *testing.T) {
	h := newCustomFieldHarness(t)

	defined, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldSelect,
		Options: []string{"high", "low"},
	})
	if err != nil {
		t.Fatalf("defining failed: %v", err)
	}

	if defined.Key != "priority" || defined.CollectionID != collectionID {
		t.Fatalf("the definition is %+v", defined)
	}
	// Omitted applies_to means a task alone, written out rather than left to the column's default
	// so the answer says what was stored.
	if len(defined.AppliesTo) != 1 || defined.AppliesTo[0] != domain.ItemTask {
		t.Errorf("applies_to is %v", defined.AppliesTo)
	}
	if len(h.authorizer.requests) != 1 {
		t.Fatalf("the authorisation service was asked %d times", len(h.authorizer.requests))
	}
	// The path runs from the tenant down through the hub, so a membership held anywhere above the
	// collection counts.
	asked := h.authorizer.requests[0].Path
	if len(asked) != 3 || asked[2].Type != identity.ScopeCollection {
		t.Errorf("the path asked about is %v", asked)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != CustomFieldDefinedAction {
		t.Fatalf("the trail is %+v", h.audit.entries)
	}
	// The options are user content and stay out of the trail (rule 10).
	if _, named := h.audit.entries[0].Changes["options"]; named {
		t.Error("the options reached the audit trail")
	}
}

// A field every entry in the workspace carries is a decision about the workspace, and the
// permission is asked there - which refuses somebody who administers one hub.
func TestAWorkspaceWideFieldIsAuthorisedAtTheWorkspace(t *testing.T) {
	h := newCustomFieldHarness(t)

	defined, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		Key: "cost_centre", Kind: domain.CustomFieldText,
	})
	if err != nil {
		t.Fatalf("defining failed: %v", err)
	}
	if !defined.IsTenantWide() {
		t.Fatalf("the definition is %+v", defined)
	}

	asked := h.authorizer.requests[0].Path
	if len(asked) != 1 || asked[0].Type != identity.ScopeTenant {
		t.Errorf("the path asked about is %v", asked)
	}
}

func TestATypeThatCarriesNoCustomFieldsIsRefusedByName(t *testing.T) {
	h := newCustomFieldHarness(t)

	_, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldText,
		AppliesTo: []domain.ItemType{domain.ItemTask, domain.ItemActivity},
	})
	if detail := shared.AsError(err).DetailCode; detail != "fields.applies_to_unsupported" {
		t.Fatalf("detail %q, want fields.applies_to_unsupported", detail)
	}
	// Refused rather than dropped from the list: a client that received a 201 would believe an
	// activity can hold it.
	if len(h.fields.inserted) != 0 {
		t.Error("a definition was written despite the refusal")
	}
}

func TestAFieldInACollectionThatCannotTakeEntriesIsRefused(t *testing.T) {
	h := newCustomFieldHarness(t)
	archived := h.containers.stored[collectionID]
	archived.ArchivedAt = &now
	h.containers.stored[collectionID] = archived

	_, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldText,
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error %v, want a conflict", err)
	}
}

func TestARefusedDefinitionWritesNothing(t *testing.T) {
	h := newCustomFieldHarness(t)
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldText,
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if len(h.fields.inserted) != 0 || len(h.audit.entries) != 0 {
		t.Error("a refusal wrote something")
	}
}

func TestTheListCarriesTheCollectionsOwnAndTheWorkspaceWideOnes(t *testing.T) {
	h := newCustomFieldHarness(t)

	if _, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		Key: "cost_centre", Kind: domain.CustomFieldText,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldText,
	}); err != nil {
		t.Fatal(err)
	}

	inCollection, err := h.list.Execute(t.Context(), actor(), ListCustomFieldsQuery{
		CollectionID: collectionID,
	})
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if len(inCollection) != 2 {
		t.Fatalf("the collection sees %d definitions", len(inCollection))
	}

	// Naming no collection answers the workspace-wide ones alone: a client configuring the
	// workspace is not asking what one collection added.
	workspaceWide, err := h.list.Execute(t.Context(), actor(), ListCustomFieldsQuery{})
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if len(workspaceWide) != 1 || workspaceWide[0].Key != "cost_centre" {
		t.Errorf("the workspace sees %+v", workspaceWide)
	}
}

func TestARefusedListAnswersNothing(t *testing.T) {
	h := newCustomFieldHarness(t)
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	if _, err := h.list.Execute(
		t.Context(), actor(), ListCustomFieldsQuery{CollectionID: collectionID},
	); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
}

// --- the edit and the deletion ------------------------------------------------------------------

func (h *customFieldHarness) writer() CustomFieldWriter {
	return CustomFieldWriter{
		Fields: h.fields, Containers: h.containers,
		Profiles:   &profiles{rows: fieldProfiles()},
		Authorizer: h.authorizer, Audit: h.audit, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now),
	}
}

// defined writes one definition through the use case and returns it.
func (h *customFieldHarness) defined(t *testing.T, cmd DefineCustomFieldCommand) domain.CustomFieldDefinition {
	t.Helper()

	definition, err := h.define.Execute(t.Context(), actor(), cmd)
	if err != nil {
		t.Fatalf("defining failed: %v", err)
	}
	h.audit.entries = nil
	h.authorizer.requests = nil
	return definition
}

func TestAnEditWritesWhatMovedAndSpendsOneVersion(t *testing.T) {
	h := newCustomFieldHarness(t)
	defined := h.defined(t, DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldSelect,
		Options: []string{"high"},
	})
	options := []string{"high", "low"}

	updated, err := (UpdateCustomField{Writer: h.writer()}).Execute(
		t.Context(), actor(), UpdateCustomFieldCommand{
			FieldID: defined.ID, Attributes: domain.CustomFieldAttributes{Options: &options},
		})
	if err != nil {
		t.Fatalf("the edit failed: %v", err)
	}

	if len(updated.Options) != 2 || updated.Version != 2 {
		t.Fatalf("the definition is %+v", updated)
	}
	if len(h.fields.updates) != 1 || h.fields.updates[0].expectedVersion != 1 {
		t.Errorf("the write is %+v", h.fields.updates)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != CustomFieldUpdatedAction {
		t.Fatalf("the trail is %+v", h.audit.entries)
	}
	// The options are user content: recorded as a fingerprint rather than in clear text, so the
	// trail can say that the list changed without carrying what a team calls its choices.
	if change, named := h.audit.entries[0].Changes[domain.FieldOptions]; !named {
		t.Error("the trail does not say that the options moved")
	} else if containsText(change, "low") {
		t.Errorf("the options reached the trail in clear text: %v", change)
	}
}

func TestAnEditThatChangesNothingSpendsNoVersion(t *testing.T) {
	h := newCustomFieldHarness(t)
	defined := h.defined(t, DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldText,
	})
	required := false

	updated, err := (UpdateCustomField{Writer: h.writer()}).Execute(
		t.Context(), actor(), UpdateCustomFieldCommand{
			FieldID: defined.ID, Attributes: domain.CustomFieldAttributes{IsRequired: &required},
		})
	if err != nil {
		t.Fatalf("the edit failed: %v", err)
	}
	if updated.Version != 1 || len(h.fields.updates) != 0 || len(h.audit.entries) != 0 {
		t.Errorf("a no-op wrote something: version %d, %+v", updated.Version, h.fields.updates)
	}
}

// The If-Match is honoured even when the change would have been a no-op: the state the caller was
// reasoning about is not the state that is there.
func TestAnEditAgainstAStaleVersionIsRefusedEvenWhenItChangesNothing(t *testing.T) {
	h := newCustomFieldHarness(t)
	defined := h.defined(t, DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldText,
	})
	required := false

	_, err := (UpdateCustomField{Writer: h.writer()}).Execute(
		t.Context(), actor(), UpdateCustomFieldCommand{
			FieldID:         defined.ID,
			Attributes:      domain.CustomFieldAttributes{IsRequired: &required},
			ExpectedVersion: 7,
		})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("error %v, want a version conflict", err)
	}
}

func TestAnEditThatNamesATypeWithoutCustomFieldsIsRefused(t *testing.T) {
	h := newCustomFieldHarness(t)
	defined := h.defined(t, DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldText,
	})
	types := []domain.ItemType{domain.ItemActivity}

	_, err := (UpdateCustomField{Writer: h.writer()}).Execute(
		t.Context(), actor(), UpdateCustomFieldCommand{
			FieldID: defined.ID, Attributes: domain.CustomFieldAttributes{AppliesTo: &types},
		})
	if detail := shared.AsError(err).DetailCode; detail != "fields.applies_to_unsupported" {
		t.Fatalf("detail %q, want fields.applies_to_unsupported", detail)
	}
}

func TestADeletionTakesTheDefinitionOutOfUseAndIsIdempotent(t *testing.T) {
	h := newCustomFieldHarness(t)
	defined := h.defined(t, DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldText,
	})
	deleter := DeleteCustomField{Writer: h.writer()}

	if err := deleter.Execute(
		t.Context(), actor(), DeleteCustomFieldCommand{FieldID: defined.ID},
	); err != nil {
		t.Fatalf("the deletion failed: %v", err)
	}
	if !h.fields.stored[defined.ID].IsDeleted() {
		t.Fatal("the definition is not out of use")
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != CustomFieldDeletedAction {
		t.Fatalf("the trail is %+v", h.audit.entries)
	}

	// A second DELETE is a client retrying, not a conflict.
	if err := deleter.Execute(
		t.Context(), actor(), DeleteCustomFieldCommand{FieldID: defined.ID},
	); err != nil {
		t.Fatalf("a second deletion answered %v", err)
	}
	if len(h.fields.deletes) != 1 {
		t.Errorf("%d deletions were written, want 1", len(h.fields.deletes))
	}

	// And it is out of every scoped read from now on.
	listed, err := h.list.Execute(t.Context(), actor(), ListCustomFieldsQuery{
		CollectionID: collectionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Errorf("a deleted definition is still listed: %+v", listed)
	}
}

func TestADefinitionThatIsNotThereIsReportedMissing(t *testing.T) {
	h := newCustomFieldHarness(t)
	required := true

	_, err := (UpdateCustomField{Writer: h.writer()}).Execute(
		t.Context(), actor(), UpdateCustomFieldCommand{
			FieldID:    shared.MustParseID("0192f000-0000-7000-8000-0000000000fe"),
			Attributes: domain.CustomFieldAttributes{IsRequired: &required},
		})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
}

// containsText reports whether a recorded change carries the text in clear. The trail masks user
// content, so this is how a test says that it did.
func containsText(change any, text string) bool {
	rendered, ok := change.(map[string]any)
	if !ok {
		return false
	}
	for _, side := range rendered {
		if value, isString := side.(string); isString && strings.Contains(value, text) {
			return true
		}
	}
	return false
}
