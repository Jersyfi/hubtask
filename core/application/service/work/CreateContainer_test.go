// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
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
	inserted []domain.Container
	stored   map[shared.ID]domain.Container
	lastKey  string
	// written is what the lifecycle use cases wrote, in order, each entry the whole container as the
	// use case decided it should read. The version it was written against travels beside it: a test
	// that only saw the row could not tell an update that honoured an If-Match from one that ignored
	// it.
	written []writtenContainer
	// placedBefore is the sibling Neighbours was asked about.
	placedBefore shared.ID
	writeErr     error
	// page is what List answers with, and asked is what it was asked - the read use cases care about
	// both: one is the projection they build, the other is the query they translated.
	page      repository.ContainerPage
	asked     repository.ContainerQuery
	findErr   error
	listErr   error
	insertErr error
	// The trash side: what each call was asked to do, and how many entries the cascade reports as
	// having gone with the containers - the item count is the one part the container double cannot
	// work out for itself, because entries live in the other repository.
	trashed       []repository.ContainerTrash
	restored      []repository.ContainerTrash
	cascadedItems int
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

func (c *containers) List(
	_ context.Context, query repository.ContainerQuery,
) (repository.ContainerPage, error) {
	c.asked = query
	if c.listErr != nil {
		return repository.ContainerPage{}, c.listErr
	}
	return c.page, nil
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

// writtenContainer is one write the lifecycle performed: which method, what the row now says, and
// the version it was written against.
type writtenContainer struct {
	method    string
	container domain.Container
	expected  int
}

func (c *containers) SetAttributes(_ context.Context, container domain.Container, expected int) error {
	return c.write("attributes", container, expected)
}

func (c *containers) SetPolicies(_ context.Context, container domain.Container, expected int) error {
	return c.write("policies", container, expected)
}

func (c *containers) SetArchived(_ context.Context, container domain.Container, expected int) error {
	return c.write("archived", container, expected)
}

func (c *containers) SetPlacement(_ context.Context, container domain.Container, expected int) error {
	return c.write("placement", container, expected)
}

// The trash side (B-10). The cascade is worked out from what is stored rather than configured, so
// that a test which puts two collections in a hub gets two collections back without having to say
// so twice - and so that the double cannot silently disagree with the fixture it was built from.
func (c *containers) TrashSubtree(
	_ context.Context, trash repository.ContainerTrash,
) (repository.Cascade, error) {
	if err := c.write("trashed", trash.Container, trash.ExpectedVersion); err != nil {
		return repository.Cascade{}, err
	}
	c.trashed = append(c.trashed, trash)

	cascade := repository.Cascade{Items: c.cascadedItems}
	for id, stored := range c.stored {
		if stored.ParentID != trash.Container.ID || stored.IsTrashed() {
			continue
		}
		stored.DeletedAt, stored.TrashBatchID = trash.Container.DeletedAt, trash.BatchID
		stored.Version++
		c.stored[id] = stored
		cascade.Collections = append(cascade.Collections, id)
	}
	return cascade, nil
}

func (c *containers) RestoreBatch(
	_ context.Context, restore repository.ContainerTrash,
) (repository.Cascade, error) {
	if err := c.write("restored", restore.Container, restore.ExpectedVersion); err != nil {
		return repository.Cascade{}, err
	}
	c.restored = append(c.restored, restore)

	cascade := repository.Cascade{Items: c.cascadedItems}
	for id, stored := range c.stored {
		if id == restore.Container.ID || stored.TrashBatchID != restore.BatchID {
			continue
		}
		stored.DeletedAt, stored.TrashBatchID = nil, ""
		stored.Version++
		c.stored[id] = stored
		cascade.Collections = append(cascade.Collections, id)
	}
	return cascade, nil
}

func (c *containers) write(method string, container domain.Container, expected int) error {
	if c.writeErr != nil {
		return c.writeErr
	}
	c.written = append(c.written, writtenContainer{method: method, container: container, expected: expected})
	if c.stored != nil {
		c.stored[container.ID] = container
	}
	return nil
}

// Neighbours answers from what is stored rather than from a canned pair, because the exclusion of
// the moving container from its own level is what makes a repeated move idempotent - a fake that
// returned fixed bounds would hide exactly the behaviour the tests are about.
func (c *containers) Neighbours(
	_ context.Context, parentID, beforeID, movingID shared.ID,
) (string, string, error) {
	c.placedBefore = beforeID

	var level []string
	var anchor string
	for _, container := range c.stored {
		if container.ParentID != parentID || container.ID == movingID || container.DeletedAt != nil {
			continue
		}
		level = append(level, container.OrderKey)
		if container.ID == beforeID {
			anchor = container.OrderKey
		}
	}

	var previous string
	for _, key := range level {
		if anchor != "" && key >= anchor {
			continue
		}
		if key > previous {
			previous = key
		}
	}
	return previous, anchor, nil
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

// journal is the item history the writers append to. It validates the way the adapter does, so an
// entry this system would not be able to write fails here rather than in integration.
type journal struct{ entries []activity.Entry }

func (j *journal) Record(_ context.Context, entry activity.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	j.entries = append(j.entries, entry)
	return nil
}

// verbs is what the history recorded, in order.
func (j *journal) verbs() []activity.Verb {
	written := make([]activity.Verb, 0, len(j.entries))
	for _, entry := range j.entries {
		written = append(written, entry.Verb)
	}
	return written
}

// only returns the single entry the history holds, and fails the test when it holds anything else -
// which is the assertion nearly every one of these tests wants to make.
func (j *journal) only(t *testing.T) activity.Entry {
	t.Helper()
	if len(j.entries) != 1 {
		t.Fatalf("the history holds %v, want exactly one entry", j.verbs())
	}
	return j.entries[0]
}

type unitOfWork struct {
	committed  bool
	rolledBack bool
	// writes and reads count the two kinds separately. The read side must never open a write
	// transaction - a read may be served by a replica, and one that asked for a writable transaction
	// would pin every list in the product to the primary (multi-tenancy.md §7, B-04). A fake that
	// could not tell the two apart could not say so.
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

// jobs is the queue as the writers see it: what was asked for, and under which key.
type jobs struct{ enqueued []queue.Request }

func (j *jobs) Enqueue(_ context.Context, request queue.Request) error {
	j.enqueued = append(j.enqueued, request)
	return nil
}

func (j *jobs) Claim(context.Context, queue.Lease) ([]queue.Job, error) { return nil, nil }
func (j *jobs) Complete(context.Context, queue.Job) error               { return nil }
func (j *jobs) Repeat(context.Context, queue.Job, time.Time) error      { return nil }
func (j *jobs) Fail(context.Context, queue.Failure) error               { return nil }
func (j *jobs) Depth(context.Context) ([]queue.Depth, error)            { return nil, nil }

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
