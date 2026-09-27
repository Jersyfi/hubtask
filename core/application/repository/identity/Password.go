// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// What a password's life needs of the store (ADR-0068 §5).
//
// Every hash travels as a secret, `SignInAccount`'s discipline: the application layer holds one
// only long enough to hand it to the verifier, and no way of printing it yields anything (rule 10).

// PasswordAccount is what setting or judging a password needs to know about the account.
type PasswordAccount struct {
	Account identity.Account
	// PasswordHash is empty for an account that signs in some other way. Setting one is then a
	// first password rather than a change, which is a difference the use case cares about and the
	// store does not.
	PasswordHash secret.Secret
	// PasswordSetAt is when the hash was last written. Zero where it is unknown - an account with
	// no password, or a row written before the column existed - and an unknown moment is not
	// treated as an expired one, because locking somebody out over a missing date would be the
	// product's mistake and not theirs.
	PasswordSetAt time.Time
}

// PasswordAccounts is the account surface of a password's life.
type PasswordAccounts interface {
	// FindForPassword answers one account with its hash and the moment. An error wrapping
	// shared.ErrNotFound where the account is gone.
	FindForPassword(ctx context.Context, accountID shared.ID) (PasswordAccount, error)

	// FindByEmailForPassword answers the account an address names, for the reset that must answer
	// the same thing whether or not it found one. An error wrapping shared.ErrNotFound otherwise -
	// which the caller turns into the same `202` a found account gets (T-02).
	FindByEmailForPassword(ctx context.Context, email string) (PasswordAccount, error)

	// SetPassword writes the hash and the moment together, so that no reader can see one without
	// the other. False means the account is not there to write - already deleted, or never was.
	SetPassword(ctx context.Context, accountID shared.ID, hash string, at time.Time) (bool, error)
}

// PasswordHistories is the last few hashes of an account (ADR-0068 §1).
//
// Hashes and moments, nothing else: no plaintext, and nothing that says anything about the shape of
// one. The depth is the policy's and the cap is the domain's, which is why neither is in the schema.
type PasswordHistories interface {
	// Recent answers the newest `count` hashes, newest first.
	Recent(ctx context.Context, accountID shared.ID, count int) ([]secret.Secret, error)

	// Append records a hash that has just stopped being current.
	Append(ctx context.Context, id, accountID shared.ID, hash string, at time.Time) error

	// Trim drops everything past the newest `keep`, in the same transaction as the append - so the
	// table cannot grow past the policy even if the policy is lowered.
	Trim(ctx context.Context, accountID shared.ID, keep int) error
}
