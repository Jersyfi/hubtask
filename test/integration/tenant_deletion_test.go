// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// multi-tenancy.md §5 against the real database, with the real step-up: an operator signed in to
// one workspace requests the deletion of another. The proof lives in the operator's own workspace
// and the deletion is written in the target's; a nested scope may not switch tenant, so the proof
// has to be judged before the target's transaction opens - which only the real unit of work and
// the real verifier can show.
var (
	deletionOperator = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000115a1")
	deletionSession  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000115b1")
	deletionTarget   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000115c1")
)

func TestAnOperatorRequestsTheDeletionOfAnotherWorkspace(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	writer, signIn, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)

	hash, err := writer.Passwords.Hash(secret.New("the operator's passphrase"))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status)
		VALUES ($1, $2, 'USER', 'deletion-operator@example.org', 'Operator', $3, 'ACTIVE')
		ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		deletionOperator.String(), tenantA.String(), hash); err != nil {
		t.Fatalf("seeding the operator: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		VALUES ($1, $2, $3, now(), now() + interval '30 days')
		ON CONFLICT (id) DO UPDATE SET revoked_at = NULL`,
		deletionSession.String(), tenantA.String(), deletionOperator.String()); err != nil {
		t.Fatalf("seeding the session: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name)
		VALUES ($1, 'deletion-target', 'Deletion Target')
		ON CONFLICT (id) DO UPDATE SET status = 'ACTIVE', purge_after = NULL`,
		deletionTarget.String()); err != nil {
		t.Fatalf("seeding the target: %v", err)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		return signIn.Clear(ctx, "stepup:"+deletionOperator.String())
	})

	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: deletionOperator,
		TokenID: deletionSession, AccountName: "Root Operator", Scopes: []string{"admin:tenants"},
	}
	grant, err := identityservice.StepUp{Writer: writer}.Execute(ctx, operator,
		identityservice.StepUpCommand{Password: secret.New("the operator's passphrase")})
	if err != nil {
		t.Fatalf("stepping up: %v", err)
	}

	deletion := adminservice.RequestTenantDeletion{
		Tenants: postgres.NewAdminTenantRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
		Automations: postgres.NewAutomationSwitch(), Jobs: postgres.NewQueue(writer.IDs, clockadapter.System{}),
		Holds:  postgres.NewLifecycleRepository(),
		StepUp: identityservice.StepUpVerifier{Writer: writer},
		Audit:  postgres.NewAuditSink(generator{t}), UnitOfWork: uow,
		Clock: clockadapter.System{}, IDs: writer.IDs,
	}

	// A mistyped name is refused before the proof is judged, so the operator keeps it.
	_, err = deletion.Execute(ctx, operator, adminservice.RequestTenantDeletionCommand{
		TenantID: deletionTarget, Confirmation: "Deletion Targte", StepUpToken: grant.Token.Reveal(),
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a mistyped name answered %v", err)
	}

	scheduled, err := deletion.Execute(ctx, operator, adminservice.RequestTenantDeletionCommand{
		TenantID: deletionTarget, Confirmation: "Deletion Target", StepUpToken: grant.Token.Reveal(),
	})
	if err != nil {
		t.Fatalf("requesting the deletion of another workspace: %v", err)
	}
	if scheduled.TenantID != deletionTarget || scheduled.PurgeAfter.IsZero() {
		t.Errorf("the deletion answered %+v", scheduled)
	}

	var status string
	if err := admin.QueryRow(ctx, `SELECT status FROM tenant WHERE id = $1`,
		deletionTarget.String()).Scan(&status); err != nil || status != "PENDING_DELETION" {
		t.Errorf("the target stands at %q (%v)", status, err)
	}
	var trail, journal, jobs int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1
		AND action = 'tenant.deletion_requested' AND actor_id = $2`,
		deletionTarget.String(), deletionOperator.String()).Scan(&trail); err != nil || trail == 0 {
		t.Errorf("the target's trail holds %d requests (%v)", trail, err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM instance_event WHERE tenant_id = $1
		AND action = 'tenant.deletion_requested'`, deletionTarget.String()).Scan(&journal); err != nil || journal == 0 {
		t.Errorf("the journal holds %d requests (%v)", journal, err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE tenant_id = $1
		AND kind = 'tenant.hard_delete' AND state = 'PENDING'`, deletionTarget.String()).Scan(&jobs); err != nil || jobs != 1 {
		t.Errorf("the target has %d hard delete jobs (%v)", jobs, err)
	}
	var operatorStatus string
	if err := admin.QueryRow(ctx, `SELECT status FROM tenant WHERE id = $1`,
		tenantA.String()).Scan(&operatorStatus); err != nil || operatorStatus != "ACTIVE" {
		t.Errorf("the operator's own workspace stands at %q (%v)", operatorStatus, err)
	}
}
