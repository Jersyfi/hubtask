// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/service/notification"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// ADR-0078 §3 against the real database: an operator opens the password for one workspace
// whose own provider is switched on - the broken provider the opening is for - and the real resolver
// reads it from the tenant row: the door opens with the cause OPERATOR, the card offers the password,
// a sign-in through it lands in the trail, the act lands in the workspace's trail and in the journal,
// and the workspace beside it is untouched. Gate SG-3: both new repository methods are asked under
// the other workspace's scope and reach nothing of this one.
var (
	openingTenant    = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000034a1")
	openingBystander = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000034a2")
	openingAccount   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000034b1")
	openingProvider  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000034c1")
)

// provenStepUp stands for a step-up the operator passed: the proof itself is the step-up's own,
// tested at length elsewhere; what this test is about is what the opening does once it is let
// through.
type provenStepUp struct{}

func (provenStepUp) Available() bool { return true }
func (provenStepUp) Satisfied(context.Context, shared.ID, string) (bool, error) {
	return true, nil
}

func (provenStepUp) Methods(context.Context, shared.ID, shared.ID) ([]stepup.Method, error) {
	return nil, nil
}

func TestAnOperatorsOpeningIsReadByTheRealResolver(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	session, signIn, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)

	hash, err := session.Passwords.Hash(secret.New(fallbackPassphrase))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	// Two workspaces of this test's own: one that signs in only through its own provider, and one
	// beside it that nothing here may touch.
	for _, seed := range []struct {
		id         shared.ID
		slug, name string
	}{
		{openingTenant, "operator-opening", "Operator Opening"},
		{openingBystander, "opening-bystander", "Opening Bystander"},
	} {
		if _, err := admin.Exec(ctx, `
			INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone, settings)
			VALUES ($1, $2, $3, 'en', 'UTC', '{"sign_in_policy": {"methods": ["OIDC"]}}'::jsonb)
			ON CONFLICT (id) DO UPDATE SET settings = EXCLUDED.settings, status = 'ACTIVE',
			  password_opened_until = NULL, password_opened_requester = NULL,
			  password_opened_reason = NULL`,
			seed.id.String(), seed.slug, seed.name); err != nil {
			t.Fatalf("seeding %s: %v", seed.slug, err)
		}
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status)
		VALUES ($1, $2, 'USER', 'opening@example.org', 'Opening', $3, 'ACTIVE')
		ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		openingAccount.String(), openingTenant.String(), hash); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DELETE FROM identity_provider WHERE id = $1`,
			openingProvider.String())
		_, _ = admin.Exec(context.Background(), `
			UPDATE tenant SET password_opened_until = NULL, password_opened_requester = NULL,
			  password_opened_reason = NULL
			WHERE id IN ($1, $2)`, openingTenant.String(), openingBystander.String())
	})
	providers := postgres.NewIdentityProviderRepository()
	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: openingProvider, TenantID: openingTenant, Issuer: "https://id.operator-opening.example",
		ClientID: "hubtask-operator-opening", Enabled: true, Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}
	inTenant(t, uow, openingTenant, func(ctx context.Context) error {
		_, err := providers.Insert(ctx, configured, cryptoport.Sealed{KeyID: "k-opening", Ciphertext: []byte("s")})
		return err
	})

	workspaces := postgres.NewWorkspaceSettingsRepository()
	resolver := identityservice.SignInPolicyResolver{
		Workspaces: workspaces, Instance: postgres.NewInstanceSettingRepository(), UnitOfWork: uow,
	}
	passwords := identityservice.PasswordWriter{
		Session: session, Resolver: resolver,
		Accounts: postgres.NewPasswordRepository(), Histories: postgres.NewPasswordRepository(),
		WaysIn: identityservice.WaysIn{
			Providers: providers, Workspaces: workspaces, UnitOfWork: uow, Clock: clockadapter.System{},
		},
		Text: text.Composing{}, UnitOfWork: uow, Clock: clockadapter.System{}, IDs: session.IDs,
	}
	session.Rule = passwords
	card := identityservice.GetSignInRules{
		Resolver: resolver, Tenants: signIn, Providers: providers,
		Workspaces: workspaces, Clock: clockadapter.System{}, UnitOfWork: uow, Multi: true,
	}
	door := func(tenant shared.ID) (bool, identityservice.FallbackCause) {
		t.Helper()
		open, cause, err := passwords.PasswordOpen(ctx, tenant)
		if err != nil {
			t.Fatalf("asking the door: %v", err)
		}
		return open, cause
	}
	signsIn := func() error {
		t.Helper()
		_, err := identityservice.SignIn{Writer: session}.Execute(ctx, identityservice.SignInCommand{
			Email: "opening@example.org", Password: secret.New(fallbackPassphrase),
			TenantHeader: openingTenant.String(), RemoteAddr: "198.51.100.34",
		})
		return err
	}

	// The provider is on: the password is shut, as the workspace decided.
	if open, cause := door(openingTenant); open || cause.Opens() {
		t.Fatalf("before the opening the door answered open %v, cause %q", open, cause)
	}

	tenants := postgres.NewAdminTenantRepository()
	writer := adminservice.PasswordOpeningWriter{
		Instance: adminservice.InstanceWriter{UnitOfWork: uow}, Tenants: tenants,
		Journal: postgres.NewInstanceJournal(pageCursors()), Audit: postgres.NewAuditSink(generator{t}),
		StepUp: provenStepUp{}, UnitOfWork: uow, Clock: clockadapter.System{}, IDs: session.IDs,
		Text: text.Composing{},
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: openingBystander, AccountID: openingAccount,
		AccountName: "Root Operator", Scopes: []string{"admin:tenants"},
	}
	opened, err := adminservice.OpenTenantPassword{Writer: writer}.Execute(ctx, operator,
		adminservice.OpenTenantPasswordCommand{
			TenantID: openingTenant, Hours: 2, Requester: "TICKET-4711",
			Reason: "the directory answers 500", StepUpToken: "proven",
		})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if until := opened.PasswordOpening.Until; until.Before(time.Now().Add(110*time.Minute)) ||
		until.After(time.Now().Add(2*time.Hour)) {
		t.Errorf("a two-hour opening ends %v", until)
	}

	// The real resolver reads it: the door, the card and the sign-in, with its cause in the trail.
	if open, cause := door(openingTenant); !open || cause != identityservice.FallbackCauseOperator {
		t.Errorf("under the opening the door answered open %v, cause %q", open, cause)
	}
	rules, err := card.Execute(ctx, identityservice.GetSignInRulesCommand{TenantHeader: openingTenant.String()})
	if err != nil {
		t.Fatalf("reading the card: %v", err)
	}
	if !rules.PasswordFallback || !slices.Contains(rules.Methods, domain.MethodDirect) {
		t.Errorf("under the opening the card offers %v (fallback %v)", rules.Methods, rules.PasswordFallback)
	}
	if err := signsIn(); err != nil {
		t.Fatalf("signing in under the opening: %v", err)
	}
	counted := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := admin.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatalf("counting: %v", err)
		}
		return n
	}
	if counted(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'auth.password_fallback'
		AND actor_id = $2 AND changes -> 'cause' ->> 'to' = 'OPERATOR'`,
		openingTenant.String(), openingAccount.String()) == 0 {
		t.Error("the sign-in under the opening is not in the trail with the cause OPERATOR")
	}
	// The act, in the workspace's trail and in the installation's journal, with who asked and why.
	if counted(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.password_opened'
		AND changes -> 'requester' ->> 'to' = 'TICKET-4711'`, openingTenant.String()) == 0 {
		t.Error("the opening is not in the workspace's trail")
	}
	if counted(`SELECT count(*) FROM instance_event WHERE tenant_id = $1 AND action = 'tenant.password_opened'
		AND details ->> 'requester_present' = 'true' AND details ->> 'reason_present' = 'true'`,
		openingTenant.String()) == 0 {
		t.Error("the opening is not in the installation's journal")
	}
	// The journal is permanent and outlives the workspace: it holds neither text.
	if counted(`SELECT count(*) FROM instance_event WHERE tenant_id = $1
		AND (details::text LIKE '%TICKET-4711%' OR details::text LIKE '%directory answers%')`,
		openingTenant.String()) != 0 {
		t.Error("the installation's journal holds who asked or why")
	}

	// The workspace beside it: no opening - it has no way in of its own, so its password is open as the
	// fallback for that reason and no other - and the listing says which workspace has one.
	if _, cause := door(openingBystander); cause == identityservice.FallbackCauseOperator {
		t.Error("the opening reached the workspace beside it")
	}
	listed, err := adminservice.ListTenants{Tenants: tenants, UnitOfWork: uow, Clock: clockadapter.System{}}.
		Execute(ctx, operator)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	for _, record := range listed {
		switch record.ID {
		case openingTenant:
			if record.PasswordOpening.Requester != "TICKET-4711" {
				t.Errorf("the listing answers the opening as %+v", record.PasswordOpening)
			}
		case openingBystander:
			if !record.PasswordOpening.Until.IsZero() {
				t.Errorf("the listing answers an opening for the workspace beside it: %+v", record.PasswordOpening)
			}
		}
	}

	// Closed early: the workspace's rule is the rule again, and the close is in both records.
	if _, err := (adminservice.CloseTenantPassword{Writer: writer}).Execute(ctx, operator, openingTenant); err != nil {
		t.Fatalf("closing: %v", err)
	}
	if open, _ := door(openingTenant); open {
		t.Error("the closed opening left the password open")
	}
	if err := signsIn(); shared.AsError(err).DetailCode != "auth.password_not_offered" {
		t.Errorf("after the close the password answered %v", err)
	}
	if counted(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.password_closed'
		AND changes -> 'ended' ->> 'to' = 'OPERATOR'`, openingTenant.String()) == 0 {
		t.Error("the close is not in the workspace's trail")
	}
	if counted(`SELECT count(*) FROM instance_event WHERE tenant_id = $1 AND action = 'tenant.password_closed'`,
		openingTenant.String()) == 0 {
		t.Error("the close is not in the installation's journal")
	}
}

// Gate SG-3 for the two new statements: asked under one workspace's scope, they write that workspace's
// row and no other - an opening of the bystander leaves the tenant untouched, and a close under the
// bystander's scope does not end the tenant's opening. And a close at a moment before the end leaves
// an opening standing, which is what keeps the job of an earlier opening from ending a later one.
func TestTheOpeningsStatementsStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	_, _, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)
	for _, seed := range []struct {
		id   shared.ID
		slug string
	}{{openingTenant, "operator-opening"}, {openingBystander, "opening-bystander"}} {
		if _, err := admin.Exec(ctx, `
			INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, $2)
			ON CONFLICT (id) DO UPDATE SET status = 'ACTIVE', password_opened_until = NULL,
			  password_opened_requester = NULL, password_opened_reason = NULL`,
			seed.id.String(), seed.slug); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `
			UPDATE tenant SET password_opened_until = NULL, password_opened_requester = NULL,
			  password_opened_reason = NULL
			WHERE id IN ($1, $2)`, openingTenant.String(), openingBystander.String())
	})
	tenants := postgres.NewAdminTenantRepository()
	until := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	opening := domain.PasswordOpening{Until: until, Requester: "TICKET-4712", Reason: "down"}
	opened := func(tenant shared.ID) adminrepo.TenantRecord {
		t.Helper()
		var record adminrepo.TenantRecord
		inTenant(t, uow, tenant, func(ctx context.Context) error {
			found, err := tenants.Find(ctx)
			record = found
			return err
		})
		return record
	}

	inTenant(t, uow, openingTenant, func(ctx context.Context) error {
		moved, err := tenants.OpenPassword(ctx, opening, time.Now())
		if !moved {
			t.Error("the opening wrote nothing in its own workspace")
		}
		return err
	})
	if got := opened(openingTenant).PasswordOpening; !got.Until.Equal(until) || got.Requester != "TICKET-4712" {
		t.Errorf("the opening reads back as %+v", got)
	}
	if got := opened(openingBystander).PasswordOpening; !got.Until.IsZero() {
		t.Errorf("the opening reached the workspace beside it: %+v", got)
	}

	// A close under the bystander's scope ends nothing of the tenant's.
	inTenant(t, uow, openingBystander, func(ctx context.Context) error {
		closed, err := tenants.ClosePassword(ctx, time.Time{}, time.Now())
		if closed {
			t.Error("a close under the workspace beside it closed something")
		}
		return err
	})
	// A close at a moment before the end leaves it standing; one at the end ends it.
	inTenant(t, uow, openingTenant, func(ctx context.Context) error {
		closed, err := tenants.ClosePassword(ctx, until.Add(-time.Minute), time.Now())
		if closed {
			t.Error("a close before the end ended the opening")
		}
		return err
	})
	if opened(openingTenant).PasswordOpening.Until.IsZero() {
		t.Fatal("the opening went before its end")
	}
	inTenant(t, uow, openingTenant, func(ctx context.Context) error {
		closed, err := tenants.ClosePassword(ctx, until, time.Now())
		if !closed {
			t.Error("a close at the end left the opening")
		}
		return err
	})
	if got := opened(openingTenant).PasswordOpening; !got.Until.IsZero() || got.Requester != "" {
		t.Errorf("the closed opening reads back as %+v", got)
	}

	// Only an active workspace opens: a suspended one, and one that is leaving, open nothing.
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `UPDATE tenant SET status = 'ACTIVE' WHERE id = $1`,
			openingBystander.String())
	})
	for _, status := range []string{"SUSPENDED", "PENDING_DELETION"} {
		if _, err := admin.Exec(ctx, `UPDATE tenant SET status = $2 WHERE id = $1`,
			openingBystander.String(), status); err != nil {
			t.Fatalf("marking the bystander %s: %v", status, err)
		}
		inTenant(t, uow, openingBystander, func(ctx context.Context) error {
			moved, err := tenants.OpenPassword(ctx, opening, time.Now())
			if moved {
				t.Errorf("a %s workspace was opened", status)
			}
			return err
		})
	}
}

// The end, once its time has passed, against PostgreSQL: the job the opening seeded runs in the
// workspace's own transaction - the runner's, here the test's - clears the row, writes the trail and
// the journal, and queues the notice for the workspace's administrator, whom the real role matrix
// names. Only a real transaction shows that every one of those writes has one to go into.
func TestTheEndOfAnOpeningIsRecordedInTheWorkspacesTransaction(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	session, _, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)

	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'operator-opening', 'Operator Opening')
		ON CONFLICT (id) DO UPDATE SET status = 'ACTIVE'`, openingTenant.String()); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'opening@example.org', 'Opening', 'ACTIVE')
		ON CONFLICT (id) DO NOTHING`, openingAccount.String(), openingTenant.String()); err != nil {
		t.Fatalf("seeding the administrator: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		VALUES ('01936f2a-7c1e-7000-8000-0000000034d1', $1, $2, 'TENANT', 'OWNER')
		ON CONFLICT (id) DO NOTHING`, openingTenant.String(), openingAccount.String()); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}
	// An opening whose time ran out a minute ago, as the row holds it until the job comes.
	if _, err := admin.Exec(ctx, `
		UPDATE tenant SET password_opened_until = now() - interval '1 minute',
		  password_opened_requester = 'TICKET-4713', password_opened_reason = 'down'
		WHERE id = $1`, openingTenant.String()); err != nil {
		t.Fatalf("seeding the opening: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `
			UPDATE tenant SET password_opened_until = NULL, password_opened_requester = NULL,
			  password_opened_reason = NULL WHERE id = $1`, openingTenant.String())
		_, _ = admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1 AND kind = $2`,
			openingTenant.String(), "notification.password_opening")
	})

	jobs := postgres.NewQueue(session.IDs, clockadapter.System{})
	writer := adminservice.PasswordOpeningWriter{
		Tenants: postgres.NewAdminTenantRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
		Audit: postgres.NewAuditSink(generator{t}), Jobs: jobs,
		Notices: notification.RecordPasswordOpening{
			Memberships: postgres.NewMembershipRepository(), Jobs: jobs,
		},
		UnitOfWork: uow, Clock: clockadapter.System{}, IDs: session.IDs,
	}
	var again time.Duration
	inTenant(t, uow, openingTenant, func(ctx context.Context) error {
		waited, err := adminservice.EndPasswordOpening{Writer: writer}.Execute(ctx, openingTenant)
		again = waited
		return err
	})
	if again != 0 {
		t.Errorf("an opening past its end asked to come back in %v", again)
	}

	counted := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := admin.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatalf("counting: %v", err)
		}
		return n
	}
	if counted(`SELECT count(*) FROM tenant WHERE id = $1 AND password_opened_until IS NULL
		AND password_opened_requester IS NULL`, openingTenant.String()) != 1 {
		t.Error("the row still holds the ended opening")
	}
	if counted(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'tenant.password_closed'
		AND actor_type = 'SYSTEM' AND changes -> 'ended' ->> 'to' = 'EXPIRED'`, openingTenant.String()) == 0 {
		t.Error("the end is not in the workspace's trail")
	}
	if counted(`SELECT count(*) FROM instance_event WHERE tenant_id = $1 AND action = 'tenant.password_closed'
		AND details ->> 'ended' = 'EXPIRED'`, openingTenant.String()) == 0 {
		t.Error("the end is not in the installation's journal")
	}
	if counted(`SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'notification.password_opening'
		AND payload ->> 'account_id' = $2 AND payload ->> 'event' = 'CLOSED'`,
		openingTenant.String(), openingAccount.String()) != 1 {
		t.Error("the workspace's administrator was not queued the notice of the end")
	}
}
