// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package crypto is the adapter behind core/port/crypto: AES-256-GCM envelope encryption over a
// keyring the environment supplies, and Argon2id where a key has to come from a passphrase
// (backup-restore.md §4, security.md §3).
//
// It is the only package in this system that names a cipher. Everything inwards of it sees the
// port, which is what makes open point S-2 - where the master key lives once somebody else
// operates the installation - a second adapter rather than a rewrite.
package crypto

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
	"regexp"
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// masterKeyInfo separates this use of a configured secret from every other one, the way
// security.md §5 asks: the key identifier is part of the label, so two keys configured with the
// same material are still two different keys, and material that also protects something else
// cannot produce a value this package would accept.
const masterKeyInfo = "hubtask/envelope-master/v1/"

// masterKeyBytes is the size of the key the envelope actually uses. AES-256, so 32.
const masterKeyBytes = 32

// MinMaterialLength is the floor for configured key material, and it is the same floor
// HUBTASK_SECRET_KEY has for the same reason: a key an operator could have typed from memory is
// not a key. The material is stretched through HKDF rather than used raw, but stretching adds no
// entropy - it only spreads what was given.
const MinMaterialLength = 32

// keyIDPattern is what a key identifier may be. Narrow on purpose: the identifier is stored in
// every row sealed under the key, appears in log lines and manifests, and becomes part of an
// environment variable name - so no case, no punctuation, and nothing that needs quoting.
var keyIDPattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// KeyMaterial is one configured master key as the environment gives it.
type KeyMaterial struct {
	ID       string
	Material secret.Secret
}

// Keyring is the installation's master keys: one current, and every predecessor it still holds.
//
// A ring rather than a single key, because that is what rotation means. A single key with an
// identifier would let an operator change the key and would make every value sealed under the old
// one unreadable at the same moment - which is not a rotation but an outage. The predecessors are
// readable and never written to; removing one is how an operator finally retires it, and a value
// still naming it then says so rather than failing quietly.
type Keyring struct {
	active string
	order  []string
	keys   map[string]secret.Bytes
}

// NewKeyring builds the ring. The first entry is the current key: the order is the configuration's
// statement of which key is which, and a separate "which one is active" setting would be a second
// place for the two to disagree.
func NewKeyring(entries []KeyMaterial) (Keyring, error) {
	ring := Keyring{keys: map[string]secret.Bytes{}}

	for position, entry := range entries {
		if !keyIDPattern.MatchString(entry.ID) {
			return Keyring{}, shared.ErrInternal.
				WithDetail("crypto.key_id_invalid").
				WithParams(map[string]string{"key_id": entry.ID})
		}
		if _, taken := ring.keys[entry.ID]; taken {
			return Keyring{}, shared.ErrInternal.
				WithDetail("crypto.key_id_duplicate").
				WithParams(map[string]string{"key_id": entry.ID})
		}
		// The length is the one thing about the material that may be named in an error. Never
		// the material, and never a prefix of it (rule 10, T-18).
		if len(entry.Material.Reveal()) < MinMaterialLength {
			return Keyring{}, shared.ErrInternal.
				WithDetail("crypto.key_too_short").
				WithParams(map[string]string{
					"key_id":  entry.ID,
					"minimum": fmt.Sprintf("%d", MinMaterialLength),
				})
		}

		derived, err := hkdf.Key(
			sha256.New, []byte(entry.Material.Reveal()), nil,
			masterKeyInfo+entry.ID, masterKeyBytes)
		if err != nil {
			return Keyring{}, shared.ErrInternal.
				WithDetail("crypto.key_underivable").
				WithParams(map[string]string{"key_id": entry.ID}).
				WithCause(fmt.Errorf("stretching the master key %s: %w", entry.ID, err))
		}

		ring.keys[entry.ID] = secret.NewBytes(derived)
		ring.order = append(ring.order, entry.ID)
		if position == 0 {
			ring.active = entry.ID
		}
	}
	return ring, nil
}

// ActiveKeyID is the key new values are sealed under, empty on a ring with no keys.
func (r Keyring) ActiveKeyID() string { return r.active }

// KeyIDs lists the keys the installation holds, current first. Safe to log: an identifier says
// which key and nothing about it, and "which keys does this process have" is the first question
// asked when something will not open.
func (r Keyring) KeyIDs() []string { return slices.Clone(r.order) }

// IsEmpty reports an installation that has configured no key at all. It is not an error at
// startup - an installation that never encrypts anything needs none - and it is the reason
// sealing refuses rather than silently writing a plaintext.
func (r Keyring) IsEmpty() bool { return len(r.keys) == 0 }

// find answers the key with that identifier.
func (r Keyring) find(id string) (secret.Bytes, error) {
	key, held := r.keys[id]
	if !held {
		return secret.Bytes{}, shared.ErrUnavailable.
			WithDetail(portCodeUnknownKey).
			WithParams(map[string]string{"key_id": id})
	}
	return key, nil
}
