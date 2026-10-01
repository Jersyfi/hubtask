// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "github.com/Jersyfi/hubtask/core/domain/model/shared"

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
