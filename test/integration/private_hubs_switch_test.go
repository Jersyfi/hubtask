// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// privateHubsRule is *Private hubs allowed* the way main.go resolves it: the identity rule's
// switch through the installation's and the workspace's levels.
func privateHubsRule(unitOfWork persistence.UnitOfWork) identityservice.SignInPolicyResolver {
	return identityservice.SignInPolicyResolver{
		Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Instance:   postgres.NewInstanceSettingRepository(),
		UnitOfWork: unitOfWork,
	}
}

// switchPrivateHubs writes the workspace's own value of the switch for the test and puts the
// previous rule back afterwards: the shared tenant's other tests must not meet it.
func switchPrivateHubs(ctx context.Context, t *testing.T, tenant shared.ID, allowed bool) {
	t.Helper()
	admin := adminPool(ctx, t)
	var previous *string
	if err := admin.QueryRow(ctx, `SELECT settings #>> '{sign_in_policy}' FROM tenant WHERE id = $1`,
		tenant.String()).Scan(&previous); err != nil {
		t.Fatalf("reading the rule: %v", err)
	}
	t.Cleanup(func() {
		restore := `UPDATE tenant SET settings = settings - 'sign_in_policy' WHERE id = $1`
		args := []any{tenant.String()}
		if previous != nil {
			restore = `UPDATE tenant SET settings = jsonb_set(settings, '{sign_in_policy}', $2::jsonb) WHERE id = $1`
			args = append(args, *previous)
		}
		_, _ = admin.Exec(context.WithoutCancel(ctx), restore, args...)
	})
	if _, err := admin.Exec(ctx, `
		UPDATE tenant SET settings = jsonb_set(settings, '{sign_in_policy}',
			coalesce(settings -> 'sign_in_policy', '{}'::jsonb) || jsonb_build_object('private_hubs', $2::boolean))
		WHERE id = $1`, tenant.String(), allowed); err != nil {
		t.Fatalf("writing the switch: %v", err)
	}
}

// UC-ID-16 check 7 against PostgreSQL: the workspace's "off", stored where the sign-in rule keeps
// it, stops a new private hub - and the rule reads it back through the same levels.
func TestPrivateHubsSwitchedOffStopsANewOne(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))

	allowed, err := privateHubsRule(unitOfWork).PrivateHubsAllowed(ctx, w.tenant)
	if err != nil || !allowed {
		t.Fatalf("by default: %v, %v; want allowed", allowed, err)
	}

	switchPrivateHubs(ctx, t, w.tenant, false)
	if allowed, err = privateHubsRule(unitOfWork).PrivateHubsAllowed(ctx, w.tenant); err != nil || allowed {
		t.Fatalf("switched off: %v, %v; want refused", allowed, err)
	}

	before := countIn(ctx, t, `SELECT count(*) FROM container WHERE tenant_id = $1 AND private`, w.tenant.String())
	_, err = catalogueFor(t).Invoke(ctx, "CreateContainer", administrator(w.tenant, w.member),
		usecase.Input{"type": "HUB", "name": freshName(t), "private": true})
	if !errors.Is(err, shared.ErrForbidden) || shared.AsError(err).DetailCode != "containers.private_hubs_off" {
		t.Fatalf("error %v, want containers.private_hubs_off", err)
	}
	if after := countIn(ctx, t, `SELECT count(*) FROM container WHERE tenant_id = $1 AND private`,
		w.tenant.String()); after != before {
		t.Errorf("%d private hubs after the refusal, want %d", after, before)
	}
	// The private hub already there stays private.
	if !storedContainer(ctx, t, w.tenant, w.hub).Private {
		t.Error("switching off made an existing private hub shared")
	}
}
