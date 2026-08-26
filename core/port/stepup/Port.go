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
package stepup

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

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
