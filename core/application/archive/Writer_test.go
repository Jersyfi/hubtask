// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const tenant = "0198f0a0-0000-7000-8000-0000000000aa"

func request() Request {
	return Request{
		ArchiveID:      shared.MustParseID("0198f0a0-0000-7000-8000-000000000001"),
		Prefix:         Name(shared.MustParseID(tenant), snapshot(), ModeFull),
		Scope:          Scope{Kind: ScopeTenant, ID: tenant},
		Mode:           ModeFull,
		SnapshotAt:     snapshot(),
		SchemaVersion:  "0014",
		ProductVersion: "0.4.5",
		Encryption:     Encryption{Mode: EncryptionAES256GCM, KeyID: "bk_2026_a"},
		Key:            key(),
		IncludeMedia:   true,
	}
}

func plaintext() Request {
	unencrypted := request()
	unencrypted.Encryption = Encryption{Mode: EncryptionNone}
	unencrypted.Key = secret.Bytes{}
	return unencrypted
}

func blobOf(content string) Blob {
	sum := sha256.Sum256([]byte(content))
	return Blob{Digest: hex.EncodeToString(sum[:]), Bytes: int64(len(content))}
}

func TestAnArchiveHasEveryMemberTheFormatNames(t *testing.T) {
	store, source := newStore(), newSource()
	source.rows["containers"] = []Record{
		{ID: "c1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"kind": "HUB"}},
	}
	writer := NewWriter(store, &reversible{}, newBlobs())

	manifest, err := writer.Write(t.Context(), request(), source)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	prefix := request().Prefix + "/"
	for _, entity := range Entities() {
		if entity.Optional {
			continue // include_audit was off
		}
		if _, found := store.object(prefix + DataName(entity.Name)); !found {
			t.Errorf("no member for %s", entity)
		}
	}
	if _, found := store.object(prefix + ManifestName); !found {
		t.Error("no manifest")
	}
	if _, found := store.object(prefix + ChecksumsName); !found {
		t.Error("no checksums.txt")
	}
	if manifest.Counts["containers"] != 1 {
		t.Fatalf("counts: %v", manifest.Counts)
	}
}

// The order is the format: the data files, then the media they reference, then the manifest that
// names them, then the file that says the run finished.
func TestChecksumsAreWrittenLastAndTheManifestSecondToLast(t *testing.T) {
	store, source := newStore(), newSource()
	blobs := newBlobs()
	attachment := blobOf("the bytes of an attachment")
	blobs.content[attachment.Digest] = "the bytes of an attachment"
	source.rows["media_objects"] = []Record{
		{ID: "m1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"bytes": attachment.Bytes},
			Blobs: []Blob{attachment}},
	}

	if _, err := NewWriter(store, &reversible{}, blobs).Write(t.Context(), request(), source); err != nil {
		t.Fatalf("write: %v", err)
	}

	written := store.keys()
	last, secondToLast := written[len(written)-1], written[len(written)-2]
	if !strings.HasSuffix(last, ChecksumsName) {
		t.Fatalf("the last member is %s, not %s", last, ChecksumsName)
	}
	if !strings.HasSuffix(secondToLast, ManifestName) {
		t.Fatalf("the second to last member is %s, not %s", secondToLast, ManifestName)
	}
	// And the media come after every data file: §5 fetches them after the snapshot, by the
	// checksums the snapshot referenced.
	lastData := slices.IndexFunc(slices.Clone(written), func(string) bool { return false })
	for i, key := range written {
		if strings.Contains(key, "/"+DataPrefix) {
			lastData = i
		}
	}
	firstMedia := slices.IndexFunc(written, func(key string) bool { return strings.Contains(key, "/"+MediaPrefix) })
	if firstMedia < 0 || firstMedia < lastData {
		t.Fatalf("media at %d, last data file at %d:\n%s", firstMedia, lastData, strings.Join(written, "\n"))
	}
}

// Encrypted before it leaves the process, not at the target (§3, §4).
func TestAnEncryptedArchiveCarriesNoPlaintextToTheTarget(t *testing.T) {
	store, source := newStore(), newSource()
	source.rows["comments"] = []Record{
		{ID: "k1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"body_length": 12}},
	}
	cipher := &reversible{}

	if _, err := NewWriter(store, cipher, newBlobs()).Write(t.Context(), request(), source); err != nil {
		t.Fatalf("write: %v", err)
	}

	member, _ := store.object(request().Prefix + "/" + DataName("comments"))
	if bytes.Contains(member, []byte(`"id":"k1"`)) {
		t.Fatalf("the member reached the target in the clear:\n%s", member)
	}
	if len(cipher.sealed) == 0 {
		t.Fatal("nothing was sealed")
	}
	// And the manifest deliberately did not go through the cipher: §8.1 has to read it without
	// the archive key.
	manifest, _ := store.object(request().Prefix + "/" + ManifestName)
	if !bytes.Contains(manifest, []byte(`"format_version"`)) {
		t.Fatalf("the manifest is not readable at the target:\n%s", manifest)
	}
}

// Every member is bound to its archive and its path, so that one lifted out of an archive and
// dropped into another stops opening.
func TestEveryMemberIsBoundToItsPlace(t *testing.T) {
	store, source := newStore(), newSource()
	cipher := &reversible{}

	if _, err := NewWriter(store, cipher, newBlobs()).Write(t.Context(), request(), source); err != nil {
		t.Fatalf("write: %v", err)
	}

	wanted := "backup/archive:" + request().ArchiveID.String() + "/" + DataName("containers")
	if !slices.Contains(cipher.sealed, crypto.Purpose(wanted)) {
		t.Fatalf("the containers member was not bound to its path: %v", cipher.sealed[:3])
	}
	seen := map[string]bool{}
	for _, purpose := range cipher.sealed {
		if seen[string(purpose)] {
			t.Fatalf("two members share a purpose: %s", purpose)
		}
		seen[string(purpose)] = true
	}
}

func TestAnUnencryptedArchiveGoesThroughNoCipherAtAll(t *testing.T) {
	store, source := newStore(), newSource()
	source.rows["labels"] = []Record{
		{ID: "l1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"colour_token": "label.red"}},
	}
	cipher := &reversible{}

	if _, err := NewWriter(store, cipher, newBlobs()).Write(t.Context(), plaintext(), source); err != nil {
		t.Fatalf("write: %v", err)
	}

	if len(cipher.sealed) != 0 {
		t.Fatalf("an unencrypted archive still called the cipher: %v", cipher.sealed)
	}
	member, _ := store.object(plaintext().Prefix + "/" + DataName("labels"))
	if !bytes.Contains(member, []byte(`"id":"l1"`)) {
		t.Fatalf("the member is not the records:\n%s", member)
	}
}

// The same attachment on ten items is one file. That is what content addressing is for.
func TestAMediumReferencedTwiceIsStoredOnce(t *testing.T) {
	store, source, blobs := newStore(), newSource(), newBlobs()
	attachment := blobOf("one attachment, two items")
	blobs.content[attachment.Digest] = "one attachment, two items"
	source.rows["item_attachments"] = []Record{
		{ID: "a1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{}, Blobs: []Blob{attachment}},
		{ID: "a2", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{}, Blobs: []Blob{attachment}},
	}

	manifest, err := NewWriter(store, &reversible{}, blobs).Write(t.Context(), request(), source)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	if manifest.MediaCount != 1 {
		t.Fatalf("media_count %d, want 1", manifest.MediaCount)
	}
	if len(blobs.opened) != 1 {
		t.Fatalf("the object store was read %d times for one file", len(blobs.opened))
	}
	if _, found := store.object(request().Prefix + "/" + MediaName(attachment.Digest)); !found {
		t.Fatal("the medium is not at its content address")
	}
}

// An incremental run does not re-transfer a file that did not change.
func TestAMediumTheChainAlreadyHoldsIsNotTransferredAgain(t *testing.T) {
	store, source, blobs := newStore(), newSource(), newBlobs()
	attachment := blobOf("unchanged since the full run")
	blobs.content[attachment.Digest] = "unchanged since the full run"

	parent := Name(shared.MustParseID(tenant), snapshot().Add(-24*time.Hour), ModeFull)
	if _, err := store.Put(t.Context(), parent+"/"+MediaName(attachment.Digest),
		strings.NewReader("already there")); err != nil {
		t.Fatalf("seeding the parent: %v", err)
	}

	source.rows["item_attachments"] = []Record{
		{ID: "a1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{}, Blobs: []Blob{attachment}},
	}
	incremental := request()
	incremental.Mode = ModeIncremental
	incremental.Since = snapshot().Add(-24 * time.Hour)
	incremental.ParentID = "0198f0a0-0000-7000-8000-000000000000"
	incremental.ParentPrefix = parent
	incremental.Ancestors = []string{parent}
	incremental.Prefix = Name(shared.MustParseID(tenant), snapshot(), ModeIncremental)

	manifest, err := NewWriter(store, &reversible{}, blobs).Write(t.Context(), incremental, source)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	if manifest.MediaCount != 0 {
		t.Fatalf("media_count %d - the chain already held it", manifest.MediaCount)
	}
	if len(blobs.opened) != 0 {
		t.Fatalf("the object store was read for a file the chain already has: %v", blobs.opened)
	}
	// The record still names it: a restore resolves a digest by searching the chain.
	if _, found := store.object(incremental.Prefix + "/" + MediaName(attachment.Digest)); found {
		t.Fatal("the medium was written into the incremental after all")
	}
}

// A record referencing a medium the object store no longer has is a refusal. An archive silently
// missing an attachment is one whose restore is a surprise.
func TestAMissingMediumStopsTheRun(t *testing.T) {
	store, source, blobs := newStore(), newSource(), newBlobs()
	gone := blobOf("deleted from the object store between the snapshot and the transfer")
	source.rows["item_attachments"] = []Record{
		{ID: "a1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{}, Blobs: []Blob{gone}},
	}

	_, err := NewWriter(store, &reversible{}, blobs).Write(t.Context(), request(), source)
	if err == nil {
		t.Fatal("a missing medium was written over")
	}
	if got := detail(t, err); got != CodeMediaMissing {
		t.Fatalf("detail code %q", got)
	}
}

func TestMediaAreLeftOutWhenTheScheduleSaysSo(t *testing.T) {
	store, source, blobs := newStore(), newSource(), newBlobs()
	attachment := blobOf("an attachment nobody asked for")
	blobs.content[attachment.Digest] = "an attachment nobody asked for"
	source.rows["item_attachments"] = []Record{
		{ID: "a1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{}, Blobs: []Blob{attachment}},
	}
	without := request()
	without.IncludeMedia = false

	manifest, err := NewWriter(store, &reversible{}, blobs).Write(t.Context(), without, source)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if manifest.MediaCount != 0 || len(blobs.opened) != 0 {
		t.Fatalf("media were transferred anyway: %d", manifest.MediaCount)
	}
}

func TestTheAuditTrailIsWrittenOnlyWhenItIsAskedFor(t *testing.T) {
	store, source := newStore(), newSource()
	with := request()
	with.IncludeAudit = true

	if _, err := NewWriter(store, &reversible{}, newBlobs()).Write(t.Context(), with, source); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, found := store.object(with.Prefix + "/" + DataName("audit")); !found {
		t.Fatal("include_audit was on and there is no audit member")
	}

	without := newStore()
	if _, err := NewWriter(without, &reversible{}, newBlobs()).Write(t.Context(), request(), newSource()); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, found := without.object(request().Prefix + "/" + DataName("audit")); found {
		t.Fatal("include_audit was off and the audit member was written")
	}
}

// An incremental asks the source for a period, not for everything.
func TestAnIncrementalAsksForWhatChanged(t *testing.T) {
	store, source := newStore(), newSource()
	since := snapshot().Add(-24 * time.Hour)
	incremental := request()
	incremental.Mode = ModeIncremental
	incremental.Since = since
	incremental.ParentID = "0198f0a0-0000-7000-8000-000000000000"

	if _, err := NewWriter(store, &reversible{}, newBlobs()).Write(t.Context(), incremental, source); err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, entity := range Entities() {
		if entity.Optional {
			continue
		}
		if asked, found := source.asked[entity.Name]; !found || !asked.Equal(since) {
			t.Fatalf("%s was asked from %v, want %v", entity, asked, since)
		}
	}
}

// The manifest's checksums are of the bytes at the target, which is what :verify can check without
// the archive key.
func TestTheManifestChecksumsAreTheBytesAtTheTarget(t *testing.T) {
	store, source := newStore(), newSource()
	source.rows["work_items"] = []Record{
		{ID: "w1", Op: OpUpsert, UpdatedAt: snapshot(), Data: map[string]any{"state": "OPEN"}},
	}

	manifest, err := NewWriter(store, &reversible{}, newBlobs()).Write(t.Context(), request(), source)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, file := range manifest.Files {
		stored, found := store.object(request().Prefix + "/" + file.Path)
		if !found {
			t.Fatalf("%s is in the manifest and not at the target", file.Path)
		}
		if int64(len(stored)) != file.Bytes {
			t.Fatalf("%s: %d bytes at the target, %d in the manifest", file.Path, len(stored), file.Bytes)
		}
		if got := digestOf(string(stored)); got != file.SHA256 {
			t.Fatalf("%s: the manifest names another checksum", file.Path)
		}
	}
}

// checksums.txt closes over the manifest, which is the one checksum the manifest cannot carry.
func TestChecksumsCoverTheManifestAndEveryDataFile(t *testing.T) {
	store, source := newStore(), newSource()

	manifest, err := NewWriter(store, &reversible{}, newBlobs()).Write(t.Context(), request(), source)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	raw, _ := store.object(request().Prefix + "/" + ChecksumsName)
	checksums, err := ParseChecksums(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	stored, _ := store.object(request().Prefix + "/" + ManifestName)
	if err := checksums.Verify(ManifestName, bytes.NewReader(stored)); err != nil {
		t.Fatalf("the manifest is not covered: %v", err)
	}
	for _, file := range manifest.Files {
		member, _ := store.object(request().Prefix + "/" + file.Path)
		if err := checksums.Verify(file.Path, bytes.NewReader(member)); err != nil {
			t.Fatalf("%s is not covered: %v", file.Path, err)
		}
	}
}

// A run that dies leaves no checksums.txt, which is exactly how the next reader tells an
// unfinished archive from a finished one.
func TestARunThatDiesLeavesNoCommitPoint(t *testing.T) {
	store, source := newStore(), newSource()
	store.failAt = DataName("comments")

	_, err := NewWriter(store, &reversible{}, newBlobs()).Write(t.Context(), request(), source)
	if err == nil {
		t.Fatal("a target that refused a member produced an archive")
	}
	if _, found := store.object(request().Prefix + "/" + ChecksumsName); found {
		t.Fatal("a failed run wrote checksums.txt")
	}
	if _, found := store.object(request().Prefix + "/" + ManifestName); found {
		t.Fatal("a failed run wrote a manifest")
	}
}

// A source that fails does not produce a truncated member that a manifest then vouches for.
func TestASourceThatFailsFailsTheRun(t *testing.T) {
	store, source := newStore(), newSource()
	source.failure = shared.ErrUnavailable.WithDetail("dependency.database")

	_, err := NewWriter(store, &reversible{}, newBlobs()).Write(t.Context(), request(), source)
	if err == nil {
		t.Fatal("a failing source produced an archive")
	}
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("the source's failure was swallowed: %v", err)
	}
}

func TestARequestThatCannotProduceARestorableArchiveIsRefused(t *testing.T) {
	cases := map[string]func(*Request){
		"archive_id":             func(r *Request) { r.ArchiveID = "" },
		"prefix":                 func(r *Request) { r.Prefix = "" },
		"mode":                   func(r *Request) { r.Mode = "PARTIAL" },
		"snapshot_at":            func(r *Request) { r.SnapshotAt = time.Time{} },
		"since":                  func(r *Request) { r.Mode = ModeIncremental },
		"since_after_snapshot":   func(r *Request) { r.Mode = ModeIncremental; r.Since = snapshot().Add(time.Hour) },
		"key_missing":            func(r *Request) { r.Key = secret.Bytes{} },
		"key_without_encryption": func(r *Request) { r.Encryption = Encryption{Mode: EncryptionNone} },
		"encryption_mode":        func(r *Request) { r.Encryption.Mode = "ROT13" },
	}

	for reason, breakIt := range cases {
		t.Run(reason, func(t *testing.T) {
			broken := request()
			breakIt(&broken)

			_, err := NewWriter(newStore(), &reversible{}, newBlobs()).Write(t.Context(), broken, newSource())
			if err == nil {
				t.Fatalf("%s was accepted", reason)
			}
			var domainErr *shared.Error
			errors.As(err, &domainErr)
			if domainErr.DetailCode != CodeArchiveRequestInvalid || domainErr.Params["reason"] != reason {
				t.Fatalf("%s: %v", reason, err)
			}
		})
	}
}

// A prefix that tries to leave the archive's directory is refused here as well as at the adapter.
// Defence in depth is cheaper than certainty about every future caller.
func TestAPrefixCannotClimbOut(t *testing.T) {
	climbing := request()
	climbing.Prefix = "../../etc"

	_, err := NewWriter(newStore(), &reversible{}, newBlobs()).Write(t.Context(), climbing, newSource())
	if err == nil {
		t.Fatal("a prefix leaving the archive's directory was accepted")
	}
}
