// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-FLW-01 check 3: an entry moved into a collection that does not offer its type is refused by
// name, neither keeping the type there nor changing it unseen; one offering every type takes it.
func TestAMoveIsRefusedWhereTheTypeIsNotOffered(t *testing.T) {
	narrowed := entryOffer{wired: true, offered: domain.NewOffered(domain.StructureBound{},
		domain.EffectiveStructure{Values: domain.StructureValues{
			EntryTypes: map[domain.ItemType][]string{domain.ItemTask: {"feature"}},
		}})}
	bug := domain.WorkItem{Type: domain.ItemTask, EntryType: "bug"}

	err := narrowed.requireEntryType(bug)
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "entry_types.not_offered" ||
		refusal.Params["entry_type"] != "bug" {
		t.Fatalf("a bug entered a collection offering only features: %v", err)
	}
	if err := narrowed.requireEntryType(domain.WorkItem{Type: domain.ItemTask, EntryType: "feature"}); err != nil {
		t.Errorf("an offered type was refused: %v", err)
	}
	if err := narrowed.requireEntryType(domain.WorkItem{Type: domain.ItemWorkPackage}); err != nil {
		t.Errorf("a level that offers every type refused: %v", err)
	}
	if err := (entryOffer{}).requireEntryType(bug); err != nil {
		t.Errorf("an unwired gate refused: %v", err)
	}
}
