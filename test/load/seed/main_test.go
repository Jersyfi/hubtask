// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"strings"
	"testing"
)

// The property the baselines rest on. A guard compares two runs against a stored figure, and if
// the dataset underneath them differed the comparison would be measuring the dataset.
func TestTheSameSeedProducesTheSameRows(t *testing.T) {
	var first, second bytes.Buffer
	if err := write(&first, tableWorkItem, 4, 400, "a-seed"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := write(&second, tableWorkItem, 4, 400, "a-seed"); err != nil {
		t.Fatalf("%v", err)
	}
	if first.String() != second.String() {
		t.Error("two runs of the generator produced different rows")
	}

	var other bytes.Buffer
	if err := write(&other, tableWorkItem, 4, 400, "another-seed"); err != nil {
		t.Fatalf("%v", err)
	}
	if other.String() == first.String() {
		t.Error("a different seed produced the same rows, so the seed does nothing")
	}
}

// The count is exact, because "about two million" is not a dataset anybody can compare against.
func TestTheDatasetHoldsExactlyWhatWasAskedFor(t *testing.T) {
	var out bytes.Buffer
	if err := write(&out, tableWorkItem, 200, 200_000, "a-seed"); err != nil {
		t.Fatalf("%v", err)
	}
	if got := strings.Count(out.String(), "\n"); got != 200_000 {
		t.Errorf("%d items, want 200000", got)
	}
}

// The decay with items per tenant is the figure H-11 records, and an even split would have none to
// measure. The shape asserted here is the shape, not the arithmetic: the largest tenant holds far
// more than the smallest, and the smallest still holds something.
func TestTheDistributionIsALongTail(t *testing.T) {
	shares := distribute(2_000_000, 200)

	total := 0
	for _, share := range shares {
		total += share
		if share < 1 {
			t.Fatal("a tenant was given no items at all")
		}
	}
	if total != 2_000_000 {
		t.Errorf("the shares add up to %d, want 2000000", total)
	}
	if shares[0] < 20*shares[len(shares)-1] {
		t.Errorf("largest %d against smallest %d - that is not a tail", shares[0], shares[len(shares)-1])
	}
	for rank := 1; rank < len(shares); rank++ {
		if shares[rank] > shares[rank-1] {
			t.Fatalf("rank %d holds more than rank %d", rank, rank-1)
		}
	}
}

// A tiny dataset is where an off-by-one hides: one item per tenant, and every tenant must still
// get a row rather than the first one getting all of them.
func TestEveryTenantGetsAtLeastOneItem(t *testing.T) {
	shares := distribute(10, 10)
	for rank, share := range shares {
		if share < 1 {
			t.Errorf("tenant %d got %d items", rank, share)
		}
	}
}

// A work package with no parent is a row the constraint refuses, and a path that does not begin
// with its parent's is a subtree the application cannot walk. Both are silent until the COPY
// fails half way through two million rows.
func TestAChildCarriesItsParentAndItsParentsPath(t *testing.T) {
	var out bytes.Buffer
	if err := write(&out, tableWorkItem, 1, 40, "a-seed"); err != nil {
		t.Fatalf("%v", err)
	}

	children := 0
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		fields := strings.Split(line, "\t")
		id, kind, parent, path, depth := fields[0], fields[3], fields[4], fields[5], fields[6]
		switch kind {
		case "TASK":
			if parent != `\N` || depth != "0" || path != "/"+id+"/" {
				t.Errorf("a task carries parent %q, depth %q, path %q", parent, depth, path)
			}
		case "WORK_PACKAGE":
			children++
			if parent == `\N` || depth != "1" {
				t.Errorf("a work package carries parent %q and depth %q", parent, depth)
			}
			if !strings.HasPrefix(path, "/"+parent+"/") || !strings.HasSuffix(path, id+"/") {
				t.Errorf("the child's path %q does not descend from its parent", path)
			}
		default:
			t.Errorf("unexpected type %q", kind)
		}
	}
	if children == 0 {
		t.Error("the dataset is flat; nothing exercises the path index")
	}
}

// Every field is written unescaped, which is only safe while nothing in a row can contain a tab, a
// newline or a backslash. The one field that is not hexadecimal or an enum is the title, so that
// is the one worth pinning.
func TestNoFieldNeedsEscaping(t *testing.T) {
	var out bytes.Buffer
	for _, table := range []string{tableTenant, tableAccount, tableMembership, tableContainer, tableWorkItem} {
		out.Reset()
		if err := write(&out, table, 3, 60, "a-seed"); err != nil {
			t.Fatalf("%v", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			for _, field := range strings.Split(line, "\t") {
				if strings.ContainsAny(field, "\\\r\n") && field != `\N` {
					t.Errorf("%s: %q would have to be escaped", table, field)
				}
			}
		}
	}
}

// The keys have to be the application's own fractional index, not merely something sortable.
// Inserting after the last row asks the domain for a key above the existing maximum and it
// validates the bound it is given - so a dataset carrying keys of another shape answers every
// insert with an internal error. RT-6's first run found exactly that, as a third of its requests
// being a five hundred.
func TestTheOrderKeysAreTheApplicationsOwnScheme(t *testing.T) {
	previous := ""
	for position := range 200_000 {
		key := orderKey(position)
		if len(key) < 2 {
			t.Fatalf("position %d produced %q", position, key)
		}
		head, digits := key[0], key[1:]
		if head < 'a' || head > 'z' {
			t.Fatalf("position %d produced the head %q", position, string(head))
		}
		if int(head-'a')+1 != len(digits) {
			t.Fatalf("%q declares %d digits and carries %d", key, int(head-'a')+1, len(digits))
		}
		for i := range len(digits) {
			if !strings.ContainsRune(orderKeyDigits, rune(digits[i])) {
				t.Fatalf("%q carries the digit %q, which is not in the alphabet", key, string(digits[i]))
			}
		}
		if key <= previous {
			t.Fatalf("position %d produced %q, which does not sort after %q", position, key, previous)
		}
		previous = key
	}
}
