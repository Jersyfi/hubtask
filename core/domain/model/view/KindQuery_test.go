// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestAKeyOfOneKindBecomesThatKindsConditions(t *testing.T) {
	location := "0192f000-0000-7000-8000-0000000000c3"
	cases := []struct {
		line string
		want KindFilter
	}{
		{"", KindFilter{}},
		{"passkey", KindFilter{}},
		{"who: me", KindFilter{Kind: ObjectEntry}},
		{"asset.class: server", KindFilter{Kind: ObjectAsset, Conditions: []KindCondition{{Field: FieldAssetClass, Values: []string{"server"}}}}},
		{"asset.stage: in_use, retired", KindFilter{Kind: ObjectAsset, Conditions: []KindCondition{{Field: FieldAssetStage, Values: []string{"IN_USE", "RETIRED"}}}}},
		{"-asset.owner: me", KindFilter{Kind: ObjectAsset, Conditions: []KindCondition{{Field: FieldAssetOwner, Values: []string{"@me"}, Negated: true}}}},
		{"asset.owner: me, none", KindFilter{Kind: ObjectAsset, Conditions: []KindCondition{{Field: FieldAssetOwner, Values: []string{"@me"}, None: true}}}},
		{"-asset.location: none", KindFilter{Kind: ObjectAsset, Conditions: []KindCondition{{Field: FieldAssetLocation, None: true, Negated: true}}}},
		{"asset.location: " + location + " asset.tag: INV-7", KindFilter{Kind: ObjectAsset, Conditions: []KindCondition{
			{Field: FieldAssetLocation, Values: []string{location}}, {Field: FieldAssetTag, Values: []string{"INV-7"}},
		}}},
		{"page.verified: true", KindFilter{Kind: ObjectPage, Conditions: []KindCondition{{Field: FieldPageVerified, Values: []string{"true"}}}}},
		{"-page.verified: true", KindFilter{Kind: ObjectPage, Conditions: []KindCondition{{Field: FieldPageVerified, Values: []string{"true"}, Negated: true}}}},
		{"handbook page.owner: me", KindFilter{Kind: ObjectPage, Conditions: []KindCondition{{Field: FieldPageOwner, Values: []string{"@me"}}}}},
		{"goal.state: at_risk, off_track", KindFilter{Kind: ObjectGoal, Conditions: []KindCondition{{Field: FieldGoalState, Values: []string{"AT_RISK", "OFF_TRACK"}}}}},
		{"asset.class = server", KindFilter{Kind: ObjectAsset, Conditions: []KindCondition{{Field: FieldAssetClass, Values: []string{"server"}}}}},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			q, err := ParseQuery(c.line, "/q")
			if err != nil {
				t.Fatal(err)
			}
			got, err := q.KindFilter("/q")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestAKindFilterRefusesWhatOneKindsSearchCannotAnswer(t *testing.T) {
	cases := []struct{ line, code string }{
		{"asset.class: server who: me", "query.kinds_mixed"},
		{"asset.class: server page.owner: me", "query.kinds_mixed"},
		{"asset.class: server (vpn OR mail)", "query.kinds_mixed"},
		{"(asset.class: server OR asset.stage: in_use)", "query.kind_filter_grouped"},
		{"asset.class: server -asset.class: laptop", "query.key_twice"},
		{"asset.class: serv*", "query.operator_unsupported"},
		{"page.verified: none", "query.operator_unsupported"},
		{"asset.owner: someone", "shared.id_malformed"},
		{"asset.owner: my-groups", "query.value_unknown"},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			q, err := ParseQuery(c.line, "/q")
			if err == nil {
				_, err = q.KindFilter("/q")
			}
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != c.code {
				t.Fatalf("want %s, got %v", c.code, err)
			}
		})
	}
}

func TestAKeyOfOneKindIsRefusedByAQueryOfEntries(t *testing.T) {
	q, err := ParseQuery("is: open asset.class: server", "/q")
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.EntryFilter("/q")
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "query.kind_not_here" || domainErr.Params["kind"] != ObjectAsset {
		t.Fatalf("want query.kind_not_here for asset, got %v", err)
	}
	if _, err := ParseFilter(map[string]any{"field": FieldAssetClass, "op": "EQ", "value": "server"}, "/filter"); err == nil {
		t.Fatal("a filter document of entries took a key of inventory items")
	}
}

func TestAKeyOfOneKindIsWrittenBackAsTyped(t *testing.T) {
	for _, line := range []string{"asset.class: server", "-asset.owner: me", "asset.owner: me, none", "page.verified: true", "goal.state: at_risk", "asset.stage: in_use, retired"} {
		q, err := ParseQuery(line, "/q")
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := FormatQuery(q); !ok || got != line {
			t.Fatalf("%q wrote back as %q (%v)", line, got, ok)
		}
	}
}

func TestEveryKeyOfAKindIsInTheCatalogueWithAnExampleThatReads(t *testing.T) {
	served := map[string]QueryKey{}
	for _, key := range QueryKeys() {
		served[key.Key] = key
	}
	for _, f := range KindFields() {
		key, ok := served[f.Name]
		if !ok || key.Field != f.Name || len(key.Examples) == 0 || len(key.Objects) != 1 {
			t.Fatalf("%s is not served as a key of one kind with examples: %+v", f.Name, key)
		}
		for _, example := range key.Examples {
			q, err := ParseQuery(example, "/q")
			if err != nil {
				t.Fatalf("%s: %v", example, err)
			}
			if got, err := q.KindFilter("/q"); err != nil || got.Kind != key.Objects[0] {
				t.Fatalf("%s reads as %+v, %v", example, got, err)
			}
		}
	}
}
