// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// recoveryFixture is the MFA writer with an armed enrolment and a step-up that can be refused.
func newRecoveryFixture(t *testing.T) *sessionFixture {
	t.Helper()
	fixture := newSessionFixture(now)
	enrollments := newEnrollments()
	enrollments.rows[account] = &repository.MfaEnrollment{
		AccountID: account, ConfirmedAt: now.Add(-time.Hour),
	}
	fixture.writer.Enrollments = enrollments
	fixture.writer.Recovery = newRecovery()
	fixture.writer.People = newAccounts(domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountUser,
		Email: "bert@example.org", DisplayName: "Bert", Status: domain.AccountActive,
	})
	// A live proof on the caller's own session, which is what the verifier consumes.
	stepUps := newStepUps()
	token, err := domain.NewStepUpToken(tenant, make([]byte, domain.TokenSecretBytes))
	if err != nil {
		t.Fatalf("minting a proof: %v", err)
	}
	if _, err := stepUps.Record(t.Context(), sessionRowID, account, token,
		domain.StepUpPassword, now); err != nil {
		t.Fatalf("recording a proof: %v", err)
	}
	fixture.writer.StepUps = stepUps
	fixture.writer.StepUpWindow = 5 * time.Minute
	fixture.proof = token.Secret()
	return fixture
}

// The old ten stop working the moment the new ten are answered.
func TestRegeneratingBurnsTheOldSetAndAnswersATenNew(t *testing.T) {
	fixture := newRecoveryFixture(t)

	fresh, err := RegenerateRecoveryCodes{Writer: fixture.writer}.Execute(t.Context(),
		recoveryActor(), fixture.proof)
	if err != nil {
		t.Fatalf("regenerating: %v", err)
	}

	if len(fresh.Codes) != domain.RecoveryCodeCount {
		t.Fatalf("%d codes answered, want %d", len(fresh.Codes), domain.RecoveryCodeCount)
	}
	store, _ := fixture.writer.Recovery.(*recoveryStore)
	if store.replacements != 1 {
		t.Errorf("%d replacements, want one - the burn and the write are one act", store.replacements)
	}
	if len(fixture.audit.entries) != 1 ||
		fixture.audit.entries[0].Action != RecoveryRegeneratedAction {
		t.Errorf("the trail holds %v", fixture.audit.entries)
	}
}

// Without a step-up it is refused: a fresh set of codes is a fresh set of ways in.
func TestRegeneratingDemandsAStepUp(t *testing.T) {
	fixture := newRecoveryFixture(t)

	if _, err := (RegenerateRecoveryCodes{Writer: fixture.writer}).Execute(t.Context(),
		recoveryActor(), ""); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a call without a proof answered %v", err)
	}
}

// An unconfirmed enrolment has its codes on a screen somebody is still reading. Replacing them
// there would leave that screen lying.
func TestRegeneratingRefusesAnUnconfirmedEnrolment(t *testing.T) {
	fixture := newRecoveryFixture(t)
	enrollments, _ := fixture.writer.Enrollments.(*enrollmentsStore)
	enrollments.rows[account].ConfirmedAt = time.Time{}

	_, err := RegenerateRecoveryCodes{Writer: fixture.writer}.Execute(t.Context(),
		recoveryActor(), fixture.proof)

	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("an unconfirmed enrolment answered %v", err)
	}
}

func recoveryActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account, TokenID: sessionRowID,
	}
}
