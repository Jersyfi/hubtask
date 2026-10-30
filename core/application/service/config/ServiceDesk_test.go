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
