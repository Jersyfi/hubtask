// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-BAK-08 check 3, the holds half, against a real database: the operator places the holds of a
// rewound period again through the registry; the workspace reads them back as the application
// role, in force, with their own identifier, placer and moment; a second run changes nothing; the
// other workspace is untouched (backup-restore.md §8.5 step 5).
func TestTheOperatorPlacesTheHoldsOfARewoundPeriodAgain(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	recoveredTo := created.Add(-24 * time.Hour)
	placedAt := created.Add(2 * time.Hour)
	inForce, released := freshID(t), freshID(t)

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created.Add(72 * time.Hour))
	holds := lifecycle.Holds{
		Holds: holdRepo(), Audit: postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed)), UnitOfWork: unitOfWork,
		Clock: fixed, Text: text.Composing{},
	}
	registry, err := usecase.NewRegistry(nil, lifecycle.ReplaceLegalHolds{
		Holds: holds, Workspaces: postgres.NewAdminTenantRepository(),
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantB, AccountID: authorB, AccountName: "Operator",
		Scopes: []string{"admin:tenants"},
	}
	input := usecase.Input{
		"tenant_id":      tenantA.String(),
		"recovery_point": recoveredTo.Format(time.RFC3339),
		"holds": []any{
			map[string]any{
				"id": inForce.String(), "scope": map[string]any{"kind": "CONTAINER", "id": collection.String()},
				"reason": "Dispute with the supplier", "placed_by": authorA.String(),
				"placed_at": placedAt.Format(time.RFC3339Nano),
			},
			map[string]any{
				"id": released.String(), "scope": map[string]any{"kind": "TENANT"},
				"reason": "Inspection", "placed_by": authorA.String(),
				"placed_at":   placedAt.Format(time.RFC3339Nano),
				"released_at": placedAt.Add(time.Hour).Format(time.RFC3339Nano),
			},
		},
	}

	if _, err := registry.Invoke(ctx, lifecycle.ReplaceLegalHoldsName, operator, input); err != nil {
		t.Fatalf("placing again: %v", err)
	}
	var active domain.Holds
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		active, err = lifecycleRepo().Active(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the holds: %v", err)
	}
	for _, id := range []shared.ID{inForce, released} {
		if !containsHold(active, id) {
			t.Errorf("hold %s is not in force", id)
		}
	}
	var found domain.LegalHold
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = holdRepo().Find(ctx, inForce)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if found.PlacedBy != authorA || !found.PlacedAt.Equal(placedAt) || found.ScopeID != collection {
		t.Errorf("placed again as %+v", found)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = $2 AND target_id = $3`,
		tenantA.String(), string(lifecycle.HoldReplacedAction), inForce.String()); n != 1 {
		t.Errorf("%d replacement entries in the workspace's trail", n)
	}

	// A second run changes nothing, and the other workspace never had any of it.
	if _, err := registry.Invoke(ctx, lifecycle.ReplaceLegalHoldsName, operator, input); err != nil {
		t.Fatalf("the second run: %v", err)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM legal_hold WHERE id = ANY($1::uuid[])`,
		[]string{inForce.String(), released.String()}); n != 2 {
		t.Errorf("%d hold rows after two runs", n)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM legal_hold WHERE tenant_id = $1 AND id = ANY($2::uuid[])`,
		tenantB.String(), []string{inForce.String(), released.String()}); n != 0 {
		t.Errorf("%d of the holds landed in the other workspace", n)
	}
}
