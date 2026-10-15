// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-BAK-08 check 3, the erasures' source: the cases one workspace completed after the recovery
// point, read from a database standing in for the cluster recovered to the end of the WAL archive -
// for that workspace only, as the application role and as a superuser. A full deletion's case no
// longer names its subject; the reader names it only where exactly one removed account matches the
// moment of the case's trail entry, and reports none or several (backup-restore.md §8.5 step 5).
func TestTheErasuresOfARewoundPeriodAreReadFromTheTemporaryCluster(t *testing.T) {
	ctx := context.Background()
	const name = "hubtask_recovered_erasures"
	adminDSN, appDSN := recoveredCluster(ctx, t, name)
	cluster, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Close()
	app, err := pgxpool.New(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	point := created
	named, anonymised, ambiguous, unresolved, address := freshID(t), freshID(t), freshID(t), freshID(t), freshID(t)
	before, open, access, theirs := freshID(t), freshID(t), freshID(t), freshID(t)
	gone, kept, twinA, twinB := freshID(t), freshID(t), freshID(t), freshID(t)
	// One clock reading per erasure, as the erasure writes its journal row and its trail entry.
	at := func(hours int) time.Time { return point.Add(time.Duration(hours) * time.Hour) }

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := cluster.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding the temporary cluster: %v\n%s", err, sql)
		}
	}
	exec(`INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'erasures-a', 'A'), ($2, 'erasures-b', 'B')`,
		tenantA.String(), tenantB.String())
	exec(`INSERT INTO account (id, tenant_id, display_name, status) VALUES ($1, $2, 'former user', 'ANONYMIZED')`,
		kept.String(), tenantA.String())
	erasureCase := func(tenant, id shared.ID, kind, status, mode string, subject any, email any, received, completed any) {
		t.Helper()
		exec(`INSERT INTO data_subject_request
		        (id, tenant_id, subject_account_id, subject_email, kind, status, erasure_mode, received_at, due_at, completed_at)
		      VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $8::timestamptz + interval '30 days', $9)`,
			id.String(), tenant.String(), subject, email, kind, status, mode, received, completed)
	}
	erasureCase(tenantA, named, "ERASURE", "COMPLETED", "FULL_DELETE", nil, nil, at(1), at(2))
	erasureCase(tenantA, anonymised, "ERASURE", "COMPLETED", "ANONYMIZE", kept.String(), nil, at(1), at(3))
	erasureCase(tenantA, ambiguous, "ERASURE", "COMPLETED", "FULL_DELETE", nil, nil, at(1), at(4))
	erasureCase(tenantA, unresolved, "ERASURE", "COMPLETED", "FULL_DELETE", nil, nil, at(1), at(5))
	erasureCase(tenantA, address, "ERASURE", "COMPLETED", "FULL_DELETE", nil, "nobody@example.org", at(1), at(6))
	erasureCase(tenantA, before, "ERASURE", "COMPLETED", "FULL_DELETE", nil, nil, at(-5), at(-1))
	erasureCase(tenantA, open, "ERASURE", "IN_PROGRESS", "FULL_DELETE", nil, nil, at(1), nil)
	erasureCase(tenantA, access, "ACCESS", "COMPLETED", "", nil, "someone@example.org", at(1), at(2))
	erasureCase(tenantB, theirs, "ERASURE", "COMPLETED", "FULL_DELETE", nil, nil, at(1), at(2))

	journal := func(tenant, account shared.ID, moment time.Time) {
		t.Helper()
		exec(`INSERT INTO deletion_journal (tenant_id, entity, entity_id, deleted_at, reason)
		      VALUES ($1, 'account', $2, $3, 'DSR_ERASURE')`, tenant.String(), account.String(), moment)
	}
	erasedEntry := func(tenant, request shared.ID, moment time.Time) {
		t.Helper()
		sink := postgres.NewAuditSink(generator{t})
		if err := postgres.NewUnitOfWork(app).Within(ctx, persistence.Scope{TenantID: tenant},
			func(ctx context.Context) error {
				return sink.Append(ctx, port.Entry{
					TenantID: tenant, OccurredAt: moment, Action: "dsr.erased",
					Outcome: port.OutcomeSuccess, Severity: port.SeverityCritical,
					ActorKind: shared.ActorSystem, ActorLabel: "the installation",
					TargetType: "data_subject_request", TargetID: request,
				})
			}); err != nil {
			t.Fatalf("writing the trail entry: %v", err)
		}
	}
	// The full deletion: one account removed at the entry's moment.
	journal(tenantA, gone, at(2))
	erasedEntry(tenantA, named, at(2))
	// Two erasures at the same instant: two accounts match one entry.
	journal(tenantA, twinA, at(4))
	journal(tenantA, twinB, at(4))
	erasedEntry(tenantA, ambiguous, at(4))
	// An entry with no removal at its moment.
	erasedEntry(tenantA, unresolved, at(5))
	// The other workspace's removal at the first moment must not name anybody here.
	journal(tenantB, freshID(t), at(5))
	erasedEntry(tenantB, theirs, at(2))

	for role, dsn := range map[string]string{"the application role": appDSN, "a superuser": adminDSN} {
		read, err := postgres.ReadRecoveredErasures(ctx, dsn, tenantA, point, 30*time.Second)
		if err != nil {
			t.Fatalf("reading as %s: %v", role, err)
		}
		got := map[shared.ID]postgres.RecoveredErasure{}
		for _, recovered := range read {
			got[recovered.Case.ID] = recovered
		}
		if len(got) != 5 {
			t.Fatalf("as %s the reader answered %d cases: %+v", role, len(got), read)
		}
		for _, left := range []shared.ID{before, open, access, theirs} {
			if _, there := got[left]; there {
				t.Errorf("as %s the reader answered %s, which it must leave out", role, left)
			}
		}
		if c := got[named]; c.Case.SubjectAccountID != gone || c.Unresolved != "" || !c.Case.CompletedAt.Equal(at(2)) {
			t.Errorf("as %s the full deletion reads %+v", role, c)
		}
		if c := got[anonymised]; c.Case.SubjectAccountID != kept || c.Unresolved != "" {
			t.Errorf("as %s the anonymisation reads %+v", role, c)
		}
		if c := got[ambiguous]; c.Unresolved != postgres.SubjectAmbiguous || !c.Case.SubjectAccountID.IsZero() {
			t.Errorf("as %s two matching removals read %+v", role, c)
		}
		if c := got[unresolved]; c.Unresolved != postgres.SubjectUnresolved || !c.Case.SubjectAccountID.IsZero() {
			t.Errorf("as %s no matching removal reads %+v", role, c)
		}
		if c := got[address]; c.Unresolved != "" || !c.Case.SubjectAccountID.IsZero() {
			t.Errorf("as %s a case about an address reads %+v", role, c)
		}
	}
}
