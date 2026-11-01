// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	filerID   = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	lineItem  = shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	lineNote  = shared.MustParseID("0192f000-0000-7000-8000-0000000000a3")
	lineAlone = shared.MustParseID("0192f000-0000-7000-8000-0000000000a4")
)

type lineStore struct {
	lines   []repository.IntakeLine
	asked   []string
	expired []string
}

func (s *lineStore) IntakeLinesFrom(_ context.Context, addresses []string) ([]repository.IntakeLine, error) {
	s.asked = append(s.asked, addresses...)
	return s.lines, nil
}

func (s *lineStore) ExpiredAddresses(context.Context, time.Time, int) ([]string, error) {
	return s.expired, nil
}

type invoked struct {
	name  string
	actor appshared.ActorContext
	in    usecase.Input
}

// entryCatalogue records the edits; refuseFiler answers the filer's edits as forbidden.
type entryCatalogue struct {
	calls       []invoked
	refuseFiler bool
	// archived answers the edit of this comment as its collection's: archived, so read-only.
	archived string
}

func (c *entryCatalogue) Invoke(_ context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c.calls = append(c.calls, invoked{name, actor, in})
	if c.archived != "" && in.String("comment_id") == c.archived {
		return nil, shared.ErrConflict.WithDetail("items.collection_archived")
	}
	if c.refuseFiler && actor.Kind == appshared.ActorAutomation {
		return nil, shared.ErrForbidden.WithDetail("access.not_permitted")
	}
	return usecase.Output{}, nil
}

func intakeLines() *lineStore {
	return &lineStore{lines: []repository.IntakeLine{
		{ID: lineItem, ItemID: lineItem, WrittenBy: filerID, Text: "From: Erika Muster <kunde@shop.test>\n\nThe printer"},
		{Comment: true, ID: lineNote, ItemID: lineItem, WrittenBy: filerID, Text: "From: kunde@shop.test\n\nStill broken"},
		{Comment: true, ID: lineAlone, ItemID: lineItem, WrittenBy: filerID, Text: "From: kunde@shop.test"},
		{Comment: true, ID: lineAlone, ItemID: lineItem, WrittenBy: filerID, Text: "From: someone@else.test\n\nHi"},
	}}
}

// Third answers 2 (UC-PRV-01): a case by address removes the intake's "From:" line from the
// entry's notes and its comments through the ordinary edits, as the account that filed them; a
// comment that held nothing else is deleted; somebody else's line stays.
func TestAnErasureByAddressRemovesTheIntakesFromLine(t *testing.T) {
	h := newErasureHarness()
	lines, entries := intakeLines(), &entryCatalogue{}
	eraser := h.eraser()
	eraser.Addresses = &addressStore{held: 1}
	eraser.IntakeLines = StripIntakeLines{Lines: lines, Entries: entries}
	erased, err := eraser.Erase(context.Background(), actor(), addressCase(domain.ModeAnonymize))
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if len(entries.calls) != 3 {
		t.Fatalf("%d edits, want 3: %+v", len(entries.calls), entries.calls)
	}
	want := []struct{ name, field, text string }{
		{updateWorkItemName, "notes", "The printer"},
		{editCommentName, "body", "Still broken"},
		{deleteCommentName, "body", ""},
	}
	for i, w := range want {
		call := entries.calls[i]
		if call.name != w.name || call.in.String(w.field) != w.text {
			t.Errorf("edit %d was %s %q, want %s %q", i, call.name, call.in.String(w.field), w.name, w.text)
		}
		if call.actor.Kind != appshared.ActorAutomation || call.actor.AccountID != filerID {
			t.Errorf("edit %d ran as %+v, want the filer", i, call.actor)
		}
	}
	if erased.Intake != 4 {
		t.Errorf("the erasure counted %d, want the record and three lines", erased.Intake)
	}
}

// Where the filer may no longer edit, the owner running the erasure does; a workspace hold keeps
// the lines.
func TestAnIntakeLineFallsBackToTheEraserAndAHoldKeepsIt(t *testing.T) {
	h := newErasureHarness()
	entries := &entryCatalogue{refuseFiler: true}
	eraser := h.eraser()
	eraser.Addresses = &addressStore{held: 1}
	eraser.IntakeLines = StripIntakeLines{Lines: intakeLines(), Entries: entries}
	if _, err := eraser.Erase(context.Background(), actor(), addressCase(domain.ModeAnonymize)); err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if len(entries.calls) != 6 || entries.calls[1].actor.Kind != appshared.ActorUser {
		t.Errorf("the refused edits were not made by the eraser: %+v", entries.calls)
	}

	h = newErasureHarness()
	h.holds.holds = lifecycle.Holds{{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000f1"), Scope: lifecycle.HoldTenant, Reason: "Litigation"}}
	entries = &entryCatalogue{}
	eraser = h.eraser()
	eraser.Addresses = &addressStore{held: 1}
	eraser.IntakeLines = StripIntakeLines{Lines: intakeLines(), Entries: entries}
	if _, err := eraser.Erase(context.Background(), actor(), addressCase(domain.ModeAnonymize)); err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if len(entries.calls) != 0 {
		t.Errorf("a workspace hold let %d edits through", len(entries.calls))
	}
}

// REQUESTER expiry: the lines of the batch's addresses go first; one nobody may edit any more is
// left without stopping the pass.
func TestExpiryRemovesTheIntakeLinesAndARefusalDoesNotStopIt(t *testing.T) {
	lines := intakeLines()
	lines.expired = []string{"kunde@shop.test"}
	entries := &entryCatalogue{refuseFiler: true}
	strip := StripIntakeLines{Lines: lines, Entries: entries}
	cleared, err := strip.BeforeExpiry(context.Background(), filerID, time.Now(), 10)
	if err != nil {
		t.Fatalf("a refused edit stopped the expiry: %v", err)
	}
	if len(cleared) != 0 {
		t.Errorf("a requester whose lines stay was cleared to go: %v", cleared)
	}
	if len(entries.calls) != 3 || len(lines.asked) != 1 || lines.asked[0] != "kunde@shop.test" {
		t.Errorf("asked %v and edited %+v", lines.asked, entries.calls)
	}
}

// A line in an archived (or trashed) collection cannot be changed now: the expiry neither fails
// nor lets that requester go, while the requester whose lines all went may; the other lines of the
// kept address still go.
func TestExpiryKeepsARequesterNamedInAnArchivedCollection(t *testing.T) {
	lines := intakeLines()
	lines.expired = []string{"kunde@shop.test", "someone@else.test"}
	entries := &entryCatalogue{archived: lineNote.String()}
	strip := StripIntakeLines{Lines: lines, Entries: entries}
	cleared, err := strip.BeforeExpiry(context.Background(), filerID, time.Now(), 10)
	if err != nil {
		t.Fatalf("an archived collection failed the expiry: %v", err)
	}
	if len(cleared) != 1 || cleared[0] != "someone@else.test" {
		t.Errorf("cleared %v, want only the address whose lines all went", cleared)
	}
	edited := map[string]bool{}
	for _, call := range entries.calls {
		edited[call.name+" "+call.in.String("comment_id")] = true
	}
	if !edited[updateWorkItemName+" "] || !edited[deleteCommentName+" "+lineAlone.String()] {
		t.Errorf("the kept address's other lines were not removed: %+v", entries.calls)
	}
}

// An erasure by address whose line sits in an archived collection stays open, and says why; the
// lines that could go went.
func TestAnErasureWithALineInAnArchivedCollectionStaysOpen(t *testing.T) {
	h := newErasureHarness()
	entries := &entryCatalogue{archived: lineNote.String()}
	eraser := h.eraser()
	eraser.Addresses = &addressStore{held: 1}
	eraser.IntakeLines = StripIntakeLines{Lines: intakeLines(), Entries: entries}
	_, err := eraser.Erase(context.Background(), actor(), addressCase(domain.ModeAnonymize))
	if shared.AsError(err).DetailCode != "items.collection_archived" {
		t.Fatalf("erasing answered %v, want the archived collection's refusal", err)
	}
	if len(entries.calls) < 3 {
		t.Errorf("the other lines were not removed: %+v", entries.calls)
	}
}
