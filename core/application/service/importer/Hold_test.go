// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	"github.com/Jersyfi/hubtask/core/application/service/importer"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// An import into a collection is held to its types' forms, and what a form refuses is reported by
// entry (UC-IMP-04 check 8).

// typedEntries answers entries of a type into the target: a story its form refuses, with a
// sub-entry, a label and an attachment of its own; a bug whose type takes no import; a story that
// meets the form, sharing nothing with the refused one but the label.
type typedEntries struct{}

func (typedEntries) Kind() domain.Kind { return domain.KindCSV }
func (typedEntries) Convert(_ context.Context, source repository.Source) (repository.Result, error) {
	into := source.Into.Collection.String()
	item := func(id, parent, entryType, title string, fields map[string]any) archive.Record {
		path := "/" + id + "/"
		if parent != "" {
			path = "/" + parent + path
		}
		data := map[string]any{
			"id": id, "collection_id": into, "type": "TASK", "entry_type": entryType, "title": title,
			"path": path, "depth": 0, "order_key": "a", "created_by": source.Actor.String(), "version": 1,
			"custom_fields": fields,
		}
		if parent != "" {
			data["type"], data["parent_id"], data["depth"] = "WORK_PACKAGE", parent, 1
		}
		return archive.Record{ID: id, Op: archive.OpUpsert, UpdatedAt: source.Now, Data: data}
	}
	const short, under, closed, fine = "0192f000-0000-7000-8000-0000000000e1", "0192f000-0000-7000-8000-0000000000e2",
		"0192f000-0000-7000-8000-0000000000e3", "0192f000-0000-7000-8000-0000000000e4"
	const label, comment = "0192f000-0000-7000-8000-0000000000e5", "0192f000-0000-7000-8000-0000000000e6"
	return repository.Result{
		Records: map[string][]archive.Record{
			"work_items": {
				item(short, "", "story", "short", map[string]any{}),
				item(under, short, "", "under it", nil),
				item(closed, "", "bug", "closed", nil),
				item(fine, "", "story", "fine", map[string]any{"severity": "high"}),
			},
			// The fake write port keys every row by `id`; the real one keys a link by both ends.
			"item_labels": {
				{ID: short + "/" + label, Op: archive.OpUpsert, UpdatedAt: source.Now, Data: map[string]any{"id": short + "/" + label, "item_id": short, "label_id": label}},
				{ID: fine + "/" + label, Op: archive.OpUpsert, UpdatedAt: source.Now, Data: map[string]any{"id": fine + "/" + label, "item_id": fine, "label_id": label}},
			},
			"comments": {
				{ID: comment, Op: archive.OpUpsert, UpdatedAt: source.Now, Data: map[string]any{"id": comment, "item_id": short, "body": "x"}},
			},
		},
		Rows: map[string]int{short: 2, under: 2, closed: 3, fine: 4},
	}, nil
}

// storyForm refuses a story without a severity and closes the import door to bugs, as the form
// gate answers.
type storyForm struct {
	asked  []work.WorkItem
	actors []appshared.ActorContext
	fail   error
}

func (f *storyForm) HoldImported(_ context.Context, actor appshared.ActorContext, collection work.Container, item work.WorkItem) error {
	f.asked = append(f.asked, item)
	f.actors = append(f.actors, actor)
	if f.fail != nil {
		return f.fail
	}
	if collection.ID != collectionID {
		return errors.New("held against another collection")
	}
	switch {
	case item.EntryType == "bug":
		return shared.ErrForbidden.WithDetail("forms.channel_closed")
	case item.EntryType == "story" && item.CustomFields["severity"] == nil:
		return shared.ErrValidation.WithDetail("forms.refused").WithFields(
			shared.FieldError{Path: "/custom_fields/severity", Code: "fields.value_required", Params: map[string]string{"key": "custom_fields.severity"}},
			shared.FieldError{Path: "", Code: "forms.validation_failed", Params: map[string]string{"validation": "estimate_set"}},
		)
	}
	return nil
}

func intoRunner(t *testing.T, form *storyForm) (importer.Runner, *runs, *importRepo) {
	t.Helper()
	run := newRuns()
	pending := pendingRun()
	pending.CollectionID = collectionID
	run.rows[runID] = pending
	object := readyObject()
	r, landed, _, _ := runner(run, object, map[string][]byte{object.StorageKey: []byte("x")}, converter{})
	r.Converters = map[domain.Kind]repository.Converter{domain.KindCSV: typedEntries{}}
	r.Containers = containers{rows: map[shared.ID]work.Container{collectionID: {ID: collectionID, Type: work.ContainerCollection, ParentID: hubID}}}
	r.Buckets, r.Labels, r.Items = buckets{}, labels{}, &itemOrders{}
	r.Forms = form
	landed.rows["container"][collectionID.String()] = map[string]any{"id": collectionID.String(), "type": "COLLECTION", "parent_id": hubID.String()}
	return r, run, landed
}

func TestAnImportIntoACollectionLandsOnlyWhatItsTypesFormsTakeAndReportsTheRestByEntry(t *testing.T) {
	form := &storyForm{}
	r, run, landed := intoRunner(t, form)
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err != nil {
		t.Fatal(err)
	}
	finished := run.rows[runID]
	if finished.Status != domain.StatusSucceeded {
		t.Fatalf("run = %+v", finished)
	}
	// The one that meets its form lands; nothing else of the entries does.
	items := landed.rows["work_item"]
	if len(items) != 1 || items["0192f000-0000-7000-8000-0000000000e4"] == nil {
		t.Errorf("landed entries = %v", items)
	}
	// Nothing that names a refused entry lands: its label link and its comment - taken out, not
	// withheld as orphans.
	if len(landed.rows["item_label"]) != 1 || len(landed.rows["comment"]) != 0 || finished.Report.Withheld["orphaned"] != 0 {
		t.Errorf("landed beside = labels %v, comments %v", landed.rows["item_label"], landed.rows["comment"])
	}
	want := []domain.Refusal{
		{Row: 2, Code: domain.CodeRowFormRefused, Fields: []string{"custom_fields.severity", "estimate_set"}},
		{Row: 2, Code: domain.CodeRowParentRefused},
		{Row: 3, Code: domain.CodeRowChannelClosed},
	}
	if len(finished.Refused) != len(want) {
		t.Fatalf("refused = %+v", finished.Refused)
	}
	for i, refusal := range want {
		got := finished.Refused[i]
		if got.Row != refusal.Row || got.Code != refusal.Code || len(got.Fields) != len(refusal.Fields) {
			t.Errorf("refused[%d] = %+v, want %+v", i, got, refusal)
			continue
		}
		for j := range refusal.Fields {
			if got.Fields[j] != refusal.Fields[j] {
				t.Errorf("refused[%d].Fields = %v, want %v", i, got.Fields, refusal.Fields)
			}
		}
	}
	// The run says it per entry, with the form's keys.
	answered, _ := importer.RunOutput(finished)["refused"].([]usecase.Output)
	if fields, _ := answered[0]["fields"].([]string); len(answered) != 3 || len(fields) != 2 || answered[1]["fields"] != nil {
		t.Errorf("answered = %v", answered)
	}
	// Each entry is asked as the importing person through the import door, with what it carries.
	if len(form.asked) != 3 {
		t.Fatalf("asked %d entries, want 3 (the one under a refused entry is not asked)", len(form.asked))
	}
	for _, actor := range form.actors {
		if actor.Channel != string(work.ChannelImport) || actor.AccountID != accountID || actor.TenantID != tenantID {
			t.Errorf("actor = %+v", actor)
		}
	}
	if fine := form.asked[2]; fine.Title != "fine" || fine.CustomFields["severity"] != "high" || fine.CollectionID != collectionID {
		t.Errorf("the entry as the form read it = %+v", fine)
	}
}

func TestAnImportUnderTheHubIsNotHeldToAForm(t *testing.T) {
	form := &storyForm{}
	run := newRuns()
	run.rows[runID] = pendingRun()
	object := readyObject()
	r, _, _, _ := runner(run, object, map[string][]byte{object.StorageKey: []byte("one\n")}, converter{})
	r.Forms = form
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err != nil {
		t.Fatal(err)
	}
	if run.rows[runID].Status != domain.StatusSucceeded || len(form.asked) != 0 {
		t.Errorf("an import that creates its collections brings its types with an empty form: asked %d", len(form.asked))
	}
}

func TestAFormThatCannotBeAskedRetriesTheImportRatherThanLandingIt(t *testing.T) {
	form := &storyForm{fail: shared.ErrUnavailable.WithDetail("db.unavailable")}
	r, run, landed := intoRunner(t, form)
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err == nil {
		t.Fatal("the database away is the job's retry")
	}
	if got := run.rows[runID]; got.Status != domain.StatusRunning || len(landed.rows["work_item"]) != 0 {
		t.Errorf("run = %+v, landed %v", got, landed.rows["work_item"])
	}
}

func TestAnAttachmentOfARefusedEntryTakesItsObjectOnlyWhereNothingElseUsesIt(t *testing.T) {
	const refused, kept, alone, common = "e1", "e2", "m1", "m2"
	record := func(id string, data map[string]any) archive.Record { return archive.Record{ID: id, Data: data} }
	result := repository.Result{
		Records: map[string][]archive.Record{
			"work_items": {record(refused, map[string]any{}), record(kept, map[string]any{})},
			"item_attachments": {
				record(refused+"/"+alone, map[string]any{"item_id": refused, "media_id": alone}),
				record(refused+"/"+common, map[string]any{"item_id": refused, "media_id": common}),
				record(kept+"/"+common, map[string]any{"item_id": kept, "media_id": common}),
			},
			"media_objects": {
				{ID: alone, Data: map[string]any{"ref_count": 1}, Blobs: []archive.Blob{{Digest: "d-alone"}}},
				{ID: common, Data: map[string]any{"ref_count": 2}, Blobs: []archive.Blob{{Digest: "d-shared"}}},
			},
		},
		Media: map[string][]byte{"d-alone": []byte("a"), "d-shared": []byte("s")},
	}
	importer.WithoutDropped(&result, map[string]bool{refused: true})
	if got := result.Records["work_items"]; len(got) != 1 || got[0].ID != kept {
		t.Errorf("entries = %+v", got)
	}
	if got := result.Records["item_attachments"]; len(got) != 1 || got[0].ID != kept+"/"+common {
		t.Errorf("attachments = %+v", got)
	}
	objects := result.Records["media_objects"]
	if len(objects) != 1 || objects[0].ID != common || objects[0].Data["ref_count"] != 1 {
		t.Errorf("objects = %+v", objects)
	}
	if _, held := result.Media["d-alone"]; held || len(result.Media) != 1 {
		t.Errorf("the bytes only the refused entry used stay out of the archive: %v", result.Media)
	}
}
