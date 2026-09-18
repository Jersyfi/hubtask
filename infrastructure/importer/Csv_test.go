// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	hub    = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	digest = "3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	actor  = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
)

func source(t *testing.T, csv string, mapping map[string]string) repository.Source {
	t.Helper()
	return repository.Source{
		Content: strings.NewReader(csv), Hub: hub, Digest: digest, Mapping: mapping,
		Now: time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC), Actor: actor, Zone: "Europe/Berlin", Language: "de",
	}
}

const sample = `title,notes,due,completed,labels,bucket,parent
Write the reference,"With a comma, and ""quotes""",2026-09-20,no,docs;api,Doing,
Draw the diagram,,2026-09-21 14:30,yes,docs,Done,Write the reference
Ship it,,,,,Done,2
`

func TestACsvBecomesOneCollectionWithItsTree(t *testing.T) {
	result, err := CSV{}.Convert(context.Background(), source(t, sample, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Refused) != 0 {
		t.Fatalf("refused: %+v", result.Refused)
	}
	counts := map[string]int{}
	for entity, records := range result.Records {
		counts[entity] = len(records)
	}
	want := map[string]int{"containers": 1, "buckets": 2, "labels": 2, "work_items": 3, "item_labels": 3}
	for entity, n := range want {
		if counts[entity] != n {
			t.Errorf("%s: %d records, want %d (all: %v)", entity, counts[entity], n, counts)
		}
	}
	collection := result.Records["containers"][0].Data
	if collection["type"] != "COLLECTION" || collection["parent_id"] != hub.String() || collection["name"] != "Imported" {
		t.Errorf("collection = %v", collection)
	}
	// Named after the file where one is known (issue 766): two files are two collections.
	named := source(t, sample, nil)
	named.Name = "errands"
	fromFile, err := CSV{}.Convert(context.Background(), named)
	if err != nil {
		t.Fatal(err)
	}
	if got := fromFile.Records["containers"][0].Data["name"]; got != "errands" {
		t.Errorf("the collection is named %v, want the file's name", got)
	}
	if fromFile.Records["containers"][0].ID == result.Records["containers"][0].ID {
		t.Error("a collection named differently is a different collection")
	}
	items := result.Records["work_items"]
	first, second, third := items[0].Data, items[1].Data, items[2].Data
	if first["title"] != "Write the reference" || first["notes"] != `With a comma, and "quotes"` {
		t.Errorf("first = %v", first)
	}
	// A date without a time is an all-day due date in the actor's zone.
	if first["due_date_only"] != true || first["due_time_zone"] != "Europe/Berlin" || first["due_at"] != "2026-09-19T22:00:00Z" {
		t.Errorf("first due = %v %v %v", first["due_at"], first["due_date_only"], first["due_time_zone"])
	}
	// A date with a time is the instant, read in the actor's zone.
	if second["due_date_only"] != false || second["due_at"] != "2026-09-21T12:30:00Z" || second["is_completed"] != true {
		t.Errorf("second = %v", second)
	}
	// The parent by title, and by row number; the path materialised from the parent's.
	if second["parent_id"] != first["id"] || second["depth"] != 1 || !strings.HasPrefix(second["path"].(string), first["path"].(string)) {
		t.Errorf("second parent = %v %v %v", second["parent_id"], second["depth"], second["path"])
	}
	if third["parent_id"] != first["id"] {
		t.Errorf("a parent by row number: %v", third["parent_id"])
	}
	// Done is the done bucket.
	var done bool
	for _, bucket := range result.Records["buckets"] {
		if bucket.Data["name"] == "Done" && bucket.Data["is_done_bucket"] == true {
			done = true
		}
	}
	if !done {
		t.Error("the Done bucket is the done bucket")
	}
	if first["content_language"] != "de" || first["created_by"] != actor.String() {
		t.Errorf("provenance = %v %v", first["content_language"], first["created_by"])
	}
}

func TestIdentitiesAreDerivedSoTheSameFileLandsTwiceAsOnce(t *testing.T) {
	once, _ := CSV{}.Convert(context.Background(), source(t, sample, nil))
	again, _ := CSV{}.Convert(context.Background(), source(t, sample, nil))
	if once.Records["work_items"][0].ID != again.Records["work_items"][0].ID {
		t.Error("the same row of the same file into the same hub is the same identity")
	}
	other := source(t, sample, nil)
	other.Digest = "another"
	different, _ := CSV{}.Convert(context.Background(), other)
	if once.Records["work_items"][0].ID == different.Records["work_items"][0].ID {
		t.Error("another file is another identity")
	}
	elsewhere := source(t, sample, nil)
	elsewhere.Hub = shared.MustParseID("0192f000-0000-7000-8000-0000000000ab")
	moved, _ := CSV{}.Convert(context.Background(), elsewhere)
	if once.Records["work_items"][0].ID == moved.Records["work_items"][0].ID {
		t.Error("the same file into another hub is another identity")
	}
}

func TestRowsAreRefusedByNumberAndTheRestLands(t *testing.T) {
	csv := "title;due\nGood;2026-09-20\n;2026-09-20\nBad date;yesterday\nOrphan;\n"
	result, err := CSV{}.Convert(context.Background(), source(t, csv, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records["work_items"]) != 2 {
		t.Errorf("two rows land, got %d", len(result.Records["work_items"]))
	}
	if len(result.Refused) != 2 || result.Refused[0].Row != 3 || result.Refused[0].Code != domain.CodeRowTitleMissing || result.Refused[1].Row != 4 || result.Refused[1].Code != domain.CodeRowDateInvalid {
		t.Errorf("refused = %+v", result.Refused)
	}
}

func TestTheMappingAndTheHeaderAndTheRefusals(t *testing.T) {
	csv := "Aufgabe,Fällig\nEins,20.09.2026\n"
	result, err := CSV{}.Convert(context.Background(), source(t, csv, map[string]string{"title": "Aufgabe", "due": "Fällig"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records["work_items"]) != 1 || result.Records["work_items"][0].Data["due_date_only"] != true {
		t.Errorf("mapped = %+v", result.Records["work_items"])
	}
	if _, err := (CSV{}).Convert(context.Background(), source(t, csv, map[string]string{"title": "Nope"})); err == nil {
		t.Error("a mapping naming a column the file lacks is refused")
	}
	if _, err := (CSV{}).Convert(context.Background(), source(t, csv, map[string]string{"colour": "Aufgabe"})); err == nil {
		t.Error("a mapping naming a field that does not exist is refused")
	}
	if _, err := (CSV{}).Convert(context.Background(), source(t, csv, nil)); err == nil {
		t.Error("a header with no title column and no mapping is not a CSV of entries")
	}
	if _, err := (CSV{}).Convert(context.Background(), source(t, "\n\n", nil)); err == nil {
		t.Error("an empty file is refused")
	}
	if _, err := (CSV{}).Convert(context.Background(), source(t, "title\n\xff\xfe", nil)); err == nil {
		t.Error("a file that is not UTF-8 is refused")
	}
}
