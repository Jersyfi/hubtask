// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The step-up (H-03, security.md §5): a fresh re-authentication on the current session,
// recorded there, valid for a configured window, consumed by the one privileged action it is
// presented to.

// StepUpTokenPrefix marks the proof, with the session tokens' reasoning.
const StepUpTokenPrefix = "hbt_sup_" //nolint:gosec // G101: a public format marker, not a credential

// StepUpMethod is what proved it - recorded in the audit trail, never the credential.
type StepUpMethod string

// The four of ADR-0075 §1, each offered only where the account holds it.
const (
	StepUpPassword StepUpMethod = "PASSWORD"
	StepUpTotp     StepUpMethod = "TOTP"
	// StepUpRecovery is a recovery code, consumed by the step-up exactly as by a sign-in.
	StepUpRecovery StepUpMethod = "RECOVERY"
	// StepUpProvider is a fresh sign-in at the provider the account is connected to.
	StepUpProvider StepUpMethod = "PROVIDER"
)

// ParseStepUpToken and NewStepUpToken are the proof's shape, ParseToken's discipline.
func ParseStepUpToken(raw string) (Token, error) { return parsePrefixed(raw, StepUpTokenPrefix) }

func NewStepUpToken(tenantID shared.ID, secret []byte) (Token, error) {
	return newPrefixed(StepUpTokenPrefix, tenantID, secret)
}

// ProviderClockSkew is how far a provider's clock may run ahead of this one before a moment it names
// is not believed - the minute T-13 allows an ID token's own times.
const ProviderClockSkew = time.Minute

// ProviderProofFresh reports whether a provider's word that the person signed in at authTime proves
// a step-up at now (ADR-0075 §2): the moment is named, it lies inside the step-up's own window, and
// it is not further ahead than a clock explains. A zero authTime is a provider that did not say -
// which ignored `max_age`, or never sends `auth_time` - and proves nothing fresh, however recent
// the round trip was.
func ProviderProofFresh(authTime, now time.Time, window time.Duration) bool {
	if authTime.IsZero() {
		return false
	}
	if authTime.After(now.Add(ProviderClockSkew)) {
		return false
	}
	return now.Sub(authTime) <= window
}
