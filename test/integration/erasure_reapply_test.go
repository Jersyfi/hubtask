// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-BAK-08 check 3, the erasure half, against a real database: a workspace "rewound" to before
// three erasures - the accounts back, one case missing, one open, one missing under a hold placed
// in the rewound period and placed again first. The operator carries the erasures out again through
// the registry, as the application role: the deleted person is gone again, the anonymised one reads
// "former user", the held one is restricted rather than erased, every case is completed at its own
// moment, the trail records each run, and a second run changes nothing (backup-restore.md §8.5
// step 5).
func TestTheErasuresOfARewoundPeriodAreCarriedOutAgain(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	tenant := auditTenant(ctx, t)
	point := created.Add(-24 * time.Hour)
	full, anonymised, held := freshID(t), freshID(t), freshID(t)
	fullCase, anonymisedCase, heldCase := freshID(t), freshID(t), freshID(t)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding the recovered workspace: %v\n%s", err, sql)
		}
	}
	for _, account := range []shared.ID{full, anonymised, held} {
		exec(`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Erika')`,
			account.String(), tenant.String())
	}
	// Received before the point, so the recovered workspace holds it open.
	exec(`INSERT INTO data_subject_request (id, tenant_id, subject_account_id, kind, status, erasure_mode, received_at, due_at)
	      VALUES ($1, $2, $3, 'ERASURE', 'IN_PROGRESS', 'ANONYMIZE', $4, $5)`,
		anonymisedCase.String(), tenant.String(), anonymised.String(), point.Add(-time.Hour), point.Add(29*24*time.Hour))
	// The hold the rewound period placed, placed again before the erasures.
	exec(`INSERT INTO legal_hold (id, tenant_id, scope_kind, scope_id, reason, placed_by, placed_at)
	      VALUES ($1, $2, 'ACCOUNT', $3, 'Dispute', $4, $5)`,
		freshID(t).String(), tenant.String(), held.String(), authorA.String(), point.Add(time.Hour))

	completedAt := func(hours int) time.Time { return point.Add(time.Duration(hours) * time.Hour) }
	record := func(id, subject shared.ID, mode domain.ErasureMode, received time.Time, completed int) map[string]any {
		return map[string]any{
			"id": id.String(), "erasure_mode": string(mode), "subject_account_id": subject.String(),
			"received_at":  received.Format(time.RFC3339Nano),
			"due_at":       received.Add(30 * 24 * time.Hour).Format(time.RFC3339Nano),
			"completed_at": completedAt(completed).Format(time.RFC3339Nano),
			"notes":        "asked by mail",
		}
	}
	erasures := []any{
		record(fullCase, full, domain.ModeFullDelete, point.Add(time.Hour), 2),
		record(anonymisedCase, anonymised, domain.ModeAnonymize, point.Add(-time.Hour), 3),
		record(heldCase, held, domain.ModeFullDelete, point.Add(2*time.Hour), 4),
	}

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	registry, err := usecase.NewRegistry(nil, privacyservice.ReapplyErasures{
		Requests: privacyRepo(), Recovered: privacyRepo(), Eraser: eraserFor(t),
		Workspaces: postgres.NewAdminTenantRepository(), StepUp: freshProof{},
		UnitOfWork: unitOfWork, Clock: portclock.Fixed(created.Add(96 * time.Hour)),
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantB, AccountID: authorB, AccountName: "Operator",
		Scopes: []string{"admin:tenants"},
	}
	run := func() map[string]string {
		t.Helper()
		out, err := registry.Invoke(ctx, privacyservice.ReapplyErasuresName, operator, usecase.Input{
			"tenant_id": tenant.String(), "recovery_point": point.Format(time.RFC3339Nano),
			"erasures": erasures, "step_up_token": "hbt_sup_fresh",
		})
		if err != nil {
			t.Fatalf("re-applying: %v", err)
		}
		outcomes := map[string]string{}
		rows, _ := out["erasures"].([]any)
		for _, row := range rows {
			fields := row.(usecase.Output)
			outcomes[fields.String("id")] = fields.String("outcome")
			if fields.String("id") == heldCase.String() &&
				fields.String("outcome") == string(privacyservice.ErasureReapplied) && fields["kept"] != true {
				t.Error("the held erasure does not say a hold kept part of it")
			}
		}
		return outcomes
	}

	first := run()
	for _, id := range []shared.ID{fullCase, anonymisedCase, heldCase} {
		if first[id.String()] != string(privacyservice.ErasureReapplied) {
			t.Fatalf("the first run answered %v", first)
		}
	}

	var accounts int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM account WHERE id = $1`, full.String()).Scan(&accounts); err != nil || accounts != 0 {
		t.Errorf("the deleted person is back (%d, %v)", accounts, err)
	}
	var name, status string
	if err := admin.QueryRow(ctx, `SELECT display_name, status::text FROM account WHERE id = $1`,
		anonymised.String()).Scan(&name, &status); err != nil || name != privacyservice.FormerUser {
		t.Errorf("the anonymised person reads %q, %s (%v)", name, status, err)
	}
	if err := admin.QueryRow(ctx, `SELECT status::text FROM account WHERE id = $1`, held.String()).Scan(&status); err != nil ||
		status != "RESTRICTED" {
		t.Errorf("the held person is %q (%v), want restricted", status, err)
	}
	for id, hours := range map[shared.ID]int{fullCase: 2, anonymisedCase: 3, heldCase: 4} {
		var caseStatus string
		var completed time.Time
		var notes *string
		if err := admin.QueryRow(ctx,
			`SELECT status, completed_at, notes FROM data_subject_request WHERE id = $1 AND tenant_id = $2`,
			id.String(), tenant.String()).Scan(&caseStatus, &completed, &notes); err != nil {
			t.Fatalf("reading case %s: %v", id, err)
		}
		if caseStatus != "COMPLETED" || !completed.Equal(completedAt(hours)) {
			t.Errorf("case %s is %s at %s", id, caseStatus, completed)
		}
	}
	var entries int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'dsr.erasure_reapplied'
		   AND changes ? 'recovery_point'`, tenant.String()).Scan(&entries); err != nil || entries != 3 {
		t.Errorf("the trail holds %d re-run entries (%v), want 3", entries, err)
	}

	second := run()
	for _, id := range []shared.ID{fullCase, anonymisedCase, heldCase} {
		if second[id.String()] != string(privacyservice.ErasureAlreadyCompleted) {
			t.Errorf("a second run answered %v", second)
		}
	}
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'dsr.erasure_reapplied'`,
		tenant.String()).Scan(&entries); err != nil || entries != 3 {
		t.Errorf("a second run wrote: %d entries (%v)", entries, err)
	}
}

// The repository methods the re-run adds see and write the transaction's workspace only.
func TestTheRecoveredCaseStoreStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	mine := auditTenant(ctx, t)
	other := auditTenant(ctx, t)
	account, request := freshID(t), freshID(t)
	if _, err := admin.Exec(ctx, `INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Erika')`,
		account.String(), mine.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO data_subject_request (id, tenant_id, subject_account_id, kind, status, erasure_mode, received_at, due_at)
	      VALUES ($1, $2, $3, 'ERASURE', 'IN_PROGRESS', 'ANONYMIZE', $4, $4::timestamptz + interval '30 days')`,
		request.String(), mine.String(), account.String(), created); err != nil {
		t.Fatal(err)
	}

	repo := privacyRepo()
	err := postgres.NewUnitOfWork(appPool(ctx, t)).Within(ctx, persistence.Scope{TenantID: other},
		func(ctx context.Context) error {
			if present, err := repo.SubjectExists(ctx, account); err != nil || present {
				t.Errorf("another workspace's account is seen (%v, %v)", present, err)
			}
			if completed, err := repo.CompleteIfOpen(ctx, request, domain.ModeAnonymize, created, account); err != nil || completed {
				t.Errorf("another workspace's case was completed (%v, %v)", completed, err)
			}
			inserted, err := repo.InsertAsItWas(ctx, domain.Request{
				ID: request, ErasureMode: domain.ModeAnonymize, SubjectAccountID: account,
				ReceivedAt: created, DueAt: created.Add(30 * 24 * time.Hour),
			})
			if err != nil || inserted {
				t.Errorf("another workspace's case identifier was written over (%v, %v)", inserted, err)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	var status string
	if err := admin.QueryRow(ctx, `SELECT status FROM data_subject_request WHERE id = $1`, request.String()).
		Scan(&status); err != nil || status != "IN_PROGRESS" {
		t.Errorf("the case is %q (%v)", status, err)
	}
	var count int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM data_subject_request WHERE tenant_id = $1`, other.String()).
		Scan(&count); err != nil || count != 0 {
		t.Errorf("the other workspace holds %d cases (%v)", count, err)
	}
}
