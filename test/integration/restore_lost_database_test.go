// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/archive"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	cryptoadapter "github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// envelopeHolding is an installation's envelope whose ring holds the given keys, id to material.
func envelopeHolding(t *testing.T, keys map[string]string) cryptoadapter.Envelope {
	t.Helper()
	var entries []cryptoadapter.KeyMaterial
	for id, material := range keys {
		entries = append(entries, cryptoadapter.KeyMaterial{ID: id, Material: secret.New(material)})
	}
	ring, err := cryptoadapter.NewKeyring(entries)
	if err != nil {
		t.Fatalf("the keyring: %v", err)
	}
	return cryptoadapter.NewEnvelope(ring, clockadapter.CryptoRandom{})
}

const keptMaterial = "the master key the operator kept, long enough to count"

// encryptedBackupOfAForeignWorkspace takes an encrypted backup of a workspace into the store and
// then removes the workspace, its target and its runs for good: what the store holds afterwards is
// all that is left of it, as after the installation's database was lost.
func encryptedBackupOfAForeignWorkspace(ctx context.Context, t *testing.T, f foreignRestore) string {
	t.Helper()
	source, _ := foreignWorkspace(ctx, t, f.store, "Water the plants")

	target := targetIn(t, source, freshID(t), freshName(t))
	if target.EncryptionMode != domain.EncryptionAES256GCM {
		t.Fatalf("the target encrypts %s", target.EncryptionMode)
	}
	insertTarget(ctx, t, source, target, crypto.Sealed{})
	envelope := envelopeHolding(t, map[string]string{"k1": keptMaterial})
	performer := backupservice.Performer{
		Runs: runRepo(), Targets: targetRepo(), Export: postgres.NewBackupExportRepository(postgres.DefaultExportBatch),
		Opener: foreignTarget{store: f.store}, Encryptor: envelope, Keys: envelope,
		Cipher: cryptoadapter.NewStream(clockadapter.CryptoRandom{}), Objects: exportObjects{key: "none"},
		Snapshot: f.uow, UnitOfWork: f.uow, Clock: clockadapter.System{}, IDs: generator{t},
		Entropy: clockadapter.CryptoRandom{}, SchemaVersion: "test", ProductVersion: "test",
	}
	run, err := performer.Perform(ctx, backupservice.PerformInput{
		RunID: freshID(t), TargetID: target.ID, TenantID: source,
		Mode: domain.ModeFull, Trigger: domain.TriggerManual,
	})
	if err != nil {
		t.Fatalf("taking the encrypted backup: %v", err)
	}
	deleteForGood(ctx, t, source)
	return run.ArchivePath
}

// mintedWorkspace is an identifier whose first twelve digits differ from every other test's.
func mintedWorkspace(t *testing.T) shared.ID {
	t.Helper()
	return shared.MustParseID(fmt.Sprintf("%08x-7c1e-7000-8f00-000000000001", 0x0aa00000+fixtureCounter.Add(1)))
}

// restoreWith runs the operator's NEW_TENANT restore of prefix on an installation whose ring is
// the given envelope's.
func (f foreignRestore) restoreWith(
	ctx context.Context, t *testing.T, keys cryptoadapter.Envelope, prefix string,
) (shared.ID, error) {
	t.Helper()
	// The minted workspace's slug is derived from the first twelve digits of its identifier, which
	// freshID keeps the same for every fixture; one of its own keeps two restores in one run apart.
	restoreID, minted := freshID(t), mintedWorkspace(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return restoreRepo().Insert(ctx, domain.Restore{
			ID: restoreID, TargetID: f.target, TenantID: minted, SourceArchive: prefix,
			Mode: domain.RestoreNewTenant, ConflictRule: domain.ConflictSkip,
			Status: domain.RestorePending, RequestedBy: authorA,
		})
	}); err != nil {
		t.Fatalf("accepting the restore: %v", err)
	}
	applier := f.applier(t)
	applier.Keys, applier.Encryptor = keys, keys
	_, err := applier.Apply(ctx, backupservice.ApplyInput{RestoreID: restoreID, TenantID: tenantA})
	return minted, err
}

// UC-BAK-04 check 4: after the database is lost, an installation given the kept master key and a
// target over the same location restores an encrypted archive - its key comes from the archive's
// own manifest, not from a target row that no longer exists.
func TestAnEncryptedArchiveRestoresAfterTheDatabaseIsLost(t *testing.T) {
	ctx := context.Background()
	f := newForeignRestore(ctx, t)
	prefix := encryptedBackupOfAForeignWorkspace(ctx, t, f)

	minted, err := f.restoreWith(ctx, t, envelopeHolding(t, map[string]string{"k1": keptMaterial}), prefix)
	if err != nil {
		t.Fatalf("restoring the encrypted archive: %v", err)
	}
	var title string
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT title FROM work_item WHERE tenant_id = $1`,
		minted.String()).Scan(&title); err != nil || title != "Water the plants" {
		t.Fatalf("the task arrived as %q (%v)", title, err)
	}
}

// UC-BAK-04 check 5: without the master key the archive names - or with another key under its
// name - the restore is refused by name, naming the key, and nothing is written.
func TestAnEncryptedArchiveWithoutItsMasterKeyIsRefusedByName(t *testing.T) {
	ctx := context.Background()
	f := newForeignRestore(ctx, t)
	prefix := encryptedBackupOfAForeignWorkspace(ctx, t, f)

	for name, ring := range map[string]map[string]string{
		"not_in_ring":   {"k2": keptMaterial},
		"does_not_open": {"k1": "another installation's master key under the same name"},
	} {
		t.Run(name, func(t *testing.T) {
			minted, err := f.restoreWith(ctx, t, envelopeHolding(t, ring), prefix)
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != archive.CodeArchiveKeyUnknown ||
				domainErr.Params["key_id"] != "k1" || domainErr.Params["reason"] != name {
				t.Fatalf("refused with %v", err)
			}
			var rows int
			if err := adminPool(ctx, t).QueryRow(ctx, `SELECT count(*) FROM container WHERE tenant_id = $1`,
				minted.String()).Scan(&rows); err != nil || rows != 0 {
				t.Errorf("the refused restore wrote %d containers (%v)", rows, err)
			}
		})
	}
}
