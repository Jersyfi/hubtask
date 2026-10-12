// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// restrictedIn reads whose processing is restricted, under one workspace, as the application role.
func restrictedIn(ctx context.Context, t *testing.T, tenant shared.ID) domain.Restricted {
	t.Helper()
	var people []domain.RestrictedPerson
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		people, err = privacyRepo().RestrictedPeople(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the restricted: %v", err)
	}
	return domain.NewRestricted(people)
}

// Who is restricted (data-protection.md §4, §4.1): an account in the RESTRICTED state, and an
// anonymised one only while a hold still keeps contributions of it - not every former user.
func TestWhoseProcessingIsRestricted(t *testing.T) {
	ctx := context.Background()

	restricted := seedSubject(ctx, t, "restricted-"+freshID(t).String()+"@example.test")
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE account SET status = 'RESTRICTED' WHERE id = $1`,
		restricted.account.String()); err != nil {
		t.Fatalf("restricting: %v", err)
	}
	set := restrictedIn(ctx, t, restricted.tenant)
	if !set.Has(restricted.account) || !set.HasAddress(emailOf(ctx, t, restricted.account)) {
		t.Error("a RESTRICTED account, or its address, is not in the set")
	}

	// Anonymised under a hold on its hub: kept, so restricted, until the rest is erased.
	kept := seedSubject(ctx, t, "kept-"+freshID(t).String()+"@example.test")
	holdOn(ctx, t, kept.tenant, "CONTAINER", kept.hub)
	request := erasureRequest(ctx, t, kept, domain.ModeFullDelete)
	if _, err := performerFor(t).Perform(ctx, privacyservice.PerformInput{
		RequestID: request.ID, TenantID: kept.tenant,
	}); err != nil {
		t.Fatalf("carrying out the case: %v", err)
	}
	if !restrictedIn(ctx, t, kept.tenant).Has(kept.account) {
		t.Error("an anonymised account a hold keeps contributions of is not restricted")
	}
	// The last kept part erased: the former user's content is the workspace's again.
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE erasure_kept SET erased_at = now() WHERE request_id = $1`, request.ID.String()); err != nil {
		t.Fatalf("erasing the rest: %v", err)
	}
	if restrictedIn(ctx, t, kept.tenant).Has(kept.account) {
		t.Error("a former user stays restricted after the last kept part was erased")
	}

	// Anonymised without a hold: not restricted.
	gone := seedSubject(ctx, t, "gone-"+freshID(t).String()+"@example.test")
	plain := erasureRequest(ctx, t, gone, domain.ModeAnonymize)
	if _, err := performerFor(t).Perform(ctx, privacyservice.PerformInput{
		RequestID: plain.ID, TenantID: gone.tenant,
	}); err != nil {
		t.Fatalf("carrying out the case: %v", err)
	}
	if restrictedIn(ctx, t, gone.tenant).Has(gone.account) {
		t.Error("a former user no hold keeps anything of is restricted")
	}
}

// Another workspace's restricted people are not this one's.
func TestTheRestrictedOfAnotherWorkspaceAreInvisible(t *testing.T) {
	ctx := context.Background()
	theirs := seedSubject(ctx, t, "theirs-"+freshID(t).String()+"@example.test")
	ours := seedSubject(ctx, t, "ours-"+freshID(t).String()+"@example.test")
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE account SET status = 'RESTRICTED' WHERE id = $1`,
		theirs.account.String()); err != nil {
		t.Fatalf("restricting: %v", err)
	}
	if set := restrictedIn(ctx, t, ours.tenant); !set.Empty() {
		t.Error("another workspace's restricted account was answered")
	}
}

func emailOf(ctx context.Context, t *testing.T, account shared.ID) string {
	t.Helper()
	var email string
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT email FROM account WHERE id = $1`,
		account.String()).Scan(&email); err != nil {
		t.Fatalf("reading the address: %v", err)
	}
	return email
}
