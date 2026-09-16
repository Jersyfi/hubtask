// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func fixtureSource(t *testing.T, name string) repository.Source {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return repository.Source{
		Content: strings.NewReader(string(raw)), Hub: hub, Digest: digest,
		Now: time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC), Actor: actor, Zone: "Europe/Berlin", Language: "en",
	}
}

func TestTakeoutBecomesOneCollectionPerListWithItsNesting(t *testing.T) {
	result, err := GoogleTasks{}.Convert(context.Background(), fixtureSource(t, "google-tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for entity, records := range result.Records {
		counts[entity] = len(records)
	}
	// Two lists; four items with a title that were not deleted; the untitled one refused.
	if counts["containers"] != 2 || counts["work_items"] != 4 || counts["buckets"] != 0 {
		t.Errorf("counts = %v", counts)
	}
	if len(result.Refused) != 1 || result.Refused[0].Code != domain.CodeRowTitleMissing || result.Refused[0].Row != 6 {
		t.Errorf("refused = %+v", result.Refused)
	}
	lists := result.Records["containers"]
	if lists[0].Data["name"] != "My Tasks" || lists[1].Data["name"] != "Groceries" || lists[0].Data["parent_id"] != hub.String() {
		t.Errorf("lists = %v %v", lists[0].Data, lists[1].Data)
	}

	items := result.Records["work_items"]
	passport := titled(items, "Renew the passport")
	// A Google due date is a date: that day, all day, in the importing person's zone.
	if passport["due_date_only"] != true || passport["due_time_zone"] != "Europe/Berlin" || passport["due_at"] != "2026-10-14T22:00:00Z" {
		t.Errorf("passport due = %v %v %v", passport["due_at"], passport["due_date_only"], passport["due_time_zone"])
	}
	if passport["notes"] != "Photos first.\n\nAppointment confirmation: https://mail.google.com/mail/#all/18f0" || passport["created_at"] != "2026-09-01T09:15:00Z" {
		t.Errorf("passport = %v", passport)
	}
	// Children under their parent, in position order, whatever order the file listed them in;
	// a hidden completed one is still one.
	var children []string
	for _, item := range items {
		if item.Data["parent_id"] == passport["id"] {
			children = append(children, item.Data["title"].(string))
		}
	}
	if strings.Join(children, ",") != "Take the photos,Book the slot" {
		t.Errorf("children = %v", children)
	}
	slot := titled(items, "Book the slot")
	if slot["is_completed"] != true || slot["completed_at"] != "2026-09-03T10:00:00Z" || slot["depth"] != 1 {
		t.Errorf("slot = %v", slot)
	}
	// The work under a task, not a second task: `TASK` and a parent are what the model refuses
	// to hold together, and the first e2e import met the database saying so.
	if slot["type"] != "WORK_PACKAGE" || passport["type"] != "TASK" {
		t.Errorf("slot type = %v, passport type = %v", slot["type"], passport["type"])
	}
	if titled(items, "Old and gone") != nil {
		t.Error("a deleted item is not imported")
	}
	if milk := titled(items, "Oat milk"); milk["collection_id"] != lists[1].Data["id"] {
		t.Errorf("milk = %v", milk)
	}
}

func TestTheWrongFileForGoogleTasksIsRefusedByName(t *testing.T) {
	for name, content := range map[string]string{
		"trello":  `{"id":"b","name":"B","lists":[{"id":"l","name":"L"}],"cards":[]}`,
		"graph":   `{"value":[{"id":"l","displayName":"Tasks","tasks":[]}]}`,
		"csv":     "title,notes\nA,b\n",
		"nothing": `{"items":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			src := fixtureSource(t, "google-tasks.json")
			src.Content = strings.NewReader(content)
			_, err := GoogleTasks{}.Convert(context.Background(), src)
			if !errors.Is(err, shared.ErrValidation) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestATakeoutWithoutTheTopLevelKindIsStillRecognisedByItsLists(t *testing.T) {
	src := fixtureSource(t, "google-tasks.json")
	src.Content = strings.NewReader(`{"items":[{"kind":"tasks#taskList","id":"l","title":"","items":[{"id":"t","title":"One","status":"needsAction","due":"not a date"}]}]}`)
	result, err := GoogleTasks{}.Convert(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if result.Records["containers"][0].Data["name"] != "Tasks" {
		t.Errorf("an untitled list is named: %v", result.Records["containers"][0].Data)
	}
	if len(result.Refused) != 1 || result.Refused[0].Code != domain.CodeRowDateInvalid {
		t.Errorf("refused = %+v", result.Refused)
	}
}
