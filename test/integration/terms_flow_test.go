// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// UC-ID-19 checks 1, 2 and 4 against the real database, as the app role: an invitation accepted
// with the workspace's terms agreed stores the link, the version and the time on the account and
// one trail entry with the version; a new version puts the TERMS step before the next session, and
// agreeing opens it. Its own workspace, named by nobody else in this shared database.
var (
	termsTenant  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000a1901")
	termsInvited = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000a1911")
)

const workspaceTerms = "https://acme.example/terms"

func setWorkspaceTerms(ctx context.Context, t *testing.T, version string) {
	t.Helper()
	if _, err := adminPool(ctx, t).Exec(ctx, `
		UPDATE tenant SET settings = settings || jsonb_build_object('sign_in_policy',
			jsonb_build_object('terms_url', $2::text, 'terms_version', $3::text))
		WHERE id = $1`, termsTenant.String(), workspaceTerms, version); err != nil {
		t.Fatalf("setting the workspace's terms: %v", err)
	}
}

func TestAnInvitationAgreesToTheTermsAndANewVersionAsksAgain(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	writer, signIn, uow := realWriter(ctx, t)
	writer.TermsIn = identityservice.SignInPolicyResolver{
		Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Instance:   postgres.NewInstanceSettingRepository(),
		UnitOfWork: uow,
	}
	writer.Terms = signIn

	redeem, err := domain.NewRedemptionToken(termsTenant, sessionSecretOf(0xD9))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'terms-a', 'Terms A')
		ON CONFLICT (id) DO NOTHING`, termsTenant.String()); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	// A fresh invitation every run: the shared database keeps the last run's account.
	if _, err := admin.Exec(ctx, `DELETE FROM account WHERE id = $1`, termsInvited.String()); err != nil {
		t.Fatalf("clearing the last run: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status,
			redemption_token_hash, redemption_expires_at)
		VALUES ($1, $2, 'USER', 'anna@terms.example', 'Anna', 'INVITED', $3, now() + interval '14 days')`,
		termsInvited.String(), termsTenant.String(),
		security.NewRedemptionTokenHasher(secret.New(installationSecret)).Hash(redeem.Secret())); err != nil {
		t.Fatalf("seeding the invitation: %v", err)
	}
	setWorkspaceTerms(ctx, t, "3")

	// Check 1: accepted with the terms agreed.
	if _, err := (identityservice.RedeemInvitation{Writer: writer}).Execute(ctx, identityservice.RedeemInvitationCommand{
		Token: secret.New(redeem.Secret()), Password: secret.New("seven blue lanterns over the harbour"),
		TermsURL: workspaceTerms, TermsVersion: "3",
	}); err != nil {
		t.Fatalf("redeeming: %v", err)
	}
	var url, version, status string
	if err := admin.QueryRow(ctx, `SELECT terms_url, terms_version, status FROM account WHERE id = $1`,
		termsInvited.String()).Scan(&url, &version, &status); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if url != workspaceTerms || version != "3" || status != "ACTIVE" {
		t.Errorf("the account holds (%q, %q, %s)", url, version, status)
	}

	// Check 2: version 4 puts the step before the session.
	setWorkspaceTerms(ctx, t, "4")
	result, err := (identityservice.SignIn{Writer: writer}).Execute(ctx, identityservice.SignInCommand{
		Email: "anna@terms.example", Password: secret.New("seven blue lanterns over the harbour"),
		TenantSlug: "terms-a",
	})
	if err != nil {
		t.Fatalf("signing in: %v", err)
	}
	if result.Pair != nil || result.Challenge == nil || result.Challenge.Terms == nil ||
		result.Challenge.Terms.Version != "4" || result.Challenge.Terms.SetBy != domain.TermsByWorkspace {
		t.Fatalf("the sign-in answered %+v, want the TERMS step for version 4", result)
	}
	pair, err := (identityservice.AgreeToTerms{Writer: writer}).Execute(ctx, identityservice.AgreeToTermsCommand{
		PendingToken: result.Challenge.Token, TermsURL: workspaceTerms, TermsVersion: "4",
	})
	if err != nil || pair.Session.AccountID != termsInvited {
		t.Fatalf("agreeing answered (%+v, %v)", pair.Session, err)
	}
	again, err := (identityservice.SignIn{Writer: writer}).Execute(ctx, identityservice.SignInCommand{
		Email: "anna@terms.example", Password: secret.New("seven blue lanterns over the harbour"),
		TenantSlug: "terms-a",
	})
	if err != nil || again.Pair == nil {
		t.Errorf("the sign-in after agreeing answered (%+v, %v), want the pair", again, err)
	}

	// Check 4: each agreement in the workspace's trail, with the version and the link.
	var versions []string
	rows, err := admin.Query(ctx, `
		SELECT changes -> 'terms_version' ->> 'to' FROM audit_log
		WHERE tenant_id = $1 AND target_id = $2 AND action = 'auth.terms_agreed'
		  AND changes -> 'terms_url' ->> 'to' = $3
		ORDER BY occurred_at`, termsTenant.String(), termsInvited.String(), workspaceTerms)
	if err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var held string
		if err := rows.Scan(&held); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		versions = append(versions, held)
	}
	if len(versions) != 2 || versions[0] != "3" || versions[1] != "4" {
		t.Errorf("the trail records versions %v, want 3 then 4", versions)
	}
}
