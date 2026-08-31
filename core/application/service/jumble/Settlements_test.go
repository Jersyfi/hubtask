// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
// create, records the provenance on both sides, settles the entry, and announces it.
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
}

// The acceptance criterion, second half: a second conversion of the same entry is refused.
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
}

// A title is derived from the body's first line when there is no subject, and an entry with no
// text at all needs one in the request.
func TestTheTitleIsDerivedOrDemanded(t *testing.T) {
	entry := domain.Entry{RawBody: "First line of the body\nSecond line"}
	title, err := titleFor(entry, "")
	if err != nil || title != "First line of the body" {
		t.Errorf("derived %q / %v", title, err)
	}

	if _, err := titleFor(domain.Entry{}, ""); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an entry with no text answered %v", err)
	}
	if title, _ := titleFor(domain.Entry{}, "By hand"); title != "By hand" {
		t.Errorf("the caller's title was rewritten to %q", title)
	}
}

// Dismissal settles without touching the item side, and stays readable.
func TestADismissalSettlesAndKeepsTheEntry(t *testing.T) {
	h := newHarness()
	seedEntry(t, h, "body")

	entry, err := (DismissJumbleEntry{Writer: h.writer}).Execute(
		context.Background(), actor(), entryID)
	if err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	if entry.Status != domain.StatusDismissed || entry.RawSubject == "" {
		t.Errorf("the dismissed entry reads %+v", entry)
	}
	if len(h.events.envelopes) != 0 {
		t.Error("a dismissal announced an event")
	}

	if _, err := (DismissJumbleEntry{Writer: h.writer}).Execute(
		context.Background(), actor(), entryID); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second dismissal answered %v, want a conflict", err)
	}
}
