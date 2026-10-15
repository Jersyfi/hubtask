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

	got, err := archiveKeysOf(context.Background(), keys, chain, targetID)
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
		[]archive.Description{described("broken", "k1", "%%%")}, targetID)
	if code := shared.AsError(err).DetailCode; code != archive.CodeManifestInvalid {
		t.Fatalf("detail code %q", code)
	}
}
