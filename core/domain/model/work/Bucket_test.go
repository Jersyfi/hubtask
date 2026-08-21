// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	collectionID = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	bucketID     = shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
	deletedAt    = time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)

	baseBucket = work.NewBucketInput{
		ID: bucketID, TenantID: tenant, CollectionID: collectionID, Name: "Doing", OrderKey: "m",
	}
)

func newBucket(t *testing.T, in work.NewBucketInput) work.Bucket {
	t.Helper()
	bucket, err := work.NewBucket(in)
	if err != nil {
		t.Fatalf("the bucket was refused: %v", err)
	}
	return bucket
}

func TestNewBucketKeepsWhatItWasGiven(t *testing.T) {
	limit := 5
	in := baseBucket
	in.WipLimit = &limit
	in.IsDoneBucket = true
	in.ColorToken = "  surface.green  "

	bucket := newBucket(t, in)

	switch {
	case bucket.Name != "Doing":
		t.Errorf("name %q", bucket.Name)
	case bucket.WipLimit == nil || *bucket.WipLimit != 5:
		t.Errorf("wip limit %v", bucket.WipLimit)
	case !bucket.IsDoneBucket:
		t.Error("the done marker was dropped")
	case bucket.ColorToken != "surface.green":
		t.Errorf("colour token %q, want it trimmed", bucket.ColorToken)
	case bucket.Version != 1:
		t.Errorf("version %d, want 1", bucket.Version)
	case bucket.IsDeleted():
		t.Error("a new bucket is deleted")
	}
}

// The name rule is the container's, in the length the column allows.
func TestNewBucketChecksTheName(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantName   string
		detailCode string
	}{
		{name: "trimmed", input: "  Doing  ", wantName: "Doing"},
		{name: "at the limit", input: strings.Repeat("a", 120), wantName: strings.Repeat("a", 120)},
		{name: "combining marks count as one code point each", input: strings.Repeat("é", 120), wantName: strings.Repeat("é", 120)},
		{name: "empty", input: "", detailCode: "buckets.name_empty"},
		{name: "whitespace only", input: " \t ", detailCode: "buckets.name_empty"},
		{name: "one code point too long", input: strings.Repeat("a", 121), detailCode: "buckets.name_too_long"},
		{name: "a newline", input: "Do\ning", detailCode: "buckets.name_malformed"},
		{name: "a C1 control", input: "Doing\u0085B", detailCode: "buckets.name_malformed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := baseBucket
			in.Name = c.input

			bucket, err := work.NewBucket(in)
			if c.detailCode == "" {
				if err != nil {
					t.Fatalf("%q was refused: %v", c.input, err)
				}
				if bucket.Name != c.wantName {
					t.Errorf("name %q, want %q", bucket.Name, c.wantName)
				}
				return
			}

			assertDetail(t, err, shared.ErrValidation, c.detailCode)
			if fields := shared.AsError(err).Fields; len(fields) != 1 || fields[0].Path != "/name" {
				t.Errorf("the finding does not point at the field: %+v", fields)
			}
		})
	}
}

// Zero is how a caller clears the limit, and the column refuses it as a value - so nothing is lost
// by reading it that way. A negative one is a caller's mistake and is named as one.
func TestNewBucketChecksTheWipLimit(t *testing.T) {
	for _, c := range []struct {
		name       string
		input      *int
		want       *int
		detailCode string
	}{
		{name: "absent is no limit", input: nil},
		{name: "zero is no limit", input: pointerTo(0)},
		{name: "one is a limit", input: pointerTo(1), want: pointerTo(1)},
		{name: "negative", input: pointerTo(-1), detailCode: "buckets.wip_limit_invalid"},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := baseBucket
			in.WipLimit = c.input

			bucket, err := work.NewBucket(in)
			if c.detailCode != "" {
				assertDetail(t, err, shared.ErrValidation, c.detailCode)
				return
			}
			if err != nil {
				t.Fatalf("the bucket was refused: %v", err)
			}
			switch {
			case c.want == nil && bucket.WipLimit != nil:
				t.Errorf("wip limit %v, want none", *bucket.WipLimit)
			case c.want != nil && (bucket.WipLimit == nil || *bucket.WipLimit != *c.want):
				t.Errorf("wip limit %v, want %d", bucket.WipLimit, *c.want)
			}
		})
	}
}

func TestNewBucketRefusesAControlCharacterInTheColour(t *testing.T) {
	in := baseBucket
	in.ColorToken = "surface\ngreen"

	_, err := work.NewBucket(in)
	assertDetail(t, err, shared.ErrValidation, "buckets.color_token_malformed")
}

// The identifiers and the rank come from ports. Missing means the use case was wired wrong, which
// is a defect rather than something a caller can fix.
func TestNewBucketReportsIncompleteIdentityAsADefect(t *testing.T) {
	for _, c := range []struct {
		name  string
		spoil func(*work.NewBucketInput)
	}{
		{name: "no identifier", spoil: func(in *work.NewBucketInput) { in.ID = "" }},
		{name: "no tenant", spoil: func(in *work.NewBucketInput) { in.TenantID = "" }},
		{name: "no collection", spoil: func(in *work.NewBucketInput) { in.CollectionID = "" }},
		{name: "no rank", spoil: func(in *work.NewBucketInput) { in.OrderKey = "" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := baseBucket
			c.spoil(&in)

			_, err := work.NewBucket(in)
			assertDetail(t, err, shared.ErrInternal, "buckets.identity_incomplete")
		})
	}
}

func TestBucketUpdatedReportsOnlyWhatMoved(t *testing.T) {
	limit := 3
	in := baseBucket
	in.WipLimit = &limit
	in.ColorToken = "surface.blue"
	bucket := newBucket(t, in)

	t.Run("a change that changes nothing writes nothing", func(t *testing.T) {
		unchanged, changes, err := bucket.Updated(work.BucketAttributes{
			Name: pointerTo("Doing"), WipLimit: pointerTo(3), ColorToken: pointerTo("surface.blue"),
		})
		if err != nil {
			t.Fatalf("an update to the stored values was refused: %v", err)
		}
		if len(changes) != 0 {
			t.Errorf("changes reported for an update that moved nothing: %+v", changes)
		}
		if unchanged.Version != bucket.Version {
			t.Errorf("the version was spent: %d", unchanged.Version)
		}
	})

	t.Run("the fields that moved, and no others", func(t *testing.T) {
		updated, changes, err := bucket.Updated(work.BucketAttributes{
			Name: pointerTo("In progress"), IsDoneBucket: pointerTo(true),
		})
		if err != nil {
			t.Fatalf("the update was refused: %v", err)
		}
		if len(changes) != 2 {
			t.Fatalf("changes %+v, want the name and the done marker", changes)
		}
		if changes[0].Field != work.FieldName || changes[0].From != "Doing" || changes[0].To != "In progress" {
			t.Errorf("the name change is wrong: %+v", changes[0])
		}
		if changes[1].Field != work.FieldIsDoneBucket || changes[1].To != "true" {
			t.Errorf("the done marker change is wrong: %+v", changes[1])
		}
		if updated.ColorToken != "surface.blue" || *updated.WipLimit != 3 {
			t.Errorf("an untouched field moved: %+v", updated)
		}
	})

	// The empty string is how "not set" reaches a change set - for a number as much as for a
	// text, so that a recipient never has to read 0 as a limit nobody may drop into.
	t.Run("clearing the limit travels as the empty string", func(t *testing.T) {
		updated, changes, err := bucket.Updated(work.BucketAttributes{WipLimit: pointerTo(0)})
		if err != nil {
			t.Fatalf("clearing the limit was refused: %v", err)
		}
		if len(changes) != 1 || changes[0].From != "3" || changes[0].To != "" {
			t.Fatalf("the change is wrong: %+v", changes)
		}
		if updated.WipLimit != nil {
			t.Errorf("the limit survived: %v", *updated.WipLimit)
		}
	})

	t.Run("clearing the colour", func(t *testing.T) {
		updated, changes, err := bucket.Updated(work.BucketAttributes{ColorToken: pointerTo("")})
		if err != nil {
			t.Fatalf("clearing the colour was refused: %v", err)
		}
		if len(changes) != 1 || changes[0].Field != work.FieldColorToken || changes[0].To != "" {
			t.Fatalf("the change is wrong: %+v", changes)
		}
		if updated.ColorToken != "" {
			t.Errorf("the colour survived: %q", updated.ColorToken)
		}
	})
}

func TestBucketUpdatedChecksWhatItIsGiven(t *testing.T) {
	bucket := newBucket(t, baseBucket)

	for _, c := range []struct {
		name       string
		attributes work.BucketAttributes
		detailCode string
	}{
		{
			name:       "an empty name",
			attributes: work.BucketAttributes{Name: pointerTo(" ")},
			detailCode: "buckets.name_empty",
		},
		{
			name:       "a negative limit",
			attributes: work.BucketAttributes{WipLimit: pointerTo(-2)},
			detailCode: "buckets.wip_limit_invalid",
		},
		{
			name:       "a control character in the colour",
			attributes: work.BucketAttributes{ColorToken: pointerTo("a\u0085b")},
			detailCode: "buckets.color_token_malformed",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := bucket.Updated(c.attributes)
			assertDetail(t, err, shared.ErrValidation, c.detailCode)
		})
	}
}

func TestBucketAttributesIsEmpty(t *testing.T) {
	if !(work.BucketAttributes{}).IsEmpty() {
		t.Error("an attribute set with nothing in it is not empty")
	}
	if (work.BucketAttributes{IsDoneBucket: pointerTo(false)}).IsEmpty() {
		t.Error("asking for false is asking for something")
	}
}

func TestBucketReordered(t *testing.T) {
	bucket := newBucket(t, baseBucket)

	t.Run("the rank it already holds writes nothing", func(t *testing.T) {
		_, changes, err := bucket.Reordered("m")
		if err != nil {
			t.Fatalf("the reorder was refused: %v", err)
		}
		if len(changes) != 0 {
			t.Errorf("changes reported for a rank that did not move: %+v", changes)
		}
	})

	t.Run("a new rank", func(t *testing.T) {
		moved, changes, err := bucket.Reordered("t")
		if err != nil {
			t.Fatalf("the reorder was refused: %v", err)
		}
		if len(changes) != 1 || changes[0].Field != work.FieldOrderKey || changes[0].To != "t" {
			t.Fatalf("the change is wrong: %+v", changes)
		}
		if moved.OrderKey != "t" {
			t.Errorf("order key %q", moved.OrderKey)
		}
	})

	// The rank comes from the ordering service, so an empty one is a defect rather than input.
	t.Run("no rank at all is a defect", func(t *testing.T) {
		_, _, err := bucket.Reordered("")
		assertDetail(t, err, shared.ErrInternal, "buckets.identity_incomplete")
	})
}

func TestBucketDeletedIsIdempotent(t *testing.T) {
	bucket := newBucket(t, baseBucket)

	deleted, changes, err := bucket.Deleted(deletedAt)
	if err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}
	if len(changes) != 1 || changes[0].Field != work.FieldDeletedAt || changes[0].From != "" {
		t.Fatalf("the change is wrong: %+v", changes)
	}
	if !deleted.IsDeleted() {
		t.Fatal("the bucket is not deleted")
	}

	again, changes, err := deleted.Deleted(deletedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("deleting twice was refused: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("the second deletion reported changes: %+v", changes)
	}
	if !again.DeletedAt.Equal(deletedAt) {
		t.Errorf("the second deletion moved the stamp to %v", again.DeletedAt)
	}
}

// A deleted bucket is no longer on the board, and every verb that changes one says so with the
// same answer - a conflict, because the request is well formed and the state is what refuses it.
func TestADeletedBucketRefusesEveryChange(t *testing.T) {
	bucket := newBucket(t, baseBucket)
	deleted, _, err := bucket.Deleted(deletedAt)
	if err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}

	t.Run("update", func(t *testing.T) {
		_, _, err := deleted.Updated(work.BucketAttributes{Name: pointerTo("Later")})
		assertDetail(t, err, shared.ErrConflict, "buckets.deleted")
	})

	t.Run("reorder", func(t *testing.T) {
		_, _, err := deleted.Reordered("z")
		assertDetail(t, err, shared.ErrConflict, "buckets.deleted")
	})
}

func pointerTo[T any](value T) *T { return &value }
