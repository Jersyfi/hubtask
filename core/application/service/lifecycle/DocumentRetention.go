// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	privacy "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// TrashedDocument is a trashed page or board whose time in the trash is up.
type TrashedDocument = repository.TrashedDocument

// TrashedDocuments is the knowledge and whiteboard modules' trash, read and removed by the pass
// (TRASH, accepted.md B).
//
// Expired answers only pages with no page left under them, so removing one never cascades to a
// page the pass did not judge; a tree goes leaf by leaf over the rounds of one pass. Remove repeats
// the read's conditions, so a page restored or given a child meanwhile stays.
type TrashedDocuments interface {
	Expired(ctx context.Context, cutoff time.Time, passed []shared.ID, batch int) ([]TrashedDocument, error)
	Remove(ctx context.Context, documents []TrashedDocument, cutoff time.Time) (int, error)
}

// PageVersions thins a page's versions (PAGE_VERSION): the held hubs and accounts are kept back by
// the statement itself, so a hold narrower than the workspace never reaches a version it covers.
type PageVersions interface {
	DeleteThinned(ctx context.Context, cutoff time.Time, heldHubs, heldAccounts []shared.ID, batch int) (int, error)
	CountThinned(ctx context.Context, cutoff time.Time, heldHubs, heldAccounts []shared.ID, ceiling int) (int, error)
}

// DocumentSweep is what the pass needs beyond the trash of entries for pages and boards. Optional
// as a whole and per field: wired without it, the pass sweeps exactly what it did before.
type DocumentSweep struct {
	Trash    TrashedDocuments
	Versions PageVersions
	// Changes and HLC tell offline devices that a purged page or board is gone; a nil Changes
	// records nothing, and the next walk leaves it out.
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
}

// documentRounds bounds the rounds of one pass: a page tree is at most 16 deep (PageSubtree), so a
// due tree goes whole in one pass, and a pass that removes nothing in a round stops.
const documentRounds = 17

// sweepTrashedDocuments removes the trashed pages and boards past the trash period, judging each
// as the entries' trash is judged: a hold on the workspace, the hub or a contributor keeps it, and
// so does a restricted contributor. The journal and the tombstone are written before the rows go.
func (h RunRetention) sweepTrashedDocuments(
	ctx context.Context, tenantID shared.ID, cutoff, now time.Time,
) (Outcome, error) {
	if h.Documents.Trash == nil {
		return Outcome{}, nil
	}
	holds, err := h.Purger.Holds.Active(ctx)
	if err != nil {
		return Outcome{}, err
	}
	restricted, err := restrictionsOf(ctx, h.Purger.Restrictions)
	if err != nil {
		return Outcome{}, err
	}

	var outcome Outcome
	var passed []shared.ID
	batch := h.Purger.BatchSize
	for round := 0; round < documentRounds; round++ {
		room := batch
		if batch > 0 {
			room = batch - outcome.Removed
			if room <= 0 {
				break
			}
		}
		expired, err := h.Documents.Trash.Expired(ctx, cutoff, passed, room)
		if err != nil {
			return outcome, err
		}
		going := judgeDocuments(expired, holds, restricted, &outcome, &passed)
		if len(going) == 0 {
			break
		}
		removals := make([]domain.Removal, 0, len(going))
		for _, document := range going {
			removals = append(removals, domain.Removal{
				Entity: document.Entity, EntityID: document.ID, Reason: domain.DeletedByRetention,
			})
		}
		if err := h.Purger.record(ctx, removals, now); err != nil {
			return outcome, err
		}
		removed, err := h.Documents.Trash.Remove(ctx, going, cutoff)
		if err != nil {
			return outcome, err
		}
		outcome.Removed += removed
		if err := h.announceGone(ctx, tenantID, going); err != nil {
			return outcome, err
		}
		if removed < len(going) {
			// Some changed meanwhile; they come back judged afresh on a later pass.
			break
		}
	}
	return outcome, nil
}

// judgeDocuments answers which documents of a round may go; the others join passed, so the next
// round reads past them.
func judgeDocuments(
	expired []TrashedDocument, holds domain.Holds, restricted privacy.Restricted,
	outcome *Outcome, passed *[]shared.ID,
) []TrashedDocument {
	going := make([]TrashedDocument, 0, len(expired))
	for _, document := range expired {
		outcome.Matched++
		if _, held := holds.Blocking(domain.Target{
			ContainerIDs: nonZero(document.HubID), Contributors: document.Contributors,
		}); held {
			outcome.blocked(BlockedByLegalHold)
			outcome.Held++
			*passed = append(*passed, document.ID)
			continue
		}
		if restrictedAmong(restricted, document.Contributors) {
			outcome.blocked(BlockedByRestriction)
			outcome.Held++
			*passed = append(*passed, document.ID)
			continue
		}
		going = append(going, document)
	}
	return going
}

// announceGone records a delete per removed page or board, filed under its hub, so a device drops
// its copy.
func (h RunRetention) announceGone(ctx context.Context, tenantID shared.ID, gone []TrashedDocument) error {
	if h.Documents.Changes == nil || h.Documents.HLC == nil {
		return nil
	}
	for _, document := range gone {
		if err := h.Documents.Changes.Record(ctx, changelog.Change{
			TenantID: tenantID, Entity: document.Entity, EntityID: document.ID, Op: changelog.Delete,
			ContainerID: document.HubID, HLC: h.Documents.HLC.Next(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// sweepPageVersions thins the page versions past PAGE_VERSION's period. A hold on the workspace
// stops it; a hold on a hub or an account, and a restricted author, keep what they cover back in
// the statement.
func (h RunRetention) sweepPageVersions(ctx context.Context, started time.Time) (Outcome, error) {
	if h.Documents.Versions == nil {
		return Outcome{}, nil
	}
	holds, err := h.Purger.Holds.Active(ctx)
	if err != nil {
		return Outcome{}, err
	}
	restricted, err := restrictionsOf(ctx, h.Purger.Restrictions)
	if err != nil {
		return Outcome{}, err
	}
	hubs, accounts := heldScopes(holds, restricted)
	return h.sweepHeldByTenant(ctx, started, domain.KindPageVersion, scopedRows{
		versions: h.Documents.Versions, hubs: hubs, accounts: accounts,
	})
}

// heldScopes are the hubs and accounts a hold narrower than the workspace names, with the
// restricted accounts among the latter.
func heldScopes(holds domain.Holds, restricted privacy.Restricted) (hubs, accounts []shared.ID) {
	for _, hold := range holds {
		if hold.ScopeID.IsZero() {
			continue
		}
		switch hold.Scope {
		case domain.HoldContainer:
			hubs = append(hubs, hold.ScopeID)
		case domain.HoldAccount:
			accounts = append(accounts, hold.ScopeID)
		}
	}
	return hubs, append(accounts, restricted.Accounts()...)
}

// scopedRows adapts PageVersions to the tenant-held sweep's port.
type scopedRows struct {
	versions PageVersions
	hubs     []shared.ID
	accounts []shared.ID
}

func (s scopedRows) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	return s.versions.DeleteThinned(ctx, cutoff, s.hubs, s.accounts, batch)
}

func (s scopedRows) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	return s.versions.CountThinned(ctx, cutoff, s.hubs, s.accounts, ceiling)
}
