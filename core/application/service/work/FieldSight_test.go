// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	activityrepo "github.com/Jersyfi/hubtask/core/application/repository/activity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

type hidings []repository.FieldHiding

func (h hidings) ReadRestricted(context.Context) ([]repository.FieldHiding, error) { return h, nil }

func (h hidings) ReadFormulas(context.Context) ([]repository.FieldFormula, error) { return nil, nil }

// hidingsAndFormulas is a workspace that also defines formulas beside its hidden fields.
type hidingsAndFormulas struct {
	hidden   []repository.FieldHiding
	formulas []repository.FieldFormula
}

func (h hidingsAndFormulas) ReadRestricted(context.Context) ([]repository.FieldHiding, error) {
	return h.hidden, nil
}

func (h hidingsAndFormulas) ReadFormulas(context.Context) ([]repository.FieldFormula, error) {
	return h.formulas, nil
}

// naming is a compiler whose formulas are the input keys they read, separated by spaces: one names
// a key its environment does not declare and is refused, as the adapter refuses it.
type naming struct{}

func (naming) Compile(formula string, env expression.Environment, _ expression.Result) (expression.Program, error) {
	declared := map[string]bool{}
	for _, field := range env.Variables[0].Fields[0].Fields {
		declared[field.Name] = true
	}
	for _, key := range strings.Fields(formula) {
		if !declared[key] {
			return nil, errors.New("undeclared " + key)
		}
	}
	return nil, nil
}

var hidingCollectionID = shared.MustParseID("0192f000-0000-7000-8000-0000000005c2")

func sightFor(roles roleOf, hidden ...repository.FieldHiding) FieldSight {
	return FieldSight{
		Hidings: hidings(hidden),
		Containers: &containers{stored: map[shared.ID]domain.Container{
			collectionID:       {ID: collectionID, TenantID: tenantID},
			hidingCollectionID: {ID: hidingCollectionID, TenantID: tenantID},
		}},
		Roles: roles,
	}
}

// UC-ID-27 check 3: a field restricted for reading is absent from the entry for whoever holds
// neither its read nor its write permission; the shipped owner and administrators, and a role
// naming either, see it; a field hidden in another collection hides nothing here.
func TestAFieldHiddenFromReadersIsTakenOutOfTheirEntries(t *testing.T) {
	ctx := context.Background()
	stored := map[string]any{"cost": 1200.0, "severity": "high", "band": "B"}
	item := domain.WorkItem{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000005c3"), CollectionID: collectionID, CustomFields: stored}
	hidden := []repository.FieldHiding{{Key: "cost"}, {Key: "band", CollectionID: hidingCollectionID}}

	cases := []struct {
		name  string
		roles roleOf
		sees  bool
	}{
		{"a member", roleOf{role: identity.RoleMember}, false},
		{"a viewer", roleOf{role: identity.RoleViewer}, false},
		{"its read permission", roleOf{custom: []string{"finance"}, permissions: []service.Permission{service.FieldRead("cost")}}, true},
		{"its write permission", roleOf{custom: []string{"finance"}, permissions: []service.Permission{service.FieldWrite("cost")}}, true},
		{"another field's", roleOf{role: identity.RoleMember, permissions: []service.Permission{service.FieldRead("band")}}, false},
		{"an administrator", roleOf{role: identity.RoleAdmin}, true},
		{"the owner", roleOf{role: identity.RoleOwner}, true},
	}
	for _, c := range cases {
		lens, err := sightFor(c.roles, hidden...).Lens(ctx, &unitOfWork{}, itemActor())
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		narrowed, err := lens.Item(ctx, &unitOfWork{}, item)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		_, sees := narrowed.CustomFields["cost"]
		if sees != c.sees {
			t.Errorf("%s: sees cost = %v, want %v", c.name, sees, c.sees)
		}
		if narrowed.CustomFields["severity"] != "high" || narrowed.CustomFields["band"] != "B" {
			t.Errorf("%s: an open field went missing: %v", c.name, narrowed.CustomFields)
		}
		if ItemOutput(narrowed)["custom_fields"].(map[string]any)["cost"] != nil && !c.sees {
			t.Errorf("%s: the projection carries the hidden value", c.name)
		}
	}
	if _, kept := stored["cost"]; !kept {
		t.Fatal("narrowing changed the stored values")
	}
}

// UC-ID-27 check 3: the change stream and the initial walk carry no hidden value either - not in
// an entry's snapshot, not as a change of the one key.
func TestAChangeRecordLosesTheHiddenKeysInEitherShape(t *testing.T) {
	snapshot := map[string]any{"title": "a", "custom_fields": map[string]any{"cost": 1.0, "severity": "low"}}
	narrowed := PayloadWithout(snapshot, []string{"cost"})
	values := narrowed["custom_fields"].(map[string]any)
	if _, held := values["cost"]; held || values["severity"] != "low" || narrowed["title"] != "a" {
		t.Fatalf("the snapshot narrowed to %v", narrowed)
	}
	if _, kept := snapshot["custom_fields"].(map[string]any)["cost"]; !kept {
		t.Fatal("narrowing changed the record it was handed")
	}

	change := map[string]any{"custom_fields.cost": 2.0, "custom_fields.severity": "high"}
	narrowed = PayloadWithout(change, []string{"cost"})
	if _, held := narrowed["custom_fields.cost"]; held || narrowed["custom_fields.severity"] != "high" {
		t.Fatalf("the change narrowed to %v", narrowed)
	}
	if got := PayloadWithout(change, nil); len(got) != 2 {
		t.Fatalf("nothing hidden narrowed %v", got)
	}
}

// UC-ID-27 check 3: a query or a search that filters, sorts or groups by a hidden field is refused
// for whoever may not see it - an order or a range over a value tells the value - and answered for
// whoever may.
func TestAQueryOverAHiddenFieldIsRefusedToWhoeverMayNotSeeIt(t *testing.T) {
	ctx := context.Background()
	filter, err := view.ParseFilter(map[string]any{"field": "custom_fields.cost", "op": "GT", "value": 1000}, "/filter")
	if err != nil {
		t.Fatalf("parsing the filter: %v", err)
	}
	spec := view.Spec{Filter: filter}
	collection := domain.Container{ID: collectionID, TenantID: tenantID}

	member := QueryItems{Gate: EntryGate{Sight: sightFor(roleOf{role: identity.RoleMember}, repository.FieldHiding{Key: "cost"})},
		UnitOfWork: &unitOfWork{}}
	_, err = member.lens(ctx, itemActor(), collection, spec)
	refusal := formRefusal(t, err, FieldReadRestrictedCode)
	if refusal.Params["key"] != "cost" {
		t.Fatalf("the refusal names %v", refusal.Params)
	}
	if _, err := member.lens(ctx, itemActor(), collection, view.Spec{}); err != nil {
		t.Fatalf("a query over open fields was refused: %v", err)
	}

	finance := QueryItems{Gate: EntryGate{Sight: sightFor(roleOf{custom: []string{"finance"},
		permissions: []service.Permission{service.FieldRead("cost")}}, repository.FieldHiding{Key: "cost"})},
		UnitOfWork: &unitOfWork{}}
	if _, err := finance.lens(ctx, itemActor(), collection, spec); err != nil {
		t.Fatalf("the field's reader was refused: %v", err)
	}

	open := QueryItems{Gate: EntryGate{Sight: sightFor(roleOf{role: identity.RoleMember})}, UnitOfWork: &unitOfWork{}}
	if _, err := open.lens(ctx, itemActor(), collection, spec); err != nil {
		t.Fatalf("a workspace hiding nothing refused: %v", err)
	}
}

// UC-ID-27 check 3: the entry's history names a hidden field's step without what it held.
func TestTheHistoryLeavesOutAHiddenFieldsValues(t *testing.T) {
	ctx := context.Background()
	page := activityrepo.EntryPage{Entries: []activity.Entry{{ChangeSet: map[string]any{
		"custom_fields.cost": map[string]any{"from": "10", "to": "12"}, "custom_fields.severity": map[string]any{"to": "high"},
	}}}}
	collection := domain.Container{ID: collectionID, TenantID: tenantID}

	member := ListActivity{Sight: sightFor(roleOf{role: identity.RoleMember}, repository.FieldHiding{Key: "cost"}), UnitOfWork: &unitOfWork{}}
	narrowed, err := member.narrowed(ctx, itemActor(), collection, page)
	if err != nil {
		t.Fatal(err)
	}
	if _, held := narrowed.Entries[0].ChangeSet["custom_fields.cost"]; held {
		t.Fatalf("the member reads the hidden step: %v", narrowed.Entries[0].ChangeSet)
	}
	if _, held := narrowed.Entries[0].ChangeSet["custom_fields.severity"]; !held {
		t.Fatal("an open field's step went missing")
	}

	admin := ListActivity{Sight: sightFor(roleOf{role: identity.RoleAdmin}, repository.FieldHiding{Key: "cost"}), UnitOfWork: &unitOfWork{}}
	if narrowed, _ = admin.narrowed(ctx, itemActor(), collection, page); narrowed.Entries[0].ChangeSet["custom_fields.cost"] == nil {
		t.Fatal("an administrator lost the hidden step")
	}
}

// UC-ID-27 check 3: a calculated field whose formula reads a hidden field is hidden with it from
// whoever may not see that field - on the entry and in a query over it - while one reading only
// open fields stays, and the field's reader sees both.
func TestACalculatedFieldReadingAHiddenOneIsHiddenWithIt(t *testing.T) {
	ctx := context.Background()
	workspace := hidingsAndFormulas{
		hidden: []repository.FieldHiding{{Key: "rate"}},
		formulas: []repository.FieldFormula{
			{Key: "hours", Kind: domain.CustomFieldNumber},
			{Key: "rate", Kind: domain.CustomFieldMoney},
			{Key: "cost", Kind: domain.CustomFieldCalculated, Formula: "hours rate"},
			{Key: "double", Kind: domain.CustomFieldCalculated, Formula: "hours"},
			{Key: "elsewhere", Kind: domain.CustomFieldCalculated, Formula: "rate", CollectionID: hidingCollectionID},
		},
	}
	sight := func(roles roleOf, compiler expression.Compiler) FieldSight {
		s := sightFor(roles)
		s.Hidings, s.Compiler = workspace, compiler
		return s
	}
	item := domain.WorkItem{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000005c4"), CollectionID: collectionID,
		CustomFields: map[string]any{"hours": 3.0, "rate": 80.0, "cost": 240.0, "double": 6.0}}

	cases := []struct {
		name     string
		roles    roleOf
		compiler expression.Compiler
		sees     []string
		misses   []string
	}{
		{"a member", roleOf{role: identity.RoleMember}, naming{}, []string{"hours", "double"}, []string{"rate", "cost"}},
		{"the field's reader", roleOf{custom: []string{"finance"}, permissions: []service.Permission{service.FieldRead("rate")}},
			naming{}, []string{"hours", "rate", "cost", "double"}, nil},
		{"a member, no compiler", roleOf{role: identity.RoleMember}, nil, []string{"hours"}, []string{"rate", "cost", "double"}},
	}
	for _, c := range cases {
		lens, err := sight(c.roles, c.compiler).Lens(ctx, &unitOfWork{}, itemActor())
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		narrowed, err := lens.Item(ctx, &unitOfWork{}, item)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, key := range c.sees {
			if _, held := narrowed.CustomFields[key]; !held {
				t.Errorf("%s: %s went missing", c.name, key)
			}
		}
		for _, key := range c.misses {
			if _, held := narrowed.CustomFields[key]; held {
				t.Errorf("%s: sees %s", c.name, key)
			}
		}
	}

	lens, err := sight(roleOf{role: identity.RoleMember}, naming{}).Lens(ctx, &unitOfWork{}, itemActor())
	if err != nil {
		t.Fatal(err)
	}
	path := containerPath(domain.Container{ID: collectionID, TenantID: tenantID})
	hidden, err := lens.HiddenFrom(ctx, path, []string{"double"})
	if err != nil || len(hidden) != 0 {
		t.Fatalf("a query over a calculation of open fields: %v, %v, want nothing hidden", hidden, err)
	}
	hidden, err = lens.HiddenFrom(ctx, path, []string{"cost", "double"})
	if err != nil || !slices.Equal(hidden, []string{"cost"}) {
		t.Fatalf("a query over the calculation alone: %v, %v, want [cost]", hidden, err)
	}
	hidden, err = lens.HiddenFrom(ctx, path, []string{"rate", "cost", "double"})
	if err != nil || !slices.Equal(hidden, []string{"rate", "cost"}) {
		t.Fatalf("a query over the hidden field and its calculation: %v, %v, want [rate cost]", hidden, err)
	}
}
