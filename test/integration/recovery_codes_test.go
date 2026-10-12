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

// The recovery codes against the real database. The service tests run them against fakes whose
// Replace and Disable are what the test says they are, and whose accounts carry their workspace;
// here the statements and the account's mapping are the real ones, and a code is presented on the
// real step-up path. Its own workspace, so that no other file's switch on tenant A can demand the
// factor.
var recoveryTenant = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053a1")

const recoveryPassphrase = "the passphrase behind the factor"

// armedForRecovery is one account of the recovery workspace with a confirmed factor and its first
// ten codes, signed in, with the step-up ledger cleared.
type armedForRecovery struct {
	writer identityservice.SessionWriter
	actor  appshared.ActorContext
	codes  []secret.Secret
	stepUp func(identityservice.StepUpCommand) (string, error)
	// byPassword is a fresh step-up proven with the password.
	byPassword func() string
	remaining  func() int
}

func armForRecovery(ctx context.Context, t *testing.T, account, session shared.ID, email string) armedForRecovery {
	t.Helper()
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
		VALUES ($1, $2, 'USER', $4, 'Recovery', $3, 'ACTIVE')
		ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash`,
		account.String(), recoveryTenant.String(), hash, email); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		VALUES ($1, $2, $3, now(), now() + interval '30 days')
		ON CONFLICT (id) DO UPDATE SET revoked_at = NULL`,
		session.String(), recoveryTenant.String(), account.String()); err != nil {
		t.Fatalf("seeding the session: %v", err)
	}
	if _, err := admin.Exec(ctx, `DELETE FROM account_mfa WHERE account_id = $1`, account.String()); err != nil {
		t.Fatalf("clearing an earlier run's factor: %v", err)
	}
	inTenant(t, uow, recoveryTenant, func(ctx context.Context) error {
		return signIn.Clear(ctx, "stepup:"+account.String())
	})
	armed := armedForRecovery{
		writer: writer,
		actor: appshared.ActorContext{
			Kind: appshared.ActorUser, TenantID: recoveryTenant, AccountID: account,
			TokenID: session, AccountName: "Recovery", Scopes: []string{"account:read"},
		},
	}
	armed.stepUp = func(cmd identityservice.StepUpCommand) (string, error) {
		t.Helper()
		grant, err := identityservice.StepUp{Writer: writer}.Execute(ctx, armed.actor, cmd)
		return grant.Token.Reveal(), err
	}
	armed.byPassword = func() string {
		t.Helper()
		proof, err := armed.stepUp(identityservice.StepUpCommand{Password: secret.New(recoveryPassphrase)})
		if err != nil {
			t.Fatalf("stepping up with the password: %v", err)
		}
		return proof
	}
	armed.remaining = func() int {
		t.Helper()
		var left int
		inTenant(t, uow, recoveryTenant, func(ctx context.Context) error {
			count, err := writer.Recovery.Remaining(ctx, account)
			left = count
			return err
		})
		return left
	}

	minted, err := identityservice.EnrollTotp{Writer: writer}.Execute(ctx, armed.actor, identityservice.EnrollTotpCommand{})
	if err != nil {
		t.Fatalf("enrolling: %v", err)
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(minted.Secret.Reveal())
	if err != nil {
		t.Fatalf("reading the secret: %v", err)
	}
	if _, err := (identityservice.ConfirmTotp{Writer: writer}).Execute(ctx, armed.actor,
		identityservice.ConfirmTotpCommand{Code: domain.TotpCode(raw, domain.TotpStep(time.Now()))}); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	armed.codes = minted.RecoveryCodes
	return armed
}

// UC-ID-03 check 6: turning the factor off removes the codes with it - none remains, and a code
// that was still unused no longer proves anything.
func TestTurningTheFactorOffRemovesTheRecoveryCodes(t *testing.T) {
	ctx := context.Background()
	account := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053c1")
	armed := armForRecovery(ctx, t, account, shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053d1"),
		"recovery-codes@example.org")

	// A code works while the factor stands, so that its refusal after is the removal's doing.
	if _, err := armed.stepUp(identityservice.StepUpCommand{RecoveryCode: armed.codes[0]}); err != nil {
		t.Fatalf("a live code was refused: %v", err)
	}
	if got := armed.remaining(); got != domain.RecoveryCodeCount-1 {
		t.Fatalf("%d codes remain before the factor is turned off, want %d", got, domain.RecoveryCodeCount-1)
	}

	if err := (identityservice.DisableTotp{Writer: armed.writer}).Execute(ctx, armed.actor,
		identityservice.DisableTotpCommand{StepUpToken: armed.byPassword()}); err != nil {
		t.Fatalf("turning the factor off: %v", err)
	}
	if got := armed.remaining(); got != 0 {
		t.Errorf("%d recovery codes remain after the factor was turned off", got)
	}
	var stored int
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT count(*) FROM account_recovery_code WHERE account_id = $1`,
		account.String()).Scan(&stored); err != nil || stored != 0 {
		t.Errorf("%d recovery code rows are left (%v)", stored, err)
	}
	if _, err := armed.stepUp(identityservice.StepUpCommand{RecoveryCode: armed.codes[1]}); err == nil {
		t.Error("an unused code still proved a step-up after the factor was turned off")
	}
}

// UC-ID-03 check 4: *New codes* asks for a fresh proof, replaces all ten, and the old ones stop
// working at once - through the real account mapping, whose rows the trail entry is built from.
func TestNewCodesReplaceAllTenAndTheOldOnesStopAtOnce(t *testing.T) {
	ctx := context.Background()
	armed := armForRecovery(ctx, t, shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053c2"),
		shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053d2"), "new-codes@example.org")
	regenerate := identityservice.RegenerateRecoveryCodes{Writer: armed.writer}

	if _, err := regenerate.Execute(ctx, armed.actor, ""); err == nil {
		t.Fatal("new codes were answered without a fresh proof")
	}

	fresh, err := regenerate.Execute(ctx, armed.actor, armed.byPassword())
	if err != nil {
		t.Fatalf("new codes behind a fresh proof: %v", err)
	}
	if len(fresh.Codes) != domain.RecoveryCodeCount {
		t.Fatalf("%d new codes, want %d", len(fresh.Codes), domain.RecoveryCodeCount)
	}
	if got := armed.remaining(); got != domain.RecoveryCodeCount {
		t.Errorf("%d codes remain after the replacement, want %d", got, domain.RecoveryCodeCount)
	}
	for _, old := range armed.codes[:3] {
		if _, err := armed.stepUp(identityservice.StepUpCommand{RecoveryCode: old}); err == nil {
			t.Fatal("an old code still proved a step-up after new codes were made")
		}
	}
	if _, err := armed.stepUp(identityservice.StepUpCommand{RecoveryCode: fresh.Codes[0]}); err != nil {
		t.Errorf("a new code was refused: %v", err)
	}
}
