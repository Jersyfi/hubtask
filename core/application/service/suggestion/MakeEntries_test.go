// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// Turning a piece of text into entries (UC-AI-09).

// keeperID keeps their content out of AI; their name in pasted text is scrubbed before it travels.
var keeperID = shared.MustParseID("0192f000-0000-7000-8000-0000000000e9")

func keeper() integration.KeptOut {
	return integration.NewKeptOut([]integration.KeptPerson{{
		AccountID: keeperID, Reason: integration.KeptOutObjected,
		Names: []string{"Kim Keeper", "kim@example.test"},
	}})
}

// listWorkspace answers the reads the material makes: the collection, with the structure in
// force that a shopping list has - entries at one level that carry notes and an amount, and no
// due date.
type listWorkspace struct {
	kind      string
	structure map[string]any
}

func shoppingList() listWorkspace {
	return listWorkspace{kind: "COLLECTION", structure: map[string]any{"settings": map[string]any{
		"levels": []any{map[string]any{"type": "TASK"}},
		"fields": map[string]any{"TASK": map[string]any{
			"capabilities": []any{"COMPLETION", "NOTES", "AMOUNT"},
		}},
	}}}
}

func (l listWorkspace) Invoke(
	_ context.Context, name string, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if name != "GetContainer" {
		return usecase.Output{}, nil
	}
	return usecase.Output{
		"id": in.String("container_id"), "type": l.kind, "name": "Groceries",
		"structure": l.structure,
	}, nil
}

func entriesAsker() appshared.ActorContext {
	actor := person()
	// A token for entries and nothing of automation: proposed entries are no configuration.
	actor.Scopes = []string{suggestionsRead, suggestionsWrite}
	return actor
}

func making(available bool, kept integration.KeptOut, workspace listWorkspace) (*usecase.Registry, asker, *requestStore) {
	a := newAsker(available)
	requests := newRequestStore()
	a.ask.Cases.Requests, a.ask.Cases.IDs = requests, sequentialIDs{}
	a.ask.Cases.Material = CatalogueSources{Catalogue: workspace, Kept: keptReader{kept}}
	registry, err := usecase.NewRegistry(nil, MakeEntries(a.ask).Descriptor())
	if err != nil {
		panic(err)
	}
	return registry, a, requests
}

// Asking holds the text, queues its own job kind with the collection as a reference, and records
// the asking against the collection - never the words.
func TestMakingEntriesQueuesItsOwnJobForTheCollection(t *testing.T) {
	registry, a, requests := making(true, integration.KeptOut{}, shoppingList())

	out, err := registry.Invoke(context.Background(), MakeEntriesName, entriesAsker(), usecase.Input{
		"collection_id": containerTargetID.String(),
		"text":          "  Pancakes\n200 g flour\n2 eggs\n500 ml milk  ",
	})
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if out.String("job_id") == "" || len(a.jobs.queued) != 1 {
		t.Fatalf("answer %v, %d jobs", out, len(a.jobs.queued))
	}
	job := a.jobs.queued[0]
	if job.Kind != queue.KindAiMakeEntries || job.MaxAttempts != queue.AiMakeEntriesAttempts {
		t.Errorf("job %s with %d attempts", job.Kind, job.MaxAttempts)
	}
	for key, want := range map[string]any{
		"target_type": "ACCOUNT", "target_id": accountID.String(), "kind": "ENTRIES",
		"prompt": makeEntriesPrompt, "scope_type": "COLLECTION", "scope_id": containerTargetID.String(),
	} {
		if job.Payload[key] != want {
			t.Errorf("payload %s = %v, want %v", key, job.Payload[key], want)
		}
	}
	for key, value := range job.Payload {
		if text, isText := value.(string); isText && strings.Contains(text, "flour") {
			t.Errorf("the payload carries the words under %s", key)
		}
	}
	if len(requests.held) != 1 {
		t.Fatalf("%d requests held", len(requests.held))
	}
	for _, held := range requests.held {
		if held.Text != "Pancakes\n200 g flour\n2 eggs\n500 ml milk" || held.AskedBy != accountID {
			t.Errorf("held %+v", held)
		}
	}
	if len(a.world.entries) != 1 || a.world.entries[0].Action != EntriesAskedAction ||
		a.world.entries[0].TargetID != containerTargetID {
		t.Errorf("trail %+v", a.world.entries)
	}
}

// writesNowhere refuses creating entries anywhere: a reader of the collection.
type writesNowhere struct{}

func (writesNowhere) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	if request.Permission == service.PermissionWriteItems {
		return shared.ErrForbidden.WithDetail("access.not_permitted")
	}
	return nil
}

func TestMakingEntriesIsRefusedWhereCreatingThemWouldBe(t *testing.T) {
	hub := shoppingList()
	hub.kind = "HUB"
	for _, testCase := range []struct {
		name      string
		available bool
		kept      integration.KeptOut
		workspace listWorkspace
		reader    bool
		text      string
		detail    string
		category  error
	}{
		{"no provider or consent", false, integration.KeptOut{}, shoppingList(), false, "milk",
			"ai.unavailable", shared.ErrUnavailable},
		{"a reader who may not create there", true, integration.KeptOut{}, shoppingList(), true, "milk",
			"access.not_permitted", shared.ErrForbidden},
		{"no words", true, integration.KeptOut{}, shoppingList(), false, "   ",
			"usecase.input_invalid", shared.ErrValidation},
		{"more than a page", true, integration.KeptOut{}, shoppingList(), false,
			strings.Repeat("a", MaxEntriesText+1), "suggestions.text_invalid", shared.ErrValidation},
		{"a hub rather than a collection", true, integration.KeptOut{}, hub, false, "milk",
			"suggestions.collection_required", shared.ErrValidation},
	} {
		registry, a, _ := making(testCase.available, testCase.kept, testCase.workspace)
		if testCase.reader {
			a.ask.Cases.Authorizer = writesNowhere{}
			registry, _ = usecase.NewRegistry(nil, MakeEntries(a.ask).Descriptor())
		}
		_, err := registry.Invoke(context.Background(), MakeEntriesName, entriesAsker(), usecase.Input{
			"collection_id": containerTargetID.String(), "text": testCase.text,
		})
		if detail := shared.AsError(err); detail == nil || detail.DetailCode != testCase.detail ||
			!errors.Is(err, testCase.category) {
			t.Errorf("%s: %v, want %s", testCase.name, err, testCase.detail)
		}
		if len(a.jobs.queued) != 0 {
			t.Errorf("%s: a job was queued", testCase.name)
		}
	}
}

// UC-AI-09 check 4, at the asking: a person who keeps their own content out is told no AI action
// is theirs, and nothing is queued or sent.
func TestAPersonWhoKeepsTheirContentOutIsNotSentAnything(t *testing.T) {
	kept := integration.NewKeptOut([]integration.KeptPerson{{
		AccountID: accountID, Reason: integration.KeptOutObjected, Names: []string{"Anna Asker"},
	}})
	registry, a, requests := making(true, kept, shoppingList())

	_, err := registry.Invoke(context.Background(), MakeEntriesName, entriesAsker(), usecase.Input{
		"collection_id": containerTargetID.String(), "text": "milk\nflour",
	})
	if detail := shared.AsError(err); detail == nil || detail.DetailCode != "ai.kept_out" {
		t.Fatalf("asking answered %v, want ai.kept_out", err)
	}
	if len(a.jobs.queued) != 0 || len(requests.held) != 0 {
		t.Error("something was queued or held for a person who keeps their content out")
	}
}

func requestFor(id shared.ID, text string) repository.Request {
	return repository.Request{ID: id, AskedBy: accountID, Text: text, CreatedAt: now}
}

func scopeOf(collectionID shared.ID) automation.Scope {
	return automation.Scope{Type: automation.ScopeCollection, ID: collectionID}
}

// entriesProducer is a producer whose material is a shopping list and whose held request is a
// pasted recipe.
func entriesProducer(answer string, workspace listWorkspace, kept integration.KeptOut) (Produce, *producerWorld, Request) {
	produce, w := producer(answer)
	requests := newRequestStore()
	requestID := shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	requests.held[requestID] = requestFor(requestID,
		"Pancakes for Sunday, ask Kim Keeper (kim@example.test) about the jam.\n200 g flour\n2 eggs")
	produce.Requests = requests
	produce.Sources = CatalogueSources{Catalogue: workspace, Kept: keptReader{kept}}
	request := Request{
		TargetType: domain.TargetAccount, TargetID: accountID, Kind: domain.KindEntries,
		PromptID: makeEntriesPrompt, RequestID: requestID,
		Scope: scopeOf(containerTargetID),
	}
	return produce, w, request
}

// The prompt's shape (UC-AI-09 checks 2 and 4): the instruction alone in the system message; in the
// content, the fields the collection carries and the pasted words - scrubbed of the person who
// keeps their content out - and nothing of the collection's name or its entries.
func TestTheQuestionCarriesTheListsFieldsAndTheScrubbedTextOnly(t *testing.T) {
	produce, w, request := entriesProducer(`{"entries": [{"title": "Flour"}]}`, shoppingList(), keeper())

	if _, err := produce.Ask(context.Background(), person(), request); err != nil {
		t.Fatalf("asking: %v", err)
	}
	if len(w.asked) != 1 || len(w.asked[0].Messages) != 2 {
		t.Fatalf("asked %+v", w.asked)
	}
	instruction, content := w.asked[0].Messages[0].Content, w.asked[0].Messages[1].Content
	if instruction != "describe what follows" || strings.Contains(instruction, "flour") {
		t.Errorf("the instruction is %q", instruction)
	}
	for _, want := range []string{
		"Fields an entry of this list carries: title, notes, amount.",
		"The text the person pasted, to turn into entries:",
		"200 g flour",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("the content lacks %q:\n%s", want, content)
		}
	}
	for _, never := range []string{"Kim", "Keeper", "kim@example.test", "Groceries", "Today's date"} {
		if strings.Contains(content, never) {
			t.Errorf("the content carries %q:\n%s", never, content)
		}
	}
}

// A list that carries a due date is told what day it is, so that "Friday" can be resolved.
func TestAListThatCarriesADueDateIsToldTheDate(t *testing.T) {
	dated := shoppingList()
	dated.structure = nil
	produce, w, request := entriesProducer(`{"entries": [{"title": "Flour"}]}`, dated, integration.KeptOut{})

	if _, err := produce.Ask(context.Background(), person(), request); err != nil {
		t.Fatalf("asking: %v", err)
	}
	content := w.asked[0].Messages[1].Content
	if !strings.Contains(content, "carries: title, notes, due_date.") ||
		!strings.Contains(content, "Today's date") {
		t.Errorf("a collection with no structure of its own is every default field and dated:\n%s", content)
	}
}

// What the model answers is held to what it was shown (UC-AI-09 check 2): only the collection's
// fields survive, the collection and the level are the material's, an entry without a title is
// dropped and counted, and every entry gets its key.
func TestAnAnswerKeepsOnlyWhatTheListCarries(t *testing.T) {
	answer := `{"entries": [
		{"title": "Flour", "amount": "200 g", "due_date": "2026-09-13", "notes": "plain"},
		{"title": "  ", "amount": "2"},
		{"title": "Eggs", "amount": "2\nor three", "collection_id": "0192f000-0000-7000-8000-0000000000aa", "type": "ACTIVITY"}
	], "collection_id": "0192f000-0000-7000-8000-0000000000aa"}`
	produce, w, request := entriesProducer(answer, shoppingList(), integration.KeptOut{})

	outcome, err := produce.Ask(context.Background(), person(), request)
	if err != nil || !outcome.Recorded || outcome.Dropped != 1 {
		t.Fatalf("outcome %+v, %v", outcome, err)
	}
	var recorded domain.Suggestion
	for _, held := range w.store.proposals {
		recorded = held
	}
	if recorded.Kind != domain.KindEntries || recorded.TargetType != domain.TargetAccount ||
		recorded.TargetID != accountID || recorded.DroppedNodes != 1 {
		t.Errorf("recorded %+v", recorded)
	}
	if recorded.Payload[collectionKey] != containerTargetID.String() || recorded.Payload["type"] != "TASK" {
		t.Errorf("the place is %v / %v, want the material's", recorded.Payload[collectionKey], recorded.Payload["type"])
	}
	entries, _ := recorded.Payload[entriesKey].([]any)
	if len(entries) != 2 {
		t.Fatalf("entries %v", entries)
	}
	first, second := entries[0].(map[string]any), entries[1].(map[string]any)
	if first["key"] != "1" || first["title"] != "Flour" || first["amount"] != "200 g" ||
		first["notes"] != "plain" || first[dueKey] != nil {
		t.Errorf("first %v: the due date is not this list's", first)
	}
	if second["key"] != "2" || second["title"] != "Eggs" || second["amount"] != nil ||
		second[collectionKey] != nil || second["type"] != nil {
		t.Errorf("second %v: a two-line amount and an entry's own place are not kept", second)
	}
}

func TestAProposalHoldsAtMostFiftyEntries(t *testing.T) {
	var many []string
	for i := range maxProposedEntries + 5 {
		many = append(many, fmt.Sprintf(`{"title": "Item %d"}`, i))
	}
	produce, w, request := entriesProducer(`{"entries": [`+strings.Join(many, ",")+`]}`, shoppingList(), integration.KeptOut{})

	outcome, err := produce.Ask(context.Background(), person(), request)
	if err != nil || outcome.Dropped != 5 {
		t.Fatalf("outcome %+v, %v", outcome, err)
	}
	for _, held := range w.store.proposals {
		if entries, _ := held.Payload[entriesKey].([]any); len(entries) != maxProposedEntries {
			t.Errorf("%d entries kept", len(entries))
		}
	}
}

func TestAnAnswerWithNoEntriesRecordsNothing(t *testing.T) {
	produce, w, request := entriesProducer(`{"entries": []}`, shoppingList(), integration.KeptOut{})

	outcome, err := produce.Ask(context.Background(), person(), request)
	if err != nil || outcome.Recorded || outcome.Code != OutcomeNothingToChange || len(w.store.proposals) != 0 {
		t.Errorf("outcome %+v, %v, %d recorded", outcome, err, len(w.store.proposals))
	}
}

// Taking (UC-AI-09 check 3).

var entriesProposalID = shared.MustParseID("0192f000-0000-7000-8000-0000000000e2")

func entriesProposal() domain.Suggestion {
	recorded, err := domain.New(domain.NewInput{
		ID: entriesProposalID, TenantID: tenantID,
		TargetType: domain.TargetAccount, TargetID: accountID, Kind: domain.KindEntries,
		Payload: map[string]any{
			collectionKey: containerTargetID.String(), "type": "TASK",
			entriesKey: []any{
				map[string]any{"key": "1", "title": "Flour", "amount": "200 g"},
				map[string]any{"key": "2", "title": "Eggs", dueKey: "2026-09-13"},
				map[string]any{"key": "3", "title": "Milk", "notes": "oat"},
			},
		},
		Provenance: domain.Provenance{
			Model: "a-model", PromptID: makeEntriesPrompt, PromptVersion: "v1", ProducedAt: now.Add(-time.Minute),
		},
		InputDigest: domain.Digest(accountID.String(), ""),
		Now:         now,
	})
	if err != nil {
		panic(err)
	}
	return recorded
}

func created(w *world) []performed {
	var creates []performed
	for _, call := range w.performed {
		if call.name == createWorkItemName {
			creates = append(creates, call)
		}
	}
	return creates
}

// Take all: one CreateWorkItem per entry, in order, in the proposal's collection and at its level,
// as the person - and the proposal is accepted, with what was created answered.
func TestTakingAllCreatesEveryEntryInTheCollection(t *testing.T) {
	cases, w := newWorld()
	w.store.proposals[entriesProposalID] = entriesProposal()
	asker := entriesAsker()
	asker.TimeZone = "Europe/Berlin"

	accepted, ids, err := AcceptSuggestion{Cases: cases}.Take(context.Background(), asker, entriesProposalID,
		map[string]any{"collection_id": "0192f000-0000-7000-8000-0000000000aa", "parent_id": targetID.String()}, nil)
	if err != nil {
		t.Fatalf("taking: %v", err)
	}
	if accepted.Status != domain.StatusAccepted || len(ids) != 3 {
		t.Errorf("status %s, %d created", accepted.Status, len(ids))
	}
	creates := created(w)
	if len(creates) != 3 {
		t.Fatalf("%d creates", len(creates))
	}
	for i, title := range []string{"Flour", "Eggs", "Milk"} {
		in := creates[i].in
		if in["title"] != title || in[collectionKey] != containerTargetID.String() || in["type"] != "TASK" ||
			in["parent_id"] != nil || creates[i].actor.AccountID != accountID {
			t.Errorf("create %d: %v", i, in)
		}
	}
	if creates[0].in["amount"] != "200 g" || creates[2].in["notes"] != "oat" {
		t.Errorf("the fields did not travel: %v / %v", creates[0].in, creates[2].in)
	}
	eggs := creates[1].in
	if eggs["due_at"] != "2026-09-13T00:00:00+02:00" || eggs["due_date_only"] != true ||
		eggs["due_time_zone"] != "Europe/Berlin" {
		t.Errorf("the date is %v, want the day in the person's zone", eggs)
	}
}

// Take one: that entry is created and the rest stays proposed, to be taken or dismissed; taking the
// last accepts the proposal.
func TestTakingOneEntryLeavesTheRestProposed(t *testing.T) {
	cases, w := newWorld()
	w.store.proposals[entriesProposalID] = entriesProposal()
	accept := AcceptSuggestion{Cases: cases}

	left, ids, err := accept.Take(context.Background(), entriesAsker(), entriesProposalID, nil, []string{"2"})
	if err != nil {
		t.Fatalf("taking one: %v", err)
	}
	if left.Status != domain.StatusProposed || len(ids) != 1 || len(created(w)) != 1 ||
		created(w)[0].in["title"] != "Eggs" {
		t.Fatalf("status %s, %d created, calls %v", left.Status, len(ids), created(w))
	}
	remaining, _ := w.store.proposals[entriesProposalID].Payload[entriesKey].([]any)
	if len(remaining) != 2 || remaining[0].(map[string]any)["key"] != "1" || remaining[1].(map[string]any)["key"] != "3" {
		t.Errorf("what is left is %v", remaining)
	}

	// The same key again is gone, and nothing is created for it.
	if _, _, err := accept.Take(context.Background(), entriesAsker(), entriesProposalID, nil, []string{"2"}); shared.AsError(err).DetailCode != "suggestions.entry_unknown" {
		t.Errorf("taking a taken entry answered %v", err)
	}
	if len(created(w)) != 1 {
		t.Errorf("%d creates after a refusal", len(created(w)))
	}

	done, ids, err := accept.Take(context.Background(), entriesAsker(), entriesProposalID, nil, []string{"1", "3"})
	if err != nil || done.Status != domain.StatusAccepted || len(ids) != 2 {
		t.Errorf("taking the rest: %s, %d, %v", done.Status, len(ids), err)
	}
}

// Nothing is created before somebody takes it, and dismissing creates nothing.
func TestDismissingCreatesNothing(t *testing.T) {
	cases, w := newWorld()
	w.store.proposals[entriesProposalID] = entriesProposal()

	dismissed, err := DismissSuggestion{Cases: cases}.Execute(context.Background(), entriesAsker(), entriesProposalID)
	if err != nil || dismissed.Status != domain.StatusDismissed || len(created(w)) != 0 {
		t.Errorf("status %s, %d creates, %v", dismissed.Status, len(created(w)), err)
	}
}

// A refusal of one entry is the answer: the proposal is not decided, and nothing narrows it.
func TestARefusedEntryLeavesTheProposalStanding(t *testing.T) {
	cases, w := newWorld()
	w.store.proposals[entriesProposalID] = entriesProposal()
	w.failCreateAfter = 1

	if _, _, err := (AcceptSuggestion{Cases: cases}).Take(context.Background(), entriesAsker(), entriesProposalID, nil, nil); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("taking answered %v", err)
	}
	standing := w.store.proposals[entriesProposalID]
	if standing.Status != domain.StatusProposed || standing.Version != 1 {
		t.Errorf("the proposal is %s at version %d", standing.Status, standing.Version)
	}
}

func TestOnlyEntriesAreTakenInPart(t *testing.T) {
	cases, w := newWorld()
	w.store.proposals[draftID] = ruleDraftFor(accountID)

	_, _, err := AcceptSuggestion{Cases: cases}.Take(context.Background(), drafter(), draftID,
		map[string]any{"name": "x", "run_as": serviceID.String()}, []string{"1"})
	if detail := shared.AsError(err); detail == nil || detail.DetailCode != "suggestions.take_not_for_kind" {
		t.Errorf("taking part of a rule answered %v", err)
	}
	if _, _, err := (AcceptSuggestion{Cases: cases}).Take(context.Background(), entriesAsker(), draftID, nil, []string{}); err == nil {
		t.Error("an empty take was accepted")
	}
}

// The proposal is the asker's alone: anybody else is told there is no such suggestion.
func TestNobodyElseTakesYourEntries(t *testing.T) {
	cases, w := newWorld()
	w.store.proposals[entriesProposalID] = entriesProposal()
	other := entriesAsker()
	other.AccountID = keeperID

	_, _, err := AcceptSuggestion{Cases: cases}.Take(context.Background(), other, entriesProposalID, nil, nil)
	if !errors.Is(err, shared.ErrNotFound) || len(created(w)) != 0 {
		t.Errorf("another person's take answered %v with %d creates", err, len(created(w)))
	}
}

// Entries are no configuration: a token that writes entries and nothing else asks for them and
// takes them, and listing them asks only the read of entries - automation's scope is never asked.
func TestATokenForEntriesAloneAsksListsAndTakesThem(t *testing.T) {
	writeOnly := entriesAsker()
	writeOnly.Scopes = []string{suggestionsWrite}

	registry, a, _ := making(true, integration.KeptOut{}, shoppingList())
	if _, err := registry.Invoke(context.Background(), MakeEntriesName, writeOnly, usecase.Input{
		"collection_id": containerTargetID.String(), "text": "Flour\nEggs",
	}); err != nil || len(a.jobs.queued) != 1 {
		t.Fatalf("a token with items:write alone could not ask: %v, %d jobs", err, len(a.jobs.queued))
	}

	cases, w := newWorld()
	w.store.proposals[entriesProposalID] = entriesProposal()
	readOnly := entriesAsker()
	readOnly.Scopes = []string{suggestionsRead}
	page, err := (ListSuggestions{Cases: cases}).Execute(context.Background(), readOnly, ListQuery{
		TargetType: domain.TargetAccount, TargetID: accountID,
	})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("a token with items:read alone does not list them: %v, %d", err, len(page.Items))
	}
	automationOnly := entriesAsker()
	automationOnly.Scopes = []string{automationScope}
	if page, err := (ListSuggestions{Cases: cases}).Execute(context.Background(), automationOnly, ListQuery{
		TargetType: domain.TargetAccount, TargetID: accountID,
	}); err != nil || len(page.Items) != 0 {
		t.Errorf("a token for automation alone listed them: %v, %d", err, len(page.Items))
	}

	if _, ids, err := (AcceptSuggestion{Cases: cases}).Take(context.Background(), writeOnly, entriesProposalID, nil, nil); err != nil || len(ids) != 3 {
		t.Fatalf("a token with items:write alone could not take them: %v, %d created", err, len(ids))
	}
}
