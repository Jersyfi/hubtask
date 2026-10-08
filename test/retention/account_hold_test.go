// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package retention

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	lifecycleDomain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// UC-LIF-06 checks 3, 4 and 5 for a hold on a person (data-protection.md §4.1): what they created,
// commented on or attached a file to is kept by the retention pass, by emptying the trash and by a
// named purge - and what nobody held goes as usual. The hold is placed through the use case.
func TestAnAccountHoldKeepsWhatThePersonContributed(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	collectionID := s.collection(t)
	other := s.account(t)

	created := s.completedItem(t, collectionID, 400)
	trashedByThem := s.trashedItem(t, collectionID, 400)
	commentedOn := s.trashedItemBy(t, collectionID, 400, other)
	s.comment(t, commentedOn, s.author)
	attachedTo := s.trashedItemBy(t, collectionID, 400, other)
	s.attachment(t, attachedTo, s.author)
	nobodys := s.trashedItemBy(t, collectionID, 400, other)
	for range 60 {
		s.completedItem(t, collectionID, 1)
	}

	s.createRule(t, lifecycle.CreateRetentionPolicyCommand{
		Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeTenant},
		DataKind: lifecycleDomain.KindCompletedItem, RetainDays: 365,
		Action: lifecycleDomain.ActionHardDelete, GraceDays: graceOf(0),
	})
	s.holdAccount(t, s.author)

	// The retention pass and the trash's own period.
	outcome := s.sweep(t)
	for name, id := range map[string]shared.ID{
		"the entry they created": created, "the trashed entry they created": trashedByThem,
		"the entry they commented on": commentedOn, "the entry they attached a file to": attachedTo,
	} {
		if !s.exists(t, id) {
			t.Errorf("%s was deleted under a hold on them", name)
		}
	}
	if s.exists(t, nobodys) {
		t.Error("an entry nobody held was kept")
	}
	if outcome.Blocked[lifecycleDomain.BlockedByLegalHold] == 0 {
		t.Errorf("the run reported %+v", outcome.Blocked)
	}
	if _, reason := s.itemRetention(t, created); reason != lifecycleDomain.BlockedByLegalHold {
		t.Errorf("the entry says %q is stopping it", reason)
	}

	// Emptying the trash, as a person does.
	err := s.uow.Within(s.ctx, persistence.Scope{TenantID: s.tenant}, func(ctx context.Context) error {
		_, err := s.engineAt(now).Purger.Sweep(ctx, s.actor(), lifecycle.Selection{
			Cutoff: now, Reason: lifecycleDomain.DeletedByUser,
		}, now)
		return err
	})
	if err != nil {
		t.Fatalf("emptying the trash: %v", err)
	}
	if !s.exists(t, commentedOn) || !s.exists(t, trashedByThem) {
		t.Error("emptying the trash removed an entry under a hold on its contributor")
	}

	// Deleting one for good by name is refused, naming the scope.
	err = s.uow.Within(s.ctx, persistence.Scope{TenantID: s.tenant}, func(ctx context.Context) error {
		deletedAt := now.AddDate(0, 0, -400)
		_, err := s.engineAt(now).Purger.Subtree(ctx, s.actor(), work.WorkItem{
			ID: attachedTo, TenantID: s.tenant, CollectionID: collectionID, Type: work.ItemTask,
			Path: work.RootPath(attachedTo), DeletedAt: &deletedAt,
		}, "", lifecycleDomain.DeletedByUser, now)
		return err
	})
	if shared.AsError(err).DetailCode != "lifecycle.legal_hold" {
		t.Fatalf("a named purge under a hold on the uploader reported %v", err)
	}
}

// account seeds a second person in the suite's workspace.
func (s *suite) account(t *testing.T) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := s.admin.Exec(s.ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Bert')`,
		id.String(), s.tenant.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	return id
}

// trashedItemBy is trashedItem, created by somebody else.
func (s *suite) trashedItemBy(t *testing.T, collectionID shared.ID, daysAgo int, author shared.ID) shared.ID {
	t.Helper()
	id := s.trashedItem(t, collectionID, daysAgo)
	if _, err := s.admin.Exec(s.ctx, `UPDATE work_item SET created_by = $2 WHERE id = $1`,
		id.String(), author.String()); err != nil {
		t.Fatalf("handing the entry to its author: %v", err)
	}
	return id
}

func (s *suite) comment(t *testing.T, itemID, author shared.ID) {
	t.Helper()
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, 'noted')`,
		freshID(t).String(), s.tenant.String(), itemID.String(), author.String()); err != nil {
		t.Fatalf("seeding the comment: %v", err)
	}
}

func (s *suite) attachment(t *testing.T, itemID, uploader shared.ID) {
	t.Helper()
	media := freshID(t)
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, usage, status,
		  ref_count, created_by)
		VALUES ($1, $2, $3, 'text/plain', 12, 'ATTACHMENT', 'READY', 1, $4)`,
		media.String(), s.tenant.String(), "media/"+media.String(), uploader.String()); err != nil {
		t.Fatalf("seeding the file: %v", err)
	}
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO item_attachment (tenant_id, item_id, media_id) VALUES ($1, $2, $3)`,
		s.tenant.String(), itemID.String(), media.String()); err != nil {
		t.Fatalf("attaching the file: %v", err)
	}
}

// holdAccount places a hold on one person, the way a person places one.
func (s *suite) holdAccount(t *testing.T, account shared.ID) {
	t.Helper()
	if _, err := (lifecycle.PlaceLegalHold{Holds: s.holds()}).Execute(s.ctx, s.actorWithAccount(),
		lifecycle.PlaceLegalHoldCommand{
			Scope: lifecycleDomain.HoldAccount, ScopeID: account,
			Reason: "Pending litigation, ref. 4 O 128/26",
		}); err != nil {
		t.Fatalf("placing the hold: %v", err)
	}
}
