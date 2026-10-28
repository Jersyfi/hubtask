// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var (
	tenant = shared.MustParseID("018f0000-0000-7000-8000-000000000001")
	lena   = shared.MustParseID("018f0000-0000-7000-8000-0000000000a1")
)

type memoryTypes struct {
	rows  map[string]work.EntryType
	inUse map[string]bool
}

func (m *memoryTypes) List(context.Context) ([]work.EntryType, error) {
	out := make([]work.EntryType, 0, len(m.rows))
	for _, row := range m.rows {
		out = append(out, row)
	}
	return out, nil
}

func (m *memoryTypes) Find(_ context.Context, key string) (work.EntryType, error) {
	row, found := m.rows[key]
	if !found {
		return work.EntryType{}, shared.ErrNotFound.WithDetail("entry_types.not_found")
	}
	return row, nil
}

func (m *memoryTypes) Insert(_ context.Context, t work.EntryType) error {
	if _, taken := m.rows[t.Key]; taken {
		return shared.ErrConflict.WithDetail("entry_types.key_taken")
	}
	m.rows[t.Key] = t
	return nil
}

func (m *memoryTypes) Update(_ context.Context, t work.EntryType, expected int) error {
	if m.rows[t.Key].Version != expected {
		return shared.ErrVersionConflict
	}
	t.Version = expected + 1
	m.rows[t.Key] = t
	return nil
}

func (m *memoryTypes) Delete(_ context.Context, key string) error {
	delete(m.rows, key)
	return nil
}

func (m *memoryTypes) InUse(_ context.Context, key string) (bool, error) { return m.inUse[key], nil }

func (m *memoryTypes) SetOf(context.Context, work.WorkItem, int) error { return nil }

type authorizer struct {
	refuse  bool
	request access.Request
}

func (a *authorizer) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	a.request = request
	if a.refuse {
		return shared.ErrForbidden
	}
	return nil
}

type log struct{ changes []changelog.Change }

func (l *log) Record(_ context.Context, change changelog.Change) error {
	l.changes = append(l.changes, change)
	return nil
}

type trail struct{ entries []audit.Entry }

func (t *trail) Append(_ context.Context, entry audit.Entry) error {
	t.entries = append(t.entries, entry)
	return nil
}

type unitOfWork struct{}

func (unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (unitOfWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type clock struct{}

func (clock) Now() time.Time { return time.Date(2026, 10, 28, 6, 0, 0, 0, time.UTC) }

type ids struct{ n int }

func (i *ids) NewID() shared.ID {
	i.n++
	return shared.MustParseID("018f0000-0000-7000-8000-0000000001" + string(rune('0'+i.n/10)) + string(rune('0'+i.n%10)))
}

type hlc struct{}

func (hlc) Next() shared.HLC { return shared.HLC{} }

type fixture struct {
	registry workflow.Registry
	types    *memoryTypes
	auth     *authorizer
	log      *log
	trail    *trail
}

func newFixture() fixture {
	f := fixture{
		types: &memoryTypes{rows: map[string]work.EntryType{}, inUse: map[string]bool{}},
		auth:  &authorizer{}, log: &log{}, trail: &trail{},
	}
	f.registry = workflow.Registry{
		Types: f.types, Authorizer: f.auth, Changes: f.log, Audit: f.trail, UnitOfWork: unitOfWork{},
		Clock: clock{}, IDs: &ids{}, HLC: hlc{}, Text: text.Composing{},
	}
	return f
}

func actor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: lena,
		Scopes: []string{"items:read", "workspace:manage"},
	}
}

func detail(err error) string {
	var refusal *shared.Error
	if errors.As(err, &refusal) {
		return refusal.DetailCode
	}
	return ""
}

// UC-FLW-01 check 1: whoever may shape the workspace creates, renames, orders and archives types on
// each level; the list always holds the three system types.
func TestShapingAWorkspacesTypes(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	create := workflow.CreateEntryType{Registry: f.registry}
	change := workflow.ChangeEntryType{Registry: f.registry}
	list := workflow.ListEntryTypes{Registry: f.registry}

	registry, err := list.Execute(ctx, actor())
	if err != nil || len(registry) != 3 {
		t.Fatalf("a new workspace lists %d types, %v", len(registry), err)
	}

	for _, level := range []work.ItemType{work.ItemTask, work.ItemWorkPackage, work.ItemActivity} {
		key := map[work.ItemType]string{work.ItemTask: "bug", work.ItemWorkPackage: "subtask", work.ItemActivity: "check"}[level]
		if _, err := create.Execute(ctx, actor(), workflow.CreateEntryTypeCommand{
			Key: key, Level: level, Name: key, Icon: "bug", ColorToken: "red",
			Behaviours: work.EntryBehaviours{Completable: true, Schedulable: true},
		}); err != nil {
			t.Fatalf("creating %s: %v", key, err)
		}
	}
	if f.auth.request.Permission != service.PermissionStructure || f.auth.request.TokenScope != "workspace:manage" {
		t.Errorf("creating asked %+v", f.auth.request)
	}

	name, position, archive := "Defect", 2, true
	renamed, err := change.Execute(ctx, actor(), workflow.ChangeEntryTypeCommand{
		Key: "bug", Change: work.EntryTypeChange{Name: &name, Position: &position, Archived: &archive},
	})
	if err != nil || renamed.Name != "Defect" || !renamed.IsArchived() || renamed.Version != 2 {
		t.Fatalf("the change did not land: %+v, %v", renamed, err)
	}

	// A system type gains its row on its first change.
	recolour := "teal"
	task, err := change.Execute(ctx, actor(), workflow.ChangeEntryTypeCommand{Key: "task", Change: work.EntryTypeChange{ColorToken: &recolour}})
	if err != nil || !task.Stored() || !task.System || task.ColorToken != "teal" {
		t.Fatalf("the system type was not shaped: %+v, %v", task, err)
	}

	registry, _ = list.Execute(ctx, actor())
	if len(registry) != 6 {
		t.Fatalf("the registry holds %d types", len(registry))
	}
	if got := registry.Of(work.ItemTask, ""); got.ColorToken != "teal" {
		t.Errorf("an entry without a type reads %+v", got)
	}
	if len(f.log.changes) != 5 || len(f.trail.entries) != 5 {
		t.Errorf("recorded %d changes and %d audit entries", len(f.log.changes), len(f.trail.entries))
	}
	for _, entry := range f.trail.entries {
		if masked, held := entry.Changes[work.FieldName].(map[string]any); held && masked["to"] != nil {
			t.Errorf("a type's name went to the trail in the clear: %v", masked)
		}
	}

	f.auth.refuse = true
	if _, err := create.Execute(ctx, actor(), workflow.CreateEntryTypeCommand{Key: "story", Level: work.ItemTask, Name: "Story", Icon: "bug", ColorToken: "red"}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("somebody without the structure permission created a type: %v", err)
	}
}

// UC-FLW-01 check 5: a type in use cannot be deleted, only archived; a system type neither.
func TestATypeInUseIsArchivedNotDeleted(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	create := workflow.CreateEntryType{Registry: f.registry}
	remove := workflow.DeleteEntryType{Registry: f.registry}
	for _, key := range []string{"bug", "idea"} {
		if _, err := create.Execute(ctx, actor(), workflow.CreateEntryTypeCommand{
			Key: key, Level: work.ItemTask, Name: key, Icon: "bug", ColorToken: "red",
			Behaviours: work.EntryBehaviours{Completable: true},
		}); err != nil {
			t.Fatal(err)
		}
	}
	f.types.inUse["bug"] = true

	if err := remove.Execute(ctx, actor(), "bug"); detail(err) != "entry_types.in_use" {
		t.Errorf("a type in use was deleted: %v", err)
	}
	if err := remove.Execute(ctx, actor(), "idea"); err != nil {
		t.Errorf("an unused type could not be deleted: %v", err)
	}
	if _, kept := f.types.rows["idea"]; kept {
		t.Error("the deleted type is still stored")
	}
	if err := remove.Execute(ctx, actor(), "task"); !errors.Is(err, shared.ErrNotFound) && detail(err) != "entry_types.system_not_deletable" {
		t.Errorf("a system type was deleted: %v", err)
	}
}

// D-07: a record type is created without completion, even when the caller did not say so.
func TestARecordTypeHasNoCompletion(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	out, err := workflow.CreateEntryType{Registry: f.registry}.Descriptor().Handler.Invoke(ctx, actor(), usecase.Input{
		"key": "server", "level": "TASK", "name": "Server", "icon": "server", "color_token": "slate", "record": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["record"] != true || out["completable"] != false {
		t.Errorf("the record came back %v", out)
	}
	if _, err := (workflow.CreateEntryType{Registry: f.registry}).Execute(ctx, actor(), workflow.CreateEntryTypeCommand{
		Key: "risk", Level: work.ItemTask, Name: "Risk", Icon: "risk", ColorToken: "red",
		Behaviours: work.EntryBehaviours{Record: true, Completable: true},
	}); detail(err) != "entry_types.record_not_completable" {
		t.Errorf("a completable record was created: %v", err)
	}
}

// UC-FLW-01 checks 7 and 8: every operation of the registry is one descriptor of the workflows
// module, so REST, MCP, hubctl and a rule reach it alike and the gate refuses it where the module
// is off.
func TestTheRegistryBelongsToTheWorkflowsModule(t *testing.T) {
	for _, descriptor := range []usecase.Descriptor{
		workflow.ListEntryTypes{}.Descriptor(), workflow.CreateEntryType{}.Descriptor(),
		workflow.ChangeEntryType{}.Descriptor(), workflow.DeleteEntryType{}.Descriptor(),
	} {
		if descriptor.Module != module.Workflows {
			t.Errorf("%s belongs to %q", descriptor.Name, descriptor.Module)
		}
	}
	if _, err := usecase.NewRegistry(nil,
		workflow.ListEntryTypes{}.Descriptor(), workflow.CreateEntryType{}.Descriptor(),
		workflow.ChangeEntryType{}.Descriptor(), workflow.DeleteEntryType{}.Descriptor(),
	); err != nil {
		t.Fatalf("the registry refused the descriptors: %v", err)
	}
}
