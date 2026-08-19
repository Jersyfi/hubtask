// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The subtree arithmetic lives on the item, so it is tested here rather than only through the hierarchy service
// that calls it: these two functions are what invariant I-W2 rests on, and a caller's test would not notice
// them being right for the wrong reason.

// Containment is a prefix test, and it is only correct because every path ends in a separator. Without that,
// `/ab/` would read as sitting inside `/a/` - and the identifiers that share a prefix are exactly the ones a
// cycle check must not confuse.
func TestContainmentIsAPrefixOfWholeSegments(t *testing.T) {
	outer := work.WorkItem{ID: "a", Path: "/a/"}

	cases := map[string]struct {
		other work.WorkItem
		want  bool
	}{
		"itself":                     {outer, true},
		"a direct child":             {work.WorkItem{ID: "b", Path: "/a/b/"}, true},
		"a grandchild":               {work.WorkItem{ID: "c", Path: "/a/b/c/"}, true},
		"a sibling sharing a prefix": {work.WorkItem{ID: "ab", Path: "/ab/"}, false},
		"an unrelated item":          {work.WorkItem{ID: "z", Path: "/z/"}, false},
		"its own parent":             {work.WorkItem{ID: "root", Path: "/"}, false},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := outer.Contains(c.other); got != c.want {
				t.Errorf("Contains(%q) = %v, want %v", c.other.Path, got, c.want)
			}
		})
	}
}

// Where a subtree lands is derived from the destination's path, so that whoever rewrites the descendants and
// whoever built the paths in the first place agree.
func TestASubtreePathIsBuiltFromTheParentPath(t *testing.T) {
	item := work.WorkItem{ID: newID}

	if got, want := item.SubtreePathUnder("/a/"), "/a/"+newID.String()+"/"; got != want {
		t.Errorf("under /a/ the subtree lands at %q, want %q", got, want)
	}
	// The top level of a collection: an empty parent path is the separator alone, so the result is a root path
	// rather than one missing its leading separator.
	if got, want := item.SubtreePathUnder(""), work.RootPath(newID); got != want {
		t.Errorf("at the top level the subtree lands at %q, want %q", got, want)
	}
	if got, want := item.SubtreePathUnder(work.PathSeparator), work.RootPath(newID); got != want {
		t.Errorf("under the separator the subtree lands at %q, want %q", got, want)
	}
}

// Only a collection holds items, and its lifecycle decides whether they may change: an archived container is
// read-only and its children inherit that (I-C3). Exercised here because the move use cases depend on it.
func TestOnlyALiveCollectionAcceptsItems(t *testing.T) {
	at := created

	collection, err := work.NewContainer(baseColl)
	if err != nil {
		t.Fatalf("building the collection: %v", err)
	}
	hub, err := work.NewContainer(baseHub)
	if err != nil {
		t.Fatalf("building the hub: %v", err)
	}

	trashed, archived := collection, collection
	trashed.DeletedAt, archived.ArchivedAt = &at, &at

	cases := map[string]struct {
		container  work.Container
		sentinel   error
		detailCode string
	}{
		"a hub holds no items":   {hub, shared.ErrValidation, "items.collection_required"},
		"a trashed collection":   {trashed, shared.ErrConflict, "items.collection_trashed"},
		"an archived collection": {archived, shared.ErrConflict, "items.collection_archived"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := c.container.EnsureAcceptsItems()
			if !errors.Is(err, c.sentinel) {
				t.Fatalf("answered %v, want %v", err, c.sentinel)
			}
			if got := shared.AsError(err).DetailCode; got != c.detailCode {
				t.Errorf("the detail code is %q, want %q", got, c.detailCode)
			}
		})
	}

	if err := collection.EnsureAcceptsItems(); err != nil {
		t.Errorf("a live collection refused items: %v", err)
	}
}
