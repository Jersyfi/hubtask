// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

var (
	urgentLabel  = shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")
	blockedLabel = shared.MustParseID("0192f000-0000-7000-8000-0000000000c2")
)

// labels is the fake vocabulary.
type labels struct {
	inserted []domain.Label
	stored   map[shared.ID]domain.Label
	written  []writtenLabel

	findErr   error
	listErr   error
	insertErr error
	writeErr  error
}

type writtenLabel struct {
	method   string
	label    domain.Label
	expected int
}

func (l *labels) Find(_ context.Context, id shared.ID) (domain.Label, error) {
	if l.findErr != nil {
		return domain.Label{}, l.findErr
	}
	label, found := l.stored[id]
	if !found {
		return domain.Label{}, shared.ErrNotFound
	}
	return label, nil
}

func (l *labels) List(_ context.Context, collection shared.ID) ([]domain.Label, error) {
	if l.listErr != nil {
		return nil, l.listErr
	}

	vocabulary := make([]domain.Label, 0, len(l.stored))
	for _, label := range l.stored {
		if label.CollectionID == collection && !label.IsDeleted() {
			vocabulary = append(vocabulary, label)
		}
	}
	sortLabels(vocabulary)
	return vocabulary, nil
}

func (l *labels) Insert(_ context.Context, label domain.Label) error {
	if l.insertErr != nil {
		return l.insertErr
	}
	l.inserted = append(l.inserted, label)
	l.stored[label.ID] = label
	return nil
}

func (l *labels) SetAttributes(_ context.Context, label domain.Label, expected int) error {
	return l.write("attributes", label, expected)
}

func (l *labels) SetDeleted(_ context.Context, label domain.Label, expected int) error {
	return l.write("deleted", label, expected)
}

func (l *labels) write(method string, label domain.Label, expected int) error {
	if l.writeErr != nil {
		return l.writeErr
	}
	l.written = append(l.written, writtenLabel{method: method, label: label, expected: expected})
	l.stored[label.ID] = label
	return nil
}

var _ repository.Labels = (*labels)(nil)

// sortLabels orders a vocabulary the way the query does: by name, then by identifier.
func sortLabels(vocabulary []domain.Label) {
	for i := 1; i < len(vocabulary); i++ {
		for j := i; j > 0; j-- {
			left, right := vocabulary[j-1], vocabulary[j]
			if left.Name < right.Name || (left.Name == right.Name && left.ID < right.ID) {
				break
			}
			vocabulary[j-1], vocabulary[j] = right, left
		}
	}
}

type labelHarness struct {
	create     CreateLabel
	list       ListLabels
	labels     *labels
	containers *containers
	events     *events
	changes    *changes
	audit      *sink
	authorizer *authorizer
	uow        *unitOfWork
}

func newLabelHarness() *labelHarness {
	vocabulary := &labels{stored: map[shared.ID]domain.Label{}}
	store := &containers{stored: map[shared.ID]domain.Container{}}
	h := &labelHarness{
		labels: vocabulary, containers: store, events: &events{}, changes: &changes{},
		audit: &sink{}, authorizer: &authorizer{}, uow: &unitOfWork{},
	}
	h.create = CreateLabel{
		Labels: vocabulary, Containers: store, Authorizer: h.authorizer, Events: h.events,
		Changes: h.changes, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.list = ListLabels{
		Labels: vocabulary, Containers: store, Authorizer: h.authorizer, UnitOfWork: h.uow,
	}
	h.withCollection()
	return h
}

func (h *labelHarness) withCollection() {
	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
}

func (h *labelHarness) withLabel(id shared.ID, name string) domain.Label {
	label := domain.Label{
		ID: id, TenantID: tenantID, CollectionID: collectionID,
		Name: name, ColorToken: "accent.red", Version: 1,
	}
	h.labels.stored[id] = label
	return label
}

func labelCommand() CreateLabelCommand {
	return CreateLabelCommand{
		CollectionID: collectionID, Name: "  Urgent  ", ColorToken: "accent.red",
	}
}

// One write owes four things, and this is the test that says so.
func TestCreatingALabelWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newLabelHarness()
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	label, err := h.create.Execute(ctx, actor(), labelCommand())
	if err != nil {
		t.Fatalf("creating the label failed: %v", err)
	}

	if label.Name != "Urgent" || label.ColorToken != "accent.red" || label.Version != 1 {
		t.Errorf("unexpected label: %+v", label)
	}
	if label.TenantID != tenantID || label.CollectionID != collectionID {
		t.Errorf("the label was written into the wrong place: %+v", label)
	}
	if len(h.labels.inserted) != 1 || !h.uow.committed {
		t.Fatalf("%d labels written, committed %v", len(h.labels.inserted), h.uow.committed)
	}

	t.Run("the event", func(t *testing.T) {
		if len(h.events.appended) != 1 {
			t.Fatalf("%d events, want 1", len(h.events.appended))
		}
		announcement := h.events.appended[0]
		if announcement.Type != event.LabelCreated {
			t.Errorf("event type %s", announcement.Type)
		}
		if announcement.Payload["id"] != label.ID.String() {
			t.Errorf("the event describes another label: %v", announcement.Payload["id"])
		}
	})

	t.Run("the change for offline clients", func(t *testing.T) {
		if len(h.changes.recorded) != 1 {
			t.Fatalf("%d changes, want 1", len(h.changes.recorded))
		}
		change := h.changes.recorded[0]
		if change.Entity != "label" || change.EntityID != label.ID {
			t.Errorf("the change describes something else: %+v", change)
		}
		if change.ContainerID != hubID {
			t.Errorf("the change is filed under %s, want the hub", change.ContainerID)
		}
	})

	// The name is user content and is recorded as a fingerprint; the colour is a theme token this
	// installation defined and carries no personal data (rule 10, audit.md §4).
	t.Run("the audit entry", func(t *testing.T) {
		if len(h.audit.entries) != 1 {
			t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
		}
		entry := h.audit.entries[0]
		if entry.Action != LabelCreatedAction || entry.TargetID != label.ID {
			t.Errorf("unexpected entry: %+v", entry)
		}
		recorded, _ := entry.Changes[domain.FieldName].(map[string]any)
		if recorded == nil || recorded["changed"] != true {
			t.Fatalf("the name is not in the trail: %+v", entry.Changes)
		}
		if _, readable := recorded["to"]; readable {
			t.Errorf("the name is in the trail in clear text: %+v", recorded)
		}
		colour, _ := entry.Changes[domain.FieldColorToken].(map[string]any)
		if colour == nil || colour["to"] != "accent.red" {
			t.Errorf("the colour is not readable in the trail: %+v", entry.Changes)
		}
	})
}

// The permission question is asked against the collection's path, so that a membership held at the
// hub applies downwards, and before the transaction opens.
func TestCreatingALabelAsksAboutTheCollectionsPath(t *testing.T) {
	h := newLabelHarness()

	if _, err := h.create.Execute(context.Background(), actor(), labelCommand()); err != nil {
		t.Fatalf("creating the label failed: %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions, want 1", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.Permission != service.PermissionStructure || request.Action != LabelCreatedAction {
		t.Errorf("unexpected request: %+v", request)
	}
	if len(request.Path) != 3 {
		t.Errorf("the path is %+v, want tenant, hub and collection", request.Path)
	}
}

func TestARefusedLabelCreateWritesNothing(t *testing.T) {
	h := newLabelHarness()
	h.authorizer.err = shared.ErrForbidden

	_, err := h.create.Execute(context.Background(), actor(), labelCommand())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal did not come back: %v", err)
	}
	if len(h.labels.inserted) != 0 || h.uow.writes != 0 {
		t.Error("a refused create wrote something")
	}
}

// A label is a vocabulary the people working in one collection agree on. A hub holds collections
// and no entries, so a label on one would tag nothing.
func TestALabelNeedsACollection(t *testing.T) {
	h := newLabelHarness()

	t.Run("a hub is refused", func(t *testing.T) {
		cmd := labelCommand()
		cmd.CollectionID = hubID

		_, err := h.create.Execute(context.Background(), actor(), cmd)
		if shared.AsError(err).DetailCode != "items.collection_required" {
			t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
		}
	})

	t.Run("no collection at all is refused", func(t *testing.T) {
		cmd := labelCommand()
		cmd.CollectionID = ""

		_, err := h.create.Execute(context.Background(), actor(), cmd)
		if shared.AsError(err).DetailCode != "items.collection_id_required" {
			t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
		}
	})

	t.Run("a collection nobody has is not found", func(t *testing.T) {
		cmd := labelCommand()
		cmd.CollectionID = shared.MustParseID("0192f000-0000-7000-8000-00000000009f")

		_, err := h.create.Execute(context.Background(), actor(), cmd)
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("an unknown collection was accepted: %v", err)
		}
	})
}

// A label is rendered as a chip and nothing else: with no colour a client would have to invent one,
// which is how two clients come to render the same label differently.
func TestALabelNeedsAColour(t *testing.T) {
	h := newLabelHarness()

	cmd := labelCommand()
	cmd.ColorToken = ""

	_, err := h.create.Execute(context.Background(), actor(), cmd)
	if shared.AsError(err).DetailCode != "labels.color_token_empty" {
		t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
	}
}

func TestALabelIsRefusedOnAnArchivedCollection(t *testing.T) {
	h := newLabelHarness()
	collection := h.containers.stored[collectionID]
	archivedAt := now
	collection.ArchivedAt = &archivedAt
	h.containers.stored[collectionID] = collection

	_, err := h.create.Execute(context.Background(), actor(), labelCommand())
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a label was added to an archived collection: %v", err)
	}
}

// The read side never opens a write transaction: a read may be served by a replica.
func TestListingAVocabularyReadsOnly(t *testing.T) {
	h := newLabelHarness()
	h.withLabel(urgentLabel, "Urgent")
	h.withLabel(blockedLabel, "Blocked")

	vocabulary, err := h.list.Execute(context.Background(), actor(), ListLabelsQuery{
		CollectionID: collectionID,
	})
	if err != nil {
		t.Fatalf("listing the vocabulary failed: %v", err)
	}

	if len(vocabulary) != 2 || vocabulary[0].Name != "Blocked" {
		t.Fatalf("the vocabulary is %+v, want it by name", vocabulary)
	}
	if h.uow.writes != 0 {
		t.Errorf("%d write transactions were opened by a read", h.uow.writes)
	}
}

func TestListingAVocabularyNeedsACollection(t *testing.T) {
	h := newLabelHarness()

	_, err := h.list.Execute(context.Background(), actor(), ListLabelsQuery{})
	if shared.AsError(err).DetailCode != "items.collection_id_required" {
		t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
	}
}

func TestARefusedVocabularyReadReturnsNothing(t *testing.T) {
	h := newLabelHarness()
	h.withLabel(urgentLabel, "Urgent")
	h.authorizer.err = shared.ErrForbidden

	vocabulary, err := h.list.Execute(context.Background(), actor(), ListLabelsQuery{
		CollectionID: collectionID,
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal did not come back: %v", err)
	}
	if vocabulary != nil {
		t.Errorf("a refused read answered with %+v", vocabulary)
	}
}

// The description is an explicit null rather than an omission: a client renders the label from
// this, and a field that appeared only once somebody had set it is one it cannot read
// unconditionally.
func TestALabelWithoutADescriptionCarriesANull(t *testing.T) {
	h := newLabelHarness()

	out, err := h.create.invoke(context.Background(), actor(), map[string]any{
		"collection_id": collectionID.String(), "name": "Urgent", "color_token": "accent.red",
	})
	if err != nil {
		t.Fatalf("creating the label failed: %v", err)
	}

	value, present := out["description"]
	if !present || value != nil {
		t.Errorf("description is %v, want null", value)
	}
}

func TestTheLabelDescriptorsCarryWhatTheChannelsNeed(t *testing.T) {
	create := CreateLabel{}.Descriptor()
	if create.Name != CreateLabelName || !create.Audit.Required {
		t.Errorf("unexpected descriptor: %+v", create)
	}

	list := ListLabels{}.Descriptor()
	if !list.ReadOnly || list.Audit.Required {
		t.Errorf("a read that is not read only, or that insists on an entry: %+v", list)
	}
}
