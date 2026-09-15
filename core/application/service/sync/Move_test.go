// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// stepJournal is the history, in memory: what the push wrote beside the use cases it performed.
type stepJournal struct{ steps []step }

type step struct {
	verb      activity.Verb
	changeSet map[string]any
	itemID    shared.ID
}

func (j *stepJournal) RecordStep(
	_ context.Context, _ appshared.ActorContext, _, itemID, _ shared.ID,
	verb activity.Verb, changeSet map[string]any, _ time.Time,
) error {
	j.steps = append(j.steps, step{verb: verb, changeSet: changeSet, itemID: itemID})
	return nil
}

// displacedFiler is the comment filer, in memory, refusing when told to.
type displacedFiler struct {
	filed  []filing
	refuse error
}

type filing struct {
	itemID shared.ID
	body   string
	params map[string]string
}

func (f *displacedFiler) FileDisplaced(
	_ context.Context, _ appshared.ActorContext, itemID shared.ID, body string, params map[string]string,
) (work.Comment, error) {
	if f.refuse != nil {
		return work.Comment{}, f.refuse
	}
	f.filed = append(f.filed, filing{itemID: itemID, body: body, params: params})
	return work.Comment{ID: purged, ItemID: itemID, Body: body, Kind: work.CommentBySystem}, nil
}

type mergeFixture struct {
	patchFixture
	journal *stepJournal
	filer   *displacedFiler
}

func merging(t *testing.T, clocks map[string]shared.HLC) mergeFixture {
	t.Helper()
	f := patching(t, clocks)
	journal, filer := &stepJournal{}, &displacedFiler{}
	f.push.Activity = journal
	f.push.Displaced = filer
	for _, name := range []string{"CompleteWorkItem", "ReopenWorkItem", "MoveWorkItem"} {
		f.catalogue.outputs[name] = usecase.Output{"id": itemX.String(), "version": 5}
	}
	f.catalogue.outputs["GetWorkItem"]["collection_id"] = collectionA.String()
	f.catalogue.outputs["GetWorkItem"]["notes"] = "The server's notes"
	return mergeFixture{patchFixture: f, journal: journal, filer: filer}
}

// A completion that wins is CompleteWorkItem, a reopen ReopenWorkItem - each idempotent in the
// use case, which is what keeps a double completion from a second occurrence (SY-8).
func TestACompletionThatWinsGoesToTheUseCaseThatOwnsIt(t *testing.T) {
	f := merging(t, map[string]shared.HLC{})
	mine := reading(0, "dev-a")

	if _, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"completion": {Value: map[string]any{"is_completed": true}, HLC: mine.String()},
	}, nil)); err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if len(f.calls("CompleteWorkItem")) != 1 || len(f.calls("ReopenWorkItem")) != 0 {
		t.Errorf("the completion went to %v", f.catalogue.calls)
	}

	f = merging(t, map[string]shared.HLC{})
	if _, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"completion": {Value: false, HLC: mine.String()},
	}, nil)); err != nil {
		t.Fatalf("pushing the reopen: %v", err)
	}
	if len(f.calls("ReopenWorkItem")) != 1 {
		t.Errorf("the reopen went to %v", f.catalogue.calls)
	}
	if len(f.journal.steps) != 0 {
		t.Errorf("a winning completion wrote a step of its own: %+v", f.journal.steps)
	}
}

// A reopen that loses is never silently discarded (§4.2's status row): the history records the
// losing act with the device and the reading, and the answer is MERGED.
func TestAReopenThatLosesIsAVisibleStep(t *testing.T) {
	f := merging(t, map[string]shared.HLC{"completion": reading(time.Minute, "server")})
	mine := reading(0, "dev-a")

	response, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"completion": {Value: map[string]any{"is_completed": false}, HLC: mine.String()},
	}, nil))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if response.Results[0].Result != domain.Merged {
		t.Errorf("answered %s, want MERGED", response.Results[0].Result)
	}
	if len(f.calls("ReopenWorkItem")) != 0 {
		t.Errorf("a losing reopen was applied")
	}
	if len(f.journal.steps) != 1 || f.journal.steps[0].verb != activity.ItemChangeLost {
		t.Fatalf("the history has %+v, want one item.change_lost", f.journal.steps)
	}
	set := f.journal.steps[0].changeSet
	if set["field"] != "completion" || set["device_id"] != device.String() || set["hlc"] != mine.String() || set["is_completed"] != false {
		t.Errorf("the step says %v", set)
	}
}

// SY-10: notes that lose are not discarded - the displaced version is filed as a system comment
// in the pushing person's name, the history marks the merge, and the answer is CONFLICT with
// both values and the comment.
func TestDisplacedNotesAreFiledAndTheAnswerIsAConflict(t *testing.T) {
	f := merging(t, map[string]shared.HLC{"notes": reading(time.Minute, "server")})
	mine := reading(0, "dev-a")

	response, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"notes": {Value: "Written on the train", HLC: mine.String()},
		"title": {Value: "Still wins", HLC: mine.String()},
	}, nil))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	result := response.Results[0]
	if result.Result != domain.Conflict || result.Conflict == nil {
		t.Fatalf("answered %+v, want CONFLICT", result)
	}
	if result.Conflict.Field != "notes" || result.Conflict.Mine != "Written on the train" ||
		result.Conflict.Theirs != "The server's notes" || result.Conflict.PreservedCommentID != purged {
		t.Errorf("the conflict is %+v", result.Conflict)
	}
	// The title still won and was applied; the notes were not.
	update := f.calls("UpdateWorkItem")[0].in
	if update["title"] != "Still wins" {
		t.Errorf("the winning title was not applied: %v", update)
	}
	if _, sent := update["notes"]; sent {
		t.Errorf("the losing notes were applied")
	}
	// The comment carries the text and the heading's parameters; the code is the filer's.
	if len(f.filer.filed) != 1 || f.filer.filed[0].body != "Written on the train" ||
		f.filer.filed[0].params["device"] != device.String() || f.filer.filed[0].params["field"] != "notes" {
		t.Errorf("filed %+v", f.filer.filed)
	}
	if len(f.journal.steps) != 1 || f.journal.steps[0].verb != activity.ItemMerged ||
		f.journal.steps[0].changeSet["preserved_comment_id"] != purged.String() {
		t.Errorf("the history has %+v", f.journal.steps)
	}
}

// A device that may not comment on the entry is told so by the comment's own permission, and the
// conflict still carries both values - with no comment to point at.
func TestAConflictWithoutTheRightToCommentStillCarriesBothValues(t *testing.T) {
	f := merging(t, map[string]shared.HLC{"notes": reading(time.Minute, "server")})
	f.filer.refuse = shared.ErrForbidden.WithDetail("access.forbidden")
	mine := reading(0, "dev-a")

	response, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"notes": {Value: "mine", HLC: mine.String()},
	}, nil))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	result := response.Results[0]
	if result.Result != domain.Conflict || result.Conflict == nil || !result.Conflict.PreservedCommentID.IsZero() ||
		result.Conflict.Mine != "mine" {
		t.Errorf("answered %+v", result)
	}
}

func moveOf(payload map[string]any, hlc shared.HLC, base *int) PushRequest {
	return PushRequest{DeviceID: device, Mutations: []Mutation{{
		OpID: opA, Kind: domain.Move, ItemID: itemX, HLC: hlc.String(), Payload: payload, BaseVersion: base,
	}}}
}

func TestAMoveThatWinsGoesToMoveWorkItemUnderItsReading(t *testing.T) {
	f := merging(t, map[string]shared.HLC{"parent_id": reading(-time.Hour, "server")})
	mine := reading(0, "dev-a")
	base := 4

	response, err := f.push.Push(t.Context(), pusher(), moveOf(map[string]any{
		"parent_id": otherPerson.String(), "order_key": "a0V", "bucket_id": nil,
	}, mine, &base))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if response.Results[0].Result != domain.Applied {
		t.Errorf("answered %s, want APPLIED", response.Results[0].Result)
	}
	moves := f.calls("MoveWorkItem")
	if len(moves) != 1 || moves[0].in["target_parent_id"] != otherPerson.String() ||
		moves[0].in["order_key"] != "a0V" || moves[0].in["target_bucket_id"] != "" {
		t.Errorf("MoveWorkItem was asked %+v", moves)
	}
	if _, sent := moves[0].in["target_collection_id"]; sent {
		t.Errorf("a field the device did not name was sent")
	}
	if f.catalogue.readings["order_key"].Compare(mine) != 0 {
		t.Errorf("the move's fields did not carry the device's reading")
	}
}

func TestAMoveToTheTopLevelIsPresentAndEmpty(t *testing.T) {
	f := merging(t, map[string]shared.HLC{})
	if _, err := f.push.Push(t.Context(), pusher(), moveOf(map[string]any{"parent_id": nil}, reading(0, "dev-a"), nil)); err != nil {
		t.Fatalf("pushing: %v", err)
	}
	in := f.calls("MoveWorkItem")[0].in
	if value, present := in["target_parent_id"]; !present || value != "" {
		t.Errorf("the top level reached the use case as %v", in)
	}
}

func TestAMoveTheServerOutvotedIsMergedAndNotApplied(t *testing.T) {
	f := merging(t, map[string]shared.HLC{"parent_id": reading(time.Minute, "server")})

	response, err := f.push.Push(t.Context(), pusher(), moveOf(map[string]any{"parent_id": nil}, reading(0, "dev-a"), nil))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if response.Results[0].Result != domain.Merged || len(f.calls("MoveWorkItem")) != 0 {
		t.Errorf("answered %s with %d moves", response.Results[0].Result, len(f.calls("MoveWorkItem")))
	}
}

// SY-12: a move the clock accepts but that would close a cycle is refused in the contract's word.
func TestAMoveThatClosesACycleIsRejectedAsSuch(t *testing.T) {
	f := merging(t, map[string]shared.HLC{})
	f.catalogue.errs["MoveWorkItem"] = shared.ErrValidation.WithDetail("items.parent_in_own_subtree")

	response, err := f.push.Push(t.Context(), pusher(), moveOf(map[string]any{"parent_id": otherPerson.String()}, reading(0, "dev-a"), nil))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	result := response.Results[0]
	if result.Result != domain.Rejected || result.Error == nil || result.Error.MessageCode != "sync.cycle_detected" {
		t.Errorf("answered %+v, want a rejection with sync.cycle_detected", result)
	}
	if result.Error.Code != "conflict" {
		t.Errorf("a cycle is answered as %q, want the conflict category", result.Error.Code)
	}
}

func TestAMoveIsJudgedByItsPayload(t *testing.T) {
	cases := map[string]struct {
		payload map[string]any
		code    string
	}{
		"no parent":            {payload: map[string]any{"order_key": "a0V"}, code: "sync.fields_required"},
		"a field nobody knows": {payload: map[string]any{"parent_id": nil, "title": "x"}, code: "sync.field_unknown"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := merging(t, map[string]shared.HLC{})
			response, err := f.push.Push(t.Context(), pusher(), moveOf(tc.payload, reading(0, "dev-a"), nil))
			if err != nil {
				t.Fatalf("pushing: %v", err)
			}
			if result := response.Results[0]; result.Result != domain.Rejected || result.Error == nil || result.Error.MessageCode != tc.code {
				t.Errorf("answered %+v, want %s", result, tc.code)
			}
		})
	}
}
