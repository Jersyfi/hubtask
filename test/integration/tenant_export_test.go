// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	backupport "github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	cryptoadapter "github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The tenant export of H-07, end to end against the real boundary: T-20's two-tenant grep, the
// manifest counts against the database, the media checksums, and the redaction - all read off an
// archive written by the real archivist through the real RLS path.

var (
	exportTenantX  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e101")
	exportTenantY  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e102")
	exportAccountX = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e111")
	exportAccountY = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e112")
)

// Markers of tenant Y: if any of these strings appears in tenant X's archive, T-20 fell.
const (
	markerTitle = "TENANT-Y-MARKER-TITLE"
	markerEmail = "tenant-y-marker@example.org"
)

const exportMediaContent = "the attached bytes of tenant X"

// exportTargetStore hands the archivist a local store rooted in a temp dir - the target
// configuration is E-05's business, and this test is about the archive.
type exportTargetStore struct{ store backupport.Store }

func (s exportTargetStore) OpenTarget(context.Context, shared.ID, shared.ID) (backupport.Store, error) {
	return s.store, nil
}

// exportObjects serves the one medium by its storage key.
type exportObjects struct{ key string }

func (o exportObjects) Put(context.Context, storage.Upload) error { return nil }

func (o exportObjects) Get(_ context.Context, key string) (storage.Object, error) {
	if key != o.key {
		return storage.Object{}, shared.ErrNotFound.WithDetail("storage.object_missing")
	}
	content := io.NopCloser(strings.NewReader(exportMediaContent))
	return storage.Object{Content: content, Size: int64(len(exportMediaContent))}, nil
}

func (o exportObjects) Delete(context.Context, string) error { return nil }

func seedExportTenants(ctx context.Context, t *testing.T) string {
	t.Helper()
	admin := adminPool(ctx, t)

	digest := sha256.Sum256([]byte(exportMediaContent))
	checksum := hex.EncodeToString(digest[:])

	statements := []string{
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ('` + exportTenantX.String() + `', 'export-x', 'Export X'),
		        ('` + exportTenantY.String() + `', 'export-y', 'Export Y')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status, password_hash)
		 VALUES ('` + exportAccountX.String() + `', '` + exportTenantX.String() + `', 'USER',
		         'export-x@example.org', 'Export X', 'ACTIVE', '$argon2id$v=19$m=64,t=1,p=1$eA$eA'),
		        ('` + exportAccountY.String() + `', '` + exportTenantY.String() + `', 'USER',
		         '` + markerEmail + `', 'Export Y', 'ACTIVE', NULL)
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000e121', '` + exportTenantX.String() + `', 'HUB',
		         'X hub', 'a0', '` + exportAccountX.String() + `'),
		        ('01936f2a-7c1e-7000-8000-00000000e122', '` + exportTenantY.String() + `', 'HUB',
		         '` + markerTitle + `', 'a0', '` + exportAccountY.String() + `')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000e123', '` + exportTenantX.String() + `',
		         'COLLECTION', '01936f2a-7c1e-7000-8000-00000000e121', 'X collection', 'a0',
		         '` + exportAccountX.String() + `')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key, created_by)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000e124', '` + exportTenantX.String() + `',
		         '01936f2a-7c1e-7000-8000-00000000e123', 'TASK',
		         '/01936f2a-7c1e-7000-8000-00000000e124/', 1, 'An X task', 'a0',
		         '` + exportAccountX.String() + `')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, checksum, usage, status)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000e125', '` + exportTenantX.String() + `',
		         'media/x/one', 'text/plain', ` + itoaLen(exportMediaContent) + `,
		         '` + checksum + `', 'ATTACHMENT', 'READY')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO calendar_feed (id, tenant_id, account_id, token_hash, created_at)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000e126', '` + exportTenantX.String() + `',
		         '` + exportAccountX.String() + `', '\xdeadbeef', now())
		 ON CONFLICT (id) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding the export tenants: %v\n%s", err, statement)
		}
	}
	return checksum
}

func itoaLen(s string) string {
	length := len(s)
	digits := []byte{}
	for length > 0 {
		digits = append([]byte{byte('0' + length%10)}, digits...)
		length /= 10
	}
	return string(digits)
}

func TestTheExportedArchiveHoldsOneTenantWholeAndNothingOfTheNeighbour(t *testing.T) {
	ctx := context.Background()
	checksum := seedExportTenants(ctx, t)

	root := t.TempDir()
	store, err := backupstorage.NewLocalStore(root, "archives")
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}

	archivist := adminservice.TenantExportArchivist{
		Targets:       exportTargetStore{store: store},
		Rows:          postgres.NewBackupExportRepository(postgres.DefaultExportBatch),
		Objects:       exportObjects{key: "media/x/one"},
		Cipher:        cryptoadapter.NewStream(clockadapter.CryptoRandom{}),
		Snapshot:      postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock:         clockadapter.System{},
		SchemaVersion: "test", ProductVersion: "test",
	}

	manifest, err := archivist.Write(ctx, adminservice.ExportArchiveRequest{
		ExportID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e1ff"),
		TenantID: exportTenantX,
		TargetID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e1fe"),
	})
	if err != nil {
		t.Fatalf("writing the archive: %v", err)
	}

	// The archive on disk, whole.
	everything := &bytes.Buffer{}
	fileBytes := map[string][]byte{}
	if err := filepath.WalkDir(filepath.Join(root, "archives"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(filepath.Join(root, "archives"), path)
		fileBytes[filepath.ToSlash(relative)] = content
		everything.Write(content)
		return nil
	}); err != nil {
		t.Fatalf("walking the archive: %v", err)
	}

	// T-20: not one byte of tenant Y. The markers are Y's content; the identifier is Y's row.
	written := everything.String()
	for _, marker := range []string{markerTitle, markerEmail, exportTenantY.String(), exportAccountY.String()} {
		if strings.Contains(written, marker) {
			t.Errorf("the archive contains the neighbour's %q (T-20)", marker)
		}
	}

	// The manifest counts match the database.
	admin := adminPool(ctx, t)
	for entity, table := range map[string]string{
		"work_items": "work_item", "containers": "container",
		"accounts": "account", "media_objects": "media_object",
	} {
		var count int64
		if err := admin.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE tenant_id = $1`, exportTenantX.String(),
		).Scan(&count); err != nil {
			t.Fatalf("counting %s: %v", table, err)
		}
		if manifest.Counts[entity] != count {
			t.Errorf("manifest counts %d %s, the database holds %d", manifest.Counts[entity], entity, count)
		}
	}

	// The media bytes are there, under their own digest, and the digest verifies.
	mediaPath := ""
	for path := range fileBytes {
		if strings.Contains(path, "/media/") {
			mediaPath = path
		}
	}
	if mediaPath == "" {
		t.Fatal("no media object in the archive")
	}
	stored := sha256.Sum256(fileBytes[mediaPath])
	if hex.EncodeToString(stored[:]) != checksum || !strings.HasSuffix(mediaPath, checksum) {
		t.Errorf("the media checksum does not verify: %s", mediaPath)
	}
	if manifest.MediaCount != 1 || manifest.MediaBytes != int64(len(exportMediaContent)) {
		t.Errorf("media counted as (%d, %d)", manifest.MediaCount, manifest.MediaBytes)
	}

	// checksums.txt verifies every member.
	checksumsPath := ""
	for path := range fileBytes {
		if strings.HasSuffix(path, "checksums.txt") {
			checksumsPath = path
		}
	}
	if checksumsPath == "" {
		t.Fatal("no checksums.txt - the archive is not committed")
	}
	prefix := strings.TrimSuffix(checksumsPath, "checksums.txt")
	for _, line := range strings.Split(strings.TrimSpace(string(fileBytes[checksumsPath])), "\n") {
		digest, member, found := strings.Cut(line, "  ")
		if !found {
			t.Fatalf("an unreadable checksum line: %q", line)
		}
		content, held := fileBytes[prefix+member]
		if !held {
			t.Errorf("checksums.txt names %s and the archive does not hold it", member)
			continue
		}
		actual := sha256.Sum256(content)
		if hex.EncodeToString(actual[:]) != digest {
			t.Errorf("%s does not verify against checksums.txt", member)
		}
	}

	// The redaction (tenant-export.md §9): the account line carries no password hash, the feed
	// line no token hash - and the redacted fields appear nowhere in the bytes.
	for _, gone := range []string{"password_hash", "token_hash"} {
		if strings.Contains(written, `"`+gone+`"`) {
			t.Errorf("the archive still carries %q", gone)
		}
	}
	var accountLine map[string]any
	if err := json.Unmarshal(firstLine(fileBytes, prefix+"data/accounts.jsonl"), &accountLine); err != nil {
		t.Fatalf("reading the account line: %v", err)
	}
	if data, _ := accountLine["data"].(map[string]any); data["email"] != "export-x@example.org" {
		t.Errorf("the account lost its data: %v", accountLine)
	}
}

func firstLine(files map[string][]byte, path string) []byte {
	content := files[path]
	if index := bytes.IndexByte(content, '\n'); index > 0 {
		return content[:index]
	}
	return content
}

// A suspended and a leaving workspace export successfully - §5's own promise.
func TestSuspendedAndLeavingWorkspacesExport(t *testing.T) {
	ctx := context.Background()
	seedExportTenants(ctx, t)
	admin := adminPool(ctx, t)

	for _, status := range []string{"SUSPENDED", "PENDING_DELETION"} {
		if _, err := admin.Exec(ctx,
			`UPDATE tenant SET status = $1 WHERE id = $2`, status, exportTenantX.String()); err != nil {
			t.Fatalf("setting the standing: %v", err)
		}

		root := t.TempDir()
		store, err := backupstorage.NewLocalStore(root, "archives")
		if err != nil {
			t.Fatalf("opening the local store: %v", err)
		}
		archivist := adminservice.TenantExportArchivist{
			Targets:       exportTargetStore{store: store},
			Rows:          postgres.NewBackupExportRepository(postgres.DefaultExportBatch),
			Objects:       exportObjects{key: "media/x/one"},
			Cipher:        cryptoadapter.NewStream(clockadapter.CryptoRandom{}),
			Snapshot:      postgres.NewUnitOfWork(appPool(ctx, t)),
			Clock:         clockadapter.System{},
			SchemaVersion: "test", ProductVersion: "test",
		}
		manifest, err := archivist.Write(ctx, adminservice.ExportArchiveRequest{
			ExportID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e2f" + string(rune('0'+len(status)%10))),
			TenantID: exportTenantX,
			TargetID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e1fe"),
		})
		if err != nil {
			t.Fatalf("a %s workspace failed to export: %v", status, err)
		}
		if manifest.Counts["work_items"] < 1 {
			t.Errorf("a %s workspace's archive is empty", status)
		}
	}

	if _, err := admin.Exec(ctx,
		`UPDATE tenant SET status = 'ACTIVE', purge_after = NULL WHERE id = $1`,
		exportTenantX.String()); err != nil {
		t.Fatalf("resetting the standing: %v", err)
	}
}
