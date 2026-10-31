// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"slices"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// documentRow is one trashed page or board of the in-memory trash; parent names a page above it.
type documentRow struct {
	doc       TrashedDocument
	deletedAt time.Time
	parent    shared.ID
}

// documentStore stands in for db/queries/DocumentRetention.sql: only leaves are expired, and the
// delete repeats that condition.
type documentStore struct {
	rows  []documentRow
	asked time.Time
}

func (s *documentStore) hasChild(id shared.ID) bool {
	return slices.ContainsFunc(s.rows, func(r documentRow) bool { return r.parent == id })
}

func (s *documentStore) Expired(_ context.Context, cutoff time.Time, passed []shared.ID, batch int) ([]TrashedDocument, error) {
	s.asked = cutoff
	var due []TrashedDocument
	for _, row := range s.rows {
		if row.deletedAt.Before(cutoff) && !slices.Contains(passed, row.doc.ID) && !s.hasChild(row.doc.ID) {
			due = append(due, row.doc)
		}
		if batch > 0 && len(due) >= batch {
			break
		}
	}
	return due, nil
}

func (s *documentStore) Remove(_ context.Context, documents []TrashedDocument, cutoff time.Time) (int, error) {
	removed := 0
	for _, document := range documents {
		index := slices.IndexFunc(s.rows, func(r documentRow) bool { return r.doc.ID == document.ID })
		if index < 0 || !s.rows[index].deletedAt.Before(cutoff) || s.hasChild(document.ID) {
			continue
		}
		s.rows = slices.Delete(s.rows, index, index+1)
		removed++
	}
	return removed, nil
}

type changeSink struct{ recorded []changelog.Change }

func (c *changeSink) Record(_ context.Context, change changelog.Change) error {
	c.recorded = append(c.recorded, change)
	return nil
}

func docID(n int) shared.ID {
	return shared.MustParseID("0192f000-0000-7000-8000-0000000d00" + string(rune('0'+n/10)) + string(rune('0'+n%10)))
}

func documentHarness(rows ...documentRow) (*runHarness, *documentStore, *changeSink) {
	h := newRunHarness()
	store := &documentStore{rows: rows}
	changes := &changeSink{}
	h.run.Documents = DocumentSweep{Trash: store, Changes: changes, HLC: &hlcSource{}}
	return h, store, changes
}

func trashedPage(n int, days int, parent shared.ID, contributors ...shared.ID) documentRow {
	return documentRow{
		doc:       TrashedDocument{Entity: "page", ID: docID(n), HubID: hubID, Contributors: contributors},
		deletedAt: now.AddDate(0, 0, -days), parent: parent,
	}
}

// accepted.md B: a page trashed on its own goes after the trash's thirty days, journalled and
// tombstoned, and devices are told; one inside the period stays.
func TestATrashedPageGoesAfterTheTrashPeriod(t *testing.T) {
	h, store, changes := documentHarness(trashedPage(1, 31, ""), trashedPage(2, 29, ""),
		documentRow{doc: TrashedDocument{Entity: "whiteboard", ID: docID(3), HubID: hubID}, deletedAt: now.AddDate(0, 0, -40)})

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if want := now.AddDate(0, 0, -30); !store.asked.Equal(want) {
		t.Errorf("the cutoff was %v, want the trash's %v", store.asked, want)
	}
	if len(store.rows) != 1 || store.rows[0].doc.ID != docID(2) {
		t.Fatalf("left %+v, want only the page inside the period", store.rows)
	}
	if outcome.Removed < 2 {
		t.Errorf("removed %d, want the page and the board counted", outcome.Removed)
	}
	journalled := map[string]bool{}
	for _, removal := range h.removals.recorded {
		journalled[removal.Entity+removal.EntityID.String()] = removal.Reason == domain.DeletedByRetention
	}
	if !journalled["page"+docID(1).String()] || !journalled["whiteboard"+docID(3).String()] {
		t.Errorf("journalled %+v, want both under retention", h.removals.recorded)
	}
	if len(changes.recorded) != 2 || changes.recorded[0].Op != changelog.Delete || changes.recorded[0].ContainerID != hubID {
		t.Errorf("devices were told %+v", changes.recorded)
	}
}

// A page goes only with everything below it: a trashed tree goes leaf by leaf in one pass, and a
// page under it that is not due keeps its parents.
func TestATrashedTreeGoesFromTheLeavesAndALivingChildKeepsItsParents(t *testing.T) {
	h, store, _ := documentHarness(trashedPage(1, 40, ""), trashedPage(2, 40, docID(1)), trashedPage(3, 40, docID(2)))
	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if len(store.rows) != 0 {
		t.Fatalf("left %+v, want the whole due tree gone in one pass", store.rows)
	}

	h, store, _ = documentHarness(trashedPage(1, 40, ""), trashedPage(2, 40, docID(1)), trashedPage(3, 10, docID(2)))
	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if len(store.rows) != 3 {
		t.Errorf("left %d, want the tree kept above a page inside the period", len(store.rows))
	}
}

// A hold on the hub or on a contributor keeps a trashed page, counted as held, and its parent with
// it; a hold elsewhere does not.
func TestAHoldKeepsATrashedPage(t *testing.T) {
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000ff")
	for _, tc := range []struct {
		name  string
		holds domain.Holds
		kept  bool
	}{
		{"on the workspace", domain.Holds{{ID: holdID, Scope: domain.HoldTenant, Reason: "Litigation"}}, true},
		{"on the hub", domain.Holds{{ID: holdID, Scope: domain.HoldContainer, ScopeID: hubID, Reason: "Litigation"}}, true},
		{"on a contributor", domain.Holds{{ID: holdID, Scope: domain.HoldAccount, ScopeID: accountID, Reason: "Litigation"}}, true},
		{"on another hub", domain.Holds{{ID: holdID, Scope: domain.HoldContainer, ScopeID: other, Reason: "Litigation"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, store, changes := documentHarness(trashedPage(1, 40, ""), trashedPage(2, 40, docID(1), accountID))
			h.holds.holds = tc.holds
			outcome, err := h.run.Execute(t.Context(), actor())
			if err != nil {
				t.Fatalf("the run failed: %v", err)
			}
			if kept := len(store.rows) == 2; kept != tc.kept {
				t.Fatalf("kept %v (%d left), want %v", kept, len(store.rows), tc.kept)
			}
			if tc.kept && (outcome.Blocked[BlockedByLegalHold] == 0 || len(changes.recorded) != 0 || len(h.removals.recorded) != 0) {
				t.Errorf("a held page reported %+v, told %d, journalled %d", outcome, len(changes.recorded), len(h.removals.recorded))
			}
		})
	}
}

// pageVersionStore stands in for the thinning statement: it records what the pass kept back.
type pageVersionStore struct {
	due      int
	asked    time.Time
	hubs     []shared.ID
	accounts []shared.ID
}

func (s *pageVersionStore) DeleteThinned(_ context.Context, cutoff time.Time, hubs, accounts []shared.ID, batch int) (int, error) {
	s.asked, s.hubs, s.accounts = cutoff, hubs, accounts
	n := min(s.due, batch)
	s.due -= n
	return n, nil
}

func (s *pageVersionStore) CountThinned(_ context.Context, cutoff time.Time, hubs, accounts []shared.ID, ceiling int) (int, error) {
	return min(s.due, ceiling), nil
}

// PAGE_VERSION: ninety days; a hold on a hub or an account is handed to the statement, which keeps
// what it covers; a hold on the workspace stops the thinning.
func TestPageVersionsAreThinnedAfterNinetyDaysAndHoldsKeepThem(t *testing.T) {
	h := newRunHarness()
	versions := &pageVersionStore{due: 3}
	h.run.Documents = DocumentSweep{Versions: versions}
	h.holds.holds = domain.Holds{
		{ID: holdID, Scope: domain.HoldContainer, ScopeID: hubID, Reason: "Audit"},
		{ID: holdID, Scope: domain.HoldAccount, ScopeID: accountID, Reason: "Audit"},
	}
	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if want := now.AddDate(0, 0, -90); !versions.asked.Equal(want) {
		t.Errorf("the cutoff was %v, want %v", versions.asked, want)
	}
	if versions.due != 0 || !slices.Contains(versions.hubs, hubID) || !slices.Contains(versions.accounts, accountID) {
		t.Errorf("left %d, held hubs %v, accounts %v", versions.due, versions.hubs, versions.accounts)
	}

	h = newRunHarness()
	versions = &pageVersionStore{due: 3}
	h.run.Documents = DocumentSweep{Versions: versions}
	h.holds.holds = domain.Holds{{ID: holdID, Scope: domain.HoldTenant, Reason: "Litigation"}}
	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if versions.due != 3 || outcome.Blocked[BlockedByLegalHold] == 0 {
		t.Errorf("a workspace hold left %d of 3 and reported %+v", versions.due, outcome)
	}
}

// boardHistoryStore stands in for the board history's statement: it records what the pass asked.
type boardHistoryStore struct {
	due      int
	asked    time.Time
	hubs     []shared.ID
	accounts []shared.ID
}

func (s *boardHistoryStore) DeleteAged(_ context.Context, cutoff time.Time, hubs, accounts []shared.ID, batch int) (int, error) {
	s.asked, s.hubs, s.accounts = cutoff, hubs, accounts
	n := min(s.due, batch)
	s.due -= n
	return n, nil
}

func (s *boardHistoryStore) CountAged(_ context.Context, _ time.Time, _, _ []shared.ID, ceiling int) (int, error) {
	return min(s.due, ceiling), nil
}

// WHITEBOARD_HISTORY (UC-CNV-06 check 5): kept whole without the workspace's rule, and with a rule
// that only announces; aged out past the period of a workspace's HARD_DELETE rule, a hub's or an
// account's hold handed to the statement and a workspace's hold stopping it.
func TestABoardsHistoryIsKeptUnlessTheWorkspaceAgesItOut(t *testing.T) {
	rule := func(action domain.Action, enabled bool) *ruleStore {
		return &ruleStore{stored: []domain.Rule{{
			ID: holdID, Scope: domain.Scope{Kind: domain.ScopeTenant}, DataKind: domain.KindWhiteboardHistory,
			RetainDays: 180, Action: action, Enabled: enabled,
		}}}
	}
	for _, tc := range []struct {
		name  string
		rules *ruleStore
	}{
		{"no rule", &ruleStore{}},
		{"a rule that only announces", rule(domain.ActionNotifyOnly, true)},
		{"a rule switched off", rule(domain.ActionHardDelete, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRunHarness()
			history := &boardHistoryStore{due: 3}
			h.run.Documents = DocumentSweep{Whiteboards: history, Rules: tc.rules}
			if _, err := h.run.Execute(t.Context(), actor()); err != nil {
				t.Fatalf("the run failed: %v", err)
			}
			if history.due != 3 || !history.asked.IsZero() {
				t.Fatalf("%d of 3 left, asked for %v", history.due, history.asked)
			}
		})
	}

	h := newRunHarness()
	history := &boardHistoryStore{due: 3}
	h.run.Documents = DocumentSweep{Whiteboards: history, Rules: rule(domain.ActionHardDelete, true)}
	h.holds.holds = domain.Holds{
		{ID: holdID, Scope: domain.HoldContainer, ScopeID: hubID, Reason: "Audit"},
		{ID: holdID, Scope: domain.HoldAccount, ScopeID: accountID, Reason: "Audit"},
	}
	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if want := now.AddDate(0, 0, -180); !history.asked.Equal(want) {
		t.Errorf("the cutoff was %v, want %v", history.asked, want)
	}
	if history.due != 0 || !slices.Contains(history.hubs, hubID) || !slices.Contains(history.accounts, accountID) {
		t.Errorf("left %d, held hubs %v, accounts %v", history.due, history.hubs, history.accounts)
	}

	h = newRunHarness()
	history = &boardHistoryStore{due: 3}
	h.run.Documents = DocumentSweep{Whiteboards: history, Rules: rule(domain.ActionHardDelete, true)}
	h.holds.holds = domain.Holds{{ID: holdID, Scope: domain.HoldTenant, Reason: "Litigation"}}
	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if history.due != 3 || outcome.Blocked[BlockedByLegalHold] == 0 {
		t.Errorf("a workspace hold left %d of 3 and reported %+v", history.due, outcome)
	}
}
