// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package retention

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	lifecycleDomain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// A deleted template's end, against a real database (data-retention.md §4, decided in #1248):
// deleted where no hold reaches, it is gone at once with a journal entry and a tombstone; deleted
// under a hold on its hub, it stays deleted through every pass while the hold stands and goes on the
// first pass after the release. A template soft-deleted before deletions removed anything goes on
// the first pass. Every removal is journalled, so a restore does not bring it back (BK-6).
func TestADeletedTemplateIsKeptOnlyWhileAHoldCoversIt(t *testing.T) {
	s := newSuite(t, 0)
	heldCollection := s.collection(t)
	freeCollection := s.collection(t)

	held := s.seedTemplate(t, heldCollection)
	free := s.seedTemplate(t, freeCollection)
	legacy := s.seedTemplate(t, freeCollection)
	if _, err := s.admin.Exec(s.ctx, `UPDATE template SET deleted_at = $2 WHERE id = $1`,
		legacy.ID.String(), now.AddDate(-1, 0, 0)); err != nil {
		t.Fatalf("soft-deleting the earlier template: %v", err)
	}

	hold := s.holdContainer(t, s.hubOf(t, heldCollection))
	s.deleteTemplate(t, held.ID)
	s.deleteTemplate(t, free.ID)

	if s.templateRows(t, free.ID) != 0 {
		t.Error("a template no hold covers was kept")
	}
	if got := s.journalReason(t, free.ID); got != string(lifecycleDomain.DeletedByUser) {
		t.Errorf("the removal is journalled as %q, want USER", got)
	}
	if s.count(t, `SELECT count(*) FROM tombstone WHERE entity = 'template' AND entity_id = $1`,
		free.ID.String()) != 1 {
		t.Error("the removal left no tombstone")
	}
	if s.templateRows(t, held.ID) != 1 {
		t.Fatal("a template under a hold was removed")
	}

	outcome := s.sweep(t)
	if s.templateRows(t, held.ID) != 1 {
		t.Fatal("the pass removed a template the hold still covers")
	}
	if outcome.Blocked[lifecycleDomain.BlockedByLegalHold] == 0 {
		t.Errorf("the pass reported %+v, want the held template counted", outcome.Blocked)
	}
	if s.templateRows(t, legacy.ID) != 0 {
		t.Error("a template deleted before this release is still stored")
	}
	if got := s.journalReason(t, legacy.ID); got != string(lifecycleDomain.DeletedByRetention) {
		t.Errorf("the earlier template is journalled as %q, want RETENTION", got)
	}

	if _, err := (lifecycle.ReleaseLegalHold{Holds: s.holds()}).Execute(
		s.ctx, s.actorWithAccount(), hold, "Settled"); err != nil {
		t.Fatalf("releasing the hold: %v", err)
	}
	s.sweep(t)
	if s.templateRows(t, held.ID) != 0 {
		t.Error("the template is still stored after its hold was released")
	}
	if got := s.journalReason(t, held.ID); got != string(lifecycleDomain.DeletedByRetention) {
		t.Errorf("the held template is journalled as %q, want RETENTION", got)
	}
}

func (s *suite) hubOf(t *testing.T, collectionID shared.ID) shared.ID {
	t.Helper()
	var hub string
	if err := s.admin.QueryRow(s.ctx, `SELECT parent_id FROM container WHERE id = $1`,
		collectionID.String()).Scan(&hub); err != nil {
		t.Fatalf("reading the hub: %v", err)
	}
	return shared.MustParseID(hub)
}

func (s *suite) templateWriter() workservice.TemplateWriter {
	return workservice.TemplateWriter{
		Templates:  postgres.NewTemplateRepository(security.NewCursorCodec(installationSecret)),
		Containers: postgres.NewContainerRepository(security.NewCursorCodec(installationSecret)),
		Authorizer: allowAll{},
		Changes:    postgres.NewChangeLog(),
		Audit:      postgres.NewAuditSink(ids),
		UnitOfWork: s.uow,
		Clock:      clock.Fixed(now), IDs: ids, HLC: hybridAt(now), Text: text.Composing{},
		Holds:           postgres.NewLifecycleRepository(),
		Removals:        postgres.NewLifecycleRepository(),
		TombstoneWindow: 90 * 24 * time.Hour,
	}
}

func (s *suite) seedTemplate(t *testing.T, collectionID shared.ID) work.Template {
	t.Helper()
	template, err := work.NewTemplate(work.NewTemplateInput{
		ID: freshID(t), TenantID: s.tenant,
		Spec: work.TemplateSpec{
			Scope: string(work.TemplateScopeCollection), ScopeID: collectionID,
			Name: "Onboarding " + freshID(t).String(), RootType: string(work.ItemTask),
			Root: work.TemplateNode{Type: work.ItemTask, Title: "Welcome Jane"},
		},
		Now: now,
	})
	if err != nil {
		t.Fatalf("the template was refused: %v", err)
	}
	err = s.uow.Within(s.ctx, persistence.Scope{TenantID: s.tenant}, func(ctx context.Context) error {
		return postgres.NewTemplateRepository(security.NewCursorCodec(installationSecret)).Insert(ctx, template)
	})
	if err != nil {
		t.Fatalf("seeding the template: %v", err)
	}
	return template
}

func (s *suite) holdContainer(t *testing.T, containerID shared.ID) shared.ID {
	t.Helper()
	hold, err := (lifecycle.PlaceLegalHold{Holds: s.holds()}).Execute(s.ctx, s.actorWithAccount(),
		lifecycle.PlaceLegalHoldCommand{
			Scope: lifecycleDomain.HoldContainer, ScopeID: containerID, Reason: "Dispute with the supplier",
		})
	if err != nil {
		t.Fatalf("placing the hold: %v", err)
	}
	return hold.ID
}

func (s *suite) deleteTemplate(t *testing.T, id shared.ID) {
	t.Helper()
	if err := (workservice.DeleteTemplate{Writer: s.templateWriter()}).Execute(
		s.ctx, s.actorWithAccount(), workservice.ChangeTemplateCommand{TemplateID: id}); err != nil {
		t.Fatalf("deleting the template: %v", err)
	}
}

func (s *suite) templateRows(t *testing.T, id shared.ID) int {
	t.Helper()
	return s.count(t, `SELECT count(*) FROM template WHERE id = $1`, id.String())
}

func (s *suite) journalReason(t *testing.T, id shared.ID) string {
	t.Helper()
	var reason string
	if err := s.admin.QueryRow(s.ctx,
		`SELECT reason FROM deletion_journal WHERE entity = 'template' AND entity_id = $1`,
		id.String()).Scan(&reason); err != nil {
		return ""
	}
	return reason
}
