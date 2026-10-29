// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// entryTypes is the registry double: the workspace's rows, and every type write.
type entryTypes struct {
	rows []domain.EntryType
	set  []domain.WorkItem
}

func (e *entryTypes) List(context.Context) ([]domain.EntryType, error) { return e.rows, nil }
func (e *entryTypes) Find(_ context.Context, key string) (domain.EntryType, error) {
	for _, row := range e.rows {
		if row.Key == key {
			return row, nil
		}
	}
	return domain.EntryType{}, shared.ErrNotFound
}
func (e *entryTypes) Insert(context.Context, domain.EntryType) error      { return nil }
func (e *entryTypes) Update(context.Context, domain.EntryType, int) error { return nil }
func (e *entryTypes) Delete(context.Context, string) error                { return nil }
func (e *entryTypes) InUse(context.Context, string) (bool, error)         { return false, nil }
func (e *entryTypes) SetOf(_ context.Context, item domain.WorkItem, _ int) error {
	e.set = append(e.set, item)
	return nil
}

func workspaceTypes() *entryTypes {
	return &entryTypes{rows: []domain.EntryType{
		{
			ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e1"), TenantID: tenantID,
			Key: "bug", Level: domain.ItemTask, Name: "Bug", Icon: "bug", ColorToken: "red",
			Behaviours: domain.EntryBehaviours{Completable: true, Schedulable: true}, Version: 1,
		},
		{
			ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e2"), TenantID: tenantID,
			Key: "server", Level: domain.ItemTask, Name: "Server", Icon: "server", ColorToken: "slate",
			Behaviours: domain.EntryBehaviours{Record: true}, Version: 1,
		},
	}}
}

// UC-FLW-01 check 3: a new entry takes a type of its level, its level's own by default, and the
// type's behaviours narrow what it may carry (D-07: a record has no completion).
func TestANewEntryTakesATypeOfItsLevel(t *testing.T) {
	ctx := context.Background()
	types := workspaceTypes()
	profile := systemProfiles()[0]

	key, narrowed, err := chooseEntryType(ctx, types, domain.ItemTask, "", nil, profile, nil)
	if err != nil || key != "" || !narrowed.Allows(domain.CapabilityCompletion) {
		t.Fatalf("the default is not the level's own: %q, %v", key, err)
	}
	key, narrowed, err = chooseEntryType(ctx, types, domain.ItemTask, "server", nil, profile, nil)
	if err != nil || key != "server" || narrowed.Allows(domain.CapabilityCompletion) {
		t.Fatalf("a record kept its completion: %q, %v", key, err)
	}
	if _, _, err := chooseEntryType(ctx, types, domain.ItemWorkPackage, "bug", nil, profile, nil); err == nil {
		t.Fatal("a task's type was given to a work package")
	}
	if _, _, err := chooseEntryType(ctx, types, domain.ItemTask, "server", nil, profile, []string{"bug"}); err == nil {
		t.Fatal("a type the collection does not offer was taken")
	}
	if key, _, err := chooseEntryType(ctx, types, domain.ItemTask, "", nil, profile, []string{"bug"}); err != nil || key != "bug" {
		t.Fatalf("without a key the collection's first type was not taken: %q, %v", key, err)
	}
	if key, _, err := chooseEntryType(ctx, nil, domain.ItemTask, "bug", nil, profile, nil); err != nil || key != "" {
		t.Fatalf("a build without a registry did not fall back: %q, %v", key, err)
	}

	record := domain.WorkItem{Type: domain.ItemTask, EntryType: "server"}
	narrowed, err = narrowByEntryType(ctx, types, record, profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.EnsureCompletable(narrowed); !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Errorf("a record could be completed: %v", err)
	}
}

// UC-FLW-01 check 4: changing an entry's type keeps what the new type carries, and refuses - by
// field - what it would drop.
func TestChangingAnEntrysTypeNamesWhatItWouldDrop(t *testing.T) {
	ctx := context.Background()
	h := newItemHarness()
	types := workspaceTypes()
	open := domain.WorkItem{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000f1"), TenantID: tenantID,
		CollectionID: collectionID, Type: domain.ItemTask, Title: "Login fails", Version: 3,
	}
	done := open
	done.ID = shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	done.Completion = domain.Completion{IsCompleted: true, CompletedAt: &now, CompletedBy: accountID}
	h.items.stored[open.ID], h.items.stored[done.ID] = open, done

	handler := SetItemEntryType{
		Items: h.items, Containers: h.containers, Profiles: h.profiles, EntryTypes: types,
		Authorizer: h.authorizer, Changes: h.changes, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), HLC: &hlcSource{},
	}

	changed, err := handler.Execute(ctx, actor(), SetItemEntryTypeCommand{ItemID: open.ID, EntryType: "bug"})
	if err != nil {
		t.Fatal(err)
	}
	if changed.EntryType != "bug" || changed.Version != 4 || len(types.set) != 1 {
		t.Fatalf("the type did not land: %+v", changed)
	}
	if out := ItemOutput(changed); out["entry_type"] != "bug" {
		t.Errorf("the entry answers %v", out["entry_type"])
	}

	_, err = handler.Execute(ctx, actor(), SetItemEntryTypeCommand{ItemID: done.ID, EntryType: "server"})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "entry_types.values_dropped" ||
		len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/completion" {
		t.Fatalf("a completed entry became a record without a word: %v", err)
	}
	if len(types.set) != 1 {
		t.Error("the refused change was written")
	}
}

// UC-FLW-01 check 2: changing an entry's type holds it to the new type's form as creating one
// would - a required field it lacks refuses the change, by field, and nothing is written.
func TestChangingAnEntrysTypeMeetsTheNewForm(t *testing.T) {
	ctx := context.Background()
	h := newItemHarness()
	types := workspaceTypes()
	types.rows[0].Form = domain.Form{Fields: []domain.FormField{{Key: "title"}, {Key: "notes", Required: true}}}
	open := domain.WorkItem{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000f3"), TenantID: tenantID,
		CollectionID: collectionID, Type: domain.ItemTask, Title: "Login fails", Version: 1,
	}
	h.items.stored[open.ID] = open
	handler := SetItemEntryType{
		Items: h.items, Containers: h.containers, Profiles: h.profiles, EntryTypes: types,
		Authorizer: h.authorizer, Changes: h.changes, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), HLC: &hlcSource{},
		Forms: FormGate{Types: types, Compiler: &answers{of: map[string]bool{}}},
	}

	_, err := handler.Execute(ctx, actor(), SetItemEntryTypeCommand{ItemID: open.ID, EntryType: "bug"})
	refusal := formRefusal(t, err, "forms.refused")
	if len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/notes" {
		t.Fatalf("the refusal does not name the field: %+v", refusal.Fields)
	}
	if len(types.set) != 0 {
		t.Fatal("the refused change was written")
	}

	open.Notes = "Since the update"
	h.items.stored[open.ID] = open
	if _, err := handler.Execute(ctx, actor(), SetItemEntryTypeCommand{ItemID: open.ID, EntryType: "bug"}); err != nil {
		t.Fatalf("an entry that meets the form was refused: %v", err)
	}
}

// UC-FLW-01 check 4: asked again with the drop confirmed, the change clears the due date and the
// series the new type does not carry, in its own transaction; a completion is never dropped.
func TestAConfirmedTypeChangeDropsWhatItNamed(t *testing.T) {
	ctx := context.Background()
	h := newItemHarness()
	types := workspaceTypes()
	rules := newRecurrences()
	at := now.Add(48 * time.Hour)
	due, err := domain.NewDueDate(&at, false, "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	ruleID := shared.MustParseID("0192f000-0000-7000-8000-0000000000f9")
	scheduled := domain.WorkItem{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000f3"), TenantID: tenantID,
		CollectionID: collectionID, Type: domain.ItemTask, Title: "Patch the host", Version: 2,
		Due: due, RecurrenceRuleID: ruleID,
	}
	rules.stored[ruleID] = domain.RecurrenceRule{ID: ruleID, TenantID: tenantID, ItemID: scheduled.ID, Version: 1}
	done := scheduled
	done.ID, done.Due, done.RecurrenceRuleID = shared.MustParseID("0192f000-0000-7000-8000-0000000000f4"), nil, ""
	done.Completion = domain.Completion{IsCompleted: true, CompletedAt: &now, CompletedBy: accountID}
	h.items.stored[scheduled.ID], h.items.stored[done.ID] = scheduled, done

	handler := SetItemEntryType{
		Items: h.items, Containers: h.containers, Profiles: h.profiles, EntryTypes: types,
		Authorizer: h.authorizer, Changes: h.changes, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), HLC: &hlcSource{}, Recurrences: rules,
	}

	_, err = handler.Execute(ctx, actor(), SetItemEntryTypeCommand{ItemID: scheduled.ID, EntryType: "server"})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "entry_types.values_dropped" || len(refusal.Fields) != 2 {
		t.Fatalf("the drop was not named first: %v", err)
	}

	changed, err := handler.Execute(ctx, actor(), SetItemEntryTypeCommand{
		ItemID: scheduled.ID, EntryType: "server", DropValues: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed.EntryType != "server" || changed.Due != nil || !changed.RecurrenceRuleID.IsZero() {
		t.Fatalf("the confirmed change kept a value: %+v", changed)
	}
	if len(rules.deleted) != 1 || len(h.items.dueDates) != 1 || changed.Version != 4 {
		t.Errorf("the series or the due date was not cleared: %d, %d, version %d",
			len(rules.deleted), len(h.items.dueDates), changed.Version)
	}

	_, err = handler.Execute(ctx, actor(), SetItemEntryTypeCommand{
		ItemID: done.ID, EntryType: "server", DropValues: true,
	})
	if !errors.As(err, &refusal) || refusal.DetailCode != "entry_types.value_not_droppable" ||
		len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/completion" {
		t.Fatalf("a completion was dropped: %v", err)
	}
}
