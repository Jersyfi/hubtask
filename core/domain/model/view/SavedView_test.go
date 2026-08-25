// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	viewTenant     = shared.MustParseID("0192f000-0000-7000-8000-0000000000a0")
	viewOwner      = shared.MustParseID("0192f000-0000-7000-8000-0000000000d0")
	viewCollection = shared.MustParseID("0192f000-0000-7000-8000-0000000000c0")
	viewID         = shared.MustParseID("0192f000-0000-7000-8000-000000000071")
)

var savedAt = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

func viewInput() NewSavedViewInput {
	return NewSavedViewInput{
		ID: viewID, TenantID: viewTenant, OwnerID: viewOwner,
		ScopeType: ViewScopeCollection, ScopeID: viewCollection,
		Name: "Due this week", Layout: "KANBAN",
		Query: map[string]any{
			"filter": map[string]any{"field": "due_at", "op": "LTE", "value": "@today+P7D"},
		},
		Sharing: SharingPrivate,
		Now:     savedAt,
	}
}

func TestASavedViewIsBuiltAndBounded(t *testing.T) {
	built, err := NewSavedView(viewInput())
	if err != nil {
		t.Fatalf("the view was refused: %v", err)
	}
	if built.Name != "Due this week" || built.Layout != LayoutKanban || built.Version != 1 {
		t.Errorf("the view came out as %+v", built)
	}
	if built.Grouping == nil || built.VisibleFields == nil {
		t.Errorf("the empty hints are nil rather than empty: %+v", built)
	}

	for name, test := range map[string]struct {
		change   func(*NewSavedViewInput)
		wantCode string
		wantPath string
	}{
		"an empty name": {
			change:   func(in *NewSavedViewInput) { in.Name = "  " },
			wantCode: "views.name_empty", wantPath: "/name",
		},
		"a name over the bound": {
			change:   func(in *NewSavedViewInput) { in.Name = strings.Repeat("x", 201) },
			wantCode: "views.name_too_long", wantPath: "/name",
		},
		"a name on two lines": {
			change:   func(in *NewSavedViewInput) { in.Name = "one\ntwo" },
			wantCode: "views.name_malformed", wantPath: "/name",
		},
		"a layout outside the declared set": {
			change:   func(in *NewSavedViewInput) { in.Layout = "GANTT" },
			wantCode: "views.layout_unknown", wantPath: "/layout",
		},
		"a workspace view naming a container": {
			change: func(in *NewSavedViewInput) {
				in.ScopeType, in.ScopeID = ViewScopeTenant, viewCollection
			},
			wantCode: "views.scope_id_not_allowed", wantPath: "/scope_id",
		},
		"a container scope naming none": {
			change:   func(in *NewSavedViewInput) { in.ScopeID = shared.ID("") },
			wantCode: "views.scope_id_required", wantPath: "/scope_id",
		},
		"a scope that is not one": {
			change:   func(in *NewSavedViewInput) { in.ScopeType = "PLANET" },
			wantCode: "views.scope_unknown", wantPath: "/scope_type",
		},
		"no query at all": {
			change:   func(in *NewSavedViewInput) { in.Query = nil },
			wantCode: "views.query_required", wantPath: "/query",
		},
		"too many visible fields": {
			change: func(in *NewSavedViewInput) {
				in.VisibleFields = make([]string, MaxVisibleFields+1)
				for i := range in.VisibleFields {
					in.VisibleFields[i] = "title"
				}
			},
			wantCode: "views.visible_fields_too_many", wantPath: "/visible_fields",
		},
		"a visible field with a control character": {
			change:   func(in *NewSavedViewInput) { in.VisibleFields = []string{"tit\tle"} },
			wantCode: "views.visible_field_malformed", wantPath: "/visible_fields",
		},
	} {
		t.Run(name, func(t *testing.T) {
			in := viewInput()
			test.change(&in)

			_, err := NewSavedView(in)
			refusal := shared.AsError(err)
			if refusal == nil || refusal.DetailCode != test.wantCode {
				t.Fatalf("answered %v, want %s", err, test.wantCode)
			}
			if len(refusal.Fields) != 1 || refusal.Fields[0].Path != test.wantPath {
				t.Errorf("the refusal does not point at %s: %v", test.wantPath, refusal.Fields)
			}
		})
	}
}

// The acceptance sentence: the stored query passes the grammar an ad-hoc one passes, with the
// same codes - refused at write, where a correction can be made, never at read.
func TestTheStoredQueryPassesTheGrammar(t *testing.T) {
	tooDeep := map[string]any{"field": "title", "op": "EQ", "value": "x"}
	for range MaxFilterDepth {
		tooDeep = map[string]any{"op": "AND", "nodes": []any{tooDeep}}
	}

	for name, test := range map[string]struct {
		query    map[string]any
		wantCode string
	}{
		"a filter naming a field nothing serves": {
			query: map[string]any{
				"filter": map[string]any{"field": "recurrence_rule_id", "op": "EQ", "value": "x"},
			},
			wantCode: "query.field_unknown",
		},
		"an over-deep tree": {
			query:    map[string]any{"filter": tooDeep},
			wantCode: "query.filter_too_deep",
		},
		"a sort by a field that cannot order": {
			query:    map[string]any{"sort": []any{map[string]any{"field": "labels"}}},
			wantCode: "query.field_not_sortable",
		},
		"a grouping by a field that cannot group": {
			query:    map[string]any{"group_by": map[string]any{"field": "title"}},
			wantCode: "query.field_not_groupable",
		},
		"a scope naming both anchors": {
			query: map[string]any{"scope": map[string]any{
				"container_id": viewCollection.String(), "item_id": viewID.String(),
			}},
			wantCode: "query.scope_ambiguous",
		},
		"a scope whose identifier is not one": {
			query:    map[string]any{"scope": map[string]any{"container_id": "not-a-uuid"}},
			wantCode: "query.value_type_invalid",
		},
	} {
		t.Run(name, func(t *testing.T) {
			in := viewInput()
			in.Query = test.query

			_, err := NewSavedView(in)
			refusal := shared.AsError(err)
			if refusal == nil || refusal.DetailCode != test.wantCode {
				t.Fatalf("answered %v, want %s", err, test.wantCode)
			}
		})
	}

	// Placeholders are stored as placeholders: `@me` in a saved view is the reader at read time,
	// which is what makes one view mean the right thing to each of its readers.
	stored, err := NewSavedView(viewInput())
	if err != nil {
		t.Fatalf("the view was refused: %v", err)
	}
	filter, _ := stored.Query["filter"].(map[string]any)
	if filter["value"] != "@today+P7D" {
		t.Errorf("the placeholder was rewritten to %v", filter["value"])
	}
}

func TestUpdatedMovesOnlyWhatWasSent(t *testing.T) {
	built, err := NewSavedView(viewInput())
	if err != nil {
		t.Fatalf("the fixture was refused: %v", err)
	}

	renamed, changed, err := built.Updated(ViewAttributes{Name: viewText("Overdue")})
	if err != nil || !changed {
		t.Fatalf("the rename answered %v, changed=%v", err, changed)
	}
	if renamed.Name != "Overdue" || renamed.Layout != built.Layout {
		t.Errorf("the rename produced %+v", renamed)
	}

	same, changed, err := built.Updated(ViewAttributes{Name: viewText("Due this week")})
	if err != nil || changed {
		t.Fatalf("an echo answered %v, changed=%v", err, changed)
	}
	if same.Name != built.Name {
		t.Errorf("the echo moved the name to %q", same.Name)
	}

	if _, _, err := built.Updated(ViewAttributes{Layout: viewText("GANTT")}); shared.AsError(err) == nil {
		t.Error("an unknown layout survived the update")
	}
	if _, _, err := built.Updated(ViewAttributes{
		Query: map[string]any{"filter": map[string]any{"field": "nope", "op": "EQ", "value": 1}},
	}); shared.AsError(err) == nil {
		t.Error("a broken query survived the update")
	}

	if _, changed, err := built.Updated(ViewAttributes{}); err != nil || changed {
		t.Errorf("an empty update answered %v, changed=%v", err, changed)
	}
}

func TestSharingIsDecidedNotStored(t *testing.T) {
	built, err := NewSavedView(viewInput())
	if err != nil {
		t.Fatalf("the fixture was refused: %v", err)
	}

	sharing, err := NewSharing("SCOPE")
	if err != nil {
		t.Fatalf("SCOPE was refused: %v", err)
	}
	published, moved, err := built.Shared(sharing)
	if err != nil || !moved || published.Sharing != SharingScope {
		t.Fatalf("sharing answered %v, moved=%v, %+v", err, moved, published.Sharing)
	}
	if _, moved, err := published.Shared(SharingScope); err != nil || moved {
		t.Errorf("sharing what is shared answered %v, moved=%v", err, moved)
	}

	if _, err := NewSharing("PUBLIC_LINK"); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "views.public_link_not_available" {
		t.Errorf("PUBLIC_LINK answered %v", err)
	}
	if _, err := NewSharing("FRIENDS"); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "views.sharing_unknown" {
		t.Errorf("an unknown sharing answered %v", err)
	}

	personal := viewInput()
	personal.ScopeType, personal.ScopeID = ViewScopeAccount, viewOwner
	own, err := NewSavedView(personal)
	if err != nil {
		t.Fatalf("the personal view was refused: %v", err)
	}
	_, _, err = own.Shared(SharingScope)
	if refusal := shared.AsError(err); refusal == nil ||
		refusal.DetailCode != "views.account_scope_not_shareable" {
		t.Errorf("sharing a personal view answered %v", err)
	}
}

func TestTheDeclaredLayoutsAreTheFour(t *testing.T) {
	if len(Layouts()) != 4 {
		t.Fatalf("%d layouts, want the four the milestone declares", len(Layouts()))
	}
	for _, layout := range Layouts() {
		if !layout.Valid() {
			t.Errorf("%s is declared and reports itself invalid", layout)
		}
	}
	if Layout("GANTT").Valid() {
		t.Error("an undeclared layout reports itself valid")
	}
}

func viewText(value string) *string { return &value }
