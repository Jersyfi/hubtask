// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package retention

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	lifecycleDomain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// UC-LIF-06 check 9, end to end: a comment deleted under a hold reads as deleted to everybody and
// keeps its text in the row; the pass keeps it while any hold covering it stands - here two, one
// on the entry and one on the author - and clears it on the first pass after the last is released,
// spending no version. A comment deleted where no hold reaches loses its text at once.
func TestADeletedCommentKeepsItsTextUntilTheLastHoldIsReleased(t *testing.T) {
	s := newSuite(t, 0)
	collectionID := s.collection(t)
	other := s.account(t)
	held := s.liveItem(t, collectionID, s.author)
	// Created by somebody else: an entry the author created is theirs, and the hold on them would
	// keep every comment on it (data-retention.md §4).
	free := s.liveItem(t, collectionID, other)
	kept := s.commentBy(t, held, s.author, "The supplier confirmed the defects, we pay anyway")
	gone := s.commentBy(t, free, other, "Nothing to see")

	onEntry := s.placeHold(t, lifecycleDomain.HoldItem, held)
	onAuthor := s.placeHold(t, lifecycleDomain.HoldAccount, s.author)

	deleted := s.deleteComment(t, held, kept, s.author)
	if deleted.Body != "" || deleted.DeletedAt == nil {
		t.Fatalf("the deletion answered %+v, want the tombstone", deleted)
	}
	if got := s.storedBody(t, kept); got == "" {
		t.Fatal("the text was cleared under a hold")
	}
	if got := s.readBody(t, kept); got != "" {
		t.Errorf("a reader was served %q", got)
	}
	s.deleteComment(t, free, gone, other)
	if got := s.storedBody(t, gone); got != "" {
		t.Errorf("a comment no hold covers kept %q", got)
	}

	version := s.storedVersion(t, kept)
	s.sweep(t)
	if s.storedBody(t, kept) == "" {
		t.Fatal("the pass cleared a text two holds keep")
	}

	s.releaseHold(t, onEntry)
	outcome := s.sweep(t)
	if s.storedBody(t, kept) == "" {
		t.Fatal("the pass cleared a text the hold on its author still keeps")
	}
	if outcome.Blocked[lifecycleDomain.BlockedByLegalHold] == 0 {
		t.Errorf("the pass reported %+v, want the held text counted", outcome.Blocked)
	}

	s.releaseHold(t, onAuthor)
	s.sweep(t)
	if got := s.storedBody(t, kept); got != "" {
		t.Errorf("the text is %q after the last hold was released", got)
	}
	if s.storedVersion(t, kept) != version {
		t.Error("clearing the kept text spent a version")
	}
}

// liveItem writes one entry that is neither trashed nor completed: a discussion can be changed.
func (s *suite) liveItem(t *testing.T, collectionID, creator shared.ID) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key, created_by)
		VALUES ($1, $2, $3, 'TASK', $4, 1, $5, 'a0', $6)`,
		id.String(), s.tenant.String(), collectionID.String(), work.RootPath(id),
		"Entry "+id.String(), creator.String()); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	return id
}

func (s *suite) commentBy(t *testing.T, itemID, author shared.ID, body string) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, $5)`,
		id.String(), s.tenant.String(), itemID.String(), author.String(), body); err != nil {
		t.Fatalf("seeding the comment: %v", err)
	}
	return id
}

func (s *suite) placeHold(t *testing.T, scope lifecycleDomain.HoldScope, id shared.ID) shared.ID {
	t.Helper()
	hold, err := (lifecycle.PlaceLegalHold{Holds: s.holds()}).Execute(s.ctx, s.actorWithAccount(),
		lifecycle.PlaceLegalHoldCommand{Scope: scope, ScopeID: id, Reason: "Dispute with the supplier"})
	if err != nil {
		t.Fatalf("placing the hold: %v", err)
	}
	return hold.ID
}

func (s *suite) releaseHold(t *testing.T, id shared.ID) {
	t.Helper()
	if _, err := (lifecycle.ReleaseLegalHold{Holds: s.holds()}).Execute(
		s.ctx, s.actorWithAccount(), id, "Settled"); err != nil {
		t.Fatalf("releasing the hold: %v", err)
	}
}

// deleteComment runs the use case as the server wires it, with the comment's author as the actor.
func (s *suite) deleteComment(t *testing.T, itemID, commentID, author shared.ID) work.Comment {
	t.Helper()
	hybrid := hybridAt(now)
	writer := workservice.CommentWriter{
		Comments:   postgres.NewCommentRepository(security.NewCursorCodec(installationSecret)),
		Items:      postgres.NewItemRepository(security.NewCursorCodec(installationSecret)),
		Containers: postgres.NewContainerRepository(security.NewCursorCodec(installationSecret)),
		Authorizer: allowAll{},
		Events:     postgres.NewOutbox(noJobs{}),
		Changes:    postgres.NewChangeLog(),
		Audit:      postgres.NewAuditSink(ids),
		UnitOfWork: s.uow,
		Clock:      clock.Fixed(now), IDs: ids, HLC: hybrid,
		Holds: postgres.NewLifecycleRepository(),
	}
	actor := s.actorWithAccount()
	actor.AccountID = author
	deleted, err := workservice.DeleteComment{Writer: writer}.Execute(s.ctx, actor,
		workservice.ChangeCommentCommand{ItemID: itemID, CommentID: commentID})
	if err != nil {
		t.Fatalf("deleting the comment: %v", err)
	}
	return deleted
}

// storedBody is the row as stored, read past the application.
func (s *suite) storedBody(t *testing.T, id shared.ID) string {
	t.Helper()
	var body string
	if err := s.admin.QueryRow(s.ctx, `SELECT body FROM comment WHERE id = $1`, id.String()).Scan(&body); err != nil {
		t.Fatalf("reading the stored body: %v", err)
	}
	return body
}

func (s *suite) storedVersion(t *testing.T, id shared.ID) int {
	t.Helper()
	return s.count(t, `SELECT version FROM comment WHERE id = $1`, id.String())
}

// readBody is what every reader gets: the repository as the application role, under the tenant.
func (s *suite) readBody(t *testing.T, id shared.ID) string {
	t.Helper()
	var comment work.Comment
	err := s.uow.WithinReadOnly(s.ctx, persistence.Scope{TenantID: s.tenant}, func(ctx context.Context) error {
		var err error
		comment, err = postgres.NewCommentRepository(security.NewCursorCodec(installationSecret)).Find(ctx, id)
		return err
	})
	if err != nil {
		t.Fatalf("reading the comment: %v", err)
	}
	return comment.Body
}
