// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Writing the archive (E-09, audit.md §5). What is under test is the format: which members exist,
// in which order they are closed, what the manifest says about the chain it covers, and that a
// target which fails leaves no archive claiming to be complete.

// targetStore is a backup target in memory, keyed exactly as the real ones are.
type targetStore struct {
	written  map[string][]byte
	order    []string
	failOn   string
	openErr  error
	putCalls int
}

func newTargetStore() *targetStore {
	return &targetStore{written: map[string][]byte{}}
}

func (s *targetStore) OpenTarget(context.Context, shared.ID, shared.ID) (backupstorage.Store, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	return s, nil
}

func (s *targetStore) Put(_ context.Context, key string, content io.Reader) (int64, error) {
	s.putCalls++
	if s.failOn != "" && strings.HasSuffix(key, s.failOn) {
		return 0, errors.New("the target went away")
	}
	written, err := io.ReadAll(content)
	if err != nil {
		return 0, err
	}
	s.written[key] = written
	s.order = append(s.order, key)
	return int64(len(written)), nil
}

func (s *targetStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, shared.ErrNotFound
}
func (s *targetStore) List(context.Context, string) ([]backupstorage.Entry, error) { return nil, nil }
func (s *targetStore) Stat(context.Context, string) (backupstorage.Entry, error) {
	return backupstorage.Entry{}, shared.ErrNotFound
}
func (s *targetStore) Delete(context.Context, string) error { return nil }

// member answers one written member by its name, whatever prefix it landed under.
func (s *targetStore) member(t *testing.T, name string) []byte {
	t.Helper()
	for key, content := range s.written {
		if strings.HasSuffix(key, "/"+name) {
			return content
		}
	}
	t.Fatalf("the archive has no %s: %v", name, s.order)
	return nil
}

// keyless is an installation with no master key configured, which is the default one.
type keyless struct{}

func (keyless) Seal(context.Context, secret.Secret, crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{}, shared.ErrUnavailable.WithDetail("crypto.no_encryption_key")
}

func (keyless) Open(context.Context, crypto.Sealed, crypto.Purpose) (secret.Secret, error) {
	return secret.Secret{}, shared.ErrUnavailable.WithDetail("crypto.unknown_key")
}
func (keyless) ActiveKeyID() string { return "" }

// keyed is an installation that holds one. The "sealing" is a marker rather than a cipher: what
// this test asks is whether the signature is written and what it says about itself, not whether
// AES works - that is infrastructure/crypto's own test.
type keyed struct{ purposes []crypto.Purpose }

func (k *keyed) Seal(_ context.Context, value secret.Secret, purpose crypto.Purpose) (crypto.Sealed, error) {
	k.purposes = append(k.purposes, purpose)
	return crypto.Sealed{KeyID: "key-1", Ciphertext: []byte("sealed:" + value.Reveal())}, nil
}

func (k *keyed) Open(context.Context, crypto.Sealed, crypto.Purpose) (secret.Secret, error) {
	return secret.Secret{}, shared.ErrUnavailable
}
func (k *keyed) ActiveKeyID() string { return "key-1" }

func newArchivist(records []repository.Record, store *targetStore, encryptor crypto.Encryptor) Archivist {
	return Archivist{
		Trail: &trailStore{records: records}, Targets: store, Encryptor: encryptor,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), ProductVersion: "0.4.5",
	}
}

func archiveRequest(format Format) ArchiveRequest {
	return ArchiveRequest{
		ExportID: shared.MustParseID("0192f000-0000-7000-8000-0000000000c2"),
		TenantID: tenantID, TargetID: targetID,
		Period: repository.Period{From: now.Add(-24 * time.Hour), To: now},
		Format: format,
	}
}

func threeRecords() []repository.Record {
	return []repository.Record{
		record("0192f000-0000-7000-8000-0000000000b1", 11, func(r *repository.Record) {
			r.Hash = []byte{0x11}
		}),
		record("0192f000-0000-7000-8000-0000000000b2", 12, func(r *repository.Record) {
			r.Hash = []byte{0x12}
		}),
		record("0192f000-0000-7000-8000-0000000000b3", 13, func(r *repository.Record) {
			r.Hash = []byte{0x13}
		}),
	}
}

func TestAnExportWritesItsEntriesItsManifestAndItsChecksums(t *testing.T) {
	store := newTargetStore()
	written, err := newArchivist(threeRecords(), store, keyless{}).
		Write(context.Background(), archiveRequest(FormatJSONL))
	if err != nil {
		t.Fatalf("writing: %v", err)
	}

	if len(store.order) != 3 {
		t.Fatalf("the archive holds %v", store.order)
	}
	// The checksums are written last: an export without them is an unfinished export, whatever
	// else is lying next to it.
	if !strings.HasSuffix(store.order[len(store.order)-1], archive.ChecksumsName) {
		t.Errorf("the archive closes with %s", store.order[len(store.order)-1])
	}

	lines := strings.Split(strings.TrimSpace(string(store.member(t, "entries.jsonl"))), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d entries were written", len(lines))
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("the first line is not JSON: %v", err)
	}
	if first["action"] != "container.created" || first["hash"] != "11" {
		t.Errorf("the first entry came out as %v", first)
	}
	// The projection is the one every channel reads, so a SECRET field is absent here too.
	changes, _ := first["changes"].([]any)
	for _, change := range changes {
		if field, _ := change.(map[string]any)["field"].(string); field == "token" {
			t.Error("a SECRET field reached the archive")
		}
	}

	// The manifest says which stretch of the chain the archive covers, which is what lets an
	// auditor compare it with `:verify` over the same period.
	if written.Entries != 3 || written.FirstSeq != 11 || written.LastSeq != 13 {
		t.Errorf("the manifest describes %+v", written)
	}
	if written.FirstHash != "11" || written.LastHash != "13" {
		t.Errorf("the chain range came out as %s..%s", written.FirstHash, written.LastHash)
	}
	if written.Signed {
		t.Error("an installation with no key claims to have signed the export")
	}
	if written.Encryption != "none" {
		t.Errorf("the manifest claims the encryption %q", written.Encryption)
	}

	checksums, err := archive.ParseChecksums(strings.NewReader(string(store.member(t, archive.ChecksumsName))))
	if err != nil {
		t.Fatalf("the checksums are unreadable: %v", err)
	}
	for _, name := range []string{"entries.jsonl", archive.ManifestName} {
		if _, found := checksums.Digest(name); !found {
			t.Errorf("%s is not covered by the checksums", name)
		}
	}
	if err := checksums.Verify("entries.jsonl", strings.NewReader(string(store.member(t, "entries.jsonl")))); err != nil {
		t.Errorf("the entries do not match their own checksum: %v", err)
	}
}

func TestACsvExportIsAFixedSetOfColumns(t *testing.T) {
	store := newTargetStore()
	if _, err := newArchivist(threeRecords(), store, keyless{}).
		Write(context.Background(), archiveRequest(FormatCSV)); err != nil {
		t.Fatalf("writing: %v", err)
	}

	rows, err := csv.NewReader(strings.NewReader(string(store.member(t, "entries.csv")))).ReadAll()
	if err != nil {
		t.Fatalf("the file is not CSV: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("%d rows were written, want a header and three entries", len(rows))
	}
	if strings.Join(rows[0], ",") != strings.Join(exportColumns, ",") {
		t.Errorf("the header is %v", rows[0])
	}
	if rows[1][1] != "11" || rows[1][3] != "container.created" {
		t.Errorf("the first entry came out as %v", rows[1])
	}
	// The masked changes travel as JSON inside one column rather than as columns that would come
	// and go with the data.
	if !strings.Contains(rows[1][len(exportColumns)-2], `"field":"status"`) {
		t.Errorf("the changes column holds %q", rows[1][len(exportColumns)-2])
	}
}

func TestAnInstallationWithAKeySignsTheManifest(t *testing.T) {
	store, encryptor := newTargetStore(), &keyed{}
	written, err := newArchivist(threeRecords(), store, encryptor).
		Write(context.Background(), archiveRequest(FormatJSONL))
	if err != nil {
		t.Fatalf("writing: %v", err)
	}

	if !written.Signed {
		t.Fatal("an installation holding a key wrote no signature")
	}
	var signature ExportSignature
	if err := json.Unmarshal(store.member(t, exportSignatureName), &signature); err != nil {
		t.Fatalf("the signature is unreadable: %v", err)
	}
	if signature.KeyID != "key-1" || signature.Over != archive.ManifestName {
		t.Errorf("the signature says %+v", signature)
	}
	if signature.Digest == "" || signature.Value == "" {
		t.Error("the signature carries neither a digest nor a sealed value")
	}
	// The purpose binds the signature to this export, so one lifted from another archive fails to
	// open rather than appearing to belong here.
	if len(encryptor.purposes) != 1 ||
		!strings.HasSuffix(string(encryptor.purposes[0]), archiveRequest(FormatJSONL).ExportID.String()) {
		t.Errorf("the signature was sealed under %v", encryptor.purposes)
	}
	// The checksums close over the signature too: an archive that grew a member after they were
	// written is an archive somebody has been editing.
	checksums, err := archive.ParseChecksums(strings.NewReader(string(store.member(t, archive.ChecksumsName))))
	if err != nil {
		t.Fatalf("the checksums are unreadable: %v", err)
	}
	if _, found := checksums.Digest(exportSignatureName); !found {
		t.Error("the signature is not covered by the checksums")
	}
}

// A target that goes away mid-stream leaves no archive that looks complete, and no goroutine
// waiting to write into a pipe nobody reads.
func TestATargetThatFailsLeavesNoFinishedArchive(t *testing.T) {
	store := newTargetStore()
	store.failOn = "entries.jsonl"

	_, err := newArchivist(threeRecords(), store, keyless{}).
		Write(context.Background(), archiveRequest(FormatJSONL))
	if err == nil {
		t.Fatal("an export whose target refused the entries reported success")
	}
	if len(store.written) != 0 {
		t.Errorf("the failed export left %v behind", store.order)
	}
}

func TestAnArchiveIsNamedAfterItsTenantAndItsPeriod(t *testing.T) {
	name := ArchiveName(tenantID, repository.Period{
		From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if !strings.Contains(name, "20260801T000000Z") || !strings.Contains(name, "20260901T000000Z") {
		t.Errorf("the archive is named %q, and an operator listing a target cannot see what it covers", name)
	}
	if !strings.HasPrefix(name, "hubtask-audit-") {
		t.Errorf("the archive is named %q", name)
	}
}

func TestAnEmptyPeriodStillWritesAnArchive(t *testing.T) {
	store := newTargetStore()
	written, err := newArchivist(nil, store, keyless{}).
		Write(context.Background(), archiveRequest(FormatJSONL))
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	// "Nothing happened in that period" is an answer an auditor asked for, and an archive that
	// refused to be written would leave them unable to tell it from an export that never ran.
	if written.Entries != 0 || written.FirstSeq != 0 {
		t.Errorf("an empty period produced %+v", written)
	}
	if len(store.order) != 3 {
		t.Errorf("an empty export wrote %v", store.order)
	}
}
