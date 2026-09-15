// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
)

// clockStore is the server's clock per field, in memory.
type clockStore struct{ clocks map[string]shared.HLC }

func (s clockStore) Of(_ context.Context, _ string, _ shared.ID) (map[string]shared.HLC, error) {
	return s.clocks, nil
}

// readingCatalogue is the stub catalogue, and it also notes the reading the context carried for
// the field each call was about - which is what the change log adapter reads.
type readingCatalogue struct {
	*stubCatalogue
	readings map[string]shared.HLC
}

func (c *readingCatalogue) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	for _, field := range []string{"title", "notes", "due_at", "assignee_id", "cover", "order_key", "custom_fields.priority"} {
		if reading, found := appshared.ReadingFrom(ctx, field); found {
			c.readings[field] = reading
		}
	}
	return c.stubCatalogue.Invoke(ctx, name, actor, in)
}

func reading(offset time.Duration, device string) shared.HLC {
	r, _ := shared.NewHLC(now.Add(offset), 1, device)
	return r
}

type patchFixture struct {
	push      PushChanges
	catalogue *readingCatalogue
	clocks    clockStore
}

func patching(t *testing.T, clocks map[string]shared.HLC) patchFixture {
	t.Helper()
	f := pushing(t)
	catalogue := &readingCatalogue{stubCatalogue: f.catalogue, readings: map[string]shared.HLC{}}
	catalogue.outputs["GetWorkItem"] = usecase.Output{
		"id": itemX.String(), "title": "Fix the tap", "version": 4,
		"due_at": "2026-09-20T09:00:00Z", "due_date_only": false, "due_time_zone": "Europe/Berlin",
	}
	for _, name := range []string{"UpdateWorkItem", "AssignWorkItem", "UnassignWorkItem", "SetCover",
		"ClearCover", "SetCustomField", "ReorderWorkItem"} {
		catalogue.outputs[name] = usecase.Output{"id": itemX.String(), "version": 5}
	}
	store := clockStore{clocks: clocks}
	f.push.Catalogue = catalogue
	f.push.Clocks = store
	return patchFixture{push: f.push, catalogue: catalogue, clocks: store}
}

func (f patchFixture) calls(name string) []invocation {
	var out []invocation
	for _, call := range f.catalogue.calls {
		if call.name == name {
			out = append(out, call)
		}
	}
	return out
}

func patchOf(fields map[string]FieldChange, base *int) PushRequest {
	return PushRequest{DeviceID: device, Mutations: []Mutation{{
		OpID: opA, Kind: domain.ItemPatch, ItemID: itemX, BaseVersion: base, Fields: fields,
	}}}
}

func TestEveryFieldTheDeviceWinsIsAppliedUnderItsReading(t *testing.T) {
	f := patching(t, map[string]shared.HLC{"title": reading(-time.Hour, "server"), "notes": reading(-time.Hour, "server")})
	base := 4
	mine := reading(-time.Minute, "dev-a")

	response, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"title": {Value: "Fix the kitchen tap", HLC: mine.String()},
		"notes": {Value: "Under the sink", HLC: mine.String()},
	}, &base))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	result := response.Results[0]
	if result.Result != domain.Applied || result.ServerState["title"] != "Fix the tap" {
		t.Errorf("answered %+v, want APPLIED with the server's state", result)
	}
	updates := f.calls("UpdateWorkItem")
	if len(updates) != 1 || updates[0].in["title"] != "Fix the kitchen tap" || updates[0].in["notes"] != "Under the sink" ||
		updates[0].in["item_id"] != itemX.String() {
		t.Errorf("UpdateWorkItem was asked %+v", updates)
	}
	// The change log adapter will record each field under the device's reading.
	if f.catalogue.readings["title"].Compare(mine) != 0 || f.catalogue.readings["notes"].Compare(mine) != 0 {
		t.Errorf("the readings in the context are %v, want the device's", f.catalogue.readings)
	}
	if f.calls("GetWorkItem") == nil {
		t.Errorf("the server's copy was never read")
	}
}

// SY-1 at the merge: two fields, one the device wins and one it loses, both survive - the winner
// applied, the loser left as the server has it, and the answer MERGED with the server's state.
func TestAFieldTheServerWroteLaterIsKeptAndTheAnswerIsMerged(t *testing.T) {
	f := patching(t, map[string]shared.HLC{
		"title": reading(time.Minute, "server"), // the server's title is later than the device's
		"notes": reading(-time.Hour, "server"),
	})
	mine := reading(-time.Minute, "dev-a")

	response, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"title": {Value: "stale", HLC: mine.String()},
		"notes": {Value: "fresh", HLC: mine.String()},
	}, nil))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if response.Results[0].Result != domain.Merged {
		t.Errorf("answered %s, want MERGED", response.Results[0].Result)
	}
	updates := f.calls("UpdateWorkItem")
	if len(updates) != 1 {
		t.Fatalf("%d updates, want one", len(updates))
	}
	if _, sent := updates[0].in["title"]; sent {
		t.Errorf("the losing title was applied")
	}
	if updates[0].in["notes"] != "fresh" {
		t.Errorf("the winning notes were not applied: %v", updates[0].in)
	}
	if _, found := f.catalogue.readings["title"]; found {
		t.Errorf("a losing field carried a reading into the context")
	}
}

func TestAFieldNobodyStampedLosesToAnyReadingAndABaseThatMovedIsMerged(t *testing.T) {
	f := patching(t, map[string]shared.HLC{})
	base := 3 // the server holds version 4
	old := reading(-365*24*time.Hour, "dev-a")

	response, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"title": {Value: "from a year ago", HLC: old.String()},
	}, &base))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if len(f.calls("UpdateWorkItem")) != 1 {
		t.Errorf("a field with no server reading did not win")
	}
	if response.Results[0].Result != domain.Merged {
		t.Errorf("a base that moved was answered %s, want MERGED", response.Results[0].Result)
	}
}

func TestEachFieldGoesToTheUseCaseThatOwnsIt(t *testing.T) {
	f := patching(t, map[string]shared.HLC{})
	mine := reading(0, "dev-a")
	cover := map[string]any{"kind": "COLOR", "color_token": "blue"}

	_, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"assignee_id":            {Value: otherPerson.String(), HLC: mine.String()},
		"cover":                  {Value: cover, HLC: mine.String()},
		"custom_fields.priority": {Value: "high", HLC: mine.String()},
		"order_key":              {Value: "a0V", HLC: mine.String()},
	}, nil))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if calls := f.calls("AssignWorkItem"); len(calls) != 1 || calls[0].in["account_id"] != otherPerson.String() {
		t.Errorf("AssignWorkItem was asked %+v", calls)
	}
	if calls := f.calls("SetCover"); len(calls) != 1 || calls[0].in["kind"] != "COLOR" || calls[0].in["color_token"] != "blue" {
		t.Errorf("SetCover was asked %+v", calls)
	}
	if calls := f.calls("SetCustomField"); len(calls) != 1 || calls[0].in["key"] != "priority" || calls[0].in["value"] != "high" {
		t.Errorf("SetCustomField was asked %+v", calls)
	}
	if calls := f.calls("ReorderWorkItem"); len(calls) != 1 || calls[0].in["order_key"] != "a0V" {
		t.Errorf("ReorderWorkItem was asked %+v", calls)
	}
	if len(f.calls("UpdateWorkItem")) != 0 {
		t.Errorf("UpdateWorkItem was asked for nothing")
	}

	// A null clears: the assignee through UnassignWorkItem, the cover through ClearCover, a
	// custom field through a null value.
	f = patching(t, map[string]shared.HLC{})
	if _, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"assignee_id":            {Value: nil, HLC: mine.String()},
		"cover":                  {Value: nil, HLC: mine.String()},
		"custom_fields.priority": {Value: nil, HLC: mine.String()},
	}, nil)); err != nil {
		t.Fatalf("pushing the clearing: %v", err)
	}
	if len(f.calls("UnassignWorkItem")) != 1 || len(f.calls("ClearCover")) != 1 {
		t.Errorf("the clearing went to %v", f.catalogue.calls)
	}
	if calls := f.calls("SetCustomField"); len(calls) != 1 || calls[0].in["value"] != nil {
		t.Errorf("the custom field's clearing was %+v", calls)
	}
}

// The due date is a trio the use case wants together: a winning member is completed with the
// server's values of the others, and a null date takes the qualifiers with it.
func TestTheDueTrioTravelsTogether(t *testing.T) {
	f := patching(t, map[string]shared.HLC{})
	mine := reading(0, "dev-a")

	if _, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"due_date_only": {Value: true, HLC: mine.String()},
	}, nil)); err != nil {
		t.Fatalf("pushing: %v", err)
	}
	update := f.calls("UpdateWorkItem")[0].in
	if update["due_date_only"] != true || update["due_at"] != "2026-09-20T09:00:00Z" || update["due_time_zone"] != "Europe/Berlin" {
		t.Errorf("the trio reached the use case as %v", update)
	}

	f = patching(t, map[string]shared.HLC{})
	if _, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"due_at": {Value: nil, HLC: mine.String()},
	}, nil)); err != nil {
		t.Fatalf("pushing the clearing: %v", err)
	}
	update = f.calls("UpdateWorkItem")[0].in
	if update["due_at"] != "" {
		t.Errorf("the clearing reached the use case as %v", update)
	}
	if _, sent := update["due_time_zone"]; sent {
		t.Errorf("a qualifier travelled with a cleared date: %v", update)
	}
}

func TestAPatchIsJudgedByItsFieldNames(t *testing.T) {
	cases := map[string]struct {
		fields map[string]FieldChange
		code   string
	}{
		"no fields":            {fields: map[string]FieldChange{}, code: "sync.fields_required"},
		"a server-owned field": {fields: map[string]FieldChange{"version": {Value: 9, HLC: reading(0, "dev-a").String()}}, code: "sync.field_not_mergeable"},
		"a field N-06 applies": {fields: map[string]FieldChange{"completion": {Value: true, HLC: reading(0, "dev-a").String()}}, code: "sync.field_unavailable"},
		"a field nobody knows": {fields: map[string]FieldChange{"colour": {Value: "x", HLC: reading(0, "dev-a").String()}}, code: "sync.field_unknown"},
		"a cover that is not an object": {
			fields: map[string]FieldChange{"cover": {Value: "blue", HLC: reading(0, "dev-a").String()}}, code: "sync.field_malformed",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := patching(t, map[string]shared.HLC{})
			response, err := f.push.Push(t.Context(), pusher(), patchOf(tc.fields, nil))
			if err != nil {
				t.Fatalf("pushing: %v", err)
			}
			result := response.Results[0]
			if result.Result != domain.Rejected || result.Error == nil || result.Error.MessageCode != tc.code {
				t.Errorf("answered %+v, want a rejection with %s", result, tc.code)
			}
			if len(f.calls("UpdateWorkItem")) != 0 {
				t.Errorf("something was applied")
			}
		})
	}
}

// A use case's refusal on one field is the mutation's answer: nothing of it is applied
// piecemeal, and the refusal carries the use case's code.
func TestARefusedFieldRejectsTheWholeMutation(t *testing.T) {
	f := patching(t, map[string]shared.HLC{})
	f.catalogue.errs["SetCover"] = shared.ErrValidation.WithDetail("items.cover_not_supported_for_type")
	mine := reading(0, "dev-a")

	response, err := f.push.Push(t.Context(), pusher(), patchOf(map[string]FieldChange{
		"cover": {Value: map[string]any{"kind": "COLOR", "color_token": "blue"}, HLC: mine.String()},
		"title": {Value: "x", HLC: mine.String()},
	}, nil))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	result := response.Results[0]
	if result.Result != domain.Rejected || result.Error == nil || result.Error.MessageCode != "items.cover_not_supported_for_type" {
		t.Errorf("answered %+v", result)
	}
}
