// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

type hidings []repository.FieldHiding

func (h hidings) ReadRestricted(context.Context) ([]repository.FieldHiding, error) { return h, nil }

var hidingCollectionID = shared.MustParseID("0192f000-0000-7000-8000-0000000005c2")

func sightFor(roles roleOf, hidden ...repository.FieldHiding) FieldSight {
	return FieldSight{
		Hidings: hidings(hidden),
		Containers: &containers{stored: map[shared.ID]domain.Container{
			collectionID:      {ID: collectionID, TenantID: tenantID},
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
