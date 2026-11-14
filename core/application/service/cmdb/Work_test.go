// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-AST-09 check 7: a CI lists every entry naming it, or a CI beneath it - one whose failure it
// suffers -, each once with its reason, open ones first, only those the reader may see and never
// archived ones; a reader of another hub learns nothing of the CI.
func TestACIListsTheWorkOnItAndBeneathIt(t *testing.T) {
	f, a := assuredFixture()
	var (
		both     = ciID("0105")
		above    = ciID("0106")
		hidden   = ciID("0107")
		archived = ciID("0108")
		done     = ciID("0109")
	)
	entries := plannedEntries{
		incident: {ID: incident, HubID: hub, CollectionID: collected, Title: "Booking is slow", Handle: "inc", Number: 7, Status: "open"},
		change1:  {ID: change1, HubID: hub, CollectionID: collected, Title: "Patch host1"},
		change2:  {ID: change2, HubID: hub, CollectionID: collected, Title: "Resize vm1"},
		both:     {ID: both, HubID: hub, CollectionID: collected, Title: "Booking and its machine"},
		above:    {ID: above, HubID: hub, CollectionID: collected, Title: "Reception offering"},
		hidden:   {ID: hidden, HubID: private, CollectionID: collected, Title: "Private"},
		archived: {ID: archived, HubID: hub, CollectionID: collected, Title: "Old", Archived: true},
		done:     {ID: done, HubID: hub, CollectionID: collected, Title: "Done", Completed: true},
	}
	for i, pair := range [][2]shared.ID{
		{done, booking}, {incident, booking}, {change1, host}, {change2, vm1}, {both, vm1}, {both, booking},
		{above, offering}, {hidden, booking}, {archived, booking},
	} {
		a.affected = append(a.affected, domain.Affected{ID: ciID(fmt.Sprintf("2%03d", i)), ItemID: pair[0], CIID: pair[1]})
	}
	use := ListCiWork{Affecting: Affecting{Inventory: f.inv, Pointers: Pointers{Entries: entries, Reader: seeing{}}}}.Descriptor()

	out, err := call(t, use, as(bert), usecase.Input{"ci_id": booking.String()})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]usecase.Output{}
	var order []string
	for _, row := range out["data"].([]usecase.Output) {
		id := row["item_id"].(string)
		if _, twice := got[id]; twice {
			t.Errorf("%s listed twice", id)
		}
		got[id] = row
		order = append(order, id)
	}
	if len(got) != 5 {
		t.Fatalf("the entries on booking and beneath it: %v", order)
	}
	if order[len(order)-1] != done.String() || got[done.String()]["completed"] != true {
		t.Errorf("the completed entry comes last: %v", order)
	}
	for _, id := range []shared.ID{incident, both, done} {
		if row := got[id.String()]; row["reason"] != "NAMES" || row["via"] != nil {
			t.Errorf("%s names booking itself: %v", id, row)
		}
	}
	for id, via := range map[shared.ID]shared.ID{change1: host, change2: vm1} {
		row := got[id.String()]
		v, _ := row["via"].(usecase.Output)
		if row["reason"] != "BENEATH" || v == nil || v["ci_id"] != via.String() {
			t.Errorf("%s names a CI beneath booking: %v", id, row)
		}
	}
	if row := got[incident.String()]; row["key"] != "INC-7" || row["status"].(usecase.Output)["key"] != "open" {
		t.Errorf("the incident's key and status: %v", row)
	}
	if row := got[change1.String()]; row["key"] != nil || row["status"] != nil {
		t.Errorf("an entry without a number or a workflow: %v", row)
	}
	for _, id := range []shared.ID{above, hidden, archived} {
		if _, ok := got[id.String()]; ok {
			t.Errorf("%s is listed", id)
		}
	}

	if _, err := call(t, use, as(bert), usecase.Input{"ci_id": vault.String()}); !errors.Is(err, shared.ErrNotFound) && !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("Bert read the work on a CI of a hub out of his reach: %v", err)
	}
}
