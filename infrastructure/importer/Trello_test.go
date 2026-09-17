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

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func boardSource(t *testing.T) repository.Source {
	t.Helper()
	raw, err := os.ReadFile("testdata/trello-board.json")
	if err != nil {
		t.Fatal(err)
	}
	return repository.Source{
		Content: strings.NewReader(string(raw)), Hub: hub, Digest: digest,
		Now: time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC), Actor: actor, Zone: "Europe/Berlin", Language: "en",
	}
}

func titled(records []archive.Record, title string) map[string]any {
	for _, record := range records {
		if record.Data["title"] == title || record.Data["name"] == title {
			return record.Data
		}
	}
	return nil
}

func TestABoardBecomesACollectionWithItsListsCardsLabelsChecklistsAndComments(t *testing.T) {
	result, err := Trello{}.Convert(context.Background(), boardSource(t))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for entity, records := range result.Records {
		counts[entity] = len(records)
	}
	// Four lists, four labels, four cards with a title, one work package per checklist of the
	// card with three, five check items, two comments; the card without a title is refused.
	want := map[string]int{"containers": 1, "buckets": 4, "labels": 4, "work_items": 4 + 3 + 5, "item_labels": 4, "comments": 2}
	for entity, n := range want {
		if counts[entity] != n {
			t.Errorf("%s: %d records, want %d (all: %v)", entity, counts[entity], n, counts)
		}
	}
	if len(result.Refused) != 1 || result.Refused[0].Code != domain.CodeRowTitleMissing {
		t.Errorf("refused = %+v", result.Refused)
	}
	if result.Unmapped["members"] != 3 {
		t.Errorf("unmapped members = %v", result.Unmapped)
	}

	collection := result.Records["containers"][0].Data
	if collection["name"] != "Garden season" || collection["parent_id"] != hub.String() {
		t.Errorf("collection = %v", collection)
	}

	// Lists in their position order, the closed one included, Done the done bucket.
	buckets := result.Records["buckets"]
	var names []string
	for _, bucket := range buckets {
		names = append(names, bucket.Data["name"].(string))
	}
	if strings.Join(names, ",") != "To do,Doing,Done,Old ideas" {
		t.Errorf("buckets = %v", names)
	}
	if titled(buckets, "Done")["is_done_bucket"] != true || titled(buckets, "Doing")["is_done_bucket"] != false {
		t.Errorf("done bucket: %v", buckets)
	}

	// Labels: the colour mapped onto the nearest token, a nameless one named after its colour,
	// a colourless one hashed by name.
	labels := result.Records["labels"]
	if got := titled(labels, "Urgent")["color_token"]; got != "red" {
		t.Errorf("Urgent token = %v", got)
	}
	if got := titled(labels, "Shopping")["color_token"]; got != "green" {
		t.Errorf("Shopping token = %v", got)
	}
	if got := titled(labels, "sky")["color_token"]; got != "blue" {
		t.Errorf("sky token = %v", got)
	}
	if got := titled(labels, "Someday")["color_token"]; !shared.IsLabelToken(got.(string)) {
		t.Errorf("Someday token = %v", got)
	}

	items := result.Records["work_items"]
	seeds := titled(items, "Order seeds")
	if seeds["due_at"] != "2026-03-01T09:00:00Z" || seeds["is_completed"] != false || seeds["bucket_id"] != titled(buckets, "To do")["id"] {
		t.Errorf("seeds = %v", seeds)
	}
	// The attachment is a line in the notes, never fetched.
	if notes := seeds["notes"].(string); !strings.HasSuffix(notes, "seed-catalogue.pdf: https://trello.com/1/cards/crd000000000000000000001/attachments/att000000000000000000001/download/seed-catalogue.pdf") ||
		!strings.HasPrefix(notes, "Tomatoes, beans, and the **purple** carrots again.\n\n") {
		t.Errorf("seeds notes = %q", notes)
	}
	// One checklist: its items straight under the card as activities.
	tomatoes := titled(items, "Tomatoes")
	if tomatoes["parent_id"] != seeds["id"] || tomatoes["type"] != "ACTIVITY" || tomatoes["is_completed"] != true {
		t.Errorf("tomatoes = %v", tomatoes)
	}
	beans := titled(items, "Beans")
	if beans["is_completed"] != false || beans["depth"] != 1 {
		t.Errorf("beans = %v", beans)
	}

	// Three checklists: three work packages in their order, the items under them, an empty
	// checklist still a work package.
	beds := titled(items, "Build the raised beds")
	var packages []string
	for _, item := range items {
		if item.Data["parent_id"] == beds["id"] {
			packages = append(packages, item.Data["title"].(string))
			if item.Data["type"] != "WORK_PACKAGE" {
				t.Errorf("%s is %v", item.Data["title"], item.Data["type"])
			}
		}
	}
	if strings.Join(packages, ",") != "Bed one,Bed two,Bed three" {
		t.Errorf("packages = %v", packages)
	}
	corners := titled(items, "Screw the corners")
	if corners["parent_id"] != titled(items, "Bed one")["id"] || corners["depth"] != 2 {
		t.Errorf("corners = %v", corners)
	}
	// The same step name under two checklists is two entries.
	boards := 0
	for _, item := range items {
		if item.Data["title"] == "Cut the boards" {
			boards++
		}
	}
	if boards != 2 {
		t.Errorf("cut the boards: %d", boards)
	}

	// A met due date completes the card at its last activity.
	fence := titled(items, "Fix the fence")
	if fence["is_completed"] != true || fence["completed_at"] != "2026-02-16T17:45:00Z" || fence["completed_by"] != actor.String() {
		t.Errorf("fence = %v", fence)
	}
	// A closed card is archived, not dropped, and lands in its (closed) list.
	pond := titled(items, "Pond?")
	if pond["archived_at"] != "2026-09-16T10:00:00Z" || pond["bucket_id"] != titled(buckets, "Old ideas")["id"] {
		t.Errorf("pond = %v", pond)
	}

	// Comments oldest first, by the importing person, the commenter named in the text.
	comments := result.Records["comments"]
	if comments[0].Data["body"] != "Alex Example: Catalogue attached." || comments[0].Data["created_at"] != "2026-02-19T07:30:00Z" ||
		comments[0].Data["author_id"] != actor.String() || comments[0].Data["item_id"] != seeds["id"] {
		t.Errorf("first comment = %v", comments[0].Data)
	}
	if comments[1].Data["body"] != "Sam Sample: The purple ones were a hit last year." {
		t.Errorf("second comment = %v", comments[1].Data)
	}
}

func TestTheSameBoardTwiceIsTheSameIdentities(t *testing.T) {
	first, err := Trello{}.Convert(context.Background(), boardSource(t))
	if err != nil {
		t.Fatal(err)
	}
	again := boardSource(t)
	again.Digest = "another-export-of-the-same-board"
	second, err := Trello{}.Convert(context.Background(), again)
	if err != nil {
		t.Fatal(err)
	}
	for entity, records := range first.Records {
		for i, record := range records {
			if second.Records[entity][i].ID != record.ID {
				t.Errorf("%s[%d]: %s then %s", entity, i, record.ID, second.Records[entity][i].ID)
			}
		}
	}
}

func TestAFileThatIsNotABoardIsRefused(t *testing.T) {
	for name, content := range map[string]string{
		"csv":        "title,notes\nA,b\n",
		"empty":      "{}",
		"no board":   `{"id":"x"}`,
		"other json": `{"items":[{"title":"a"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			src := boardSource(t)
			src.Content = strings.NewReader(content)
			_, err := Trello{}.Convert(context.Background(), src)
			if !errors.Is(err, shared.ErrValidation) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestAnUnreadableDueDateRefusesTheCard(t *testing.T) {
	src := boardSource(t)
	src.Content = strings.NewReader(`{"id":"b","name":"B","lists":[{"id":"l","name":"L"}],
		"cards":[{"id":"c","name":"Card","idList":"l","due":"yesterday"},{"id":"d","name":"Other","idList":"l"}]}`)
	result, err := Trello{}.Convert(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Refused) != 1 || result.Refused[0].Code != domain.CodeRowDateInvalid || result.Refused[0].Row != 1 {
		t.Errorf("refused = %+v", result.Refused)
	}
	if len(result.Records["work_items"]) != 1 {
		t.Errorf("items = %v", result.Records["work_items"])
	}
}

func TestTokenNamedMapsTrellosColoursOntoTheTen(t *testing.T) {
	for colour, want := range map[string]string{
		"red": "red", "green_dark": "green", "yellow": "lime", "orange_light": "orange", "purple": "violet",
		"blue": "blue", "sky": "blue", "lime": "lime", "pink": "magenta", "black": "slate", "": "",
	} {
		got := tokenNamed(colour, "Name")
		if want == "" {
			if !shared.IsLabelToken(got) {
				t.Errorf("%q: %q is no token", colour, got)
			}
			continue
		}
		if got != want {
			t.Errorf("%q = %q, want %q", colour, got, want)
		}
	}
}
