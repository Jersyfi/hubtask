// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package realtime

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestARefNeedsAKindAndAnIdentifier(t *testing.T) {
	id := shared.MustParseID("01936f2a-7c1e-7000-8000-000000000001")
	cases := []struct {
		name string
		kind string
		id   shared.ID
		ok   bool
	}{
		{"a page", "page", id, true},
		{"a two-word kind", "white_board", id, true},
		{"upper case", "Page", id, false},
		{"one letter", "p", id, false},
		{"a digit", "page2", id, false},
		{"no identifier", "page", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewRef(c.kind, c.id)
			if (err == nil) != c.ok {
				t.Errorf("NewRef(%q) = %v, want ok %v", c.kind, err, c.ok)
			}
		})
	}
}

func TestAnUpdateIsBounded(t *testing.T) {
	cases := []struct {
		name string
		size int
		code string
	}{
		{"empty", 0, "realtime.update_empty"},
		{"one byte", 1, ""},
		{"at the bound", MaxUpdateBytes, ""},
		{"past the bound", MaxUpdateBytes + 1, "realtime.update_too_large"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckUpdate(make([]byte, c.size))
			if got := detail(err); got != c.code {
				t.Errorf("CheckUpdate(%d bytes) = %q, want %q", c.size, got, c.code)
			}
		})
	}
}

func TestASnapshotCoversOnlyWhatWasThereAndOnlyMoreThanBefore(t *testing.T) {
	state := []byte{1}
	cases := []struct {
		name    string
		snap    Snapshot
		head    int64
		current int64
		code    string
	}{
		{"the head", Snapshot{UpToSeq: 10, State: state}, 10, 0, ""},
		{"behind the head", Snapshot{UpToSeq: 4, State: state}, 10, 2, ""},
		{"past the head", Snapshot{UpToSeq: 11, State: state}, 10, 0, "realtime.snapshot_beyond_head"},
		{"nothing", Snapshot{UpToSeq: 0, State: state}, 10, 0, "realtime.snapshot_beyond_head"},
		{"the one held", Snapshot{UpToSeq: 5, State: state}, 10, 5, "realtime.snapshot_superseded"},
		{"older than the one held", Snapshot{UpToSeq: 3, State: state}, 10, 5, "realtime.snapshot_superseded"},
		{"empty", Snapshot{UpToSeq: 3}, 10, 0, "realtime.snapshot_empty"},
		{"too large", Snapshot{UpToSeq: 3, State: make([]byte, MaxSnapshotBytes+1)}, 10, 0, "realtime.snapshot_too_large"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detail(CheckSnapshot(c.snap, c.head, c.current)); got != c.code {
				t.Errorf("CheckSnapshot = %q, want %q", got, c.code)
			}
		})
	}
}

func TestCompactionIsDueAfterEnoughUpdates(t *testing.T) {
	if (Head{Seq: CompactAfter - 1}).CompactionDue() {
		t.Error("compaction is due before the bound")
	}
	if !(Head{Seq: CompactAfter + 7, SnapshotSeq: 7}).CompactionDue() {
		t.Error("compaction is not due at the bound past the snapshot")
	}
}

func TestAwarenessIsBounded(t *testing.T) {
	if CheckAwareness(nil) == nil || CheckAwareness(make([]byte, MaxAwarenessBytes+1)) == nil {
		t.Error("an empty or oversized awareness message passed")
	}
	if err := CheckAwareness(make([]byte, MaxAwarenessBytes)); err != nil {
		t.Errorf("an awareness message at the bound was refused: %v", err)
	}
}

func detail(err error) string {
	if err == nil {
		return ""
	}
	var coded *shared.Error
	if errors.As(err, &coded) {
		return coded.DetailCode
	}
	return err.Error()
}
