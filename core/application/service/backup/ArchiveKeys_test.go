// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/archive"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// derivation is one key asked for, as the materialiser saw it.
type derivation struct {
	keyID   string
	purpose crypto.Purpose
	salt    []byte
}

// recordingKeys answers a key that names how it was asked for, so a test can tell which
// derivation an archive got.
type recordingKeys struct{ asked []derivation }

func (k *recordingKeys) DeriveFromMaster(context.Context, crypto.Purpose, []byte, int) (crypto.MasterDerived, error) {
	return crypto.MasterDerived{}, errors.New("not used")
}

func (k *recordingKeys) ReproduceFromMaster(
	_ context.Context, keyID string, purpose crypto.Purpose, salt []byte, _ int,
) (secret.Bytes, error) {
	k.asked = append(k.asked, derivation{keyID: keyID, purpose: purpose, salt: salt})
	return secret.NewBytes([]byte(keyID + "|" + string(purpose) + "|" + string(salt))), nil
}

func described(id, keyID, salt string) archive.Description {
	encryption := archive.Encryption{Mode: archive.EncryptionAES256GCM, KeyID: keyID, ArchiveSalt: salt}
	if keyID == "" {
		encryption = archive.Encryption{Mode: archive.EncryptionNone}
	}
	return archive.Description{Prefix: id, Manifest: archive.Manifest{ArchiveID: id, Encryption: encryption}}
}

// Every archive of a chain opens with its own key: a salted one from its own salt and the master
// key it names, one written before archives carried a salt from the target the reader names - so a
// chain across a rotation, or across the day salts arrived, reads end to end.
func TestEveryArchiveOfAChainOpensWithItsOwnKey(t *testing.T) {
	salt := bytes.Repeat([]byte{9}, archiveSaltBytes)
	chain := []archive.Description{
		described("incremental", "k2", base64.StdEncoding.EncodeToString(salt)),
		described("full", "k1", ""),
		described("plain", "", ""),
	}
	keys := &recordingKeys{}

	got, err := archiveKeysOf(context.Background(), keys, chain, targetID, ArchivePassphrase{})
	if err != nil {
		t.Fatalf("deriving: %v", err)
	}
	if len(keys.asked) != 2 || len(got) != 2 {
		t.Fatalf("asked %d keys, answered %d", len(keys.asked), len(got))
	}
	salted, legacy := keys.asked[0], keys.asked[1]
	switch {
	case salted.keyID != "k2" || salted.purpose != archivePurpose || !bytes.Equal(salted.salt, salt):
		t.Fatalf("the salted archive was derived as %+v", salted)
	case legacy.keyID != "k1" || legacy.purpose != legacyArchivePurpose(targetID) || legacy.salt != nil:
		t.Fatalf("the archive without a salt was derived as %+v", legacy)
	case got.Of(chain[0]).Equal(got.Of(chain[1])):
		t.Fatal("two archives of the chain got one key")
	case got.Of(chain[2]).Len() != 0:
		t.Fatal("an unencrypted archive got a key")
	}
}

func TestASaltThatIsNotBase64IsAManifestNobodyCanUse(t *testing.T) {
	_, err := archiveKeysOf(context.Background(), &recordingKeys{},
		[]archive.Description{described("broken", "k1", "%%%")}, targetID, ArchivePassphrase{})
	if code := shared.AsError(err).DetailCode; code != archive.CodeManifestInvalid {
		t.Fatalf("detail code %q", code)
	}
}

// echoDeriver answers a key that names the passphrase and the salt, so a test can tell what an
// archive's key was derived from.
type echoDeriver struct{}

func (echoDeriver) NewDerivation() (crypto.Derivation, error) {
	return crypto.Derivation{Salt: []byte("salt-of-sixteen!"), Passes: 3, MemoryKiB: 64, Parallelism: 1, KeyLength: 32}, nil
}

func (echoDeriver) Derive(passphrase secret.Secret, from crypto.Derivation) (secret.Bytes, error) {
	if passphrase.IsEmpty() {
		return secret.Bytes{}, shared.ErrValidation.WithDetail(crypto.CodePassphraseRequired)
	}
	return secret.NewBytes([]byte(passphrase.Reveal() + "|" + string(from.Salt))), nil
}

// UC-BAK-07 check 7: an archive downloaded with a passphrase opens with the key that passphrase
// and its manifest's derivation give - not with a master key - and is refused by name without
// one; a passphrase given for a chain no archive of which needs one is refused, not ignored.
func TestAPassphraseArchiveOpensWithItsPassphraseAlone(t *testing.T) {
	encryption, written, err := PassphraseEncryption(echoDeriver{}, secret.New("correct horse battery staple"))
	if err != nil {
		t.Fatal(err)
	}
	chain := []archive.Description{{Prefix: "download", Manifest: archive.Manifest{ArchiveID: "download", Encryption: encryption}}}
	keys := &recordingKeys{}

	given := ArchivePassphrase{Deriver: echoDeriver{}, Passphrase: secret.New("correct horse battery staple")}
	got, err := archiveKeysOf(context.Background(), keys, chain, targetID, given)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Of(chain[0]).Reveal()) != string(written.Reveal()) || len(keys.asked) != 0 {
		t.Errorf("the key was %q (master asked %d times)", got.Of(chain[0]).Reveal(), len(keys.asked))
	}

	_, err = archiveKeysOf(context.Background(), keys, chain, targetID, ArchivePassphrase{Deriver: echoDeriver{}})
	if code := shared.AsError(err).DetailCode; code != archive.CodeArchivePassphraseRequired {
		t.Errorf("without a passphrase: %q", code)
	}

	_, err = archiveKeysOf(context.Background(), keys,
		[]archive.Description{described("plain", "", ""), described("salted", "k1", base64.StdEncoding.EncodeToString([]byte("s")))},
		targetID, given)
	if code := shared.AsError(err).DetailCode; code != archive.CodeArchivePassphraseUnused {
		t.Errorf("a passphrase nothing needs: %q", code)
	}
}
