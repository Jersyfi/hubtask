// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// stepUpStore is StepUps in memory, with the statement's guards written out: one live proof per
// session, judged and burned atomically.
type stepUpStore struct {
	byToken map[string]stepUpRow
}

type stepUpRow struct {
	sessionID shared.ID
	accountID shared.ID
	method    domain.StepUpMethod
	at        time.Time
	consumed  bool
}

func newStepUps() *stepUpStore { return &stepUpStore{byToken: map[string]stepUpRow{}} }

func (s *stepUpStore) Record(
	_ context.Context, sessionID, accountID shared.ID,
	presented domain.Token, method domain.StepUpMethod, at time.Time,
) (bool, error) {
	// Replacing whatever stood on the session, the UPDATE's own behaviour.
	for token, row := range s.byToken {
		if row.sessionID == sessionID {
			delete(s.byToken, token)
		}
	}
	s.byToken[presented.Secret()] = stepUpRow{
		sessionID: sessionID, accountID: accountID, method: method, at: at,
	}
	return true, nil
}

func (s *stepUpStore) Consume(
	_ context.Context, presented domain.Token, accountID shared.ID,
	cutoff, _ time.Time,
) (domain.StepUpMethod, bool, error) {
	row, ok := s.byToken[presented.Secret()]
	if !ok || row.accountID != accountID || row.consumed || row.at.Before(cutoff) {
		return row.method, false, nil
	}
	row.consumed = true
	s.byToken[presented.Secret()] = row
	return row.method, true, nil
}

// stepUpFixture is the MFA fixture with a live session behind the actor, which a step-up lands
// on.
func stepUpFixture(at time.Time) *sessionFixture {
	fixture := mfaFixture(at)
	fixture.writer.StepUps = newStepUps()
	credential, _ := refreshCredential(at)
	fixture.sessions.sessions[sessionRowID] = repository.SessionCredential{
		Session: credential.Session, Account: credential.Account,
	}
	fixture.withAccount("bert@example.org", "correct horse battery")
	return fixture
}

func TestAPasswordStepUpLandsOnTheSession(t *testing.T) {
	fixture := stepUpFixture(now)

	grant, err := StepUp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{Password: secret.New("correct horse battery")})
	if err != nil {
		t.Fatalf("stepping up: %v", err)
	}
	if !strings.HasPrefix(grant.Token.Reveal(), domain.StepUpTokenPrefix) {
		t.Errorf("token %q lacks its prefix", grant.Token.Reveal())
	}
	if grant.Method != domain.StepUpPassword {
		t.Errorf("method %q, want PASSWORD", grant.Method)
	}
	if !grant.ExpiresAt.After(now) {
		t.Errorf("expiry %v is not in the future", grant.ExpiresAt)
	}

	audited := false
	for _, entry := range fixture.audit.entries {
		if entry.Action == StepUpAction {
			audited = true
		}
	}
	if !audited {
		t.Error("the proof was not audited")
	}
}

func TestATotpStepUpUsesTheArmedFactor(t *testing.T) {
	fixture := stepUpFixture(now)
	material := enrolled(t, fixture)

	later := now.Add(domain.TotpStepSeconds * time.Second)
	writer := fixture.writer
	writer.Clock = clock.Fixed(later)
	grant, err := StepUp{Writer: writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{Code: domain.TotpCode(material, domain.TotpStep(later))})
	if err != nil {
		t.Fatalf("stepping up with a code: %v", err)
	}
	if grant.Method != domain.StepUpTotp {
		t.Errorf("method %q, want TOTP", grant.Method)
	}
}

func TestAStepUpRefusesTheWrongProof(t *testing.T) {
	fixture := stepUpFixture(now)
	handler := StepUp{Writer: fixture.writer}

	_, err := handler.Execute(t.Context(), signedInActor(),
		StepUpCommand{Password: secret.New("a wrong password!!")})
	if err == nil || !strings.Contains(err.Error(), "auth.sign_in_failed") {
		t.Fatalf("a wrong password answered %v", err)
	}
	if got := fixture.attempts.standing[stepUpSubject(account)].Failures; got != 1 {
		t.Errorf("the ledger stands at %d, want 1", got)
	}

	// Neither or both methods is a refusal before any credential is examined.
	if _, err := handler.Execute(t.Context(), signedInActor(), StepUpCommand{}); err == nil ||
		!strings.Contains(err.Error(), "auth.step_up_method_required") {
		t.Fatalf("an empty proof answered %v", err)
	}
	if _, err := handler.Execute(t.Context(), signedInActor(), StepUpCommand{
		Password: secret.New("x"), Code: "123456",
	}); err == nil || !strings.Contains(err.Error(), "auth.step_up_method_required") {
		t.Fatalf("both methods answered %v", err)
	}
}

// A step-up is a session's act: a personal access token has no session and no person to ask.
func TestAStepUpRefusesANonSessionActor(t *testing.T) {
	fixture := stepUpFixture(now)
	patActor := signedInActor()
	patActor.TokenID = refreshRowID // a token row's identifier names no session

	_, err := StepUp{Writer: fixture.writer}.Execute(t.Context(), patActor,
		StepUpCommand{Password: secret.New("correct horse battery")})
	if !errors.Is(err, shared.ErrForbidden) || !strings.Contains(err.Error(), "auth.step_up_session_required") {
		t.Fatalf("a token actor answered %v", err)
	}
}

// The verifier: one proof covers one privileged action, by the clock and by consumption.
func TestTheVerifierConsumesOnceAndExpiresByTheClock(t *testing.T) {
	fixture := stepUpFixture(now)
	grant, err := StepUp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{Password: secret.New("correct horse battery")})
	if err != nil {
		t.Fatalf("stepping up: %v", err)
	}
	verifier := StepUpVerifier{Writer: fixture.writer}

	// Somebody else's proof is nobody's.
	if ok, err := verifier.Satisfied(t.Context(), refreshRowID, grant.Token.Reveal()); err != nil || ok {
		t.Fatalf("another account satisfied (%v, %v)", ok, err)
	}
	// The holder's, once.
	ok, err := verifier.Satisfied(t.Context(), account, grant.Token.Reveal())
	if err != nil || !ok {
		t.Fatalf("the first consumption answered (%v, %v)", ok, err)
	}
	// And never twice: a consumed step-up does not cover a second privileged action.
	if ok, err := verifier.Satisfied(t.Context(), account, grant.Token.Reveal()); err != nil || ok {
		t.Fatalf("the second consumption answered (%v, %v)", ok, err)
	}

	// A fresh proof, presented after the window, is stale by the clock.
	grant, err = StepUp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{Password: secret.New("correct horse battery")})
	if err != nil {
		t.Fatalf("stepping up again: %v", err)
	}
	late := fixture.writer
	late.Clock = clock.Fixed(now.Add(late.stepUpWindow() + time.Second))
	if ok, err := (StepUpVerifier{Writer: late}).Satisfied(
		t.Context(), account, grant.Token.Reveal()); err != nil || ok {
		t.Fatalf("a stale proof satisfied (%v, %v)", ok, err)
	}

	// Garbage is "not proved", never an error.
	if ok, err := verifier.Satisfied(t.Context(), account, "not-a-token"); err != nil || ok {
		t.Fatalf("garbage answered (%v, %v)", ok, err)
	}
}

// The demand reaches the operations security.md §5 names: an OWNER grant without a proof is
// refused with the one demand, and with a proof it passes.
func TestAnOwnerGrantDemandsTheProof(t *testing.T) {
	fixture := stepUpFixture(now)
	grant := GrantMembership{
		Grants: newGrants(), Accounts: fixture.writer.People.(*accountStore),
		Groups: &groupStore{}, Authorizer: &authorizer{},
		Audit: fixture.audit, UnitOfWork: fixture.work,
		Clock: clock.Fixed(now), IDs: &idSequence{},
		StepUp: StepUpVerifier{Writer: fixture.writer},
	}

	_, err := grant.Execute(t.Context(), sessionActor(membersWrite), GrantMembershipCommand{
		AccountID: account, Scope: domain.TenantScope(), Role: domain.RoleOwner,
	})
	if err == nil || !strings.Contains(err.Error(), stepup.CodeRequired) {
		t.Fatalf("an OWNER grant without a proof answered %v", err)
	}

	proof, err := StepUp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{Password: secret.New("correct horse battery")})
	if err != nil {
		t.Fatalf("stepping up: %v", err)
	}
	if _, err := grant.Execute(t.Context(), sessionActor(membersWrite), GrantMembershipCommand{
		AccountID: account, Scope: domain.TenantScope(), Role: domain.RoleOwner,
		StepUpToken: proof.Token.Reveal(),
	}); err != nil {
		t.Fatalf("an OWNER grant with the proof answered %v", err)
	}

	// A MEMBER grant asks for nothing.
	if _, err := grant.Execute(t.Context(), sessionActor(membersWrite), GrantMembershipCommand{
		AccountID: account, Scope: domain.TenantScope(), Role: domain.RoleMember,
	}); err != nil {
		t.Fatalf("a MEMBER grant answered %v", err)
	}
}
