// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	annaID = shared.MustParseID("0192f000-0000-7000-8000-000000000501")
	benID  = shared.MustParseID("0192f000-0000-7000-8000-000000000502")
)

// kudosStore keeps kudos as the table does: one row per entry, giver and receiver.
type kudosStore struct{ rows []domain.Kudos }

func (s *kudosStore) Give(ctx context.Context, kudos domain.Kudos) (bool, error) {
	if given, _ := s.Given(ctx, kudos.ItemID, kudos.GivenBy, kudos.GivenTo); given {
		return false, nil
	}
	s.rows = append(s.rows, kudos)
	return true, nil
}

func (s *kudosStore) TakeBack(_ context.Context, item, by, to shared.ID) (bool, error) {
	for i, row := range s.rows {
		if row.ItemID == item && row.GivenBy == by && row.GivenTo == to {
			s.rows = append(s.rows[:i], s.rows[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (s *kudosStore) Given(_ context.Context, item, by, to shared.ID) (bool, error) {
	for _, row := range s.rows {
		if row.ItemID == item && row.GivenBy == by && row.GivenTo == to {
			return true, nil
		}
	}
	return false, nil
}

func (s *kudosStore) Received(_ context.Context, item, to shared.ID) ([]domain.Kudos, error) {
	var received []domain.Kudos
	for _, row := range s.rows {
		if row.ItemID == item && row.GivenTo == to {
			received = append(received, row)
		}
	}
	return received, nil
}

// kudosHarness is a task Anna completed, in the completion harness's collection.
type kudosHarness struct {
	registry   *usecase.Registry
	kudos      *kudosStore
	audit      *sink
	authorizer *authorizer
	items      *items
}

func newKudosHarness(t *testing.T) *kudosHarness {
	t.Helper()
	completion := newCompletionHarness(domain.CompletionManual)
	task := completion.items.stored[taskID]
	completion.items.stored[taskID] = task.Completed(annaID, now)
	h := &kudosHarness{kudos: &kudosStore{}, audit: &sink{}, authorizer: &authorizer{}, items: completion.items}
	desk := KudosDesk{
		Kudos: h.kudos, Items: completion.items, Containers: completion.containers, Authorizer: h.authorizer,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), Audit: h.audit,
	}
	registry, err := usecase.NewRegistry(nil,
		ReadKudos{Desk: desk}.Descriptor(), GiveKudos{Desk: desk}.Descriptor(), TakeBackKudos{Desk: desk}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	h.registry = registry
	return h
}

func person(id shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: id, AccountName: "Someone",
		Scopes: []string{"items:read", "items:write"},
	}
}

func (h *kudosHarness) invoke(t *testing.T, name string, who shared.ID) (usecase.Output, error) {
	t.Helper()
	return h.registry.Invoke(t.Context(), name, person(who), usecase.Input{"item_id": taskID.String()})
}

func fromOf(out usecase.Output) []any {
	from, _ := out["from"].([]any)
	return from
}

func TestKudosReachOnlyThePersonWhoCompleted(t *testing.T) {
	h := newKudosHarness(t)

	out, err := h.invoke(t, GiveKudosName, benID)
	if err != nil || out["given"] != true || len(fromOf(out)) != 0 {
		t.Fatalf("Ben gives: %v %v", out, err)
	}
	if len(h.kudos.rows) != 1 || h.kudos.rows[0].GivenTo != annaID || !h.kudos.rows[0].GivenAt.Equal(now) {
		t.Fatalf("stored %+v", h.kudos.rows)
	}

	out, err = h.invoke(t, ReadKudosName, annaID)
	if err != nil || out["given"] != false || len(fromOf(out)) != 1 {
		t.Fatalf("Anna reads: %v %v", out, err)
	}
	if from := fromOf(out)[0].(map[string]any); from["account_id"] != benID.String() {
		t.Errorf("Anna's kudos are from %v", from["account_id"])
	}

	// The workspace's owner reads the entry like anybody else: only their own side.
	out, err = h.invoke(t, ReadKudosName, accountID)
	if err != nil || out["given"] != false || len(fromOf(out)) != 0 {
		t.Fatalf("somebody else reads: %v %v", out, err)
	}
}

func TestGivingAndTakingBackAreAuditedOnlyWhenTheyChangeSomething(t *testing.T) {
	h := newKudosHarness(t)
	for range 2 {
		if _, err := h.invoke(t, GiveKudosName, benID); err != nil {
			t.Fatalf("giving: %v", err)
		}
	}
	for range 2 {
		if out, err := h.invoke(t, TakeBackKudosName, benID); err != nil || out["given"] != false {
			t.Fatalf("taking back: %v %v", out, err)
		}
	}
	if len(h.audit.entries) != 2 {
		t.Fatalf("audited %d acts, want one give and one take-back", len(h.audit.entries))
	}
	for i, action := range []string{string(KudosGivenAction), string(KudosTakenBackAction)} {
		entry := h.audit.entries[i]
		if string(entry.Action) != action || entry.TargetID != taskID || entry.ActorID != benID {
			t.Errorf("entry %d: %+v", i, entry)
		}
		// Whom the kudos went to is not in the trail (UC-WRK-29 check 6).
		if len(entry.Changes) != 0 {
			t.Errorf("entry %d carries changes %v", i, entry.Changes)
		}
	}
}

func TestKudosAreRefusedOnAnOpenEntryAndOnOnesOwnCompletion(t *testing.T) {
	h := newKudosHarness(t)
	if _, err := h.invoke(t, GiveKudosName, annaID); shared.AsError(err).DetailCode != domain.CodeKudosOwnCompletion {
		t.Errorf("Anna's own: %v", err)
	}
	task := h.items.stored[taskID]
	h.items.stored[taskID] = task.Reopened(now)
	if _, err := h.invoke(t, GiveKudosName, benID); shared.AsError(err).DetailCode != domain.CodeKudosNotCompleted {
		t.Errorf("an open entry: %v", err)
	}
	// Taking back on an open entry has nothing to take back, and is no error.
	if out, err := h.invoke(t, TakeBackKudosName, benID); err != nil || out["given"] != false {
		t.Errorf("taking back on an open entry: %v %v", out, err)
	}
	if len(h.kudos.rows) != 0 || len(h.audit.entries) != 0 {
		t.Errorf("a refusal wrote %v and audited %v", h.kudos.rows, h.audit.entries)
	}
}

func TestKudosAskOnlyToReadTheEntry(t *testing.T) {
	h := newKudosHarness(t)
	for name, scope := range map[string]string{ReadKudosName: itemsRead, GiveKudosName: itemsWrite, TakeBackKudosName: itemsWrite} {
		h.authorizer.requests = nil
		if _, err := h.invoke(t, name, benID); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		request := h.authorizer.requests[0]
		if request.Permission != service.PermissionRead || request.TokenScope != scope || request.TargetID != taskID {
			t.Errorf("%s asks %+v", name, request)
		}
	}

	h.authorizer.err = shared.ErrForbidden
	if _, err := h.invoke(t, GiveKudosName, benID); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a refused reader gave kudos: %v", err)
	}
	if len(h.kudos.rows) != 0 {
		t.Errorf("a refusal stored %v", h.kudos.rows)
	}
}
