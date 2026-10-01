// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// resetFixture is the password fixture with the session machinery a reset needs to answer with.
type resetFixture struct {
	*passwordFixture
	pending *pendingStore
	session *sessionFixture
}

func newResetFixture(at time.Time) *resetFixture {
	passwords := newPasswordFixture(at)
	sessions := newSessionFixture(at)
	pending := newPending()

	sessions.writer.Pending = pending
	passwords.writer.Session = sessions.writer
	passwords.writer.Pending = pending
	// The session machinery is the sign-in fixture's - opening one is what a reset answers with -
	// and the stores this fixture asserts on stay the password fixture's, so that one test reads
	// one set of rows.
	passwords.writer.Session.Sessions = passwords.sessions
	passwords.writer.Session.Audit = passwords.audit
	passwords.writer.Session.Passwords = &passwordsFake{}

	return &resetFixture{passwordFixture: passwords, pending: pending, session: sessions}
}

// mintedFor puts a live reset credential in the store and answers the plaintext token.
func (f *resetFixture) mintedFor(t *testing.T, at time.Time, purpose domain.PendingPurpose) string {
	t.Helper()
	token, err := domain.NewPendingToken(tenant, make([]byte, domain.TokenSecretBytes))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if err := f.pending.Insert(t.Context(), domain.PendingCredential{
		ID: resetRowID, TenantID: tenant, AccountID: account, Purpose: purpose,
		CreatedAt: at, ExpiresAt: at.Add(domain.ResetLifetime),
	}, token); err != nil {
		t.Fatalf("inserting: %v", err)
	}
	return token.Secret()
}

const resetRowID = shared.ID("018f2a1b-0000-7000-8000-00000000cc01")

// The link sets the password, ends every session, and answers a session of its own.
func TestAResetSetsThePasswordAndOpensASession(t *testing.T) {
	fixture := newResetFixture(now)
	token := fixture.mintedFor(t, now, domain.PendingReset)
	fixture.sessions.listed = []domain.Session{
		{ID: sessionRowID, TenantID: tenant, AccountID: account},
	}

	result, err := ResetPassword{Writer: fixture.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("seven blue lanterns"),
	})
	if err != nil {
		t.Fatalf("the reset was refused: %v", err)
	}
	if result.Pair == nil {
		t.Fatal("no session answered")
	}
	if len(fixture.accounts.writes) != 1 {
		t.Fatalf("%d passwords written", len(fixture.accounts.writes))
	}
	// Every session, this time: the person asking has none of their own to keep.
	if len(fixture.sessions.revoked) != 1 || fixture.sessions.revoked[0] != sessionRowID {
		t.Errorf("sessions ended: %v, want all of them", fixture.sessions.revoked)
	}
}

// UC-ID-04 check 5: a reset of an account with a second factor opens no session - it answers the
// code step - and the step says whose account it is, because the person arrived from a mail link
// and typed no address the card could show.
func TestAResetOfAnAccountWithAFactorAnswersTheCodeStepAndWhoseItIs(t *testing.T) {
	fixture := newResetFixture(now)
	armed := newEnrollments()
	armed.rows[account] = &repository.MfaEnrollment{AccountID: account, ConfirmedAt: now.Add(-time.Hour)}
	fixture.writer.Session.Enrollments = armed
	token := fixture.mintedFor(t, now, domain.PendingReset)

	result, err := ResetPassword{Writer: fixture.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("seven blue lanterns"),
	})
	if err != nil {
		t.Fatalf("the reset was refused: %v", err)
	}
	if result.Pair != nil {
		t.Fatal("a reset opened a session past the account's second factor")
	}
	challenge := result.Challenge
	if challenge == nil || len(challenge.Methods) != 2 || challenge.Methods[0] != methodTotp {
		t.Fatalf("the reset answered %+v, want the code step", challenge)
	}
	if challenge.Email != "bert@example.org" {
		t.Errorf("the step names %q, want the account's address for the identity line", challenge.Email)
	}
	if challenge.ExpiresAt.IsZero() {
		t.Error("the step carries no end the card could count down to")
	}
}

// The link works once. A second use is the same refusal an unknown one gets.
func TestAResetLinkWorksExactlyOnce(t *testing.T) {
	fixture := newResetFixture(now)
	token := fixture.mintedFor(t, now, domain.PendingReset)

	if _, err := (ResetPassword{Writer: fixture.writer}).Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("seven blue lanterns"),
	}); err != nil {
		t.Fatalf("the first use was refused: %v", err)
	}

	_, second := ResetPassword{Writer: fixture.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("eight green lanterns"),
	})
	if !errors.Is(second, shared.ErrUnauthenticated) {
		t.Fatalf("the second use answered %v", second)
	}
}

// Unknown, spent and expired are one refusal, byte for byte.
func TestTheResetRefusalsAreOneAnswer(t *testing.T) {
	unknown := newResetFixture(now)
	stranger, err := domain.NewPendingToken(tenant, make([]byte, domain.TokenSecretBytes))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	_, unknownErr := ResetPassword{Writer: unknown.writer}.Execute(t.Context(),
		ResetPasswordCommand{
			Token: secret.New(stranger.Secret()), Password: secret.New("seven blue lanterns"),
		})

	expired := newResetFixture(now)
	token := expired.mintedFor(t, now.Add(-2*domain.ResetLifetime), domain.PendingReset)
	expired.writer.Clock = clock.Fixed(now)
	_, expiredErr := ResetPassword{Writer: expired.writer}.Execute(t.Context(),
		ResetPasswordCommand{
			Token: secret.New(token), Password: secret.New("seven blue lanterns"),
		})

	if unknownErr == nil || expiredErr == nil {
		t.Fatalf("one of them was accepted: %v / %v", unknownErr, expiredErr)
	}
	if unknownErr.Error() != expiredErr.Error() {
		t.Errorf("an unknown link answers %q and an expired one %q", unknownErr, expiredErr)
	}
}

// A credential of another purpose completes nothing here: an enrolment token is not a reset.
func TestACredentialOfAnotherPurposeIsNotAReset(t *testing.T) {
	fixture := newResetFixture(now)
	token := fixture.mintedFor(t, now, domain.PendingTotp)

	_, err := ResetPassword{Writer: fixture.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("seven blue lanterns"),
	})

	if !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("a TOTP credential reset a password: %v", err)
	}
}

// A password the rule refuses is refused before the link is spent, so the person can try again
// with the same mail.
func TestARefusedPasswordLeavesTheLinkUnspent(t *testing.T) {
	fixture := newResetFixture(now)
	token := fixture.mintedFor(t, now, domain.PendingReset)

	if _, err := (ResetPassword{Writer: fixture.writer}).Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("short"),
	}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a short password answered %v", err)
	}

	if _, err := (ResetPassword{Writer: fixture.writer}).Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("seven blue lanterns"),
	}); err != nil {
		t.Fatalf("the link was spent by the refusal: %v", err)
	}
}
