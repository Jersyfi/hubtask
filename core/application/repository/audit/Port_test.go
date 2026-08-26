// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
)

// The port carries no logic, so there is nothing here to measure - only two things to hold in
// place. A fake can implement it, which is what the use case tests depend on; and it is read-only,
// which is not a convention but the reason the whole trail can be trusted (audit.md §3).

type double struct{ stored []Record }

func (d *double) Query(_ context.Context, filter Filter) (RecordPage, error) {
	var out []Record
	for _, record := range d.stored {
		if !filter.ActorID.IsZero() && record.Entry.ActorID != filter.ActorID {
			continue
		}
		out = append(out, record)
	}
	return RecordPage{Records: out}, nil
}

func (d *double) Walk(_ context.Context, _ Period, yield func(Record) error) error {
	for _, record := range d.stored {
		if err := yield(record); err != nil {
			return err
		}
	}
	return nil
}

func (d *double) LatestAnchor(context.Context) (Anchor, error) { return Anchor{}, nil }

var _ Trail = (*double)(nil)

func TestTheTrailIsReadableAndNarrowable(t *testing.T) {
	mine := shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	theirs := shared.MustParseID("0192f000-0000-7000-8000-0000000000a3")

	trail := &double{stored: []Record{
		{Seq: 1, Entry: port.Entry{ActorID: mine, OccurredAt: time.Now()}},
		{Seq: 2, Entry: port.Entry{ActorID: theirs, OccurredAt: time.Now()}},
	}}

	page, err := trail.Query(t.Context(), Filter{ActorID: mine})
	if err != nil {
		t.Fatalf("reading failed: %v", err)
	}
	if len(page.Records) != 1 || page.Records[0].Seq != 1 {
		t.Errorf("narrowing to one actor answered %+v", page.Records)
	}
	if page.Info.HasMore || page.Info.NextCursor != "" {
		t.Errorf("an exhausted walk reports %+v", page.Info)
	}
}

// The walk is the shape a verification and an export both need: one entry at a time, and an error
// from the caller ends it. A slice would read a year of evidence into memory before the first link
// was checked.
func TestAWalkStopsWhenTheCallerHasSeenEnough(t *testing.T) {
	trail := &double{stored: []Record{{Seq: 1}, {Seq: 2}, {Seq: 3}}}

	seen := 0
	stop := errors.New("enough")
	err := trail.Walk(t.Context(), Period{}, func(Record) error {
		seen++
		if seen == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) {
		t.Errorf("the walk answered %v, want the caller's own error", err)
	}
	if seen != 2 {
		t.Errorf("the walk handed over %d entries after being stopped at the second", seen)
	}
}

// Nothing anchors yet, and the zero anchor is what says so. A verification has to be able to
// answer "nothing is sealed" rather than leave the question unasked (audit.md §3).
func TestAnUnanchoredTrailSaysSoRatherThanClaimingASeal(t *testing.T) {
	anchor, err := (&double{}).LatestAnchor(t.Context())
	if err != nil {
		t.Fatalf("reading the anchor: %v", err)
	}
	if !anchor.IsZero() {
		t.Errorf("an installation that has never anchored reports %+v", anchor)
	}
}

// The record carries what the chain added. A reader that saw only what a use case wrote could not
// check anything: `:verify` recomputes the digest over the stored entry and compares it with the
// hash beside it.
func TestARecordCarriesItsPlaceInTheChain(t *testing.T) {
	record := Record{Seq: 9, Hash: []byte{0x01}, PrevHash: nil}

	if record.Seq != 9 || len(record.Hash) == 0 {
		t.Errorf("the record lost its place in the chain: %+v", record)
	}
	if record.PrevHash != nil {
		t.Error("the first entry of a chain has no predecessor, and that is not the same as an empty one")
	}
}
