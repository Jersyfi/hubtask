// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package security holds the cryptographic primitives. Nothing here is home-grown: every
// construction is one from the standard library, used the way its documentation says
// (security.md §8).
package security

import (
	"crypto/hmac"
	"crypto/sha256"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// pepperInfo separates this use of the installation secret from every other one. The same key
// also protects signed cursors and feed tokens; deriving each purpose through its own label means
// a hash from one can never be replayed as a value of another.
const pepperInfo = "hubtask/access-token/v1"

// TokenHasher turns a presented token into the value stored in access_token.token_hash.
//
// HMAC-SHA-256 with the installation secret as the key, which is what "SHA-256 with a server-side
// pepper" means in practice (security.md §8): the pepper lives in the environment, never in the
// database, so a stolen database dump cannot be brute-forced offline. No salt per token and no
// Argon2 here on purpose - a 256-bit random secret has nothing to guess at, and this runs on
// every single request, where a deliberately slow hash would be the rate limit.
type TokenHasher struct {
	pepper []byte
}

func NewTokenHasher(installationSecret secret.Secret) TokenHasher {
	// The derivation exists so that the raw configuration value is not itself the key of every
	// construction in the system. HMAC is a pseudo-random function, so its output is a fine key.
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(pepperInfo))
	return TokenHasher{pepper: mac.Sum(nil)}
}

// Hash is deterministic, which is the point: the lookup is an index seek on the hash rather than
// a scan comparing every row. The hash covers the whole presented string, tenant half included,
// so a token cannot be rewritten to name another tenant and still match.
func (h TokenHasher) Hash(presented string) []byte {
	mac := hmac.New(sha256.New, h.pepper)
	mac.Write([]byte(presented))
	return mac.Sum(nil)
}
