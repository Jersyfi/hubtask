// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	holds "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	rowHold      = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	trailHold    = shared.MustParseID("0192f000-0000-7000-8000-0000000000d2")
	againHold    = shared.MustParseID("0192f000-0000-7000-8000-0000000000d3")
	heldEntry    = shared.MustParseID("0192f000-0000-7000-8000-0000000000d4")
	holdsPlacer  = shared.MustParseID("0192f000-0000-7000-8000-0000000000d5")
	placedBefore = now.Add(-72 * time.Hour)
)

func at(moment time.Time) string { return moment.Format(time.RFC3339Nano) }

func to(value string) map[string]any { return map[string]any{"to": value} }

// holdsArchive writes one archive of the workspace with the performer: its own copy of the holds,
// and - when asked - the trail.
func holdsArchive(t *testing.T, withTrail bool, trail []repository.Row) (*performHarness, string) {
	t.Helper()
	h := newPerformHarness(t)
	h.export.byTable["legal_hold"] = []repository.Row{
		{ID: rowHold.String(), Data: map[string]any{
			"id": rowHold.String(), "scope_kind": "ITEM", "scope_id": heldEntry.String(),
			"reason": "Dispute with the supplier", "placed_by": holdsPlacer.String(),
			"placed_at": at(placedBefore), "released_by": holdsPlacer.String(),
			"released_at": at(placedBefore.Add(time.Hour)),
		}},
	}
	h.export.byTable["audit_log"] = trail
	input := performInput()
	input.IncludeAudit = withTrail
	run, err := h.performer().Perform(context.Background(), input)
	if err != nil {
		t.Fatalf("writing the archive: %v", err)
	}
	return h, run.ArchivePath
}

func (p *performHarness) archivedHolds() ArchivedHolds {
	return ArchivedHolds{Restorer: p.restorer(), Keys: p.keys}
}

func workspaceScope() persistence.Scope {
	return persistence.Scope{TenantID: tenantID, ActorID: actorID}
}

func trailEntry(seq, action string, target shared.ID, occurred time.Time, actor string, changes map[string]any) repository.Row {
	return repository.Row{ID: seq, ChangedAt: occurred, Data: map[string]any{
		"seq": seq, "action": action, "target_type": "legal_hold", "target_id": target.String(),
		"occurred_at": at(occurred), "actor_id": actor, "changes": changes,
	}}
}

// The archive's own copy of the holds and its trail together: a hold only the trail still knows
// of comes back with its placer and moment, one placed again after an earlier recovery with the
// values that entry kept, a release the trail records is marked, and an entry that names too
// little is counted rather than refused.
func TestAnArchiveGivesBackItsHoldsAndItsTrailsHolds(t *testing.T) {
	trail := []repository.Row{
		trailEntry("1", "item.created", heldEntry, placedBefore, holdsPlacer.String(), map[string]any{}),
		trailEntry("2", holdPlaced, trailHold, placedBefore.Add(2*time.Hour), holdsPlacer.String(), map[string]any{
			"scope_kind": to("TENANT"), "scope_id": to(""), "reason": to("Inspection"),
		}),
		trailEntry("3", holdReleased, trailHold, placedBefore.Add(3*time.Hour), holdsPlacer.String(), map[string]any{}),
		trailEntry("4", holdReplaced, againHold, placedBefore.Add(4*time.Hour), actorID.String(), map[string]any{
			"scope_kind": to("CONTAINER"), "scope_id": to(heldEntry.String()), "reason": to("Audit 2026"),
			"placed_by": to(holdsPlacer.String()), "placed_at": to(at(placedBefore.Add(-time.Hour))),
		}),
		trailEntry("5", holdPlaced, heldEntry, placedBefore.Add(5*time.Hour), "", map[string]any{
			"scope_kind": to("TENANT"), "reason": to("no placer"),
		}),
		// The row's own hold, named by the trail as well: the row wins.
		trailEntry("6", holdPlaced, rowHold, placedBefore, holdsPlacer.String(), map[string]any{
			"scope_kind": to("ITEM"), "scope_id": to(heldEntry.String()), "reason": to("an older wording"),
		}),
	}
	h, path := holdsArchive(t, true, trail)

	got, err := h.archivedHolds().Read(context.Background(), workspaceScope(),
		lifecycle.ArchiveSource{TargetID: targetID, Path: path})
	if err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	if !got.AuditTrail || got.UnreadableEntries != 1 || !got.SnapshotAt.Equal(now) || got.Path != path {
		t.Errorf("the archive answered trail %v, unreadable %d, taken %s, path %q",
			got.AuditTrail, got.UnreadableEntries, got.SnapshotAt, got.Path)
	}
	want := []lifecycle.HoldRecord{
		{ID: againHold, Scope: holds.HoldContainer, ScopeID: heldEntry, Reason: "Audit 2026",
			PlacedBy: holdsPlacer, PlacedAt: placedBefore.Add(-time.Hour)},
		{ID: rowHold, Scope: holds.HoldItem, ScopeID: heldEntry, Reason: "Dispute with the supplier",
			PlacedBy: holdsPlacer, PlacedAt: placedBefore, ReleasedAt: placedBefore.Add(time.Hour)},
		{ID: trailHold, Scope: holds.HoldTenant, Reason: "Inspection",
			PlacedBy: holdsPlacer, PlacedAt: placedBefore.Add(2 * time.Hour), ReleasedAt: placedBefore.Add(3 * time.Hour)},
	}
	if len(got.Holds) != len(want) {
		t.Fatalf("answered %d holds: %+v", len(got.Holds), got.Holds)
	}
	for i := range want {
		if g, w := got.Holds[i], want[i]; g.ID != w.ID || g.Scope != w.Scope || g.ScopeID != w.ScopeID ||
			g.Reason != w.Reason || g.PlacedBy != w.PlacedBy || !g.PlacedAt.Equal(w.PlacedAt) ||
			!g.ReleasedAt.Equal(w.ReleasedAt) {
			t.Errorf("hold %d is %+v, want %+v", i, g, w)
		}
	}
}

// Without the trail the archive's own copy is all there is, and the answer says so.
func TestAnArchiveWithoutItsTrailGivesBackItsOwnCopy(t *testing.T) {
	h, path := holdsArchive(t, false, nil)
	got, err := h.archivedHolds().Read(context.Background(), workspaceScope(),
		lifecycle.ArchiveSource{TargetID: targetID, Path: path})
	if err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	if got.AuditTrail || len(got.Holds) != 1 || got.Holds[0].ID != rowHold {
		t.Errorf("answered %+v", got)
	}
}

// Another workspace's archive is refused before its target is asked anything (BK-10).
func TestAnotherWorkspacesArchiveIsRefused(t *testing.T) {
	h, path := holdsArchive(t, true, nil)
	other := persistence.Scope{TenantID: shared.MustParseID("0192f000-0000-7000-8000-0000000000ee")}
	_, err := h.archivedHolds().Read(context.Background(), other,
		lifecycle.ArchiveSource{TargetID: targetID, Path: path})
	if got := shared.AsError(err).DetailCode; got != domain.CodeRestoreArchiveScopeMismatch {
		t.Errorf("refused as %q", got)
	}

	_, err = h.archivedHolds().Read(context.Background(), workspaceScope(),
		lifecycle.ArchiveSource{TargetID: targetID, Path: archive.Prefix(tenantID) + "nothing-there"})
	if err == nil {
		t.Error("an archive that is not there answered")
	}
}
