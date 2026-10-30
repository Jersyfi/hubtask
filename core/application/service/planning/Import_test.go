// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"strconv"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// importing answers each write as its use case would and keeps them in order, by name.
type importing struct {
	names []string
	calls []usecase.Input
}

func (c *importing) Invoke(_ context.Context, name string, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c.names, c.calls = append(c.names, name), append(c.calls, in)
	return usecase.Output{"id": "01936f2a-7c1e-7000-8000-0000000009" + pad2(len(c.calls))}, nil
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

// UC-PLN-09 checks 4 and 5: a file comes into a collection as entries at its outline's depth -
// a row below an activity flattened onto its parent -, with milestones marked and each
// predecessor a dependency with its timing, every write through its own use case; a dry run
// writes nothing.
func TestAProjectFileComesInThroughTheEntriesOwnUseCases(t *testing.T) {
	f := newFixture()
	writes := &importing{}
	f.planner.Catalogue = writes
	csv := "ID,Name,Start,Finish,Predecessors,Outline Level,Milestone\n" +
		"1,Phase,2026-11-02,2026-11-20,,1,\n" +
		"2,Design,2026-11-02,2026-11-06,,2,\n" +
		"3,Sketch,2026-11-02,2026-11-03,,3,\n" +
		"4,Detail,2026-11-04,2026-11-04,,4,\n" +
		"5,Launch,,2026-11-20,2SS+1d;3,1,yes\n"
	cmd := ImportPlanCommand{CollectionID: collection, Format: domain.FormatCSV, Content: []byte(csv), DryRun: true}
	dry, err := ImportPlan{Planner: f.planner}.Execute(context.Background(), actor(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes.calls) != 0 || dry.Tasks != 2 || dry.WorkPackages != 1 || dry.Activities != 2 || dry.Flattened != 1 ||
		dry.Dependencies != 2 || dry.Milestones != 1 {
		t.Fatalf("the dry run %+v wrote %v", dry, writes.names)
	}
	if asked := f.gate.asked[len(f.gate.asked)-1]; asked.Permission != service.PermissionWriteItems || asked.TargetID != collection {
		t.Errorf("asked %+v, want WRITE_ITEMS at the collection", asked)
	}
	cmd.DryRun = false
	done, err := ImportPlan{Planner: f.planner}.Execute(context.Background(), actor(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		createWorkItemName, createWorkItemName, createWorkItemName, createWorkItemName, createWorkItemName,
		SetEntrySchedulingName, linkEntriesName, SetDependencyTimingName, linkEntriesName,
	}
	if len(writes.names) != len(want) {
		t.Fatalf("wrote %v, want %v", writes.names, want)
	}
	for i, name := range want {
		if writes.names[i] != name {
			t.Fatalf("wrote %v, want %v", writes.names, want)
		}
	}
	design, sketch, detail, launch := writes.calls[1], writes.calls[2], writes.calls[3], writes.calls[4]
	if design["type"] != "WORK_PACKAGE" || sketch["type"] != "ACTIVITY" || detail["type"] != "ACTIVITY" ||
		sketch["parent_id"] != created(done, "2") || detail["parent_id"] != created(done, "2") {
		t.Errorf("the tree: design %v, sketch %v, detail %v", design, sketch, detail)
	}
	if launch["start_at"] != nil || launch["due_date_only"] != true || writes.calls[5]["milestone"] != true {
		t.Errorf("the milestone %v, %v", launch, writes.calls[5])
	}
	if timing := writes.calls[7]; timing["dependency_kind"] != "SS" || timing["lag_days"] != 1 {
		t.Errorf("the timing %v", timing)
	}
	if last := f.audit.entries[len(f.audit.entries)-1]; last.Action != PlanImportedAction {
		t.Errorf("recorded %v, want the import", last.Action)
	}
}

func created(i Imported, key string) string {
	for _, row := range i.Created {
		if row["key"] == key {
			return row["item_id"].(string)
		}
	}
	return ""
}
