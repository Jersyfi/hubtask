// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/privacy"
)

// Consent is a record rather than a flag (E-10): not "is this allowed now" but "was it allowed
// then", which is what an operator has to be able to show.

func TestAWithdrawalIsRecordedEvenWhereNothingWasGranted(t *testing.T) {
	consent, err := privacy.NewWithdrawal(requestID, subjectID, " ai_processing ", "", now)
	if err != nil {
		t.Fatalf("withdrawing: %v", err)
	}

	if consent.Purpose != "ai_processing" {
		t.Errorf("the purpose came back as %q", consent.Purpose)
	}
	if consent.InForce() || !consent.Withdrawn() {
		t.Errorf("the record says %+v", consent)
	}
	// The person said no, and the record says they said no - rather than there being a gap where
	// an operator has to guess.
	if !consent.RevokedAt.Equal(now) || consent.Source != privacy.SourceUser {
		t.Errorf("the record came back as %+v", consent)
	}
}

func TestAPurposeIsRequiredAndBounded(t *testing.T) {
	for _, purpose := range []string{"", "   ", string(make([]byte, 200))} {
		if _, err := privacy.NewWithdrawal(requestID, subjectID, purpose, "", now); err == nil {
			t.Errorf("a withdrawal was recorded for the purpose %q", purpose)
		}
	}
}

func TestWithdrawingKeepsTheMomentProcessingStopped(t *testing.T) {
	standing := privacy.Consent{
		ID: requestID, AccountID: subjectID, Purpose: "metering",
		Granted: true, GrantedAt: now.Add(-24 * time.Hour),
	}
	if !standing.InForce() {
		t.Fatal("a granted consent is not in force")
	}

	withdrawn := standing.Withdraw(now)
	if withdrawn.InForce() || !withdrawn.RevokedAt.Equal(now) {
		t.Errorf("the withdrawal came out as %+v", withdrawn)
	}

	// A second withdrawal moves nothing: when the processing stopped is the fact being kept.
	again := withdrawn.Withdraw(now.Add(time.Hour))
	if !again.RevokedAt.Equal(now) {
		t.Errorf("a second withdrawal moved the moment to %s", again.RevokedAt)
	}
}
