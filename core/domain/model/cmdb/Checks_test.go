// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestAValueIsReadIntoItsAttributesKind(t *testing.T) {
	account := id("01")
	cases := []struct {
		name string
		a    Attribute
		raw  any
		want any
		ok   bool
	}{
		{"nothing", Attribute{Kind: AttrNumber}, nil, nil, true},
		{"blank text is nothing", Attribute{Kind: AttrNumber}, "  ", nil, true},
		{"text trimmed", Attribute{Kind: AttrText}, " rack 4 ", "rack 4", true},
		{"text over lines", Attribute{Kind: AttrText}, "a\nb", "a\nb", true},
		{"a number over lines", Attribute{Kind: AttrNumber}, "1\n2", nil, false},
		{"text too long", Attribute{Kind: AttrText}, strings.Repeat("x", 2001), nil, false},
		{"text that is not text", Attribute{Kind: AttrText}, 4, nil, false},
		{"a float", Attribute{Kind: AttrNumber}, 2.5, 2.5, true},
		{"an int", Attribute{Kind: AttrNumber}, 3, 3.0, true},
		{"a number as text", Attribute{Kind: AttrNumber}, "16.5", 16.5, true},
		{"not a number", Attribute{Kind: AttrNumber}, math.NaN(), nil, false},
		{"an infinite number", Attribute{Kind: AttrNumber}, math.Inf(1), nil, false},
		{"text that is no number", Attribute{Kind: AttrNumber}, "many", nil, false},
		{"a flag", Attribute{Kind: AttrBoolean}, true, true, true},
		{"a flag as text", Attribute{Kind: AttrBoolean}, "TRUE", true, true},
		{"text that is no flag", Attribute{Kind: AttrBoolean}, "perhaps", nil, false},
		{"a day", Attribute{Kind: AttrDate}, "2026-10-29", "2026-10-29", true},
		{"a day that is none", Attribute{Kind: AttrDate}, "2026-02-30", nil, false},
		{"a time is no day", Attribute{Kind: AttrDate}, "2026-10-29T10:00", nil, false},
		{"a choice offered", Attribute{Kind: AttrChoice, Options: []string{"GOLD"}}, "GOLD", "GOLD", true},
		{"a choice not offered", Attribute{Kind: AttrChoice, Options: []string{"GOLD"}}, "TIN", nil, false},
		{"a link", Attribute{Kind: AttrURL}, "https://wiki.example/rack", "https://wiki.example/rack", true},
		{"a link without a host", Attribute{Kind: AttrURL}, "https://", nil, false},
		{"a link of another scheme", Attribute{Kind: AttrURL}, "ftp://files.example", nil, false},
		{"an account", Attribute{Kind: AttrAccount}, account.String(), account.String(), true},
		{"a group that is no id", Attribute{Kind: AttrGroup}, "admins", nil, false},
	}
	for _, c := range cases {
		c.a.Key = "k"
		got, err := NormaliseValue(c.a, c.raw)
		if (err == nil) != c.ok {
			t.Errorf("%s: error %v", c.name, err)
			continue
		}
		if !c.ok {
			if code(err) != "cmdb.attribute_value_invalid" {
				t.Errorf("%s: code %q", c.name, code(err))
			}
			continue
		}
		if got != c.want {
			t.Errorf("%s: %#v, want %#v", c.name, got, c.want)
		}
	}
}

func TestALifecycleAndAComplianceAreCheckedByName(t *testing.T) {
	if err := CheckLifecycle(StageInUse, StatusOK); err != nil {
		t.Errorf("a known stage and status: %v", err)
	}
	if got := code(CheckLifecycle("LOST", StatusOK)); got != "cmdb.stage_unknown" {
		t.Errorf("an unknown stage: %q", got)
	}
	if got := code(CheckLifecycle(StageInUse, "LOST")); got != "cmdb.status_unknown" {
		t.Errorf("an unknown status: %q", got)
	}
	fixed := OwnProtection{Protection: Protection{C: LevelHigh}, Fixed: []string{"c"}, Reason: "Personal data"}
	cases := []struct {
		name string
		c    Compliance
		code string
	}{
		{"one object, nothing fixed", Compliance{Quantity: 1}, ""},
		{"a fixed dimension with its reason", Compliance{Quantity: 3, Protection: fixed}, ""},
		{"an unknown level", Compliance{Quantity: 1, Protection: OwnProtection{Protection: Protection{I: "EXTREME"}}}, "cmdb.protection_level_unknown"},
		{"an unknown dimension", Compliance{Quantity: 1, Protection: OwnProtection{Fixed: []string{"x"}, Reason: "r"}}, "cmdb.protection_dimension_unknown"},
		{"fixed without a reason", Compliance{Quantity: 1, Protection: OwnProtection{Fixed: []string{"a"}, Reason: " "}}, "cmdb.protection_reason_required"},
		{"a reason too long", Compliance{Quantity: 1, Protection: OwnProtection{Reason: strings.Repeat("r", 2001)}}, "cmdb.protection_reason_too_long"},
		{"no object", Compliance{Quantity: 0}, "cmdb.quantity_invalid"},
		{"too many objects", Compliance{Quantity: 1000001}, "cmdb.quantity_invalid"},
	}
	for _, c := range cases {
		if got := code(CheckCompliance(c.c)); got != c.code {
			t.Errorf("%s: %q, want %q", c.name, got, c.code)
		}
	}
	for name, value := range map[string]string{"empty": "", "two lines": "a\nb", "too long": strings.Repeat("n", MaxNameLength+1)} {
		if code(CheckName(value)) != "cmdb.ci_name_invalid" {
			t.Errorf("a name %s is taken", name)
		}
	}
}

func TestAnAssetsStateLeadsTheStageAndItsWarrantyEnds(t *testing.T) {
	for state, stage := range map[AssetState]Stage{
		AssetRequested: StagePlanned, AssetOrdered: StageOrdered, AssetInStock: StageInStock,
		AssetDeployed: StageInUse, AssetRetired: StageRetired, AssetDisposed: StageDisposed,
	} {
		if got := state.Stage(); got != stage {
			t.Errorf("%s: stage %s, want %s", state, got, stage)
		}
	}
	until := now
	asset := Asset{WarrantyUntil: &until}
	if asset.WarrantyEnded(now) || !asset.WarrantyEnded(now.Add(time.Hour)) || (Asset{}).WarrantyEnded(now) {
		t.Error("the warranty ends on the wrong day")
	}
}

func TestASourceIsCheckedAndWritesOnlyItsClasses(t *testing.T) {
	classes := NewClasses(nil)
	system := SystemSources()
	if len(system) != 1 || system[0].Key != ManualSource || !system[0].System || !system[0].MayCreate {
		t.Fatalf("the shipped sources: %+v", system)
	}
	mapping := &FileMapping{Class: "server", NameColumn: "Host"}
	in := SourceInput{ID: id("10"), Key: "inventory", Kind: SourceFile, Rank: 20, Classes: []string{"computer"}, Mapping: mapping, Now: now}
	cases := []struct {
		name   string
		change func(*SourceInput)
		code   string
	}{
		{"valid", func(*SourceInput) {}, ""},
		{"a malformed key", func(in *SourceInput) { in.Key = "In Ventory" }, "cmdb.source_key_malformed"},
		{"the people's key", func(in *SourceInput) { in.Key = ManualSource }, "cmdb.source_key_taken"},
		{"people as a kind", func(in *SourceInput) { in.Kind = SourceManual }, "cmdb.source_kind_unknown"},
		{"no rank", func(in *SourceInput) { in.Rank = 0 }, "cmdb.source_rank_invalid"},
		{"a rank too low", func(in *SourceInput) { in.Rank = 1001 }, "cmdb.source_rank_invalid"},
		{"an unknown class", func(in *SourceInput) { in.Classes = []string{"spaceship"} }, "cmdb.class_unknown"},
		{"a mapping to an abstract class", func(in *SourceInput) { in.Mapping = &FileMapping{Class: "hardware", NameColumn: "Host"} }, "cmdb.class_not_instantiable"},
		{"a mapping without a name", func(in *SourceInput) { in.Mapping = &FileMapping{Class: "server", NameColumn: " "} }, "cmdb.mapping_name_column_required"},
	}
	for _, c := range cases {
		next := in
		c.change(&next)
		if _, err := NewSource(next, nil, classes); code(err) != c.code {
			t.Errorf("%s: %q, want %q", c.name, code(err), c.code)
		}
	}
	created, err := NewSource(in, nil, classes)
	if err != nil || created.Version != 1 || !created.CreatedAt.Equal(now) {
		t.Fatalf("created %+v %v", created, err)
	}
	changed, err := NewSource(SourceInput{Key: "inventory", Kind: SourceAPI, Rank: 30, Now: now.Add(time.Hour)}, &created, classes)
	if err != nil || changed.Version != 2 || changed.ID != created.ID || changed.Rank != 30 || !changed.CreatedAt.Equal(now) {
		t.Fatalf("changed %+v %v", changed, err)
	}
	if !created.Writes("server", classes) || created.Writes("application_service", classes) || !changed.Writes("application_service", classes) {
		t.Error("a source writes outside its classes, or one without classes writes none")
	}
}

func TestAMergeKeepsTheSurvivorsValuesAndRetiresTheDuplicate(t *testing.T) {
	early, late := now.Add(-time.Hour), now
	survivor := CI{ID: id("20"), Attributes: map[string]any{"hostname": "web-1"}, LastSeen: &early}
	duplicate := CI{ID: id("21"), Stage: StageInUse, Status: StatusOK, LastSeen: &late,
		Attributes: map[string]any{"hostname": "web-one", "serial_number": "SN-7"},
		Provenance: map[string]Provenance{"serial_number": {}}}
	kept, merged := Merge(survivor, duplicate, now, id("99"))
	if kept.Attributes["hostname"] != "web-1" || kept.Attributes["serial_number"] != "SN-7" {
		t.Errorf("the survivor's values: %v", kept.Attributes)
	}
	if _, ok := kept.Provenance["serial_number"]; !ok || !kept.LastSeen.Equal(late) || kept.UpdatedBy != id("99") {
		t.Errorf("the survivor: %+v", kept)
	}
	if survivor.Attributes["serial_number"] != nil {
		t.Error("the merge wrote into the survivor it was given")
	}
	if merged.Stage != StageRetired || merged.Status != StatusMerged || merged.MergedInto != survivor.ID {
		t.Errorf("the duplicate: %+v", merged)
	}
	empty, _ := Merge(CI{ID: id("22")}, CI{Attributes: map[string]any{"a": 1}}, now, id("99"))
	if empty.Attributes["a"] != 1 || empty.Provenance == nil {
		t.Errorf("a survivor without values: %+v", empty)
	}
}

func TestAWorkspacesRelationTypeIsCheckedAndListedAfterTheShippedOnes(t *testing.T) {
	classes := NewClasses(nil)
	types := NewRelationTypes(nil)
	in := RelationTypeInput{ID: id("30"), Key: "backs_up", Name: "Backs up", ReverseName: "Backed up by", Impact: ImpactTarget,
		Pairs: []ClassPair{{From: "server", To: "database_instance"}}, Now: now}
	cases := []struct {
		name   string
		change func(*RelationTypeInput)
		code   string
	}{
		{"valid", func(*RelationTypeInput) {}, ""},
		{"a malformed key", func(in *RelationTypeInput) { in.Key = "Backs Up" }, "cmdb.relation_type_key_malformed"},
		{"a shipped key", func(in *RelationTypeInput) { in.Key = RelDependsOn }, "cmdb.relation_type_key_taken"},
		{"no name", func(in *RelationTypeInput) { in.Name = " " }, "cmdb.relation_type_name_invalid"},
		{"a reverse name over lines", func(in *RelationTypeInput) { in.ReverseName = "a\nb" }, "cmdb.relation_type_name_invalid"},
		{"an unknown impact", func(in *RelationTypeInput) { in.Impact = "SIDEWAYS" }, "cmdb.impact_unknown"},
		{"an unknown source class", func(in *RelationTypeInput) { in.Pairs = []ClassPair{{From: "x", To: "server"}} }, "cmdb.class_unknown"},
		{"an unknown target class", func(in *RelationTypeInput) { in.Pairs = []ClassPair{{From: "server", To: "x"}} }, "cmdb.class_unknown"},
	}
	for _, c := range cases {
		next := in
		c.change(&next)
		if _, err := NewRelationType(next, types, classes); code(err) != c.code {
			t.Errorf("%s: %q, want %q", c.name, code(err), c.code)
		}
	}
	own, err := NewRelationType(in, types, classes)
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := own.Changed(RelationTypeInput{Name: "Saves", ReverseName: "Saved by", Impact: ImpactBoth, Now: now.Add(time.Hour)}, classes)
	if err != nil || renamed.Version != own.Version+1 || renamed.Name != "Saves" {
		t.Fatalf("changed %+v %v", renamed, err)
	}
	all := NewRelationTypes([]RelationType{renamed}).All()
	if last := all[len(all)-1]; last.Key != "backs_up" || last.System {
		t.Errorf("the workspace's type is not listed after the shipped ones: %+v", last)
	}
	for i := 1; i < len(all)-1; i++ {
		if all[i-1].System == all[i].System && all[i-1].Key > all[i].Key {
			t.Errorf("not in key order: %s before %s", all[i-1].Key, all[i].Key)
		}
	}
	source, target := NewRelationTypes([]RelationType{renamed}).Suffering()
	if !containsKey(source, "backs_up") || !containsKey(target, "backs_up") || !containsKey(source, RelDependsOn) ||
		containsKey(target, RelDependsOn) {
		t.Errorf("suffering: source %v target %v", source, target)
	}
	if !own.Allows("server", "database_instance", classes) || own.Allows("database_instance", "server", classes) ||
		!(RelationType{}).Allows("server", "server", classes) {
		t.Error("a type allows the wrong pair")
	}
}

func containsKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

func TestTheClassesAreListedShippedFirstAndAWorkspaceClassChanges(t *testing.T) {
	classes := NewClasses(nil)
	own, err := NewClass(ClassInput{Key: "firewall", Parent: "network_device", Name: "Firewall", Now: now}, classes)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := own.Changed(ClassInput{Parent: "network_device", Name: " Packet filter ", Now: now.Add(time.Hour)}, classes)
	if err != nil || changed.Key != "firewall" || changed.Name != "Packet filter" || changed.Version != own.Version+1 {
		t.Fatalf("changed %+v %v", changed, err)
	}
	if _, err := own.Changed(ClassInput{Parent: "nope", Name: "X"}, classes); code(err) != "cmdb.class_parent_unknown" {
		t.Errorf("a change to an unknown parent: %v", err)
	}
	all := NewClasses([]Class{changed}).All()
	if last := all[len(all)-1]; last.Key != "firewall" || last.System {
		t.Errorf("the workspace's class is not listed last: %+v", last)
	}
	if !all[0].System {
		t.Error("a shipped class is not first")
	}
}

func TestTheCountsAddEveryOutcome(t *testing.T) {
	var counts Counts
	for _, outcome := range []Outcome{OutcomeCreate, OutcomeUpdate, OutcomeUnchanged, OutcomeHold, OutcomeDuplicate, OutcomeRefused, OutcomeCreate} {
		counts.Count(Decision{Outcome: outcome})
	}
	if counts.Items != 7 || counts.Created != 2 || counts.Updated != 1 || counts.Unchanged != 1 || counts.Held != 1 ||
		counts.Duplicates != 1 || counts.Refused != 1 {
		t.Errorf("counts %+v", counts)
	}
	settings := map[string]string{"url": "https://monitor.example"}
	copied := Connector{Settings: settings}.SettingsCopy()
	copied["url"] = "elsewhere"
	if settings["url"] != "https://monitor.example" {
		t.Error("the settings copy is the connector's own map")
	}
}
