// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A hub's own SLA policy against the real boundary (UC-SVC-02 check 11, gate SG-3): it is stored
// with its hub and the key it was copied from, the workspace next door reads none of it, and the
// hub takes its policies with it when it goes.

var (
	shTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc01")
	shTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc02")
	shAccount = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc03")
	shHub     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc04")
)

func TestAHubsOwnSlaPolicyStaysWithItsHub(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	a, account := shTenantA.String(), shAccount.String()
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + a + `', 'sh-a', 'Hub policies A') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + shTenantB.String() + `', 'sh-b', 'Hub policies B') ON CONFLICT (id) DO NOTHING`,
		`DELETE FROM sla_policy WHERE tenant_id = '` + a + `'`,
		`DELETE FROM container WHERE tenant_id = '` + a + `' AND type = 'COLLECTION'`,
		`DELETE FROM container WHERE tenant_id = '` + a + `'`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		 VALUES ('` + account + `', '` + a + `', 'USER', 'hubpolicies@example.org', 'Hub policies', 'ACTIVE')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		 VALUES ('` + shHub.String() + `', '` + a + `', 'HUB', 'Desk', 'a0', '` + account + `')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	policies := postgres.ServiceDeskRepository{}.Policies()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Microsecond)
	common, err := servicedesk.NewSLAPolicy(servicedesk.NewSLAPolicyInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc11"), TenantID: shTenantA, Key: "incidents",
		Name: "Incidents", EntryTypes: []string{"incident"},
		Goals:   []servicedesk.SLAGoal{{Kind: servicedesk.GoalResolution, Minutes: 480}},
		Enabled: true, Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	copied, err := common.CopyAt(servicedesk.SLAPolicyCopy{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fc12"), HubID: shHub, Key: "desk_incidents",
		Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	inTenant(t, uow, shTenantA, func(ctx context.Context) error {
		for _, p := range []servicedesk.SLAPolicy{common, copied} {
			if err := policies.Insert(ctx, p); err != nil {
				t.Fatalf("storing %s: %v", p.Key, err)
			}
		}
		read, err := policies.Find(ctx, "desk_incidents")
		if err != nil || read.HubID != shHub || read.CopiedFrom != "incidents" {
			t.Errorf("read back %+v, %v", read, err)
		}
		return nil
	})
	inTenant(t, uow, shTenantB, func(ctx context.Context) error {
		if _, err := policies.Find(ctx, "desk_incidents"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("B found A's hub policy: %v", err)
		}
		if listed, err := policies.List(ctx); err != nil || len(listed) != 0 {
			t.Errorf("B listed %+v, %v", listed, err)
		}
		return nil
	})
	if _, err := admin.Exec(ctx, `DELETE FROM container WHERE tenant_id = '`+a+`' AND id = '`+shHub.String()+`'`); err != nil {
		t.Fatalf("purging the hub: %v", err)
	}
	inTenant(t, uow, shTenantA, func(ctx context.Context) error {
		listed, err := policies.List(ctx)
		if err != nil || len(listed) != 1 || listed[0].Key != "incidents" {
			t.Errorf("after the purge: %+v, %v", listed, err)
		}
		return nil
	})
}
