// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package crypto is the port for keeping a value unreadable to whoever holds the storage it sits
// on (E-02, backup-restore.md §4, security.md §3).
//
// Two seams, because there are two questions. Sealing a value the server itself has to read back -
// a target's credential - is envelope encryption under a master key the installation supplies:
// the key identifier travels with the ciphertext, so rotating the master key is a configuration
// change rather than a data migration. Deriving a key from a passphrase somebody typed is the
// other: the passphrase is not stored anywhere, and what has to be stored is only what a later run
// needs in order to arrive at the same key again.
//
// Nothing here says AES, Argon2id or GCM. Those are the adapter's, which is what makes open point
// S-2 - where the master key lives in provider operation - a second adapter rather than a rewrite.
package crypto

import (
	"context"
	"io"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Sealed is a ciphertext and the identifier of the master key that has to open it.
//
// The identifier is stored beside the ciphertext rather than inside it, because that is what makes
// a rotation cheap: new values are sealed under the new key, old ones keep naming the old one, and
// nothing is rewritten. `0001_init` has had the pair of columns since phase 0 - `credential_enc`
// and `credential_key_id` on `backup_target`.
//
// A key identifier is not a secret. It says which key, never anything about it, and it is the one
// part of this that may appear in a log line an operator has to read.
type Sealed struct {
	KeyID      string
	Ciphertext []byte
}

func (s Sealed) IsZero() bool { return s.KeyID == "" && len(s.Ciphertext) == 0 }

// Purpose is what a ciphertext is bound to: it is authenticated along with the value, so a
// ciphertext lifted out of one row and dropped into another no longer opens.
//
// It is not a secret and not a key - it travels in the clear and may be reconstructed by anybody
// who can read the row. What it buys is that the ciphertext alone is not a portable capability: a
// caller that binds the field and the row's identifier into it ("backup_target.credential:<id>")
// makes substitution fail as loudly as tampering does.
type Purpose string

// Encryptor seals a value under the installation's current master key and opens one sealed under
// any master key the installation still holds.
//
// It takes a context because an implementation may have to reach a key management service to use
// a key (open point S-2, `0.6.0`); the one this milestone ships holds its keys in memory and
// ignores it, and a port shaped so that it could not wait would make that adapter impossible.
type Encryptor interface {
	// Seal encrypts under the current key. ErrUnavailable with `crypto.no_encryption_key` when
	// the installation has none configured - a refusal rather than a silent plaintext write.
	Seal(ctx context.Context, plaintext secret.Secret, purpose Purpose) (Sealed, error)

	// Open decrypts, and answers ErrUnavailable with `crypto.unknown_key` when the key named is
	// not one the installation holds - which is what an operator sees after removing a key that
	// something was still sealed under.
	//
	// A wrong key, a changed byte and a mismatched purpose are one answer,
	// `crypto.not_authentic`, and never a partial plaintext: the value is authenticated before
	// any of it is returned.
	Open(ctx context.Context, sealed Sealed, purpose Purpose) (secret.Secret, error)

	// ActiveKeyID is the key new values are sealed under. Empty when the installation holds none.
	ActiveKeyID() string
}

// StreamCipher protects something too large to hold: an archive member on its way to a backup
// target, encrypted as it is written and decrypted as it is read.
//
// It is a second seam beside Encryptor rather than a widening of it, because the shapes are
// genuinely different. An envelope takes a value and gives back a value; both fit in memory by
// construction, and infrastructure/crypto bounds them at a mebibyte to keep the arithmetic
// provable. A member of an archive has no such bound - it is as large as the holding it describes,
// and an interface that took a []byte would be an interface no archive could use (T-17,
// observability-reliability.md §6).
//
// The other difference is the key. An envelope uses the installation's master key, which the
// process holds; a stream uses the backup key of a target, which arrives with the run and is
// derived from a passphrase this system never stores. So the key is a parameter here and a field
// there.
type StreamCipher interface {
	// Seal wraps a writer. Everything written to the result is encrypted on its way to w, and
	// Close finishes the stream - a stream that was not closed is one whose last chunk was never
	// written, and Open refuses it rather than returning the part that happened to arrive.
	//
	// The purpose is authenticated with every chunk, which is what stops a member being swapped
	// for another: a caller that binds the archive and the member's path into it makes
	// data/comments.jsonl presented as data/labels.jsonl fail exactly as a flipped bit does.
	Seal(w io.Writer, key secret.Bytes, purpose Purpose) (io.WriteCloser, error)

	// Open unwraps a reader. Nothing is returned before it has been authenticated: the stream is
	// authenticated chunk by chunk, so a reader sees only bytes that carried a valid tag, and a
	// stream that stops early fails rather than ending quietly.
	//
	// That last property is the one a backup depends on. A truncated transfer is the most common
	// way an archive goes wrong, and a cipher that could not tell a finished stream from a cut
	// one would restore three quarters of a tenant without saying so.
	Open(r io.Reader, key secret.Bytes, purpose Purpose) (io.Reader, error)

	// KeyBytes is how long a key this cipher wants, so that a caller deriving one asks for the
	// right length rather than assuming 32.
	KeyBytes() int
}

// Derivation is everything a later run needs in order to arrive at the same key from the same
// passphrase - and, deliberately, nothing that would let anybody arrive at it without one.
//
// The cost parameters are stored rather than assumed, for the reason the key identifier is stored:
// the constants this system derives with will be raised as machines get faster, and an archive
// written under the old ones has to keep opening. A salt and three numbers are not secret, and
// they belong in the manifest beside the ciphertext they describe.
type Derivation struct {
	Salt []byte
	// Passes is how many times the memory is swept.
	Passes uint32
	// MemoryKiB is the working set, in kibibytes - the parameter that actually costs an attacker
	// anything, because it is the one they cannot trade away for silicon.
	MemoryKiB uint32
	// Parallelism is how many lanes the sweep uses.
	Parallelism uint8
	// KeyLength is the size of the key produced, in bytes.
	KeyLength uint32
}

func (d Derivation) IsZero() bool { return len(d.Salt) == 0 }

// KeyDeriver turns a passphrase into a key, repeatably.
//
// The passphrase is never stored - that is the whole point, and it is why the specification
// documents `BackupTargetCreate.encryption_passphrase` as not stored. What is stored is the
// Derivation, and a Derivation without the passphrase is worth nothing.
type KeyDeriver interface {
	// NewDerivation draws a fresh salt and states this build's cost parameters. A new salt per
	// key, always: two targets protected by the same passphrase must not end up with the same
	// key, or cracking one cracks both.
	NewDerivation() (Derivation, error)

	// Derive computes the key. The same passphrase and the same Derivation give the same key on
	// any machine and any version, which is what lets an archive written last year open today.
	Derive(passphrase secret.Secret, from Derivation) (secret.Bytes, error)
}

// The refusals, as codes rather than as prose. They are here rather than in each adapter so that
// two implementations of this port cannot describe the same failure differently - a caller
// deciding between "ask the operator for the key" and "this archive is damaged" is reading these.
const (
	// CodeNoEncryptionKey is an installation asked to seal something with no master key
	// configured.
	CodeNoEncryptionKey = "crypto.no_encryption_key"
	// CodeUnknownKey is a ciphertext naming a key this installation does not hold.
	CodeUnknownKey = "crypto.unknown_key"
	// CodeNotAuthentic is a wrong key, a changed byte, or a purpose that does not match. One code
	// for the three deliberately: telling them apart would tell somebody probing which of their
	// guesses was closer.
	CodeNotAuthentic = "crypto.not_authentic"
	// CodeCiphertextMalformed is a value that is not one of ours at all - too short to hold the
	// parts, or a version this build does not know.
	CodeCiphertextMalformed = "crypto.ciphertext_malformed"
	// CodePassphraseRequired is a derivation asked for without one. The name is what gosec's
	// hardcoded-credential heuristic reacts to; the value is a message code, and the passphrase
	// it names is the one thing this system never holds.
	CodePassphraseRequired = "crypto.passphrase_required" //nolint:gosec // G101: a message code, not a credential
)

// NotAuthentic is the refusal every failed open answers. A function rather than a value, because
// a shared error value with a detail code attached is one another caller can mutate.
func NotAuthentic() error {
	return shared.ErrValidation.WithDetail(CodeNotAuthentic)
}
