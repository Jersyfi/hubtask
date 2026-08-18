// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

var (
	tenantID  = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	accountID = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	hubID     = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	now       = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
)

// The fakes. Each one records what it was given, because what the use case owes is not only a
// return value: an event, a change log entry and an audit entry are part of the result.

type containers struct {
	inserted  []domain.Container
	stored    map[shared.ID]domain.Container
	lastKey   string
	findErr   error
	insertErr error
}

func (c *containers) Find(_ context.Context, id shared.ID) (domain.Container, error) {
	if c.findErr != nil {
		return domain.Container{}, c.findErr
	}
	container, found := c.stored[id]
	if !found {
		return domain.Container{}, shared.ErrNotFound
	}
	return container, nil
}

func (c *containers) LastOrderKey(context.Context, shared.ID) (string, error) {
	return c.lastKey, nil
}

func (c *containers) Insert(_ context.Context, container domain.Container) error {
	if c.insertErr != nil {
		return c.insertErr
	}
	c.inserted = append(c.inserted, container)
	return nil
}

type events struct{ appended []event.Envelope }

func (e *events) Append(_ context.Context, envelope event.Envelope) error {
	e.appended = append(e.appended, envelope)
	return nil
}

type changes struct{ recorded []changelog.Change }

func (c *changes) Record(_ context.Context, change changelog.Change) error {
	c.recorded = append(c.recorded, change)
	return nil
}

// The fake sink judges what it is given the way the adapter does, so an entry the database would
// refuse fails here rather than in an integration test.
type sink struct{ entries []audit.Entry }

func (s *sink) Append(_ context.Context, entry audit.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	s.entries = append(s.entries, entry)
	return nil
}

type unitOfWork struct {
	committed  bool
	rolledBack bool
	// writes and reads count the two kinds separately, so a test can say that a refusal never opened a
	// write transaction and that a read never opened one at all.
	writes int
	reads  int
}

func (u *unitOfWork) Within(ctx context.Context, s persistence.Scope, fn func(context.Context) error) error {
	u.writes++
	return u.run(ctx, s, fn)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, s persistence.Scope, fn func(context.Context) error) error {
	u.reads++
	return u.run(ctx, s, fn)
}

func (u *unitOfWork) run(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		u.rolledBack = true
		return err
	}
	u.committed = true
	return nil
}

type authorizer struct {
	err      error
	requests []access.Request
}

func (a *authorizer) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	a.requests = append(a.requests, request)
	return a.err
}

// ids hands out predictable identifiers, so a test can assert on what was written.
type ids struct{ issued int }

func (i *ids) NewID() shared.ID {
	i.issued++
	return shared.MustParseID("0192f000-0000-7000-8000-00000000010" + string(rune('0'+i.issued)))
}

type hlcSource struct{ reading shared.HLC }

func (h *hlcSource) Next() shared.HLC {
	next, _ := h.reading.Tick(now, "server")
	h.reading = next
	return next
}

type harness struct {
	handler    CreateContainer
	containers *containers
	events     *events
	changes    *changes
	audit      *sink
	authorizer *authorizer
	uow        *unitOfWork
}

func newHarness() *harness {
	store := &containers{stored: map[shared.ID]domain.Container{}}
	h := &harness{
		containers: store,
		events:     &events{},
		changes:    &changes{},
		audit:      &sink{},
		authorizer: &authorizer{},
		uow:        &unitOfWork{},
	}
	h.handler = CreateContainer{
		Containers: store, Authorizer: h.authorizer, Events: h.events, Changes: h.changes,
		Audit: h.audit, UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	return h
}

func (h *harness) withHub() domain.Container {
	hub := domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[hubID] = hub
	return hub
}

func actor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
		AccountName: "Anna Beispiel", Scopes: []string{"containers:write"},
	}
}

func hubCommand() CreateContainerCommand {
	return CreateContainerCommand{Type: domain.ContainerHub, Name: "  Private  "}
}

// One write owes four things, and this is the test that says so.
func TestCreatingAHubWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newHarness()
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	container, err := h.handler.Execute(ctx, actor(), hubCommand())
	if err != nil {
		t.Fatalf("creating the hub failed: %v", err)
	}

	if container.Name != "Private" || container.Type != domain.ContainerHub || container.Version != 1 {
		t.Errorf("unexpected container: %+v", container)
	}
	if container.TenantID != tenantID {
		t.Errorf("the tenant came from somewhere other than the actor: %s", container.TenantID)
	}
	if len(h.containers.inserted) != 1 {
		t.Fatalf("%d containers written, want 1", len(h.containers.inserted))
	}
	if !h.uow.committed {
		t.Error("the transaction did not commit")
	}

	t.Run("the event", func(t *testing.T) {
		if len(h.events.appended) != 1 {
			t.Fatalf("%d events, want 1", len(h.events.appended))
		}
		announcement := h.events.appended[0]
		if announcement.Type != event.ContainerCreated {
			t.Errorf("event type %s", announcement.Type)
		}
		if announcement.Payload["id"] != container.ID.String() {
			t.Errorf("the event describes another container: %v", announcement.Payload["id"])
		}
		if announcement.Actor.ID != accountID || announcement.Actor.Kind != shared.ActorUser {
			t.Errorf("the event does not name the actor: %+v", announcement.Actor)
		}
	})

	t.Run("the change for offline clients", func(t *testing.T) {
		if len(h.changes.recorded) != 1 {
			t.Fatalf("%d changes, want 1", len(h.changes.recorded))
		}
		change := h.changes.recorded[0]
		if change.Op != changelog.Upsert || change.EntityID != container.ID || change.Entity != "container" {
			t.Errorf("unexpected change: %+v", change)
		}
		if change.HLC.IsZero() {
			t.Error("the change carries no hybrid logical clock, so it cannot be merged")
		}
		if change.ContainerID != container.ID {
			t.Errorf("a hub filters on itself, not on %s", change.ContainerID)
		}
		if change.Payload["name"] != "Private" {
			t.Errorf("the change carries no snapshot: %v", change.Payload)
		}
	})

	t.Run("the audit entry", func(t *testing.T) {
		if len(h.audit.entries) != 1 {
			t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
		}
		entry := h.audit.entries[0]
		if entry.Action != ContainerCreatedAction || entry.Outcome != audit.OutcomeSuccess {
			t.Errorf("unexpected entry: %+v", entry)
		}
		if entry.TargetID != container.ID || entry.TargetType != "container" {
			t.Errorf("the entry does not name the target: %+v", entry)
		}
		if entry.ActorLabel != "Anna Beispiel" {
			t.Error("the entry carries no readable actor, so it stops being evidence once the account is deleted")
		}
		if entry.Context.RequestID != "01J9REQUEST" {
			t.Errorf("the entry cannot be tied to the request: %+v", entry.Context)
		}
	})
}

// Rule 10 at the one place it is easiest to break: the name is user content, and the trail keeps
// it as a fingerprint rather than as text.
func TestTheAuditEntryCarriesNoContainerName(t *testing.T) {
	h := newHarness()

	container, err := h.handler.Execute(context.Background(), actor(), hubCommand())
	if err != nil {
		t.Fatalf("creating the hub failed: %v", err)
	}

	entry := h.audit.entries[0]
	if entry.TargetLabel != "" {
		t.Errorf("the container name reached the trail as a label: %q", entry.TargetLabel)
	}
	name, ok := entry.Changes["name"].(map[string]any)
	if !ok {
		t.Fatalf("the name was not recorded at all: %v", entry.Changes)
	}
	if name["to"] != nil || name["changed"] != true || name["to_hash"] == nil {
		t.Errorf("the name is not masked: %v", name)
	}
	if entry.Changes["type"].(map[string]any)["to"] != string(container.Type) {
		t.Errorf("the type was not recorded readably: %v", entry.Changes["type"])
	}
}

func TestCreatingACollectionUnderItsHub(t *testing.T) {
	h := newHarness()
	h.withHub()
	h.containers.lastKey = "a0"

	container, err := h.handler.Execute(context.Background(), actor(), CreateContainerCommand{
		Type: domain.ContainerCollection, ParentID: hubID, Name: "Shopping", ColorToken: "blue",
	})
	if err != nil {
		t.Fatalf("creating the collection failed: %v", err)
	}

	if container.ParentID != hubID || container.ColorToken != "blue" {
		t.Errorf("unexpected container: %+v", container)
	}
	// Ranked after its last sibling rather than at the same key.
	if container.OrderKey <= "a0" {
		t.Errorf("order key %q does not sort after its sibling", container.OrderKey)
	}
	// A device subscribed to the hub has to see the new collection appear.
	if change := h.changes.recorded[0]; change.ContainerID != hubID {
		t.Errorf("the change filters on %s rather than on the hub", change.ContainerID)
	}
	// The permission is asked for along the path, so a role on the hub is enough.
	if path := h.authorizer.requests[0].Path; len(path) != 2 || path[1].ID != hubID {
		t.Errorf("the permission was not asked for along the path: %+v", path)
	}
}

// The check happens before the transaction, and a refusal writes nothing at all.
func TestARefusalWritesNothing(t *testing.T) {
	h := newHarness()
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := h.handler.Execute(context.Background(), actor(), hubCommand())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if len(h.containers.inserted) != 0 || len(h.events.appended) != 0 ||
		len(h.changes.recorded) != 0 || len(h.audit.entries) != 0 {
		t.Error("a refused request wrote something")
	}
	if h.uow.committed {
		t.Error("a refused request opened and committed a transaction")
	}
	if request := h.authorizer.requests[0]; request.Permission != service.PermissionStructure ||
		request.TokenScope != "containers:write" {
		t.Errorf("the wrong permission was asked for: %+v", request)
	}
}

// Test AT-5 from the application side: an event that cannot be written takes the whole write with
// it, so there is no container without its announcement.
func TestAFailureAnywhereRollsTheWholeWriteBack(t *testing.T) {
	h := newHarness()
	h.containers.insertErr = shared.ErrConflict.WithDetail("containers.name_taken")

	_, err := h.handler.Execute(context.Background(), actor(), hubCommand())
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error %v, want a conflict", err)
	}
	if !h.uow.rolledBack || h.uow.committed {
		t.Error("the transaction did not roll back")
	}
	if len(h.audit.entries) != 0 {
		t.Error("an audit entry survived a failed write")
	}
}

func TestTheParentIsChecked(t *testing.T) {
	archived := now

	cases := map[string]struct {
		parent     *domain.Container
		command    CreateContainerCommand
		detailCode string
	}{
		"a parent that does not exist": {
			command:    CreateContainerCommand{Type: domain.ContainerCollection, ParentID: hubID, Name: "x"},
			detailCode: "containers.parent_not_found",
		},
		"a parent that is archived": {
			parent: &domain.Container{
				ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
				ArchivedAt: &archived,
			},
			command:    CreateContainerCommand{Type: domain.ContainerCollection, ParentID: hubID, Name: "x"},
			detailCode: "containers.parent_archived",
		},
		"a hub named as a parent of a hub": {
			parent: &domain.Container{
				ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
			},
			command:    CreateContainerCommand{Type: domain.ContainerHub, ParentID: hubID, Name: "x"},
			detailCode: "containers.parent_type_invalid",
		},
		"a collection without a parent": {
			command:    CreateContainerCommand{Type: domain.ContainerCollection, Name: "x"},
			detailCode: "containers.collection_needs_parent",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness()
			if c.parent != nil {
				h.containers.stored[c.parent.ID] = *c.parent
			}

			_, err := h.handler.Execute(context.Background(), actor(), c.command)
			if err == nil {
				t.Fatalf("no error, want %s", c.detailCode)
			}
			if got := shared.AsError(err).DetailCode; got != c.detailCode {
				t.Errorf("detail code %s, want %s", got, c.detailCode)
			}
			if len(h.containers.inserted) != 0 {
				t.Error("something was written anyway")
			}
		})
	}
}

// The catalogue entry is part of the use case, not of the composition root: the summary an agent
// reads, the fields every channel validates against, and the audit declaration gate SG-13 checks.
func TestTheDescriptorIsComplete(t *testing.T) {
	h := newHarness()
	descriptor := h.handler.Descriptor()

	registry, err := usecase.NewRegistry(nil, descriptor)
	if err != nil {
		t.Fatalf("the catalogue refused the entry: %v", err)
	}
	if descriptor.RESTOperation() != "createContainer" || descriptor.MCPTool() != "create_container" ||
		descriptor.AutomationAction() != "CREATE_CONTAINER" {
		t.Errorf("unexpected channel identities: %+v", descriptor)
	}
	if !descriptor.Audit.Required || descriptor.Audit.Action != ContainerCreatedAction {
		t.Errorf("the audit declaration is missing: %+v", descriptor.Audit)
	}
	if descriptor.ReadOnly || descriptor.Destructive {
		t.Error("creating a container is neither read-only nor destructive")
	}

	// Through the catalogue, the way MCP and automation reach it.
	out, err := registry.Invoke(context.Background(), CreateContainerName, actor(),
		usecase.Input{"type": "HUB", "name": "Private", "color_token": "blue"})
	if err != nil {
		t.Fatalf("the invocation failed: %v", err)
	}
	if out["type"] != "HUB" || out["name"] != "Private" || out["parent_id"] != nil {
		t.Errorf("unexpected output: %v", out)
	}
	if out["id"] == nil || out["order_key"] == nil || out["version"] != 1 {
		t.Errorf("the output is not the contract's shape: %v", out)
	}
}

// The typed path and the catalogue path are the same path - which is what makes the audit entry
// and the event identical whichever channel the call arrived through (test AT-6).
func TestTheCatalogueAndTheTypedCallProduceTheSameWrite(t *testing.T) {
	typed := newHarness()
	if _, err := typed.handler.Execute(context.Background(), actor(), hubCommand()); err != nil {
		t.Fatalf("the typed call failed: %v", err)
	}

	catalogued := newHarness()
	registry, err := usecase.NewRegistry(nil, catalogued.handler.Descriptor())
	if err != nil {
		t.Fatalf("the catalogue refused the entry: %v", err)
	}
	if _, err := registry.Invoke(context.Background(), CreateContainerName, actor(),
		usecase.Input{"type": "HUB", "name": "Private"}); err != nil {
		t.Fatalf("the catalogued call failed: %v", err)
	}

	if len(typed.audit.entries) != len(catalogued.audit.entries) {
		t.Fatal("the two paths wrote a different number of audit entries")
	}
	first, second := typed.audit.entries[0], catalogued.audit.entries[0]
	if first.Action != second.Action || first.Outcome != second.Outcome ||
		first.ActorLabel != second.ActorLabel {
		t.Errorf("the two paths wrote different entries:\n%+v\n%+v", first, second)
	}
	if typed.events.appended[0].Type != catalogued.events.appended[0].Type {
		t.Error("the two paths announced different events")
	}
}

// An input the declaration does not describe never reaches the handler.
func TestTheCatalogueRefusesAnUnknownField(t *testing.T) {
	h := newHarness()
	registry, _ := usecase.NewRegistry(nil, h.handler.Descriptor())

	_, err := registry.Invoke(context.Background(), CreateContainerName, actor(),
		usecase.Input{"type": "HUB", "name": "Private", "colour_token": "blue"})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation error", err)
	}
	if len(h.containers.inserted) != 0 {
		t.Error("the handler ran on an input nobody declared")
	}
}
