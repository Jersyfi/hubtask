// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// PasswordHasher turns the local accounts' credential into its stored form and back into a
// yes/no (ADR-0005, T-02). Nothing here says Argon2id: the cost and the format are the adapter's,
// and the stored form carries its own parameters so the adapter can raise them without a
// migration.
type PasswordHasher interface {
	// Hash computes the stored form under the build's current cost, with a fresh salt.
	Hash(password secret.Secret) (string, error)

	// Verify compares a presented password against a stored hash. False is an answer, not a
	// failure; the error path is for a stored value the adapter cannot read.
	Verify(stored string, password secret.Secret) (bool, error)

	// VerifyDecoy burns the work a real verification would, against a hash of a secret nobody
	// knows. It is what "no such account" costs, so that it costs what "wrong password" costs
	// (T-02's constant shape).
	VerifyDecoy(password secret.Secret)
}

// SessionClaims is what a session access token says: which session is speaking, for which
// account, in which tenant, until when. Values from a verified token are trustworthy exactly
// because they were signed; what they do not say - is the session alive, may the account act -
// is the row's business.
type SessionClaims struct {
	TenantID  shared.ID
	SessionID shared.ID
	AccountID shared.ID
	ExpiresAt time.Time
}

// SessionTokenSigner mints and verifies the access half of the pair (security.md §5): fifteen
// minutes, verified by its signature without a database read. The token is never stored.
type SessionTokenSigner interface {
	// Issue mints the token for the claims.
	Issue(claims SessionClaims) string

	// Validate judges a presented token. Every forgery is one indistinguishable refusal; an
	// expired token is the one distinguished answer, because it is a token this system really
	// minted and "refresh" is actionable where "invalid" is not.
	Validate(presented string, now time.Time) (SessionClaims, error)
}
