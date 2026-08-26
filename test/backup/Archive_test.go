// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portstorage "github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
)

// BK-2, BK-3 and BK-4: the archive format against a real target, a real cipher and real files.
//
// Everything here goes through the local adapter rather than a double, because the properties
// being asserted are about bytes that left the process. An archive that is only ever written into
// a map has never been truncated by a disk, and truncation is what these tests are for.

var tenantID = shared.MustParseID("0198f0a0-0000-7000-8000-0000000000aa")

func archiveStore(t *testing.T, root string) portstorage.Store {
	t.Helper()
	return open(t, registry(t, root), specFor(domain.KindLocal, domain.TargetConfig{"path": ""}, nil))
}

func realCipher() crypto.Stream { return crypto.NewStream(clockadapter.CryptoRandom{}) }

// keyring is the archive keys an installation still holds, by identifier. It is the shape BK-2's
// second half turns on: a rotation adds a key and removes nothing, so the manifest's key_id is
// enough to open an archive written a year ago.
type keyring map[string]secret.Bytes

func keyNamed(id string) secret.Bytes {
	material := sha256.Sum256([]byte("archive key " + id))
	return secret.NewBytes(material[:])
}

func digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// ── The source a test writes by hand ──────────────────────────────────────────────────────────

// world is a tenant as a test can hold one: the rows that are there, when each last changed, and
// when each of the deleted ones went.
//
// It is the smallest thing that can answer the question an incremental export asks - "what
// happened after this instant" - and BK-3 is entirely a statement about that answer.
type world struct {
	rows    map[string]map[string]map[string]any
	changed map[string]map[string]time.Time
	gone    map[string]map[string]time.Time
	blobs   map[string]string
}

func newWorld() *world {
	return &world{
		rows:    map[string]map[string]map[string]any{},
		changed: map[string]map[string]time.Time{},
		gone:    map[string]map[string]time.Time{},
		blobs:   map[string]string{},
	}
}

func (w *world) put(entity, id string, at time.Time, data map[string]any) {
	if w.rows[entity] == nil {
		w.rows[entity] = map[string]map[string]any{}
		w.changed[entity] = map[string]time.Time{}
		w.gone[entity] = map[string]time.Time{}
	}
	w.rows[entity][id] = data
	w.changed[entity][id] = at
	delete(w.gone[entity], id)
}

func (w *world) remove(entity, id string, at time.Time) {
	delete(w.rows[entity], id)
	delete(w.changed[entity], id)
	if w.gone[entity] == nil {
		w.gone[entity] = map[string]time.Time{}
	}
	w.gone[entity][id] = at
}

func (w *world) attach(content string) archive.Blob {
	w.blobs[digest(content)] = content
	return archive.Blob{Digest: digest(content), Bytes: int64(len(content))}
}

// Records answers what changed after since, tombstones included. On a full run - since is zero -
// it answers the state and no tombstones: a full archive is the whole truth and has nothing to
// deny.
func (w *world) Records(_ context.Context, entity archive.Entity, since time.Time, yield func(archive.Record) error) error {
	type line struct {
		record archive.Record
		at     time.Time
	}
	var lines []line

	for id, data := range w.rows[entity.Name] {
		at := w.changed[entity.Name][id]
		if !since.IsZero() && !at.After(since) {
			continue
		}
		record := archive.Record{ID: id, Op: archive.OpUpsert, UpdatedAt: at, Data: maps.Clone(data)}
		if blobs, carries := data["blobs"].([]archive.Blob); carries {
			record.Blobs = blobs
			record.Data = maps.Clone(data)
			delete(record.Data, "blobs")
		}
		lines = append(lines, line{record, at})
	}
	if !since.IsZero() {
		for id, at := range w.gone[entity.Name] {
			if !at.After(since) {
				continue
			}
			lines = append(lines, line{archive.Record{ID: id, Op: archive.OpDelete, UpdatedAt: at}, at})
		}
	}

	// Oldest first, and ties broken by identity so that two runs of the same world produce the
	// same archive - a format whose bytes depend on map iteration is a format nobody can compare.
	slices.SortFunc(lines, func(a, b line) int {
		if !a.at.Equal(b.at) {
			return a.at.Compare(b.at)
		}
		return strings.Compare(a.record.ID, b.record.ID)
	})
	for _, l := range lines {
		if err := yield(l.record); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) Open(_ context.Context, sha string) (io.ReadCloser, error) {
	content, found := w.blobs[sha]
	if !found {
		return nil, shared.ErrNotFound
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

var (
	_ archive.Source = (*world)(nil)
	_ archive.Media  = (*world)(nil)
)

// ── BK-2 ──────────────────────────────────────────────────────────────────────────────────────

// BK-2, first half: an encrypted archive is unreadable without its key.
func TestAnEncryptedArchiveIsUnreadableWithoutItsKey(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := archiveStore(t, root)

	source := newWorld()
	source.put("work_items", "w1", theHour(1), map[string]any{"state": "OPEN", "title_length": float64(11)})

	request := fullRun(t, "bk_2026_a", theHour(2))
	if _, err := archive.NewWriter(store, realCipher(), source).Write(ctx, request, source); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Nothing of the record is visible on disk. The whole point of §4 is that the target's owner
	// is not a trusted party.
	for _, path := range filesUnder(t, root) {
		if strings.HasSuffix(path, archive.ManifestName) || strings.HasSuffix(path, archive.ChecksumsName) {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if strings.Contains(string(body), `"w1"`) || strings.Contains(string(body), "OPEN") {
			t.Fatalf("%s holds the plaintext", path)
		}
	}

	reader := archive.NewReader(store, realCipher())
	description, err := reader.Describe(ctx, request.Prefix)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	items, _ := archive.FindEntity("work_items")

	// Without a key at all.
	err = reader.Records(ctx, description, items, secret.Bytes{}, func(archive.Record) error { return nil })
	if err == nil {
		t.Fatal("an encrypted archive was read without a key")
	}
	// And with the wrong one.
	err = reader.Records(ctx, description, items, keyNamed("somebody_elses"), func(archive.Record) error { return nil })
	if err == nil {
		t.Fatal("an encrypted archive was read with the wrong key")
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("the refusal is not a typed validation error: %v", err)
	}

	// The manifest, deliberately, is readable without any of that - §8.1 lists archives at a
	// target that may have been lost along with the database.
	if description.Manifest.Encryption.KeyID != "bk_2026_a" {
		t.Fatalf("the manifest does not name its key: %+v", description.Manifest.Encryption)
	}
}

// BK-2, second half: after a rotation the old archive still opens with the key its manifest names.
func TestAfterARotationAnOldArchiveOpensWithTheKeyItsManifestNames(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := archiveStore(t, root)
	held := keyring{"bk_2026_a": keyNamed("bk_2026_a")}

	source := newWorld()
	source.put("labels", "l1", theHour(1), map[string]any{"colour_token": "label.red"})

	old := fullRun(t, "bk_2026_a", theHour(2))
	if _, err := archive.NewWriter(store, realCipher(), source).Write(ctx, old, source); err != nil {
		t.Fatalf("the first run: %v", err)
	}

	// The rotation: a new key is added and the old one is kept, because keeping it is what makes
	// a rotation a configuration change rather than a data migration.
	held["bk_2026_b"] = keyNamed("bk_2026_b")

	source.put("labels", "l2", theHour(3), map[string]any{"colour_token": "label.blue"})
	fresh := fullRun(t, "bk_2026_b", theHour(4))
	if _, err := archive.NewWriter(store, realCipher(), source).Write(ctx, fresh, source); err != nil {
		t.Fatalf("the run after the rotation: %v", err)
	}

	reader := archive.NewReader(store, realCipher())
	for _, prefix := range []string{old.Prefix, fresh.Prefix} {
		description, err := reader.Describe(ctx, prefix)
		if err != nil {
			t.Fatalf("describe %s: %v", prefix, err)
		}
		key, holds := held[description.Manifest.Encryption.KeyID]
		if !holds {
			t.Fatalf("%s names a key the installation does not hold: %s",
				prefix, description.Manifest.Encryption.KeyID)
		}

		labels, _ := archive.FindEntity("labels")
		var read int
		if err := reader.Records(ctx, description, labels, key, func(archive.Record) error {
			read++
			return nil
		}); err != nil {
			t.Fatalf("%s under %s: %v", prefix, description.Manifest.Encryption.KeyID, err)
		}
		if read == 0 {
			t.Fatalf("%s opened and held nothing", prefix)
		}
	}

	// And the new key does not open the old archive, which is what "the key its manifest names"
	// means rather than "any key the installation holds".
	description, err := reader.Describe(ctx, old.Prefix)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	labels, _ := archive.FindEntity("labels")
	if err := reader.Records(ctx, description, labels, held["bk_2026_b"],
		func(archive.Record) error { return nil }); err == nil {
		t.Fatal("the rotated-in key opened an archive written before the rotation")
	}
}

// ── BK-3 ──────────────────────────────────────────────────────────────────────────────────────

// BK-3: an incremental chain of ten runs including deletions reproduces the source state exactly.
//
// The deletions are the point. A chain that carried only upserts would restore every object that
// ever existed, and the ones deleted three runs ago would come back looking current.
func TestAnIncrementalChainOfTenRunsReproducesTheSource(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := archiveStore(t, root)
	key := keyNamed("bk_2026_a")

	source := newWorld()
	attachment := source.attach("the bytes of an attachment written in the first run")
	source.put("containers", "c1", theHour(0), map[string]any{"kind": "HUB"})
	source.put("work_items", "w1", theHour(0), map[string]any{"state": "OPEN"})
	source.put("item_attachments", "a1", theHour(0), map[string]any{"blobs": []archive.Blob{attachment}})

	writer := archive.NewWriter(store, realCipher(), source)

	var chain []string
	var previous archive.Request
	for run := range 10 {
		at := theHour(run + 1)
		mutate(source, run, at)

		request := chainedRun(t, run, at, previous, chain)
		if _, err := writer.Write(ctx, request, source); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		chain = append([]string{request.Prefix}, chain...)
		previous = request
	}
	// Replay: the chain, oldest first, applied line by line - which is what a restore does.
	reader := archive.NewReader(store, realCipher())
	walked, err := reader.Chain(ctx, chain[0])
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if len(walked) != 10 {
		t.Fatalf("the chain is %d archives long, want 10", len(walked))
	}

	restored := map[string]map[string]map[string]any{}
	for i := len(walked) - 1; i >= 0; i-- {
		whole := map[string]bool{}
		for _, name := range walked[i].Manifest.Whole {
			whole[name] = true
		}

		for _, entity := range archive.Entities() {
			if entity.Optional {
				continue
			}
			// The rule the manifest carries: for an entity written whole, the newest archive of
			// the chain holds the whole truth, and the copies in older archives are superseded
			// rather than merged. A restore therefore replaces the set instead of adding to it -
			// which is what makes a delete visible for a table that cannot carry a tombstone.
			if whole[entity.Name] {
				restored[entity.Name] = map[string]map[string]any{}
			}
			err := reader.Records(ctx, walked[i], entity, key, func(record archive.Record) error {
				if restored[entity.Name] == nil {
					restored[entity.Name] = map[string]map[string]any{}
				}
				if record.Op == archive.OpDelete {
					delete(restored[entity.Name], record.ID)
					return nil
				}
				row := maps.Clone(record.Data)
				if len(record.Blobs) > 0 {
					row["blobs"] = record.Blobs
				}
				restored[entity.Name][record.ID] = row
				return nil
			})
			if err != nil {
				t.Fatalf("reading %s from %s: %v", entity, walked[i].Prefix, err)
			}
		}
	}

	compare(t, source, restored)

	// And every attachment that survived is resolvable from the chain, wherever it was first
	// stored. That is the other half of "an incremental does not re-transfer a file that did not
	// change".
	for id, row := range source.rows["item_attachments"] {
		blobs, carries := row["blobs"].([]archive.Blob)
		if !carries {
			continue
		}
		for _, blob := range blobs {
			stream, err := reader.Medium(ctx, walked, blob.Digest, key)
			if err != nil {
				t.Fatalf("the attachment of %s is nowhere in the chain: %v", id, err)
			}
			content, err := io.ReadAll(stream)
			_ = stream.Close()
			if err != nil {
				t.Fatalf("reading the attachment of %s: %v", id, err)
			}
			if string(content) != source.blobs[blob.Digest] {
				t.Fatalf("the attachment of %s came back as %q", id, content)
			}
		}
	}
}

// mutate is the ten runs' worth of change: creations, updates, deletions, a deletion of something
// created two runs earlier, and a new attachment half way through.
func mutate(source *world, run int, at time.Time) {
	switch run {
	case 0:
		source.put("work_items", "w2", at, map[string]any{"state": "OPEN"})
		source.put("containers", "c2", at, map[string]any{"kind": "COLLECTION"})
	case 1:
		source.put("work_items", "w1", at, map[string]any{"state": "DONE"})
		source.put("work_items", "w3", at, map[string]any{"state": "OPEN"})
	case 2:
		// Deleted three runs before the end, and it must not come back.
		source.remove("work_items", "w2", at)
	case 3:
		source.put("labels", "l1", at, map[string]any{"colour_token": "label.red"})
		source.put("work_items", "w4", at, map[string]any{"state": "OPEN"})
	case 4:
		// Created and deleted within the chain: it appears in one archive and is denied by a
		// later one, and the end state must have no trace of it.
		source.put("work_items", "w5", at, map[string]any{"state": "OPEN"})
	case 5:
		source.remove("work_items", "w5", at)
		second := source.attach("a second attachment, added half way through the chain")
		source.put("item_attachments", "a2", at, map[string]any{"blobs": []archive.Blob{second}})
	case 6:
		source.put("containers", "c2", at, map[string]any{"kind": "COLLECTION", "archived": true})
	case 7:
		source.remove("containers", "c2", at)
		source.remove("labels", "l1", at)
	case 8:
		source.put("work_items", "w3", at, map[string]any{"state": "DONE"})
	case 9:
		source.put("comments", "k1", at, map[string]any{"body_length": float64(42)})
		source.remove("item_attachments", "a1", at)
	}
}

// compare states the acceptance criterion literally: the restored state and the source state are
// the same rows with the same fields, and neither has anything the other has not.
func compare(t *testing.T, source *world, restored map[string]map[string]map[string]any) {
	t.Helper()

	for _, entity := range archive.Entities() {
		want, got := source.rows[entity.Name], restored[entity.Name]
		for id, row := range want {
			back, found := got[id]
			if !found {
				t.Errorf("%s/%s is in the source and not in the restore", entity.Name, id)
				continue
			}
			for field, value := range row {
				if field == "blobs" {
					continue // compared by content further down
				}
				if fmt.Sprint(back[field]) != fmt.Sprint(value) {
					t.Errorf("%s/%s.%s came back as %v, want %v", entity.Name, id, field, back[field], value)
				}
			}
		}
		for id := range got {
			if _, alive := want[id]; !alive {
				t.Errorf("%s/%s came back from the dead", entity.Name, id)
			}
		}
	}
}

// ── BK-4 ──────────────────────────────────────────────────────────────────────────────────────

// BK-4: the golden archives in the repository import.
//
// They are committed rather than generated, and that is the whole value of them: a test that writes
// an archive and reads it back proves the writer and the reader agree with each other, which they
// always will. These prove that today's reader agrees with a writer from another release - and they
// will keep proving it when that writer no longer exists.
//
// Every directory under golden/ is imported, so adding one at a major release is committing a
// directory rather than editing this file.
func TestTheGoldenArchivesImport(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.Abs("golden")
	if err != nil {
		t.Fatalf("locating the golden archives: %v", err)
	}
	archives, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading the golden archives: %v", err)
	}
	if len(archives) == 0 {
		t.Fatal("no golden archive in the repository - BK-4 has nothing to import")
	}

	store := archiveStore(t, root)
	reader := archive.NewReader(store, realCipher())

	for _, entry := range archives {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			importGolden(ctx, t, reader, entry.Name())
		})
	}
}

func importGolden(ctx context.Context, t *testing.T, reader *archive.Reader, prefix string) {
	t.Helper()

	description, err := reader.Describe(ctx, prefix)
	if err != nil {
		t.Fatalf("the golden archive does not describe: %v", err)
	}
	if description.Manifest.FormatVersion < archive.MinimumReadableFormatVersion {
		t.Fatalf("format version %d is below what this build reads", description.Manifest.FormatVersion)
	}
	if !description.Complete {
		t.Fatal("the golden archive has no checksums.txt")
	}
	if err := reader.Verify(ctx, prefix); err != nil {
		t.Fatalf("the golden archive does not verify: %v", err)
	}

	counted := map[string]int64{}
	for _, entity := range archive.Entities() {
		err := reader.Records(ctx, description, entity, secret.Bytes{}, func(record archive.Record) error {
			counted[entity.Name]++
			if record.Op == archive.OpUpsert && record.Data == nil {
				return fmt.Errorf("%s/%s came back without its row", entity.Name, record.ID)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("reading %s: %v", entity, err)
		}
	}

	for entity, want := range description.Manifest.Counts {
		if counted[entity] != want {
			t.Errorf("%s: %d records read, %d in the manifest", entity, counted[entity], want)
		}
	}
	if counted["work_items"] == 0 || counted["containers"] == 0 {
		t.Fatalf("the golden archive is empty of the things it exists to carry: %v", counted)
	}
}

// The golden archive of the format this build writes is produced by this test with
// HUBTASK_WRITE_GOLDEN=1, and is otherwise only read. Regenerating it is a deliberate act at a
// release rather than something a test does when it happens to disagree - an archive that rewrites
// itself when the reader changes proves nothing.
//
// It is unencrypted on purpose. Format compatibility and encryption are separate promises with
// separate tests, and the key that would open it would have to live in this repository - where it
// would sit in every secret scanner's report forever, protecting fabricated data.
func TestWriteTheGoldenArchive(t *testing.T) {
	if os.Getenv("HUBTASK_WRITE_GOLDEN") != "1" {
		t.Skip("set HUBTASK_WRITE_GOLDEN=1 to regenerate the golden archive")
	}
	ctx := context.Background()

	root, err := filepath.Abs("golden")
	if err != nil {
		t.Fatalf("locating the golden archive: %v", err)
	}
	current := fmt.Sprintf("v%d", archive.FormatVersion)
	if err := os.RemoveAll(filepath.Join(root, current)); err != nil {
		t.Fatalf("clearing the old golden archive: %v", err)
	}
	store := archiveStore(t, root)

	source := newWorld()
	source.put("tenants", tenantID.String(), theHour(0), map[string]any{"locale": "en", "plan": "SELF_HOSTED"})
	source.put("accounts", "0198f0a0-0000-7000-8000-00000000ac01", theHour(0), map[string]any{"role": "OWNER"})
	source.put("containers", "0198f0a0-0000-7000-8000-0000000c0001", theHour(0),
		map[string]any{"kind": "HUB", "order_key": "a0"})
	source.put("containers", "0198f0a0-0000-7000-8000-0000000c0002", theHour(1),
		map[string]any{"kind": "COLLECTION", "order_key": "a1"})
	source.put("labels", "0198f0a0-0000-7000-8000-00000001ab01", theHour(1),
		map[string]any{"colour_token": "label.red"})
	source.put("work_items", "0198f0a0-0000-7000-8000-000000010001", theHour(2),
		map[string]any{"state": "OPEN", "type": "TASK", "order_key": "a0"})
	source.put("work_items", "0198f0a0-0000-7000-8000-000000010002", theHour(2),
		map[string]any{"state": "DONE", "type": "TASK", "order_key": "a1"})
	source.put("comments", "0198f0a0-0000-7000-8000-00000000c001", theHour(3),
		map[string]any{"body_length": float64(23)})

	request := archive.Request{
		ArchiveID:      shared.MustParseID("0198f0a0-0000-7000-8000-00000000901d"),
		Prefix:         current,
		Scope:          archive.Scope{Kind: archive.ScopeTenant, ID: tenantID.String()},
		Mode:           archive.ModeFull,
		SnapshotAt:     theHour(4),
		SchemaVersion:  schemaVersion(t),
		ProductVersion: "0.4.5",
		Encryption:     archive.Encryption{Mode: archive.EncryptionNone},
	}
	if _, err := archive.NewWriter(store, realCipher(), source).Write(ctx, request, source); err != nil {
		t.Fatalf("writing the golden archive: %v", err)
	}
	t.Logf("golden archive written to %s - commit it", filepath.Join(root, current))
}

// ── Rule 10 ───────────────────────────────────────────────────────────────────────────────────

// The exporter is held to rule 10 exactly as everything else is, and here it matters twice: the
// manifest is the one member that is never encrypted, so whatever reaches it is readable by
// whoever holds the storage.
func TestNothingOfTheContentReachesTheManifestOrTheChecksums(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := archiveStore(t, root)

	const title = "Ring the oncologist about the biopsy result"
	const person = "dr.hoffmann@example.org"

	source := newWorld()
	source.put("work_items", "w1", theHour(1), map[string]any{"title": title, "state": "OPEN"})
	source.put("accounts", "ac1", theHour(1), map[string]any{"email": person})

	request := fullRun(t, "bk_2026_a", theHour(2))
	manifest, err := archive.NewWriter(store, realCipher(), source).Write(ctx, request, source)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, name := range []string{archive.ManifestName, archive.ChecksumsName} {
		body, err := os.ReadFile(filepath.Join(root, request.Prefix, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for _, content := range []string{title, person, "oncologist", "hoffmann"} {
			if strings.Contains(string(body), content) {
				t.Fatalf("%s carries user content: %q", name, content)
			}
		}
	}

	// What the manifest does carry is a number per entity, which is the whole of what a dry run
	// and an operator need.
	if manifest.Counts["work_items"] != 1 || manifest.Counts["accounts"] != 1 {
		t.Fatalf("the counts are not what the manifest is for: %v", manifest.Counts)
	}
	// And the printed error of a refusal quotes nothing of what it refused.
	broken := archive.Record{ID: "w1", Op: archive.OpDelete, UpdatedAt: theHour(1),
		Data: map[string]any{"title": title}}
	refusal := broken.Validate()
	if refusal == nil {
		t.Fatal("a tombstone carrying a payload was accepted")
	}
	if strings.Contains(fmt.Sprintf("%+v", refusal), title) {
		t.Fatalf("a refusal quoted what it refused: %v", refusal)
	}
}

// The exporter writes no log line at all, which is the cheapest way to keep rule 10 on a code path
// that handles every row a tenant has. A metric and a span belong to the job that drives it (E-05),
// where they can be labelled by target and run rather than by anything a row contains.
func TestTheExporterLogsNothing(t *testing.T) {
	root, err := filepath.Abs("../../core/application/archive")
	if err != nil {
		t.Fatalf("locating the package: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading the package: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		for _, forbidden := range []string{`"log"`, `"log/slog"`, `"fmt".Print`} {
			if strings.Contains(string(body), forbidden) {
				t.Errorf("%s imports %s - the exporter sees every row a tenant has (rule 10)",
					entry.Name(), forbidden)
			}
		}
	}
}

// ── Shared machinery ──────────────────────────────────────────────────────────────────────────

// theHour is a fixed clock: hour n of the day the archive format was written. Fixed, because an
// archive whose bytes depend on when the test ran is one that cannot be committed.
func theHour(n int) time.Time {
	return time.Date(2026, 8, 26, n, 0, 0, 0, time.UTC)
}

func fullRun(t *testing.T, keyID string, at time.Time) archive.Request {
	t.Helper()
	return archive.Request{
		ArchiveID:      freshArchiveID(t, at),
		Prefix:         archive.Name(tenantID, at, archive.ModeFull),
		Scope:          archive.Scope{Kind: archive.ScopeTenant, ID: tenantID.String()},
		Mode:           archive.ModeFull,
		SnapshotAt:     at,
		SchemaVersion:  schemaVersion(t),
		ProductVersion: "0.4.5",
		Encryption:     archive.Encryption{Mode: archive.EncryptionAES256GCM, KeyID: keyID},
		Key:            keyNamed(keyID),
		IncludeMedia:   true,
	}
}

// chainedRun is run n of the chain: the first is full, the rest continue their predecessor.
func chainedRun(t *testing.T, run int, at time.Time, previous archive.Request, ancestors []string) archive.Request {
	t.Helper()

	request := fullRun(t, "bk_2026_a", at)
	if run == 0 {
		return request
	}
	request.Mode = archive.ModeIncremental
	request.Prefix = archive.Name(tenantID, at, archive.ModeIncremental)
	request.Since = previous.SnapshotAt
	request.ParentID = previous.ArchiveID.String()
	request.ParentPrefix = previous.Prefix
	request.Ancestors = ancestors
	return request
}

func freshArchiveID(t *testing.T, at time.Time) shared.ID {
	t.Helper()
	return shared.MustParseID(fmt.Sprintf("0198f0a0-0000-7000-8000-0000%08x", at.Unix()&0xFFFFFFFF))
}

// schemaVersion is the migration the repository stands at, read from the file names rather than
// written down twice.
func schemaVersion(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs("../../db/migrations")
	if err != nil {
		t.Fatalf("locating the migrations: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading the migrations: %v", err)
	}
	latest := ""
	for _, entry := range entries {
		if name, _, found := strings.Cut(entry.Name(), "_"); found && name > latest {
			latest = name
		}
	}
	if latest == "" {
		t.Fatal("no migrations found")
	}
	return latest
}

func filesUnder(t *testing.T, root string) []string {
	t.Helper()

	var found []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return found
}
