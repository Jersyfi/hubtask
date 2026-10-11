// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// workspaceDouble answers whether the workspace is there.
type workspaceDouble struct{ missing bool }

func (w workspaceDouble) Find(context.Context) (adminrepo.TenantRecord, error) {
	if w.missing {
		return adminrepo.TenantRecord{}, shared.ErrNotFound
	}
	return adminrepo.TenantRecord{}, nil
}

var (
	recoveredTo  = time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	replacedHold = shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	releasedHold = shared.MustParseID("0192f000-0000-7000-8000-0000000000f3")
	goneEntry    = shared.MustParseID("0192f000-0000-7000-8000-0000000000a9")
)

func operator() appshared.ActorContext {
	op := actor()
	op.Scopes = []string{operatorScope}
	return op
}

func replaceCommand() ReplaceLegalHoldsCommand {
	return ReplaceLegalHoldsCommand{
		TenantID: tenantID, RecoveryPoint: recoveredTo,
		Holds: []HoldRecord{
			{
				ID: replacedHold, Scope: domain.HoldContainer, ScopeID: hubID,
				Reason: "Dispute with the supplier", PlacedBy: accountID,
				PlacedAt: recoveredTo.Add(24 * time.Hour),
			},
			{
				ID: releasedHold, Scope: domain.HoldItem, ScopeID: goneEntry,
				Reason: "Inspection", PlacedBy: accountID,
				PlacedAt: recoveredTo.Add(25 * time.Hour), ReleasedAt: recoveredTo.Add(48 * time.Hour),
			},
		},
	}
}

func (h *holdsHarness) replacer() ReplaceLegalHolds {
	service := h.service()
	service.Text = text.Composing{}
	return ReplaceLegalHolds{Holds: service, Workspaces: workspaceDouble{}}
}

// Each hold goes back as it was and in force, a released one and one whose target is gone
// included, each with its own audit entry; a second run changes nothing (backup-restore.md §8.5).
func TestTheHoldsOfARewoundPeriodArePlacedAgain(t *testing.T) {
	h := newHoldsHarness()
	h.holds.missing = map[shared.ID]bool{goneEntry: true}

	replaced, err := h.replacer().Execute(context.Background(), operator(), replaceCommand())
	if err != nil {
		t.Fatalf("placing again: %v", err)
	}
	want := []Replacement{
		{ID: replacedHold, Outcome: HoldPlacedAgain, TargetPresent: true},
		{ID: releasedHold, Outcome: HoldPlacedAgain, TargetPresent: false, ReleasedInPeriod: true},
	}
	if got := replaced.Holds; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || replaced.Archive != nil {
		t.Fatalf("answered %+v, want %+v", replaced, want)
	}
	if h.holds.locked == 0 {
		t.Error("the holds were written without the exclusive hold lock")
	}
	if len(h.holds.stored) != 2 {
		t.Fatalf("%d holds were written", len(h.holds.stored))
	}
	for _, hold := range h.holds.stored {
		if hold.Released() || hold.PlacedBy != accountID || hold.PlacedAt.Before(recoveredTo) {
			t.Errorf("placed again as %+v", hold)
		}
	}
	if len(h.audit.entries) != 2 || h.audit.entries[0].Action != HoldReplacedAction ||
		h.audit.entries[0].TenantID != tenantID {
		t.Errorf("audit entries %+v", h.audit.entries)
	}

	again, err := h.replacer().Execute(context.Background(), operator(), replaceCommand())
	if err != nil {
		t.Fatalf("the second run: %v", err)
	}
	if len(h.holds.stored) != 2 || len(h.audit.entries) != 2 || again.Holds[0].Outcome != HoldAlreadyPresent {
		t.Errorf("the second run changed something: %+v, %d holds", again, len(h.holds.stored))
	}
}

// The operator's alone; and a bad call places nothing at all.
func TestReplacingHoldsIsRefusedWholeOrNotAtAll(t *testing.T) {
	cases := []struct {
		name   string
		actor  appshared.ActorContext
		change func(*ReplaceLegalHoldsCommand)
		code   string
	}{
		{"without the operator's scope", actor(), func(*ReplaceLegalHoldsCommand) {}, "access.insufficient_scope"},
		{"no workspace", operator(), func(c *ReplaceLegalHoldsCommand) { c.TenantID = "" }, "admin.tenant_not_found"},
		{"no recovery point", operator(), func(c *ReplaceLegalHoldsCommand) { c.RecoveryPoint = time.Time{} },
			codeRecoveryPointRequired},
		{"no holds", operator(), func(c *ReplaceLegalHoldsCommand) { c.Holds = nil }, codeHoldsRequired},
		{"too many holds", operator(), func(c *ReplaceLegalHoldsCommand) {
			c.Holds = make([]HoldRecord, maxReplacedHolds+1)
		}, codeTooManyHolds},
		{"one record without a reason", operator(), func(c *ReplaceLegalHoldsCommand) { c.Holds[1].Reason = "" },
			domain.CodeHoldReasonRequired},
		{"one record without its moment", operator(), func(c *ReplaceLegalHoldsCommand) {
			c.Holds[1].PlacedAt = time.Time{}
		}, domain.CodeHoldIncomplete},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHoldsHarness()
			cmd := replaceCommand()
			c.change(&cmd)
			_, err := h.replacer().Execute(context.Background(), c.actor, cmd)
			if got := shared.AsError(err).DetailCode; got != c.code {
				t.Errorf("refused as %q, want %q", got, c.code)
			}
			if len(h.holds.stored) != 0 || len(h.audit.entries) != 0 {
				t.Error("a refused call wrote something")
			}
		})
	}

	t.Run("a workspace the installation does not have", func(t *testing.T) {
		h := newHoldsHarness()
		replacer := h.replacer()
		replacer.Workspaces = workspaceDouble{missing: true}
		_, err := replacer.Execute(context.Background(), operator(), replaceCommand())
		if got := shared.AsError(err).DetailCode; got != "admin.tenant_not_found" {
			t.Errorf("refused as %q", got)
		}
	})
}

// Through the registry, as REST, MCP and automation reach it: the records as the channels
// deliver them, and the answer per hold.
func TestReplacingHoldsThroughTheRegistry(t *testing.T) {
	h := newHoldsHarness()
	registry, err := usecase.NewRegistry(nil, h.replacer().Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	out, err := registry.Invoke(context.Background(), ReplaceLegalHoldsName, operator(), usecase.Input{
		"tenant_id":      tenantID.String(),
		"recovery_point": recoveredTo.Format(time.RFC3339),
		"holds": []any{map[string]any{
			"id": replacedHold.String(), "scope": map[string]any{"kind": "CONTAINER", "id": hubID.String()},
			"reason": "Dispute with the supplier", "placed_by": accountID.String(),
			"placed_at": recoveredTo.Add(time.Hour).Format(time.RFC3339),
		}},
	})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	rows, _ := out["holds"].([]any)
	if len(rows) != 1 {
		t.Fatalf("answered %+v", out)
	}
	row, _ := rows[0].(usecase.Output)
	if row["id"] != replacedHold.String() || row["outcome"] != "PLACED" || row["target_present"] != true {
		t.Errorf("answered %+v", row)
	}
	if len(h.holds.stored) != 1 || h.holds.stored[0].ScopeID != hubID {
		t.Errorf("stored %+v", h.holds.stored)
	}

	_, err = registry.Invoke(context.Background(), ReplaceLegalHoldsName, operator(), usecase.Input{
		"tenant_id": tenantID.String(), "recovery_point": recoveredTo.Format(time.RFC3339),
		"holds": []any{"not an object"},
	})
	if got := shared.AsError(err).DetailCode; got != domain.CodeHoldIncomplete {
		t.Errorf("a malformed record answered %q", got)
	}
}

// archiveDouble answers what an archive gave back, and remembers what it was asked.
type archiveDouble struct {
	answer ArchivedHolds
	err    error
	asked  []ArchiveSource
	scopes []shared.ID
}

func (a *archiveDouble) Read(_ context.Context, scope persistence.Scope, source ArchiveSource) (ArchivedHolds, error) {
	a.asked = append(a.asked, source)
	a.scopes = append(a.scopes, scope.TenantID)
	return a.answer, a.err
}

var backupTarget = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")

// The fallback source: the holds the workspace's archive carries are placed again like the ones a
// request carries, and the answer says from when the archive is - where the gap begins.
func TestTheHoldsOfAnArchiveArePlacedAgainWithTheGapReported(t *testing.T) {
	h := newHoldsHarness()
	taken := recoveredTo.Add(30 * time.Hour)
	archived := &archiveDouble{answer: ArchivedHolds{
		Holds: replaceCommand().Holds, Path: "hubtask-backup-x", SnapshotAt: taken,
		AuditTrail: true, UnreadableEntries: 1,
	}}
	replacer := h.replacer()
	replacer.Archive = archived
	registry, err := usecase.NewRegistry(nil, replacer.Descriptor())
	if err != nil {
		t.Fatal(err)
	}

	out, err := registry.Invoke(context.Background(), ReplaceLegalHoldsName, operator(), usecase.Input{
		"tenant_id": tenantID.String(), "recovery_point": recoveredTo.Format(time.RFC3339),
		"archive": map[string]any{"target_id": backupTarget.String(), "path": "hubtask-backup-x"},
	})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if len(archived.asked) != 1 || archived.asked[0].TargetID != backupTarget || archived.scopes[0] != tenantID {
		t.Errorf("the archive was asked %+v under %v", archived.asked, archived.scopes)
	}
	if rows, _ := out["holds"].([]any); len(rows) != 2 || len(h.holds.stored) != 2 {
		t.Errorf("answered %+v, stored %d", out["holds"], len(h.holds.stored))
	}
	from, _ := out["archive"].(usecase.Output)
	if from["snapshot_at"] != taken.Format(time.RFC3339Nano) || from["audit_trail"] != true ||
		from["unreadable_entries"] != 1 {
		t.Errorf("the archive answered as %+v", from)
	}
}

// An archive with no hold in it is an answer, not a refusal; the gap is still the operator's.
func TestAnArchiveWithoutHoldsPlacesNothingAndSaysFromWhen(t *testing.T) {
	h := newHoldsHarness()
	replacer := h.replacer()
	replacer.Archive = &archiveDouble{answer: ArchivedHolds{Path: "p", SnapshotAt: recoveredTo}}
	cmd := ReplaceLegalHoldsCommand{
		TenantID: tenantID, RecoveryPoint: recoveredTo,
		Archive: &ArchiveSource{TargetID: backupTarget, Path: "p"},
	}
	result, err := replacer.Execute(context.Background(), operator(), cmd)
	if err != nil {
		t.Fatalf("placing again: %v", err)
	}
	if len(result.Holds) != 0 || result.Archive == nil || !result.Archive.SnapshotAt.Equal(recoveredTo) {
		t.Errorf("answered %+v", result)
	}
}

// One source per call, named whole, and asked only by the operator.
func TestTheArchiveSourceIsRefusedWholeOrNotAtAll(t *testing.T) {
	cases := []struct {
		name   string
		actor  appshared.ActorContext
		change func(*ReplaceLegalHoldsCommand)
		code   string
	}{
		{"without the operator's scope", actor(), func(*ReplaceLegalHoldsCommand) {}, "access.insufficient_scope"},
		{"holds and an archive", operator(), func(c *ReplaceLegalHoldsCommand) {
			c.Holds = replaceCommand().Holds
		}, codeHoldsSourceAmbiguous},
		{"no target", operator(), func(c *ReplaceLegalHoldsCommand) { c.Archive.TargetID = "" },
			codeHoldArchiveIncomplete},
		{"no path", operator(), func(c *ReplaceLegalHoldsCommand) { c.Archive.Path = "" },
			codeHoldArchiveIncomplete},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHoldsHarness()
			archived := &archiveDouble{}
			replacer := h.replacer()
			replacer.Archive = archived
			cmd := ReplaceLegalHoldsCommand{
				TenantID: tenantID, RecoveryPoint: recoveredTo,
				Archive: &ArchiveSource{TargetID: backupTarget, Path: "p"},
			}
			c.change(&cmd)
			_, err := replacer.Execute(context.Background(), c.actor, cmd)
			if got := shared.AsError(err).DetailCode; got != c.code {
				t.Errorf("refused as %q, want %q", got, c.code)
			}
			if len(archived.asked) != 0 || len(h.holds.stored) != 0 {
				t.Error("a refused call read the archive or wrote something")
			}
		})
	}

	t.Run("an archive the backup service refuses", func(t *testing.T) {
		h := newHoldsHarness()
		replacer := h.replacer()
		replacer.Archive = &archiveDouble{err: shared.ErrValidation.WithDetail("backup.restore_archive_scope_mismatch")}
		_, err := replacer.Execute(context.Background(), operator(), ReplaceLegalHoldsCommand{
			TenantID: tenantID, RecoveryPoint: recoveredTo,
			Archive: &ArchiveSource{TargetID: backupTarget, Path: "another workspace's"},
		})
		if got := shared.AsError(err).DetailCode; got != "backup.restore_archive_scope_mismatch" {
			t.Errorf("refused as %q", got)
		}
		if len(h.holds.stored) != 0 {
			t.Error("something was written")
		}
	})
}
