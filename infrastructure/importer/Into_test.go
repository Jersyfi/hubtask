// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
)

// Every converter into a collection of the hub rather than as collections under it.

var (
	target         = shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")
	existingDoing  = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	existingDocs   = shared.MustParseID("0192f000-0000-7000-8000-0000000000d2")
	existingUrgent = shared.MustParseID("0192f000-0000-7000-8000-0000000000d3")
)

// into points a source at the target collection, which already holds the column *doing* and the
// labels *DOCS* and *urgent*, and ranks after keys of its own.
func into(t *testing.T, src repository.Source) (repository.Source, string) {
	t.Helper()
	last, err := domainservice.OrderKeyAfter("")
	if err != nil {
		t.Fatal(err)
	}
	last, err = domainservice.OrderKeyAfter(last)
	if err != nil {
		t.Fatal(err)
	}
	src.Into = &repository.Target{
		Collection:      target,
		Buckets:         []repository.Named{{ID: existingDoing, Name: "doing"}},
		Labels:          []repository.Named{{ID: existingDocs, Name: "DOCS"}, {ID: existingUrgent, Name: "urgent"}},
		LastBucketOrder: last, LastEntryOrder: last,
	}
	return src, last
}

// landsInTarget checks what every converter owes a target: no collection of its own, and every
// entry, column and label in the target.
func landsInTarget(t *testing.T, records map[string][]archive.Record) {
	t.Helper()
	if n := len(records["containers"]); n != 0 {
		t.Errorf("%d collections written, want none", n)
	}
	for _, entity := range []string{"work_items", "buckets", "labels"} {
		for _, record := range records[entity] {
			if record.Data["collection_id"] != target.String() {
				t.Errorf("%s %v lands in %v", entity, record.Data["title"], record.Data["collection_id"])
			}
		}
	}
	if len(records["work_items"]) == 0 {
		t.Error("nothing landed")
	}
}

// labelsOf answers the names of the labels linked to the entry with that title, the target's own
// by their identities.
func labelsOf(records map[string][]archive.Record, title string) []string {
	names := map[string]string{existingDocs.String(): "DOCS", existingUrgent.String(): "urgent"}
	for _, label := range records["labels"] {
		names[label.ID] = label.Data["name"].(string)
	}
	item := titled(records["work_items"], title)
	if item == nil {
		return nil
	}
	var out []string
	for _, link := range records["item_labels"] {
		if link.Data["item_id"] == item["id"] {
			out = append(out, names[link.Data["label_id"].(string)])
		}
	}
	return out
}

func TestACsvIntoACollectionMeetsItsColumnsAndLabelsByNameAndRanksAfterIt(t *testing.T) {
	file := "title,labels,bucket,parent,collection\n" +
		"Write the reference,docs;api,Doing,,Work\n" +
		"Draw the diagram,,Done,Write the reference,Work\n" +
		"Buy milk,,done,,Home\n"
	src, last := into(t, source(t, file, nil))
	result, err := CSV{}.Convert(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	landsInTarget(t, result.Records)

	// *Doing* is the target's *doing*; *Done* and *done* are one new column, after the target's.
	buckets := result.Records["buckets"]
	if len(buckets) != 1 || buckets[0].Data["name"] != "Done" || buckets[0].Data["order_key"].(string) <= last {
		t.Fatalf("buckets = %v", buckets)
	}
	reference := titled(result.Records["work_items"], "Write the reference")
	if reference["bucket_id"] != existingDoing.String() {
		t.Errorf("the reference is in %v, want the target's own column", reference["bucket_id"])
	}
	if milk := titled(result.Records["work_items"], "Buy milk"); milk["bucket_id"] != buckets[0].ID {
		t.Errorf("*done* is *Done*: %v", milk["bucket_id"])
	}
	if reference["order_key"].(string) <= last {
		t.Errorf("a top-level entry ranks after the target's: %v", reference["order_key"])
	}

	// The row's collection is a label of the top-level entry; *docs* is the target's *DOCS*.
	if got := strings.Join(labelsOf(result.Records, "Write the reference"), ","); got != "Work,DOCS,api" {
		t.Errorf("the reference's labels = %s", got)
	}
	if got := strings.Join(labelsOf(result.Records, "Buy milk"), ","); got != "Home" {
		t.Errorf("milk's labels = %s", got)
	}
	if got := labelsOf(result.Records, "Draw the diagram"); len(got) != 0 {
		t.Errorf("the work under an entry comes with it and carries no list label: %v", got)
	}

	// The same file twice is the same identities; into the hub, other ones.
	again, _ := into(t, source(t, file, nil))
	second, err := CSV{}.Convert(context.Background(), again)
	if err != nil {
		t.Fatal(err)
	}
	if second.Records["work_items"][0].ID != result.Records["work_items"][0].ID {
		t.Error("the same file into the same collection derives the same identities")
	}
	hubResult, err := CSV{}.Convert(context.Background(), source(t, file, nil))
	if err != nil {
		t.Fatal(err)
	}
	if hubResult.Records["work_items"][0].ID == result.Records["work_items"][0].ID {
		t.Error("the same file under the hub is another set of rows")
	}
	if n := len(hubResult.Records["containers"]); n != 2 {
		t.Errorf("under the hub the collection column still makes %d collections, want 2", n)
	}
	for _, label := range hubResult.Records["labels"] {
		if label.Data["name"] == "Work" || label.Data["name"] == "Home" {
			t.Error("under the hub a collection is no label")
		}
	}
}

func TestABoardIntoACollectionBringsItsListsAsColumnsAndNoListLabels(t *testing.T) {
	src, _ := into(t, boardSource(t))
	result, err := Trello{}.Convert(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	landsInTarget(t, result.Records)
	// Four lists: *Doing* is the target's, the other three are new columns.
	names := []string{}
	for _, bucket := range result.Records["buckets"] {
		names = append(names, bucket.Data["name"].(string))
	}
	if strings.Join(names, ",") != "To do,Done,Old ideas" {
		t.Errorf("columns = %v", names)
	}
	// *Urgent* is the target's *urgent*; the board's other labels are new.
	for _, label := range result.Records["labels"] {
		if strings.EqualFold(label.Data["name"].(string), "urgent") {
			t.Errorf("a second *urgent*: %v", label.Data)
		}
	}
	urgent := false
	for _, link := range result.Records["item_labels"] {
		urgent = urgent || link.Data["label_id"] == existingUrgent.String()
	}
	if !urgent {
		t.Error("a card labelled *Urgent* carries the target's *urgent*")
	}
}

func TestSeveralGoogleTasksListsIntoACollectionAreALabelEach(t *testing.T) {
	src, _ := into(t, fixtureSource(t, "google-tasks.json"))
	result, err := GoogleTasks{}.Convert(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	landsInTarget(t, result.Records)
	if got := strings.Join(labelsOf(result.Records, "Renew the passport"), ","); got != "My Tasks" {
		t.Errorf("passport = %s", got)
	}
	if got := strings.Join(labelsOf(result.Records, "Oat milk"), ","); got != "Groceries" {
		t.Errorf("oat milk = %s", got)
	}
	if got := labelsOf(result.Records, "Book the slot"); len(got) != 0 {
		t.Errorf("a subtask comes with its task and carries no list label: %v", got)
	}

	// One list is the collection's own: no label.
	single := `{"kind":"tasks#taskLists","items":[{"kind":"tasks#taskList","id":"only","title":"Only",` +
		`"items":[{"kind":"tasks#task","id":"t1","title":"Water the plants","status":"needsAction"}]}]}`
	one := source(t, single, nil)
	one, _ = into(t, one)
	alone, err := GoogleTasks{}.Convert(context.Background(), one)
	if err != nil {
		t.Fatal(err)
	}
	landsInTarget(t, alone.Records)
	if len(alone.Records["labels"]) != 0 || len(alone.Records["item_labels"]) != 0 {
		t.Errorf("a single list carries no label: %v", alone.Records["labels"])
	}
}

func TestSeveralMicrosoftTodoListsIntoACollectionAreALabelEach(t *testing.T) {
	src, _ := into(t, fixtureSource(t, "microsoft-todo.json"))
	result, err := MicrosoftTodo{}.Convert(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	landsInTarget(t, result.Records)
	if got := strings.Join(labelsOf(result.Records, "File the quarterly report"), ","); got != "Tasks" {
		t.Errorf("report = %s", got)
	}
	holiday := 0
	for _, label := range result.Records["labels"] {
		if label.Data["name"] == "Holiday" {
			holiday++
		}
	}
	if holiday != 1 {
		t.Errorf("the list *Holiday* is one label: %v", result.Records["labels"])
	}
}
