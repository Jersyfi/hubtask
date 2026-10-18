// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	recurrenceadapter "github.com/Jersyfi/hubtask/infrastructure/recurrence"
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
		Clock: clockadapter.System{}, IDs: writer.IDs, Tenancy: env.TenancyMulti,
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

// refusingStepUp proves nothing: the platform's path must never need it.
type refusingStepUp struct{}

func (refusingStepUp) Available() bool { return true }
func (refusingStepUp) Methods(context.Context, shared.ID, shared.ID) ([]stepup.Method, error) {
	return []stepup.Method{stepup.MethodPassword}, nil
}
func (refusingStepUp) Satisfied(context.Context, shared.ID, string) (bool, error) { return false, nil }

// UC-INS-08 check 7 against the real register and the real stores, in a database of its own (the
// register is the installation's, and the suite's shared one is other tests' state): a purchase
// platform's service account the register names requests the deletion with the typed name and no
// proof, and gets §5's whole phase; a workspace under a legal hold is refused it; the operator calls
// the platform's request off. The person in the same register is still asked for a proof.
func TestAPlatformRequestsADeletionWithoutAStepUp(t *testing.T) {
	ctx := context.Background()
	admin, app := privateInstallation(ctx, t, "hubtask_platform_deletion")
	const (
		target = "019364de-0000-7000-8000-0000000136d0"
		held   = "019364de-0000-7000-8000-0000000136e0"
	)
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + target + `', 'customer', 'A Customer'),
		   ('` + held + `', 'held', 'Held Customer')`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status) VALUES
		   ('019364de-0000-7000-8000-0000000136e1', '` + held + `', 'USER', 'h@held.example', 'H', 'ACTIVE')`,
		`INSERT INTO legal_hold (id, tenant_id, scope_kind, reason, placed_by, placed_at) VALUES
		   ('019364de-0000-7000-8000-0000000136e2', '` + held + `', 'TENANT', 'Pending litigation',
		    '019364de-0000-7000-8000-0000000136e1', now())`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	// The first addition to an empty register brings the owner who made it along.
	if got := enlist(ctx, t, app, rosterService, rosterOwner); got != "ADDED_WITH_CALLER" {
		t.Fatalf("enlisting the shop answered %q", got)
	}

	uow := postgres.NewUnitOfWork(app)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	deletion := adminservice.RequestTenantDeletion{
		Tenants: postgres.NewAdminTenantRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
		Automations: postgres.NewAutomationSwitch(), Jobs: postgres.NewQueue(ids, clockadapter.System{}),
		Holds: postgres.NewLifecycleRepository(), StepUp: refusingStepUp{},
		Register: postgres.NewOperatorRepository(),
		Audit:    postgres.NewAuditSink(generator{t}), UnitOfWork: uow,
		Clock: clockadapter.System{}, IDs: ids, Tenancy: env.TenancyMulti,
	}
	platform := appshared.ActorContext{
		Kind: appshared.ActorServiceAccount, TenantID: shared.ID(rosterTenant),
		AccountID: shared.ID(rosterService), AccountName: "The shop", Scopes: []string{"admin:provisioning"},
	}

	// The person in the same register, without a proof, is still refused.
	person := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.ID(rosterTenant), AccountID: shared.ID(rosterOwner),
		AccountName: "Jerome", Scopes: []string{"admin:tenants"},
	}
	_, err := deletion.Execute(ctx, person, adminservice.RequestTenantDeletionCommand{
		TenantID: shared.ID(target), Confirmation: "A Customer",
	})
	if shared.AsError(err) == nil || shared.AsError(err).DetailCode != stepup.CodeRequired {
		t.Fatalf("the person without a proof answered %v", err)
	}

	scheduled, err := deletion.Execute(ctx, platform, adminservice.RequestTenantDeletionCommand{
		TenantID: shared.ID(target), Confirmation: "A Customer",
	})
	if err != nil {
		t.Fatalf("the platform's request was refused: %v", err)
	}
	if grace := time.Until(scheduled.PurgeAfter); grace < 29*24*time.Hour || grace > 30*24*time.Hour {
		t.Errorf("the grace runs until %v, not 30 days", scheduled.PurgeAfter)
	}
	var status string
	var jobs, trail int
	if err := admin.QueryRow(ctx, `SELECT status FROM tenant WHERE id = $1`, target).Scan(&status); err != nil ||
		status != "PENDING_DELETION" {
		t.Errorf("the workspace stands at %q (%v)", status, err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'tenant.hard_delete'
		AND state = 'PENDING'`, target).Scan(&jobs); err != nil || jobs != 1 {
		t.Errorf("%d hard delete jobs (%v)", jobs, err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1
		AND action = 'tenant.deletion_requested' AND actor_id = $2`, target, rosterService).Scan(&trail); err != nil ||
		trail != 1 {
		t.Errorf("the trail names the platform %d times (%v)", trail, err)
	}

	// The legal-hold refusal stays.
	_, err = deletion.Execute(ctx, platform, adminservice.RequestTenantDeletionCommand{
		TenantID: shared.ID(held), Confirmation: "Held Customer",
	})
	if shared.AsError(err) == nil || shared.AsError(err).DetailCode != lifecycle.CodeLegalHold {
		t.Errorf("a held workspace answered %v", err)
	}

	// And the operator calls the platform's request off.
	cancel := adminservice.CancelTenantDeletion{
		Tenants: postgres.NewAdminTenantRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
		Automations: automationservice.PausedRules{
			Paused: postgres.NewPausedAutomationRules(), Expander: recurrenceadapter.New(), Jobs: &wakes{},
		},
		Audit: postgres.NewAuditSink(generator{t}), UnitOfWork: uow, Clock: systemClock{}, IDs: generator{t},
	}
	back, err := cancel.Execute(ctx, person, shared.ID(target))
	if err != nil || back != identity.TenantActive {
		t.Fatalf("the cancel answered (%s, %v)", back, err)
	}
}
