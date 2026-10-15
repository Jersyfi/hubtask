// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	cryptoadapter "github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-PRV-06 checks 3 and 4: an installation-wide case, recorded and started by an operator holding
// `admin:tenants`, is carried out by its own job - which holds no scope - into one archive with the
// person's data from every workspace they are in, and every workspace visited records it.

// memberWithWork seeds a workspace in which the address has an account, an entry and a comment, and
// answers the identifiers the archive must carry.
func memberWithWork(ctx context.Context, t *testing.T, email string) (tenant shared.ID, carried []string) {
	t.Helper()
	tenant = fanOutTenant(ctx, t, email)
	admin := adminPool(ctx, t)
	var account string
	if err := admin.QueryRow(ctx,
		`SELECT id FROM account WHERE tenant_id = $1 AND email = $2`, tenant.String(), email).Scan(&account); err != nil {
		t.Fatalf("reading the person's account: %v", err)
	}
	hub, collection, entry, comment := freshID(t), freshID(t), freshID(t), freshID(t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by) VALUES ($1, $2, 'HUB', 'Office', 'a0', $3)`,
			[]any{hub.String(), tenant.String(), account}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Errands', 'a0', $4)`,
			[]any{collection.String(), tenant.String(), hub.String(), account}},
		{`INSERT INTO work_item (id, tenant_id, collection_id, type, title, path, depth, order_key, created_by)
		  VALUES ($1, $2, $3, 'TASK', 'Renew the lease', $4, 1, 'a0', $5)`,
			[]any{entry.String(), tenant.String(), collection.String(), work.RootPath(entry), account}},
		{`INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, 'Signed today')`,
			[]any{comment.String(), tenant.String(), entry.String(), account}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the person's work: %v\n%s", err, statement.sql)
		}
	}
	return tenant, []string{account, entry.String(), comment.String()}
}

func TestAnInstallationWideCaseIsCarriedOutByItsOwnJob(t *testing.T) {
	ctx := context.Background()
	email := "everywhere-" + freshID(t).String() + "@example.org"
	operatorsOwn := fanOutTenant(ctx, t, "olga-"+freshID(t).String()+"@example.org")
	first, firstCarried := memberWithWork(ctx, t, email)
	second, secondCarried := memberWithWork(ctx, t, email)

	pool := appPool(ctx, t)
	unitOfWork := postgres.NewUnitOfWork(pool)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	cases := privacyservice.Cases{
		Requests: privacyRepo(), Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Subjects: privacyRepo(), Jobs: postgres.NewQueue(ids, clockadapter.System{}), Authorizer: permissive{},
		Audit: postgres.NewAuditSink(generator{t}), UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: generator{t},
	}
	registry, err := usecase.NewRegistry(nil,
		privacyservice.CreateDataSubjectRequest{Cases: cases}.Descriptor(),
		privacyservice.UpdateDataSubjectRequest{Cases: cases}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: operatorsOwn, AccountID: readerE, AccountName: "Olga Operator",
		Scopes: []string{"privacy:manage", "admin:tenants"},
	}
	recorded, err := registry.Invoke(ctx, privacyservice.CreateDataSubjectRequestName, operator, usecase.Input{
		"kind": string(domain.KindAccess), "scope": string(domain.ScopeInstallation), "subject_email": email,
		"target_id": freshID(t).String(),
	})
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	caseID := shared.ID(recorded.String("id"))
	if _, err := registry.Invoke(ctx, privacyservice.UpdateDataSubjectRequestName, operator, usecase.Input{
		"request_id": caseID.String(), "status": string(domain.StatusInProgress),
	}); err != nil {
		t.Fatalf("starting: %v", err)
	}

	root := t.TempDir()
	store, err := backupstorage.NewLocalStore(root, "archives")
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	performer := privacyservice.Performer{
		Requests: privacyRepo(),
		Exporter: privacyservice.Exporter{
			Requests: privacyRepo(), Subjects: privacyRepo(), Accounts: postgres.NewAccountRepository(),
			Targets: exportTargetStore{store: store},
			Rows:    postgres.NewBackupExportRepository(postgres.DefaultExportBatch), Objects: exportObjects{key: "none"},
			Cipher: cryptoadapter.NewStream(clockadapter.CryptoRandom{}), Audit: postgres.NewAuditSink(generator{t}),
			Snapshot: unitOfWork, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: generator{t},
			SchemaVersion: "test", ProductVersion: "test",
		},
		UnitOfWork: unitOfWork, Clock: clockadapter.System{},
	}

	// The job, as the worker runs it: the case's workspace, and no scope of anybody's.
	done, err := performer.Perform(ctx, privacyservice.PerformInput{RequestID: caseID, TenantID: operatorsOwn})
	if err != nil {
		t.Fatalf("the job was refused: %v", err)
	}
	if done.Status != domain.StatusCompleted || !strings.HasSuffix(done.ResultArchive, ".zip") {
		t.Fatalf("the case came back %s with the archive %q", done.Status, done.ResultArchive)
	}
	if strings.Contains(done.ResultArchive, "everywhere") {
		t.Errorf("the archive's name carries the address: %q", done.ResultArchive)
	}

	// One archive at the target, holding each workspace's archive with the person's account, entry
	// and comment in it.
	var objects []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			objects = append(objects, path)
		}
		return err
	}); err != nil {
		t.Fatalf("listing the target: %v", err)
	}
	if len(objects) != 1 || !strings.HasSuffix(objects[0], done.ResultArchive) {
		t.Fatalf("the target holds %v, want the one archive %s", objects, done.ResultArchive)
	}
	content, err := os.ReadFile(objects[0])
	if err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	bundled, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	parts := map[string]*bytes.Buffer{}
	for _, member := range bundled.File {
		tenant, _, _ := strings.Cut(member.Name, "/")
		if parts[tenant] == nil {
			parts[tenant] = &bytes.Buffer{}
		}
		reader, err := member.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", member.Name, err)
		}
		_, _ = io.Copy(parts[tenant], reader)
		_ = reader.Close()
	}
	if len(parts) != 2 {
		t.Fatalf("the archive holds the workspaces %v, want two", keysOfParts(parts))
	}
	for tenant, carried := range map[shared.ID][]string{first: firstCarried, second: secondCarried} {
		part := parts[tenant.String()]
		if part == nil {
			t.Fatalf("the archive holds nothing of the workspace %s", tenant)
		}
		for _, id := range carried {
			if !strings.Contains(part.String(), id) {
				t.Errorf("the workspace %s's archive misses %s", tenant, id)
			}
		}
	}

	// Every workspace visited records the collection in its own trail; the operator's own, which
	// the person is not a member of, holds none.
	for _, tenant := range []shared.ID{first, second} {
		if entries := countIn(ctx, t,
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'dsr.collected' AND target_id = $2`,
			tenant.String(), caseID.String()); entries != 1 {
			t.Errorf("the workspace %s recorded %d collections, want 1", tenant, entries)
		}
	}
}

func keysOfParts(parts map[string]*bytes.Buffer) []string {
	keys := make([]string, 0, len(parts))
	for key := range parts {
		keys = append(keys, key)
	}
	return keys
}
