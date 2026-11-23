// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// confirmingRegistry is AcknowledgeEmergencyNotice and ListEmergencyNotices the way main.go builds
// them, as the application role, at the moment `at`.
func confirmingRegistry(ctx context.Context, t *testing.T, at time.Time) *usecase.Registry {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	registry, err := usecase.NewRegistry(nil,
		work.AcknowledgeEmergencyNotice{
			Accesses:   postgres.NewEmergencyAccessRepository(),
			Audit:      postgres.NewAuditSink(clockadapter.NewUUIDv7(portclock.Fixed(at))),
			UnitOfWork: unitOfWork, Clock: portclock.Fixed(at),
		}.Descriptor(),
		work.ListEmergencyNotices{
			Accesses: postgres.NewEmergencyAccessRepository(), UnitOfWork: unitOfWork, Clock: portclock.Fixed(at),
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// openedWorld is a private hub whose member (Lena) was told of one emergency access its owner
// opened at `created`.
func openedWorld(ctx context.Context, t *testing.T) (privateWorld, shared.ID) {
	t.Helper()
	w := seedPrivateWorld(ctx, t)
	out, err := openingRegistry(ctx, t, postgres.NewNotificationRepository()).Invoke(ctx, work.OpenPrivateHubName,
		present(w.tenant, w.administrator),
		usecase.Input{"hub_id": w.hub.String(), "reason": "Maria is in hospital", "step_up_token": "hbt_sup_fresh"})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	return w, shared.MustParseID(out.String("id"))
}

// UC-ID-16 check 10 against PostgreSQL: the member who was told confirms once - one row, one trail
// entry naming the hub and the access and not the reason - and their notices say when; the owner
// who opened the hub, a person of the workspace who was not told and the member after the 30 days
// are answered not found.
func TestAMemberConfirmsAnEmergencyAccessOnce(t *testing.T) {
	ctx := context.Background()
	w, accessID := openedWorld(ctx, t)
	confirmedAt := created.Add(10 * time.Minute)
	confirm := func(at time.Time, account shared.ID) error {
		_, err := confirmingRegistry(ctx, t, at).Invoke(ctx, work.AcknowledgeEmergencyNoticeName,
			present(w.tenant, account), usecase.Input{"access_id": accessID.String()})
		return err
	}

	for _, at := range []time.Time{confirmedAt, confirmedAt.Add(time.Minute)} {
		if err := confirm(at, w.member); err != nil {
			t.Fatalf("the member's confirmation at %v: %v", at, err)
		}
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM private_hub_access_acknowledgement
		WHERE access_id = $1 AND account_id = $2 AND acknowledged_at = $3`,
		accessID.String(), w.member.String(), confirmedAt); n != 1 {
		t.Errorf("%d confirmations of the member at the first moment, want one", n)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1
		AND action = 'container.emergency_notice_acknowledged' AND actor_id = $2 AND target_id = $3
		AND changes::text LIKE '%' || $4 || '%' AND NOT (changes::text LIKE '%hospital%')`,
		w.tenant.String(), w.member.String(), w.hub.String(), accessID.String()); n != 1 {
		t.Errorf("%d trail entries naming the hub and the access without the reason, want one", n)
	}

	notices, err := confirmingRegistry(ctx, t, confirmedAt).Invoke(ctx, work.ListEmergencyNoticesName,
		present(w.tenant, w.member), usecase.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if rows, _ := notices["data"].([]usecase.Output); len(rows) != 1 || rows[0]["acknowledged_at"] != confirmedAt {
		t.Errorf("the member's notices: %v, want the access confirmed at %v", notices["data"], confirmedAt)
	}

	stranger := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Max Nachbar')`,
		stranger.String(), w.tenant.String()); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	for name, attempt := range map[string]func() error{
		"the owner who opened it":          func() error { return confirm(confirmedAt, w.administrator) },
		"a person who was not told":        func() error { return confirm(confirmedAt, stranger) },
		"the member after the thirty days": func() error { return confirm(created.Add(31*24*time.Hour), w.member) },
	} {
		if err := attempt(); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%s: %v, want not found", name, err)
		}
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM private_hub_access_acknowledgement WHERE access_id = $1`,
		accessID.String()); n != 1 {
		t.Errorf("%d confirmations after the refusals, want the member's one", n)
	}
}

// The repository as the application role: a confirmation is written once and read back by its
// member; another workspace neither writes one for the same identifiers nor reads one; the
// confirmations go with their access and with the member's account.
func TestConfirmationsStayInTheirWorkspaceAndGoWithTheirAccess(t *testing.T) {
	ctx := context.Background()
	w, accessID := openedWorld(ctx, t)
	repository := postgres.NewEmergencyAccessRepository()
	since := created.Add(-time.Hour)
	noticesIn := func(tenant shared.ID) []workdomain.EmergencyNotice {
		t.Helper()
		var notices []workdomain.EmergencyNotice
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			notices, err = repository.NoticesFor(ctx, w.member, since)
			return err
		}); err != nil {
			t.Fatalf("reading the notices: %v", err)
		}
		return notices
	}
	acknowledgeIn := func(tenant, account shared.ID) (bool, error) {
		var written bool
		err := write(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			written, err = repository.Acknowledge(ctx, accessID, account, created.Add(time.Minute))
			return err
		})
		return written, err
	}
	rows := func() int {
		return countIn(ctx, t, `SELECT count(*) FROM private_hub_access_acknowledgement WHERE access_id = $1`, accessID.String())
	}

	if notices := noticesIn(w.tenant); len(notices) != 1 || notices[0].Acknowledged() {
		t.Fatalf("before confirming: %+v, want the access unconfirmed", notices)
	}

	// Another workspace, the same identifiers: nothing is written there, and nothing read.
	if written, err := acknowledgeIn(tenantB, w.member); err == nil && written {
		t.Error("another workspace wrote a confirmation of this workspace's access")
	}
	if rows() != 0 {
		t.Fatalf("%d confirmations after another workspace's attempt", rows())
	}

	if written, err := acknowledgeIn(w.tenant, w.member); err != nil || !written {
		t.Fatalf("the first confirmation: %v, %v", written, err)
	}
	if written, err := acknowledgeIn(w.tenant, w.member); err != nil || written {
		t.Errorf("the second confirmation: written %v, %v; want nothing written", written, err)
	}
	if notices := noticesIn(w.tenant); len(notices) != 1 || !notices[0].AcknowledgedAt.Equal(created.Add(time.Minute)) {
		t.Errorf("after confirming: %+v, want the moment", notices)
	}
	if notices := noticesIn(tenantB); len(notices) != 0 {
		t.Errorf("another workspace reads %+v", notices)
	}

	// The table's own policy, as the application role: the joins and the other tables' policies
	// above hold the repository's reads, so only this proves 0448's tenant_isolation.
	asTenant(ctx, t, tenantB, func(_ *sqlc.Queries, tx pgx.Tx) {
		var seen int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM private_hub_access_acknowledgement WHERE access_id = $1`,
			accessID.String()).Scan(&seen); err != nil {
			t.Fatalf("reading the table under another workspace: %v", err)
		}
		if seen != 0 {
			t.Errorf("another workspace reads %d confirmations from the table", seen)
		}
		for _, statement := range []string{
			`UPDATE private_hub_access_acknowledgement SET acknowledged_at = now() WHERE access_id = $1`,
			`DELETE FROM private_hub_access_acknowledgement WHERE access_id = $1`,
		} {
			tag, err := tx.Exec(ctx, statement, accessID.String())
			if err != nil {
				t.Fatalf("%s: %v", statement, err)
			}
			if tag.RowsAffected() != 0 {
				t.Errorf("another workspace changed %d confirmations: %s", tag.RowsAffected(), statement)
			}
		}
	})
	if notices := noticesIn(w.tenant); len(notices) != 1 || !notices[0].AcknowledgedAt.Equal(created.Add(time.Minute)) {
		t.Errorf("after another workspace's writes: %+v, want the confirmation unchanged", notices)
	}

	// The member's account goes, and their confirmation with it.
	if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM account WHERE id = $1`, w.member.String()); err != nil {
		t.Fatalf("deleting the member's account: %v", err)
	}
	if rows() != 0 {
		t.Errorf("%d confirmations outlived the member's account", rows())
	}

	// The access goes, and every confirmation of it.
	other := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Maria Parent')`,
		other.String(), w.tenant.String()); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	// Another workspace cannot write a row naming this one, whatever the keys would allow.
	if insertedUnder(ctx, t, tenantB, `INSERT INTO private_hub_access_acknowledgement
		  (tenant_id, access_id, account_id, acknowledged_at) VALUES ($1, $2, $3, now())`,
		w.tenant.String(), accessID.String(), other.String()) {
		t.Error("another workspace wrote a confirmation into this workspace")
	}
	if written, err := acknowledgeIn(w.tenant, other); err != nil || !written {
		t.Fatalf("writing a second confirmation: %v, %v", written, err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM private_hub_access WHERE id = $1`, accessID.String()); err != nil {
		t.Fatalf("deleting the access: %v", err)
	}
	if rows() != 0 {
		t.Errorf("%d confirmations outlived their access", rows())
	}
}

// insertedUnder runs one insert as the application role under a tenant's context and answers whether
// it wrote; a refusal by the row policy is no write.
func insertedUnder(ctx context.Context, t *testing.T, tenant shared.ID, statement string, args ...any) bool {
	t.Helper()
	tx, err := appPool(ctx, t).Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenant.String()); err != nil {
		t.Fatalf("setting the tenant context: %v", err)
	}
	if _, err := tx.Exec(ctx, statement, args...); err != nil {
		return false
	}
	return tx.Commit(ctx) == nil
}

// The archive carries a member's confirmation with its access, and a restore writes it back; one
// whose access the restore did not bring back is skipped rather than refused.
func TestTheArchiveCarriesTheConfirmations(t *testing.T) {
	ctx := context.Background()
	w, accessID := openedWorld(ctx, t)
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		_, err := postgres.NewEmergencyAccessRepository().Acknowledge(ctx, accessID, w.member, created.Add(time.Minute))
		return err
	}); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	const entity = "private_hub_access_acknowledgement"
	var row map[string]any
	for _, each := range exported(ctx, t, w.tenant, 100, entity, time.Time{}) {
		if each.ID == accessID.String()+"/"+w.member.String() {
			row = each.Data
		}
	}
	if row == nil || row["access_id"] != accessID.String() || row["account_id"] != w.member.String() {
		t.Fatalf("the export carries %v", row)
	}
	remove := func() {
		if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM private_hub_access_acknowledgement WHERE access_id = $1`,
			accessID.String()); err != nil {
			t.Fatalf("removing the confirmation: %v", err)
		}
	}
	restore := func(data map[string]any) {
		if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
			_, err := importRepo().Write(ctx, entity, data, false)
			return err
		}); err != nil {
			t.Fatalf("restoring the confirmation: %v", err)
		}
	}
	count := func() int {
		return countIn(ctx, t, `SELECT count(*) FROM private_hub_access_acknowledgement WHERE access_id = $1`, accessID.String())
	}

	remove()
	restore(row)
	if count() != 1 {
		t.Errorf("%d confirmations after the restore, want one", count())
	}

	remove()
	orphan := map[string]any{}
	for key, value := range row {
		orphan[key] = value
	}
	orphan["access_id"] = freshID(t).String()
	restore(orphan)
	if n := countIn(ctx, t, `SELECT count(*) FROM private_hub_access_acknowledgement WHERE account_id = $1`,
		w.member.String()); n != 0 {
		t.Errorf("a confirmation without its access was written")
	}
}
