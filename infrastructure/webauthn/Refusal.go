// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package webauthn is Hubtask's own verifier of WebAuthn ceremonies (ADR-0085, identity.md §23.3):
// the CBOR subset, the COSE keys of ES256, EdDSA and RS256, and the checks of WebAuthn Level 3 §7.1,
// on the standard library alone. It is the adapter behind core/port/webauthn; the core sees the
// verdict and the credential, never a byte of CBOR or JSON (rule 1).
package webauthn

import "errors"

// refusal is why a ceremony was refused, as a stable reason (`cbor.truncated`, `client_data.origin`):
// for a test to assert on and an operator to read in a log, never shown to a person - every refusal
// reaches them as one message code (rule 8). It carries no input byte, because a ceremony's bytes
// include the credential's identifier (rule 10).
type refusal struct{ reason string }

func (r refusal) Error() string { return "webauthn: " + r.reason }

func refuse(reason string) error { return refusal{reason: reason} }

// reasonOf answers the reason of a refusal, or "" for any other error.
func reasonOf(err error) string {
	var r refusal
	if errors.As(err, &r) {
		return r.reason
	}
	return ""
}
