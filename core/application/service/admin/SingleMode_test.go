// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// In single mode the one workspace is the installation and holds every operator: suspending it or
// requesting its deletion would shut everybody out with no way back (milestone SC decision 3), so
// both are refused - before the typed name and the proof, so a refusal burns neither. Resume and the
// cancel stay open: they are the way back for a workspace that got there before.
func TestSingleModeRefusesToShutItsOnlyWorkspace(t *testing.T) {
	for _, tenancy := range []env.TenancyMode{env.TenancySingle, ""} {
		t.Run("tenancy "+string(tenancy), func(t *testing.T) {
			shift := newShiftFixture(domain.TenantActive)
			shift.shift.Tenancy = tenancy
			deletion := newDeletionFixture(domain.TenantActive)
			deletion.handler.Tenancy = tenancy
			registry, err := usecase.NewRegistry(nil,
				SuspendTenant{shift.shift}.Descriptor(), deletion.handler.Descriptor())
			if err != nil {
				t.Fatalf("building the registry: %v", err)
			}

			for name, in := range map[string]usecase.Input{
				SuspendTenantName: {"tenant_id": lifecycleTenant.String()},
				RequestTenantDeletionName: {
					"tenant_id": lifecycleTenant.String(), "confirmation": "Acme GmbH",
					"step_up_token": "hbt_sup_proof",
				},
			} {
				_, err := registry.Invoke(t.Context(), name, operator(), in)
				var refusal *shared.Error
				if !errors.As(err, &refusal) || !errors.Is(err, shared.ErrForbidden) ||
					refusal.DetailCode != "admin.multi_mode_required" {
					t.Errorf("%s answered %v, want admin.multi_mode_required", name, err)
				}
			}
			if len(shift.tenants.moved) != 0 || len(deletion.tenants.deletions) != 0 ||
				deletion.stepUp.consumed != 0 {
				t.Error("a refused act moved a workspace or spent the proof")
			}
		})
	}
}

func TestSingleModeKeepsTheWaysBack(t *testing.T) {
	shift := newShiftFixture(domain.TenantSuspended)
	shift.shift.Tenancy = env.TenancySingle
	if err := (ResumeTenant{shift.shift}).Execute(t.Context(), operator(), lifecycleTenant); err != nil {
		t.Errorf("resuming in single mode: %v", err)
	}

	cancel := newCancelFixture(domain.TenantActive)
	if _, err := cancel.cancel(t, operator()); err != nil {
		t.Errorf("cancelling in single mode: %v", err)
	}
}
