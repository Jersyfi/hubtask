// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// A dry run answers what the move would change - what rests, the exceptions that stay, the would-be
// workspace - and writes no trail, no journal and no event; the transaction it ran in ends with a
// refusal of its own, so the plan's assignment does not commit either (the integration test reads
// that from the database).
func TestADryRunAnswersTheMoveAndRecordsNothing(t *testing.T) {
	f := newMoveFixture(t)
	recorder, log := recorderOn(f.tenants)
	f.registry = moveRegistry(t, f, recorder)
	journal := len(f.plans.journal.entries)
	in := moveTo("family", 4)
	in["dry_run"] = true

	out, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), in)
	if err != nil {
		t.Fatalf("the dry run: %v", err)
	}
	tenant, _ := out["tenant"].(usecase.Output)
	if tenant["plan"] != "family" {
		t.Errorf("the would-be workspace answered %v", tenant)
	}
	if resting, _ := out["features_resting"].([]any); len(resting) != 1 || resting[0] != "WEBHOOKS" {
		t.Errorf("features resting %v", out["features_resting"])
	}
	if kept, _ := out["exceptions_kept"].([]string); len(kept) != 1 || kept[0] != "items" {
		t.Errorf("exceptions kept %v", out["exceptions_kept"])
	}
	if len(f.audit.entries) != 0 || len(f.plans.journal.entries) != journal {
		t.Errorf("a dry run recorded %d trail and %d journal entries", len(f.audit.entries), len(f.plans.journal.entries)-journal)
	}
	if len(log.events) != 0 {
		t.Errorf("a dry run told the platform %d events", len(log.events))
	}
}

// A dry run to the plan the workspace is on answers it as it stands, as the move does.
func TestADryRunToThePlanItIsOnAnswersItAsItStands(t *testing.T) {
	f := newMoveFixture(t)
	if _, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("family", 4)); err != nil {
		t.Fatalf("moving: %v", err)
	}
	in := moveTo("family", 5)
	in["dry_run"] = true
	out, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), in)
	if err != nil {
		t.Fatalf("the dry run: %v", err)
	}
	if tenant, _ := out["tenant"].(usecase.Output); tenant["plan"] != "family" || tenant["version"] != 5 {
		t.Errorf("the workspace answered %v", tenant)
	}
	if len(f.audit.entries) != 1 {
		t.Errorf("trail %+v", f.audit.entries)
	}
}

// Inside a transaction somebody else opened, a dry run is refused before it reads anything: its
// rollback would not reach that transaction, and the caller's commit would keep the move.
func TestADryRunInsideAnotherTransactionIsRefused(t *testing.T) {
	f := newMoveFixture(t)
	ctx, held := persistence.HoldAfterwards(t.Context())
	defer held.Run(ctx)
	in := moveTo("family", 4)
	in["dry_run"] = true

	_, err := f.registry.Invoke(ctx, MoveTenantPlanName, operator(), in)
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("got %v, want an internal refusal", err)
	}
	if f.tenants.record.PlanName != "" || len(f.tenants.moved) != 0 {
		t.Errorf("the workspace moved: %+v", f.tenants.moved)
	}
}
