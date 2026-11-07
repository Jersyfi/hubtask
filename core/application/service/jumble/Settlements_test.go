// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var (
	collectionID = shared.ID("01936f2a-7c1e-7000-8000-000000001020")
	createdItem  = shared.ID("01936f2a-7c1e-7000-8000-000000001021")
)

// registry is the catalogue slice: it performs CreateWorkItem and records what it was asked.
type registry struct {
	invoked []usecase.Input
	actor   appshared.ActorContext
	err     error
}

func (r *registry) Invoke(
	_ context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if name != "CreateWorkItem" {
		return nil, shared.ErrInternal.WithDetail("jumble.entry_incomplete")
	}
	r.invoked, r.actor = append(r.invoked, in), actor
	if r.err != nil {
		return nil, r.err
	}
	return usecase.Output{"id": createdItem.String()}, nil
}

// origins records the provenance writes.
type origins struct{ recorded map[shared.ID]shared.ID }

func (o *origins) RecordOrigin(_ context.Context, itemID, entryID shared.ID) (bool, error) {
	if o.recorded == nil {
		o.recorded = map[shared.ID]shared.ID{}
	}
	if _, taken := o.recorded[itemID]; taken {
		return false, nil
	}
	o.recorded[itemID] = entryID
	return true, nil
}

func seedEntry(t *testing.T, h *harness, body string) domain.Entry {
	t.Helper()
	entry, err := domain.NewEntry(domain.NewEntryInput{
		ID: entryID, TenantID: tenant, Channel: domain.ChannelAPI,
		RawSubject: "Order #42", RawBody: body, Now: now,
	})
	if err != nil {
		t.Fatalf("building the entry: %v", err)
	}
	if err := h.store.Insert(context.Background(), entry); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	return entry
}

// The acceptance criterion: conversion creates the item at the named destination through the item
// create, records the provenance on both sides, settles the entry, and announces it; the trail
// holds the conversion beside the create's own entry (UC-JUM-05 checks 1, 2 and 8).
func TestAConversionCreatesRecordsAndSettles(t *testing.T) {
	h := newHarness()
	catalogue := &registry{}
	provenance := &origins{}
	seedEntry(t, h, "The customer asked for a call back.")

	entry, err := (ConvertJumbleEntry{Writer: h.writer, Catalogue: catalogue, Origins: provenance}).
		Execute(context.Background(), actor(), ConvertCommand{
			EntryID: entryID, CollectionID: collectionID,
		})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}

	if entry.Status != domain.StatusProcessed || entry.TargetItemID != createdItem {
		t.Errorf("the settled entry reads %+v", entry)
	}
	if len(catalogue.invoked) != 1 {
		t.Fatalf("the item create was invoked %d times", len(catalogue.invoked))
	}
	in := catalogue.invoked[0]
	if in["collection_id"] != collectionID.String() || in["type"] != "TASK" {
		t.Errorf("the create was asked %v", in)
	}
	// The title defaults to the entry's subject.
	if in["title"] != "Order #42" {
		t.Errorf("the title reads %v", in["title"])
	}
	// As the caller - a rule's run_as converts with its real rights.
	if catalogue.actor.AccountID != account {
		t.Errorf("the create ran as %s", catalogue.actor.AccountID)
	}
	if provenance.recorded[createdItem] != entryID {
		t.Error("the provenance was not recorded on the item")
	}
	if len(h.events.envelopes) != 1 || h.events.envelopes[0].Type != event.JumbleEntryConverted {
		t.Errorf("announced %+v", h.events.envelopes)
	}
	if h.events.envelopes[0].Payload["collection_id"] != collectionID.String() {
		t.Errorf("the event payload reads %v", h.events.envelopes[0].Payload)
	}
	if len(h.sink.entries) != 1 || h.sink.entries[0].Action != EntryConvertedAction {
		t.Errorf("audited %+v", h.sink.entries)
	}
}

// The kind and the column are the request's: a task unless it names another level, and the
// destination's board column passed on (UC-JUM-05 check 3).
func TestAConversionHonoursTheKindAndTheColumn(t *testing.T) {
	bucket := shared.ID("01936f2a-7c1e-7000-8000-000000001031")
	for _, kind := range []string{"WORK_PACKAGE", "ACTIVITY"} {
		h := newHarness()
		catalogue := &registry{}
		seedEntry(t, h, "body")
		if _, err := (ConvertJumbleEntry{Writer: h.writer, Catalogue: catalogue, Origins: &origins{}}).
			Execute(context.Background(), actor(), ConvertCommand{
				EntryID: entryID, CollectionID: collectionID, Type: kind, BucketID: bucket,
			}); err != nil {
			t.Fatalf("%s: converting: %v", kind, err)
		}
		if in := catalogue.invoked[0]; in["type"] != kind || in["bucket_id"] != bucket.String() {
			t.Errorf("%s: the create was asked %v", kind, in)
		}
	}
}

// A conversion without a destination is refused before anything is read (UC-JUM-05 check 4).
func TestAConversionNeedsADestination(t *testing.T) {
	h := newHarness()
	catalogue := &registry{}
	seedEntry(t, h, "body")
	_, err := (ConvertJumbleEntry{Writer: h.writer, Catalogue: catalogue, Origins: &origins{}}).
		Execute(context.Background(), actor(), ConvertCommand{EntryID: entryID})
	if code := shared.AsError(err).DetailCode; !errors.Is(err, shared.ErrValidation) || code != "jumble.destination_required" {
		t.Fatalf("answered %v (%s), want jumble.destination_required", err, code)
	}
	if len(catalogue.invoked) != 0 {
		t.Error("a conversion without a destination created an entry")
	}
}

// The destination's rights are the create's: refused there, the conversion is refused as a plain
// create would be and the jumble entry stays undecided (UC-JUM-05 check 5).
func TestTheDestinationsRightsDecide(t *testing.T) {
	h := newHarness()
	catalogue := &registry{err: shared.ErrForbidden}
	seedEntry(t, h, "body")
	_, err := (ConvertJumbleEntry{Writer: h.writer, Catalogue: catalogue, Origins: &origins{}}).
		Execute(context.Background(), actor(), ConvertCommand{EntryID: entryID, CollectionID: collectionID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("answered %v, want forbidden", err)
	}
	if stored := h.store.rows[entryID]; stored.Status != domain.StatusNew {
		t.Errorf("the refused conversion left the entry %s", stored.Status)
	}
}

// The acceptance criterion, second half: a second conversion of the same entry is refused with
// jumble.entry_settled, inside the transaction that rolls its create back (UC-JUM-05 check 6; the
// race is the integration test's). A rule's convert action is this use case, so a run reaching
// an entry somebody settled is refused the same way (UC-JUM-08 check 5).
func TestASecondConversionIsRefused(t *testing.T) {
	h := newHarness()
	catalogue := &registry{}
	provenance := &origins{}
	seedEntry(t, h, "body")
	convert := ConvertJumbleEntry{Writer: h.writer, Catalogue: catalogue, Origins: provenance}

	if _, err := convert.Execute(context.Background(), actor(), ConvertCommand{
		EntryID: entryID, CollectionID: collectionID,
	}); err != nil {
		t.Fatalf("the first conversion: %v", err)
	}
	_, err := convert.Execute(context.Background(), actor(), ConvertCommand{
		EntryID: entryID, CollectionID: collectionID,
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("the second conversion answered %v, want a conflict", err)
	}
	if code := shared.AsError(err).DetailCode; code != "jumble.entry_settled" {
		t.Errorf("the second conversion answered code %s", code)
	}
}

// A title is derived from the body's first line when there is no subject, and an entry with no
// text at all needs one in the request (UC-JUM-05 check 2).
func TestTheTitleIsDerivedOrDemanded(t *testing.T) {
	entry := domain.Entry{RawBody: "First line of the body\nSecond line"}
	title, err := titleFor(entry, "")
	if err != nil || title != "First line of the body" {
		t.Errorf("derived %q / %v", title, err)
	}

	if _, err := titleFor(domain.Entry{}, ""); !errors.Is(err, shared.ErrValidation) ||
		shared.AsError(err).DetailCode != "jumble.title_required" {
		t.Errorf("an entry with no text answered %v", err)
	}
	if title, _ := titleFor(domain.Entry{}, "By hand"); title != "By hand" {
		t.Errorf("the caller's title was rewritten to %q", title)
	}
}

// Dismissal settles without touching the item side, stays readable, is audited without content,
// and a settled entry is neither dismissed nor converted again (UC-JUM-06 checks 1, 2 and 7).
func TestADismissalSettlesAndKeepsTheEntry(t *testing.T) {
	h := newHarness()
	seedEntry(t, h, "The customer asked for a call back.")

	entry, err := (DismissJumbleEntry{Writer: h.writer}).Execute(
		context.Background(), actor(), entryID)
	if err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	if entry.Status != domain.StatusDismissed || entry.RawSubject != "Order #42" ||
		entry.RawBody != "The customer asked for a call back." {
		t.Errorf("the dismissed entry reads %+v", entry)
	}
	if len(h.sink.entries) != 1 || h.sink.entries[0].Action != EntryDismissedAction {
		t.Fatalf("audited %+v", h.sink.entries)
	}
	if trail := fmt.Sprintf("%+v", h.sink.entries[0]); strings.Contains(trail, "Order #42") ||
		strings.Contains(trail, "call back") {
		t.Errorf("the dismissal's trail carries the content: %s", trail)
	}
	if len(h.events.envelopes) != 0 {
		t.Error("a dismissal announced an event")
	}

	if _, err := (DismissJumbleEntry{Writer: h.writer}).Execute(
		context.Background(), actor(), entryID); !errors.Is(err, shared.ErrConflict) ||
		shared.AsError(err).DetailCode != "jumble.entry_settled" {
		t.Errorf("a second dismissal answered %v, want jumble.entry_settled", err)
	}
	catalogue := &registry{}
	if _, err := (ConvertJumbleEntry{Writer: h.writer, Catalogue: catalogue, Origins: &origins{}}).
		Execute(context.Background(), actor(), ConvertCommand{EntryID: entryID, CollectionID: collectionID}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("converting a dismissed entry answered %v, want a conflict", err)
	}
}

// A converted entry cannot be dismissed (UC-JUM-06 check 2).
func TestAConvertedEntryCannotBeDismissed(t *testing.T) {
	h := newHarness()
	seedEntry(t, h, "body")
	if _, err := (ConvertJumbleEntry{Writer: h.writer, Catalogue: &registry{}, Origins: &origins{}}).
		Execute(context.Background(), actor(), ConvertCommand{EntryID: entryID, CollectionID: collectionID}); err != nil {
		t.Fatalf("converting: %v", err)
	}
	_, err := (DismissJumbleEntry{Writer: h.writer}).Execute(context.Background(), actor(), entryID)
	if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).DetailCode != "jumble.entry_settled" {
		t.Errorf("dismissing a converted entry answered %v, want jumble.entry_settled", err)
	}
}

// Dismissing asks the right capturing asks; refused, the entry stays undecided (UC-JUM-06 check 3).
func TestDismissingNeedsTheCapturesRight(t *testing.T) {
	h := newHarness()
	seedEntry(t, h, "body")
	h.auth.refuse = shared.ErrForbidden
	_, err := (DismissJumbleEntry{Writer: h.writer}).Execute(context.Background(), actor(), entryID)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("answered %v, want forbidden", err)
	}
	if len(h.auth.requests) != 1 || h.auth.requests[0].Permission != service.PermissionWriteItems {
		t.Errorf("asked %+v, want the permission a capture asks", h.auth.requests)
	}
	if h.store.rows[entryID].Status != domain.StatusNew {
		t.Error("a refused dismissal settled the entry")
	}
}
