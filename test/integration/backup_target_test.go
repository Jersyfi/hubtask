// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The statements the backup targets run on, against a real database (E-03): a target round-trips,
// its credential is read by exactly one method and by nothing else, the probe's result is written
// down, and the tenant boundary holds per method (gate SG-3).

func targetRepo() postgres.BackupTargetRepository { return postgres.NewBackupTargetRepository() }

func targetIn(t *testing.T, tenant, author shared.ID, name string) domain.Target {
	t.Helper()

	target, err := domain.NewTarget(domain.NewTargetInput{
		ID: freshID(t), TenantID: tenant, Name: name, Kind: domain.KindS3,
		Config: domain.TargetConfig{
			"bucket": "hubtask-backups", "region": "eu-central-1",
			"endpoint": "https://s3.example.org",
		},
		RegionNote: "Frankfurt", CreatedBy: author, Now: created,
	})
	if err != nil {
		t.Fatalf("building the target: %v", err)
	}
	return target
}

// sealedCredential stands in for what E-02's encryptor produces. The repository never opens one,
// so a fixed blob is exactly as good as a real ciphertext here - and much clearer about the fact
// that this layer cannot read it.
func sealedCredential(text string) crypto.Sealed {
	return crypto.Sealed{KeyID: "k2026", Ciphertext: []byte("sealed:" + text)}
}

func insertTarget(ctx context.Context, t *testing.T, tenant shared.ID, target domain.Target, credential crypto.Sealed) {
	t.Helper()
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return targetRepo().Insert(ctx, target, credential)
	}); err != nil {
		t.Fatalf("writing the target: %v", err)
	}
}

func TestABackupTargetRoundTrips(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	target := targetIn(t, tenantA, authorA, freshName(t))
	insertTarget(ctx, t, tenantA, target, sealedCredential("the-bucket-secret"))

	var stored domain.Target
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = targetRepo().Find(ctx, target.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the target: %v", err)
	}

	switch {
	case stored.ID != target.ID || stored.TenantID != tenantA:
		t.Fatalf("read back %s in tenant %s", stored.ID, stored.TenantID)
	case stored.Kind != domain.KindS3:
		t.Fatalf("kind %q", stored.Kind)
	case stored.Config.Get("bucket") != "hubtask-backups":
		t.Fatalf("the configuration came back as %v", stored.Config)
	case stored.EncryptionMode != domain.EncryptionAES256GCM:
		t.Fatalf("encryption mode %q", stored.EncryptionMode)
	case stored.RegionNote != "Frankfurt":
		t.Fatalf("region note %q", stored.RegionNote)
	case stored.CredentialKeyID != "k2026":
		t.Fatalf("credential key %q - the identifier is not the credential and is read back", stored.CredentialKeyID)
	case !stored.Enabled || stored.Version != 1:
		t.Fatalf("enabled=%v version=%d", stored.Enabled, stored.Version)
	case stored.LastTestOK != nil:
		t.Fatal("a target nobody tested says it was tested")
	}
}

// The property the whole task turns on: a credential is answered by one method, and the two that
// feed a response cannot carry one because the statements behind them do not select it.
func TestACredentialIsAnsweredByOneMethodAndNoOther(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	target := targetIn(t, tenantA, authorA, freshName(t))
	insertTarget(ctx, t, tenantA, target, sealedCredential("the-bucket-secret"))

	var sealed crypto.Sealed
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		sealed, err = targetRepo().Credential(ctx, target.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the credential: %v", err)
	}
	if sealed.KeyID != "k2026" || string(sealed.Ciphertext) != "sealed:the-bucket-secret" {
		t.Fatalf("the credential came back as %v", sealed)
	}

	// And the domain object that reaches a response has nowhere to put one: it carries the key
	// identifier, which says which key, and nothing that says anything about the value.
	var stored domain.Target
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = targetRepo().Find(ctx, target.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the target: %v", err)
	}
	for _, printed := range []string{stored.Name, stored.RegionNote, stored.CredentialKeyID} {
		if printed == "the-bucket-secret" {
			t.Fatal("the credential reached a field that goes to a response")
		}
	}
}

// The cross-tenant negative, per method, because there is a policy underneath but each method is
// still its own statement (gate SG-3).
func TestAnotherTenantSeesNothingOfATarget(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	target := targetIn(t, tenantA, authorA, freshName(t))
	insertTarget(ctx, t, tenantA, target, sealedCredential("the-bucket-secret"))

	err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := targetRepo().Find(ctx, target.ID)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B read tenant A's target: %v", err)
	}

	err = read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := targetRepo().Credential(ctx, target.ID)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B read tenant A's credential: %v", err)
	}

	err = write(ctx, t, tenantB, func(ctx context.Context) error {
		return targetRepo().RecordTest(ctx, target.ID, created, true, "")
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B wrote a probe result onto tenant A's target: %v", err)
	}

	var seen []domain.Target
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		seen, err = targetRepo().List(ctx)
		return err
	}); err != nil {
		t.Fatalf("listing in tenant B: %v", err)
	}
	for _, other := range seen {
		if other.ID == target.ID {
			t.Fatal("tenant A's target is in tenant B's list")
		}
	}
}

// An instance-wide target - the shape 0001_init has always allowed - is visible to nobody through
// this repository. Instance administration has no surface on this API yet, and a policy that
// compares against a tenant cannot match a row that has none.
func TestAnInstanceWideTargetIsVisibleToNoTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	id := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO backup_target (id, tenant_id, name, kind, config, encryption_mode, created_at)
		VALUES ($1, NULL, $2, 'S3', '{"bucket":"instance"}'::jsonb, 'AES256_GCM', now())`,
		id.String(), freshName(t)); err != nil {
		t.Fatalf("planting the instance-wide target: %v", err)
	}

	err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := targetRepo().Find(ctx, id)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a tenant read the instance's target: %v", err)
	}
}

func TestTheProbesResultIsWrittenDown(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	target := targetIn(t, tenantA, authorA, freshName(t))
	insertTarget(ctx, t, tenantA, target, sealedCredential("x"))

	at := created.Add(time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return targetRepo().RecordTest(ctx, target.ID, at, false, "backup.target_unreachable")
	}); err != nil {
		t.Fatalf("recording the probe: %v", err)
	}

	var stored domain.Target
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = targetRepo().Find(ctx, target.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the target: %v", err)
	}
	switch {
	case stored.LastTestOK == nil || *stored.LastTestOK:
		t.Fatalf("the failed probe reads as %v", stored.LastTestOK)
	case !stored.LastTestAt.Equal(at):
		t.Fatalf("the probe is dated %s", stored.LastTestAt)
	case stored.LastTestError != "backup.target_unreachable":
		t.Fatalf("the probe recorded %q", stored.LastTestError)
	}

	// A probe that worked clears the reason, because a stale one would have an operator chasing
	// a problem that is over.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return targetRepo().RecordTest(ctx, target.ID, at.Add(time.Hour), true, "")
	}); err != nil {
		t.Fatalf("recording the second probe: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = targetRepo().Find(ctx, target.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the target: %v", err)
	}
	if stored.LastTestOK == nil || !*stored.LastTestOK || stored.LastTestError != "" {
		t.Fatalf("the successful probe reads as %v / %q", stored.LastTestOK, stored.LastTestError)
	}
}

// Two names that differ only in case are the same name: 0001_init's unique index is on the
// lower-cased one, and an operator staring at "Hetzner" and "hetzner" in a list has a problem the
// database can prevent.
func TestTwoTargetsCannotShareAName(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	name := freshName(t)
	insertTarget(ctx, t, tenantA, targetIn(t, tenantA, authorA, name), sealedCredential("x"))

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		clash := targetIn(t, tenantA, authorA, name)
		return targetRepo().Insert(ctx, clash, sealedCredential("y"))
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("the second target was accepted: %v", err)
	}

	// The same name in another tenant is a different target and is fine.
	insertTarget(ctx, t, tenantB, targetIn(t, tenantB, authorB, name), sealedCredential("z"))
}

func TestCoverageCountsWhatTheHealthSurfaceAsks(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	var before repository.Coverage
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		before, err = targetRepo().Coverage(ctx)
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}

	insertTarget(ctx, t, tenantA, targetIn(t, tenantA, authorA, freshName(t)), sealedCredential("x"))

	unencrypted, err := domain.NewTarget(domain.NewTargetInput{
		ID: freshID(t), TenantID: tenantA, Name: freshName(t), Kind: domain.KindLocal,
		Config: domain.TargetConfig{"path": "backups"}, EncryptionMode: domain.EncryptionNone,
		InsecureAcknowledged: true, CreatedBy: authorA, Now: created,
	})
	if err != nil {
		t.Fatalf("building the unencrypted target: %v", err)
	}
	insertTarget(ctx, t, tenantA, unencrypted, sealedCredential(""))

	var after repository.Coverage
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		after, err = targetRepo().Coverage(ctx)
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if after.Configured != before.Configured+2 {
		t.Fatalf("configured went from %d to %d", before.Configured, after.Configured)
	}
	if after.Unencrypted != before.Unencrypted+1 {
		t.Fatalf("unencrypted went from %d to %d", before.Unencrypted, after.Unencrypted)
	}
}
