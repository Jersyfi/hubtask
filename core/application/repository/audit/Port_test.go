// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"context"
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
