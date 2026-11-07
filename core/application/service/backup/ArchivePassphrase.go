// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ArchivePassphrase is the passphrase a restore was asked with and the deriver that turns it into
// an archive's key by the parameters the archive's manifest names (backup-restore.md §4). The zero
// value is a restore asked without one.
type ArchivePassphrase struct {
	Deriver    crypto.KeyDeriver
	Passphrase secret.Secret
}

// Given reports whether the restore was asked with a passphrase.
func (p ArchivePassphrase) Given() bool { return !p.Passphrase.IsEmpty() }

// keyOf derives the key of an archive encrypted to a passphrase.
func (p ArchivePassphrase) keyOf(encryption archive.Encryption) (secret.Bytes, error) {
	if !p.Given() {
		return secret.Bytes{}, shared.ErrValidation.WithDetail(archive.CodeArchivePassphraseRequired)
	}
	if p.Deriver == nil {
		return secret.Bytes{}, shared.Internalf("backup: a passphrase restore without a key deriver")
	}
	salt, err := base64.StdEncoding.DecodeString(encryption.Salt)
	if err != nil || len(salt) == 0 {
		return secret.Bytes{}, shared.ErrValidation.WithDetail(archive.CodeManifestInvalid).
			WithParams(map[string]string{"reason": "encryption.salt"}).
			WithCause(errors.New("a derivation salt that is not base64"))
	}
	return p.Deriver.Derive(p.Passphrase, crypto.Derivation{
		Salt: salt, Passes: encryption.Passes, MemoryKiB: encryption.MemoryKiB,
		Parallelism: encryption.Parallelism, KeyLength: encryption.KeyLength,
	})
}

// PassphraseEncryption derives a fresh key from a passphrase and answers it with the manifest's
// description of the derivation - what an archive written to a passphrase carries.
func PassphraseEncryption(deriver crypto.KeyDeriver, passphrase secret.Secret) (archive.Encryption, secret.Bytes, error) {
	derivation, err := deriver.NewDerivation()
	if err != nil {
		return archive.Encryption{}, secret.Bytes{}, err
	}
	key, err := deriver.Derive(passphrase, derivation)
	if err != nil {
		return archive.Encryption{}, secret.Bytes{}, err
	}
	return archive.Encryption{
		Mode: archive.EncryptionAES256GCM, KeyID: archive.PassphraseKeyID, KDF: archive.KDFArgon2id,
		Salt:   base64.StdEncoding.EncodeToString(derivation.Salt),
		Passes: derivation.Passes, MemoryKiB: derivation.MemoryKiB,
		Parallelism: derivation.Parallelism, KeyLength: derivation.KeyLength,
	}, key, nil
}

// restorePassphrasePurpose binds a held passphrase to its run: lifted into another row it no
// longer opens.
func restorePassphrasePurpose(restoreID shared.ID) crypto.Purpose {
	return crypto.Purpose("restore_run.passphrase:" + restoreID.String())
}

// HoldPassphrase seals the passphrase a restore was asked with to its run and keeps it there until
// the run ends; nothing where none was given.
func HoldPassphrase(
	ctx context.Context, encryptor crypto.Encryptor, restores repository.Restores,
	restoreID shared.ID, passphrase secret.Secret,
) error {
	if passphrase.IsEmpty() {
		return nil
	}
	sealed, err := encryptor.Seal(ctx, passphrase, restorePassphrasePurpose(restoreID))
	if err != nil {
		return err
	}
	return restores.HoldPassphrase(ctx, restoreID, sealed)
}

// heldPassphrase opens the passphrase a run holds, the zero value where it holds none.
func heldPassphrase(
	ctx context.Context, encryptor crypto.Encryptor, sealed crypto.Sealed, restoreID shared.ID,
) (secret.Secret, error) {
	if sealed.IsZero() {
		return secret.Secret{}, nil
	}
	return encryptor.Open(ctx, sealed, restorePassphrasePurpose(restoreID))
}
