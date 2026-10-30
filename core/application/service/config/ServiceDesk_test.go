// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

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
	if !(at[sectionTypes] < at[sectionCalendars] && at[sectionCalendars] < at[sectionPolicies]) {
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

// The portal's organisations are a section of the document after the canned replies: a change
// is flagged as changing who may read what, and a domain moved from one organisation to another
// in one apply is given up before it is claimed, whatever the keys' order.
func TestTheOrganisationsAreInTheDocument(t *testing.T) {
	var organisations section
	at := map[string]int{}
	for i, s := range sections() {
		at[s.name] = i
		if s.name == sectionOrganisations {
			organisations = s
		}
	}
	if organisations.module != module.ServiceDesk || at[sectionOrganisations] <= at[sectionReplies] {
		t.Fatalf("the organisations' section is %+v at %v", organisations, at)
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

// A published template leaves out the organisations: their domains name this workspace's customers.
func TestATemplateLeavesTheOrganisationsOut(t *testing.T) {
	kept := templateDocument(domain.Document{
		sectionOrganisations: {"acme": {"name": "Acme", "domains": []any{"acme.example"}}},
		sectionReplies:       {"thanks": {"name": "Thanks", "body": "Thank you."}},
	})
	if _, held := kept[sectionOrganisations]; held || kept[sectionReplies]["thanks"] == nil {
		t.Fatalf("the template keeps %v", kept)
	}
}
