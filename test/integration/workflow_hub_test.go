// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A hub's own workflow against the real boundary (UC-FLW-02 check 9, gate SG-3): it is stored
// with its hub and the key it was copied from, the workspace next door reads none of it, and the
// hub takes its workflows with it when it goes - its scheme rows included, in the same purge.

var (
	whTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe01")
	whTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe02")
	whAccount = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe03")
	whHub     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe04")
	whList    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe06")
)

func TestAHubsOwnWorkflowStaysWithItsHub(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	a, account := whTenantA.String(), whAccount.String()
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + a + `', 'wh-a', 'Hub workflows A') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + whTenantB.String() + `', 'wh-b', 'Hub workflows B') ON CONFLICT (id) DO NOTHING`,
		`DELETE FROM workflow_scheme WHERE tenant_id = '` + a + `'`,
		`DELETE FROM workflow WHERE tenant_id = '` + a + `'`,
		`DELETE FROM container WHERE tenant_id = '` + a + `' AND type = 'COLLECTION'`,
		`DELETE FROM container WHERE tenant_id = '` + a + `'`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		 VALUES ('` + account + `', '` + a + `', 'USER', 'hubworkflows@example.org', 'Hub workflows', 'ACTIVE')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		 VALUES ('` + whHub.String() + `', '` + a + `', 'HUB', 'Support', 'a0', '` + account + `')`,
		`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		 VALUES ('` + whList.String() + `', '` + a + `', 'COLLECTION', '` + whHub.String() + `', 'Desk', 'a0', '` + account + `')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	workflows := postgres.NewWorkflowRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Microsecond)
	shape := domain.WorkflowShape{
		Statuses: []domain.WorkflowStatus{
			{Key: "open", Name: "Open", Category: domain.CategoryTodo},
			{Key: "closed", Name: "Closed", Category: domain.CategoryDone},
		},
		Steps: []domain.WorkflowStep{{Key: "close", Name: "Close", To: "closed"}},
	}
	common, err := domain.NewWorkflow(domain.NewWorkflowInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe11"), TenantID: whTenantA,
		Key: "service", Name: "Service", Shape: shape, Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	copied, err := common.CopyAt(domain.WorkflowCopy{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe12"), HubID: whHub, Key: "support_service",
		Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	inTenant(t, uow, whTenantA, func(ctx context.Context) error {
		for _, w := range []domain.Workflow{common, copied} {
			if err := workflows.Insert(ctx, w); err != nil {
				t.Fatalf("storing %s: %v", w.Key, err)
			}
		}
		// The collection and the hub both run the hub's own workflow.
		for _, container := range []shared.ID{whList, whHub} {
			if err := workflows.SetScheme(ctx, domain.WorkflowScheme{ContainerID: container, WorkflowKey: "support_service"}, now); err != nil {
				t.Fatalf("scheme: %v", err)
			}
		}
		rows, err := workflows.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows {
			if row.Key == "support_service" {
				found = row.HubID == whHub && row.CopiedFrom == "service"
			}
		}
		if !found {
			t.Errorf("the copy did not keep its hub and source: %+v", rows)
		}
		return nil
	})

	// The workspace next door sees none of it.
	inTenant(t, uow, whTenantB, func(ctx context.Context) error {
		rows, err := workflows.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Errorf("B read A's workflows: %+v", rows)
		}
		schemes, err := workflows.Schemes(ctx)
		if err != nil || len(schemes) != 0 {
			t.Errorf("B read A's scheme: %+v, %v", schemes, err)
		}
		return nil
	})

	// A purge takes the collection first, then the hub with its own workflow and its scheme row
	// in one statement; the shared workflow stays.
	for _, statement := range []string{
		`DELETE FROM container WHERE tenant_id = '` + a + `' AND id = '` + whList.String() + `'`,
		`DELETE FROM container WHERE tenant_id = '` + a + `' AND id = '` + whHub.String() + `'`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("purging the hub: %v", err)
		}
	}
	inTenant(t, uow, whTenantA, func(ctx context.Context) error {
		rows, err := workflows.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].Key != "service" {
			t.Errorf("after the purge: %+v", rows)
		}
		return nil
	})
}
