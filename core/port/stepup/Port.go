// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package stepup is the seam a destructive act asks for a second, stronger proof of identity
// through (E-06, backup-restore.md §8.3, security.md §5).
//
// It exists before anything can satisfy it, and that is the point rather than an accident.
// Sessions and multi-factor authentication arrive in `0.6.0`; a restore that replaces a tenant is
// being built now. The choice was between letting the destructive modes proceed with the
// confirmation skipped - a promise the documents make and the code does not keep - and defining
// where the proof arrives, refusing without it, and saying plainly that this installation cannot
// produce one yet.
//
// A confirmation that is structurally impossible to give is a stronger position than one that is
// skipped: the refusal is visible, it names its own reason, and the day an installation can issue
// a step-up the mode starts working without anything here changing shape.
//
// That day was H-03: the verifier exists - a fresh re-authentication on the current session,
// consumed by the one privileged action it is presented to - and the refusal every demanding
// operation answers without one lives here, so that two operations cannot describe the same
// demand differently.
package stepup

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CodeRequired is the demand every privileged operation surfaces without a satisfied step-up:
// 403, naming the accepted methods, so a client knows to ask the person rather than to retry
// (H-03).
const CodeRequired = "auth.step_up_required"

// Required is the demand itself, minted in exactly one place.
func Required() error {
	return shared.ErrForbidden.
		WithDetail(CodeRequired).
		WithParams(map[string]string{"methods": "PASSWORD TOTP"})
}

// Demand is the check every privileged operation runs: a wired verifier, a presented token, a
// satisfied proof - or the one refusal. A nil or unavailable verifier refuses rather than
// permits, because a destructive mode permitted by omission is the failure E-06 built this seam
// against.
func Demand(ctx context.Context, verifier Verifier, accountID shared.ID, token string) error {
	if verifier == nil || !verifier.Available() || token == "" {
		return Required()
	}
	satisfied, err := verifier.Satisfied(ctx, accountID, token)
	if err != nil {
		return err
	}
	if !satisfied {
		return Required()
	}
	return nil
}

// Verifier judges the proof.
type Verifier interface {
	// Available reports whether this installation can ask anybody for a step-up at all.
	//
	// Separate from Satisfied, because the two failures are different problems with different
	// fixes: "you did not prove it" is something the caller can act on, and "nothing here can
	// prove it" is something only the operator can. A single boolean would collapse them into one
	// message that is wrong for one of the two.
	Available() bool

	// Satisfied reports whether the token proves a fresh, stronger authentication of this
	// account. It is asked only when Available says yes.
	//
	// The token is opaque here on purpose: what proves a step-up is an authentication decision,
	// and the application layer's business is whether one was made rather than how.
	Satisfied(ctx context.Context, accountID shared.ID, token string) (bool, error)
}
