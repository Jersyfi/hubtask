// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// written produces an archive at a store and answers what a reader needs to open it again.
func written(t *testing.T, store *memoryStore, request Request, rows map[string][]Record, blobs *blobStore) Manifest {
	t.Helper()

	source := newSource()
	for entity, records := range rows {
		source.rows[entity] = records
	}
	manifest, err := NewWriter(store, &reversible{}, blobs).Write(t.Context(), request, source)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	return manifest
}

func read(t *testing.T, store *memoryStore, description Description, entity string, key secret.Bytes) []Record {
	t.Helper()

	found, ok := FindEntity(entity)
	if !ok {
		t.Fatalf("no entity %s", entity)
	}
	var records []Record
	err := NewReader(store, &reversible{}).Records(t.Context(), description, found, key,
		func(r Record) error { records = append(records, r); return nil })
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	return records
}

func TestAnArchiveComesBackTheWayItWentIn(t *testing.T) {
	store := newStore()
	rows := map[string][]Record{
		"containers": {
			{ID: "c1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"kind": "HUB", "depth": float64(1)}},
			{ID: "c2", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"kind": "COLLECTION"}},
		},
		"work_items": {{ID: "w1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"state": "OPEN"}}},
	}
	written(t, store, request(), rows, newBlobs())

	description, err := NewReader(store, &reversible{}).Describe(t.Context(), request().Prefix)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if !description.Complete {
		t.Fatal("a finished archive reads as incomplete")
	}

	containers := read(t, store, description, "containers", key())
	if len(containers) != 2 || containers[0].ID != "c1" || containers[1].ID != "c2" {
		t.Fatalf("containers: %+v", containers)
	}
	if containers[0].Data["kind"] != "HUB" {
		t.Fatalf("the row did not survive: %+v", containers[0].Data)
	}
}

// The listing is assembled from the target alone. That is what makes a restore possible after a
// total loss, and it is the sentence §8.1 makes.
func TestTheListingComesFromTheTargetAndNothingElse(t *testing.T) {
	store := newStore()
	tenantID := shared.MustParseID(tenant)

	for i, at := range []time.Time{
		snapshot().Add(-48 * time.Hour), snapshot().Add(-24 * time.Hour), snapshot(),
	} {
		run := request()
		run.SnapshotAt = at
		run.Prefix = "backups/" + Name(tenantID, at, ModeFull)
		run.ArchiveID = shared.MustParseID("0198f0a0-0000-7000-8000-00000000000" + string(rune('1'+i)))
		written(t, store, run, nil, newBlobs())
	}
	// And something at the target that is not ours, which must be left alone (BK-8).
	if _, err := store.Put(t.Context(), "backups/somebody-elses-file.tar", strings.NewReader("x")); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	listed, err := NewReader(store, &reversible{}).List(t.Context(), "backups/")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 3 {
		t.Fatalf("%d archives listed, want 3: %+v", len(listed), listed)
	}
	if !listed[0].Manifest.SnapshotAt.Equal(snapshot()) {
		t.Fatalf("not newest first: %v", listed[0].Manifest.SnapshotAt)
	}
	for _, description := range listed {
		if description.Bytes == 0 || !description.Complete {
			t.Fatalf("%s: %d bytes, complete %v", description.Prefix, description.Bytes, description.Complete)
		}
	}
}

// An archive with no checksums.txt is a run that died. It is not damaged, and the difference
// matters to whoever is deciding what to restore from.
func TestAnUnfinishedArchiveIsRecognisedRatherThanRefused(t *testing.T) {
	store := newStore()
	written(t, store, request(), nil, newBlobs())
	if err := store.Delete(t.Context(), request().Prefix+"/"+ChecksumsName); err != nil {
		t.Fatalf("delete: %v", err)
	}

	description, err := NewReader(store, &reversible{}).Describe(t.Context(), request().Prefix)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if description.Complete {
		t.Fatal("an archive with no checksums.txt reads as complete")
	}

	err = NewReader(store, &reversible{}).Verify(t.Context(), request().Prefix)
	if got := detail(t, err); got != CodeArchiveIncomplete {
		t.Fatalf("verify: detail code %q", got)
	}
}

// :verify checks an archive at the target without restoring it and without the key.
func TestVerifyChecksTheArchiveWithoutTheKey(t *testing.T) {
	store := newStore()
	written(t, store, request(), map[string][]Record{
		"comments": {{ID: "k1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"body_length": 4}}},
	}, newBlobs())

	if err := NewReader(store, &reversible{}).Verify(t.Context(), request().Prefix); err != nil {
		t.Fatalf("a sound archive failed verification: %v", err)
	}
}

func TestACorruptedByteInAMemberIsFound(t *testing.T) {
	store := newStore()
	written(t, store, request(), map[string][]Record{
		"comments": {{ID: "k1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"body_length": 4}}},
	}, newBlobs())

	member, _ := store.object(request().Prefix + "/" + DataName("comments"))
	corrupted := bytes.Clone(member)
	corrupted[len(corrupted)/2] ^= 0x01
	if _, err := store.Put(t.Context(), request().Prefix+"/"+DataName("comments"), bytes.NewReader(corrupted)); err != nil {
		t.Fatalf("put: %v", err)
	}

	err := NewReader(store, &reversible{}).Verify(t.Context(), request().Prefix)
	if err == nil {
		t.Fatal("one flipped bit passed verification")
	}
	if got := detail(t, err); got != CodeChecksumMismatch {
		t.Fatalf("detail code %q", got)
	}
}

// A member's checksum is checked as it is read, not in a separate pass that would verify a
// different read from the one that was used.
func TestReadingAMemberChecksItsChecksum(t *testing.T) {
	store := newStore()
	manifest := written(t, store, request(), map[string][]Record{
		"labels": {{ID: "l1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"colour_token": "label.red"}}},
	}, newBlobs())

	member, _ := store.object(request().Prefix + "/" + DataName("labels"))
	// A byte that reads back as a blank line, so that the record decoder has nothing to object to
	// and the only thing left to catch it is the checksum.
	extended := append(bytes.Clone(member), byte('\n')^reversibleMask)
	if _, err := store.Put(t.Context(), request().Prefix+"/"+DataName("labels"), bytes.NewReader(extended)); err != nil {
		t.Fatalf("put: %v", err)
	}

	description := Description{Prefix: request().Prefix, Manifest: manifest, Complete: true}
	labels, _ := FindEntity("labels")
	err := NewReader(store, &reversible{}).Records(t.Context(), description, labels, key(), func(Record) error { return nil })
	if err == nil {
		t.Fatal("a member with an extra byte was read without complaint")
	}
	if got := detail(t, err); got != CodeChecksumMismatch {
		t.Fatalf("detail code %q", got)
	}
}

// An encrypted archive is unreadable without its key, and a key offered for one that has none is
// a caller confusing two archives.
func TestTheKeyIsRequiredExactlyWhenTheArchiveSaysSo(t *testing.T) {
	store := newStore()
	manifest := written(t, store, request(), map[string][]Record{
		"labels": {{ID: "l1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{}}},
	}, newBlobs())
	description := Description{Prefix: request().Prefix, Manifest: manifest, Complete: true}
	labels, _ := FindEntity("labels")

	err := NewReader(store, &reversible{}).Records(t.Context(), description, labels, secret.Bytes{},
		func(Record) error { return nil })
	if got := detail(t, err); got != CodeArchiveKeyRequired {
		t.Fatalf("without a key: detail code %q", got)
	}

	unencryptedStore := newStore()
	unencrypted := written(t, unencryptedStore, plaintext(), map[string][]Record{
		"labels": {{ID: "l1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{}}},
	}, newBlobs())
	err = NewReader(unencryptedStore, &reversible{}).Records(
		t.Context(), Description{Prefix: plaintext().Prefix, Manifest: unencrypted, Complete: true},
		labels, key(), func(Record) error { return nil })
	if got := detail(t, err); got != CodeArchiveKeyUnexpected {
		t.Fatalf("with an unwanted key: detail code %q", got)
	}
}

// A chain is walked by reading manifests, and one that does not end at a full archive is refused.
// Restoring the part that happens to be present would restore a tenant to a state it was never in.
func TestAChainIsWalkedToTheFullArchiveOrRefused(t *testing.T) {
	store := newStore()
	tenantID := shared.MustParseID(tenant)

	fullAt := snapshot().Add(-48 * time.Hour)
	fullPrefix := Name(tenantID, fullAt, ModeFull)
	fullRun := request()
	fullRun.SnapshotAt, fullRun.Prefix = fullAt, fullPrefix
	written(t, store, fullRun, nil, newBlobs())

	incrementalAt := snapshot()
	incrementalPrefix := Name(tenantID, incrementalAt, ModeIncremental)
	incremental := request()
	incremental.Mode = ModeIncremental
	incremental.SnapshotAt, incremental.Since = incrementalAt, fullAt
	incremental.Prefix = incrementalPrefix
	incremental.ParentID, incremental.ParentPrefix = fullRun.ArchiveID.String(), fullPrefix
	incremental.ArchiveID = shared.MustParseID("0198f0a0-0000-7000-8000-000000000002")
	written(t, store, incremental, nil, newBlobs())

	chain, err := NewReader(store, &reversible{}).Chain(t.Context(), incrementalPrefix)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if len(chain) != 2 || chain[0].Prefix != incrementalPrefix || chain[1].Prefix != fullPrefix {
		t.Fatalf("chain: %+v", chain)
	}

	// Now lose the full archive's manifest, which is what an over-eager retention run looks like.
	if err := store.Delete(t.Context(), fullPrefix+"/"+ManifestName); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = NewReader(store, &reversible{}).Chain(t.Context(), incrementalPrefix)
	if err == nil {
		t.Fatal("a chain with no root was walked")
	}
	if got := detail(t, err); got != CodeChainBroken {
		t.Fatalf("detail code %q", got)
	}
}

// The medium lives in whichever archive first referenced it, and a restore finds it by searching
// the chain rather than by every archive carrying a copy.
func TestAMediumIsFoundInWhicheverArchiveHoldsIt(t *testing.T) {
	store, blobs := newStore(), newBlobs()
	tenantID := shared.MustParseID(tenant)
	content := "the bytes of an attachment that never changed"
	attachment := blobOf(content)
	blobs.content[attachment.Digest] = content

	fullAt := snapshot().Add(-48 * time.Hour)
	fullPrefix := Name(tenantID, fullAt, ModeFull)
	fullRun := request()
	fullRun.SnapshotAt, fullRun.Prefix = fullAt, fullPrefix
	written(t, store, fullRun, map[string][]Record{
		"item_attachments": {{ID: "a1", Op: OpUpsert, UpdatedAt: fullAt, Data: map[string]any{}, Blobs: []Blob{attachment}}},
	}, blobs)

	incrementalPrefix := Name(tenantID, snapshot(), ModeIncremental)
	incremental := request()
	incremental.Mode, incremental.Since = ModeIncremental, fullAt
	incremental.Prefix = incrementalPrefix
	incremental.ParentID, incremental.ParentPrefix = fullRun.ArchiveID.String(), fullPrefix
	incremental.Ancestors = []string{fullPrefix}
	incremental.ArchiveID = shared.MustParseID("0198f0a0-0000-7000-8000-000000000002")
	written(t, store, incremental, map[string][]Record{
		"item_attachments": {{ID: "a2", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{}, Blobs: []Blob{attachment}}},
	}, blobs)

	reader := NewReader(store, &reversible{})
	chain, err := reader.Chain(t.Context(), incrementalPrefix)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}

	stream, err := reader.Medium(t.Context(), chain, attachment.Digest, key())
	if err != nil {
		t.Fatalf("medium: %v", err)
	}
	defer func() { _ = stream.Close() }()

	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != content {
		t.Fatalf("the medium came back as %q", got)
	}
}

func TestAMediumNoArchiveInTheChainHoldsIsNotFound(t *testing.T) {
	store := newStore()
	written(t, store, request(), nil, newBlobs())
	description, err := NewReader(store, &reversible{}).Describe(t.Context(), request().Prefix)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}

	_, err = NewReader(store, &reversible{}).Medium(t.Context(), []Description{description},
		blobOf("never stored anywhere").Digest, key())
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a medium nobody holds: %v", err)
	}
}

// The manifest and checksums.txt are written by the same run. Disagreeing is not corruption of one
// of them - it is an archive assembled from parts.
func TestAManifestThatDisagreesWithTheChecksumsIsRefused(t *testing.T) {
	store := newStore()
	manifest := written(t, store, request(), nil, newBlobs())

	manifest.Files[0].SHA256 = digestOf("something else entirely")
	var edited bytes.Buffer
	if err := manifest.Encode(&edited); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, err := store.Put(t.Context(), request().Prefix+"/"+ManifestName, &edited); err != nil {
		t.Fatalf("put: %v", err)
	}

	err := NewReader(store, &reversible{}).Verify(t.Context(), request().Prefix)
	if err == nil {
		t.Fatal("an edited manifest passed verification")
	}
	if got := detail(t, err); got != CodeChecksumMismatch {
		t.Fatalf("detail code %q", got)
	}
}

// The optional audit member is the one whose absence is a configuration rather than a defect.
func TestAMissingAuditMemberIsNotAFailure(t *testing.T) {
	store := newStore()
	manifest := written(t, store, request(), nil, newBlobs())
	description := Description{Prefix: request().Prefix, Manifest: manifest, Complete: true}

	audit, _ := FindEntity("audit")
	var seen int
	err := NewReader(store, &reversible{}).Records(t.Context(), description, audit, key(),
		func(Record) error { seen++; return nil })
	if err != nil {
		t.Fatalf("an archive written without the audit trail: %v", err)
	}
	if seen != 0 {
		t.Fatalf("%d audit records out of nowhere", seen)
	}

	// Anything else missing from the manifest is an archive somebody has taken a file out of.
	containers, _ := FindEntity("containers")
	missing := description
	missing.Manifest.Files = nil
	err = NewReader(store, &reversible{}).Records(t.Context(), missing, containers, key(), func(Record) error { return nil })
	if err == nil {
		t.Fatal("a member the manifest does not name was read anyway")
	}
}
