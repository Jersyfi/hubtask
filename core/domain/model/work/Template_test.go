// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func offset(t *testing.T, spec string) *time.Duration {
	t.Helper()

	parsed, err := work.ParseTemplateOffset(spec)
	if err != nil {
		t.Fatalf("the offset %q was refused: %v", spec, err)
	}
	return &parsed
}

// moveTemplate is the fixture: a task with two steps under it, the second due three days in.
func moveTemplate(t *testing.T) work.TemplateSpec {
	t.Helper()

	return work.TemplateSpec{
		Scope: string(work.TemplateScopeCollection), ScopeID: "c1",
		Name: "Move house", RootType: string(work.ItemTask),
		Root: work.TemplateNode{
			Type: work.ItemTask, Title: "Move house",
			Children: []work.TemplateNode{
				{Type: work.ItemWorkPackage, Title: "Book the van"},
				{
					Type: work.ItemWorkPackage, Title: "Pack the kitchen",
					DueOffset: offset(t, "P3D"), DueDateOnly: true,
				},
			},
		},
	}
}

func draftTemplate(t *testing.T, spec work.TemplateSpec) (work.Template, error) {
	t.Helper()

	return work.NewTemplate(work.NewTemplateInput{
		ID: "t1", TenantID: "tenant1", Spec: spec, Now: remindedAt,
	})
}

// What a definition may be, and every way of getting it wrong. The refusals are the substance: a
// template discovered broken at instantiation is one nobody can fix from a failed instantiation.
func TestATemplateIsCheckedAtItsDefinition(t *testing.T) {
	for name, test := range map[string]struct {
		spec     func(*testing.T, work.TemplateSpec) work.TemplateSpec
		wantCode string
		wantPath string
	}{
		"a tree with two steps": {
			spec: func(_ *testing.T, s work.TemplateSpec) work.TemplateSpec { return s },
		},
		"a workspace-wide template": {
			spec: func(_ *testing.T, s work.TemplateSpec) work.TemplateSpec {
				s.Scope, s.ScopeID = string(work.TemplateScopeTenant), ""
				return s
			},
		},
		"a scope nobody defined": {
			spec: func(_ *testing.T, s work.TemplateSpec) work.TemplateSpec {
				s.Scope = "EVERYWHERE"
				return s
			},
			wantCode: "templates.scope_unknown", wantPath: "/scope_type",
		},
		"a collection scope without its collection": {
			spec: func(_ *testing.T, s work.TemplateSpec) work.TemplateSpec {
				s.ScopeID = ""
				return s
			},
			wantCode: "templates.scope_id_required", wantPath: "/scope_id",
		},
		"a workspace-wide template naming a container": {
			spec: func(_ *testing.T, s work.TemplateSpec) work.TemplateSpec {
				s.Scope = string(work.TemplateScopeTenant)
				return s
			},
			wantCode: "templates.scope_id_not_allowed", wantPath: "/scope_id",
		},
		"no name": {
			spec: func(_ *testing.T, s work.TemplateSpec) work.TemplateSpec {
				s.Name = "  "
				return s
			},
			wantCode: "templates.name_required", wantPath: "/name",
		},
		"a name longer than the bound": {
			spec: func(_ *testing.T, s work.TemplateSpec) work.TemplateSpec {
				s.Name = strings.Repeat("a", work.MaxTemplateNameLength+1)
				return s
			},
			wantCode: "templates.name_too_long", wantPath: "/name",
		},
		"a root that is not the type the template declares": {
			spec: func(_ *testing.T, s work.TemplateSpec) work.TemplateSpec {
				s.Root.Type = work.ItemWorkPackage
				return s
			},
			wantCode: "templates.root_type_mismatch", wantPath: "/nodes/0/type",
		},
		"a step with no title": {
			spec: func(_ *testing.T, s work.TemplateSpec) work.TemplateSpec {
				s.Root.Children[1].Title = " "
				return s
			},
			wantCode: "items.title_empty", wantPath: "/nodes/0/children/1/title",
		},
		"an all-day flag with no date to qualify": {
			spec: func(_ *testing.T, s work.TemplateSpec) work.TemplateSpec {
				s.Root.Children[0].DueDateOnly = true
				return s
			},
			wantCode: "templates.due_date_only_without_offset",
			wantPath: "/nodes/0/children/0/due_date_only",
		},
	} {
		t.Run(name, func(t *testing.T) {
			template, err := draftTemplate(t, test.spec(t, moveTemplate(t)))

			if test.wantCode != "" {
				refusal := shared.AsError(err)
				if refusal == nil || refusal.DetailCode != test.wantCode {
					t.Fatalf("refused as %v, want %s", err, test.wantCode)
				}
				if len(refusal.Fields) != 1 || refusal.Fields[0].Path != test.wantPath {
					t.Errorf("the refusal points at %v, want %s", refusal.Fields, test.wantPath)
				}
				return
			}

			if err != nil {
				t.Fatalf("the template was refused: %v", err)
			}
			if template.NodeCount() != 3 || template.Version != 1 {
				t.Errorf("the template is %+v", template)
			}
		})
	}
}

// The bound the backlog set instead of a jobs resource, and the refusal names the number.
func TestATemplateStaysInsideItsNodeCap(t *testing.T) {
	spec := moveTemplate(t)
	for i := 0; i < work.MaxTemplateNodes; i++ {
		spec.Root.Children = append(spec.Root.Children,
			work.TemplateNode{Type: work.ItemWorkPackage, Title: "Step"})
	}

	_, err := draftTemplate(t, spec)
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "templates.too_many_nodes" {
		t.Fatalf("refused as %v, want templates.too_many_nodes", err)
	}
	if refusal.Params["maximum"] == "" || refusal.Params["count"] == "" {
		t.Errorf("the refusal does not say how many are allowed and how many there were: %v",
			refusal.Params)
	}
}

// A relative date is a length of time, and the two things it may not be are the ones a calendar
// decides: "+1 month" would mean two different things in two different months.
func TestARelativeDateIsADurationAndNotACalendarUnit(t *testing.T) {
	for name, test := range map[string]struct {
		spec     string
		wantCode string
	}{
		"three days":            {spec: "P3D"},
		"the day before":        {spec: "-P1D"},
		"two weeks and an hour": {spec: "P2WT1H"},
		"a month":               {spec: "P1M", wantCode: "templates.due_offset_calendar_unit"},
		"a year":                {spec: "P1Y", wantCode: "templates.due_offset_calendar_unit"},
		"a decade of days":      {spec: "P4000D", wantCode: "templates.due_offset_out_of_range"},
		"not a duration":        {spec: "in three days", wantCode: "templates.due_offset_invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := work.ParseTemplateOffset(test.spec)

			if test.wantCode == "" {
				if err != nil {
					t.Fatalf("the offset was refused: %v", err)
				}
				return
			}
			if refusal := shared.AsError(err); refusal == nil ||
				refusal.DetailCode != test.wantCode {
				t.Fatalf("refused as %v, want %s", err, test.wantCode)
			}
		})
	}
}

// The acceptance's own sentence: a relative date becomes an absolute one against the anchor, in
// the right zone, with the all-day flag preserved.
func TestARelativeDateResolvesAgainstTheAnchor(t *testing.T) {
	template, err := draftTemplate(t, moveTemplate(t))
	if err != nil {
		t.Fatalf("the template was refused: %v", err)
	}

	anchor := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	nodes := template.Nodes()
	packing := nodes[2]

	due, err := packing.DueAt(anchor, "Europe/Berlin")
	if err != nil {
		t.Fatalf("resolving the date: %v", err)
	}
	if due == nil {
		t.Fatal("a node with an offset produced no due date")
	}
	if !due.At.Equal(anchor.AddDate(0, 0, 3)) {
		t.Errorf("the date is %v rather than three days in", due.At)
	}
	if !due.DateOnly || due.TimeZone != "Europe/Berlin" {
		t.Errorf("the date came out as %+v", due)
	}

	// And a node with no offset has no due date at all, rather than the anchor's own.
	if due, err := nodes[1].DueAt(anchor, "Europe/Berlin"); err != nil || due != nil {
		t.Errorf("a node with no offset produced %v (%v)", due, err)
	}
}

// A change is a merge patch, and the tree is one field of it: two devices editing two branches of
// one tree are editing one shape.
func TestChangingATemplateReportsWhatMoved(t *testing.T) {
	template, err := draftTemplate(t, moveTemplate(t))
	if err != nil {
		t.Fatalf("the template was refused: %v", err)
	}

	renamed := "Move flat"
	tree := template.Root
	tree.Children = tree.Children[:1]
	changed, changes, err := template.Changed(
		work.TemplatePatch{Name: &renamed, Root: &tree}, remindedAt)
	if err != nil {
		t.Fatalf("the change was refused: %v", err)
	}

	moved := map[string]bool{}
	for _, change := range changes {
		moved[change.Field] = true
	}
	if len(moved) != 2 || !moved["name"] || !moved["nodes"] {
		t.Fatalf("the changes are %v", changes)
	}
	if changed.NodeCount() != 2 || changed.UpdatedAt == nil {
		t.Errorf("the changed template is %+v", changed)
	}

	// A patch that says what is already stored moves nothing.
	same := changed.Name
	_, none, err := changed.Changed(work.TemplatePatch{Name: &same}, remindedAt)
	if err != nil {
		t.Fatalf("the change was refused: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("a change that changed nothing reported %v", none)
	}
}

// A deletion is soft and idempotent, and a deleted template is not edited back into existence.
func TestADeletedTemplateStaysDeleted(t *testing.T) {
	template, err := draftTemplate(t, moveTemplate(t))
	if err != nil {
		t.Fatalf("the template was refused: %v", err)
	}

	removed := template.Removed(remindedAt)
	if removed.DeletedAt == nil {
		t.Fatal("the deletion left no stamp")
	}
	if again := removed.Removed(remindedAt.Add(time.Hour)); !again.DeletedAt.Equal(*removed.DeletedAt) {
		t.Error("deleting it twice moved the stamp")
	}

	renamed := "Move house again"
	if _, _, err := removed.Changed(work.TemplatePatch{Name: &renamed}, remindedAt); err == nil {
		t.Fatal("a deleted template was edited")
	}
}
