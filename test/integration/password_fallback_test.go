// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// ADR-0076 §4 as the owner decided it on 2026-10-04 (E2, #1138), against the real database and the
// real resolver: the password opens whenever the workspace's resolved methods leave no way in that
// works - here an installation default without the password, which no workspace screen wrote and no
// workspace guard could refuse, in a workspace whose invited owner has to get in before anybody can
// switch a way in on. The service tests show the same over fakes; what only this test shows
// is that the rule the resolver reads from `instance_setting` is the rule the fallback answers.
//
// The installation's level is one row set for every workspace in this database, so the test restores
// it whole when it ends; the package runs its tests one after another, never beside each other.
var (
	fallbackTenant   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000031a1")
	fallbackAccount  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000031b1")
	fallbackProvider = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000031c1")
	fallbackInvited  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000031b2")
)

const fallbackPassphrase = "seven blue lanterns over the harbour"

func TestAnInstallationDefaultWithoutThePasswordOpensItAsTheFallback(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	session, signIn, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)

	hash, err := session.Passwords.Hash(secret.New(fallbackPassphrase))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	// A workspace of this test's own, with nothing of its own decided: the installation's level is
	// its rule.
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, 'no-way-in', 'No Way In', 'en', 'UTC')
		ON CONFLICT (id) DO UPDATE SET settings = '{}'::jsonb, status = 'ACTIVE'`,
		fallbackTenant.String()); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status)
		VALUES ($1, $2, 'USER', 'fallback@example.org', 'Fallback', $3, 'ACTIVE')
		ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		fallbackAccount.String(), fallbackTenant.String(), hash); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	// And the person a provisioned workspace is born with: invited, holding no password yet.
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'invited-owner@example.org', 'Invited Owner', 'INVITED')
		ON CONFLICT (id) DO UPDATE SET status = 'INVITED', password_hash = NULL`,
		fallbackInvited.String(), fallbackTenant.String()); err != nil {
		t.Fatalf("seeding the invited account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DELETE FROM identity_provider WHERE id = $1`,
			fallbackProvider.String())
	})

	// The installation's default: providers only. Restored whole when the test ends.
	instance := postgres.NewInstanceSettingRepository()
	var before repository.InstanceLevel
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		read, err := instance.Read(ctx)
		before = read
		return err
	}); err != nil {
		t.Fatalf("reading the installation's level: %v", err)
	}
	writeLevel := func(level repository.InstanceLevel) error {
		return uow.Within(context.Background(), persistence.SystemScope(), func(ctx context.Context) error {
			return instance.Write(ctx, level, "", time.Now().UTC())
		})
	}
	t.Cleanup(func() {
		if err := writeLevel(before); err != nil {
			t.Errorf("restoring the installation's level: %v", err)
		}
	})
	providersAlone := before
	providersAlone.Policy.Patch.Methods = &[]string{domain.MethodOidc}
	if err := writeLevel(providersAlone); err != nil {
		t.Fatalf("writing the installation's default: %v", err)
	}

	resolver := identityservice.SignInPolicyResolver{
		Workspaces: postgres.NewWorkspaceSettingsRepository(), Instance: instance, UnitOfWork: uow,
	}
	providers := postgres.NewIdentityProviderRepository()
	passwords := identityservice.PasswordWriter{
		Session: session, Resolver: resolver,
		Accounts: postgres.NewPasswordRepository(), Histories: postgres.NewPasswordRepository(),
		WaysIn: identityservice.WaysIn{
			Providers: providers, Workspaces: postgres.NewWorkspaceSettingsRepository(),
			UnitOfWork: uow, Clock: clockadapter.System{},
		},
		Text: text.Composing{}, UnitOfWork: uow, Clock: clockadapter.System{}, IDs: session.IDs,
	}
	session.Rule = passwords
	card := identityservice.GetSignInRules{
		Resolver: resolver, Tenants: signIn, Providers: providers,
		Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Clock:      clockadapter.System{}, UnitOfWork: uow, Multi: true,
	}

	read := func() (identityservice.SignInRules, bool, bool) {
		t.Helper()
		rules, err := card.Execute(ctx, identityservice.GetSignInRulesCommand{
			TenantHeader: fallbackTenant.String(),
		})
		if err != nil {
			t.Fatalf("reading the card: %v", err)
		}
		open, fallback, err := passwords.PasswordOpen(ctx, fallbackTenant)
		if err != nil {
			t.Fatalf("asking the door: %v", err)
		}
		return rules, open, fallback.Opens()
	}
	signsIn := func() error {
		t.Helper()
		_, err := identityservice.SignIn{Writer: session}.Execute(ctx, identityservice.SignInCommand{
			Email: "fallback@example.org", Password: secret.New(fallbackPassphrase),
			TenantHeader: fallbackTenant.String(), RemoteAddr: "198.51.100.31",
		})
		return err
	}

	// No way in works: the card offers the password, the door lets it through, and the sign-in is in
	// the workspace's trail.
	rules, open, fallback := read()
	if !rules.PasswordFallback || !slices.Equal(rules.Methods, []string{domain.MethodDirect}) {
		t.Errorf("the card offers %v (fallback %v), want the password as the fallback",
			rules.Methods, rules.PasswordFallback)
	}
	if !open || !fallback {
		t.Errorf("the door answers open %v, fallback %v", open, fallback)
	}
	if err := signsIn(); err != nil {
		t.Fatalf("signing in through the fallback: %v", err)
	}
	recorded := func(actor shared.ID) int {
		t.Helper()
		var counted int
		if err := admin.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			WHERE tenant_id = $1 AND action = 'auth.password_fallback' AND actor_id = $2
			  AND changes -> 'cause' ->> 'to' = $3`,
			fallbackTenant.String(), actor.String(),
			string(identityservice.FallbackCauseNoWayIn)).Scan(&counted); err != nil {
			t.Fatalf("reading the trail: %v", err)
		}
		return counted
	}
	if recorded(fallbackAccount) == 0 {
		t.Error("the sign-in through the fallback is not in the workspace's trail with its cause")
	}

	// The invited owner accepts with a password - nobody else is in to switch a way in on - and the
	// entry lands in the redemption's own transaction, which only a real one can show.
	minted, err := identityservice.MintRedemptionToken{
		Accounts: signIn, UnitOfWork: uow, Clock: clockadapter.System{}, Entropy: clockadapter.CryptoRandom{},
	}.MintRedemptionToken(ctx, fallbackTenant, fallbackInvited)
	if err != nil || minted.IsEmpty() {
		t.Fatalf("minting the invitation: (%v, empty %v)", err, minted.IsEmpty())
	}
	if _, err := (identityservice.RedeemInvitation{Writer: session, Passwords: &passwords}).Execute(ctx,
		identityservice.RedeemInvitationCommand{
			Token: minted, Password: secret.New(fallbackPassphrase), TenantHeader: fallbackTenant.String(),
		}); err != nil {
		t.Fatalf("redeeming through the fallback: %v", err)
	}
	if recorded(fallbackInvited) == 0 {
		t.Error("the redemption through the fallback is not in the workspace's trail with its cause")
	}

	// A way in switched on ends it: the workspace's own provider, and the installation's rule is the
	// rule again - the password is refused.
	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: fallbackProvider, TenantID: fallbackTenant, Issuer: "https://id.no-way-in.example",
		ClientID: "hubtask-no-way-in", Enabled: true, Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}
	inTenant(t, uow, fallbackTenant, func(ctx context.Context) error {
		_, err := providers.Insert(ctx, configured, cryptoport.Sealed{KeyID: "k-fallback", Ciphertext: []byte("s")})
		return err
	})
	rules, open, fallback = read()
	if rules.PasswordFallback || open || fallback {
		t.Errorf("with a way in switched on: card %v (fallback %v), door open %v, fallback %v",
			rules.Methods, rules.PasswordFallback, open, fallback)
	}
	if err := signsIn(); shared.AsError(err).DetailCode != "auth.password_not_offered" {
		t.Errorf("with a way in switched on the password answered %v", err)
	}
}
