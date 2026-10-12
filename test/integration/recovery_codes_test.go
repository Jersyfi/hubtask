// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/base32"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The recovery codes' end with the factor, against the real database. The service tests run it
// against a fake whose Disable is what the test says it is; here the statements are the real ones,
// and a code is presented on the real step-up path. Its own workspace, so that no other file's
// switch on tenant A can demand the factor.
var (
	recoveryTenant  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053a1")
	recoveryAccount = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053c1")
	recoverySession = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053d1")
)

const recoveryPassphrase = "the passphrase behind the factor"

// UC-ID-03 check 6: turning the factor off removes the codes with it - none remains, and a code
// that was still unused no longer proves anything.
func TestTurningTheFactorOffRemovesTheRecoveryCodes(t *testing.T) {
	ctx := context.Background()
	writer, signIn, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)
	hash, err := writer.Passwords.Hash(secret.New(recoveryPassphrase))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, 'recovery-codes', 'Recovery Codes', 'en', 'UTC')
		ON CONFLICT (id) DO NOTHING`, recoveryTenant.String()); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status)
		VALUES ($1, $2, 'USER', 'recovery-codes@example.org', 'Recovery', $3, 'ACTIVE')
		ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		recoveryAccount.String(), recoveryTenant.String(), hash); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		VALUES ($1, $2, $3, now(), now() + interval '30 days')
		ON CONFLICT (id) DO UPDATE SET revoked_at = NULL`,
		recoverySession.String(), recoveryTenant.String(), recoveryAccount.String()); err != nil {
		t.Fatalf("seeding the session: %v", err)
	}
	if _, err := admin.Exec(ctx, `DELETE FROM account_mfa WHERE account_id = $1`, recoveryAccount.String()); err != nil {
		t.Fatalf("clearing an earlier run's factor: %v", err)
	}
	inTenant(t, uow, recoveryTenant, func(ctx context.Context) error {
		return signIn.Clear(ctx, "stepup:"+recoveryAccount.String())
	})
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: recoveryTenant, AccountID: recoveryAccount,
		TokenID: recoverySession, AccountName: "Recovery", Scopes: []string{"account:read"},
	}
	stepUp := func(cmd identityservice.StepUpCommand) (string, error) {
		t.Helper()
		grant, err := identityservice.StepUp{Writer: writer}.Execute(ctx, actor, cmd)
		return grant.Token.Reveal(), err
	}
	byPassword := func() string {
		t.Helper()
		proof, err := stepUp(identityservice.StepUpCommand{Password: secret.New(recoveryPassphrase)})
		if err != nil {
			t.Fatalf("stepping up with the password: %v", err)
		}
		return proof
	}
	remaining := func() int {
		t.Helper()
		var left int
		inTenant(t, uow, recoveryTenant, func(ctx context.Context) error {
			count, err := writer.Recovery.Remaining(ctx, recoveryAccount)
			left = count
			return err
		})
		return left
	}

	// Armed, with its first ten codes.
	minted, err := identityservice.EnrollTotp{Writer: writer}.Execute(ctx, actor, identityservice.EnrollTotpCommand{})
	if err != nil {
		t.Fatalf("enrolling: %v", err)
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(minted.Secret.Reveal())
	if err != nil {
		t.Fatalf("reading the secret: %v", err)
	}
	if _, err := (identityservice.ConfirmTotp{Writer: writer}).Execute(ctx, actor,
		identityservice.ConfirmTotpCommand{Code: domain.TotpCode(raw, domain.TotpStep(time.Now()))}); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	codes := minted.RecoveryCodes
	// A code works while the factor stands, so that its refusal after is the removal's doing.
	if _, err := stepUp(identityservice.StepUpCommand{RecoveryCode: codes[0]}); err != nil {
		t.Fatalf("a live code was refused: %v", err)
	}
	if got := remaining(); got != domain.RecoveryCodeCount-1 {
		t.Fatalf("%d codes remain before the factor is turned off, want %d", got, domain.RecoveryCodeCount-1)
	}

	if err := (identityservice.DisableTotp{Writer: writer}).Execute(ctx, actor,
		identityservice.DisableTotpCommand{StepUpToken: byPassword()}); err != nil {
		t.Fatalf("turning the factor off: %v", err)
	}
	if got := remaining(); got != 0 {
		t.Errorf("%d recovery codes remain after the factor was turned off", got)
	}
	var stored int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM account_recovery_code WHERE account_id = $1`,
		recoveryAccount.String()).Scan(&stored); err != nil || stored != 0 {
		t.Errorf("%d recovery code rows are left (%v)", stored, err)
	}
	if _, err := stepUp(identityservice.StepUpCommand{RecoveryCode: codes[1]}); err == nil {
		t.Error("an unused code still proved a step-up after the factor was turned off")
	}
}
