// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func id(n string) shared.ID { return shared.ID("00000000-0000-4000-8000-0000000000" + n) }

// UC-AST-06 check 2: a class without its own period is stale after the root's 60 days; the health
// test finds the CI unconfirmed for 90 days stale.
func TestAClassInheritsAttributesAndRulesNearestFirst(t *testing.T) {
	r := NewClasses(nil).Resolve("server")
	if _, ok := r.Attribute("serial_number"); !ok {
		t.Fatal("a server inherits hardware's serial number")
	}
	if r.Rules[0].Class != "computer" || r.Rules[len(r.Rules)-1].Class != "hardware" {
		t.Fatalf("rules nearest first, got %+v", r.Rules)
	}
	if r.StalenessDays != DefaultStalenessDay || DefaultStalenessDay != 60 {
		t.Fatalf("staleness inherited from the root's 60 days, got %d", r.StalenessDays)
	}
	if vm := NewClasses(nil).Resolve("virtual_machine"); len(vm.Required) != 1 || vm.Required[0].Class != "virtual_machine" {
		t.Fatalf("a required relation names the class that declared it, got %+v", vm.Required)
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

// The index is asked per lookup: the native key alone, and every rule the item carries whole, so
// a shared value of one rule never stands for the item on its own.
func TestLookupsAreTheNativeKeyAndTheWholeRules(t *testing.T) {
	server := NewClasses(nil).Resolve("server")
	cases := []struct {
		name   string
		item   Item
		native []Lookup
		rules  []Lookup
	}{
		{"native key and a whole rule", Item{Name: "x", NativeKey: "42", Attributes: map[string]any{"mac_address": "AA-BB", "hostname": "web1"}},
			[]Lookup{{NativeKey("netbox"): "42"}}, []Lookup{{"mac_address": "aa:bb"}}},
		{"a shared value only with its partner", Item{Name: "x", Attributes: map[string]any{"manufacturer": "Acme", "serial_number": "S-1"}},
			nil, []Lookup{{"serial_number": "s-1", "manufacturer": "acme"}}},
		{"half a rule asks nothing", Item{Name: "x", Attributes: map[string]any{"manufacturer": "Acme"}}, nil, nil},
	}
	for _, c := range cases {
		native, rules := Lookups(server, ItemIdentifiers(c.item, server, "netbox"), "netbox")
		if !reflect.DeepEqual(native, c.native) || !reflect.DeepEqual(rules, c.rules) {
			t.Errorf("%s: native %v, rules %v", c.name, native, rules)
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

// UC-AST-05 check 3.
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

// UC-AST-06 check 2.
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
// UC-AST-06 checks 7 and 8.
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

func TestAnItemHoldingAZeroByteIsKnown(t *testing.T) {
	cases := []struct {
		name string
		item Item
		want bool
	}{
		{"clean", Item{Ref: "r", Class: "server", Name: "web01", NativeKey: "1", Attributes: map[string]any{"hostname": "web01", "n": 2.0}}, false},
		{"in the name", Item{Name: "web\x0001"}, true},
		{"in the reference", Item{Ref: "r\x00"}, true},
		{"in the class", Item{Class: "server\x00"}, true},
		{"in the native key", Item{NativeKey: "\x001"}, true},
		{"in an attribute's value", Item{Attributes: map[string]any{"hostname": "a\x00"}}, true},
		{"in an attribute's key", Item{Attributes: map[string]any{"host\x00": "a"}}, true},
		{"in a list", Item{Attributes: map[string]any{"tags": []any{"a", "b\x00"}}}, true},
		{"in a nested object", Item{Attributes: map[string]any{"o": map[string]any{"k": "v\x00"}}}, true},
	}
	for _, c := range cases {
		if got := c.item.HoldsZeroByte(); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// A review is due by a day of the workspace's zone, the one now is read in: in Berlin a review
// due 5 November is overdue from 00:00 on the 6th there - an hour before UTC's day turns - and a
// review asked for at 00:30 on 30 October there is next due on 30 October a year on.
func TestAReviewFallsDueByTheDayOfTheZoneNowIsIn(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no zone database")
	}
	due := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	ci := []CI{{ID: id("01"), Class: "server", Stage: StageInUse, Compliance: Compliance{ReviewDue: &due, OwnerGroup: id("99")}}}
	overdue := func(at time.Time) bool {
		return Judge(ci, nil, NewClasses(nil), NewRelationTypes(nil), nil, nil, at).ReviewOverdue == 1
	}
	if overdue(time.Date(2026, 11, 5, 23, 30, 0, 0, berlin)) {
		t.Error("overdue on its own day in Berlin")
	}
	if !overdue(time.Date(2026, 11, 6, 0, 30, 0, 0, berlin)) {
		t.Error("not overdue on the next day in Berlin")
	}
	asked := time.Date(2026, 10, 30, 0, 30, 0, 0, berlin)
	if next := (Campaign{ReviewMonths: 12}).NextReview(asked).Format(time.DateOnly); next != "2027-10-30" {
		t.Errorf("next review %s, want 2027-10-30", next)
	}
	if today := Today(asked).Format(time.DateOnly); today != "2026-10-30" {
		t.Errorf("today %s, want 2026-10-30", today)
	}
}

// Software is identified by its package URL, then its CPE, then its name and version
// (UC-AST-04 check 2).
func TestSoftwareIsIdentifiedByPackageURLThenCPEThenNameAndVersion(t *testing.T) {
	var got [][]string
	for _, rule := range NewClasses(nil).Resolve("software").Rules {
		got = append(got, rule.Attributes)
	}
	want := [][]string{{"purl"}, {"cpe"}, {"name", "version"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("software is identified by %v, want %v", got, want)
	}
}

// The shipped relation types are the eight the catalogue names, each saying which end suffers
// (UC-AST-05 check 1).
func TestTheShippedRelationTypesSayWhichEndSuffers(t *testing.T) {
	var keys []string
	for _, rt := range SystemRelationTypes() {
		keys = append(keys, rt.Key)
		switch rt.Impact {
		case ImpactNone, ImpactSource, ImpactTarget, ImpactBoth:
		default:
			t.Errorf("%s says %q suffers", rt.Key, rt.Impact)
		}
		if !rt.System {
			t.Errorf("%s is not marked as shipped", rt.Key)
		}
	}
	want := []string{RelDependsOn, RelRunsOn, RelHostedOn, RelInstalled, RelContains, RelConnects, RelMemberOf, RelOfferingOf}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("the shipped types are %v, want %v", keys, want)
	}
}
