// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The last factor against the real database (identity.md §23.2, UC-ID-22 check 5, UC-ID-03 check
// 5): a passkey's removal and the app's take the same lock on the account and decide inside it, so
// two of them at once never each count the other as the factor that remains.

// demandedOf is the workspace's rule as the factor reads it.
type demandedOf domain.MfaRequirement

func (r demandedOf) JudgeSignIn(
	context.Context, shared.ID, domain.Account, secret.Secret,
) (identityservice.SignInVerdict, error) {
	return identityservice.SignInVerdict{FactorRequired: domain.MfaRequirement(r)}, nil
}

// withAKey gives an armed account of the recovery workspace one passkey of the workspace's address,
// and the writers that count it.
func withAKey(
	ctx context.Context, t *testing.T, armed armedForRecovery, credential byte, rule domain.MfaRequirement,
) (identityservice.SessionWriter, identityservice.PasskeyWriter, shared.ID) {
	t.Helper()
	key := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO webauthn_credential (id, tenant_id, account_id, credential_id, public_key, algorithm,
		                                 user_handle, backup_eligible, backed_up, name, created_at, rp_id)
		VALUES ($1, $2, $3, $4, '\xa50102', -7, $5, true, true, 'Key', now(), 'recovery-codes.hubtask.example')`,
		key.String(), recoveryTenant.String(), armed.actor.AccountID.String(),
		[]byte{0x53, credential, byte(time.Now().UnixNano())}, make([]byte, domain.PasskeyUserHandleBytes)); err != nil {
		t.Fatalf("seeding the key: %v", err)
	}
	party := identityservice.PasskeyParty{
		Workspaces:   postgres.NewWorkspaceSettingsRepository(),
		Installation: identityservice.PasskeyInstallation{Host: "hubtask.example", Scheme: "https", Multi: true},
	}
	writer := armed.writer
	writer.Rule = demandedOf(rule)
	writer.Keys = identityservice.PasskeyKeys{Passkeys: postgres.NewPasskeyRepository(), Party: party}
	passkeys := identityservice.PasskeyWriter{
		Session: writer, Passkeys: postgres.NewPasskeyRepository(),
		Workspaces: party.Workspaces, Installation: party.Installation,
		StepUp: identityservice.StepUpVerifier{Writer: writer},
	}
	return writer, passkeys, key
}

// raceTheLastTwo turns the app off and removes the key at once, while the test holds the account's
// lock - so both have passed their first check and given their proof before either decides - and
// answers both outcomes once the lock is let go.
func raceTheLastTwo(
	ctx context.Context, t *testing.T, armed armedForRecovery,
	writer identityservice.SessionWriter, passkeys identityservice.PasskeyWriter, key shared.ID,
) []error {
	t.Helper()
	// Two sessions, since a session holds one fresh proof at a time: the person in two windows.
	other := armed.actor
	other.TokenID = freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		VALUES ($1, $2, $3, now(), now() + interval '30 days')`,
		other.TokenID.String(), recoveryTenant.String(), other.AccountID.String()); err != nil {
		t.Fatalf("seeding the second session: %v", err)
	}
	disableProof := armed.byPassword()
	grant, err := identityservice.StepUp{Writer: armed.writer}.Execute(ctx, other,
		identityservice.StepUpCommand{Password: secret.New(recoveryPassphrase)})
	if err != nil {
		t.Fatalf("stepping up in the second window: %v", err)
	}
	removeProof := grant.Token.Reveal()
	outcomes := make(chan error, 2)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	//nolint:contextcheck // the helper opens a transaction of its own, as every test of this package does
	inTenant(t, uow, recoveryTenant, func(ctx context.Context) error {
		if err := postgres.NewPasskeyRepository().LockAccount(ctx, armed.actor.AccountID); err != nil {
			t.Fatal(err)
		}
		//nolint:contextcheck // separate requests: the context handed in carries the lock's transaction
		go func() {
			outcomes <- identityservice.DisableTotp{Writer: writer}.Execute(context.Background(), armed.actor,
				identityservice.DisableTotpCommand{StepUpToken: disableProof})
		}()
		//nolint:contextcheck // as above
		go func() {
			outcomes <- identityservice.RemoveOwnPasskey{Writer: passkeys}.Execute(context.Background(),
				other, key, removeProof)
		}()
		select {
		case err := <-outcomes:
			t.Fatalf("a removal decided while the account was held: %v", err)
		case <-time.After(700 * time.Millisecond):
		}
		return nil
	})
	var errs []error
	for range 2 {
		select {
		case err := <-outcomes:
			errs = append(errs, err)
		case <-time.After(10 * time.Second):
			t.Fatal("a removal never finished")
		}
	}
	return errs
}

func factorsLeft(ctx context.Context, t *testing.T, account shared.ID) (app, keys, codes int) {
	t.Helper()
	admin := adminPool(ctx, t)
	for target, query := range map[*int]string{
		&app:   `SELECT count(*) FROM account_mfa WHERE account_id = $1`,
		&keys:  `SELECT count(*) FROM webauthn_credential WHERE account_id = $1`,
		&codes: `SELECT count(*) FROM account_recovery_code WHERE account_id = $1 AND used_at IS NULL`,
	} {
		if err := admin.QueryRow(ctx, query, account.String()).Scan(target); err != nil {
			t.Fatalf("counting: %v", err)
		}
	}
	return app, keys, codes
}

// Under a rule that demands a factor, one of the two goes and the other is refused: never both.
func TestTheLastTwoFactorsAreNeverBothRemovedWhereTheRuleDemandsOne(t *testing.T) {
	ctx := context.Background()
	account := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053c4")
	armed := armForRecovery(ctx, t, account, shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053d4"),
		"two-factors-required@example.org")
	writer, passkeys, key := withAKey(ctx, t, armed, 4, domain.MfaForEveryone)

	errs := raceTheLastTwo(ctx, t, armed, writer, passkeys, key)

	refused := 0
	for _, err := range errs {
		switch {
		case err == nil:
		case errors.Is(err, shared.ErrForbidden) && strings.Contains(err.Error(), "auth.mfa_required_by_tenant"):
			refused++
		default:
			t.Errorf("a removal failed otherwise: %v", err)
		}
	}
	app, keys, codes := factorsLeft(ctx, t, account)
	if refused != 1 || app+keys != 1 {
		t.Errorf("%d refused; the account holds %d app and %d keys", refused, app, keys)
	}
	if codes != domain.RecoveryCodeCount {
		t.Errorf("%d codes beside the factor that remains, want all ten", codes)
	}
}

// Where nothing demands one, both go - and whichever is second takes the codes with it.
func TestTheLastTwoFactorsBothGoWhereNothingDemandsOneAndTheCodesWithThem(t *testing.T) {
	ctx := context.Background()
	account := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053c5")
	armed := armForRecovery(ctx, t, account, shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053d5"),
		"two-factors-free@example.org")
	writer, passkeys, key := withAKey(ctx, t, armed, 5, domain.MfaForNobody)

	for _, err := range raceTheLastTwo(ctx, t, armed, writer, passkeys, key) {
		if err != nil {
			t.Errorf("a removal was refused: %v", err)
		}
	}
	if app, keys, codes := factorsLeft(ctx, t, account); app+keys+codes != 0 {
		t.Errorf("left %d app, %d keys, %d codes", app, keys, codes)
	}
}

// A removal reaches only the account's own passkey (gate SG-3): another account's, and any from next
// door, match nothing.
func TestAPasskeyIsRemovedOnlyByItsOwnAccountInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	people := seedPasskeyPeople(ctx, t)
	repo := postgres.NewPasskeyRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	at := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	adas, beas := aPasskey(t, people.ada, 1, "iPhone", at), aPasskey(t, people.bea, 2, "Laptop", at)
	inTenant(t, uow, people.tenant, func(ctx context.Context) error {
		for _, credential := range []domain.PasskeyCredential{adas, beas} {
			if written, err := repo.Insert(ctx, credential); err != nil || !written {
				t.Fatalf("inserting: %v %v", written, err)
			}
		}
		if removed, err := repo.Remove(ctx, people.ada, beas.ID); err != nil || removed {
			t.Errorf("Ada removed Bea's passkey: %v %v", removed, err)
		}
		return nil
	})
	inTenant(t, uow, people.other, func(ctx context.Context) error {
		if removed, err := repo.Remove(ctx, people.ada, adas.ID); err != nil || removed {
			t.Errorf("next door removed Ada's passkey: %v %v", removed, err)
		}
		return nil
	})
	inTenant(t, uow, people.tenant, func(ctx context.Context) error {
		if removed, err := repo.Remove(ctx, people.ada, adas.ID); err != nil || !removed {
			t.Errorf("Ada could not remove her own: %v %v", removed, err)
		}
		if count, err := repo.CountOf(ctx, people.ada); err != nil || count != 0 {
			t.Errorf("Ada still holds %d (%v)", count, err)
		}
		if count, err := repo.CountOf(ctx, people.bea); err != nil || count != 1 {
			t.Errorf("Bea holds %d (%v)", count, err)
		}
		return nil
	})
}

// The codes' discard is one statement whose condition is the factor itself: while an app row stands -
// armed, or an enrolment that just showed its codes - or a passkey, it takes nothing.
func TestTheCodesGoOnlyWhereNoFactorRowStands(t *testing.T) {
	ctx := context.Background()
	account := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053c6")
	armed := armForRecovery(ctx, t, account, shared.MustParseID("01936f2a-7c1e-7000-8000-0000000053d6"),
		"discard-condition@example.org")
	_, _, key := withAKey(ctx, t, armed, 6, domain.MfaForNobody)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	discard := func() {
		t.Helper()
		inTenant(t, uow, recoveryTenant, func(ctx context.Context) error {
			return armed.writer.Recovery.DiscardUnbacked(ctx, account)
		})
	}
	admin := adminPool(ctx, t)

	discard()
	if _, _, codes := factorsLeft(ctx, t, account); codes != domain.RecoveryCodeCount {
		t.Fatalf("%d codes beside the app and a key", codes)
	}
	if _, err := admin.Exec(ctx, `UPDATE account_mfa SET confirmed_at = NULL WHERE account_id = $1`, account.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `DELETE FROM webauthn_credential WHERE id = $1`, key.String()); err != nil {
		t.Fatal(err)
	}
	discard()
	if _, _, codes := factorsLeft(ctx, t, account); codes != domain.RecoveryCodeCount {
		t.Errorf("%d codes beside an enrolment begun, want the ten it showed", codes)
	}
	if _, err := admin.Exec(ctx, `DELETE FROM account_mfa WHERE account_id = $1`, account.String()); err != nil {
		t.Fatal(err)
	}
	discard()
	if _, _, codes := factorsLeft(ctx, t, account); codes != 0 {
		t.Errorf("%d codes outlived every factor", codes)
	}
}
