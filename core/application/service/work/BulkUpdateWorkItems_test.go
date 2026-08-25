// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// What a bulk owns: the order, and what happens when one operation fails (C-11). What each
// operation does is the use case performing it, which is why the double below is a catalogue rather
// than a set of item fakes - a bulk that reimplemented the operations would be the design this one
// exists not to be.

var bulkItemID = shared.MustParseID("0192f000-0000-7000-8000-0000000b0101")

// catalogueDouble records what it was asked to run, and refuses the operations a test names.
type catalogueDouble struct {
	invoked []invocation
	// refuse maps a use case name to the refusal it answers with. Everything else succeeds.
	refuse map[string]error
	// refuseAt refuses the invocation at this position, counted from zero, whatever it is: that is
	// how a test says "the third operation fails" without caring which one it is.
	refuseAt int
	failure  error
}

type invocation struct {
	name string
	in   usecase.Input
}

func newCatalogueDouble() *catalogueDouble {
	return &catalogueDouble{refuse: map[string]error{}, refuseAt: -1}
}

func (c *catalogueDouble) Invoke(
	_ context.Context, name string, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	position := len(c.invoked)
	c.invoked = append(c.invoked, invocation{name: name, in: in})

	if refusal, refused := c.refuse[name]; refused {
		return nil, refusal
	}
	if position == c.refuseAt {
		return nil, c.failure
	}
	return usecase.Output{"id": bulkItemID.String(), "version": 2}, nil
}

type bulkHarness struct {
	handler   BulkUpdateWorkItems
	catalogue *catalogueDouble
	audit     *sink
	uow       *unitOfWork
}

func newBulkHarness() *bulkHarness {
	h := &bulkHarness{catalogue: newCatalogueDouble(), audit: &sink{}, uow: &unitOfWork{}}
	h.handler = BulkUpdateWorkItems{
		Catalogue: h.catalogue, Audit: h.audit, UnitOfWork: h.uow, Clock: clock.Fixed(now),
	}
	return h
}

// operations builds a bulk of completions, one per entry, which is the shape a client sends when it
// ticks off a page of a list.
func operations(count int) []BulkOperation {
	built := make([]BulkOperation, 0, count)
	for range count {
		built = append(built, BulkOperation{Op: "COMPLETE_ITEM", ItemID: bulkItemID})
	}
	return built
}

// An operation is the single-entry operation, run through the catalogue: the same use case, with
// the entry and the body the operation carried.
func TestABulkRunsEachOperationThroughTheUseCaseThatOwnsIt(t *testing.T) {
	h := newBulkHarness()

	result, err := h.handler.Execute(t.Context(), actor(), BulkUpdateWorkItemsCommand{
		Operations: []BulkOperation{
			{Op: "COMPLETE_ITEM", ItemID: bulkItemID},
			{Op: "UPDATE_ITEM", ItemID: bulkItemID, Payload: map[string]any{"title": "Oat milk"}},
			{Op: "CREATE_ITEM", Payload: map[string]any{"type": "TASK", "title": "Buy milk"}},
		},
	})
	if err != nil {
		t.Fatalf("the bulk was refused: %v", err)
	}

	if result.Applied != 3 || result.Failed != 0 {
		t.Fatalf("%d applied, %d failed", result.Applied, result.Failed)
	}
	if len(h.catalogue.invoked) != 3 {
		t.Fatalf("the catalogue ran %d operations", len(h.catalogue.invoked))
	}
	if h.catalogue.invoked[0].name != CompleteWorkItemName ||
		h.catalogue.invoked[1].name != UpdateWorkItemName ||
		h.catalogue.invoked[2].name != CreateWorkItemName {
		t.Errorf("the operations ran as %+v", h.catalogue.invoked)
	}
	// The entry travels as the field the single-entry operation takes, and so does the body.
	if h.catalogue.invoked[1].in["item_id"] != bulkItemID.String() ||
		h.catalogue.invoked[1].in["title"] != "Oat milk" {
		t.Errorf("the second operation was run with %v", h.catalogue.invoked[1].in)
	}
	// A creation has no entry yet, and must not be handed an empty one.
	if _, sent := h.catalogue.invoked[2].in["item_id"]; sent {
		t.Errorf("the creation was handed an entry: %v", h.catalogue.invoked[2].in)
	}
}

// The acceptance criterion: a bulk with one invalid operation applies the rest and reports the one.
func TestABulkAppliesWhatItCanAndReportsTheRest(t *testing.T) {
	h := newBulkHarness()
	h.catalogue.refuseAt = 1
	h.catalogue.failure = shared.ErrNotFound.WithDetail("items.not_found")

	result, err := h.handler.Execute(t.Context(), actor(), BulkUpdateWorkItemsCommand{
		Operations: operations(3),
	})
	if err != nil {
		t.Fatalf("the bulk was refused: %v", err)
	}

	if result.Applied != 2 || result.Failed != 1 {
		t.Fatalf("%d applied, %d failed", result.Applied, result.Failed)
	}
	// The operation after the failed one was still tried: without `atomic`, one refusal takes
	// nothing with it.
	if len(h.catalogue.invoked) != 3 {
		t.Errorf("the catalogue ran %d operations", len(h.catalogue.invoked))
	}
	if outcome := result.Outcomes[1]; outcome.Err == nil ||
		outcome.Err.DetailCode != "items.not_found" || outcome.Index != 1 {
		t.Errorf("the second outcome is %+v", outcome)
	}
	for _, index := range []int{0, 2} {
		if result.Outcomes[index].Err != nil {
			t.Errorf("outcome %d failed: %+v", index, result.Outcomes[index])
		}
	}
	// One entry for the bulk itself, whatever happened inside it - and a bulk that lost an
	// operation is not a success to an auditor asking whether what somebody asked for happened.
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ItemBulkAppliedAction {
		t.Fatalf("the trail is %+v", h.audit.entries)
	}
	if outcome := h.audit.entries[0].Outcome; string(outcome) != "FAILED" {
		t.Errorf("the trail calls the bulk %s", outcome)
	}
}

// And the same input with `atomic` applies none of it.
func TestAnAtomicBulkAppliesNoneOfItWhenOneOperationFails(t *testing.T) {
	h := newBulkHarness()
	h.catalogue.refuseAt = 1
	h.catalogue.failure = shared.ErrVersionConflict.WithDetail("items.version_conflict")

	result, err := h.handler.Execute(t.Context(), actor(), BulkUpdateWorkItemsCommand{
		Atomic: true, Operations: operations(3),
	})
	if err != nil {
		t.Fatalf("the bulk was refused: %v", err)
	}

	if result.Applied != 0 || result.Failed != 3 {
		t.Fatalf("%d applied, %d failed", result.Applied, result.Failed)
	}
	if !h.uow.rolledBack {
		t.Error("the transaction was not rolled back")
	}
	// The operation after the failure never ran: the first refusal ends an atomic bulk.
	if len(h.catalogue.invoked) != 2 {
		t.Errorf("the catalogue ran %d operations", len(h.catalogue.invoked))
	}
	if outcome := result.Outcomes[1]; outcome.Err == nil ||
		outcome.Err.DetailCode != "items.version_conflict" {
		t.Errorf("the refusal is reported as %+v", outcome)
	}
	// Everything else says it was not applied, and names the operation that ended the bulk.
	for _, index := range []int{0, 2} {
		outcome := result.Outcomes[index]
		if outcome.Err == nil || outcome.Err.DetailCode != "bulk.rolled_back" ||
			outcome.Err.Params["failed_index"] != "1" {
			t.Errorf("outcome %d is %+v", index, outcome)
		}
	}
	// The trail records the bulk even though nothing applied - it is written outside the
	// transaction that unwound, which is the only reason it survives.
	if len(h.audit.entries) != 1 {
		t.Fatalf("the trail is %+v", h.audit.entries)
	}
	applied, _ := h.audit.entries[0].Changes["applied"].(map[string]any)
	if applied["to"] != "0" {
		t.Errorf("the trail says %v applied", h.audit.entries[0].Changes["applied"])
	}
}

// An operation this installation does not offer is that operation's failure rather than the bulk's:
// a client that mistyped one of five hundred is told which one.
func TestAnUnknownOperationFailsOnlyItself(t *testing.T) {
	h := newBulkHarness()

	result, err := h.handler.Execute(t.Context(), actor(), BulkUpdateWorkItemsCommand{
		Operations: []BulkOperation{
			{Op: "COMPLETE_ITEM", ItemID: bulkItemID},
			{Op: "DELETE_EVERYTHING", ItemID: bulkItemID},
		},
	})
	if err != nil {
		t.Fatalf("the bulk was refused: %v", err)
	}
	if result.Applied != 1 || result.Failed != 1 {
		t.Fatalf("%d applied, %d failed", result.Applied, result.Failed)
	}
	if outcome := result.Outcomes[1]; outcome.Err == nil ||
		outcome.Err.DetailCode != "bulk.operation_unknown" {
		t.Errorf("the unknown operation is reported as %+v", outcome)
	}
	if len(h.catalogue.invoked) != 1 {
		t.Errorf("the catalogue was asked to run something it does not offer: %+v", h.catalogue.invoked)
	}
}

// The cap is about the request rather than about any operation in it, so it refuses the bulk.
func TestABulkOverTheCapIsRefusedWhole(t *testing.T) {
	h := newBulkHarness()

	_, err := h.handler.Execute(t.Context(), actor(), BulkUpdateWorkItemsCommand{
		Operations: operations(maxBulkOperations + 1),
	})
	if !errors.Is(err, shared.ErrValidation) ||
		shared.AsError(err).DetailCode != "bulk.too_many_operations" {
		t.Fatalf("a bulk over the cap answered %v", err)
	}
	if len(h.catalogue.invoked) != 0 {
		t.Error("something ran all the same")
	}
	if shared.AsError(err).Params["limit"] != strconv.Itoa(maxBulkOperations) {
		t.Errorf("the refusal does not name the cap: %v", shared.AsError(err).Params)
	}
}

// A bulk at the cap is not over it.
func TestABulkAtTheCapIsApplied(t *testing.T) {
	h := newBulkHarness()

	result, err := h.handler.Execute(t.Context(), actor(), BulkUpdateWorkItemsCommand{
		Operations: operations(maxBulkOperations),
	})
	if err != nil {
		t.Fatalf("a bulk of %d was refused: %v", maxBulkOperations, err)
	}
	if result.Applied != maxBulkOperations {
		t.Errorf("%d of %d applied", result.Applied, maxBulkOperations)
	}
}

// A bulk that asks for nothing is the caller's mistake, and says which field it is about.
func TestABulkWithNoOperationsIsRefused(t *testing.T) {
	h := newBulkHarness()

	_, err := h.handler.Execute(t.Context(), actor(), BulkUpdateWorkItemsCommand{})
	if !errors.Is(err, shared.ErrValidation) ||
		shared.AsError(err).DetailCode != "bulk.no_operations" {
		t.Fatalf("an empty bulk answered %v", err)
	}
}

// The untyped input a channel hands over is read into operations, and what cannot be read at all is
// refused as a request rather than reported as an operation that failed.
func TestTheOperationsAreReadFromTheChannelsInput(t *testing.T) {
	h := newBulkHarness()

	out, err := h.handler.invoke(t.Context(), actor(), usecase.Input{
		"operations": []any{
			map[string]any{
				"op": "UPDATE_ITEM", "item_id": bulkItemID.String(),
				"payload": map[string]any{"title": "Oat milk"},
			},
		},
	})
	if err != nil {
		t.Fatalf("the bulk was refused: %v", err)
	}
	results, ok := out["results"].([]usecase.Output)
	if !ok || len(results) != 1 {
		t.Fatalf("the answer is %v", out)
	}
	if results[0]["op"] != "UPDATE_ITEM" || results[0]["index"] != 0 {
		t.Errorf("the result is %v", results[0])
	}

	for name, in := range map[string]usecase.Input{
		"bulk.operation_malformed": {"operations": []any{"COMPLETE_ITEM"}},
		"bulk.operation_required":  {"operations": []any{map[string]any{"item_id": bulkItemID.String()}}},
		"bulk.item_id_malformed":   {"operations": []any{map[string]any{"op": "COMPLETE_ITEM", "item_id": "milk"}}},
		"bulk.payload_malformed":   {"operations": []any{map[string]any{"op": "COMPLETE_ITEM", "payload": "milk"}}},
	} {
		if _, err := h.handler.invoke(t.Context(), actor(), in); err == nil ||
			shared.AsError(err).DetailCode != name {
			t.Errorf("%s answered %v", name, err)
		}
	}
}

// A refusal travels as data rather than as an error value: the call succeeded, and this is what
// happened inside it. The fields are the error model's own, so that a channel can rebuild the
// answer it would have given for the same refusal outside a bulk.
func TestARefusalTravelsAsTheErrorModelsOwnFields(t *testing.T) {
	h := newBulkHarness()
	h.catalogue.refuseAt = 0
	h.catalogue.failure = shared.ErrValidation.
		WithDetail("items.update_empty").
		WithParams(map[string]string{"item_id": bulkItemID.String()}).
		WithFields(shared.FieldError{Path: "/title", Code: "items.title_required"})

	out, err := h.handler.invoke(t.Context(), actor(), usecase.Input{
		"operations": []any{map[string]any{"op": "UPDATE_ITEM", "item_id": bulkItemID.String()}},
	})
	if err != nil {
		t.Fatalf("the bulk was refused: %v", err)
	}

	results, _ := out["results"].([]usecase.Output)
	problem, held := results[0]["problem"].(usecase.Output)
	if !held {
		t.Fatalf("the result carries no problem: %v", results[0])
	}
	if problem["code"] != shared.ErrValidation.Code || problem["detail_code"] != "items.update_empty" {
		t.Errorf("the problem is %v", problem)
	}
	if problem["category"] != string(shared.CategoryValidation) {
		t.Errorf("the problem carries no category a status can be derived from: %v", problem)
	}
	findings, carried := problem["field_errors"].([]usecase.Output)
	if !carried || len(findings) != 1 || findings[0]["path"] != "/title" {
		t.Errorf("the findings are %v", problem["field_errors"])
	}
}
