// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// templates is the in-memory fake of the template store, with the one rule the index enforces: two
// live templates in one scope may not share a name.
type templates struct {
	stored   map[shared.ID]domain.Template
	inserted []domain.Template
	updates  []domain.Template
	deleted  []domain.Template
}

func newTemplates() *templates {
	return &templates{stored: map[shared.ID]domain.Template{}}
}

func (s *templates) Find(_ context.Context, id shared.ID) (domain.Template, error) {
	template, found := s.stored[id]
	if !found {
		return domain.Template{}, shared.ErrNotFound
	}
	return template, nil
}

func (s *templates) ListInScopes(
	_ context.Context, scopeIDs []shared.ID, _ repository.Page,
) (repository.TemplatePage, error) {
	reachable := map[shared.ID]bool{}
	for _, id := range scopeIDs {
		reachable[id] = true
	}

	page := repository.TemplatePage{}
	for _, template := range s.stored {
		if template.DeletedAt != nil {
			continue
		}
		if template.Scope == domain.TemplateScopeTenant || reachable[template.ScopeID] {
			page.Templates = append(page.Templates, template)
		}
	}
	return page, nil
}

func (s *templates) Insert(_ context.Context, template domain.Template) error {
	for _, stored := range s.stored {
		if stored.DeletedAt == nil && stored.Scope == template.Scope &&
			stored.ScopeID == template.ScopeID && stored.Name == template.Name {
			return shared.ErrConflict.WithDetail("templates.name_taken")
		}
	}
	s.inserted = append(s.inserted, template)
	s.stored[template.ID] = template
	return nil
}

func (s *templates) Update(
	_ context.Context, template domain.Template, expectedVersion int,
) error {
	stored, found := s.stored[template.ID]
	if !found || stored.Version != expectedVersion || stored.DeletedAt != nil {
		return shared.ErrVersionConflict.WithDetail("templates.version_conflict")
	}
	s.updates = append(s.updates, template)
	written := template
	written.Version = expectedVersion + 1
	s.stored[template.ID] = written
	return nil
}

func (s *templates) SetDeleted(
	_ context.Context, template domain.Template, expectedVersion int,
) error {
	stored, found := s.stored[template.ID]
	if !found || stored.Version != expectedVersion || stored.DeletedAt != nil {
		return shared.ErrVersionConflict.WithDetail("templates.version_conflict")
	}
	s.deleted = append(s.deleted, template)
	written := template
	written.Version = expectedVersion + 1
	s.stored[template.ID] = written
	return nil
}

// templateProfiles is the copy's fixture with the due date put back where the matrix grants it:
// every type carries one, and a template's relative dates are only testable where it does.
func templateProfiles() []domain.CapabilityProfile {
	rows := copyProfiles()
	for i := range rows {
		rows[i].Capabilities = append(rows[i].Capabilities, domain.CapabilityDueDate)
	}
	return rows
}

type templateHarness struct {
	create      CreateTemplate
	update      UpdateTemplate
	remove      DeleteTemplate
	get         GetTemplate
	list        ListTemplates
	instantiate InstantiateTemplate
	templates   *templates
	items       *items
	containers  *containers
	changes     *changes
	audit       *sink
	events      *events
	history     *journal
	visibility  *visibility
	authorizer  *authorizer
}

func newTemplateHarness(t *testing.T) *templateHarness {
	t.Helper()

	h := &templateHarness{
		templates:  newTemplates(),
		items:      &items{stored: map[shared.ID]domain.WorkItem{}, lastKey: "a0"},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		changes:    &changes{}, audit: &sink{}, events: &events{}, history: &journal{},
		visibility: newVisibility(accountID, colleagueAccountID),
		authorizer: &authorizer{},
	}

	writer := TemplateWriter{
		Templates: h.templates, Containers: h.containers,
		Profiles: &profiles{rows: templateProfiles()}, Authorizer: h.authorizer,
		Changes: h.changes, Audit: h.audit,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.create = CreateTemplate{Writer: writer}
	h.update = UpdateTemplate{Writer: writer}
	h.remove = DeleteTemplate{Writer: writer}
	h.get = GetTemplate{Writer: writer}
	h.list = ListTemplates{
		Templates: h.templates, Containers: h.containers, Authorizer: h.authorizer,
		UnitOfWork: &unitOfWork{},
	}
	h.instantiate = InstantiateTemplate{
		Writer: writer, Items: h.items, ItemMembers: newItemMembers(),
		Visibility: h.visibility, Events: h.events,
		Activity: ActivityJournal{Entries: h.history, IDs: &ids{}},
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

// moveSpec is the fixture: a task with two steps, the second due three days in and always on a
// colleague.
func moveSpec(t *testing.T) domain.TemplateSpec {
	t.Helper()

	offset, err := domain.ParseTemplateOffset("P3D")
	if err != nil {
		t.Fatalf("the offset was refused: %v", err)
	}
	return domain.TemplateSpec{
		Scope: string(domain.TemplateScopeCollection), ScopeID: collectionID,
		Name: "Move house", RootType: string(domain.ItemTask),
		Root: domain.TemplateNode{
			Type: domain.ItemTask, Title: "Move house",
			Children: []domain.TemplateNode{
				{Type: domain.ItemWorkPackage, Title: "Book the van"},
				{
					Type: domain.ItemWorkPackage, Title: "Pack the kitchen",
					DueOffset: &offset, DueDateOnly: true, AssigneeID: colleagueAccountID,
				},
			},
		},
	}
}

// One definition owes three things: the row, the record an offline client reads, and the audit
// entry - and the permission it asks for is STRUCTURE at the template's own scope.
func TestDefiningATemplateWritesTheRowTheChangeAndTheEntry(t *testing.T) {
	h := newTemplateHarness(t)

	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
		Spec: moveSpec(t),
	})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	if template.NodeCount() != 3 || template.Version != 1 {
		t.Errorf("the template is %+v", template)
	}
	if len(h.templates.inserted) != 1 {
		t.Fatalf("rows written: %d", len(h.templates.inserted))
	}
	if len(h.authorizer.requests) != 1 ||
		h.authorizer.requests[0].Permission != service.PermissionStructure {
		t.Errorf("the permission asked for is %+v", h.authorizer.requests)
	}
	if len(h.changes.recorded) != 1 || h.changes.recorded[0].Entity != "template" {
		t.Fatalf("the change entries are %+v", h.changes.recorded)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != TemplateCreatedAction {
		t.Fatalf("the audit entries are %+v", h.audit.entries)
	}
}

// The check the definition exists for: a tree the profiles do not permit is refused where it is
// written, with the path of the node that cannot sit there.
func TestATreeTheProfilesRefuseIsRefusedAtTheDefinition(t *testing.T) {
	h := newTemplateHarness(t)

	spec := moveSpec(t)
	spec.Root.Children[0].Children = []domain.TemplateNode{
		{Type: domain.ItemTask, Title: "A task under a work package"},
	}

	_, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{Spec: spec})
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "templates.node_not_permitted" {
		t.Fatalf("refused as %v, want templates.node_not_permitted", err)
	}
	if len(refusal.Fields) != 1 ||
		refusal.Fields[0].Path != "/nodes/0/children/0/children/0/type" {
		t.Errorf("the refusal points at %v", refusal.Fields)
	}
	if len(h.templates.inserted) != 0 {
		t.Error("the template was stored despite the refusal")
	}
}

// The acceptance's own sentence: relative dates become absolute ones against the anchor, in the
// right zone, with the all-day flags preserved.
func TestInstantiatingResolvesTheDatesAndWritesTheTree(t *testing.T) {
	h := newTemplateHarness(t)
	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
		Spec: moveSpec(t),
	})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	anchor := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	result, err := h.instantiate.Execute(t.Context(), actor(), InstantiateTemplateCommand{
		TemplateID: template.ID, CollectionID: collectionID, Anchor: anchor,
	})
	if err != nil {
		t.Fatalf("instantiating failed: %v", err)
	}

	if result.Created != 3 || result.Root.Title != "Move house" {
		t.Fatalf("the instantiation produced %+v", result)
	}
	if len(h.items.stored) != 3 {
		t.Fatalf("%d entries were written", len(h.items.stored))
	}

	var packing domain.WorkItem
	for _, item := range h.items.stored {
		if item.Title == "Pack the kitchen" {
			packing = item
		}
	}
	if packing.Due == nil || !packing.Due.At.Equal(anchor.AddDate(0, 0, 3)) {
		t.Errorf("the relative date became %v", packing.Due)
	}
	if !packing.Due.DateOnly {
		t.Error("the all-day flag was lost")
	}
	if packing.AssigneeID != colleagueAccountID {
		t.Errorf("the fixed assignee became %q", packing.AssigneeID)
	}
	if packing.ParentID != result.Root.ID {
		t.Errorf("the step hangs from %q rather than from the root", packing.ParentID)
	}

	// Every entry announces itself, and the act announces itself once.
	created, instantiated := 0, 0
	for _, envelope := range h.events.appended {
		switch envelope.Type {
		case event.ItemCreated:
			created++
		case event.TemplateInstantiated:
			instantiated++
			if envelope.Payload["template_id"] != template.ID.String() {
				t.Errorf("the announcement names %v", envelope.Payload["template_id"])
			}
		}
	}
	if created != 3 || instantiated != 1 {
		t.Errorf("%d entries and %d instantiations were announced", created, instantiated)
	}
	if len(h.history.entries) != 3 {
		t.Errorf("%d history steps were written", len(h.history.entries))
	}
}

// I-W6 at the moment it matters: a template written last year may name somebody who has left, and
// the loss is reported rather than written.
func TestAnAssigneeTheDestinationCannotReachIsReported(t *testing.T) {
	h := newTemplateHarness(t)
	spec := moveSpec(t)
	stranger := shared.MustParseID("0192f000-0000-7000-8000-0000000000fd")
	spec.Root.Children[1].AssigneeID = stranger

	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{Spec: spec})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	result, err := h.instantiate.Execute(t.Context(), actor(), InstantiateTemplateCommand{
		TemplateID: template.ID, CollectionID: collectionID,
	})
	if err != nil {
		t.Fatalf("instantiating failed: %v", err)
	}

	if len(result.DroppedReferences) != 1 {
		t.Fatalf("the losses are %+v", result.DroppedReferences)
	}
	dropped := result.DroppedReferences[0]
	if dropped.Kind != domain.ReferenceAssignee || dropped.ID != stranger.String() {
		t.Errorf("the loss is %+v", dropped)
	}
	for _, item := range h.items.stored {
		if item.AssigneeID == stranger {
			t.Error("the entry was written on somebody who cannot see the collection")
		}
	}
}

// The instantiation asks for the right to create entries where the tree lands, not for the right
// to define templates: the two are different acts by different people.
func TestInstantiatingAsksForTheRightToCreateEntries(t *testing.T) {
	h := newTemplateHarness(t)
	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
		Spec: moveSpec(t),
	})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	before := len(h.authorizer.requests)
	if _, err := h.instantiate.Execute(t.Context(), actor(), InstantiateTemplateCommand{
		TemplateID: template.ID, CollectionID: collectionID,
	}); err != nil {
		t.Fatalf("instantiating failed: %v", err)
	}

	asked := h.authorizer.requests[before]
	if asked.Permission != service.PermissionWriteItems {
		t.Errorf("the permission asked for is %s", asked.Permission)
	}
}

// A deletion is soft and idempotent, and what it stamped out is not touched.
func TestDeletingATemplateLeavesItsTreesStanding(t *testing.T) {
	h := newTemplateHarness(t)
	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
		Spec: moveSpec(t),
	})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}
	if _, err := h.instantiate.Execute(t.Context(), actor(), InstantiateTemplateCommand{
		TemplateID: template.ID, CollectionID: collectionID,
	}); err != nil {
		t.Fatalf("instantiating failed: %v", err)
	}

	if err := h.remove.Execute(t.Context(), actor(), ChangeTemplateCommand{
		TemplateID: template.ID,
	}); err != nil {
		t.Fatalf("deleting the template failed: %v", err)
	}
	if len(h.items.stored) != 3 {
		t.Errorf("the deletion took %d entries with it", 3-len(h.items.stored))
	}

	// And it is gone as far as anybody asking for it is concerned.
	if _, err := h.get.Execute(t.Context(), actor(), template.ID); err == nil {
		t.Error("a deleted template was read back")
	}
	if err := h.remove.Execute(t.Context(), actor(), ChangeTemplateCommand{
		TemplateID: template.ID,
	}); err == nil {
		t.Error("deleting it twice succeeded, and there is nothing left to delete")
	}
}

// A change is a merge patch, and the tree is one member of it: what is not sent is not touched.
func TestChangingATemplateTouchesOnlyWhatWasSent(t *testing.T) {
	h := newTemplateHarness(t)
	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
		Spec: moveSpec(t),
	})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	renamed := "Move flat"
	changed, err := h.update.Execute(t.Context(), actor(), ChangeTemplateCommand{
		TemplateID: template.ID, ExpectedVersion: template.Version,
		Patch: domain.TemplatePatch{Name: &renamed},
	})
	if err != nil {
		t.Fatalf("the change failed: %v", err)
	}
	if changed.Name != renamed || changed.NodeCount() != 3 || changed.Version != 2 {
		t.Errorf("the changed template is %+v", changed)
	}
	if len(h.templates.updates) != 1 {
		t.Fatalf("the writes are %+v", h.templates.updates)
	}
	if h.audit.entries[len(h.audit.entries)-1].Action != TemplateUpdatedAction {
		t.Errorf("the audit entries are %+v", h.audit.entries)
	}

	// A patch that says what is already stored writes nothing, and still honours the If-Match.
	before := len(h.templates.updates)
	if _, err := h.update.Execute(t.Context(), actor(), ChangeTemplateCommand{
		TemplateID: template.ID, Patch: domain.TemplatePatch{Name: &renamed},
	}); err != nil {
		t.Fatalf("the repeat failed: %v", err)
	}
	if len(h.templates.updates) != before {
		t.Error("a repeat wrote something")
	}

	stale := "Move somewhere"
	_, err = h.update.Execute(t.Context(), actor(), ChangeTemplateCommand{
		TemplateID: template.ID, ExpectedVersion: 99,
		Patch: domain.TemplatePatch{Name: &stale},
	})
	if err == nil {
		t.Error("a stale If-Match was accepted")
	}

	// An update naming no member at all is a request that says nothing.
	if _, err := h.update.Execute(t.Context(), actor(), ChangeTemplateCommand{
		TemplateID: template.ID,
	}); err == nil {
		t.Error("an empty update was accepted")
	}
}

// A changed tree is checked against the profiles exactly as a new one is: the check is about what
// is being stored rather than about when.
func TestAChangedTreeIsCheckedToo(t *testing.T) {
	h := newTemplateHarness(t)
	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
		Spec: moveSpec(t),
	})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	tree := template.Root
	tree.Children = append(tree.Children, domain.TemplateNode{
		Type: domain.ItemTask, Title: "A task under a task",
	})
	_, err = h.update.Execute(t.Context(), actor(), ChangeTemplateCommand{
		TemplateID: template.ID, Patch: domain.TemplatePatch{Root: &tree},
	})
	if got := shared.AsError(err); got == nil || got.DetailCode != "templates.node_not_permitted" {
		t.Fatalf("refused as %v", err)
	}
}

// The scope is where a template lives and who may shape it, and the three ways of getting it wrong
// are answered before anything is written.
func TestATemplateScopeIsCheckedBeforeAnythingIsWritten(t *testing.T) {
	for name, test := range map[string]struct {
		spec     func(domain.TemplateSpec) domain.TemplateSpec
		wantCode string
	}{
		"a container that is not the type the scope says": {
			spec: func(s domain.TemplateSpec) domain.TemplateSpec {
				s.Scope, s.ScopeID = string(domain.TemplateScopeHub), collectionID
				return s
			},
			wantCode: "templates.scope_container_mismatched",
		},
		"a container that is not there": {
			spec: func(s domain.TemplateSpec) domain.TemplateSpec {
				s.ScopeID = shared.MustParseID("0192f000-0000-7000-8000-0000000000ee")
				return s
			},
			wantCode: "containers.not_found",
		},
		"a scope that needs a container and has none": {
			spec: func(s domain.TemplateSpec) domain.TemplateSpec {
				s.ScopeID = ""
				return s
			},
			wantCode: "templates.scope_id_required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newTemplateHarness(t)

			_, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
				Spec: test.spec(moveSpec(t)),
			})
			if got := shared.AsError(err); got == nil || got.DetailCode != test.wantCode {
				t.Fatalf("refused as %v, want %s", err, test.wantCode)
			}
			if len(h.templates.inserted) != 0 {
				t.Error("the template was stored despite the refusal")
			}
		})
	}
}

// A workspace-wide template is judged at the tenant, which is where an administrator holds
// STRUCTURE and nobody else does.
func TestAWorkspaceWideTemplateIsJudgedAtTheTenant(t *testing.T) {
	h := newTemplateHarness(t)
	spec := moveSpec(t)
	spec.Scope, spec.ScopeID = string(domain.TemplateScopeTenant), ""

	if _, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
		Spec: spec,
	}); err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	asked := h.authorizer.requests[0]
	if len(asked.Path) != 1 || asked.Path[0].Type != identity.ScopeTenant {
		t.Errorf("the permission was asked about %+v", asked.Path)
	}
}

// The list is the path's, and the read of one template answers what the caller may see - and
// exactly what a missing template answers otherwise (T-04).
func TestTheListAndTheReadAnswerWhatTheCallerMaySee(t *testing.T) {
	h := newTemplateHarness(t)
	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
		Spec: moveSpec(t),
	})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	page, err := h.list.Execute(t.Context(), actor(), ListTemplatesQuery{
		ContainerID: collectionID,
	})
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if len(page.Templates) != 1 || page.Templates[0].ID != template.ID {
		t.Errorf("the list is %+v", page.Templates)
	}

	read, err := h.get.Execute(t.Context(), actor(), template.ID)
	if err != nil {
		t.Fatalf("reading failed: %v", err)
	}
	if read.NodeCount() != 3 {
		t.Errorf("the template read as %+v", read)
	}

	// What the caller may not see answers that there is no such template rather than that they
	// may not have it.
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")
	_, err = h.get.Execute(t.Context(), actor(), template.ID)
	if got := shared.AsError(err); got == nil || got.DetailCode != "templates.not_found" {
		t.Fatalf("a template the caller may not see answered %v", err)
	}
}

// The anchor defaults to today in the caller's own zone, which is what makes "+3 days" mean the
// same thing to the person who wrote the template and the person using it.
func TestTheAnchorDefaultsToTodayInTheCallersZone(t *testing.T) {
	h := newTemplateHarness(t)
	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
		Spec: moveSpec(t),
	})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	acting := actor()
	acting.TimeZone = "Europe/Berlin"
	if _, err := h.instantiate.Execute(t.Context(), acting, InstantiateTemplateCommand{
		TemplateID: template.ID, CollectionID: collectionID,
	}); err != nil {
		t.Fatalf("instantiating failed: %v", err)
	}

	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range h.items.stored {
		if item.Due == nil {
			continue
		}
		local := item.Due.At.In(berlin)
		if local.Hour() != 0 || local.Minute() != 0 {
			t.Errorf("the date is %s local rather than the start of a day",
				local.Format(time.RFC3339))
		}
		if !local.After(now.In(berlin)) {
			t.Errorf("the date %s is not after today", local.Format(time.RFC3339))
		}
	}
}

// An instantiation into a collection that is not there, or under a parent from another collection,
// is refused before anything is written.
func TestAnInstantiationNeedsSomewhereToLand(t *testing.T) {
	h := newTemplateHarness(t)
	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{
		Spec: moveSpec(t),
	})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	if _, err := h.instantiate.Execute(t.Context(), actor(), InstantiateTemplateCommand{
		TemplateID: template.ID,
	}); err == nil {
		t.Error("an instantiation with no collection was accepted")
	}

	_, err = h.instantiate.Execute(t.Context(), actor(), InstantiateTemplateCommand{
		TemplateID:   template.ID,
		CollectionID: shared.MustParseID("0192f000-0000-7000-8000-0000000000ef"),
	})
	if got := shared.AsError(err); got == nil || got.DetailCode != "containers.not_found" {
		t.Fatalf("refused as %v", err)
	}
	if len(h.items.stored) != 0 {
		t.Error("entries were written despite the refusal")
	}
}

// The catalogue's door, which is the one every channel comes through: an untyped document in, a
// projection out. What it proves beyond the typed path is the tree's own parsing - a client sends
// nodes as a document, and a node with a malformed offset is refused by the node rather than by
// the tree.
func TestTheCatalogueDoorTakesTheWholeTemplate(t *testing.T) {
	h := newTemplateHarness(t)

	document := map[string]any{
		"scope_type": string(domain.TemplateScopeCollection),
		"scope_id":   collectionID.String(),
		"name":       "Move house",
		"root_type":  string(domain.ItemTask),
		"nodes": []any{
			map[string]any{
				"type": string(domain.ItemTask), "title": "Move house",
				"children": []any{
					map[string]any{
						"type": string(domain.ItemWorkPackage), "title": "Pack the kitchen",
						"due_offset": "P3D", "due_date_only": true,
						"assignee_id": colleagueAccountID.String(),
					},
				},
			},
		},
	}

	out, err := h.create.Descriptor().Handler.Invoke(t.Context(), actor(), document)
	if err != nil {
		t.Fatalf("defining through the catalogue failed: %v", err)
	}
	if out["name"] != "Move house" {
		t.Fatalf("the projection is %+v", out)
	}
	nodes, ok := out["nodes"].([]usecase.Output)
	if !ok || len(nodes) != 1 {
		t.Fatalf("the tree came back as %+v", out["nodes"])
	}
	children, ok := nodes[0]["children"].([]usecase.Output)
	if !ok || len(children) != 1 || children[0]["due_offset"] != "P3D" {
		t.Fatalf("the child came back as %+v", nodes[0]["children"])
	}
	templateID := out.String("id")

	// The read, the list and the change, through the same door.
	if _, err := h.get.Descriptor().Handler.Invoke(t.Context(), actor(),
		usecase.Input{"template_id": templateID}); err != nil {
		t.Fatalf("reading through the catalogue failed: %v", err)
	}
	listed, err := h.list.Descriptor().Handler.Invoke(t.Context(), actor(),
		usecase.Input{"container_id": collectionID.String()})
	if err != nil {
		t.Fatalf("listing through the catalogue failed: %v", err)
	}
	if rows, ok := listed["data"].([]usecase.Output); !ok || len(rows) != 1 {
		t.Errorf("the list came back as %+v", listed["data"])
	}

	changed, err := h.update.Descriptor().Handler.Invoke(t.Context(), actor(), usecase.Input{
		"template_id": templateID, "description": "Everything a move needs",
	})
	if err != nil {
		t.Fatalf("changing through the catalogue failed: %v", err)
	}
	if changed["description"] != "Everything a move needs" {
		t.Errorf("the change came back as %+v", changed)
	}

	// The instantiation, with the anchor as the date a client sends.
	acting := actor()
	acting.TimeZone = "Europe/Berlin"
	instance, err := h.instantiate.Descriptor().Handler.Invoke(t.Context(), acting, usecase.Input{
		"template_id": templateID, "collection_id": collectionID.String(),
		"anchor_date": "2026-09-07", "title": "Move to the new flat",
	})
	if err != nil {
		t.Fatalf("instantiating through the catalogue failed: %v", err)
	}
	if instance["created"] != 2 {
		t.Errorf("the instantiation reports %v entries", instance["created"])
	}
	for _, item := range h.items.stored {
		if item.ParentID.IsZero() && item.Title != "Move to the new flat" {
			t.Errorf("the root is called %q rather than what the request asked for", item.Title)
		}
	}

	// A date that is not one is refused by the field a client sent.
	if _, err := h.instantiate.Descriptor().Handler.Invoke(t.Context(), acting, usecase.Input{
		"template_id": templateID, "collection_id": collectionID.String(),
		"anchor_date": "next monday",
	}); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "templates.anchor_invalid" {
		t.Fatalf("an anchor that is not a date answered %v", err)
	}

	// And the deletion, which answers nothing at all.
	if _, err := h.remove.Descriptor().Handler.Invoke(t.Context(), actor(),
		usecase.Input{"template_id": templateID}); err != nil {
		t.Fatalf("deleting through the catalogue failed: %v", err)
	}
}

// A node whose offset is not a duration is refused with the path of that node, which is what makes
// a tree of fifty steps fixable.
func TestAMalformedNodeIsRefusedByItsOwnPath(t *testing.T) {
	h := newTemplateHarness(t)

	_, err := h.create.Descriptor().Handler.Invoke(t.Context(), actor(), usecase.Input{
		"scope_type": string(domain.TemplateScopeCollection),
		"scope_id":   collectionID.String(),
		"name":       "Move house",
		"root_type":  string(domain.ItemTask),
		"nodes": []any{
			map[string]any{
				"type": string(domain.ItemTask), "title": "Move house",
				"children": []any{
					map[string]any{
						"type": string(domain.ItemWorkPackage), "title": "Pack",
						"due_offset": "in three days",
					},
				},
			},
		},
	})
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "templates.due_offset_invalid" {
		t.Fatalf("refused as %v", err)
	}
	if len(refusal.Fields) != 1 ||
		refusal.Fields[0].Path != "/nodes/0/children/0/due_offset" {
		t.Errorf("the refusal points at %v", refusal.Fields)
	}
}
