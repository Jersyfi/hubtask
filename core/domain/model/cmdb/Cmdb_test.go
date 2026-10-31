// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func id(n string) shared.ID { return shared.ID("00000000-0000-4000-8000-0000000000" + n) }

func TestAClassInheritsAttributesAndRulesNearestFirst(t *testing.T) {
	r := NewClasses(nil).Resolve("server")
	if _, ok := r.Attribute("serial_number"); !ok {
		t.Fatal("a server inherits hardware's serial number")
	}
	if r.Rules[0].Class != "computer" || r.Rules[len(r.Rules)-1].Class != "hardware" {
		t.Fatalf("rules nearest first, got %+v", r.Rules)
	}
	if r.StalenessDays != DefaultStalenessDay {
		t.Fatalf("staleness inherited from the root, got %d", r.StalenessDays)
	}
}

func TestAWorkspaceClassIsCheckedAgainstTheRegistry(t *testing.T) {
	classes := NewClasses(nil)
	cases := []struct {
		name string
		in   ClassInput
		code string
	}{
		{"a shipped key", ClassInput{Key: "server", Parent: "computer", Name: "S"}, "cmdb.class_key_taken"},
		{"an unknown parent", ClassInput{Key: "x", Parent: "nope", Name: "X"}, "cmdb.class_parent_unknown"},
		{"an inherited attribute again", ClassInput{Key: "x", Parent: "server", Name: "X",
			Attributes: []Attribute{{Key: "hostname", Kind: AttrText, Need: NeedOptional}}}, "cmdb.attribute_key_invalid"},
		{"a rule on an unknown attribute", ClassInput{Key: "x", Parent: "server", Name: "X",
			Rules: []IdentifierRule{{Attributes: []string{"nope"}}}}, "cmdb.rule_attribute_unknown"},
		{"a choice without options", ClassInput{Key: "x", Parent: "server", Name: "X",
			Attributes: []Attribute{{Key: "tier", Kind: AttrChoice, Need: NeedOptional}}}, "cmdb.attribute_options_invalid"},
		{"fine", ClassInput{Key: "firewall", Parent: "network_device", Name: "Firewall",
			Attributes: []Attribute{{Key: "zone", Kind: AttrText, Need: NeedRequired}}}, ""},
	}
	for _, c := range cases {
		_, err := NewClass(c.in, classes)
		if got := code(err); got != c.code {
			t.Errorf("%s: got %q, want %q", c.name, got, c.code)
		}
	}
}

func code(err error) string {
	if err == nil {
		return ""
	}
	if e := (*shared.Error)(nil); errors.As(err, &e) {
		return e.DetailCode
	}
	return err.Error()
}

func TestIdentifyTriesTheNativeKeyThenTheRulesInOrder(t *testing.T) {
	classes := NewClasses(nil)
	server := classes.Resolve("server")
	rows := []IdentifierRow{
		{CI: id("01"), Class: "server", Name: "mac_address", Value: "aa:bb"},
		{CI: id("02"), Class: "server", Name: NativeKey("netbox"), Value: "42"},
		{CI: id("03"), Class: "server", Name: "hostname", Value: "web1"},
		{CI: id("03"), Class: "server", Name: "domain", Value: "example.org"},
		{CI: id("04"), Class: "software", Name: "hostname", Value: "web1"},
	}
	cases := []struct {
		name  string
		item  Item
		rule  string
		match []shared.ID
		use   bool
	}{
		{"native key first", Item{Name: "x", NativeKey: "42", Attributes: map[string]any{"mac_address": "AA-BB"}}, "native_key", []shared.ID{id("02")}, true},
		{"mac normalised", Item{Name: "x", Attributes: map[string]any{"mac_address": "AA-BB"}}, "mac_address", []shared.ID{id("01")}, true},
		{"every attribute of a rule", Item{Name: "x", Attributes: map[string]any{"hostname": "WEB1", "domain": "example.org"}}, "hostname+domain", []shared.ID{id("03")}, true},
		{"half a rule is no rule", Item{Name: "x", Attributes: map[string]any{"hostname": "web1"}}, "", nil, false},
		{"usable, nothing found", Item{Name: "x", Attributes: map[string]any{"mac_address": "cc"}}, "", nil, true},
	}
	for _, c := range cases {
		got := Identify(server, ItemIdentifiers(c.item, server, "netbox"), "netbox", rows, classes)
		if got.Rule != c.rule || len(got.Matches) != len(c.match) || got.Usable != c.use || (len(c.match) > 0 && got.Matches[0] != c.match[0]) {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}

func TestALowerSourceDoesNotOverwriteWhatAHigherOneWrote(t *testing.T) {
	server := NewClasses(nil).Resolve("server")
	p := Precedence{ManualSource: 10, "netbox": 50, "scan": 100}
	ci, taken, _, err := Apply(CI{}, Item{Name: "web1", Attributes: map[string]any{"model": "R650"}}, server, "netbox", id("09"), p, now)
	if err != nil || len(taken) != 2 {
		t.Fatalf("a new CI takes everything, got %v %v", taken, err)
	}
	ci, taken, kept, _ := Apply(ci, Item{Attributes: map[string]any{"model": "R640", "manufacturer": "Dell"}}, server, "scan", id("10"), p, now.Add(time.Hour))
	if ci.Attributes["model"] != "R650" || len(kept) != 1 || kept[0].By != "netbox" || len(taken) != 1 {
		t.Fatalf("model kept by netbox, manufacturer taken: %v %v %v", ci.Attributes, taken, kept)
	}
	later := now.Add(61 * 24 * time.Hour)
	ci, _, kept, _ = Apply(ci, Item{Attributes: map[string]any{"model": "R640"}}, server, "scan", id("11"), p, later)
	if ci.Attributes["model"] != "R640" || len(kept) != 0 {
		t.Fatalf("after the staleness period a lower source takes over, got %v", ci.Attributes)
	}
	if _, _, _, err := Apply(ci, Item{Attributes: map[string]any{"nope": "x"}}, server, "scan", id("12"), p, later); code(err) != "cmdb.attribute_unknown" {
		t.Fatalf("an unknown attribute is refused, got %v", err)
	}
}

func TestProtectionIsInheritedAgainstTheImpact(t *testing.T) {
	types := NewRelationTypes(nil)
	app := CI{ID: id("01"), Class: "application_service", Compliance: Compliance{Protection: OwnProtection{Protection: Protection{C: LevelHigh, A: LevelVeryHigh}}}}
	vm := CI{ID: id("02"), Class: "virtual_machine"}
	host := CI{ID: id("03"), Class: "server", Compliance: Compliance{Protection: OwnProtection{Protection: Protection{A: LevelNormal}, Fixed: []string{"a"}, Reason: "spread over a cluster"}}}
	rels := []Relation{{Type: RelRunsOn, SourceCI: app.ID, TargetCI: vm.ID}, {Type: RelHostedOn, SourceCI: vm.ID, TargetCI: host.ID}}
	got := EffectiveProtection([]CI{app, vm, host}, rels, types)
	if got[vm.ID] != (Protection{C: LevelHigh, A: LevelVeryHigh}) {
		t.Fatalf("the VM inherits the application's need, got %+v", got[vm.ID])
	}
	if got[host.ID] != (Protection{C: LevelHigh, A: LevelNormal}) {
		t.Fatalf("a fixed dimension keeps its own value, got %+v", got[host.ID])
	}
}

func TestHealthFindsIncompleteStaleOrphanedAndDuplicateCIs(t *testing.T) {
	seen := now.Add(-90 * 24 * time.Hour)
	fresh := now.Add(-time.Hour)
	cis := []CI{
		{ID: id("01"), Class: "application_service", Stage: StageInUse, LastSeen: &fresh},
		{ID: id("02"), Class: "cloud_resource", Stage: StageInUse, LastSeen: &seen, Attributes: map[string]any{"provider": "x", "resource_id": "y", "region": "z"},
			Compliance: Compliance{OwnerGroup: id("99")}},
		{ID: id("03"), Class: "server", Stage: StageRetired},
	}
	h := Judge(cis, nil, NewClasses(nil), NewRelationTypes(nil), map[shared.ID]bool{id("02"): true}, nil, now)
	if h.Total != 2 || h.Orphans != 1 || h.Stale != 1 || h.Duplicates != 1 || h.WithoutOwner != 1 {
		t.Fatalf("got %+v", h)
	}
}

// A service's score is the share of what it rests on that health finds wanting in nothing (H7); a
// relation a program no longer confirms is stale (R10), a person's never.
func TestHealthScoresServicesAndCountsStaleRelations(t *testing.T) {
	fresh, old := now.Add(-time.Hour), now.Add(-90*24*time.Hour)
	owner := Compliance{OwnerGroup: id("99")}
	cis := []CI{
		{ID: id("01"), Class: "business_service", Name: "Booking", Stage: StageInUse, LastSeen: &fresh, Compliance: owner},
		{ID: id("02"), Class: "application_service", Stage: StageInUse, LastSeen: &fresh, Compliance: owner},
		{ID: id("03"), Class: "virtual_machine", Stage: StageInUse, LastSeen: &old},
		{ID: id("04"), Class: "business_service", Name: "Alone", Stage: StageInUse, LastSeen: &fresh, Compliance: owner},
	}
	rels := []Relation{
		{Type: RelDependsOn, SourceCI: id("01"), TargetCI: id("02"), DataSource: ManualSource, LastSeen: old},
		{Type: RelRunsOn, SourceCI: id("02"), TargetCI: id("03"), DataSource: "netbox", LastSeen: old},
	}
	h := Judge(cis, rels, NewClasses(nil), NewRelationTypes(nil), nil, nil, now)
	if h.StaleRelations != 1 {
		t.Errorf("stale relations %d, want the program's one", h.StaleRelations)
	}
	// The application service is a service too, and the worst scored comes first.
	if len(h.Services) != 3 || h.Services[0].Service != id("02") || h.Services[0].CIs != 2 {
		t.Fatalf("services %+v, want the application first over itself and its VM", h.Services)
	}
	if booking := h.Services[1]; booking.Service != id("01") || booking.CIs != 3 || booking.Wanting != 1 {
		t.Errorf("Booking %+v, want its three CIs with the stale VM wanting", booking)
	}
	if h.Services[2].Service != id("04") || h.Services[2].Score != 1 {
		t.Errorf("the lone service %+v, want a full score", h.Services[2])
	}
}

// A source's conflict rate is the share of the attributes it reported that a better source kept
// (H5).
func TestConflictsAreTheShareKeptByABetterSource(t *testing.T) {
	runs := []Run{
		{Source: "scan", Decisions: []Decision{{Taken: []string{"a", "b"}, Kept: []Kept{{Attribute: "c", By: "netbox"}}}}},
		{Source: "scan", Decisions: []Decision{{Kept: []Kept{{Attribute: "c", By: "netbox"}}}}},
		{Source: "netbox", Decisions: []Decision{{Taken: []string{"c"}}}},
	}
	got := Conflicts(runs)
	if len(got) != 2 || got[0].Source != "scan" || got[0].Reported != 4 || got[0].Kept != 2 || got[0].Rate != 0.5 || got[1].Rate != 0 {
		t.Fatalf("conflicts %+v", got)
	}
}

func TestARelationIsHeldToItsClassPairs(t *testing.T) {
	classes, types := NewClasses(nil), NewRelationTypes(nil)
	runsOn, _ := types.Find(RelRunsOn)
	app := CI{ID: id("01"), HubID: id("50"), Class: "application_service"}
	vm := CI{ID: id("02"), HubID: id("50"), Class: "virtual_machine"}
	room := CI{ID: id("03"), HubID: id("50"), Class: "room"}
	if err := CheckRelation(runsOn, app, vm, classes); err != nil {
		t.Fatalf("an application runs on a VM: %v", err)
	}
	if code(CheckRelation(runsOn, app, room, classes)) != "cmdb.relation_not_allowed" {
		t.Fatal("an application does not run on a room")
	}
}

func TestAVirtualMachineIsHostedOnAVirtualisationCluster(t *testing.T) {
	classes, types := NewClasses(nil), NewRelationTypes(nil)
	hostedOn, _ := types.Find(RelHostedOn)
	vm := CI{ID: id("01"), HubID: id("50"), Class: "virtual_machine"}
	cluster := CI{ID: id("02"), HubID: id("50"), Class: "virtualization_cluster"}
	host := CI{ID: id("03"), HubID: id("50"), Class: "server"}
	if err := CheckRelation(hostedOn, vm, cluster, classes); err != nil {
		t.Fatalf("a VM is hosted on its cluster: %v", err)
	}
	if err := CheckRelation(hostedOn, cluster, host, classes); err != nil {
		t.Fatalf("a cluster is hosted on its servers: %v", err)
	}
	if code(CheckRelation(hostedOn, host, cluster, classes)) != "cmdb.relation_not_allowed" {
		t.Fatal("a server is not hosted on a cluster")
	}
	if c, ok := classes.Find("virtualization_cluster"); !ok || !c.System {
		t.Fatal("the cluster is a shipped class")
	}
}
