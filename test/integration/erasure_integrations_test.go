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
// is not this workspace's to count. Every agent of theirs counts until it is retired: a drafted or
// suspended one can be put live again and would then act on behalf of an account that is gone.
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

	for _, status := range []string{"DRAFT", "SUSPENDED", "RETIRED"} {
		if _, err := admin.Exec(ctx, `INSERT INTO agent (id, tenant_id, account_id, name, purpose, kind, owner_id, risk_class,
				policy_reversible, policy_irreversible, tokens_per_day, actions_per_hour, concurrent_runs, status,
				created_by, created_at, updated_at)
			VALUES ($1, $2, $3, 'triage', 'sorts tickets', 'TICKET_TRIAGE', $4, 'MINIMAL', 'ALLOW', 'APPROVAL', 0, 0, 0, $5,
				$4, now(), now())`,
			freshID(t).String(), tenantA.String(), freshID(t).String(), person.String(), status); err != nil {
			t.Fatalf("seeding a %s agent: %v", status, err)
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
	if counted != (repository.IntegrationsAs{Hooks: 1, Agents: 2}) {
		t.Errorf("counted %+v, want the one switched-on hook and the two agents not retired of this workspace", counted)
	}
}
