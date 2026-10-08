// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A legal hold wins over an erasure as far as it reaches (UC-PRV-03 checks 3, 4, 6, 10, 13;
// data-protection.md §4.1), scope by scope and mode by mode, through the job's own performer
// against the real schema as the application role. The person has a task they created in a
// collection under a hub, assigned to them, with a comment of theirs on it, a token and a
// notification. What is kept is read back unchanged; what is not is gone.

// holdOn writes a hold as it is stored. The ACCOUNT scope is written this way too, so the erasure
// is proven before the scope can be placed through the API.
func holdOn(ctx context.Context, t *testing.T, tenant shared.ID, scope string, scopeID shared.ID) shared.ID {
	t.Helper()
	id := freshID(t)
	var target any
	if !scopeID.IsZero() {
		target = scopeID.String()
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO legal_hold (id, tenant_id, scope_kind, scope_id, reason, placed_by)
		VALUES ($1, $2, $3, $4, 'Pending litigation, ref. 4 O 128/26', $5)`,
		id.String(), tenant.String(), scope, target, freshID(t).String()); err != nil {
		t.Fatalf("placing the hold: %v", err)
	}
	return id
}

func performerFor(t *testing.T) privacyservice.Performer {
	t.Helper()
	return privacyservice.Performer{
		Requests: privacyRepo(), Eraser: eraserFor(t),
		UnitOfWork: postgres.NewUnitOfWork(appPool(context.Background(), t)),
		Clock:      portclock.Fixed(created),
	}
}

func TestAHoldKeepsWhatItReachesAndTheErasureTakesTheRest(t *testing.T) {
	type outcome struct {
		comment, assigned, token bool
		status                   string // the account's status; "" when it is gone
		email                    bool
		pseudonym                bool
		subjectKept              bool // the case still names the account
	}
	for _, c := range []struct {
		name  string
		scope string
		at    func(person) shared.ID
		mode  domain.ErasureMode
		want  outcome
		kept  [2]int // comments, assignments recorded as kept
		acct  bool   // the record says the account was kept
	}{
		{name: "no hold, delete in full", mode: domain.ModeFullDelete,
			want: outcome{status: "", pseudonym: true}},
		{name: "the hub, delete in full", scope: "CONTAINER", at: func(p person) shared.ID { return p.hub },
			mode: domain.ModeFullDelete,
			want: outcome{comment: true, assigned: true, status: "ANONYMIZED", pseudonym: true, subjectKept: true},
			kept: [2]int{1, 1}},
		{name: "the collection, anonymise", scope: "CONTAINER", at: func(p person) shared.ID { return p.collection },
			mode: domain.ModeAnonymize,
			want: outcome{comment: true, assigned: true, status: "ANONYMIZED", pseudonym: true, subjectKept: true},
			kept: [2]int{0, 1}},
		{name: "the entry, delete in full", scope: "ITEM", at: func(p person) shared.ID { return p.item },
			mode: domain.ModeFullDelete,
			want: outcome{comment: true, assigned: true, status: "ANONYMIZED", pseudonym: true, subjectKept: true},
			kept: [2]int{1, 1}},
		{name: "the person, delete in full", scope: "ACCOUNT", at: func(p person) shared.ID { return p.account },
			mode: domain.ModeFullDelete,
			want: outcome{comment: true, assigned: true, status: "RESTRICTED", email: true, subjectKept: true},
			kept: [2]int{1, 1}, acct: true},
		{name: "the workspace, anonymise", scope: "TENANT", at: func(person) shared.ID { return "" },
			mode: domain.ModeAnonymize,
			want: outcome{comment: true, assigned: true, status: "RESTRICTED", email: true, subjectKept: true},
			kept: [2]int{1, 1}, acct: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			subject := seedSubject(ctx, t, "held-"+freshID(t).String()+"@example.test")
			var hold shared.ID
			if c.scope != "" {
				hold = holdOn(ctx, t, subject.tenant, c.scope, c.at(subject))
			}
			request := erasureRequest(ctx, t, subject, c.mode)

			done, err := performerFor(t).Perform(ctx, privacyservice.PerformInput{
				RequestID: request.ID, TenantID: subject.tenant,
			})
			if err != nil {
				t.Fatalf("carrying out the case: %v", err)
			}
			if done.Status != domain.StatusCompleted {
				t.Errorf("the case is %s, want COMPLETED", done.Status)
			}

			got := outcome{
				comment: countIn(ctx, t, `SELECT count(*) FROM comment WHERE id = $1`, subject.comment.String()) == 1,
				assigned: countIn(ctx, t, `SELECT count(*) FROM work_item WHERE id = $1 AND assignee_id = $2`,
					subject.item.String(), subject.account.String()) == 1,
				token: countIn(ctx, t, `SELECT count(*) FROM access_token WHERE id = $1`, subject.token.String()) == 1,
				email: countIn(ctx, t, `SELECT count(*) FROM account WHERE id = $1 AND email IS NOT NULL`,
					subject.account.String()) == 1,
				pseudonym: countIn(ctx, t, `SELECT count(*) FROM audit_pseudonym WHERE actor_id = $1`,
					subject.account.String()) == 1,
				subjectKept: countIn(ctx, t, `SELECT count(*) FROM data_subject_request
					WHERE id = $1 AND subject_account_id = $2`, request.ID.String(), subject.account.String()) == 1,
			}
			_ = adminPool(ctx, t).QueryRow(ctx, `SELECT status::text FROM account WHERE id = $1`,
				subject.account.String()).Scan(&got.status)
			if got != c.want {
				t.Errorf("after the erasure: %+v\nwant %+v", got, c.want)
			}

			// What was kept is on record under the hold, and nothing is when no hold kept anything.
			var comments, assignments int
			var account bool
			rows := countIn(ctx, t, `SELECT count(*) FROM erasure_kept WHERE request_id = $1`, request.ID.String())
			if hold.IsZero() {
				if rows != 0 {
					t.Errorf("%d kept rows for an erasure no hold touched", rows)
				}
				return
			}
			if err := adminPool(ctx, t).QueryRow(ctx, `
				SELECT comments, assignments, account FROM erasure_kept
				WHERE request_id = $1 AND hold_id = $2 AND erased_at IS NULL`,
				request.ID.String(), hold.String()).Scan(&comments, &assignments, &account); err != nil {
				t.Fatalf("reading what the hold kept: %v", err)
			}
			if comments != c.kept[0] || assignments != c.kept[1] || account != c.acct {
				t.Errorf("the record says comments %d, assignments %d, account %v; want %v, account %v",
					comments, assignments, account, c.kept, c.acct)
			}
		})
	}
}
