// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

func repairHarness() (*writerHarness, RepairBoards) {
	h := newWriterHarness()
	h.buckets.stored = map[shared.ID]domain.Bucket{}
	for i, column := range []domain.Bucket{{ID: todoColumn, Name: "Offen"}, {ID: doneColumn, Name: "Erledigt"}} {
		column.TenantID, column.CollectionID, column.Version = tenantID, collectionID, 1
		column.OrderKey = string(rune('a'+i)) + "0"
		h.buckets.stored[column.ID] = column
	}
	statuses := &mapping{}
	return h, RepairBoards{Boards: Boards{Buckets: h.buckets, Statuses: statuses}, Columns: h.update.Writer}
}

// The repair is an administrator's: STRUCTURE at the workspace, asked in the application layer.
func TestTheBoardRepairIsAnAdministrators(t *testing.T) {
	h, repair := repairHarness()
	h.authorizer.err = shared.ErrForbidden
	if _, err := repair.Execute(t.Context(), actor(), false); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a person without STRUCTURE at the workspace repaired: %v", err)
	}
	if len(h.authorizer.requests) != 1 || h.authorizer.requests[0].Permission != service.PermissionStructure ||
		h.authorizer.requests[0].TokenScope != workspaceManage {
		t.Errorf("asked %+v", h.authorizer.requests)
	}
	if len(h.buckets.written) != 0 {
		t.Error("a refused repair wrote")
	}
}

// A board without a done column gets the rightmost one; a dry run counts the same and writes
// nothing, and the audit carries counts only.
func TestTheBoardRepairGivesABoardOneDoneColumn(t *testing.T) {
	h, repair := repairHarness()
	dry, err := repair.Execute(t.Context(), actor(), true)
	if err != nil || !dry.DryRun || dry.DoneMarked != 1 || dry.Boards != 1 {
		t.Fatalf("the dry run counted %+v: %v", dry, err)
	}
	if !h.uow.rolledBack || len(h.audit.entries) != 0 {
		t.Error("the dry run kept what it wrote")
	}
	h, repair = repairHarness()
	report, err := repair.Execute(t.Context(), actor(), false)
	if err != nil || report.DoneMarked != 1 {
		t.Fatalf("repaired %+v: %v", report, err)
	}
	if !h.buckets.stored[doneColumn].IsDoneBucket || h.buckets.stored[todoColumn].IsDoneBucket {
		t.Error("the rightmost column is not the one done column")
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != BoardsRepairedAction {
		t.Fatalf("audited %+v", h.audit.entries)
	}
}
