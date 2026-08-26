// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// Placing and lifting a hold (E-08). What is under test is who may, what is recorded, and the two
// refusals that matter: a hold nothing would honour, and a lifting that happens twice.

// holdWriter is the holds, in memory, with the "lift once" guard the statement has.
type holdWriter struct {
	stored []domain.LegalHold
	lifted int
	refuse bool
}

func (w *holdWriter) Place(_ context.Context, hold domain.LegalHold) error {
	w.stored = append(w.stored, hold)
	return nil
}

func (w *holdWriter) Find(_ context.Context, id shared.ID) (domain.LegalHold, error) {
	for _, hold := range w.stored {
		if hold.ID == id {
			return hold, nil
		}
	}
	return domain.LegalHold{}, shared.ErrNotFound.WithDetail(domain.CodeHoldNotFound)
}

func (w *holdWriter) List(_ context.Context, includeReleased bool) ([]domain.LegalHold, error) {
	var out []domain.LegalHold
	for _, hold := range w.stored {
		if hold.Released() && !includeReleased {
			continue
		}
		out = append(out, hold)
	}
	return out, nil
}

func (w *holdWriter) Release(_ context.Context, hold domain.LegalHold) (bool, error) {
	if w.refuse {
		// Somebody else lifted it between the read and the write, which is what the statement's
		// own predicate answers.
		return false, nil
	}
	for i, stored := range w.stored {
		if stored.ID == hold.ID {
			w.stored[i] = hold
			w.lifted++
			return true, nil
		}
	}
	return false, nil
}

type holdsHarness struct {
	holds      *holdWriter
	authorizer *authorizerDouble
	audit      *auditSink
	uow        *unitOfWork
}

func newHoldsHarness() *holdsHarness {
	return &holdsHarness{
		holds: &holdWriter{}, authorizer: &authorizerDouble{},
		audit: &auditSink{}, uow: &unitOfWork{},
	}
}

func (h *holdsHarness) service() Holds {
	return Holds{
		Holds: h.holds, Authorizer: h.authorizer, Audit: h.audit,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &idSource{},
	}
}

func placeCommand(change func(*PlaceLegalHoldCommand)) PlaceLegalHoldCommand {
	cmd := PlaceLegalHoldCommand{
		Scope: domain.HoldContainer, ScopeID: hubID,
		Reason: "Pending litigation, ref. 4 O 128/26",
	}
	change(&cmd)
	return cmd
}

func TestPlacingAHoldRecordsWhoAndWhyAndAsksForTheOwnersRight(t *testing.T) {
	h := newHoldsHarness()

	hold, err := (PlaceLegalHold{Holds: h.service()}).
		Execute(context.Background(), actor(), placeCommand(func(*PlaceLegalHoldCommand) {}))
	if err != nil {
		t.Fatalf("placing: %v", err)
	}

	if len(h.holds.stored) != 1 {
		t.Fatalf("%d holds were written", len(h.holds.stored))
	}
	if hold.PlacedBy != accountID || !hold.PlacedAt.Equal(now) {
		t.Errorf("the hold was placed by %s at %s", hold.PlacedBy, hold.PlacedAt)
	}

	// A hold overrides the workspace's own configured periods and a person emptying their own
	// trash, which is the owner's line rather than the administrator's.
	request := h.authorizer.requests[0]
	if request.Permission != domainservice.PermissionDeleteContainer {
		t.Errorf("placing a hold asked for %q", request.Permission)
	}

	// The reason is in the trail: an auditor with a date and no case has nothing.
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != HoldPlacedAction {
		t.Fatalf("the trail holds %+v", h.audit.entries)
	}
	if !carries(h.audit.entries[0], "reason", "Pending litigation, ref. 4 O 128/26") {
		t.Error("the audit entry does not carry the reason")
	}
}

// carries reads the masked shape `audit.Changes` produces: one entry per field, with `to` and
// optionally `from`.
func carries(entry audit.Entry, field, value string) bool {
	change, present := entry.Changes[field].(map[string]any)
	if !present {
		return false
	}
	return change["to"] == value || change["from"] == value
}

// The scope the schema accepts and the engine ignores. Refused, so that nobody believes a hold is
// in force that nothing honours.
func TestAnAccountHoldIsRefusedAndWritesNothing(t *testing.T) {
	h := newHoldsHarness()

	_, err := (PlaceLegalHold{Holds: h.service()}).Execute(context.Background(), actor(),
		placeCommand(func(cmd *PlaceLegalHoldCommand) {
			cmd.Scope, cmd.ScopeID = domain.HoldAccount, accountID
		}))

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) ||
		domainErr.DetailCode != domain.CodeHoldAccountScopeUnavailable {
		t.Fatalf("refused with %v", err)
	}
	if len(h.holds.stored) != 0 || len(h.audit.entries) != 0 {
		t.Error("a refused hold left something behind")
	}
}

func TestLiftingRecordsBothReasonsAndHappensOnce(t *testing.T) {
	h := newHoldsHarness()
	service := h.service()

	hold, err := (PlaceLegalHold{Holds: service}).
		Execute(context.Background(), actor(), placeCommand(func(*PlaceLegalHoldCommand) {}))
	if err != nil {
		t.Fatalf("placing: %v", err)
	}

	released, err := (ReleaseLegalHold{Holds: service}).
		Execute(context.Background(), actor(), hold.ID, "The proceedings ended")
	if err != nil {
		t.Fatalf("lifting: %v", err)
	}

	if !released.Released() || released.ReleasedReason != "The proceedings ended" {
		t.Fatalf("the hold came back as %+v", released)
	}
	// Both reasons in one entry, so that comparing why it went on with why it came off needs no
	// second lookup.
	entry := h.audit.entries[1]
	if entry.Action != HoldReleasedAction {
		t.Fatalf("the second entry is %s", entry.Action)
	}
	if !carries(entry, "released_reason", "The proceedings ended") ||
		!carries(entry, "reason", "Pending litigation, ref. 4 O 128/26") {
		t.Errorf("the entry carries %+v", entry.Changes)
	}

	// And a second lifting is refused rather than overwriting who lifted it.
	if _, err := (ReleaseLegalHold{Holds: service}).
		Execute(context.Background(), actor(), hold.ID, "again"); err == nil {
		t.Fatal("a hold was lifted twice")
	}
}

// Somebody else lifting it between the read and the write is a conflict, because the caller's
// reading of who lifted it is now wrong.
func TestLiftingLosesTheRaceRatherThanOverwritingIt(t *testing.T) {
	h := newHoldsHarness()
	service := h.service()

	hold, err := (PlaceLegalHold{Holds: service}).
		Execute(context.Background(), actor(), placeCommand(func(*PlaceLegalHoldCommand) {}))
	if err != nil {
		t.Fatalf("placing: %v", err)
	}
	h.holds.refuse = true

	_, err = (ReleaseLegalHold{Holds: service}).
		Execute(context.Background(), actor(), hold.ID, "The proceedings ended")

	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("refused with %v", err)
	}
}

// The default is what is frozen now; the lifted ones are what shows a hold was.
func TestTheListingAnswersWhatIsInForceUnlessAskedForMore(t *testing.T) {
	h := newHoldsHarness()
	service := h.service()

	standing, err := (PlaceLegalHold{Holds: service}).
		Execute(context.Background(), actor(), placeCommand(func(*PlaceLegalHoldCommand) {}))
	if err != nil {
		t.Fatalf("placing: %v", err)
	}
	lifted, err := (PlaceLegalHold{Holds: service}).Execute(context.Background(), actor(),
		placeCommand(func(cmd *PlaceLegalHoldCommand) { cmd.ScopeID = collectionID }))
	if err != nil {
		t.Fatalf("placing the second: %v", err)
	}
	if _, err := (ReleaseLegalHold{Holds: service}).
		Execute(context.Background(), actor(), lifted.ID, "Done"); err != nil {
		t.Fatalf("lifting: %v", err)
	}

	inForce, err := (ListLegalHolds{Holds: service}).Execute(context.Background(), actor(), false)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(inForce) != 1 || inForce[0].ID != standing.ID {
		t.Fatalf("the listing answered %d holds", len(inForce))
	}

	all, err := (ListLegalHolds{Holds: service}).Execute(context.Background(), actor(), true)
	if err != nil {
		t.Fatalf("listing all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("the full listing answered %d holds", len(all))
	}
}

// Reading which holds exist is the administrator's line: knowing what is frozen is part of running
// the workspace, and somebody who may not place one still has to be able to see that one exists.
func TestReadingHoldsIsTheAdministratorsLine(t *testing.T) {
	h := newHoldsHarness()

	if _, err := (ListLegalHolds{Holds: h.service()}).
		Execute(context.Background(), actor(), false); err != nil {
		t.Fatalf("listing: %v", err)
	}
	request := h.authorizer.requests[0]
	if request.Permission != domainservice.PermissionStructure {
		t.Errorf("the listing asked for %q", request.Permission)
	}
	if request.TokenScope != retentionRead {
		t.Errorf("the listing asked for the scope %q", request.TokenScope)
	}
}

// The registry refuses an input key none of the descriptors declares, one layer above every test
// that calls Execute directly.
func TestTheHoldDescriptorsTakeWhatTheControllerSends(t *testing.T) {
	if err := (PlaceLegalHold{}).Descriptor().ValidateInput(usecase.Input{
		"scope": "CONTAINER", "scope_id": hubID.String(), "reason": "Pending litigation",
	}); err != nil {
		t.Fatalf("placing was refused: %v", err)
	}
	if err := (ReleaseLegalHold{}).Descriptor().ValidateInput(usecase.Input{
		"hold_id": holdID.String(), "reason": "The proceedings ended",
	}); err != nil {
		t.Fatalf("lifting was refused: %v", err)
	}
	if err := (ListLegalHolds{}).Descriptor().ValidateInput(usecase.Input{
		"include_released": true,
	}); err != nil {
		t.Fatalf("the listing was refused: %v", err)
	}
	if (PlaceLegalHold{}).Descriptor().ReadOnly {
		t.Error("placing a hold is registered as read-only")
	}
	if !(ListLegalHolds{}).Descriptor().ReadOnly {
		t.Error("the listing is not registered as read-only")
	}
}

// The three channels answer through the handler, so the mapping in between is exercised by the
// test that says the fields are right.
func TestTheHoldHandlersMapWhatTheChannelsSendAndAnswer(t *testing.T) {
	h := newHoldsHarness()
	service := h.service()
	ctx := context.Background()

	placed, err := (PlaceLegalHold{Holds: service}).Descriptor().Handler.Invoke(ctx, actor(),
		usecase.Input{"scope": "TENANT", "reason": "An investigation"})
	if err != nil {
		t.Fatalf("placing through the handler: %v", err)
	}
	scope, present := placed["scope"].(map[string]any)
	if !present || scope["kind"] != string(domain.HoldTenant) {
		t.Fatalf("the scope came back as %+v", placed["scope"])
	}
	if _, released := placed["released_at"]; released {
		t.Error("a new hold came back with a lifting")
	}

	lifted, err := (ReleaseLegalHold{Holds: service}).Descriptor().Handler.Invoke(ctx, actor(),
		usecase.Input{"hold_id": placed.String("id"), "reason": "It ended"})
	if err != nil {
		t.Fatalf("lifting through the handler: %v", err)
	}
	if lifted.String("released_reason") != "It ended" {
		t.Errorf("the lifting came back as %+v", lifted)
	}
	if at := timeOf(lifted["released_at"]); !at.Equal(now) {
		t.Errorf("it was lifted at %s", at)
	}

	listed, err := (ListLegalHolds{Holds: service}).Descriptor().Handler.Invoke(ctx, actor(),
		usecase.Input{"include_released": true})
	if err != nil {
		t.Fatalf("listing through the handler: %v", err)
	}
	if rows, ok := listed["data"].([]usecase.Output); !ok || len(rows) != 1 {
		t.Fatalf("the listing answered %+v", listed)
	}
}

func timeOf(value any) time.Time {
	at, _ := value.(time.Time)
	return at
}
