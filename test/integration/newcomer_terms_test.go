// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strconv"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// A newcomer through a provider where the workspace has terms of use (UC-ID-19 check 1, migration
// 0142), against PostgreSQL as the application role: the arrival leaves no account, only a TERMS
// row carrying it; agreeing makes the account, connects the provider and records the agreement in
// one transaction; a second agreement of the same arrival signs that account in. Its own workspace,
// because terms in force change what every other arrival there meets.
var (
	newcomerTermsTenant   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000141c01")
	newcomerTermsProvider = shared.MustParseID("01936f2a-7c1e-7000-8000-000000141c21")
)

func TestANewcomerIsMadeOnlyByAgreeingToTheTerms(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'newcomer-terms', 'Newcomer terms')
		ON CONFLICT (id) DO NOTHING`, newcomerTermsTenant.String()); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		UPDATE tenant SET settings = settings || jsonb_build_object('sign_in_policy',
			jsonb_build_object('terms_url', 'https://newcomer.example/terms', 'terms_version', '3'))
		WHERE id = $1`, newcomerTermsTenant.String()); err != nil {
		t.Fatalf("setting the terms: %v", err)
	}
	if _, err := admin.Exec(ctx, `DELETE FROM identity_provider WHERE id = $1`,
		newcomerTermsProvider.String()); err != nil {
		t.Fatalf("clearing an earlier run: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, `DELETE FROM identity_provider WHERE id = $1`,
			newcomerTermsProvider.String()); err != nil {
			t.Errorf("cleaning up the provider: %v", err)
		}
	})

	writer, signIn, uow := realWriter(ctx, t)
	writer.Encryptor = ring(t, crypto.KeyMaterial{
		ID: "k141newcomer", Material: secret.New("f1338-newcomer-key-not-a-real-secret"),
	})
	writer.TermsIn = identityservice.SignInPolicyResolver{
		Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Instance:   postgres.NewInstanceSettingRepository(),
		UnitOfWork: uow,
	}
	writer.Terms = signIn
	providers := postgres.NewIdentityProviderRepository()
	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: newcomerTermsProvider, TenantID: newcomerTermsTenant, Issuer: "https://newcomer-terms.example",
		ClientID: "hubtask", Provisioning: string(domain.ProvisionAny), Enabled: true, Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if err := uow.Within(ctx, persistence.Scope{TenantID: newcomerTermsTenant}, func(ctx context.Context) error {
		sealed, err := writer.Encryptor.Seal(ctx, secret.New("s3cr3t"),
			identityservice.ClientSecretPurpose(newcomerTermsTenant))
		if err != nil {
			return err
		}
		_, err = providers.Insert(ctx, configured, sealed)
		return err
	}); err != nil {
		t.Fatalf("storing the provider: %v", err)
	}

	run := strconv.FormatInt(time.Now().UnixNano(), 36)
	email := "nora-" + run + "@newcomer.example"
	cursors := security.NewCursorCodec(secret.New(installationSecret))
	oidc := identityservice.OidcWriter{
		Session:   writer,
		Providers: providers,
		Flows:     postgres.NewOidcFlowRepository(security.NewOidcFlowHasher(secret.New(installationSecret))),
		External:  postgres.NewExternalAccountRepository(),
		Accounts:  postgres.NewAccountRepository(),
		Grants:    postgres.NewMembershipGrantRepository(cursors),
		Groups:    postgres.NewGroupRepository(cursors),
		Relying: stubRelying{identity: identityprovider.Identity{
			Subject: "nora-" + run, Email: email, EmailVerified: true, DisplayName: "Nora",
		}},
		RedirectURL: "https://hubtask.example/auth/callback",
	}
	arrive := func() secret.Secret {
		t.Helper()
		started, err := identityservice.StartOidcSignIn{Writer: oidc}.Execute(ctx,
			identityservice.StartOidcSignInCommand{ProviderID: newcomerTermsProvider, TenantHeader: newcomerTermsTenant.String()})
		if err != nil {
			t.Fatalf("starting: %v", err)
		}
		result, err := identityservice.CompleteOidcSignIn{Writer: oidc}.Execute(ctx,
			identityservice.CompleteOidcSignInCommand{Code: "the-code", State: started.State})
		if err != nil || result.Challenge == nil || result.Challenge.Methods[0] != "TERMS" {
			t.Fatalf("the arrival answered (%+v, %v), want the TERMS step", result, err)
		}
		if result.Challenge.Email != email {
			t.Errorf("the step names %q, want the arriving address", result.Challenge.Email)
		}
		return result.Challenge.Token
	}
	accountsOf := func() int {
		t.Helper()
		var count int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM account WHERE tenant_id = $1 AND email = $2`,
			newcomerTermsTenant.String(), email).Scan(&count); err != nil {
			t.Fatalf("counting accounts: %v", err)
		}
		return count
	}

	first, second := arrive(), arrive()
	// What a person who declines leaves: no account, only the steps' rows, which carry the arrival.
	if made := accountsOf(); made != 0 {
		t.Fatalf("the arrival made %d accounts before the terms were agreed", made)
	}
	var waiting int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM auth_pending
		WHERE tenant_id = $1 AND account_id IS NULL AND arrival_email = $2 AND link_subject = $3`,
		newcomerTermsTenant.String(), email, "nora-"+run).Scan(&waiting); err != nil || waiting != 2 {
		t.Fatalf("the steps waiting: %d (%v), want two", waiting, err)
	}

	agree := identityservice.AgreeToTerms{Writer: writer, Arrivals: oidc}
	command := func(token secret.Secret) identityservice.AgreeToTermsCommand {
		return identityservice.AgreeToTermsCommand{
			PendingToken: token, TermsURL: "https://newcomer.example/terms", TermsVersion: "3",
		}
	}
	one, err := agree.Execute(ctx, command(first))
	if err != nil {
		t.Fatalf("agreeing: %v", err)
	}
	var (
		status, version string
		linked          int
	)
	if err := admin.QueryRow(ctx, `
		SELECT a.status::text, coalesce(a.terms_version, ''),
		       (SELECT count(*) FROM account_identity i WHERE i.account_id = a.id AND i.provider_id = $3)
		FROM account a WHERE a.id = $1 AND a.tenant_id = $2`,
		one.Session.AccountID.String(), newcomerTermsTenant.String(), newcomerTermsProvider.String(),
	).Scan(&status, &version, &linked); err != nil {
		t.Fatalf("reading the account: %v", err)
	}
	if status != "ACTIVE" || version != "3" || linked != 1 {
		t.Errorf("the account is %s, agreed to %q, connected %d times", status, version, linked)
	}
	if one.Session.SignedInWith != domain.SignedInWithOidc || one.Session.SignedInVia != newcomerTermsProvider {
		t.Errorf("the session was opened by %q via %q", one.Session.SignedInWith, one.Session.SignedInVia)
	}

	two, err := agree.Execute(ctx, command(second))
	if err != nil {
		t.Fatalf("agreeing in the second tab: %v", err)
	}
	if two.Session.AccountID != one.Session.AccountID || accountsOf() != 1 {
		t.Errorf("the second agreement signed in %s beside %s, %d accounts", two.Session.AccountID,
			one.Session.AccountID, accountsOf())
	}
}
