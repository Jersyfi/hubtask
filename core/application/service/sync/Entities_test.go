// SPDX-License-Identifier: Apache-2.0
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

var (
	entityE = shared.ID("01936f2a-7c1e-7000-8000-000000000d01")
	opC     = shared.ID("01936f2a-7c1e-7000-8000-000000000f03")
)

// readingsCatalogue notes the reading the context carried for every field each call was about.
type readingsCatalogue struct {
	*stubCatalogue
	readings map[string]map[string]shared.HLC
}

func (c *readingsCatalogue) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	for field := range in {
		if reading, found := appshared.ReadingFrom(ctx, field); found {
			if c.readings[name] == nil {
				c.readings[name] = map[string]shared.HLC{}
			}
			c.readings[name][field] = reading
		}
	}
	return c.stubCatalogue.Invoke(ctx, name, actor, in)
}

func beyondEntry(t *testing.T, clocks map[string]shared.HLC) (PushChanges, *readingsCatalogue) {
	t.Helper()
	f := pushing(t)
	catalogue := &readingsCatalogue{stubCatalogue: f.catalogue, readings: map[string]map[string]shared.HLC{}}
	state := usecase.Output{"id": entityE.String(), "name": "Doing", "version": 2}
	for _, name := range []string{"CreateReminder", "UpdateReminder", "CreateContainer", "RenameContainer", "GetContainer",
		"CreateLabel", "UpdateLabel", "CreateBucket", "UpdateBucket", "SetRecurrence"} {
		catalogue.outputs[name] = state
	}
	list := usecase.Output{"data": []usecase.Output{state}}
	for _, name := range []string{"ListReminders", "ListLabels", "ListBuckets"} {
		catalogue.outputs[name] = list
	}
	catalogue.outputs["InstantiateTemplate"] = usecase.Output{"root_item_id": itemX.String(), "created": 3}
	f.push.Catalogue = catalogue
	f.push.Clocks = clockStore{clocks: clocks}
	return f.push, catalogue
}

func pushOne(t *testing.T, push PushChanges, m Mutation) Result {
	t.Helper()
	response, err := push.Push(t.Context(), pusher(), PushRequest{DeviceID: device, Mutations: []Mutation{m}})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	return response.Results[0]
}

// Every kind beyond the entry's reaches the use case the route performs, as the pushing person,
// with the identifier the device minted on a creation.
func TestEachKindIsAppliedThroughTheRoutesUseCase(t *testing.T) {
	hlc := "1757937600000:00001:dev-a"
	cases := []struct {
		kind  domain.MutationKind
		m     Mutation
		name  string
		input map[string]any
	}{
		{domain.ReminderCreate, Mutation{ItemID: itemX, Payload: map[string]any{"offset_spec": "-PT1H"}}, "CreateReminder",
			map[string]any{"id": entityE.String(), "item_id": itemX.String(), "offset_spec": "-PT1H"}},
		{domain.ReminderDelete, Mutation{ItemID: itemX}, "DeleteReminder",
			map[string]any{"reminder_id": entityE.String(), "item_id": itemX.String()}},
		{domain.ContainerCreate, Mutation{Payload: map[string]any{"type": "COLLECTION", "name": "Garden"}}, "CreateContainer",
			map[string]any{"id": entityE.String(), "type": "COLLECTION", "name": "Garden"}},
		{domain.ContainerDelete, Mutation{}, "TrashContainer", map[string]any{"container_id": entityE.String()}},
		{domain.LabelCreate, Mutation{Payload: map[string]any{"collection_id": collectionA.String(), "name": "Urgent"}}, "CreateLabel",
			map[string]any{"id": entityE.String(), "collection_id": collectionA.String(), "name": "Urgent"}},
		{domain.LabelDelete, Mutation{ContainerID: collectionA}, "DeleteLabel", map[string]any{"label_id": entityE.String()}},
		{domain.BucketCreate, Mutation{Payload: map[string]any{"collection_id": collectionA.String(), "name": "Doing"}}, "CreateBucket",
			map[string]any{"id": entityE.String(), "collection_id": collectionA.String(), "name": "Doing"}},
		{domain.BucketDelete, Mutation{ContainerID: collectionA}, "DeleteBucket", map[string]any{"bucket_id": entityE.String()}},
		{domain.RecurrenceDelete, Mutation{ItemID: itemX}, "RemoveRecurrence", map[string]any{"item_id": itemX.String()}},
		{domain.TemplateApply, Mutation{Payload: map[string]any{"template_id": "t", "collection_id": collectionA.String()}}, "InstantiateTemplate",
			map[string]any{"template_id": "t", "collection_id": collectionA.String()}},
	}
	for _, c := range cases {
		t.Run(string(c.kind), func(t *testing.T) {
			push, catalogue := beyondEntry(t, nil)
			m := c.m
			m.OpID, m.Kind, m.HLC = opA, c.kind, hlc
			if c.kind != domain.TemplateApply {
				m.EntityID = entityE
			}
			result := pushOne(t, push, m)
			if result.Result != domain.Applied {
				t.Fatalf("answered %+v", result)
			}
			var call *invocation
			for i := range catalogue.calls {
				if catalogue.calls[i].name == c.name {
					call = &catalogue.calls[i]
				}
			}
			if call == nil {
				t.Fatalf("%s was not performed; calls %v", c.name, catalogue.calls)
			}
			if call.actor.AccountID != pusher().AccountID || call.device != device {
				t.Errorf("performed as %v from %v", call.actor.AccountID, call.device)
			}
			for key, want := range c.input {
				if call.in[key] != want {
					t.Errorf("%s got %s = %v, want %v", c.name, key, call.in[key], want)
				}
			}
		})
	}
}

// A patch of each family is decided field by field: the field the device wrote later wins and is
// applied under its reading, the one the server wrote later stays, and the answer is MERGED.
func TestAPatchOfEachFamilyIsDecidedFieldByField(t *testing.T) {
	early, _ := shared.NewHLC(now.Add(-time.Hour), 1, "dev-a")
	late, _ := shared.NewHLC(now.Add(time.Minute), 1, "dev-a")
	server, _ := shared.NewHLC(now, 1, "server")
	cases := []struct {
		kind         domain.MutationKind
		update       string
		wins, loses  string
		winValue     any
		itemOrParent func(*Mutation)
	}{
		{domain.ReminderPatch, "UpdateReminder", "offset_spec", "channels", "-PT2H", func(m *Mutation) { m.ItemID = itemX }},
		{domain.ContainerPatch, "RenameContainer", "name", "icon", "Garden", func(*Mutation) {}},
		{domain.LabelPatch, "UpdateLabel", "name", "color_token", "Later", func(m *Mutation) { m.ContainerID = collectionA }},
		{domain.BucketPatch, "UpdateBucket", "wip_limit", "name", nil, func(m *Mutation) { m.ContainerID = collectionA }},
	}
	for _, c := range cases {
		t.Run(string(c.kind), func(t *testing.T) {
			push, catalogue := beyondEntry(t, map[string]shared.HLC{c.loses: server, c.wins: server})
			m := Mutation{OpID: opA, Kind: c.kind, EntityID: entityE, Fields: map[string]FieldChange{
				c.wins:  {Value: c.winValue, HLC: late.String()},
				c.loses: {Value: "x", HLC: early.String()},
			}}
			c.itemOrParent(&m)
			result := pushOne(t, push, m)
			if result.Result != domain.Merged || result.ServerState["id"] != entityE.String() {
				t.Fatalf("answered %+v", result)
			}
			var call *invocation
			for i := range catalogue.calls {
				if catalogue.calls[i].name == c.update {
					call = &catalogue.calls[i]
				}
			}
			if call == nil {
				t.Fatalf("%s was not performed", c.update)
			}
			if _, sent := call.in[c.loses]; sent {
				t.Errorf("the lost field %s was applied", c.loses)
			}
			if _, sent := call.in[c.wins]; !sent {
				t.Errorf("the won field %s was not applied", c.wins)
			}
			if c.winValue == nil && call.in[c.wins] != 0 {
				t.Errorf("a cleared %s reached the use case as %v", c.wins, call.in[c.wins])
			}
			if got := catalogue.readings[c.update][c.wins]; got != late {
				t.Errorf("%s applied under %v, want the device's %v", c.wins, got, late)
			}
		})
	}
}

// A field only the server writes is refused by name and recorded; the repeat answers the same
// refusal without performing anything.
func TestAFieldTheServerDecidesIsRefusedAndTheRepeatAnswersTheSame(t *testing.T) {
	push, catalogue := beyondEntry(t, nil)
	m := Mutation{OpID: opA, Kind: domain.ReminderPatch, ItemID: itemX, EntityID: entityE, Fields: map[string]FieldChange{
		"fire_at": {Value: "2026-10-28T09:00:00Z", HLC: "1757937600000:00001:dev-a"},
	}}
	for range 2 {
		result := pushOne(t, push, m)
		if result.Result != domain.Rejected || result.Error.MessageCode != "sync.field_not_mergeable" || result.EntityID != entityE {
			t.Fatalf("answered %+v", result)
		}
	}
	if len(catalogue.calls) != 0 {
		t.Errorf("a refused patch performed %v", catalogue.calls)
	}
}

// A creation replayed is applied once: the operation log answers the repeat.
func TestARepeatedCreationIsAppliedOnce(t *testing.T) {
	push, catalogue := beyondEntry(t, nil)
	m := Mutation{OpID: opC, Kind: domain.LabelCreate, EntityID: entityE, HLC: "1757937600000:00001:dev-a",
		Payload: map[string]any{"collection_id": collectionA.String(), "name": "Urgent"}}
	first, second := pushOne(t, push, m), pushOne(t, push, m)
	if first.Result != domain.Applied || second.Result != domain.Applied || second.EntityID != entityE {
		t.Fatalf("answered %+v then %+v", first, second)
	}
	if n := len(catalogue.calls); n != 1 {
		t.Errorf("CreateLabel performed %d times", n)
	}
}

// The entry's one series: a creation where it has none carries the device's identifier, and a
// creation for an entry that already has one merges field by field with it.
func TestASeriesIsCreatedOnceAndMergedAfterwards(t *testing.T) {
	hlc := "1757937600000:00001:dev-a"
	creation := Mutation{OpID: opA, Kind: domain.RecurrenceCreate, ItemID: itemX, EntityID: entityE, HLC: hlc,
		Payload: map[string]any{"rrule": "FREQ=WEEKLY", "time_zone": "Europe/Berlin", "mode": "SCHEDULED"}}

	push, catalogue := beyondEntry(t, nil)
	catalogue.errs["GetRecurrence"] = shared.ErrNotFound.WithDetail("recurrence.not_found")
	if result := pushOne(t, push, creation); result.Result != domain.Applied {
		t.Fatalf("answered %+v", result)
	}
	set := catalogue.calls[len(catalogue.calls)-1]
	if set.name != "SetRecurrence" || set.in["id"] != entityE.String() || set.in["rrule"] != "FREQ=WEEKLY" {
		t.Fatalf("performed %+v", set)
	}

	later, _ := shared.NewHLC(now.Add(time.Hour), 1, "server")
	push, catalogue = beyondEntry(t, map[string]shared.HLC{"time_zone": later})
	catalogue.outputs["GetRecurrence"] = usecase.Output{"id": "01936f2a-7c1e-7000-8000-000000000d09",
		"rrule": "FREQ=DAILY", "time_zone": "UTC", "mode": "SCHEDULED", "horizon_days": 30, "ends_at": nil, "max_count": nil}
	result := pushOne(t, push, creation)
	if result.Result != domain.Merged || result.EntityID != "01936f2a-7c1e-7000-8000-000000000d09" {
		t.Fatalf("answered %+v", result)
	}
	set = catalogue.calls[len(catalogue.calls)-1]
	if set.name != "SetRecurrence" || set.in["rrule"] != "FREQ=WEEKLY" || set.in["time_zone"] != "UTC" ||
		set.in["horizon_days"] != 30 || set.in["ends_at"] != "" || set.in["max_count"] != 0 {
		t.Errorf("the series was set to %v", set.in)
	}
	if _, carried := set.in["id"]; carried {
		t.Errorf("a series the entry has took the device's identifier")
	}
}

// A patch names what it is about: a reminder without its identifier, and a label without its
// collection, are refused by name.
func TestAPatchWithoutItsObjectIsRefusedByName(t *testing.T) {
	push, _ := beyondEntry(t, nil)
	field := map[string]FieldChange{"name": {Value: "x", HLC: "1757937600000:00001:dev-a"}}
	if got := pushOne(t, push, Mutation{OpID: opA, Kind: domain.ContainerPatch, Fields: field}); got.Error == nil ||
		got.Error.MessageCode != "sync.entity_required" {
		t.Errorf("a patch without its object answered %+v", got)
	}
	if got := pushOne(t, push, Mutation{OpID: opB, Kind: domain.LabelPatch, EntityID: entityE, Fields: field}); got.Error == nil ||
		got.Error.MessageCode != "sync.container_required" {
		t.Errorf("a label's patch without its collection answered %+v", got)
	}
}
