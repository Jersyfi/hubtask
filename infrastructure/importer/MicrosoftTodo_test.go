// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestGraphListsBecomeCollectionsWithTheirTasksRemindersAndChecklists(t *testing.T) {
	result, err := MicrosoftTodo{}.Convert(context.Background(), fixtureSource(t, "microsoft-todo.json"))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for entity, records := range result.Records {
		counts[entity] = len(records)
	}
	// Two lists; three tasks and two checklist items; one reminder; the task in a zone nobody
	// knows refused by its zone rather than landing on a guessed day.
	if counts["containers"] != 2 || counts["work_items"] != 5 || counts["reminders"] != 1 {
		t.Errorf("counts = %v", counts)
	}
	if len(result.Refused) != 1 || result.Refused[0].Code != domain.CodeRowZoneUnknown || result.Refused[0].Row != 3 {
		t.Errorf("refused = %+v", result.Refused)
	}

	items := result.Records["work_items"]
	report := titled(items, "File the quarterly report")
	// A Graph due date at midnight in Pacific Standard Time is that day in America/Los_Angeles.
	if report["due_date_only"] != true || report["due_time_zone"] != "America/Los_Angeles" || report["due_at"] != "2026-10-01T07:00:00Z" {
		t.Errorf("report due = %v %v %v", report["due_at"], report["due_date_only"], report["due_time_zone"])
	}
	if report["notes"] != "Numbers from finance first.\n\nQ3 figures: https://example.com/finance/q3" || report["created_at"] != "2026-09-01T16:03:12.5678901Z" {
		t.Errorf("report = %v", report)
	}
	// importance: high is recorded as nothing - there is no field for it to land in.
	for key := range report {
		if strings.Contains(key, "importance") || strings.Contains(key, "priority") {
			t.Errorf("importance landed as %s", key)
		}
	}
	// The reminder, at its moment in its zone.
	reminder := result.Records["reminders"][0].Data
	if reminder["item_id"] != report["id"] || reminder["fire_at"] != "2026-09-30T16:00:00Z" || reminder["offset_spec"] != "ABS:2026-09-30T16:00:00Z" || reminder["state"] != "PENDING" {
		t.Errorf("reminder = %v", reminder)
	}
	// The checklist as activities under the task, the checked one completed when it was.
	finance := titled(items, "Ask finance")
	if finance["parent_id"] != report["id"] || finance["type"] != "ACTIVITY" || finance["is_completed"] != true || finance["completed_at"] != "2026-09-04T12:00:00Z" {
		t.Errorf("finance = %v", finance)
	}
	if summary := titled(items, "Write the summary"); summary["is_completed"] != false {
		t.Errorf("summary = %v", summary)
	}

	// A due date with a time, in a Windows zone, is the instant; completed when Graph says.
	dentist := titled(items, "Call the dentist")
	if dentist["due_date_only"] != false || dentist["due_at"] != "2026-08-22T13:30:00Z" || dentist["is_completed"] != true || dentist["completed_at"] != "2026-08-22T09:12:00Z" {
		t.Errorf("dentist = %v", dentist)
	}

	// The second list's tasks pasted in as the `/tasks` answer, whole.
	pack := titled(items, "Pack")
	if pack == nil || pack["collection_id"] != result.Records["containers"][1].Data["id"] || pack["notes"] != "<p>Sun cream</p>" {
		t.Errorf("pack = %v", pack)
	}
}

func TestTheWrongFileForMicrosoftTodoIsRefusedByName(t *testing.T) {
	for name, content := range map[string]string{
		"takeout":      `{"kind":"tasks#taskLists","items":[{"kind":"tasks#taskList","id":"l","title":"T","items":[]}]}`,
		"trello":       `{"id":"b","name":"B","lists":[{"id":"l","name":"L"}],"cards":[]}`,
		"csv":          "title,notes\nA,b\n",
		"empty value":  `{"value":[]}`,
		"no list name": `{"value":[{"id":"l"}]}`,
		"tasks broken": `{"value":[{"id":"l","displayName":"T","tasks":"nope"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			src := fixtureSource(t, "microsoft-todo.json")
			src.Content = strings.NewReader(content)
			_, err := MicrosoftTodo{}.Convert(context.Background(), src)
			if !errors.Is(err, shared.ErrValidation) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestZoneNamedReadsWindowsAndIanaNames(t *testing.T) {
	for name, want := range map[string]string{
		"Pacific Standard Time": "America/Los_Angeles", "W. Europe Standard Time": "Europe/Berlin", "UTC": "UTC",
		"tzone://Microsoft/Utc": "UTC", "Europe/Berlin": "Europe/Berlin", "": "UTC", "India Standard Time": "Asia/Calcutta",
	} {
		zone, ok := zoneNamed(name)
		if !ok || zone.String() != want {
			t.Errorf("%q = %v %v, want %s", name, zone, ok, want)
		}
	}
	if _, ok := zoneNamed("Atlantis Standard Time"); ok {
		t.Error("an unknown name is not a zone")
	}
}

func TestEveryWindowsZoneLoads(t *testing.T) {
	for windows, iana := range windowsZones {
		if _, ok := zoneNamed(windows); !ok {
			t.Errorf("%s → %s does not load", windows, iana)
		}
	}
}
