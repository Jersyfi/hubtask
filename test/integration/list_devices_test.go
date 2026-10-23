// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
)

// UC-WRK-25 on two phones in the shop (checks 3, 4 and 6): device A ticks "milk", and device B,
// pulling from where it stood, learns that it is done, how often it has been, and its amount -
// everything its list needs to show the entry under *Done*. Then B reopens it from *Recently* and A
// learns that, the amount kept. And a change of the amount on one device and of the title on the
// other both survive (SY-1 for the field a list adds).

// pullDelta walks a device from its cursor to the end and answers the item records it was told,
// payload by payload, with the cursor it now stands at.
func pullDelta(
	ctx context.Context, t *testing.T, pull syncservice.PullChanges, actor appshared.ActorContext,
	device shared.ID, cursor string,
) ([]map[string]any, string) {
	t.Helper()
	var payloads []map[string]any
	request := syncservice.PullRequest{DeviceID: device, Cursor: cursor, Limit: 500}
	for range 1000 {
		page, err := pull.Pull(ctx, actor, request)
		if err != nil {
			t.Fatalf("pulling: %v", err)
		}
		for _, record := range page.Records {
			if record.Entity == "item" && record.Payload != nil {
				payloads = append(payloads, record.Payload)
			}
		}
		request.Cursor = pull.Encode(page.Cursor)
		if !page.More {
			return payloads, request.Cursor
		}
	}
	t.Fatal("the pull does not end")
	return nil, ""
}

func told(payloads []map[string]any, field string) (any, bool) {
	var value any
	found := false
	for _, payload := range payloads {
		if v, present := payload[field]; present {
			value, found = v, true
		}
	}
	return value, found
}

func TestTwoDevicesSeeAnEntryTickedOnOneAsDoneOnTheOther(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := listCollectionFor(ctx, t, tenantB, authorB)
	created, err := itemCatalogueFor(ctx, t).Invoke(ctx, "CreateWorkItem", itemWriter(tenantB, authorB), usecase.Input{
		"type": "TASK", "collection_id": collection.String(), "title": "milk", "amount": "2",
	})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	milk := shared.MustParseID(created.String("id"))

	push := mergingPushFor(ctx, t)
	pull := pullFor(ctx, t)
	actor := pushActor(tenantB, authorB)
	wall := time.Now()
	deviceA, deviceB := freshID(t), freshID(t)

	// B stands somewhere: its walk ends on a delta cursor after the entry was created.
	_, cursorB := pullDelta(ctx, t, pull, actor, deviceB, "")

	tickedOnA, _ := shared.NewHLC(wall.Add(time.Minute), 1, deviceA.String())
	answer, err := push.Push(ctx, actor, syncservice.PushRequest{DeviceID: deviceA, Mutations: []syncservice.Mutation{
		patchMutation(t, milk, "completion", map[string]any{"is_completed": true}, tickedOnA),
	}})
	if err != nil || answer.Results[0].Result != syncdomain.Applied {
		t.Fatalf("A's tick answered %+v, %v", answer, err)
	}

	payloads, cursorB := pullDelta(ctx, t, pull, actor, deviceB, cursorB)
	completion, _ := told(payloads, "completion")
	if done, _ := completion.(map[string]any)["is_completed"].(bool); !done {
		t.Fatalf("B was not told the entry is done: %v", payloads)
	}
	if count, found := told(payloads, "completion_count"); !found || count != float64(1) && count != 1 {
		t.Errorf("B was told the count %#v (found %v), want 1", count, found)
	}
	if stored := findWorkItem(ctx, t, tenantB, milk); stored.Amount != "2" || stored.CompletionCount != 1 {
		t.Errorf("the entry is %q, counted %d", stored.Amount, stored.CompletionCount)
	}

	// B brings it back from *Recently*: the same entry reopens, its amount kept, its count too.
	reopenedOnB, _ := shared.NewHLC(wall.Add(2*time.Minute), 1, deviceB.String())
	answer, err = push.Push(ctx, actor, syncservice.PushRequest{DeviceID: deviceB, Mutations: []syncservice.Mutation{
		patchMutation(t, milk, "completion", map[string]any{"is_completed": false}, reopenedOnB),
	}})
	if err != nil || answer.Results[0].Result != syncdomain.Applied {
		t.Fatalf("B's reopen answered %+v, %v", answer, err)
	}
	if stored := findWorkItem(ctx, t, tenantB, milk); stored.Completion.IsCompleted || stored.Amount != "2" ||
		stored.CompletionCount != 1 {
		t.Errorf("after the reopen the entry is %+v, %q, counted %d", stored.Completion, stored.Amount, stored.CompletionCount)
	}

	// The amount on A and the title on B, concurrently: both survive.
	amountOnA, _ := shared.NewHLC(wall.Add(3*time.Minute), 1, deviceA.String())
	titleOnB, _ := shared.NewHLC(wall.Add(4*time.Minute), 1, deviceB.String())
	if _, err := push.Push(ctx, actor, syncservice.PushRequest{DeviceID: deviceA, Mutations: []syncservice.Mutation{
		patchMutation(t, milk, "amount", "3", amountOnA),
	}}); err != nil {
		t.Fatalf("A's amount: %v", err)
	}
	if _, err := push.Push(ctx, actor, syncservice.PushRequest{DeviceID: deviceB, Mutations: []syncservice.Mutation{
		patchMutation(t, milk, "title", "oat milk", titleOnB),
	}}); err != nil {
		t.Fatalf("B's title: %v", err)
	}
	if stored := findWorkItem(ctx, t, tenantB, milk); stored.Amount != "3" || stored.Title != "oat milk" {
		t.Errorf("the entry ended as %q %q; one device's change was lost", stored.Amount, stored.Title)
	}
	payloads, _ = pullDelta(ctx, t, pull, actor, deviceB, cursorB)
	if amount, _ := told(payloads, "amount"); amount != "3" {
		t.Errorf("B was told the amount %#v", amount)
	}
}
