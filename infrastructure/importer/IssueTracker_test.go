// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func trackerSource(t *testing.T, content []byte) repository.Source {
	t.Helper()
	return repository.Source{
		Content: bytes.NewReader(content), Hub: hub, Digest: digest,
		Now: time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC), Actor: actor, Zone: "Europe/Berlin", Language: "en",
	}
}

func trackerFile(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/issue-tracker.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// trackerZip is the answer with one attachment beside it, as the ZIP carries them.
func trackerZip(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string][]byte{
		"search/page-1.json":           trackerFile(t),
		"attachments/7001/trace.txt":   []byte("panic: card declined twice\n"),
		"attachments/9999/unknown.bin": []byte("nobody names me"),
	} {
		member, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func recordOf(records []archive.Record, field string, value any) map[string]any {
	for _, record := range records {
		if record.Data[field] == value {
			return record.Data
		}
	}
	return nil
}

// UC-IMP-04 check 1: a project becomes a collection whose issue types are entry types, whose
// statuses with their categories are a workflow bound to it, and whose entries sit in them.
func TestAProjectBecomesACollectionWithItsTypesAndWorkflow(t *testing.T) {
	result, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, trackerFile(t)))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(result.Records["containers"]); n != 1 {
		t.Fatalf("containers = %d, want one per project", n)
	}
	collection := result.Records["containers"][0].Data
	if collection["name"] != "Web shop" {
		t.Errorf("collection = %v", collection["name"])
	}
	types := map[string]string{}
	for _, record := range result.Records["entry_types"] {
		types[record.Data["key"].(string)] = record.Data["level"].(string)
	}
	want := map[string]string{"epic": "TASK", "bug": "TASK", "story": "TASK", "sub_task": "WORK_PACKAGE"}
	for key, level := range want {
		if types[key] != level {
			t.Errorf("entry type %s at %q, want %s (all %v)", key, types[key], level, types)
		}
	}
	if n := len(result.Records["workflows"]); n != 1 {
		t.Fatalf("workflows = %d", n)
	}
	published := result.Records["workflows"][0].Data["published"].(map[string]any)
	statuses := published["statuses"].([]map[string]any)
	var got []string
	for _, status := range statuses {
		got = append(got, status["key"].(string)+":"+status["category"].(string))
	}
	if strings.Join(got, ",") != "to_do:TODO,in_progress:IN_PROGRESS,done:DONE" || published["initial"] != "to_do" {
		t.Errorf("statuses = %v, initial %v", got, published["initial"])
	}
	scheme := result.Records["workflow_schemes"]
	if len(scheme) != 1 || scheme[0].Data["container_id"] != collection["id"] || scheme[0].Data["workflow_key"] != "imported_shop" {
		t.Errorf("scheme = %+v", scheme)
	}
	bug := titled(result.Records["work_items"], "Card payment fails on retry")
	if bug["entry_type"] != "bug" || bug["status"] != "done" || bug["is_completed"] != true {
		t.Errorf("bug = type %v status %v completed %v", bug["entry_type"], bug["status"], bug["is_completed"])
	}
	if bug["notes"] != "Steps **to reproduce**\n\n- Pay by card" {
		t.Errorf("notes = %q", bug["notes"])
	}
	sub := titled(result.Records["work_items"], "Store the method per account")
	story := titled(result.Records["work_items"], "Remember the last payment method")
	if sub["type"] != "WORK_PACKAGE" || sub["parent_id"] != story["id"] || sub["entry_type"] != "sub_task" {
		t.Errorf("sub-task = %v under %v, type %v", sub["type"], sub["parent_id"], sub["entry_type"])
	}
	// A sub-task whose parent the file does not carry is refused by row, the rest lands.
	if len(result.Refused) != 1 || result.Refused[0].Code != domain.CodeRowParentUnknown || result.Refused[0].Row != 5 {
		t.Errorf("refused = %+v", result.Refused)
	}
}

// UC-IMP-04 check 2: custom fields, priority, components and versions become the collection's
// fields with their values; a field the product has no kind for is counted.
func TestCustomFieldsBecomeFieldsWithTheirValues(t *testing.T) {
	result, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, trackerFile(t)))
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	options := map[string][]string{}
	for _, record := range result.Records["custom_field_definitions"] {
		key := record.Data["key"].(string)
		kinds[key] = record.Data["kind"].(string)
		options[key] = record.Data["options"].([]string)
	}
	want := map[string]string{
		"issue_key": "TEXT", "priority": "SELECT", "components": "MULTI_SELECT", "fix_versions": "MULTI_SELECT",
		"source_assignee": "TEXT", "source_reporter": "TEXT", "story_points": "NUMBER", "customer_tier": "SELECT", "go_live": "DATE",
	}
	for key, kind := range want {
		if kinds[key] != kind {
			t.Errorf("field %s = %q, want %s (all %v)", key, kinds[key], kind, kinds)
		}
	}
	if _, sprint := kinds["sprint"]; sprint {
		t.Error("a sprint has no kind and is not a field")
	}
	if strings.Join(options["priority"], ",") != "High,Medium" {
		t.Errorf("priority options = %v", options["priority"])
	}
	bug := titled(result.Records["work_items"], "Card payment fails on retry")
	values := bug["custom_fields"].(map[string]any)
	refs := bug["custom_field_refs"].(map[string]any)
	if values["issue_key"] != "SHOP-2" || values["priority"] != "High" || values["story_points"] != float64(3) ||
		values["customer_tier"] != "Gold" || values["go_live"] != "2026-05-01" || values["source_assignee"] != "Ada Lovelace" {
		t.Errorf("values = %v", values)
	}
	if components := values["components"].([]string); len(components) != 1 || components[0] != "Checkout" {
		t.Errorf("components = %v", values["components"])
	}
	for key := range values {
		if refs[key] == nil {
			t.Errorf("value %s names no definition", key)
		}
	}
	if result.Unmapped["fields"] != 1 {
		t.Errorf("unmapped = %v", result.Unmapped)
	}
	if n := len(result.Records["item_labels"]); n != 2 {
		t.Errorf("item labels = %d", n)
	}
}

// UC-IMP-04 check 3: the change log becomes the entry's history and the comments its comments,
// each naming the source's author, who is no account here.
func TestTheChangeLogBecomesTheHistory(t *testing.T) {
	result, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, trackerFile(t)))
	if err != nil {
		t.Fatal(err)
	}
	bug := titled(result.Records["work_items"], "Card payment fails on retry")
	entries := result.Records["activity_entries"]
	if len(entries) != 1 {
		t.Fatalf("history = %d entries", len(entries))
	}
	step := entries[0].Data
	set := step["change_set"].(map[string]any)
	if step["verb"] != string(activity.ItemHistoryImported) || step["item_id"] != bug["id"] || step["actor_id"] != actor.String() {
		t.Errorf("step = %v", step)
	}
	if status := set["status"].(map[string]any); status["from"] != "To Do" || status["to"] != "In Progress" {
		t.Errorf("status change = %v", status)
	}
	if by := set["changed_by"].(map[string]any); by["to"] != "Grace Hopper" {
		t.Errorf("changed_by = %v", by)
	}
	if step["occurred_at"] != "2026-03-10T11:00:00Z" {
		t.Errorf("occurred_at = %v", step["occurred_at"])
	}
	comment := result.Records["comments"]
	if len(comment) != 1 || comment[0].Data["body"] != "Ada Lovelace: Reproduced." {
		t.Errorf("comments = %+v", comment)
	}
}

// UC-IMP-04 check 4: links between issues become links of their type; an epic is what the
// issue under it implements; a link to an issue the file does not carry is counted.
func TestIssueLinksAndEpicsBecomeLinks(t *testing.T) {
	result, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, trackerFile(t)))
	if err != nil {
		t.Fatal(err)
	}
	bug := titled(result.Records["work_items"], "Card payment fails on retry")
	story := titled(result.Records["work_items"], "Remember the last payment method")
	epic := titled(result.Records["work_items"], "Checkout redesign")
	links := result.Records["entry_links"]
	if recordOf(links, "link_type", "blocks") == nil || recordOf(links, "link_type", "blocks")["target_id"] != story["id"] {
		t.Errorf("blocks = %+v", links)
	}
	implements := recordOf(links, "link_type", "implements")
	if implements == nil || implements["source_id"] != bug["id"] || implements["target_id"] != epic["id"] {
		t.Errorf("implements = %v", implements)
	}
	if result.Unmapped["links"] != 1 {
		t.Errorf("unmapped = %v", result.Unmapped)
	}
}

// UC-IMP-04 check 5: a ZIP carries the attachments beside the answer: each becomes the entry's
// attachment with its bytes; one the ZIP lacks is counted.
func TestAZipCarriesTheAttachments(t *testing.T) {
	result, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, trackerZip(t)))
	if err != nil {
		t.Fatal(err)
	}
	media := result.Records["media_objects"]
	if len(media) != 1 {
		t.Fatalf("media = %d", len(media))
	}
	object := media[0]
	if object.Data["file_name"] != "trace.txt" || object.Data["status"] != "READY" || object.Data["ref_count"] != 1 ||
		!strings.HasPrefix(object.Data["mime_type"].(string), "text/plain") {
		t.Errorf("object = %v", object.Data)
	}
	if len(object.Blobs) != 1 || string(result.Media[object.Blobs[0].Digest]) != "panic: card declined twice\n" {
		t.Errorf("blobs = %+v", object.Blobs)
	}
	bug := titled(result.Records["work_items"], "Card payment fails on retry")
	attached := result.Records["item_attachments"]
	if len(attached) != 1 || attached[0].Data["item_id"] != bug["id"] || attached[0].Data["media_id"] != object.Data["id"] {
		t.Errorf("attachments = %+v", attached)
	}
	if result.Unmapped["attachments"] != 1 {
		t.Errorf("unmapped = %v", result.Unmapped)
	}
}

// UC-IMP-04 check 6: the same projects twice derive the same identities, so the second import
// creates nothing; a file that is no answer is refused.
func TestTheSameProjectsTwiceAreTheSameRowsAndAForeignFileIsRefused(t *testing.T) {
	first, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, trackerFile(t)))
	if err != nil {
		t.Fatal(err)
	}
	again, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, trackerZip(t)))
	if err != nil {
		t.Fatal(err)
	}
	if first.Records["work_items"][0].ID != again.Records["work_items"][0].ID ||
		first.Records["containers"][0].ID != again.Records["containers"][0].ID {
		t.Error("a fresh export of the same projects derives other identities")
	}
	for _, content := range []string{`{"lists": []}`, `not json`} {
		_, err := IssueTracker{}.Convert(context.Background(), trackerSource(t, []byte(content)))
		if !errors.Is(err, shared.ErrValidation) || shared.AsError(err).DetailCode != domain.CodeFileNotKind {
			t.Errorf("%s: err = %v", content, err)
		}
	}
	_, err = IssueTracker{}.Convert(context.Background(), trackerSource(t, []byte("PK\x03\x04broken")))
	if shared.AsError(err).DetailCode != domain.CodeArchiveUnreadable {
		t.Errorf("a broken ZIP: err = %v", err)
	}
}

// Into a collection that runs its own workflow, the statuses are not carried - only completion.
func TestIntoACollectionTheWorkflowIsNotReplaced(t *testing.T) {
	source := trackerSource(t, trackerFile(t))
	source.Into = &repository.Target{Collection: shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")}
	result, err := IssueTracker{}.Convert(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records["workflows"]) != 0 || len(result.Records["workflow_schemes"]) != 0 || len(result.Records["containers"]) != 0 {
		t.Errorf("into a collection: %d workflows, %d schemes, %d containers",
			len(result.Records["workflows"]), len(result.Records["workflow_schemes"]), len(result.Records["containers"]))
	}
	bug := titled(result.Records["work_items"], "Card payment fails on retry")
	if bug["status"] != nil || bug["is_completed"] != true {
		t.Errorf("bug = status %v completed %v", bug["status"], bug["is_completed"])
	}
}
