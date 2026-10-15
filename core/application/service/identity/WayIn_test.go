// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ADR-0078 §6, UC-ID-04 check 9, UC-ID-05 check 6: a person is told when a way into their account
// changes. These tests hold each door to queueing exactly one notice, through its use case.

// toldOf records the notices queued, as the notification context would queue them.
type toldOf struct{ notices []toldNotice }

type toldNotice struct {
	tenant, account shared.ID
	change          domain.WayInChange
}

func (t *toldOf) WayInChanged(_ context.Context, tenantID, accountID shared.ID, change domain.WayInChange) error {
	t.notices = append(t.notices, toldNotice{tenant: tenantID, account: accountID, change: change})
	return nil
}

func (t *toldOf) only(test *testing.T, event domain.WayInEvent) {
	test.Helper()
	if len(t.notices) != 1 {
		test.Fatalf("%d notices queued, want one: %+v", len(t.notices), t.notices)
	}
	got := t.notices[0]
	if got.change.Event != event || got.account != account || got.tenant != tenant || !got.change.At.Equal(now) {
		test.Errorf("queued %+v, want %s for the account now", got, event)
	}
}

func TestAChangedPasswordIsTold(t *testing.T) {
	fixture := newPasswordFixture(now)
	told := &toldOf{}
	fixture.writer.WayIn = told

	if err := (ChangePassword{Writer: fixture.writer}).Execute(t.Context(), signedIn(),
		ChangePasswordCommand{Password: secret.New("seven blue lanterns"), StepUpToken: "hbt_stp_x"}); err != nil {
		t.Fatalf("the change was refused: %v", err)
	}
	told.only(t, domain.WayInPasswordChanged)
}

// A refused change changed nothing, and nothing is told.
func TestARefusedChangeIsNotTold(t *testing.T) {
	fixture := newPasswordFixture(now)
	told := &toldOf{}
	fixture.writer.WayIn = told
	fixture.stepUp.satisfied = false

	_ = (ChangePassword{Writer: fixture.writer}).Execute(t.Context(), signedIn(),
		ChangePasswordCommand{Password: secret.New("seven blue lanterns"), StepUpToken: "hbt_stp_x"})
	if len(told.notices) != 0 {
		t.Errorf("a refused change was told: %+v", told.notices)
	}
}

// An account without an address has no mailbox to be told in (ADR-0078 §7): only audited.
func TestAChangeOnAnAccountWithoutAnAddressIsOnlyAudited(t *testing.T) {
	fixture := newPasswordFixture(now)
	told := &toldOf{}
	fixture.writer.WayIn = told
	held := fixture.accounts.rows[account]
	held.Account.Email, held.Account.SignInName = "", "bert"
	fixture.accounts.rows[account] = held

	if err := (ChangePassword{Writer: fixture.writer}).Execute(t.Context(), signedIn(),
		ChangePasswordCommand{Password: secret.New("seven blue lanterns"), StepUpToken: "hbt_stp_x"}); err != nil {
		t.Fatalf("the change was refused: %v", err)
	}
	if len(told.notices) != 0 {
		t.Errorf("an account without an address was told: %+v", told.notices)
	}
	if len(fixture.audit.entries) != 1 || fixture.audit.entries[0].Action != PasswordChangedAction {
		t.Errorf("the trail holds %v", fixture.audit.entries)
	}
}

// Without a mail server the writer is not given a notifier: the change goes through, audited.
func TestAChangeWithoutAMailServerIsOnlyAudited(t *testing.T) {
	fixture := newPasswordFixture(now)
	if err := (ChangePassword{Writer: fixture.writer}).Execute(t.Context(), signedIn(),
		ChangePasswordCommand{Password: secret.New("seven blue lanterns"), StepUpToken: "hbt_stp_x"}); err != nil {
		t.Fatalf("the change was refused: %v", err)
	}
	if len(fixture.audit.entries) != 1 {
		t.Errorf("the trail holds %v", fixture.audit.entries)
	}
}

func TestAPasswordSetThroughAResetIsToldAsAReset(t *testing.T) {
	fixture := newResetFixture(now)
	told := &toldOf{}
	fixture.writer.WayIn = told
	token := fixture.mintedFor(t, now, domain.PendingReset)

	if _, err := (ResetPassword{Writer: fixture.writer}).Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("seven blue lanterns"),
	}); err != nil {
		t.Fatalf("the reset was refused: %v", err)
	}
	told.only(t, domain.WayInPasswordReset)
}

// The PASSWORD_CHANGE step is a change the person makes, told as one.
func TestAPasswordChangedAtTheChangeStepIsTold(t *testing.T) {
	fixture := newStepFixture(now)
	told := &toldOf{}
	fixture.passwords.writer.WayIn = told
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(30)}

	first := fixture.signsIn(t, "correct horse battery").Challenge
	if first == nil || first.Methods[0] != methodPasswordChange {
		t.Fatalf("the sign-in owes %+v, want the change step", first)
	}
	if _, err := (SetPasswordAndSignIn{Writer: fixture.passwords.writer}).Execute(t.Context(),
		SetPasswordAndSignInCommand{
			PendingToken: first.Token,
			Password:     secret.New("seven blue lanterns above the quiet harbour"),
		}); err != nil {
		t.Fatalf("the change step was refused: %v", err)
	}
	told.only(t, domain.WayInPasswordChanged)
}
