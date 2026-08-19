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
	labelID = shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")

	baseLabel = work.NewLabelInput{
		ID: labelID, TenantID: tenant, CollectionID: collectionID,
		Name: "Urgent", ColorToken: "accent.red",
	}
)

func newLabel(t *testing.T, in work.NewLabelInput) work.Label {
	t.Helper()
	label, err := work.NewLabel(in)
	if err != nil {
		t.Fatalf("the label was refused: %v", err)
	}
	return label
}

func TestNewLabelKeepsWhatItWasGiven(t *testing.T) {
	in := baseLabel
	in.Description = "  Needs a decision today  "

	label := newLabel(t, in)

	switch {
	case label.Name != "Urgent":
		t.Errorf("name %q", label.Name)
	case label.ColorToken != "accent.red":
		t.Errorf("colour token %q", label.ColorToken)
	case label.Description != "Needs a decision today":
		t.Errorf("description %q, want it trimmed", label.Description)
	case label.Version != 1:
		t.Errorf("version %d, want 1", label.Version)
	case label.IsDeleted():
		t.Error("a new label is deleted")
	}
}

func TestNewLabelChecksTheName(t *testing.T) {
	for _, c := range []struct {
		name       string
		input      string
		detailCode string
	}{
		{name: "empty", input: " ", detailCode: "labels.name_empty"},
		{name: "one code point too long", input: strings.Repeat("a", 121), detailCode: "labels.name_too_long"},
		{name: "a newline", input: "Ur\ngent", detailCode: "labels.name_malformed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := baseLabel
			in.Name = c.input

			_, err := work.NewLabel(in)
			assertDetail(t, err, shared.ErrValidation, c.detailCode)
		})
	}
}

// A label is rendered as a chip and nothing else. With no colour there is nothing to render it as,
// and a client would have to invent one - which is how two clients come to render one label
// differently. So the colour is required, unlike a bucket's.
func TestNewLabelNeedsAColour(t *testing.T) {
	for _, c := range []struct {
		name       string
		input      string
		detailCode string
	}{
		{name: "absent", input: "", detailCode: "labels.color_token_empty"},
		{name: "whitespace only", input: "  ", detailCode: "labels.color_token_empty"},
		{name: "a control character", input: "accent\u0085red", detailCode: "labels.color_token_malformed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := baseLabel
			in.ColorToken = c.input

			_, err := work.NewLabel(in)
			assertDetail(t, err, shared.ErrValidation, c.detailCode)
		})
	}
}

func TestNewLabelReportsIncompleteIdentityAsADefect(t *testing.T) {
	for _, c := range []struct {
		name  string
		spoil func(*work.NewLabelInput)
	}{
		{name: "no identifier", spoil: func(in *work.NewLabelInput) { in.ID = "" }},
		{name: "no tenant", spoil: func(in *work.NewLabelInput) { in.TenantID = "" }},
		{name: "no collection", spoil: func(in *work.NewLabelInput) { in.CollectionID = "" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := baseLabel
			c.spoil(&in)

			_, err := work.NewLabel(in)
			assertDetail(t, err, shared.ErrInternal, "labels.identity_incomplete")
		})
	}
}

func TestLabelUpdatedReportsOnlyWhatMoved(t *testing.T) {
	in := baseLabel
	in.Description = "Needs a decision today"
	label := newLabel(t, in)

	t.Run("a change that changes nothing writes nothing", func(t *testing.T) {
		_, changes, err := label.Updated(work.LabelAttributes{
			Name: pointerTo("Urgent"), ColorToken: pointerTo("accent.red"),
		})
		if err != nil {
			t.Fatalf("an update to the stored values was refused: %v", err)
		}
		if len(changes) != 0 {
			t.Errorf("changes reported for an update that moved nothing: %+v", changes)
		}
	})

	t.Run("the fields that moved, and no others", func(t *testing.T) {
		updated, changes, err := label.Updated(work.LabelAttributes{
			ColorToken: pointerTo("accent.amber"),
		})
		if err != nil {
			t.Fatalf("the update was refused: %v", err)
		}
		if len(changes) != 1 || changes[0].Field != work.FieldColorToken ||
			changes[0].From != "accent.red" || changes[0].To != "accent.amber" {
			t.Fatalf("the change is wrong: %+v", changes)
		}
		if updated.Name != "Urgent" || updated.Description != "Needs a decision today" {
			t.Errorf("an untouched field moved: %+v", updated)
		}
	})

	t.Run("clearing the description", func(t *testing.T) {
		updated, changes, err := label.Updated(work.LabelAttributes{Description: pointerTo("")})
		if err != nil {
			t.Fatalf("clearing the description was refused: %v", err)
		}
		if len(changes) != 1 || changes[0].Field != work.FieldDescription || changes[0].To != "" {
			t.Fatalf("the change is wrong: %+v", changes)
		}
		if updated.Description != "" {
			t.Errorf("the description survived: %q", updated.Description)
		}
	})
}

func TestLabelUpdatedChecksWhatItIsGiven(t *testing.T) {
	label := newLabel(t, baseLabel)

	for _, c := range []struct {
		name       string
		attributes work.LabelAttributes
		detailCode string
	}{
		{
			name:       "an empty name",
			attributes: work.LabelAttributes{Name: pointerTo(" ")},
			detailCode: "labels.name_empty",
		},
		{
			name:       "clearing the colour",
			attributes: work.LabelAttributes{ColorToken: pointerTo("")},
			detailCode: "labels.color_token_empty",
		},
		{
			name:       "a control character in the description",
			attributes: work.LabelAttributes{Description: pointerTo("one\ntwo")},
			detailCode: "labels.description_malformed",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := label.Updated(c.attributes)
			assertDetail(t, err, shared.ErrValidation, c.detailCode)
		})
	}
}

func TestLabelAttributesIsEmpty(t *testing.T) {
	if !(work.LabelAttributes{}).IsEmpty() {
		t.Error("an attribute set with nothing in it is not empty")
	}
	if (work.LabelAttributes{Description: pointerTo("")}).IsEmpty() {
		t.Error("asking for an empty description is asking for something")
	}
}

func TestLabelDeletedIsIdempotent(t *testing.T) {
	label := newLabel(t, baseLabel)

	deleted, changes, err := label.Deleted(deletedAt)
	if err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}
	if len(changes) != 1 || changes[0].Field != work.FieldDeletedAt {
		t.Fatalf("the change is wrong: %+v", changes)
	}
	if !deleted.IsDeleted() {
		t.Fatal("the label is not deleted")
	}

	_, changes, err = deleted.Deleted(deletedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("deleting twice was refused: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("the second deletion reported changes: %+v", changes)
	}
}

func TestADeletedLabelRefusesEveryChange(t *testing.T) {
	label := newLabel(t, baseLabel)
	deleted, _, err := label.Deleted(deletedAt)
	if err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}

	_, _, err = deleted.Updated(work.LabelAttributes{Name: pointerTo("Later")})
	assertDetail(t, err, shared.ErrConflict, "labels.deleted")
}
