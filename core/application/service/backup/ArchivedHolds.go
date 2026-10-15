// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	holds "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The trail's entries about a hold, as the lifecycle service writes them.
const (
	holdPlaced   = "lifecycle.hold_placed"
	holdReplaced = "lifecycle.hold_replaced"
	holdReleased = "lifecycle.hold_released"
)

// ArchivedHolds reads the legal holds a workspace archive carries: the fallback source of a
// point-in-time recovery where the end of the WAL archive is unusable (backup-restore.md §8.5
// step 5).
//
// Two parts of the archive say which holds there were. The `legal_holds` file is the workspace's
// own copy of them, whole in every archive, released ones included. The trail, where the schedule
// includes it, records every hold placed since the chain's full archive - also one whose row a
// later restore of the workspace took away. Both are read; a row and an entry about the same hold
// are the same hold, and the row wins.
type ArchivedHolds struct {
	Restorer Restorer
	Keys     crypto.KeyMaterialiser
}

var _ lifecycle.HoldArchive = ArchivedHolds{}

// Read answers the holds of the archive at the path, which must be the workspace's own (BK-10).
func (a ArchivedHolds) Read(
	ctx context.Context, scope persistence.Scope, source lifecycle.ArchiveSource,
) (lifecycle.ArchivedHolds, error) {
	// The name first, before the target is asked anything: the scope's workspace is the one the
	// holds are placed in, and another workspace's archive would place its holds there.
	if !strings.HasPrefix(source.Path, archive.Prefix(scope.TenantID)) {
		return lifecycle.ArchivedHolds{}, scopeMismatch()
	}
	store, err := a.Restorer.open(ctx, scope, source.TargetID)
	if err != nil {
		return lifecycle.ArchivedHolds{}, err
	}
	reader := archive.NewReader(store, a.Restorer.Cipher)
	chain, err := reader.Chain(ctx, source.Path)
	if err != nil {
		return lifecycle.ArchivedHolds{}, err
	}
	newest := chain[0]
	if newest.Manifest.Scope.ID != scope.TenantID.String() {
		return lifecycle.ArchivedHolds{}, scopeMismatch()
	}

	keys, err := archiveKeysOf(ctx, a.Keys, chain, source.TargetID)
	if err != nil {
		return lifecycle.ArchivedHolds{}, err
	}

	rows, _ := archive.FindEntityByTable("legal_hold")
	trail, _ := archive.FindEntityByTable("audit_log")
	collected := newHoldCollector()
	if err := reader.Records(ctx, newest, rows, keys.Of(newest), collected.row); err != nil {
		return lifecycle.ArchivedHolds{}, err
	}
	// Oldest first, so that a release is read after the placing it ends.
	for i := len(chain) - 1; i >= 0; i-- {
		if err := reader.Records(ctx, chain[i], trail, keys.Of(chain[i]), collected.entry); err != nil {
			return lifecycle.ArchivedHolds{}, err
		}
	}

	return lifecycle.ArchivedHolds{
		Holds: collected.holds(), Path: newest.Prefix, SnapshotAt: newest.Manifest.SnapshotAt,
		AuditTrail:        slices.ContainsFunc(newest.Manifest.Files, func(f archive.File) bool { return f.Path == archive.DataName(trail.Name) }),
		UnreadableEntries: collected.unreadable,
	}, nil
}

func scopeMismatch() error {
	return shared.ErrValidation.WithDetail(domain.CodeRestoreArchiveScopeMismatch).
		WithFields(shared.FieldError{Path: "/archive/path", Code: domain.CodeRestoreArchiveScopeMismatch}).
		WithCause(errors.New("the archive is not the workspace's own"))
}

// holdCollector gathers the holds an archive names, from its rows and from its trail.
type holdCollector struct {
	rows       map[shared.ID]lifecycle.HoldRecord
	entries    map[shared.ID]lifecycle.HoldRecord
	released   map[shared.ID]time.Time
	unreadable int
}

func newHoldCollector() *holdCollector {
	return &holdCollector{
		rows:     map[shared.ID]lifecycle.HoldRecord{},
		entries:  map[shared.ID]lifecycle.HoldRecord{},
		released: map[shared.ID]time.Time{},
	}
}

// row takes one line of the `legal_holds` file: the row as `to_jsonb` wrote it.
func (c *holdCollector) row(record archive.Record) error {
	if record.Op != archive.OpUpsert {
		return nil
	}
	data := record.Data
	hold := lifecycle.HoldRecord{
		ID: idIn(data, "id"), Scope: holds.HoldScope(textIn(data, "scope_kind")), ScopeID: idIn(data, "scope_id"),
		Reason: textIn(data, "reason"), PlacedBy: idIn(data, "placed_by"), PlacedAt: momentIn(data, "placed_at"),
		ReleasedAt: momentIn(data, "released_at"),
	}
	if !complete(hold) {
		c.unreadable++
		return nil
	}
	c.rows[hold.ID] = hold
	return nil
}

// entry takes one line of the trail, and keeps the ones about a hold. A placing names the hold's
// scope and reason in its changes, its placer and moment are the entry's own; a hold placed again
// after an earlier recovery names them in its changes.
func (c *holdCollector) entry(record archive.Record) error {
	if record.Op != archive.OpUpsert {
		return nil
	}
	data := record.Data
	action := textIn(data, "action")
	if action != holdPlaced && action != holdReplaced && action != holdReleased {
		return nil
	}
	id := idIn(data, "target_id")
	at := momentIn(data, "occurred_at")
	if action == holdReleased {
		if id.IsZero() || at.IsZero() {
			c.unreadable++
			return nil
		}
		c.released[id] = at
		return nil
	}

	changes, _ := data["changes"].(map[string]any)
	hold := lifecycle.HoldRecord{
		ID: id, Scope: holds.HoldScope(changed(changes, "scope_kind")), ScopeID: parsedID(changed(changes, "scope_id")),
		Reason: changed(changes, "reason"), PlacedBy: idIn(data, "actor_id"), PlacedAt: at,
	}
	if action == holdReplaced {
		hold.PlacedBy = parsedID(changed(changes, "placed_by"))
		hold.PlacedAt = parsedMoment(changed(changes, "placed_at"))
	}
	if !complete(hold) {
		c.unreadable++
		return nil
	}
	delete(c.released, hold.ID) // placed again after a release: in force from here
	c.entries[hold.ID] = hold
	return nil
}

// holds answers every hold found, oldest first; a row wins over an entry about the same hold, and
// a release the trail records marks one the row does not already show released.
func (c *holdCollector) holds() []lifecycle.HoldRecord {
	out := make([]lifecycle.HoldRecord, 0, len(c.rows)+len(c.entries))
	for _, hold := range c.rows {
		out = append(out, hold)
	}
	for id, hold := range c.entries {
		if _, known := c.rows[id]; known {
			continue
		}
		out = append(out, hold)
	}
	for i := range out {
		if at, released := c.released[out[i].ID]; released && out[i].ReleasedAt.IsZero() {
			out[i].ReleasedAt = at
		}
	}
	slices.SortFunc(out, func(a, b lifecycle.HoldRecord) int {
		if order := a.PlacedAt.Compare(b.PlacedAt); order != 0 {
			return order
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	return out
}

// complete is whether a record names what placing a hold again needs; the domain's own checks
// still follow.
func complete(hold lifecycle.HoldRecord) bool {
	switch {
	case hold.ID.IsZero(), hold.PlacedBy.IsZero(), hold.PlacedAt.IsZero(), hold.Reason == "":
		return false
	case !hold.Scope.Valid():
		return false
	case hold.Scope != holds.HoldTenant && hold.ScopeID.IsZero():
		return false
	}
	return true
}

func textIn(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return value
}

func idIn(data map[string]any, key string) shared.ID { return parsedID(textIn(data, key)) }

func momentIn(data map[string]any, key string) time.Time { return parsedMoment(textIn(data, key)) }

// parsedID is the identifier, or nothing when the value is not one.
func parsedID(raw string) shared.ID {
	id, err := shared.ParseID(raw)
	if err != nil {
		return ""
	}
	return id
}

// parsedMoment is the instant, or the zero time when the value is not one.
func parsedMoment(raw string) time.Time {
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return at.UTC()
}

// changed is the value a change of an audit entry was changed to.
func changed(changes map[string]any, field string) string {
	change, _ := changes[field].(map[string]any)
	value, _ := change["to"].(string)
	return value
}
