// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-DOC-15: a hub's document holds its space's drafts as one block while the hub keeps them, and
// writes them back through SetSpaceDrafts; removing the block makes the space live again.
func TestAHubsDocumentCarriesItsSpacesDrafts(t *testing.T) {
	s := hubPagesSection(shared.MustParseID(supportHub))
	if rows := s.rowsOf(map[string]any{"drafts": true, "source": "HUB"}); len(rows) != 1 {
		t.Fatalf("rows %v", rows)
	}
	if rows := s.rowsOf(map[string]any{"drafts": false, "source": "HUB"}); len(rows) != 0 {
		t.Fatalf("a live space has a block: %v", rows)
	}
	if rows := s.rowsOf(map[string]any{"drafts": false, "source": "DEFAULT"}); len(rows) != 0 {
		t.Fatalf("the default is a block: %v", rows)
	}
	ops, err := s.ops(domain.BlockChange{Section: sectionPages, Key: draftsKey, Op: domain.OpAdd, After: domain.Block{settingValueField: true}}, nil)
	if err != nil || len(ops) != 1 || ops[0].UseCase != "SetSpaceDrafts" || ops[0].Input["drafts"] != true || ops[0].Input["hub_id"] != supportHub {
		t.Fatalf("ops %+v, %v", ops, err)
	}
	ops, err = s.ops(domain.BlockChange{Section: sectionPages, Key: draftsKey, Op: domain.OpRemove}, nil)
	if err != nil || ops[0].Input["drafts"] != false {
		t.Fatalf("remove %+v, %v", ops, err)
	}
	if _, err := s.ops(domain.BlockChange{Section: sectionPages, Key: "other", Op: domain.OpAdd}, nil); err == nil {
		t.Fatal("an unknown key was applied")
	}
}
