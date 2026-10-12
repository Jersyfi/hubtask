// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// auditFingerprintInfo separates this use of the installation secret from every other one, as
// pepperInfo does for access tokens: a fingerprint can never equal a token hash or a signature.
const auditFingerprintInfo = "hubtask/audit-fingerprint/v1"

// AuditFingerprintScheme prefixes every fingerprint this type writes. A fingerprint without it
// was written unkeyed by an earlier version, and the two must never be read as two values.
const AuditFingerprintScheme = "hmac-sha256:"

// AuditFingerprinter is the audit trail's keyed fingerprint (audit.md §4, security.md §8).
//
// HMAC-SHA-256 under a pepper derived from the installation secret, over the workspace and the
// value. Not a password hash and not meant to be one: the point is that two entries of one
// workspace compare without either being readable, and a guess at a name cannot be checked
// without the secret, which lives in the environment and never in the database.
type AuditFingerprinter struct {
	pepper []byte
}

var _ audit.Fingerprinter = AuditFingerprinter{}

func NewAuditFingerprinter(installationSecret secret.Secret) AuditFingerprinter {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(auditFingerprintInfo))
	return AuditFingerprinter{pepper: mac.Sum(nil)}
}

// Fingerprint is deterministic within one workspace and one secret, which is the whole of its
// comparability. The zero byte after the workspace keeps the two parts from running into each
// other; the identifier's text form has a fixed length, so it is a separator, not an escape.
func (f AuditFingerprinter) Fingerprint(tenantID shared.ID, value string) string {
	mac := hmac.New(sha256.New, f.pepper)
	mac.Write([]byte(tenantID.String()))
	mac.Write([]byte{0})
	mac.Write([]byte(value))
	return AuditFingerprintScheme + hex.EncodeToString(mac.Sum(nil))
}
