// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// PH-09's acceptance for AI (UC-PRV-03 check 10, UC-PRV-04 check 5): a restricted person's comment
// never reaches a prompt the adapter receives - nor what they created, what is assigned to them,
// or their name - and unlike an objection the restriction holds where AI is part of everybody's
// work and for a person alone. The world and the recording provider are PH-03's.

func (w keptWorld) restrict(ctx context.Context, t *testing.T, restricted bool) {
	t.Helper()
	status := "ACTIVE"
	if restricted {
		status = "RESTRICTED"
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE account SET status = $2 WHERE id = $1`,
		w.kim.String(), status); err != nil {
		t.Fatalf("setting Kim's status: %v", err)
	}
}

// assignedToKim writes an entry Ada created and put on Kim.
func (w keptWorld) assignedToKim(ctx context.Context, t *testing.T, title string) shared.ID {
	t.Helper()
	id := freshID(t)
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(w.tenant, w.ada, w.collection, id, title, "a3"))
	}); err != nil {
		t.Fatalf("seeding Kim's assignment: %v", err)
	}
	// The insert writes no assignee; an assignment is its own write.
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE work_item SET assignee_id = $2 WHERE id = $1`,
		id.String(), w.kim.String()); err != nil {
		t.Fatalf("assigning Kim: %v", err)
	}
	return id
}

func TestARestrictedPersonsCommentNeverReachesAPrompt(t *testing.T) {
	ctx := context.Background()
	w := seedKeptWorld(ctx, t)
	h := newKeptHarness(ctx, t)
	assigned := w.assignedToKim(ctx, t, "ADAFORKIM plan the drinks")
	askAboutAssigned := func() string {
		if _, err := h.translate.Execute(ctx, w.as(w.ada), work.TranslateCommand{ItemID: assigned, TargetLocale: "de"}); err != nil {
			t.Logf("translating the assigned entry: %v", err)
		}
		return strings.Join(h.recorder.take(), "\n")
	}

	sent := h.askEverything(ctx, t, w) + askAboutAssigned()
	if !strings.Contains(sent, "KIMSAYS") || !strings.Contains(sent, "ADAFORKIM") {
		t.Fatalf("before the restriction Kim's content did not reach the adapter, so the test proves nothing:\n%s", sent)
	}

	// Kim's processing is restricted - in both of the workspace's positions.
	w.restrict(ctx, t, true)
	for _, notOffered := range []bool{false, true} {
		w.position(ctx, t, notOffered)
		sent = h.askEverything(ctx, t, w) + askAboutAssigned()
		for _, trace := range append(w.kimTraces(), "ADAFORKIM") {
			if strings.Contains(strings.ToLower(sent), strings.ToLower(trace)) {
				t.Errorf("NOT_OFFERED=%v: %q reached the adapter while Kim is restricted:\n%s", notOffered, trace, sent)
			}
		}
		for _, own := range []string{"ADASAYS", "ADANOTES", "[person]"} {
			if !strings.Contains(sent, own) {
				t.Errorf("NOT_OFFERED=%v: %q is missing: Ada's own content must still be sent:\n%s", notOffered, own, sent)
			}
		}
	}
	w.position(ctx, t, false)

	// Lifted: Kim's comment is sent again.
	w.restrict(ctx, t, false)
	if sent = h.askEverything(ctx, t, w); !strings.Contains(sent, "KIMSAYS") {
		t.Errorf("after the restriction was lifted Kim's comment did not reach the adapter:\n%s", sent)
	}
}

// Unlike an objection, a restriction holds for a person alone in a workspace.
func TestARestrictionHoldsForAPersonAlone(t *testing.T) {
	ctx := context.Background()
	w := seedKeptWorld(ctx, t)
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE account SET deleted_at = now() WHERE id = $1`, w.ada.String()); err != nil {
		t.Fatalf("leaving Kim alone: %v", err)
	}
	w.restrict(ctx, t, true)
	if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
		kept, err := keptOutExclusion().Read(ctx, w.tenant)
		if err == nil && !kept.Has(w.kim) {
			t.Error("alone, Kim's restriction kept nothing out")
		}
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
}

// The restricted person asking is told their own reason; a colleague asking about an entry that is
// assigned to them is answered what an empty entry is answered.
func TestARestrictedPersonIsToldTheirOwnReason(t *testing.T) {
	ctx := context.Background()
	w := seedKeptWorld(ctx, t)
	assigned := w.assignedToKim(ctx, t, "ADAFORKIM plan the drinks")
	w.restrict(ctx, t, true)
	readers := readerRegistry(ctx, t)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	queued := &keptQueue{}
	cases := suggestion.Cases{
		Suggestions: postgres.NewSuggestionRepository(pageCursors()),
		Targets:     suggestion.EntryTargets{Catalogue: readers},
		Authorizer:  realAuthoriser(ctx, t), Catalogue: readers,
		Audit: postgres.NewAuditSink(ids), UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock:    clockadapter.System{},
		Material: suggestion.CatalogueSources{Catalogue: readers, Kept: keptOutExclusion()},
	}
	registry, err := usecase.NewRegistry(nil,
		suggestion.AiSummarize{Cases: cases, AI: aiAvailable{}, Queue: queued}.Descriptor())
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	ask := func(account, item shared.ID) *shared.Error {
		_, err := registry.Invoke(ctx, suggestion.AiSummarizeName, w.as(account), usecase.Input{"item_id": item.String()})
		return shared.AsError(err)
	}
	if got := ask(w.kim, w.adaEntry); got == nil || got.DetailCode != "ai.processing_restricted" {
		t.Errorf("Kim asking answered %v, want ai.processing_restricted", got)
	}
	if got := ask(w.ada, assigned); got == nil || got.DetailCode != "ai.nothing_to_work_with" {
		t.Errorf("asking about the entry Kim is on answered %v, want ai.nothing_to_work_with", got)
	}
	if len(queued.requests) != 0 {
		t.Errorf("%d questions were queued", len(queued.requests))
	}
}
