// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

func TestOrderKeyBetweenPlacesTheKeyBetweenItsNeighbours(t *testing.T) {
	cases := []struct {
		name           string
		previous, next string
		want           string
	}{
		{name: "an empty list", want: "a0"},
		{name: "appending to the first", previous: "a0", want: "a1"},
		{name: "appending past the last digit", previous: "az", want: "b00"},
		{name: "before the first", next: "a0", want: "Zz"},
		{name: "before a key that already has a fraction", next: "a0V", want: "a0"},
		{name: "between two neighbours", previous: "a0", next: "a1", want: "a0V"},
		{name: "between two distant keys", previous: "a0", next: "a2", want: "a1"},
		{name: "between two fractions", previous: "a0V", next: "a0W", want: "a0VV"},
		{name: "sharing a long prefix", previous: "a0VVV", next: "a0VVW", want: "a0VVVV"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key, err := service.OrderKeyBetween(c.previous, c.next)
			if err != nil {
				t.Fatalf("no key between %q and %q: %v", c.previous, c.next, err)
			}
			if key != c.want {
				t.Errorf("key %q, want %q", key, c.want)
			}
			if c.previous != "" && key <= c.previous {
				t.Errorf("%q does not sort after %q", key, c.previous)
			}
			if c.next != "" && key >= c.next {
				t.Errorf("%q does not sort before %q", key, c.next)
			}
		})
	}
}

// The property the scheme exists for: a list can be inserted into for ever without renumbering
// anything. A thousand insertions at the same place is the worst case - each one halves the gap
// the next has to fit into.
func TestRepeatedInsertionAtTheSamePlaceStaysOrdered(t *testing.T) {
	lower, upper := "a0", "a1"

	for i := range 1000 {
		key, err := service.OrderKeyBetween(lower, upper)
		if err != nil {
			t.Fatalf("insertion %d failed: %v", i, err)
		}
		if key <= lower || key >= upper {
			t.Fatalf("insertion %d produced %q, which is not between %q and %q", i, key, lower, upper)
		}
		// Insert immediately below the previous insertion, so the gap shrinks from both sides.
		upper = key
	}
}

// Appending is the ordinary case, and the reason the integer part carries its own length: the
// thousandth key has to stay short. A scheme without that encoding would produce one digit per
// append.
func TestAppendingKeepsTheOrderAndStaysShort(t *testing.T) {
	keys := make([]string, 0, 1000)
	last := ""

	for i := range 1000 {
		key, err := service.OrderKeyAfter(last)
		if err != nil {
			t.Fatalf("append %d failed: %v", i, err)
		}
		keys = append(keys, key)
		last = key
	}

	if !slices.IsSorted(keys) {
		t.Error("appended keys do not sort in the order they were created")
	}
	if length := len(keys[len(keys)-1]); length > 4 {
		t.Errorf("the thousandth appended key is %d digits long: %q", length, keys[len(keys)-1])
	}
}

// Moving to the top a thousand times has to stay ordered too - that is the negative half of the
// integer space, which exists for exactly this.
func TestPrependingKeepsTheOrder(t *testing.T) {
	first := ""

	for i := range 1000 {
		key, err := service.OrderKeyBetween("", first)
		if err != nil {
			t.Fatalf("prepend %d failed: %v", i, err)
		}
		if first != "" && key >= first {
			t.Fatalf("prepend %d produced %q, which does not sort before %q", i, key, first)
		}
		first = key
	}
	if len(first) > 4 {
		t.Errorf("the thousandth prepended key is %d digits long: %q", len(first), first)
	}
}

// A key nothing here produced is a row written by something that does not share this scheme.
// Continuing would place the new key in an unpredictable position, so it fails as a defect.
func TestOrderKeyRefusesWhatItDidNotProduce(t *testing.T) {
	cases := []struct {
		name           string
		previous, next string
		detailCode     string
	}{
		{name: "a character outside the alphabet", previous: "a0!", detailCode: "ordering.key_malformed"},
		{name: "a head that is not a letter", previous: "00", detailCode: "ordering.key_malformed"},
		{name: "an integer part shorter than its head declares", previous: "b0", detailCode: "ordering.key_malformed"},
		{name: "a fraction ending in the lowest digit", previous: "a0V0", detailCode: "ordering.key_malformed"},
		{name: "the smallest integer itself", next: "A" + strings.Repeat("0", 26), detailCode: "ordering.key_malformed"},
		{name: "bounds the wrong way round", previous: "a1", next: "a0", detailCode: "ordering.bounds_invalid"},
		{name: "identical bounds", previous: "a0", next: "a0", detailCode: "ordering.bounds_invalid"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := service.OrderKeyBetween(c.previous, c.next)
			if err == nil {
				t.Fatalf("no error, want %s", c.detailCode)
			}
			if !errors.Is(err, shared.ErrInternal) {
				t.Errorf("category %s, want INTERNAL", shared.AsError(err).Category)
			}
			if got := shared.AsError(err).DetailCode; got != c.detailCode {
				t.Errorf("detail code %s, want %s", got, c.detailCode)
			}
		})
	}
}

// Every key this package produces has to be a key it accepts again - otherwise the second
// insertion at the same place fails on a row the first one wrote.
func TestEveryProducedKeyIsAcceptedAgain(t *testing.T) {
	previous := ""
	for range 200 {
		key, err := service.OrderKeyAfter(previous)
		if err != nil {
			t.Fatalf("append after %q failed: %v", previous, err)
		}
		if _, err := service.OrderKeyBetween(previous, key); err != nil {
			t.Fatalf("nothing fits between %q and %q: %v", previous, key, err)
		}
		if _, err := service.OrderKeyAfter(key); err != nil {
			t.Fatalf("nothing fits after %q: %v", key, err)
		}
		previous = key
	}
}

// The top of the integer space is not the end of the list: the key grows a fraction instead, and
// the order holds across the transition.
func TestTheTopOfTheIntegerSpaceGrowsAFraction(t *testing.T) {
	last := "z" + strings.Repeat("z", 26)

	key, err := service.OrderKeyAfter(last)
	if err != nil {
		t.Fatalf("appending after the largest integer failed: %v", err)
	}
	if key <= last {
		t.Errorf("%q does not sort after %q", key, last)
	}
	if !strings.HasPrefix(key, last) {
		t.Errorf("%q is not the largest integer plus a fraction", key)
	}
}
