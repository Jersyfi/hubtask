// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The integrations that act as a person, counted before a full deletion: a switched-on forge hook
// counts, a switched-off one does not, and another workspace's hook acting as the same identifier
// is not this workspace's to count.
func TestTheIntegrationsActingAsAPersonAreCountedInTheirWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	person := seedServiceAccount(ctx, t, tenantA)
	hubA, _ := seedHubWithCollection(ctx, t, tenantA, person)
	hubB, _ := seedHubWithCollection(ctx, t, tenantB, person)
	admin := adminPool(ctx, t)
	for _, hook := range []struct {
		tenant, hub string
		enabled     bool
	}{
		{tenantA.String(), hubA.ID.String(), true},
		{tenantA.String(), hubA.ID.String(), false},
		{tenantB.String(), hubB.ID.String(), true},
	} {
		if _, err := admin.Exec(ctx, `INSERT INTO code_hook (id, tenant_id, hub_id, kind, name, enabled, salt, run_as, created_at, updated_at)
			VALUES ($1, $2, $3, 'GITHUB', 'repo', $4, 'abcdefghijklmnopqrstuvwx', $5, now(), now())`,
			freshID(t).String(), hook.tenant, hook.hub, hook.enabled, person.String()); err != nil {
			t.Fatalf("seeding a hook: %v", err)
		}
	}

	var counted repository.IntegrationsAs
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		counted, err = postgres.NewPrivacyRepository(pageCursors()).IntegrationsRunningAs(ctx, person)
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if counted != (repository.IntegrationsAs{Hooks: 1}) {
		t.Errorf("counted %+v, want the one switched-on hook of this workspace", counted)
	}
}
