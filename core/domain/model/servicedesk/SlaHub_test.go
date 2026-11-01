// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	deskHub  = shared.MustParseID("0192f000-0000-7000-8000-0000000005b1")
	salesHub = shared.MustParseID("0192f000-0000-7000-8000-0000000005b2")
)

// UC-SVC-02 check 11: a hub's own policy measures only the entries inside that hub and is tried
// there before the workspace's, whatever its position; another hub's is never a choice.
func TestAHubsOwnPolicyIsTriedFirstInsideItsHub(t *testing.T) {
	common, err := NewSLAPolicy(policyInput())
	if err != nil {
		t.Fatal(err)
	}
	in := policyInput()
	in.Key, in.Position, in.HubID = "desk_incidents", 9, deskHub
	own, err := NewSLAPolicy(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Key, in.Position, in.HubID = "sales_incidents", 0, salesHub
	other, err := NewSLAPolicy(in)
	if err != nil {
		t.Fatal(err)
	}
	all := []SLAPolicy{common, own, other}
	for _, tc := range []struct {
		name string
		hub  shared.ID
		want string
	}{
		{"inside the hub", deskHub, "desk_incidents"},
		{"another hub", salesHub, "sales_incidents"},
		{"a hub without its own", shared.MustParseID("0192f000-0000-7000-8000-0000000005b3"), "incidents"},
		{"no hub", "", "incidents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chosen, found := Choose(PoliciesWithin(all, tc.hub), "incident", "")
			if !found || chosen.Key != tc.want {
				t.Errorf("chose %q (%v), want %q", chosen.Key, found, tc.want)
			}
		})
	}
}

// A copy takes everything of its source but its identity and key, belongs to the hub, and names
// the key it came from.
func TestCopyingAPolicyToAHub(t *testing.T) {
	source, err := NewSLAPolicy(policyInput())
	if err != nil {
		t.Fatal(err)
	}
	name := "Desk incidents"
	copied, err := source.CopyAt(SLAPolicyCopy{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000005b4"), HubID: deskHub, Key: "desk_incidents",
		Name: &name, Now: policyInput().Now, Text: policyInput().Text,
	})
	if err != nil {
		t.Fatal(err)
	}
	if copied.HubID != deskHub || copied.CopiedFrom != "incidents" || copied.Name != name || copied.Key != "desk_incidents" ||
		copied.TenantID != source.TenantID || len(copied.Goals) != len(source.Goals) || copied.CalendarKey != "office" {
		t.Errorf("copy %+v", copied)
	}
	if _, err := source.CopyAt(SLAPolicyCopy{ID: copied.ID, Key: "x", Now: policyInput().Now, Text: policyInput().Text}); hubDetail(err) != "sla_policies.hub_required" {
		t.Errorf("a copy without a hub: %v", err)
	}
	// A change keeps the hub and the source.
	changed, err := copied.Change(policyInput())
	if err != nil || changed.HubID != deskHub || changed.CopiedFrom != "incidents" {
		t.Errorf("a change moved the policy: %+v, %v", changed, err)
	}
}

func hubDetail(err error) string {
	var coded *shared.Error
	if errors.As(err, &coded) {
		return coded.DetailCode
	}
	return ""
}
