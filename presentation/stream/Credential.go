// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package stream

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// fingerprintLength is how much of the digest is kept. 16 hex digits is 64 bits: far too much to
// collide across the connections of one process, and far too little to attack the token behind it.
const fingerprintLength = 16

// Credential is the per-credential key a connection is counted under.
//
// Here rather than in either adapter, and that is not tidiness: the per-credential cap only means
// anything if both streams key a credential the same way. An agent and a browser presenting the
// same token have to land in the same counter, or a client could double its allowance by opening
// half its connections at the other endpoint.
//
// A fingerprint rather than the credential itself, for the rate limiter's reason: the map ends up
// in a heap dump, and a heap dump with live tokens in it is a second incident on top of the first
// (rule 10, security.md §9). Plain SHA-256 without a pepper is enough - the value never leaves the
// process and is never compared against anything stored, so the only property that matters is that
// two different tokens land in two different counters.
//
// A request with no bearer credential answers empty, which the registry reads as "do not count this
// one per credential". That is the right answer rather than a hole: every stream in this product is
// behind authentication, so an empty key means a shape the caller has already refused.
func Credential(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:fingerprintLength]
}
