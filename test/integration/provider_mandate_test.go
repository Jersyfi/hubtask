// SPDX-License-Identifier: Apache-2.0
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

// UC-INS-11 check 4 against the real database and the real resolver (identity.md §18): locking the
// installation's ways in with providers among them turns the installation's invite-only offer on in
// every workspace. The card draws it, the password fallback stays shut, and the installation's count
// (migration 0133) counts every live workspace - so the removal guard refuses to delete the
// connections of a provider that is the way into all of them. The SQL predicate and the domain's are
// held to the same table of stored lock shapes, with the answers written down.
//
// The installation's level is one row set for every workspace in this database, so the test restores
// it whole when it ends; the package runs its tests one after another.
var (
	mandateTenant   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000038a1")
	mandateAccount  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000038b1")
	mandateProvider = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000038c1")
)

func TestALockWithProvidersTurnsTheInviteOnlyOfferOnEverywhere(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	session, signIn, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)
	providers := postgres.NewIdentityProviderRepository()
	instance := postgres.NewInstanceSettingRepository()

	hash, err := session.Passwords.Hash(secret.New(fallbackPassphrase))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	// A workspace of this test's own that never switched the provider on.
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, 'mandate', 'Mandate', 'en', 'UTC')
		ON CONFLICT (id) DO UPDATE SET settings = '{}'::jsonb, status = 'ACTIVE', deleted_at = NULL`,
		mandateTenant.String()); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status)
		VALUES ($1, $2, 'USER', 'mandate@example.org', 'Mandate', $3, 'ACTIVE')
		ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		mandateAccount.String(), mandateTenant.String(), hash); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}

	// The installation's invite-only offer, switched on in no workspace.
	offered, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: mandateProvider, Issuer: "https://id.mandate.example", ClientID: "hubtask-mandate",
		Provisioning: string(domain.ProvisionInvitedOnly), Enabled: true, Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("building the offer: %v", err)
	}
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := providers.Insert(ctx, offered, cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("s")})
		return err
	}); err != nil {
		t.Fatalf("offering it: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DELETE FROM identity_provider WHERE id = $1`,
			mandateProvider.String())
	})

	// The installation's level, restored whole when the test ends.
	var before repository.InstanceLevel
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		read, err := instance.Read(ctx)
		before = read
		return err
	}); err != nil {
		t.Fatalf("reading the installation's level: %v", err)
	}
	t.Cleanup(func() {
		if err := uow.Within(context.Background(), persistence.SystemScope(), func(ctx context.Context) error {
			return instance.Write(ctx, before, "", time.Now().UTC())
		}); err != nil {
			t.Errorf("restoring the installation's level: %v", err)
		}
	})
	// storeMethods writes the stored shape directly, so shapes no screen writes are asked as well.
	storeMethods := func(value, lock string) {
		t.Helper()
		if value == "" {
			if _, err := admin.Exec(ctx, `DELETE FROM instance_setting WHERE key = 'sign_in.methods'`); err != nil {
				t.Fatalf("clearing the methods: %v", err)
			}
			return
		}
		if _, err := admin.Exec(ctx, `
			INSERT INTO instance_setting (key, value, lock_origin) VALUES ('sign_in.methods', $1::jsonb, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, lock_origin = EXCLUDED.lock_origin`,
			value, lock); err != nil {
			t.Fatalf("storing the methods: %v", err)
		}
	}
	liveWorkspaces := func() int {
		t.Helper()
		var counted int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM tenant WHERE deleted_at IS NULL`).Scan(&counted); err != nil {
			t.Fatalf("counting the workspaces: %v", err)
		}
		return counted
	}
	counted := func(scope persistence.Scope) int {
		t.Helper()
		var found domain.IdentityProvider
		if err := uow.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
			var err error
			found, err = providers.Find(ctx, mandateProvider)
			return err
		}); err != nil {
			t.Fatalf("reading the count: %v", err)
		}
		return found.OfferedWorkspaces
	}
	lockedIn := func() bool {
		t.Helper()
		var level repository.InstanceLevel
		if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			var err error
			level, err = instance.Read(ctx)
			return err
		}); err != nil {
			t.Fatalf("reading the installation's level: %v", err)
		}
		return domain.Effective(level.Policy, domain.PolicyLayer{}, domain.PolicyLayer{}).ProvidersLockedIn()
	}

	// The two predicates, one table, the answers written down rather than one predicate checked
	// against the other.
	for _, shape := range []struct {
		name, value, lock string
		mandates          bool
	}{
		{"no row", "", "", false},
		{"providers alone, locked", `["OIDC"]`, "INSTANCE", true},
		{"password and providers, locked", `["PASSWORD","OIDC"]`, "INSTANCE", true},
		{"the password alone, locked", `["PASSWORD"]`, "INSTANCE", false},
		{"providers alone, open", `["OIDC"]`, "OPEN", false},
		{"an empty list, locked", `[]`, "INSTANCE", true},
		{"an unknown way in, locked", `["SEMAPHORE"]`, "INSTANCE", true},
	} {
		storeMethods(shape.value, shape.lock)
		if got := lockedIn(); got != shape.mandates {
			t.Errorf("%s: the domain reads mandated %v, want %v", shape.name, got, shape.mandates)
		}
		live, got := liveWorkspaces(), counted(persistence.SystemScope())
		if (got == live) != shape.mandates || (!shape.mandates && got != 0) {
			t.Errorf("%s: the installation counts %d of %d live workspaces, mandated %v",
				shape.name, got, live, shape.mandates)
		}
		// A workspace learns nothing of the others, mandate or not (P-01).
		if got := counted(persistence.Scope{TenantID: mandateTenant}); got != 0 {
			t.Errorf("%s: a workspace reads a count of %d", shape.name, got)
		}
	}

	// Under a lock to providers alone: the card draws the provider, and the password stays shut.
	storeMethods(`["OIDC"]`, "INSTANCE")
	resolver := identityservice.SignInPolicyResolver{
		Workspaces: postgres.NewWorkspaceSettingsRepository(), Instance: instance, UnitOfWork: uow,
	}
	passwords := identityservice.PasswordWriter{
		Session: session, Resolver: resolver,
		Accounts: postgres.NewPasswordRepository(), Histories: postgres.NewPasswordRepository(),
		WaysIn: identityservice.WaysIn{
			Providers: providers, Workspaces: postgres.NewWorkspaceSettingsRepository(),
			Instance: instance, UnitOfWork: uow, Clock: clockadapter.System{},
		},
		Text: text.Composing{}, UnitOfWork: uow, Clock: clockadapter.System{}, IDs: session.IDs,
	}
	session.Rule = passwords
	card := identityservice.GetSignInRules{
		Resolver: resolver, Tenants: signIn, Providers: providers,
		Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Clock:      clockadapter.System{}, UnitOfWork: uow, Multi: true,
	}
	rules, err := card.Execute(ctx, identityservice.GetSignInRulesCommand{TenantHeader: mandateTenant.String()})
	if err != nil {
		t.Fatalf("reading the card: %v", err)
	}
	if rules.PasswordFallback || !slices.ContainsFunc(rules.Providers, func(p identityservice.ProviderSummary) bool {
		return p.ID == mandateProvider.String()
	}) {
		t.Errorf("under the mandate the card offers %v and %v (fallback %v)",
			rules.Methods, rules.Providers, rules.PasswordFallback)
	}
	open, fallback, err := passwords.PasswordOpen(ctx, mandateTenant)
	if err != nil || open || fallback.Opens() {
		t.Errorf("under the mandate the door answers open %v, fallback %q (%v)", open, fallback, err)
	}
	_, err = identityservice.SignIn{Writer: session}.Execute(ctx, identityservice.SignInCommand{
		Email: "mandate@example.org", Password: secret.New(fallbackPassphrase),
		TenantHeader: mandateTenant.String(), RemoteAddr: "198.51.100.38",
	})
	if shared.AsError(err).DetailCode != "auth.password_not_offered" {
		t.Errorf("under the mandate the password answered %v", err)
	}

	// The removal guard reads the mandate in the statement that deletes.
	remove := func() bool {
		t.Helper()
		var removed bool
		if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
			var err error
			removed, err = providers.Delete(ctx, mandateProvider, time.Now().UTC())
			return err
		}); err != nil {
			t.Fatalf("removing: %v", err)
		}
		return removed
	}
	if remove() {
		t.Fatal("a provider the installation's lock turns on everywhere was removed")
	}
	// The lock lifted, nobody switched it on: nothing uses it, and it may go.
	storeMethods("", "")
	if !remove() {
		t.Error("an offer no workspace uses was not removed once the lock lifted")
	}
}
