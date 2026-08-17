// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package clock_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/clock"
)

var start = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)

// moving is a clock that advances by a millisecond on every reading, which is what an identifier
// generator has to be tested against: the ordering is the point of version 7.
type moving struct{ at time.Time }

func (m *moving) Now() time.Time {
	m.at = m.at.Add(time.Millisecond)
	return m.at
}

func TestTheIdentifiersAreValidUUIDv7(t *testing.T) {
	generator := clock.NewUUIDv7(port.Fixed(start))

	for range 100 {
		id := generator.NewID()

		if _, err := shared.ParseID(id.String()); err != nil {
			t.Fatalf("%q is not a canonical identifier: %v", id, err)
		}
		if !id.IsUUIDv7() {
			t.Fatalf("%q does not carry version 7", id)
		}
		// The variant nibble is 8, 9, a or b (RFC 9562 §4.1).
		if variant := id.String()[19]; variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
			t.Fatalf("%q carries the variant nibble %q", id, variant)
		}
	}
}

// Time-ordered is the property an index depends on: identifiers minted later sort later as text.
func TestTheIdentifiersSortInTheOrderTheyWereMinted(t *testing.T) {
	generator := clock.NewUUIDv7(&moving{at: start})

	ids := make([]string, 0, 200)
	for range 200 {
		ids = append(ids, generator.NewID().String())
	}

	if !slices.IsSorted(ids) {
		t.Error("the identifiers do not sort in the order they were minted")
	}
}

// Two identifiers from the same millisecond still differ: the timestamp is the first half, the
// randomness is the rest.
func TestTwoIdentifiersInOneMillisecondDiffer(t *testing.T) {
	generator := clock.NewUUIDv7(port.Fixed(start))

	seen := make(map[shared.ID]bool, 1000)
	for range 1000 {
		id := generator.NewID()
		if seen[id] {
			t.Fatalf("%q was handed out twice", id)
		}
		seen[id] = true
	}
}

// The timestamp is the first 48 bits, so an identifier says when it was minted - which is what
// makes it useful in a support conversation.
func TestTheTimestampIsTheOneFromTheClock(t *testing.T) {
	first := clock.NewUUIDv7(port.Fixed(start)).NewID().String()
	later := clock.NewUUIDv7(port.Fixed(start.Add(time.Hour))).NewID().String()

	if first[:8] == later[:8] {
		t.Errorf("two identifiers an hour apart share their timestamp: %s, %s", first, later)
	}
	if first >= later {
		t.Errorf("%s does not sort before %s", first, later)
	}
}
