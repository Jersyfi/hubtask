// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	entryID  = shared.ID("01936f2a-7c1e-7000-8000-000000000f01")
	tenantID = shared.ID("01936f2a-7c1e-7000-8000-000000000f02")
	itemID   = shared.ID("01936f2a-7c1e-7000-8000-000000000f03")
	arrived  = time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
)

func validInput() jumble.NewEntryInput {
	return jumble.NewEntryInput{
		ID: entryID, TenantID: tenantID, Channel: jumble.ChannelAPI,
		Sender: "orders@example.org", RawSubject: "Order #42 needs a follow-up",
		RawBody: "The customer asked for a call back.",
		Now:     arrived,
	}
}

func TestAnArrivalIsStoredNewAndWhole(t *testing.T) {
	entry, err := jumble.NewEntry(validInput())
	if err != nil {
		t.Fatalf("a valid arrival was refused: %v", err)
	}
	if entry.Status != jumble.StatusNew {
		t.Errorf("status %q, want NEW - nothing has decided about it", entry.Status)
	}
	if entry.RawSubject != "Order #42 needs a follow-up" || entry.Sender != "orders@example.org" {
		t.Errorf("the arrival was rewritten: %+v", entry)
	}
	if entry.SettledAt != nil || !entry.TargetItemID.IsZero() {
		t.Errorf("a fresh entry claims a settlement: %+v", entry)
	}
	if !entry.ReceivedAt.Equal(arrived) {
		t.Errorf("received at %v", entry.ReceivedAt)
	}
}

// The bounds, each named at its field: a crafted payload costs a refusal rather than storage.
func TestAnArrivalIsBoundedAtTheDoor(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*jumble.NewEntryInput)
		code   string
	}{
		{"an unknown channel", func(in *jumble.NewEntryInput) { in.Channel = "CARRIER_PIGEON" }, "jumble.channel_unknown"},
		{"a subject past the bound", func(in *jumble.NewEntryInput) {
			in.RawSubject = strings.Repeat("s", jumble.MaxSubjectLength+1)
		}, "jumble.subject_too_long"},
		{"a body past the bound", func(in *jumble.NewEntryInput) {
			in.RawBody = strings.Repeat("b", jumble.MaxBodyBytes+1)
		}, "jumble.body_too_large"},
		{"a sender past the bound", func(in *jumble.NewEntryInput) {
			in.Sender = strings.Repeat("a", jumble.MaxSenderLength+1)
		}, "jumble.sender_too_long"},
		{"too many attachments", func(in *jumble.NewEntryInput) {
			for range jumble.MaxAttachments + 1 {
				in.Attachments = append(in.Attachments, itemID)
			}
		}, "jumble.too_many_attachments"},
		{"nothing in it", func(in *jumble.NewEntryInput) {
			in.RawSubject, in.RawBody, in.Attachments = "", "  ", nil
		}, "jumble.entry_empty"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mutate(&in)

			_, err := jumble.NewEntry(in)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want a validation refusal", err)
			}
			var coded *shared.Error
			if !errors.As(err, &coded) {
				t.Fatalf("no coded refusal: %v", err)
			}
			found := false
			for _, field := range coded.Fields {
				if field.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Errorf("refused with %+v, want %s", coded.Fields, tc.code)
			}
		})
	}
}

// An entry with only an attachment is still something to catch.
func TestAnAttachmentAloneIsEnough(t *testing.T) {
	in := validInput()
	in.RawSubject, in.RawBody = "", ""
	in.Attachments = []shared.ID{itemID}

	if _, err := jumble.NewEntry(in); err != nil {
		t.Fatalf("an attachment-only arrival was refused: %v", err)
	}
}

// The acceptance criterion: conversion settles the entry with its target, and a second conversion
// of the same entry is refused rather than producing a second item.
func TestConversionSettlesOnceAndOnlyOnce(t *testing.T) {
	entry, err := jumble.NewEntry(validInput())
	if err != nil {
		t.Fatalf("arriving: %v", err)
	}

	converted, err := entry.Convert(itemID, arrived.Add(time.Hour))
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if converted.Status != jumble.StatusProcessed || converted.TargetItemID != itemID {
		t.Errorf("the conversion reads %+v", converted)
	}
	if converted.SettledAt == nil {
		t.Error("the settlement has no moment")
	}

	if _, err := converted.Convert(itemID, arrived.Add(2*time.Hour)); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second conversion answered %v, want a conflict", err)
	}
	if _, err := converted.Dismiss(arrived.Add(2 * time.Hour)); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("dismissing a converted entry answered %v, want a conflict", err)
	}
}

// Dismissal is a state, not a deletion - and it settles the entry exactly as a conversion does.
func TestDismissalIsAStateAndSettles(t *testing.T) {
	entry, err := jumble.NewEntry(validInput())
	if err != nil {
		t.Fatalf("arriving: %v", err)
	}

	dismissed, err := entry.Dismiss(arrived.Add(time.Hour))
	if err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	if dismissed.Status != jumble.StatusDismissed || dismissed.SettledAt == nil {
		t.Errorf("the dismissal reads %+v", dismissed)
	}
	if dismissed.RawSubject == "" {
		t.Error("dismissal erased the entry - it is a state, not a deletion")
	}

	if _, err := dismissed.Convert(itemID, arrived.Add(2*time.Hour)); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("converting a dismissed entry answered %v, want a conflict", err)
	}
}
