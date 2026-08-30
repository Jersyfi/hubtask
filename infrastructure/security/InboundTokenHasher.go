// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/hmac"
	"crypto/sha256"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// inboundPepperInfo separates this use of the installation secret from every other one (G-08).
//
// The label is the whole of the separation security.md §5 asks for: the same key protects access
// tokens, calendar feeds, signed cursors and media tokens, and deriving each purpose through its
// own label means a stored inbound hash can never match a value produced for any of them - so a
// leak of `automation_rule` cannot be replayed as a credential of another kind.
const inboundPepperInfo = "hubtask/automation-inbound/v1"

// InboundTokenHasher turns a presented inbound token into the value stored in
// `automation_rule.inbound_token_hash`.
//
// FeedTokenHasher's construction, for FeedTokenHasher's reasons: HMAC-SHA-256 with a pepper that
// lives in the environment rather than in the database, so a stolen dump has nothing to attack
// offline; no per-token salt and no Argon2, because a 256-bit random secret has nothing to guess
// at and this runs on every delivery, where a deliberately slow hash would be the rate limit.
//
// It is deterministic, which is what makes the lookup an index seek on a unique index - and
// therefore what makes it constant-time in the only sense that matters here: the server never
// compares a stored secret against a presented one at all.
type InboundTokenHasher struct {
	pepper []byte
}

func NewInboundTokenHasher(installationSecret secret.Secret) InboundTokenHasher {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(inboundPepperInfo))
	return InboundTokenHasher{pepper: mac.Sum(nil)}
}

// Hash covers the whole presented string, tenant half included, so a token cannot be rewritten to
// name another tenant and still match.
func (h InboundTokenHasher) Hash(presented string) []byte {
	mac := hmac.New(sha256.New, h.pepper)
	mac.Write([]byte(presented))
	return mac.Sum(nil)
}
