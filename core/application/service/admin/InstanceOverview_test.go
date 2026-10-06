// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// censusStore answers the counts the function would.
type censusStore struct {
	census adminrepo.Census
	asked  int
}

func (s *censusStore) Census(context.Context) (adminrepo.Census, error) {
	s.asked++
	return s.census, nil
}

// The overview is counts and states, and nothing that could carry a workspace's content: what the
// use case answers is exactly the five numbers the census holds (ADR-0070 §5).
func TestTheOverviewIsCountsAndNothingElse(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(operatorID))
	census := &censusStore{census: adminrepo.Census{
		WorkspacesActive: 12, WorkspacesSuspended: 2, WorkspacesPendingDeletion: 1,
		AccountsActive: 340, AccountsTotal: 361,
	}}

	out, err := ReadInstanceOverview{Writer: writer, Installation: census}.
		invoke(t.Context(), operator(), nil)
	if err != nil {
		t.Fatalf("reading the overview: %v", err)
	}
	if census.asked != 1 {
		t.Errorf("the census was asked %d times", census.asked)
	}

	want := map[string]int64{
		"workspaces_active": 12, "workspaces_suspended": 2, "workspaces_pending_deletion": 1,
		"accounts_active": 340, "accounts_total": 361,
	}
	if len(out) != len(want) {
		t.Errorf("the overview answers %d fields, want the five counts: %v", len(out), out)
	}
	for field, value := range want {
		if out[field] != value {
			t.Errorf("%s is %v, want %d", field, out[field], value)
		}
	}
}

// The pair every operation at this level demands: the scope, then the register. Either alone is a
// hole, and a read is not the exception.
func TestTheOverviewAndTheJournalDemandTheRegisterAsWellAsTheScope(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(secondOperator))
	census := &censusStore{}

	if _, err := (ReadInstanceOverview{Writer: writer, Installation: census}).
		Execute(t.Context(), operator()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a reader outside the register answered %v", err)
	}
	if census.asked != 0 {
		t.Error("a refused caller reached the census")
	}
	if _, err := (ListInstanceJournal{Writer: writer}).
		Execute(t.Context(), operator(), "", 0); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a journal read outside the register answered %v", err)
	}
	if _, err := (ReadInstanceOverview{Writer: writer, Installation: census}).
		Execute(t.Context(), anonymousActor()); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("a caller with no credential answered %v", err)
	}
}

// Newest first, one page at a time, and the size is clamped rather than refused: a client asking
// for five hundred entries wants as many as it can have.
func TestTheJournalIsWalkedNewestFirstAndTheSizeIsClamped(t *testing.T) {
	writer, _, journal := newInstanceWriter(newRegister(operatorID))
	at := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	for i := range 5 {
		if err := journal.Record(t.Context(), adminrepo.InstanceEvent{
			ID:         shared.ID("01936f2a-7c1e-7000-8000-00000000000" + strconv.Itoa(i)),
			OccurredAt: at.Add(time.Duration(i) * time.Minute),
			Action:     "tenant.provisioned", TenantSlug: "acme-" + strconv.Itoa(i),
		}); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}

	page, err := ListInstanceJournal{Writer: writer}.Execute(t.Context(), operator(), "", 2)
	if err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	if len(page.Entries) != 2 || !page.Page.HasMore {
		t.Fatalf("the first page is %+v", page)
	}
	if page.Entries[0].TenantSlug != "acme-4" {
		t.Errorf("the first entry is %q, want the newest", page.Entries[0].TenantSlug)
	}
	if page.Page.NextCursor == "" {
		t.Error("a page with more behind it carries no cursor")
	}

	second, err := ListInstanceJournal{Writer: writer}.
		Execute(t.Context(), operator(), page.Page.NextCursor, 2)
	if err != nil {
		t.Fatalf("reading the second page: %v", err)
	}
	if len(second.Entries) != 2 || second.Entries[0].TenantSlug != "acme-2" {
		t.Errorf("the second page is %+v", second.Entries)
	}

	// Past the ceiling is the ceiling, and nothing at all is the default.
	if got := journalPageSize(500); got != maxJournalPage {
		t.Errorf("a request for 500 answered %d", got)
	}
	if got := journalPageSize(0); got != defaultJournalPage {
		t.Errorf("a request for none answered %d", got)
	}
}

// An entry that names no workspace and carries no details answers neither field: an absent key is
// what a reader branches on, and an empty one would have to be branched on twice.
func TestAJournalEntryAnswersOnlyWhatItHas(t *testing.T) {
	writer, _, journal := newInstanceWriter(newRegister(operatorID))
	if err := journal.Record(t.Context(), adminrepo.InstanceEvent{
		ID:         shared.ID("01936f2a-7c1e-7000-8000-000000000001"),
		OccurredAt: time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC),
		Action:     "instance.settings_changed", ActorLabel: "The operator",
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}

	out, err := ListInstanceJournal{Writer: writer}.invoke(t.Context(), operator(), nil)
	if err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	rows, held := out["data"].([]usecase.Output)
	if !held {
		t.Fatalf("the journal answers %T", out["data"])
	}
	if len(rows) != 1 {
		t.Fatalf("the journal answers %d entries", len(rows))
	}
	for _, absent := range []string{"tenant_id", "tenant_slug", "details"} {
		if _, held := rows[0][absent]; held {
			t.Errorf("an entry that has no %s answered one", absent)
		}
	}
	if rows[0]["actor_label"] != "The operator" {
		t.Errorf("the actor is %v", rows[0]["actor_label"])
	}
}
