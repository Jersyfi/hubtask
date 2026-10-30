// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const projectXML = `<?xml version="1.0" encoding="UTF-8"?>
<Project xmlns="http://schemas.microsoft.com/project">
  <Tasks>
    <Task><UID>0</UID><Name>Project</Name><OutlineLevel>0</OutlineLevel></Task>
    <Task><UID>1</UID><Name>Design</Name><OutlineLevel>1</OutlineLevel>
      <Start>2026-11-02T08:00:00</Start><Finish>2026-11-06T17:00:00</Finish></Task>
    <Task><UID>2</UID><Name>Draft</Name><OutlineLevel>2</OutlineLevel>
      <Start>2026-11-02T08:00:00</Start><Finish>2026-11-03T17:00:00</Finish></Task>
    <Task><UID>3</UID><Name>Build</Name><OutlineLevel>1</OutlineLevel>
      <Start>2026-11-09T08:00:00</Start><Finish>2026-11-20T17:00:00</Finish>
      <PredecessorLink><PredecessorUID>1</PredecessorUID><Type>1</Type><LinkLag>9600</LinkLag><LagFormat>7</LagFormat></PredecessorLink></Task>
    <Task><UID>4</UID><Name>Launch</Name><OutlineLevel>1</OutlineLevel><Milestone>1</Milestone>
      <Start>2026-11-20T17:00:00</Start><Finish>2026-11-20T17:00:00</Finish>
      <PredecessorLink><PredecessorUID>3</PredecessorUID><Type>0</Type><LinkLag>0</LinkLag><LagFormat>7</LagFormat></PredecessorLink></Task>
  </Tasks>
</Project>`

// UC-PLN-09 check 1: a project tool's XML brings its tasks in with their outline, dates,
// milestones and typed, lagged predecessors; the project's own summary task stays out.
func TestAProjectXMLBringsItsTasksAndLinks(t *testing.T) {
	tasks, err := planning.ParseProjectFile(planning.FormatProjectXML, []byte(projectXML))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 4 || tasks[0].Name != "Design" || tasks[1].Level != 2 || !tasks[3].Milestone {
		t.Fatalf("tasks %+v", tasks)
	}
	if tasks[0].Start.String() != "2026-11-02" || tasks[0].Finish.String() != "2026-11-06" {
		t.Errorf("Design runs %v … %v", tasks[0].Start, tasks[0].Finish)
	}
	build := tasks[2].Predecessors
	if len(build) != 1 || build[0].Key != "1" || build[0].Timing.Kind != planning.FinishToStart || build[0].Timing.LagDays != 2 {
		t.Errorf("Build waits on %+v, want Design FS+2", build)
	}
	if launch := tasks[3].Predecessors; len(launch) != 1 || launch[0].Timing.Kind != planning.FinishToFinish {
		t.Errorf("Launch waits on %+v, want Build FF", launch)
	}
}

// UC-PLN-09 check 2: a CSV with a header and the predecessors column in the usual notation.
func TestACSVBringsItsTasksAndPredecessors(t *testing.T) {
	csv := "ID;Name;Start;Finish;Predecessors;Outline Level;Milestone\n" +
		"1;Design;2026-11-02;2026-11-06;;1;\n" +
		"2;Build;2026-11-09;2026-11-20;1FS+2d;1;\n" +
		"3;Test;2026-11-16;2026-11-20;2SS-1d, 1FF;1;\n" +
		"4;Launch;;2026-11-20;3;1;yes\n"
	tasks, err := planning.ParseProjectFile(planning.FormatCSV, []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 4 || !tasks[3].Milestone || tasks[3].Start != nil {
		t.Fatalf("tasks %+v", tasks)
	}
	got := tasks[2].Predecessors
	if len(got) != 2 || got[0].Key != "2" || got[0].Timing.Kind != planning.StartToStart || got[0].Timing.LagDays != -1 ||
		got[1].Key != "1" || got[1].Timing.Kind != planning.FinishToFinish {
		t.Errorf("Test waits on %+v", got)
	}
	if got := tasks[1].Predecessors; len(got) != 1 || got[0].Timing.LagDays != 2 {
		t.Errorf("Build waits on %+v", got)
	}
}

// UC-PLN-09 check 3: a file that cannot be brought in whole is refused with where it went wrong.
func TestAProjectFileThatCannotBeReadIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name, format, content, code string
	}{
		{"no name column", "CSV", "ID,Start\n1,2026-11-02\n", "planning.import_name_column_missing"},
		{"an unknown predecessor", "CSV", "ID,Name,Predecessors\n1,Design,7\n", "planning.import_predecessor_unknown"},
		{"a date that is none", "CSV", "Name,Start\nDesign,soon\n", "planning.import_date_invalid"},
		{"not XML", "MSPROJECT_XML", "<Project><Tasks>", "planning.import_not_xml"},
		{"empty", "CSV", "Name\n", "planning.import_empty"},
		{"an unknown format", "MPP", "x", "planning.import_format_unknown"},
		{"too many", "CSV", "Name\n" + strings.Repeat("a\n", planning.MaxImportedEntries+1), "planning.import_too_many_entries"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := planning.ParseProjectFile(planning.FileFormat(tc.format), []byte(tc.content))
			var typed *shared.Error
			if !errors.As(err, &typed) || typed.DetailCode != tc.code {
				t.Fatalf("err %v, want %s", err, tc.code)
			}
		})
	}
}
