// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The sections of an address's own copy against the real statements: the intake's log of the
// address and the rating links mailed to it, each read in its workspace alone, the address matched
// whatever its case, and another address's receipt left out (data-protection.md §4.1).
func TestAnAddressCopyReadsItsReceiptsAndRatingLinksInItsWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	item := seedWorkItemForRules(ctx, t, tenantA)
	requesters := postgres.RequesterRepository{IDs: freshIDs{t}}
	links := postgres.NewSatisfactionLinkRepository(security.NewSatisfactionLinkHasher(secret.New(feedSecret)))
	address := "copy-" + freshID(t).String()[28:] + "@shop.test"
	now := time.Now().UTC().Truncate(time.Microsecond)

	for _, receipt := range []struct {
		tenant string
		sender string
	}{
		{tenantA.String(), strings.ToUpper(address[:1]) + address[1:]},
		{tenantA.String(), "other-" + address},
		{tenantB.String(), address},
	} {
		if _, err := admin.Exec(ctx, `INSERT INTO mail_receipt (id, tenant_id, message_id, received_at, sender, subject, outcome, door, item_id)
			VALUES ($1, $2, $3, now(), $4, 'Printer', 'ENTRY', 'MAIL', $5)`,
			freshID(t).String(), receipt.tenant, "<"+freshID(t).String()+"@shop.test>", receipt.sender, item.String()); err != nil {
			t.Fatalf("writing a receipt: %v", err)
		}
	}
	token, err := integration.NewSatisfactionLinkToken(tenantA,
		bytes.Repeat([]byte{byte(freshID(t).String()[35])}, integration.SatisfactionLinkTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := requesters.RecordContact(ctx, address, "Kunde", item, now); err != nil {
			return err
		}
		on, err := requesters.AddressesOn(ctx, item)
		if err != nil || len(on) == 0 {
			t.Fatalf("the entry's requesters are %+v (%v)", on, err)
		}
		link, err := servicedesk.NewSatisfactionLink(tenantA, on[0].ID, item, now)
		if err != nil {
			return err
		}
		return links.Put(ctx, link, token)
	}); err != nil {
		t.Fatal(err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		receipts, err := requesters.MailReceiptsFrom(ctx, address)
		if err != nil {
			return err
		}
		if len(receipts) != 1 || receipts[0].Subject != "Printer" || receipts[0].ItemID != item || !receipts[0].EntryID.IsZero() {
			t.Errorf("the address's receipts in its workspace are %+v", receipts)
		}
		rated, err := requesters.RatingLinksTo(ctx, address)
		if err != nil {
			return err
		}
		if len(rated) != 1 || rated[0].ItemID != item || rated[0].UsedAt != nil ||
			!rated[0].ExpiresAt.Equal(now.Add(servicedesk.SatisfactionLinkLife)) {
			t.Errorf("the address's rating links are %+v", rated)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		receipts, err := requesters.MailReceiptsFrom(ctx, address)
		if err != nil {
			return err
		}
		if len(receipts) != 1 || receipts[0].ItemID != item {
			t.Errorf("the workspace next door read the receipts %+v", receipts)
		}
		rated, err := requesters.RatingLinksTo(ctx, address)
		if err != nil {
			return err
		}
		if len(rated) != 0 {
			t.Errorf("the workspace next door read the rating links %+v", rated)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
