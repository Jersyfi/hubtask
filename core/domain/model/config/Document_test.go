// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

func current() Document {
	return Document{
		"types": Section{
			"incident": Block{"level": "TASK", "name": "Incident", "position": float64(1)},
			"problem":  Block{"level": "TASK", "name": "Problem"},
		},
		"modules": Section{"workflows": Block{"switch": "ON"}},
	}
}

func TestResolveReadsADocumentInItsMode(t *testing.T) {
	cases := map[string]struct {
		desired Document
		mode    Mode
		want    []string
	}{
		"merge keeps what it does not name": {
			desired: Document{"types": Section{"incident": Block{"name": "Störung"}}},
			mode:    ModeMerge, want: []string{"CHANGE types.incident name"},
		},
		"merge removes a nil block": {
			desired: Document{"types": Section{"problem": nil}},
			mode:    ModeMerge, want: []string{"REMOVE types.problem"},
		},
		"merge adds a new block": {
			desired: Document{"types": Section{"change": Block{"level": "TASK", "name": "Change"}}},
			mode:    ModeMerge, want: []string{"ADD types.change level,name"},
		},
		"replace states a section whole": {
			desired: Document{"types": Section{"incident": Block{"level": "TASK", "name": "Incident", "position": 1}}},
			mode:    ModeReplace, want: []string{"REMOVE types.problem"},
		},
		"an int and a float that say the same are no change": {
			desired: Document{"types": Section{"incident": Block{"position": 1}}},
			mode:    ModeMerge, want: nil,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			changes := Diff(current(), Resolve(current(), tc.desired, tc.mode))
			var got []string
			for _, change := range changes {
				line := string(change.Op) + " " + change.Path()
				if change.Op != OpRemove {
					line += " " + join(change.Fields)
				}
				got = append(got, line)
			}
			if join(got) != join(tc.want) {
				t.Errorf("changes %v, want %v", got, tc.want)
			}
		})
	}
}

func join(s []string) string {
	out := ""
	for i, item := range s {
		if i > 0 {
			out += ","
		}
		out += item
	}
	return out
}

func TestTheHashDependsOnContentNotOnSpelling(t *testing.T) {
	a := Document{"types": Section{"x": Block{"n": float64(2), "list": []any{"a", "b"}}}}
	b := Document{"types": Section{"x": Block{"list": []string{"a", "b"}, "n": 2}}}
	if Hash(a) != Hash(b) {
		t.Error("two spellings of one document hash apart")
	}
	if PlanHash(Hash(a), a) == PlanHash(Hash(b)+"x", a) {
		t.Error("a plan's hash ignores the version it was planned on")
	}
}

func TestValidKey(t *testing.T) {
	for key, want := range map[string]bool{"incident": true, "it_desk2": true, "Incident": false, "1st": false, "": false, "a-b": false} {
		if ValidKey(key) != want {
			t.Errorf("ValidKey(%q) = %v", key, !want)
		}
	}
}

func TestAnAgentsApplyWaitsByDefaultAndAWorkspaceOnlyTightens(t *testing.T) {
	definition := AgentApplyDefinition()
	if err := definition.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := AgentApplyOf(setting.Resolve(definition)); got != AgentApplyApproval {
		t.Errorf("default %s, want APPROVAL (D-16)", got)
	}
	above := setting.Resolve(definition, setting.Layer{Level: setting.LevelInstance})
	loosen := setting.Entry{Value: string(AgentApplyAllowed), Hold: setting.HoldSet}
	if err := setting.Check(definition, setting.LevelWorkspace, above, loosen); err == nil {
		t.Error("a workspace loosened approval to allowed")
	}
	tighten := setting.Entry{Value: string(AgentApplyNever), Hold: setting.HoldSet}
	if err := setting.Check(definition, setting.LevelWorkspace, above, tighten); err != nil {
		t.Errorf("a workspace could not tighten: %v", err)
	}
}

func TestCountsOf(t *testing.T) {
	counts := CountsOf([]Change{{Op: OpAdd}, {Op: OpAdd}, {Op: OpChange}, {Op: OpRemove}})
	if counts != (Counts{Added: 2, Changed: 1, Removed: 1}) {
		t.Errorf("counts %+v", counts)
	}
}
