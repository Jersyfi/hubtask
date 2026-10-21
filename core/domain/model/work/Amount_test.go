// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

func amountProfile() CapabilityProfile {
	profile := taskProfile()
	profile.Capabilities = append(profile.Capabilities, CapabilityAmount)
	return profile
}

// UC-WRK-25 check 2: an amount is stored beside the title, trimmed, in normal form C, bounded in
// code points; spaces alone are no amount.
func TestAnAmountIsCheckedAtItsBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		amount string
		want   string
		stored string
	}{
		{"a number", "2", "", "2"},
		{"a number and a unit", " 1 kg ", "", "1 kg"},
		{"decomposed, stored composed", "1 Tüte", "", "1 Tüte"},
		{"the longest permitted", strings.Repeat("é", MaxItemAmountLength), "", strings.Repeat("é", MaxItemAmountLength)},
		{"none", "", "", ""},
		{"spaces are none", "  \t ", "", ""},
		{"one code point too long", strings.Repeat("é", MaxItemAmountLength+1), "items.amount_invalid", ""},
		{"a line break", "1\nkg", "items.amount_invalid", ""},
		{"a control character", "1\x07", "items.amount_invalid", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := taskInput()
			in.Profile = amountProfile()
			in.Amount = c.amount

			item, err := NewWorkItem(in)
			if c.want != "" {
				if detail := shared.AsError(err); detail == nil || detail.DetailCode != c.want {
					t.Fatalf("error = %v, want %s", err, c.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if item.Amount != c.stored {
				t.Errorf("amount = %q, want %q", item.Amount, c.stored)
			}
		})
	}
}

// An amount on a type whose profile does not carry AMOUNT is refused, not dropped; no amount needs
// nothing.
func TestAnAmountNeedsTheCapability(t *testing.T) {
	in := taskInput()
	in.Amount = "2"
	if _, err := NewWorkItem(in); !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("error = %v, want capability_not_supported", err)
	}
	in.Amount = " "
	if _, err := NewWorkItem(in); err != nil {
		t.Errorf("no amount was treated as one: %v", err)
	}
}

// An update changes, keeps and clears the amount, and reports it as a field that moved.
func TestAnUpdateChangesAndClearsTheAmount(t *testing.T) {
	in := taskInput()
	in.Profile = amountProfile()
	in.Amount = "2"
	item, err := NewWorkItem(in)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	text3 := "3"
	updated, changes, err := item.Updated(ItemAttributes{Amount: &text3}, amountProfile(), text.Composing{}, at)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Amount != "3" || len(changes) != 1 || changes[0] != (FieldChange{Field: FieldAmount, From: "2", To: "3"}) {
		t.Errorf("updated %q, changes %+v", updated.Amount, changes)
	}

	same := "3"
	if _, changes, _ := updated.Updated(ItemAttributes{Amount: &same}, amountProfile(), text.Composing{}, at); len(changes) != 0 {
		t.Errorf("an unchanged amount reported %+v", changes)
	}

	empty := ""
	cleared, changes, err := updated.Updated(ItemAttributes{Amount: &empty}, taskProfile(), text.Composing{}, at)
	if err != nil {
		t.Fatalf("clearing needs no capability: %v", err)
	}
	if cleared.Amount != "" || len(changes) != 1 {
		t.Errorf("cleared %q, changes %+v", cleared.Amount, changes)
	}

	if _, _, err := cleared.Updated(ItemAttributes{Amount: &text3}, taskProfile(), text.Composing{}, at); !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Errorf("an amount without the capability: %v", err)
	}
	if (ItemAttributes{Amount: &empty}).IsEmpty() {
		t.Error("an update clearing the amount is not empty")
	}
}

// AMOUNT is opt-in: the system offers it, Hubtask's default and a type a layer leaves out do not
// carry it, and only *List* names it.
func TestTheAmountIsOptIn(t *testing.T) {
	matrix := seededMatrix()
	matrix[0].Capabilities = append(matrix[0].Capabilities, CapabilityAmount)
	bound := NewStructureBound(matrix, matrix)

	if slices.Contains(bound.Default().Fields[ItemTask].Capabilities, CapabilityAmount) {
		t.Error("Hubtask's default carries AMOUNT")
	}
	clipped := bound.Clip(StructureValues{Fields: map[ItemType]LevelFields{
		ItemWorkPackage: {Capabilities: []Capability{CapabilityCompletion}},
	}})
	if slices.Contains(clipped.Fields[ItemTask].Capabilities, CapabilityAmount) {
		t.Error("a type a layer leaves out carries AMOUNT")
	}
	named := bound.Clip(StructureValues{Fields: map[ItemType]LevelFields{
		ItemTask: {Capabilities: []Capability{CapabilityCompletion, CapabilityAmount}},
	}})
	if !slices.Contains(named.Fields[ItemTask].Capabilities, CapabilityAmount) {
		t.Error("a layer naming AMOUNT lost it")
	}

	for _, preset := range BuiltInPresets() {
		offered := preset.Offered(bound)
		carries := slices.Contains(offered.Values.Fields[ItemTask].Capabilities, CapabilityAmount)
		if carries != (preset.Key == PresetList) {
			t.Errorf("%s carries AMOUNT: %v", preset.Key, carries)
		}
	}

	// An installation without AMOUNT clips it out of List rather than refusing the preset.
	list, err := FindPreset(PresetList, seededBound())
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(list.Values.Fields[ItemTask].Capabilities, CapabilityAmount) {
		t.Error("List offers AMOUNT where the installation has none")
	}
	if !CapabilityAmount.OptIn() || CapabilityLabels.OptIn() {
		t.Error("the opt-in set is AMOUNT alone")
	}
}
