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
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CodeRequired is the demand every privileged operation surfaces without a satisfied step-up:
// 403, naming the accepted methods, so a client knows to ask the person rather than to retry
// (H-03).
const CodeRequired = "auth.step_up_required"

// Method is one way a person proves themselves afresh, as `POST /auth/step-up` takes it.
type Method string

const (
	// MethodPassword is the proof every account with a password can give.
	MethodPassword Method = "PASSWORD"
	// MethodTotp is the authenticator's code, offered only where a factor is armed.
	MethodTotp Method = "TOTP"
)

// Required is the demand itself, minted in exactly one place, naming the methods this account can
// answer with - space-separated, in the order given, the way the contract describes the
// parameter. A demand that names TOTP to an account without a factor sends the person to a
// prompt for a code they cannot produce (issue 544), which is why the list is the account's and
// not the endpoint's.
func Required(methods ...Method) error {
	if len(methods) == 0 {
		methods = []Method{MethodPassword}
	}
	names := make([]string, 0, len(methods))
	for _, method := range methods {
		names = append(names, string(method))
	}
	return shared.ErrForbidden.
		WithDetail(CodeRequired).
		WithParams(map[string]string{"methods": strings.Join(names, " ")})
}

// Demand is the check every privileged operation runs: a wired verifier, a presented token, a
// satisfied proof - or the one refusal. A nil or unavailable verifier refuses rather than
// permits, because a destructive mode permitted by omission is the failure E-06 built this seam
// against; it names both methods, because nothing can look the account's up.
func Demand(
	ctx context.Context, verifier Verifier, tenantID, accountID shared.ID, token string,
) error {
	if verifier == nil || !verifier.Available() {
		return Required(MethodPassword, MethodTotp)
	}
	if token == "" {
		return refuse(ctx, verifier, tenantID, accountID)
	}
	satisfied, err := verifier.Satisfied(ctx, accountID, token)
	if err != nil {
		return err
	}
	if !satisfied {
		return refuse(ctx, verifier, tenantID, accountID)
	}
	return nil
}

// refuse is the demand with the account's own methods in it.
func refuse(ctx context.Context, verifier Verifier, tenantID, accountID shared.ID) error {
	methods, err := verifier.Methods(ctx, tenantID, accountID)
	if err != nil {
		return err
	}
	return Required(methods...)
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

	// Methods answers which proofs this account can give, for the refusal to name: the password
	// always, the code where a factor is armed. Asked only when a demand is about to refuse.
	Methods(ctx context.Context, tenantID, accountID shared.ID) ([]Method, error)
}
