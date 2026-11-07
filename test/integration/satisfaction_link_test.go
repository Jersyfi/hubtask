// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// UC-SVC-07 check 6 against the real statements: a satisfaction link is found by its token in its
// workspace alone, answers once and only while it lives, is replaced by a later one, and goes with
// the requester's record.
func TestASatisfactionLinkAnswersOnceInItsWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, 'tenant-a', 'A', 'de', 'Europe/Berlin'), ($2, 'tenant-b', 'B', 'en', 'UTC')
		ON CONFLICT (id) DO NOTHING`, tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding the workspaces: %v", err)
	}
	item := seedWorkItemForRules(ctx, t, tenantA)
	requesters := postgres.RequesterRepository{IDs: freshIDs{t}}
	links := postgres.NewSatisfactionLinkRepository(security.NewSatisfactionLinkHasher(secret.New(feedSecret)))
	address := "kaya-" + freshID(t).String()[28:] + "@shop.test"
	now := time.Now().UTC().Truncate(time.Microsecond)
	mint := func(tenant shared.ID, fill byte) integration.SatisfactionLinkToken {
		token, err := integration.NewSatisfactionLinkToken(tenant, bytes.Repeat([]byte{fill}, integration.SatisfactionLinkTokenSecretBytes))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	fill := byte(freshID(t).String()[35])
	first, second := mint(tenantA, fill), mint(tenantA, fill+1)
	var requesterID shared.ID
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := requesters.RecordContact(ctx, address, "Kaya", item, now); err != nil {
			return err
		}
		on, err := requesters.AddressesOn(ctx, item)
		if err != nil || len(on) != 1 || on[0].ID.IsZero() {
			t.Fatalf("the entry's requesters are %+v (%v)", on, err)
		}
		requesterID = on[0].ID
		link, err := servicedesk.NewSatisfactionLink(tenantA, requesterID, item, now)
		if err != nil {
			return err
		}
		if err := links.Put(ctx, link, first); err != nil {
			return err
		}
		found, err := links.FindByToken(ctx, first)
		if err != nil || found.RequesterID != requesterID || found.ItemID != item || found.UsedAt != nil ||
			!found.ExpiresAt.Equal(now.Add(servicedesk.SatisfactionLinkLife)) {
			t.Errorf("by token %+v, %v", found, err)
		}
		return links.Put(ctx, link, second)
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := links.FindByToken(ctx, second); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door found the link: %v", err)
		}
		if used, err := links.Use(ctx, second, now); err != nil || used {
			t.Errorf("the workspace next door answered the link: %v %v", used, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := links.FindByToken(ctx, first); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the replaced token still answers: %v", err)
		}
		if used, err := links.Use(ctx, second, now.Add(servicedesk.SatisfactionLinkLife)); err != nil || used {
			t.Errorf("an expired link was answered: %v %v", used, err)
		}
		if used, err := links.Use(ctx, second, now.Add(time.Hour)); err != nil || !used {
			t.Errorf("the link was not answered: %v %v", used, err)
		}
		if used, err := links.Use(ctx, second, now.Add(2*time.Hour)); err != nil || used {
			t.Errorf("the link was answered twice: %v %v", used, err)
		}
		if found, err := links.FindByToken(ctx, second); err != nil || found.UsedAt == nil {
			t.Errorf("the answer is not kept: %+v %v", found, err)
		}
		_, err := requesters.EraseAddress(ctx, address, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM satisfaction_link WHERE requester_id = $1`, requesterID.String()); n != 0 {
		t.Errorf("the link outlived the requester's erasure")
	}
}
