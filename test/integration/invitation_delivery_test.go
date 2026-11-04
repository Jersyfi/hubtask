// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The people list's delivery state reads the newest invitation job per account, of the reading
// workspace only: the queue has no row level security, so the statement's own tenant condition is
// what keeps another workspace's invitations out (gate SG-3).
func TestTheLastInvitationMailPerAccountStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	invitedA := seedAccount(ctx, t, tenantA)
	invitedB := seedAccount(ctx, t, tenantB)

	seed := func(tenant shared.ID, account shared.ID, kind, state, lastError, age string) {
		t.Helper()
		if _, err := admin.Exec(ctx, `
			INSERT INTO job (id, tenant_id, kind, state, dedupe_key, last_error, created_at, finished_at, payload)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), now() - $7::interval,
			        CASE WHEN $4 IN ('PENDING', 'RUNNING') THEN NULL ELSE now() - $7::interval END, '{}')`,
			freshID(t).String(), tenant.String(), kind, state, account.String(), lastError, age); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	// An older mail that went out, then a renewal that failed: the newer one is the row's.
	seed(tenantA, invitedA, "notification.invitation", "SUCCEEDED", "", "2 days")
	seed(tenantA, invitedA, "notification.invitation", "DEAD_LETTER", "mail.recipient_rejected", "1 hour")
	// Another kind under the same key is not an invitation.
	seed(tenantA, invitedA, "notification.digest", "PENDING", "", "1 minute")
	// Tenant B's invitation, keyed by its own account.
	seed(tenantB, invitedB, "notification.invitation", "PENDING", "", "1 minute")

	jobs := postgres.NewInvitationJobRepository()
	latest := func(tenant shared.ID, ids ...shared.ID) []repository.InvitationJob {
		t.Helper()
		var got []repository.InvitationJob
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			got, err = jobs.Latest(ctx, ids)
			return err
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		return got
	}

	got := latest(tenantA, invitedA, invitedB)
	if len(got) != 1 || got[0].AccountID != invitedA || got[0].State != "DEAD_LETTER" ||
		got[0].Code != "mail.recipient_rejected" || got[0].FinishedAt.IsZero() {
		t.Fatalf("tenant A read %+v, want only its own newest invitation", got)
	}
	// Tenant B asking about tenant A's account sees nothing of it.
	if got := latest(tenantB, invitedA); len(got) != 0 {
		t.Errorf("tenant B read tenant A's invitation: %+v", got)
	}
	if got := latest(tenantB, invitedB); len(got) != 1 || got[0].State != "PENDING" {
		t.Errorf("tenant B read %+v of its own", got)
	}
}

// A link handed over instead of the mail (*Copy invitation link*) cancels the mail still waiting -
// the delivery of a recorded invitation included - and sets the unsent record aside, so the people
// list says it was withdrawn; a delivery being sent is a conflict; and no workspace cancels
// another's (gate SG-3).
func TestHandingOverALinkCancelsTheWaitingMailOfItsOwnWorkspaceOnly(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	invited := seedAccount(ctx, t, tenantA)

	notificationFor := func(age string) shared.ID {
		t.Helper()
		id := freshID(t)
		if _, err := admin.Exec(ctx, `
			INSERT INTO notification (id, tenant_id, recipient_id, category, channel, state, created_at)
			VALUES ($1, $2, $3, 'INVITATION', 'EMAIL', 'PENDING', now() - $4::interval)`,
			id.String(), tenantA.String(), invited.String(), age); err != nil {
			t.Fatalf("seeding the notification: %v", err)
		}
		return id
	}
	job := func(kind, state, key, age string) shared.ID {
		t.Helper()
		id := freshID(t)
		if _, err := admin.Exec(ctx, `
			INSERT INTO job (id, tenant_id, kind, state, dedupe_key, created_at, finished_at, payload)
			VALUES ($1, $2, $3, $4, $5, now() - $6::interval,
			        CASE WHEN $4 IN ('PENDING', 'RUNNING') THEN NULL ELSE now() - $6::interval END, '{}')`,
			id.String(), tenantA.String(), kind, state, key, age); err != nil {
			t.Fatalf("seeding a job: %v", err)
		}
		return id
	}
	stateOf := func(table string, id shared.ID) string {
		t.Helper()
		var state string
		if err := admin.QueryRow(ctx, `SELECT state FROM `+table+` WHERE id = $1`, id.String()).Scan(&state); err != nil {
			t.Fatalf("reading %s: %v", table, err)
		}
		return state
	}

	job("notification.invitation", "SUCCEEDED", invited.String(), "2 hours")
	recorded := notificationFor("1 hour")
	delivery := job("notification.deliver", "PENDING", recorded.String(), "1 hour")

	jobs := postgres.NewInvitationJobRepository()
	cancel := func(tenant shared.ID) (int, error) {
		t.Helper()
		var cancelled int
		err := write(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			cancelled, err = jobs.CancelOpen(ctx, invited, time.Now())
			return err
		})
		return cancelled, err
	}
	latest := func() repository.InvitationJob {
		t.Helper()
		var got []repository.InvitationJob
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			got, err = jobs.Latest(ctx, []shared.ID{invited})
			return err
		}); err != nil || len(got) != 1 {
			t.Fatalf("reading the delivery: %v, %+v", err, got)
		}
		return got[0]
	}

	if got := latest(); got.State != "SUCCEEDED" || got.MailState != "PENDING" {
		t.Fatalf("before: %+v, want a recorded mail waiting to be sent", got)
	}

	t.Run("another workspace", func(t *testing.T) {
		if cancelled, err := cancel(tenantB); err != nil || cancelled != 0 {
			t.Fatalf("tenant B cancelled %d (%v) of tenant A's invitation mail", cancelled, err)
		}
		if stateOf("job", delivery) != "PENDING" || stateOf("notification", recorded) != "PENDING" {
			t.Error("tenant B reached tenant A's invitation mail")
		}
	})

	cancelled, err := cancel(tenantA)
	if err != nil || cancelled != 1 {
		t.Fatalf("cancelled %d (%v), want the one waiting delivery", cancelled, err)
	}
	if stateOf("job", delivery) != "CANCELLED" || stateOf("notification", recorded) != "SUPPRESSED" {
		t.Errorf("after: the delivery is %s and the record %s", stateOf("job", delivery), stateOf("notification", recorded))
	}
	if got := latest(); got.MailState != "SUPPRESSED" || got.MailReason != "notifications.replaced_by_link" {
		t.Errorf("the people list reads %+v, want the mail set aside for the link", got)
	}

	sending := notificationFor("1 minute")
	job("notification.deliver", "RUNNING", sending.String(), "1 minute")
	if _, err := cancel(tenantA); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a mail being sent answered %v, want a conflict", err)
	}
}
