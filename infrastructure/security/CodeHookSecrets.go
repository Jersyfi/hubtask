// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// codeHookPepperInfo separates a forge hook's secrets from every other value the installation key
// protects (security.md §5).
const codeHookPepperInfo = "hubtask/code-hook/v1"

// CodeHookSecrets derives a forge hook's secret rather than storing it: HMAC-SHA-256 under a
// pepper of its own, over the workspace, the hook and the hook's salt. A forge needs the secret
// itself to sign with, so a hash would not do, and a sealed value would be one more store to
// re-seal; derived, a dump of the database opens no hook, and a new salt replaces the secret.
type CodeHookSecrets struct {
	pepper []byte
}

// NewCodeHookSecrets answers the deriver under the installation's key.
func NewCodeHookSecrets(installationSecret secret.Secret) CodeHookSecrets {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(codeHookPepperInfo))
	return CodeHookSecrets{pepper: mac.Sum(nil)}
}

// Derive answers the hook's secret, 64 hexadecimal characters.
func (s CodeHookSecrets) Derive(tenantID, hookID shared.ID, salt string) string {
	mac := hmac.New(sha256.New, s.pepper)
	mac.Write([]byte(tenantID.String() + "/" + hookID.String() + "/" + salt))
	return "hbt_hook_" + hex.EncodeToString(mac.Sum(nil))
}
