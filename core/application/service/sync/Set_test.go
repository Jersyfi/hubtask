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
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var labelX = shared.ID("01936f2a-7c1e-7000-8000-00000000ab01")

// elementStore is one set's tags in memory.
type elementStore struct{ elements []work.SetElement }

func (s elementStore) Elements(_ context.Context, _ shared.ID) ([]work.SetElement, error) {
	return s.elements, nil
}

// tagCatalogue notes the tag the context carried for the set of each call.
type tagCatalogue struct {
	*stubCatalogue
	tags map[string]shared.HLC
}

func (c *tagCatalogue) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	for _, set := range []string{"labels", "members", "attachments"} {
		if tag, found := appshared.ReadingFrom(ctx, set); found {
			c.tags[set] = tag
		}
	}
	return c.stubCatalogue.Invoke(ctx, name, actor, in)
}

type setFixture struct {
	pushFixture
	catalogue *tagCatalogue
}

func setting(t *testing.T, stored ...work.SetElement) setFixture {
	t.Helper()
	f := pushing(t)
	catalogue := &tagCatalogue{stubCatalogue: f.catalogue, tags: map[string]shared.HLC{}}
	catalogue.outputs["GetWorkItem"] = usecase.Output{"id": itemX.String(), "version": 4}
	for _, name := range []string{"AddLabel", "RemoveLabel", "AddMember", "RemoveMember", "AttachMedia", "DetachMedia"} {
		catalogue.outputs[name] = usecase.Output{"item_id": itemX.String()}
	}
	f.push.Catalogue = catalogue
	f.push.Sets = Sets{Labels: elementStore{stored}, Members: elementStore{}, Attachments: elementStore{}}
	return setFixture{pushFixture: f, catalogue: catalogue}
}

func (f setFixture) calls(name string) int {
	count := 0
	for _, call := range f.catalogue.calls {
		if call.name == name {
			count++
		}
	}
	return count
}

func setOf(kind domain.MutationKind, set string, element shared.ID, tag shared.HLC) PushRequest {
	return PushRequest{DeviceID: device, Mutations: []Mutation{{
		OpID: opA, Kind: kind, ItemID: itemX, Set: set, Element: element, HLC: tag.String(),
	}}}
}

// SY-3 at the merge: an addition on one device and the removal of a different element on another
// both hold, because each element carries its own tags.
func TestAnAdditionAndARemovalOfDifferentElementsBothHold(t *testing.T) {
	added, _ := shared.NewHLC(now.Add(-time.Hour), 1, "server")
	other := shared.ID("01936f2a-7c1e-7000-8000-00000000ab02")
	f := setting(t, work.SetElement{ElementID: other, AddedAt: added})
	mine := reading(0, "dev-a")

	// Device A adds a label the server does not hold.
	response, err := f.push.Push(t.Context(), pusher(), setOf(domain.SetAdd, "labels", labelX, mine))
	if err != nil {
		t.Fatalf("adding: %v", err)
	}
	if response.Results[0].Result != domain.Applied || f.calls("AddLabel") != 1 {
		t.Errorf("the addition was answered %s with %d AddLabel calls", response.Results[0].Result, f.calls("AddLabel"))
	}
	if f.catalogue.tags["labels"].Compare(mine) != 0 {
		t.Errorf("the writer was not handed the device's tag: %v", f.catalogue.tags)
	}

	// Device B removes the other label, which the server holds.
	f = setting(t, work.SetElement{ElementID: other, AddedAt: added})
	response, err = f.push.Push(t.Context(), pusher(), setOf(domain.SetRemove, "labels", other, reading(time.Minute, "dev-b")))
	if err != nil {
		t.Fatalf("removing: %v", err)
	}
	if response.Results[0].Result != domain.Applied || f.calls("RemoveLabel") != 1 {
		t.Errorf("the removal was answered %s with %d RemoveLabel calls", response.Results[0].Result, f.calls("RemoveLabel"))
	}
}

// The merge decides the way SetElement_test.go says it does, through the request path this time.
func TestTheMergeDecidesThroughTheRequestPath(t *testing.T) {
	// Every reading within the skew of server time, so that the bound leaves them alone and the
	// merge alone decides.
	added, _ := shared.NewHLC(now.Add(-4*time.Minute), 1, "server")
	removed, _ := shared.NewHLC(now.Add(-time.Minute), 1, "server")
	cases := map[string]struct {
		stored    []work.SetElement
		kind      domain.MutationKind
		tag       shared.HLC
		result    domain.ResultKind
		applied   string
		untouched string
	}{
		"an addition a later removal already undid is merged and applies nothing": {
			stored: []work.SetElement{{ElementID: labelX, AddedAt: added, RemovedAt: removed}},
			kind:   domain.SetAdd, tag: reading(-3*time.Minute, "dev-a"),
			result: domain.Merged, untouched: "AddLabel",
		},
		"a re-add later than the removal wins": {
			stored: []work.SetElement{{ElementID: labelX, AddedAt: added, RemovedAt: removed}},
			kind:   domain.SetAdd, tag: reading(0, "dev-a"),
			result: domain.Applied, applied: "AddLabel",
		},
		"a removal of an element this replica never saw is merged": {
			stored: nil, kind: domain.SetRemove, tag: reading(0, "dev-a"),
			result: domain.Merged, untouched: "RemoveLabel",
		},
		"a removal older than the addition loses": {
			stored: []work.SetElement{{ElementID: labelX, AddedAt: added}},
			kind:   domain.SetRemove, tag: reading(-4*time.Minute-time.Second, "dev-a"),
			result: domain.Merged, untouched: "RemoveLabel",
		},
		"a repeat of what the server holds changes nothing": {
			stored: []work.SetElement{{ElementID: labelX, AddedAt: added}},
			kind:   domain.SetAdd, tag: reading(0, "dev-a"),
			result: domain.Merged, untouched: "AddLabel",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := setting(t, tc.stored...)
			response, err := f.push.Push(t.Context(), pusher(), setOf(tc.kind, "labels", labelX, tc.tag))
			if err != nil {
				t.Fatalf("pushing: %v", err)
			}
			if response.Results[0].Result != tc.result {
				t.Errorf("answered %s, want %s", response.Results[0].Result, tc.result)
			}
			if tc.applied != "" && f.calls(tc.applied) != 1 {
				t.Errorf("%s was called %d times, want once", tc.applied, f.calls(tc.applied))
			}
			if tc.untouched != "" && f.calls(tc.untouched) != 0 {
				t.Errorf("%s was called", tc.untouched)
			}
		})
	}
}

func TestEachSetGoesToTheUseCaseThatOwnsIt(t *testing.T) {
	for set, want := range map[string]struct{ add, element string }{
		"members":     {"AddMember", "account_id"},
		"attachments": {"AttachMedia", "media_id"},
	} {
		f := setting(t)
		if _, err := f.push.Push(t.Context(), pusher(), setOf(domain.SetAdd, set, labelX, reading(0, "dev-a"))); err != nil {
			t.Fatalf("%s: %v", set, err)
		}
		found := false
		for _, call := range f.catalogue.calls {
			if call.name == want.add && call.in[want.element] == labelX.String() {
				found = true
			}
		}
		if !found {
			t.Errorf("%s went to %v, want %s with %s", set, f.catalogue.calls, want.add, want.element)
		}
	}
}

func TestASetChangeNeedsItsElement(t *testing.T) {
	f := setting(t)
	response, err := f.push.Push(t.Context(), pusher(), PushRequest{DeviceID: device, Mutations: []Mutation{{
		OpID: opA, Kind: domain.SetAdd, ItemID: itemX, Set: "labels", HLC: reading(0, "dev-a").String(),
	}}})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if result := response.Results[0]; result.Result != domain.Rejected || result.Error == nil || result.Error.MessageCode != "sync.element_required" {
		t.Errorf("answered %+v", result)
	}
}
