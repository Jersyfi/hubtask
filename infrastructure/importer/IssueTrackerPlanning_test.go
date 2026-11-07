// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
)

const planningAnswer = `{
  "names": {"summary": "Summary", "customfield_10010": "Story Points", "customfield_10020": "Sprint"},
  "schema": {
    "customfield_10010": {"type": "number", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:float"},
    "customfield_10020": {"type": "array", "items": "json", "custom": "com.pyxis.greenhopper.jira:gh-sprint"}
  },
  "issues": [
    {"id": "1", "key": "APP-1", "fields": {"project": {"key": "APP", "name": "App"}, "summary": "Sign in",
      "issuetype": {"name": "Story"}, "status": {"id": "10002", "name": "Done", "statusCategory": {"key": "done"}},
      "customfield_10010": 5,
      "customfield_10020": [{"id": 1, "name": "Sprint 1", "state": "closed", "startDate": "2026-03-02T09:00:00.000Z", "endDate": "2026-03-13T17:00:00.000Z", "completeDate": "2026-03-13T16:00:00.000Z", "goal": "Sign in works"}]}},
    {"id": "2", "key": "APP-2", "fields": {"project": {"key": "APP", "name": "App"}, "summary": "Sign out",
      "issuetype": {"name": "Story"}, "status": {"id": "3", "name": "In Progress", "statusCategory": {"key": "indeterminate"}},
      "customfield_10010": 3,
      "customfield_10020": [{"id": 1, "name": "Sprint 1", "state": "closed", "startDate": "2026-03-02T09:00:00.000Z", "endDate": "2026-03-13T17:00:00.000Z"},
                            {"id": 2, "name": "Sprint 2", "state": "active", "startDate": "2026-03-16T09:00:00.000Z", "endDate": "2026-03-27T17:00:00.000Z"}]}},
    {"id": "3", "key": "APP-3", "fields": {"project": {"key": "APP", "name": "App"}, "summary": "Reset password",
      "issuetype": {"name": "Story"}, "status": {"id": "1", "name": "To Do", "statusCategory": {"key": "new"}},
      "customfield_10020": [{"id": 3, "name": "Sprint 3", "state": "future"}]}},
    {"id": "4", "key": "APP-4", "fields": {"project": {"key": "APP", "name": "App"}, "summary": "Review",
      "issuetype": {"name": "Story"}, "status": {"id": "4", "name": "Review", "statusCategory": {"key": "indeterminate"}}}}
  ]
}`

const planningWorkflows = `{"values": [
  {"id": {"name": "Other workflow"}, "statuses": [{"id": "99", "name": "Elsewhere"}], "transitions": []},
  {"id": {"name": "App workflow"}, "statuses": [{"id": "1", "name": "To Do"}, {"id": "3", "name": "In Progress"}, {"id": "4", "name": "Review"}, {"id": "10002", "name": "Done"}],
   "transitions": [
     {"id": "1", "name": "Create", "from": [], "to": "1", "type": "initial"},
     {"id": "11", "name": "Start", "from": ["1"], "to": "3", "type": "directed"},
     {"id": "21", "name": "Ask for review", "from": ["3"], "to": "4", "type": "directed"},
     {"id": "31", "name": "Approve", "from": ["4"], "to": "10002", "type": "directed"},
     {"id": "41", "name": "Reopen", "from": [], "to": "1", "type": "global"},
     {"id": "51", "name": "Archive", "from": ["10002"], "to": "77", "type": "directed"}
   ]}
]}`

const planningBoards = `[{"id": 5, "name": "App board", "type": "scrum", "location": {"projectKey": "APP"},
  "columnConfig": {"columns": [{"name": "Backlog", "statuses": [{"id": "1"}]}, {"name": "Doing", "statuses": [{"id": "3"}]},
    {"name": "Done", "statuses": [{"id": "10002"}]}, {"name": "Check", "statuses": [{"id": "4"}]}]}}]`

func planningZip(t *testing.T, withPlanning bool) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	files := map[string]string{"search/page-1.json": planningAnswer}
	if withPlanning {
		files["workflows.json"] = planningWorkflows
		files["boards/app.json"] = planningBoards
	}
	for name, content := range files {
		member, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// UC-IMP-07 check 1: the tracker's transitions become the workflow's steps - a directed one from
// the statuses it names, a global one from any - its initial transition the initial status; a
// transition to a status not carried is counted.
func TestATrackersTransitionsBecomeTheWorkflowsSteps(t *testing.T) {
	result, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, planningZip(t, true)))
	if err != nil {
		t.Fatal(err)
	}
	published := result.Records["workflows"][0].Data["published"].(map[string]any)
	if published["initial"] != "to_do" {
		t.Errorf("initial = %v", published["initial"])
	}
	var got []string
	for _, step := range published["steps"].([]map[string]any) {
		got = append(got, step["key"].(string)+":"+strings.Join(step["from"].([]string), "|")+">"+step["to"].(string))
	}
	if strings.Join(got, ",") != "start:to_do>in_progress,ask_for_review:in_progress>review,approve:review>done,reopen:>to_do" {
		t.Errorf("steps = %v", got)
	}
	if result.Unmapped["transitions"] != 1 {
		t.Errorf("unmapped = %v", result.Unmapped)
	}

	// Without the workflow answer, every status stays reachable from any other.
	plain, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, planningZip(t, false)))
	if err != nil {
		t.Fatal(err)
	}
	steps := plain.Records["workflows"][0].Data["published"].(map[string]any)["steps"].([]map[string]any)
	if len(steps) != 4 || len(steps[0]["from"].([]string)) != 0 {
		t.Errorf("steps without the answer = %v", steps)
	}
	if len(plain.Records["saved_views"]) != 0 {
		t.Error("no board, no board view")
	}
}

// UC-IMP-07 check 2: a board's columns order the workflow's statuses, and the board becomes a
// board view of the collection by status, shared with its members.
func TestATrackersBoardOrdersTheStatusesAndBecomesABoardView(t *testing.T) {
	result, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, planningZip(t, true)))
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, status := range result.Records["workflows"][0].Data["published"].(map[string]any)["statuses"].([]map[string]any) {
		order = append(order, status["key"].(string))
	}
	if strings.Join(order, ",") != "to_do,in_progress,done,review" {
		t.Errorf("statuses = %v, want the board's column order", order)
	}
	views := result.Records["saved_views"]
	if len(views) != 1 {
		t.Fatalf("views = %v", views)
	}
	view := views[0].Data
	collection := result.Records["containers"][0].Data["id"]
	if view["name"] != "App board" || view["layout"] != "KANBAN" || view["scope_type"] != "COLLECTION" || view["scope_id"] != collection ||
		view["sharing"] != "SCOPE" || view["grouping"].(map[string]any)["field"] != "status" ||
		view["query"].(map[string]any)["scope_container_id"] != collection {
		t.Errorf("view = %v", view)
	}
}

// UC-IMP-07 checks 3 and 4: the issues' sprints become the collection's sprints with their days,
// goal and state, each issue's place in them with its outcome, the story points the entries'
// estimates and a closed sprint's totals.
func TestATrackersSprintsBecomeTheCollectionsSprints(t *testing.T) {
	result, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, planningZip(t, false)))
	if err != nil {
		t.Fatal(err)
	}
	sprints := result.Records["sprints"]
	if len(sprints) != 3 {
		t.Fatalf("sprints = %v", sprints)
	}
	first, second, third := titled(sprints, "Sprint 1"), titled(sprints, "Sprint 2"), titled(sprints, "Sprint 3")
	if first["state"] != "CLOSED" || first["start_on"] != "2026-03-02" || first["end_on"] != "2026-03-13" || first["goal"] != "Sign in works" ||
		first["closed_at"] != "2026-03-13T16:00:00Z" || first["committed_count"] != 2 || first["completed_count"] != 1 ||
		first["committed_value"] != 8.0 || first["completed_value"] != 5.0 {
		t.Errorf("sprint 1 = %v", first)
	}
	if second["state"] != "ACTIVE" || second["closed_at"] != nil || third["state"] != "PLANNED" || third["started_at"] != nil ||
		third["start_on"] != "2026-10-30" || third["end_on"] != "2026-11-12" {
		t.Errorf("sprint 2 = %v, sprint 3 = %v", second, third)
	}
	entry := func(title string) any { return titled(result.Records["work_items"], title)["id"] }
	outcome := map[string]any{}
	for _, record := range result.Records["sprint_entries"] {
		outcome[record.Data["sprint_id"].(string)+"/"+record.Data["entry_id"].(string)] = record.Data["outcome"]
	}
	for key, want := range map[string]any{
		first["id"].(string) + "/" + entry("Sign in").(string):        "DONE",
		first["id"].(string) + "/" + entry("Sign out").(string):       "CARRIED",
		second["id"].(string) + "/" + entry("Sign out").(string):      nil,
		third["id"].(string) + "/" + entry("Reset password").(string): nil,
	} {
		if got, ok := outcome[key]; !ok || got != want {
			t.Errorf("membership %s = %v (%v), want %v", key, got, ok, want)
		}
	}
	if len(outcome) != 4 {
		t.Errorf("memberships = %v", outcome)
	}
	estimates := result.Records["entry_estimates"]
	if len(estimates) != 2 || recordOf(estimates, "entry_id", entry("Sign in"))["value"] != 5.0 {
		t.Errorf("estimates = %v", estimates)
	}
}

// UC-IMP-07 check 4, the bound: into a collection that runs its own sprints, none of the
// imported ones is active.
func TestIntoACollectionNoImportedSprintIsActive(t *testing.T) {
	source := trackerSource(t, planningZip(t, false))
	source.Into = &repository.Target{Collection: hub}
	result, err := IssueTracker{}.Convert(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	for _, sprint := range result.Records["sprints"] {
		if sprint.Data["state"] == "ACTIVE" {
			t.Errorf("sprint %v is active in a collection that runs its own", sprint.Data["name"])
		}
	}
}
