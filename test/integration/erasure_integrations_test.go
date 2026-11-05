// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

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
	personB := seedServiceAccount(ctx, t, tenantB)
	hubA, _ := seedHubWithCollection(ctx, t, tenantA, person)
	hubB, _ := seedHubWithCollection(ctx, t, tenantB, personB)
	admin := adminPool(ctx, t)
	for _, hook := range []struct {
		tenant, hub, runAs string
		enabled            bool
	}{
		{tenantA.String(), hubA.ID.String(), person.String(), true},
		{tenantA.String(), hubA.ID.String(), person.String(), false},
		{tenantB.String(), hubB.ID.String(), personB.String(), true},
	} {
		if _, err := admin.Exec(ctx, `INSERT INTO code_hook (id, tenant_id, hub_id, kind, name, enabled, salt, run_as, created_at, updated_at)
			VALUES ($1, $2, $3, 'GITHUB', 'repo', $4, 'abcdefghijklmnopqrstuvwx', $5, now(), now())`,
			freshID(t).String(), hook.tenant, hook.hub, hook.enabled, hook.runAs); err != nil {
			t.Fatalf("seeding a hook: %v", err)
		}
	}
	// The key carries the tenant (0369): a hook of B cannot name A's person at all.
	if _, err := admin.Exec(ctx, `INSERT INTO code_hook (id, tenant_id, hub_id, kind, name, enabled, salt, run_as, created_at, updated_at)
		VALUES ($1, $2, $3, 'GITHUB', 'repo', true, 'abcdefghijklmnopqrstuvwx', $4, now(), now())`,
		freshID(t).String(), tenantB.String(), hubB.ID.String(), person.String()); err == nil {
		t.Error("a hook of another workspace named this workspace's person")
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

// Before a full deletion the switched-off connectors and hooks and the retired agents move to
// another owner and stay off (UC-PRV-01 check 12); another workspace's are untouched, and while
// one still names the person the account row cannot go (0369).
func TestSwitchedOffIntegrationsMoveToAnotherOwnerInTheirWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	person := seedServiceAccount(ctx, t, tenantA)
	heir := seedServiceAccount(ctx, t, tenantA)
	personB := seedServiceAccount(ctx, t, tenantB)
	hubA, _ := seedHubWithCollection(ctx, t, tenantA, person)
	hubB, _ := seedHubWithCollection(ctx, t, tenantB, personB)
	admin := adminPool(ctx, t)
	hook := func(tenant, hub, runAs string, enabled bool) string {
		id := freshID(t).String()
		if _, err := admin.Exec(ctx, `INSERT INTO code_hook (id, tenant_id, hub_id, kind, name, enabled, salt, run_as, created_at, updated_at)
			VALUES ($1, $2, $3, 'GITHUB', 'repo', $4, 'abcdefghijklmnopqrstuvwx', $5, now(), now())`,
			id, tenant, hub, enabled, runAs); err != nil {
			t.Fatalf("seeding a hook: %v", err)
		}
		return id
	}
	off := hook(tenantA.String(), hubA.ID.String(), person.String(), false)
	on := hook(tenantA.String(), hubA.ID.String(), person.String(), true)
	other := hook(tenantB.String(), hubB.ID.String(), personB.String(), false)
	if _, err := admin.Exec(ctx, `INSERT INTO agent (id, tenant_id, account_id, name, purpose, kind, owner_id, risk_class,
			policy_reversible, policy_irreversible, tokens_per_day, actions_per_hour, concurrent_runs, status,
			created_by, created_at, updated_at)
		VALUES ($1, $2, $3, 'triage', 'sorts tickets', 'TICKET_TRIAGE', $4, 'MINIMAL', 'ALLOW', 'APPROVAL', 0, 0, 0, 'RETIRED',
			$4, now(), now())`,
		freshID(t).String(), tenantA.String(), freshID(t).String(), person.String()); err != nil {
		t.Fatalf("seeding an agent: %v", err)
	}

	store := postgres.NewPrivacyRepository(pageCursors())
	var moved, left repository.IntegrationsAs
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if moved, err = store.ReownSwitchedOffIntegrations(ctx, person, heir, time.Now()); err != nil {
			return err
		}
		left, err = store.IntegrationsNaming(ctx, person)
		return err
	}); err != nil {
		t.Fatalf("moving: %v", err)
	}
	if moved != (repository.IntegrationsAs{Hooks: 1, Agents: 1}) || left != (repository.IntegrationsAs{Hooks: 1}) {
		t.Errorf("moved %+v, left %+v; want the switched-off hook and the retired agent moved, the switched-on hook left", moved, left)
	}
	runAs := func(id string) (string, bool) {
		var account string
		var enabled bool
		if err := admin.QueryRow(ctx, `SELECT run_as::text, enabled FROM code_hook WHERE id = $1`, id).Scan(&account, &enabled); err != nil {
			t.Fatalf("reading a hook: %v", err)
		}
		return account, enabled
	}
	if account, enabled := runAs(off); account != heir.String() || enabled {
		t.Errorf("the switched-off hook runs as %s, switched on %v", account, enabled)
	}
	if account, _ := runAs(on); account != person.String() {
		t.Errorf("the switched-on hook was moved to %s", account)
	}
	if account, _ := runAs(other); account != personB.String() {
		t.Errorf("another workspace's hook was moved to %s", account)
	}
	if _, err := admin.Exec(ctx, `DELETE FROM account WHERE id = $1`, person.String()); err == nil ||
		!strings.Contains(err.Error(), "code_hook_run_as_fkey") {
		t.Errorf("deleting the account while a switched-on hook names it answered %v", err)
	}
}
