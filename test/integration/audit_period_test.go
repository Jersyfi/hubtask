// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	auditservice "github.com/Jersyfi/hubtask/core/application/service/audit"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	auditadapter "github.com/Jersyfi/hubtask/infrastructure/audit"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

// The trail's period against the real boundary (audit.md §3): a month past the period goes whole,
// with each workspace's count; a tenant-wide hold keeps the month for every workspace in it; the
// default partition is only counted; and a workspace's chain still verifies once its oldest month
// went, the removal entry included.
//
// In its own database: the drop removes every aged month of the installation, and the shared
// database's trail belongs to the other tests.
func TestAMonthPastThePeriodGoesWholeAndAHoldKeepsIt(t *testing.T) {
	ctx := context.Background()
	admin, _ := databaseMigratedTo(ctx, t, "audit_period", 153)
	app := appPoolOf(ctx, t, "audit_period")
	unitOfWork := postgres.NewUnitOfWork(app)
	partitions := postgres.NewAuditPartitionRepository()
	sink := postgres.NewAuditSink(generator{t})

	kept, held, aside := freshID(t), freshID(t), freshID(t)
	for _, tenant := range []shared.ID{kept, held, aside} {
		if _, err := admin.Exec(ctx, `INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Period')`,
			tenant.String(), slugOf(tenant)); err != nil {
			t.Fatalf("seeding the tenant: %v", err)
		}
	}

	now := time.Now().UTC()
	oldMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -15, 0)
	var oldPartition string
	if err := admin.QueryRow(ctx, `SELECT ensure_audit_partition($1::date)`, oldMonth).Scan(&oldPartition); err != nil || oldPartition == "" {
		t.Fatalf("ensuring the old month (%q, %v)", oldPartition, err)
	}

	write := func(tenant shared.ID, at time.Time, action port.Action) {
		t.Helper()
		entry := auditEntry(tenant, action, "period")
		entry.OccurredAt = at
		if err := unitOfWork.Within(ctx, persistence.Scope{TenantID: tenant},
			func(ctx context.Context) error { return sink.Append(ctx, entry) }); err != nil {
			t.Fatalf("appending: %v", err)
		}
	}
	inOld := oldMonth.AddDate(0, 0, 14)
	write(kept, inOld, "container.created")
	write(kept, inOld.Add(time.Hour), "container.renamed")
	write(held, inOld, "container.created")
	write(kept, now.Add(-time.Hour), "container.renamed")
	write(held, now.Add(-time.Hour), "container.renamed")
	// A month with no partition of its own lands in the default, which no drop reaches.
	write(aside, oldMonth.AddDate(0, -5, 0), "container.created")

	drop := func() []repository.TrailDrop {
		t.Helper()
		var drops []repository.TrailDrop
		if err := unitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
			var err error
			drops, err = partitions.DropAged(ctx, 400)
			return err
		}); err != nil {
			t.Fatalf("dropping: %v", err)
		}
		return drops
	}
	partitionExists := func() bool {
		t.Helper()
		var count int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM pg_class WHERE relname = $1`, oldPartition).Scan(&count); err != nil {
			t.Fatalf("reading the catalogue: %v", err)
		}
		return count == 1
	}

	// A hold on one workspace keeps the whole month, for the other workspace as well.
	hold := freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO legal_hold (id, tenant_id, scope_kind, reason, placed_by)
		VALUES ($1, $2, 'TENANT', 'litigation', $3)`, hold.String(), held.String(), freshID(t).String()); err != nil {
		t.Fatalf("placing the hold: %v", err)
	}
	outcomes := map[string]int{}
	for _, d := range drop() {
		outcomes[d.Outcome]++
		if d.Outcome == repository.TrailHeld && (d.Partition != oldPartition || !d.Month.Equal(oldMonth)) {
			t.Errorf("held %s of %s, want %s of %s", d.Partition, d.Month, oldPartition, oldMonth)
		}
		if d.Outcome == repository.TrailDefaultAged && d.Entries != 1 {
			t.Errorf("the default partition holds %d aged entries, want 1", d.Entries)
		}
	}
	if outcomes[repository.TrailHeld] != 1 || outcomes[repository.TrailDropped] != 0 || outcomes[repository.TrailDefaultAged] != 1 {
		t.Fatalf("under the hold: %v", outcomes)
	}
	if !partitionExists() {
		t.Fatal("the held month went")
	}

	// The hold released, the month goes, with each workspace's count.
	if _, err := admin.Exec(ctx, `UPDATE legal_hold SET released_at = now(), released_by = placed_by WHERE id = $1`,
		hold.String()); err != nil {
		t.Fatalf("releasing the hold: %v", err)
	}
	counts := map[shared.ID]int64{}
	for _, d := range drop() {
		if d.Outcome != repository.TrailDropped {
			continue
		}
		if d.Partition != oldPartition || !d.Month.Equal(oldMonth) {
			t.Errorf("dropped %s of %s, want %s of %s", d.Partition, d.Month, oldPartition, oldMonth)
		}
		counts[d.Workspace] = d.Entries
	}
	if len(counts) != 2 || counts[kept] != 2 || counts[held] != 1 {
		t.Fatalf("the counts were %v, want 2 for one workspace and 1 for the other", counts)
	}
	if partitionExists() {
		t.Fatal("the month is still there")
	}
	if again := drop(); len(again) != 1 || again[0].Outcome != repository.TrailDefaultAged {
		t.Errorf("a second run answered %v, want only the default's count", again)
	}

	// The workspace's own trail records what left it, and its chain verifies from what remains.
	record := auditservice.RecordTrailRemoval{Audit: sink, Clock: portclock.Fixed(now)}
	if err := unitOfWork.Within(ctx, persistence.Scope{TenantID: kept}, func(ctx context.Context) error {
		_, err := record.Execute(ctx, auditservice.TrailRemoval{
			TenantID: kept, Partition: oldPartition, Month: oldMonth.Format("2006-01"), Entries: 2,
		})
		return err
	}); err != nil {
		t.Fatalf("recording the removal: %v", err)
	}
	verifier := auditservice.VerifyAuditChain{
		Trail: auditTrailRepo(), Chain: auditadapter.Links{}, Authorizer: permissive{},
		Audit: sink, UnitOfWork: unitOfWork, Clock: portclock.Fixed(now),
	}
	found, err := verifier.Execute(ctx, auditActor(kept), repository.Period{
		From: oldMonth.AddDate(-1, 0, 0), To: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if !found.Valid || found.Checked != 2 {
		t.Fatalf("the chain after the month went: valid %v, %d checked, gaps %v, broken at %d",
			found.Valid, found.Checked, found.Gaps, found.FirstBrokenSeq)
	}
}

// appPoolOf connects as the application role to a database of the test's own.
func appPoolOf(ctx context.Context, t *testing.T, name string) *pgxpool.Pool {
	t.Helper()
	db := dbtest.Start(t)
	dsn := regexp.MustCompile(`/[^/?]+(\?|$)`).ReplaceAllString(db.AppDSN, "/"+name+"$1")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to %s: %v", name, err)
	}
	t.Cleanup(pool.Close)
	return pool
}
