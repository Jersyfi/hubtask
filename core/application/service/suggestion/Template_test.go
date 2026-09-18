// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A template drafted from a description (P-11): the last of ai-first.md §2's seven rows.

// requestStore holds the words a question was asked with, as the real one does under row level
// security.
type requestStore struct {
	held    map[shared.ID]repository.Request
	deleted []shared.ID
}

func newRequestStore() *requestStore {
	return &requestStore{held: map[shared.ID]repository.Request{}}
}

func (s *requestStore) Put(_ context.Context, request repository.Request) error {
	s.held[request.ID] = request
	return nil
}

func (s *requestStore) Get(_ context.Context, id shared.ID) (repository.Request, error) {
	request, found := s.held[id]
	if !found {
		return repository.Request{}, shared.ErrNotFound.WithDetail("suggestions.request_not_found")
	}
	return request, nil
}

func (s *requestStore) Delete(_ context.Context, id shared.ID) error {
	delete(s.held, id)
	s.deleted = append(s.deleted, id)
	return nil
}

// The profiles the installation ships: a task takes work packages and activities, a work package
// takes activities, an activity takes nothing.
type shippedProfiles struct{}

func (shippedProfiles) List(context.Context) ([]work.CapabilityProfile, error) {
	return []work.CapabilityProfile{
		{Type: work.ItemTask, AllowedChildTypes: []work.ItemType{work.ItemWorkPackage, work.ItemActivity}, MaxDepth: 3},
		{Type: work.ItemWorkPackage, AllowedChildTypes: []work.ItemType{work.ItemActivity}, MaxDepth: 2},
		{Type: work.ItemActivity, MaxDepth: 1},
	}, nil
}

func (p shippedProfiles) ListSystem(ctx context.Context) ([]work.CapabilityProfile, error) {
	return p.List(ctx)
}

const requestID = "0192f000-0000-7000-8000-0000000000d9"

// Asking holds the words for the job, names them in the payload, and sends none of them there.
func TestAskingForATemplateHoldsTheWordsAndQueuesAReference(t *testing.T) {
	a := newAsker(true)
	requests := newRequestStore()
	a.ask.Cases.Requests, a.ask.Cases.IDs = requests, sequentialIDs{}

	if err := (AiGenerateTemplate(a.ask)).Execute(context.Background(), person(),
		containerTargetID, "  Onboarding a new colleague: accounts, introductions, the first week.  "); err != nil {
		t.Fatalf("asking: %v", err)
	}
	if len(a.jobs.queued) != 1 {
		t.Fatalf("%d jobs queued", len(a.jobs.queued))
	}
	job := a.jobs.queued[0]
	if job.Payload["kind"] != "TEMPLATE" || job.Payload["target_type"] != "CONTAINER" || job.Payload["prompt"] != templatePrompt {
		t.Errorf("the job is %v", job.Payload)
	}
	if job.Payload["request_id"] != requestID {
		t.Errorf("the job names %v, want the held request", job.Payload["request_id"])
	}
	for key, value := range job.Payload {
		if text, isText := value.(string); isText && strings.Contains(text, "Onboarding") {
			t.Errorf("the words travelled in the payload under %s", key)
		}
	}
	held := requests.held[shared.MustParseID(requestID)]
	if held.Text != "Onboarding a new colleague: accounts, introductions, the first week." || held.AskedBy != accountID {
		t.Errorf("held %+v", held)
	}
	if len(a.world.entries) != 1 || a.world.entries[0].Action != TemplateAskedAction {
		t.Errorf("audited %+v", a.world.entries)
	}
}

func TestAskingForATemplateRefusesNoWordsAndTooMany(t *testing.T) {
	for name, description := range map[string]string{
		"nothing":  "   ",
		"too many": strings.Repeat("x", MaxTemplateDescription+1),
	} {
		t.Run(name, func(t *testing.T) {
			a := newAsker(true)
			a.ask.Cases.Requests, a.ask.Cases.IDs = newRequestStore(), sequentialIDs{}
			err := (AiGenerateTemplate(a.ask)).Execute(context.Background(), person(), containerTargetID, description)
			if !errors.Is(err, shared.ErrValidation) {
				t.Errorf("the answer was %v", err)
			}
			if len(a.jobs.queued) != 0 {
				t.Error("a refused ask queued a job")
			}
		})
	}
}

func TestAskingForATemplateIsRefusedWithoutAProvider(t *testing.T) {
	a := newAsker(false)
	requests := newRequestStore()
	a.ask.Cases.Requests, a.ask.Cases.IDs = requests, sequentialIDs{}
	err := (AiGenerateTemplate(a.ask)).Execute(context.Background(), person(), containerTargetID, "a template")
	if !IsUnavailable(err) {
		t.Fatalf("the answer was %v, want ai.unavailable", err)
	}
	if len(requests.held) != 0 || len(a.jobs.queued) != 0 {
		t.Error("a refused ask held words or queued a job")
	}
}

func TestTheDescriptorTakesTheCollectionAndTheWords(t *testing.T) {
	a := newAsker(true)
	a.ask.Cases.Requests, a.ask.Cases.IDs = newRequestStore(), sequentialIDs{}
	if _, err := (AiGenerateTemplate(a.ask)).invoke(context.Background(), person(), usecase.Input{
		"collection_id": containerTargetID.String(), "description": "a template",
	}); err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if len(a.jobs.queued) != 1 {
		t.Error("nothing was queued")
	}
}

// The producer, over a fake provider.

func templateProducer(answer string) (Produce, *producerWorld, *requestStore) {
	produce, world := producer(answer)
	requests := newRequestStore()
	requests.held[shared.MustParseID(requestID)] = repository.Request{
		ID: shared.MustParseID(requestID), AskedBy: accountID,
		Text: "Onboarding a new colleague: the accounts to create, and the introductions in the first week.",
	}
	produce.Requests = requests
	produce.Sources = CatalogueSources{Catalogue: world, Profiles: shippedProfiles{}}
	return produce, world, requests
}

func templateRequest() Request {
	return Request{
		TargetType: domain.TargetContainer, TargetID: containerTargetID, Kind: domain.KindTemplate,
		RequestID: shared.MustParseID(requestID),
	}
}

const wellFormed = `{"name":"Onboarding","description":"A new colleague's first week.","nodes":[
	{"type":"TASK","title":"Onboard {name}","due_offset":"P1W","children":[
		{"type":"WORK_PACKAGE","title":"Accounts","children":[
			{"type":"ACTIVITY","title":"Create the mail account","due_offset":"P1D"},
			{"type":"ACTIVITY","title":"Add to the chat","notes":"the team channel too"}]},
		{"type":"ACTIVITY","title":"Introduce to the team","due_offset":"P1Y"}]}]}`

func TestAWellFormedAnswerIsRecordedAsATemplateForTheCollection(t *testing.T) {
	produce, world, requests := templateProducer(wellFormed)

	outcome, err := produce.Ask(context.Background(), person(), templateRequest())
	if err != nil {
		t.Fatalf("producing: %v", err)
	}
	if !outcome.Recorded || outcome.Dropped != 0 {
		t.Errorf("outcome = %+v", outcome)
	}
	// The material told the model the shape it is held to, and then the person's words - as
	// content, after the instruction.
	if len(world.asked) != 1 {
		t.Fatalf("%d questions asked", len(world.asked))
	}
	material := world.asked[0].Messages[len(world.asked[0].Messages)-1].Content
	for _, expected := range []string{"Collection: This quarter", "A root node is one of: TASK", "- WORK_PACKAGE: ACTIVITY", "- ACTIVITY: nothing", "in the person's words:\nOnboarding a new colleague"} {
		if !strings.Contains(material, expected) {
			t.Errorf("the material lacks %q:\n%s", expected, material)
		}
	}
	if len(world.store.proposals) != 1 {
		t.Fatalf("%d suggestions recorded", len(world.store.proposals))
	}
	for _, recorded := range world.store.proposals {
		if recorded.Kind != domain.KindTemplate || recorded.TargetType != domain.TargetContainer || recorded.TargetID != containerTargetID {
			t.Errorf("recorded %+v", recorded)
		}
		payload := recorded.Payload
		// The scope is the target's, written by the producer and never read from the answer.
		if payload["scope_type"] != "COLLECTION" || payload["scope_id"] != containerTargetID.String() || payload["root_type"] != "TASK" {
			t.Errorf("the scope is %v %v %v", payload["scope_type"], payload["scope_id"], payload["root_type"])
		}
		if payload["name"] != "Onboarding" || payload["description"] != "A new colleague's first week." {
			t.Errorf("named %v, %v", payload["name"], payload["description"])
		}
		root := payload["nodes"].([]any)[0].(map[string]any)
		if root["title"] != "Onboard {name}" || root["due_offset"] != "P1W" || root["due_date_only"] != true {
			t.Errorf("the root is %v", root)
		}
		children := root["children"].([]any)
		if len(children) != 2 {
			t.Fatalf("the root has %d children", len(children))
		}
		accounts := children[0].(map[string]any)
		if accounts["type"] != "WORK_PACKAGE" || len(accounts["children"].([]any)) != 2 {
			t.Errorf("accounts = %v", accounts)
		}
		chat := accounts["children"].([]any)[1].(map[string]any)
		if chat["notes"] != "the team channel too" {
			t.Errorf("chat = %v", chat)
		}
		// An offset CreateTemplate would refuse - a year - is left off the node, which stands.
		introduce := children[1].(map[string]any)
		if _, kept := introduce["due_offset"]; kept || introduce["title"] != "Introduce to the team" {
			t.Errorf("introduce = %v", introduce)
		}
	}
	// The words are done with once the job is over.
	if len(requests.held) != 0 || len(requests.deleted) != 1 {
		t.Errorf("the request was not discarded: %+v", requests)
	}
}

// A node of a type the profile refuses under its parent is dropped with everything under it, the
// rest of the draft stands, and the count is the job's result.
func TestANodeTheProfileRefusesIsDroppedAndCounted(t *testing.T) {
	produce, world, _ := templateProducer(`{"name":"Release","nodes":[
		{"type":"TASK","title":"Release {version}","children":[
			{"type":"TASK","title":"A task under a task","children":[{"type":"ACTIVITY","title":"lost with it"}]},
			{"type":"WORK_PACKAGE","title":"Build","children":[
				{"type":"WORK_PACKAGE","title":"A package under a package"},
				{"type":"ACTIVITY","title":"Tag"}]}]}]}`)

	outcome, err := produce.Ask(context.Background(), person(), templateRequest())
	if err != nil {
		t.Fatalf("producing: %v", err)
	}
	if !outcome.Recorded || outcome.Dropped != 3 {
		t.Errorf("outcome = %+v, want three nodes dropped", outcome)
	}
	for _, recorded := range world.store.proposals {
		// The count is on the row as well as in the outcome (issue 767): the suggestion is what
		// a client renders, and the job's result reaches none.
		if recorded.DroppedNodes != 3 {
			t.Errorf("the suggestion records %d dropped nodes, want 3", recorded.DroppedNodes)
		}
		root := recorded.Payload["nodes"].([]any)[0].(map[string]any)
		children := root["children"].([]any)
		if len(children) != 1 || children[0].(map[string]any)["title"] != "Build" {
			t.Errorf("the root's children are %v", children)
		}
		build := children[0].(map[string]any)["children"].([]any)
		if len(build) != 1 || build[0].(map[string]any)["title"] != "Tag" {
			t.Errorf("build's children are %v", build)
		}
	}
}

// What cannot be a template is not recorded: no name, no root, a root the profile refuses in a
// collection, a node without a title, prose.
func TestADraftThatCouldNotBeDefinedIsNotRecorded(t *testing.T) {
	for _, testCase := range []struct{ name, answer string }{
		{"no name", `{"nodes":[{"type":"TASK","title":"A"}]}`},
		{"no nodes", `{"name":"A"}`},
		{"two roots", `{"name":"A","nodes":[{"type":"TASK","title":"A"},{"type":"TASK","title":"B"}]}`},
		{"a root that only sits under another", `{"name":"A","nodes":[{"type":"ACTIVITY","title":"A"}]}`},
		{"a node without a title", `{"name":"A","nodes":[{"type":"TASK","title":"A","children":[{"type":"ACTIVITY","title":" "}]}]}`},
		{"prose", `Here is a template you could use.`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			produce, world, requests := templateProducer(testCase.answer)
			outcome, err := produce.Ask(context.Background(), person(), templateRequest())
			if err != nil {
				t.Fatalf("producing: %v", err)
			}
			if outcome.Recorded || len(world.store.proposals) != 0 {
				t.Error("a draft that could not be defined was recorded")
			}
			if len(requests.held) != 0 {
				t.Error("the words outlived the job")
			}
		})
	}
}

// No answer: the workspace switched AI off between the asking and the running. The job is over,
// so the words are done with, and the caller reads the port's one refusal.
func TestNoAnswerIsUnavailableAndTheWordsAreDoneWith(t *testing.T) {
	produce, world, requests := templateProducer(wellFormed)
	world.completion = false

	_, err := produce.Ask(context.Background(), person(), templateRequest())
	if !IsUnavailable(err) {
		t.Fatalf("the answer was %v, want ai.unavailable", err)
	}
	if len(world.store.proposals) != 0 {
		t.Error("something was recorded without an answer")
	}
	if len(requests.held) != 0 {
		t.Error("the words outlived the job")
	}
}

// A job naming words nobody holds any more fails by name rather than asking with nothing.
func TestAJobWhoseWordsAreGoneFailsByName(t *testing.T) {
	produce, _, requests := templateProducer(wellFormed)
	delete(requests.held, shared.MustParseID(requestID))

	_, err := produce.Ask(context.Background(), person(), templateRequest())
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the answer was %v", err)
	}
}

// Accepting is CreateTemplate as the accepting person, with the scope written from the target.
func TestAcceptingATemplateDefinesItThroughCreateTemplate(t *testing.T) {
	cases, world := newWorld()
	stored := proposal()
	stored.TargetType, stored.TargetID, stored.Kind = domain.TargetContainer, containerTargetID, domain.KindTemplate
	stored.InputDigest = domain.Digest(world.containerName, "")
	stored.Payload = map[string]any{
		"scope_type": "COLLECTION", "scope_id": "0192f000-0000-7000-8000-0000000000ee",
		"name": "Onboarding", "root_type": "TASK",
		"nodes": []any{map[string]any{"type": "TASK", "title": "Onboard"}},
	}
	world.store.proposals[proposalID] = stored

	if _, err := (AcceptSuggestion{Cases: cases}).Execute(context.Background(), person(), proposalID, nil); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	var defined usecase.Input
	for _, call := range world.performed {
		if call.name == createTemplateName {
			defined = call.in
			if call.actor.AccountID != accountID {
				t.Errorf("defined as %v, want the accepting person", call.actor.AccountID)
			}
		}
	}
	if defined == nil {
		t.Fatalf("CreateTemplate was not performed: %+v", world.performed)
	}
	// The scope is the suggestion's target, whatever the payload said.
	if defined["scope_id"] != containerTargetID.String() || defined["scope_type"] != "COLLECTION" || defined["name"] != "Onboarding" {
		t.Errorf("defined %v", defined)
	}
	if _, leaked := defined["container_id"]; leaked {
		t.Error("the target travelled under a key CreateTemplate does not declare")
	}
	if got := world.store.proposals[proposalID].Status; got != domain.StatusAccepted {
		t.Errorf("the proposal is %s", got)
	}
}
