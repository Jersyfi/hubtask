// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The extension's statement against a real PostgreSQL (UC-PRV-01 check 9): the guard that leaves
// one winner, the row level security that keeps it inside the workspace, and the columns read back
// as they were written.

// extensionTenant seeds a workspace of its own and one open case in it.
func extensionTenant(ctx context.Context, t *testing.T, received, due time.Time) (shared.ID, domain.Request) {
	t.Helper()
	tenant := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Extension')`,
		tenant.String(), slugOf(tenant)); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}

	request := domain.Request{
		ID: freshID(t), Kind: domain.KindAccess, Status: domain.StatusReceived,
		Scope: domain.ScopeTenant, SubjectEmail: "ext-" + tenant.String() + "@example.org",
		ReceivedAt: received, DueAt: due,
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return privacyRepo().Insert(ctx, request)
	}); err != nil {
		t.Fatalf("recording the case: %v", err)
	}
	return tenant, request
}

func extendOnce(
	ctx context.Context, t *testing.T, tenant shared.ID, request domain.Request, now time.Time,
) bool {
	t.Helper()
	var extended bool
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		extended, err = privacyRepo().Extend(ctx, request, now)
		return err
	}); err != nil {
		t.Fatalf("extending: %v", err)
	}
	return extended
}

func findCase(ctx context.Context, t *testing.T, tenant, id shared.ID) domain.Request {
	t.Helper()
	var found domain.Request
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		found, err = privacyRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the case: %v", err)
	}
	return found
}

func TestTheExtensionStatementWritesOnceAndReadsBack(t *testing.T) {
	ctx := context.Background()
	received := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	original := time.Date(2026, 12, 2, 8, 0, 0, 0, time.UTC)
	tenant, request := extensionTenant(ctx, t, received, original)
	now := received.Add(48 * time.Hour)

	extended := request
	extended.DueAt = time.Date(2027, 1, 15, 22, 59, 59, 0, time.UTC)
	extended.ExtensionReason = domain.ReasonComplexity
	extended.InformedOn = domain.Day{Year: 2026, Month: time.November, Day: 3}

	if !extendOnce(ctx, t, tenant, extended, now) {
		t.Fatal("the first extension wrote nothing")
	}

	// The round trip of the columns: Find and List map the same row.
	found := findCase(ctx, t, tenant, request.ID)
	if !found.OriginalDueAt.Equal(original) || !found.DueAt.Equal(extended.DueAt) ||
		found.ExtensionReason != domain.ReasonComplexity || found.InformedOn.String() != "2026-11-03" {
		t.Errorf("the case reads back as original %s, due %s, %s, informed %s",
			found.OriginalDueAt, found.DueAt, found.ExtensionReason, found.InformedOn)
	}

	// The second write meets the guard: the original stays the first one.
	again := extended
	again.DueAt = time.Date(2027, 1, 20, 22, 59, 59, 0, time.UTC)
	if extendOnce(ctx, t, tenant, again, now) {
		t.Error("a second extension was written")
	}
	if found := findCase(ctx, t, tenant, request.ID); !found.DueAt.Equal(extended.DueAt) ||
		!found.OriginalDueAt.Equal(original) {
		t.Errorf("the second extension moved the case to %s (original %s)", found.DueAt, found.OriginalDueAt)
	}
}

func TestTheExtensionStatementRefusesALateOrClosedCase(t *testing.T) {
	ctx := context.Background()
	received := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	original := time.Date(2026, 12, 2, 8, 0, 0, 0, time.UTC)

	late, lateCase := extensionTenant(ctx, t, received, original)
	lateCase.DueAt = time.Date(2027, 1, 15, 22, 59, 59, 0, time.UTC)
	lateCase.ExtensionReason, lateCase.InformedOn = domain.ReasonComplexity, domain.Day{Year: 2026, Month: 11, Day: 3}
	if extendOnce(ctx, t, late, lateCase, original) {
		t.Error("a case was extended at its deadline")
	}

	closed, closedCase := extensionTenant(ctx, t, received, original)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE data_subject_request SET status = 'COMPLETED', completed_at = $2 WHERE id = $1`,
		closedCase.ID.String(), received.Add(time.Hour)); err != nil {
		t.Fatalf("closing the case: %v", err)
	}
	closedCase.DueAt, closedCase.ExtensionReason = lateCase.DueAt, domain.ReasonComplexity
	closedCase.InformedOn = lateCase.InformedOn
	if extendOnce(ctx, t, closed, closedCase, received.Add(2*time.Hour)) {
		t.Error("a closed case was extended")
	}
}

// SG-3: another workspace's case is not there, and the statement writes nothing to it.
func TestTheExtensionStatementStaysInsideTheWorkspace(t *testing.T) {
	ctx := context.Background()
	received := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	original := time.Date(2026, 12, 2, 8, 0, 0, 0, time.UTC)
	owner, request := extensionTenant(ctx, t, received, original)
	stranger, _ := extensionTenant(ctx, t, received, original)

	extended := request
	extended.DueAt = time.Date(2027, 1, 15, 22, 59, 59, 0, time.UTC)
	extended.ExtensionReason, extended.InformedOn = domain.ReasonNumberOfRequests, domain.Day{Year: 2026, Month: 11, Day: 3}
	if extendOnce(ctx, t, stranger, extended, received.Add(time.Hour)) {
		t.Fatal("another workspace extended the case")
	}
	if found := findCase(ctx, t, owner, request.ID); found.Extended() || !found.DueAt.Equal(original) {
		t.Errorf("the case was moved from another workspace: %+v", found)
	}
}

// The database holds the extension whole: a moved deadline without its reason is refused there too.
func TestAnExtensionIsStoredWholeOrNotAtAll(t *testing.T) {
	ctx := context.Background()
	received := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	_, request := extensionTenant(ctx, t, received, received.Add(30*24*time.Hour))

	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE data_subject_request SET original_due_at = due_at WHERE id = $1`,
		request.ID.String()); err == nil {
		t.Error("an original deadline without a reason was stored")
	}
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE data_subject_request
		 SET original_due_at = due_at, extension_reason = 'HOLIDAYS', informed_on = '2026-11-03'
		 WHERE id = $1`, request.ID.String()); err == nil {
		t.Error("a reason the law does not name was stored")
	}
}
