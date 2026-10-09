// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The statements behind what a legal hold keeps from an erasure (data-protection.md §4.1), each
// against the real schema and each from another workspace too (SG-3).

// keptCase records an erasure case for the subject, which the kept rows hang off.
func keptCase(ctx context.Context, t *testing.T, subject person) domain.Request {
	t.Helper()
	request := domain.Request{
		ID: freshID(t), Kind: domain.KindErasure, Status: domain.StatusInProgress,
		Scope: domain.ScopeTenant, SubjectAccountID: subject.account, ErasureMode: domain.ModeAnonymize,
		ReceivedAt: created, DueAt: created.Add(30 * 24 * time.Hour),
	}
	if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
		return privacyRepo().Insert(ctx, request)
	}); err != nil {
		t.Fatalf("recording the case: %v", err)
	}
	return request
}

// The person's rows on entries come back with where each entry is: the comment they wrote, the
// entry assigned to them, the entry they created - and nothing to another workspace.
func TestThePersonsRowsOnEntriesComeBackWithWhereTheyAre(t *testing.T) {
	ctx := context.Background()
	subject := seedSubject(ctx, t, "rows@example.test")
	seedContainerTenants(ctx, t)

	contributions := func(tenant shared.ID) []repository.Contribution {
		t.Helper()
		var found []repository.Contribution
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			found, err = privacyRepo().Contributions(ctx, subject.account)
			return err
		}); err != nil {
			t.Fatalf("reading the contributions: %v", err)
		}
		return found
	}

	found := contributions(subject.tenant)
	kinds := map[repository.ContributionKind]repository.Contribution{}
	for _, contribution := range found {
		kinds[contribution.Kind] = contribution
	}
	if len(found) != 3 || len(kinds) != 3 {
		t.Fatalf("the contributions are %+v, want one comment, one assignment, one entry", found)
	}
	comment := kinds[repository.ContributedComment]
	if comment.ID != subject.comment || comment.ItemID != subject.item {
		t.Errorf("the comment came back as %+v", comment)
	}
	for kind, contribution := range kinds {
		if contribution.CollectionID.IsZero() || contribution.HubID.IsZero() ||
			contribution.ItemCreatedBy != subject.account || contribution.Path == "" {
			t.Errorf("the %s does not say where it is: %+v", kind, contribution)
		}
	}
	if other := contributions(tenantB); len(other) != 0 {
		t.Errorf("another workspace was told %+v", other)
	}
}

// The deletes take what was judged, and nothing else: another comment of the person's stays, and so
// does the assignment on an entry not named.
func TestTheErasuresDeletesTakeOnlyWhatTheyAreHanded(t *testing.T) {
	ctx := context.Background()
	subject := seedSubject(ctx, t, "scoped@example.test")
	seedContainerTenants(ctx, t)
	kept := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, 'kept')`,
		kept.String(), subject.tenant.String(), subject.item.String(), subject.account.String()); err != nil {
		t.Fatalf("seeding the second comment: %v", err)
	}

	// Another workspace reaches nothing.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		removed, err := privacyRepo().DeleteComments(ctx, subject.account, []shared.ID{subject.comment})
		if removed != 0 {
			t.Errorf("another workspace removed %d comments", removed)
		}
		if err != nil {
			return err
		}
		released, err := privacyRepo().ReleaseAssignmentsOn(ctx, subject.account, []shared.ID{subject.item}, created)
		if released != 0 {
			t.Errorf("another workspace released %d assignments", released)
		}
		return err
	}); err != nil {
		t.Fatalf("from another workspace: %v", err)
	}

	if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
		removed, err := privacyRepo().DeleteComments(ctx, subject.account, []shared.ID{subject.comment})
		if err != nil {
			return err
		}
		if removed != 1 {
			t.Errorf("%d comments were removed, want the one named", removed)
		}
		released, err := privacyRepo().ReleaseAssignmentsOn(ctx, subject.account, []shared.ID{freshID(t)}, created)
		if released != 0 {
			t.Errorf("%d assignments were released on an entry not named", released)
		}
		return err
	}); err != nil {
		t.Fatalf("the scoped deletes: %v", err)
	}

	if countIn(ctx, t, `SELECT count(*) FROM comment WHERE id = $1`, kept.String()) != 1 {
		t.Error("a comment that was not named was removed")
	}
	if countIn(ctx, t, `SELECT count(*) FROM work_item WHERE id = $1 AND assignee_id = $2`,
		subject.item.String(), subject.account.String()) != 1 {
		t.Error("an assignment that was not named was released")
	}
}

// What each hold kept is recorded, read back, made pending again, blocked and finally erased - the
// whole life of a row, and none of it visible from another workspace.
func TestWhatAHoldKeptIsRecordedAndMovesWithTheRemainder(t *testing.T) {
	ctx := context.Background()
	subject := seedSubject(ctx, t, "kept@example.test")
	seedContainerTenants(ctx, t)
	request := keptCase(ctx, t, subject)
	hub, account := freshID(t), freshID(t)
	first := created.Add(time.Hour)

	parts := []domain.Kept{
		{HoldID: hub, HoldScope: lifecycle.HoldContainer, HoldScopeID: subject.item, Entries: 1, Comments: 2, Assignments: 1},
		{HoldID: account, HoldScope: lifecycle.HoldAccount, HoldScopeID: subject.account, Account: true},
	}
	keptOf := func(tenant shared.ID) []domain.Kept {
		t.Helper()
		var found map[shared.ID][]domain.Kept
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			found, err = privacyRepo().KeptOf(ctx, []shared.ID{request.ID})
			return err
		}); err != nil {
			t.Fatalf("reading what was kept: %v", err)
		}
		return found[request.ID]
	}
	pending := func(hold shared.ID) []repository.PendingKept {
		t.Helper()
		var found []repository.PendingKept
		if err := read(ctx, t, subject.tenant, func(ctx context.Context) error {
			var err error
			found, err = privacyRepo().PendingKept(ctx, hold)
			return err
		}); err != nil {
			t.Fatalf("reading what is pending: %v", err)
		}
		return found
	}

	if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
		locked, err := privacyRepo().Lock(ctx, request.ID)
		if !locked {
			t.Error("the case could not be locked")
		}
		if err != nil {
			return err
		}
		return privacyRepo().RecordKept(ctx, request.ID, parts, first)
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}

	stored := keptOf(subject.tenant)
	if len(stored) != 2 {
		t.Fatalf("%d parts stored, want 2", len(stored))
	}
	for _, part := range stored {
		if !part.Pending() || !part.RecordedAt.Equal(first) {
			t.Errorf("a new part came back as %+v", part)
		}
		if part.HoldID == hub && (part.Comments != 2 || part.Entries != 1 || part.Account) {
			t.Errorf("the hub's part came back as %+v", part)
		}
		if part.HoldID == account && !part.Account {
			t.Errorf("the account's part lost the account: %+v", part)
		}
	}
	if got := pending(hub); len(got) != 1 || got[0].RequestID != request.ID {
		t.Errorf("pending under the hub hold: %+v", got)
	}
	if got := pending(""); len(got) != 2 {
		t.Errorf("pending under any hold: %+v", got)
	}

	// The rest waits for a reason, then the hub's part goes and the account's stays.
	later := first.Add(time.Hour)
	if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
		if err := privacyRepo().BlockKept(ctx, request.ID, "privacy.erasure_blocked_by_rule",
			map[string]string{"rules": "1"}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("blocking: %v", err)
	}
	for _, part := range keptOf(subject.tenant) {
		if part.BlockedCode != "privacy.erasure_blocked_by_rule" || part.BlockedParams["rules"] != "1" {
			t.Errorf("a pending part does not say why it waits: %+v", part)
		}
	}
	if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
		return privacyRepo().RecordKept(ctx, request.ID, parts[1:], later)
	}); err != nil {
		t.Fatalf("recording the remainder: %v", err)
	}
	for _, part := range keptOf(subject.tenant) {
		switch part.HoldID {
		case hub:
			if !part.ErasedAt.Equal(later) || part.BlockedCode != "" {
				t.Errorf("the hub's part was not erased cleanly: %+v", part)
			}
		case account:
			if !part.Pending() || part.BlockedCode != "" || !part.RecordedAt.Equal(first) {
				t.Errorf("the account's part came back as %+v", part)
			}
		}
	}
	if got := pending(hub); len(got) != 0 {
		t.Errorf("the hub hold still keeps %+v", got)
	}

	// Another workspace sees none of it, cannot lock the case, and changes nothing.
	if other := keptOf(tenantB); len(other) != 0 {
		t.Errorf("another workspace was told %+v", other)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		locked, err := privacyRepo().Lock(ctx, request.ID)
		if locked {
			t.Error("another workspace locked the case")
		}
		if err != nil {
			return err
		}
		return privacyRepo().RecordKept(ctx, request.ID, nil, later.Add(time.Hour))
	}); err == nil {
		// Its insert would name its own tenant against a case it cannot see; nothing it does may
		// mark this workspace's part erased.
		for _, part := range keptOf(subject.tenant) {
			if part.HoldID == account && !part.Pending() {
				t.Error("another workspace marked the account's part erased")
			}
		}
	}
}
