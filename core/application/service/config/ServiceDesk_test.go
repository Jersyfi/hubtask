// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"maps"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"

	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
)

// The service desk's calendars, SLA policies and canned replies are blocks of the document, after
// the types a policy measures and with calendars before the policies naming them; a change defines
// the block whole under its key, a removal deletes it by key.
func TestTheServiceDesksBlocksAreInTheDocument(t *testing.T) {
	at := map[string]int{}
	byName := map[string]section{}
	for i, s := range sections() {
		at[s.name], byName[s.name] = i, s
	}
	for _, name := range []string{sectionCalendars, sectionPolicies, sectionReplies} {
		if _, held := at[name]; !held || byName[name].module != module.ServiceDesk {
			t.Fatalf("%s is not a service desk section", name)
		}
	}
	if at[sectionTypes] >= at[sectionCalendars] || at[sectionCalendars] >= at[sectionPolicies] {
		t.Errorf("the apply order is %v", at)
	}
	policies := byName[sectionPolicies]
	ops, err := policies.operations(domain.BlockChange{
		Section: sectionPolicies, Key: "incidents", Op: domain.OpChange, Fields: []string{"goals"},
		After: domain.Block{"name": "Incidents", "entry_types": []any{"incident"}, "goals": []any{}, "enabled": true},
	}, map[string]any{"key": "incidents"})
	if err != nil || len(ops) != 1 || ops[0].UseCase != "DefineSlaPolicy" || ops[0].Input["key"] != "incidents" ||
		ops[0].Input["name"] != "Incidents" {
		t.Fatalf("a changed policy runs %+v, %v", ops, err)
	}
	removed, err := byName[sectionReplies].operations(domain.BlockChange{Section: sectionReplies, Key: "thanks", Op: domain.OpRemove},
		map[string]any{"key": "thanks"})
	if err != nil || len(removed) != 1 || removed[0].UseCase != "DeleteReplyTemplate" || removed[0].Input["key"] != "thanks" {
		t.Fatalf("a removed reply runs %+v, %v", removed, err)
	}
}

// UC-SVC-11 check 6: the portal's organisations are a section of the document by their keys,
// before the policies naming them, exported with name, domains and sharing and applied through
// their own use cases: a change is flagged as changing who may read what, and a domain moved from
// one organisation to another in one apply is given up before it is claimed, whatever the keys'
// order.
func TestTheOrganisationsAreInTheDocument(t *testing.T) {
	var organisations section
	at := map[string]int{}
	for i, s := range sections() {
		at[s.name] = i
		if s.name == sectionOrganisations {
			organisations = s
		}
	}
	if organisations.module != module.ServiceDesk || at[sectionOrganisations] >= at[sectionPolicies] {
		t.Fatalf("the organisations' section is %+v at %v", organisations, at)
	}
	if !slices.Equal(organisations.attributes, []string{"name", "domains", "shares_requests"}) || organisations.key != "key" {
		t.Errorf("the organisations are exported as %v by %q", organisations.attributes, organisations.key)
	}
	now := current{
		Document: domain.Document{sectionOrganisations: {
			"zeta": {"name": "Zeta", "domains": []any{"zeta.example", "acme.example"}, "shares_requests": false},
		}},
		Rows: map[string]map[string]map[string]any{sectionOrganisations: {"zeta": {"key": "zeta"}}},
	}
	desired := domain.Document{sectionOrganisations: {
		"alpha": {"name": "Alpha", "domains": []any{"acme.example"}, "shares_requests": true},
		"zeta":  {"name": "Zeta", "domains": []any{"zeta.example"}, "shares_requests": false},
	}}
	p := Desk{Catalogue: &scoped{}}.planOf(now, desired, domain.ModeMerge)
	if len(p.Refusals) > 0 || len(p.Ops) != 2 || !p.Flags.ChangesPermissions {
		t.Fatalf("the plan is %+v", p)
	}
	if p.Ops[0].Path != "requester_organisations.zeta" || p.Ops[1].Path != "requester_organisations.alpha" ||
		p.Ops[1].UseCase != "DefineRequesterOrganisation" || p.Ops[1].Input["shares_requests"] != true {
		t.Fatalf("the domain is claimed before it is given up: %+v", p.Ops)
	}

	removed := Desk{Catalogue: &scoped{}}.planOf(now, domain.Document{sectionOrganisations: {
		"zeta":  nil,
		"alpha": {"name": "Alpha", "domains": []any{"acme.example"}},
	}}, domain.ModeMerge)
	if len(removed.Ops) != 2 || removed.Ops[0].UseCase != "DeleteRequesterOrganisation" {
		t.Fatalf("a removal giving up the domain does not run first: %+v", removed.Ops)
	}
}

// UC-SVC-11 check 6: a published template leaves out the organisations - their domains name this
// workspace's customers.
func TestATemplateLeavesTheOrganisationsOut(t *testing.T) {
	kept := templateDocument(domain.Document{
		sectionOrganisations: {"acme": {"name": "Acme", "domains": []any{"acme.example"}}},
		sectionReplies:       {"thanks": {"name": "Thanks", "body": "Thank you."}},
	})
	if _, held := kept[sectionOrganisations]; held || kept[sectionReplies]["thanks"] == nil {
		t.Fatalf("the template keeps %v", kept)
	}
}

// A policy's requester organisations and an OLA's assignee group are attributes of its block: a
// policy is defined whole, so a block without them would clear them on any apply that touches the
// policy - a rollback, a template's upgrade, a draft published.
func TestAPolicysOrganisationsAndGroupSurviveAnApply(t *testing.T) {
	var policies section
	for _, s := range sections() {
		if s.name == sectionPolicies {
			policies = s
		}
	}
	group := "0192f000-0000-7000-8000-00000000000a"
	key, block := policies.read(map[string]any{
		"key": "network", "name": "Network team", "entry_types": []any{"incident"}, "goals": []any{},
		"organisations": []any{"acme"}, "assignee_group": group, "position": 1, "enabled": true,
	})
	if key != "network" || block["assignee_group"] != group || block["organisations"] == nil {
		t.Fatalf("the block %q leaves out what the policy holds: %v", key, block)
	}
	renamed := domain.Block{}
	for attribute, value := range block {
		renamed[attribute] = value
	}
	renamed["name"] = "Network"
	ops, err := policies.operations(domain.BlockChange{
		Section: sectionPolicies, Key: "network", Op: domain.OpChange, Fields: []string{"name"}, Before: block, After: renamed,
	}, map[string]any{"key": "network"})
	if err != nil || len(ops) != 1 || ops[0].Input["assignee_group"] != group || ops[0].Input["organisations"] == nil {
		t.Fatalf("a renamed policy is defined as %+v, %v", ops, err)
	}
}

// A policy for an organisation and the organisation, added in one apply - a workspace's document
// applied to a new one -, define the organisation first: the policy names only one the workspace
// holds (sla_policies.organisation_unknown).
func TestAnOrganisationIsDefinedBeforeThePolicyNamingIt(t *testing.T) {
	policy := domain.Block{
		"name": "Acme incidents", "entry_types": []any{"incident"}, "organisations": []any{"acme"},
		"goals": []any{map[string]any{"kind": "RESOLUTION", "minutes": 480}}, "position": 1, "enabled": true,
	}
	organisation := domain.Block{"name": "Acme", "domains": []any{"acme.example"}, "shares_requests": false}
	p := Desk{Catalogue: &scoped{}}.planOf(current{Document: domain.Document{}}, domain.Document{
		sectionPolicies: {"acme_incidents": policy}, sectionOrganisations: {"acme": organisation},
	}, domain.ModeMerge)
	if len(p.Refusals) > 0 || len(p.Ops) != 2 {
		t.Fatalf("the plan is %+v", p)
	}
	if p.Ops[0].UseCase != "DefineRequesterOrganisation" || p.Ops[1].UseCase != "DefineSlaPolicy" {
		t.Errorf("the apply runs %s before %s", p.Ops[0].UseCase, p.Ops[1].UseCase)
	}
}

// An organisation removed with the policy naming it is deleted after the policy: deleting it
// first is refused as organisations.in_use. Only a removal whose domain another organisation of
// the same apply claims runs early.
func TestAnOrganisationIsRemovedAfterThePolicyNamingIt(t *testing.T) {
	now := current{
		Document: domain.Document{
			sectionOrganisations: {"acme": {"name": "Acme", "domains": []any{"acme.example"}, "shares_requests": false}},
			sectionPolicies:      {"gold": {"name": "Gold", "organisations": []any{"acme"}}},
		},
		Rows: map[string]map[string]map[string]any{
			sectionOrganisations: {"acme": {"key": "acme"}},
			sectionPolicies:      {"gold": {"key": "gold"}},
		},
	}
	p := Desk{Catalogue: &scoped{}}.planOf(now, domain.Document{
		sectionOrganisations: {"acme": nil}, sectionPolicies: {"gold": nil},
	}, domain.ModeMerge)
	var order []string
	for _, op := range p.Ops {
		order = append(order, op.UseCase)
	}
	if !slices.Equal(order, []string{"DeleteSlaPolicy", "DeleteRequesterOrganisation"}) {
		t.Fatalf("the removals run as %v", order)
	}
}

// The plan refuses removing a calendar a policy of the document still names, as the apply's
// DeleteBusinessCalendar would, in the apply's order: a policy removed or moved to another
// calendar in the same document no longer names it, and a key the installation also offers is
// what the policy then counts on.
func TestAPlanRefusesRemovingACalendarAPolicyStillNames(t *testing.T) {
	calendar := map[string]any{"key": "office", "name": "Office", "zone": "Europe/Berlin", "hours": []any{}, "holidays": []any{}}
	policy := map[string]any{"key": "incidents", "name": "Incidents", "calendar": "office", "entry_types": []any{}, "goals": []any{}, "enabled": true}
	setUp := func(offered ...string) fixture {
		f := newFixture()
		f.world.calendars = map[string]map[string]any{"office": maps.Clone(calendar)}
		f.world.policies = map[string]map[string]any{"incidents": maps.Clone(policy)}
		f.world.offered = offered
		return f
	}
	removal := func(also map[string]any) map[string]any {
		document := map[string]any{sectionCalendars: map[string]any{}}
		maps.Copy(document, also)
		return document
	}
	refused := func(t *testing.T, f fixture, document map[string]any) []any {
		t.Helper()
		out := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document, "mode": "REPLACE"})
		refusals, _ := out["refusals"].([]usecase.Output)
		var found []any
		for _, r := range refusals {
			if r["code"] != "calendars.in_use" {
				t.Fatalf("the plan refused for another reason: %v", refusals)
			}
			found = append(found, r)
		}
		return found
	}

	if got := refused(t, setUp(), removal(nil)); len(got) != 1 {
		t.Errorf("a calendar a policy names was planned away: %v", got)
	}
	if got := refused(t, setUp(), removal(map[string]any{sectionPolicies: map[string]any{}})); len(got) != 0 {
		t.Errorf("a calendar whose policy goes in the same apply was refused: %v", got)
	}
	moved := maps.Clone(policy)
	moved["calendar"] = "standard"
	delete(moved, "key")
	if got := refused(t, setUp(), removal(map[string]any{sectionPolicies: map[string]any{"incidents": moved}})); len(got) != 0 {
		t.Errorf("a calendar whose policy moves to another was refused: %v", got)
	}
	if got := refused(t, setUp("office"), removal(nil)); len(got) != 0 {
		t.Errorf("a calendar the installation also offers was refused: %v", got)
	}
}
