// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The claim this whole package exists to make: accepting is an ordinary write. What the test can
// see is that the acceptance names the use case a person would name, hands it the accepting person
// as the actor, and adds nothing of its own.
func TestAcceptingPerformsTheOrdinaryUseCaseAsTheAcceptingPerson(t *testing.T) {
	cases, world := newWorld()
	world.store.proposals[proposalID] = proposal()

	accepted, err := AcceptSuggestion{Cases: cases}.
		Execute(context.Background(), person(), proposalID, nil)
	if err != nil {
		t.Fatalf("accepting: %v", err)
	}

	// Two calls and no more: the read that fingerprints the target, and the write that owns the
	// change. Nothing this package does to a workspace happens any other way.
	if len(world.performed) != 2 {
		t.Fatalf("%d use cases performed, want the read and the write", len(world.performed))
	}
	if world.performed[0].name != "GetWorkItem" {
		t.Errorf("the target was read through %q", world.performed[0].name)
	}
	call := world.performed[1]
	if call.name != "UpdateWorkItem" {
		t.Errorf("the acceptance performed %q", call.name)
	}
	if call.actor.AccountID != person().AccountID {
		t.Error("the acceptance ran as somebody other than the accepting person")
	}
	if call.in["title"] != "Buy oat milk" {
		t.Errorf("the payload did not reach the use case: %v", call.in)
	}
	if accepted.Status != domain.StatusAccepted || accepted.DecidedBy != person().AccountID {
		t.Errorf("the record came back as %+v", accepted)
	}
}

// The one field a payload must never decide. A proposal about one entry able to name another would
// be a stored capability, and `item_id` is a field UpdateWorkItem declares - the registry would
// refuse nothing about it.
func TestThePayloadCannotRedirectTheChangeToAnotherEntry(t *testing.T) {
	cases, world := newWorld()
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000ff")
	stored := proposal()
	stored.Payload = map[string]any{"title": "Buy oat milk", "item_id": elsewhere.String()}
	world.store.proposals[proposalID] = stored

	if _, err := (AcceptSuggestion{Cases: cases}).
		Execute(context.Background(), person(), proposalID, nil); err != nil {
		t.Fatalf("accepting: %v", err)
	}

	if got := world.performed[1].in["item_id"]; got != targetID.String() {
		t.Errorf("the change was aimed at %v, want the suggestion's own target", got)
	}
}

// A proposal about an entry that has been rewritten no longer fits it. Refused rather than applied,
// because applying it would produce a task nobody recognises - and refused for a dismissal too, so
// that somebody who meant to accept is told why.
func TestAStaleSuggestionIsRefusedForBothAnswers(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		answer func(Cases) error
	}{
		{"accepting", func(c Cases) error {
			_, err := AcceptSuggestion{Cases: c}.
				Execute(context.Background(), person(), proposalID, nil)
			return err
		}},
		{"dismissing", func(c Cases) error {
			_, err := DismissSuggestion{Cases: c}.Execute(context.Background(), person(), proposalID)
			return err
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cases, world := newWorld()
			world.store.proposals[proposalID] = proposal()
			world.title = "somebody rewrote this"

			err := testCase.answer(cases)
			if !errors.Is(err, shared.ErrConflict) {
				t.Fatalf("the answer was %v, want a conflict", err)
			}
			if got := shared.AsError(err).DetailCode; got != "suggestions.stale" {
				t.Errorf("detail code %q, want suggestions.stale", got)
			}
			if len(world.performed) != 1 {
				// Only the read that computed the digest.
				t.Errorf("a stale suggestion performed %d calls", len(world.performed))
			}
			if world.store.proposals[proposalID].Status != domain.StatusProposed {
				t.Error("a stale suggestion was decided anyway")
			}
		})
	}
}

// The change happens or the record does; never one without the other.
func TestARefusedChangeLeavesTheSuggestionStanding(t *testing.T) {
	cases, world := newWorld()
	world.store.proposals[proposalID] = proposal()
	world.performFails = shared.ErrForbidden

	if _, err := (AcceptSuggestion{Cases: cases}).
		Execute(context.Background(), person(), proposalID, nil); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the acceptance answered %v, want the use case's own refusal", err)
	}
	if world.store.proposals[proposalID].Status != domain.StatusProposed {
		t.Error("a suggestion whose change was refused is marked accepted")
	}
	if len(world.entries) != 0 {
		t.Error("a refused acceptance was audited as a success")
	}
}

// Two people answering one proposal: the first wins and the second is told.
func TestTheSecondAnswerToOneProposalIsRefused(t *testing.T) {
	cases, world := newWorld()
	world.store.proposals[proposalID] = proposal()
	world.store.versionMoved = true

	_, err := AcceptSuggestion{Cases: cases}.
		Execute(context.Background(), person(), proposalID, nil)
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("the answer was %v, want a conflict", err)
	}
	if got := shared.AsError(err).DetailCode; got != "suggestions.already_decided" {
		t.Errorf("detail code %q", got)
	}
}

// Dismissing changes nothing but the record. It is the whole difference between the two answers.
func TestDismissingPerformsNoChange(t *testing.T) {
	cases, world := newWorld()
	world.store.proposals[proposalID] = proposal()

	dismissed, err := DismissSuggestion{Cases: cases}.
		Execute(context.Background(), person(), proposalID)
	if err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	if dismissed.Status != domain.StatusDismissed {
		t.Errorf("status %q", dismissed.Status)
	}
	// Only the digest read; nothing was performed.
	for _, call := range world.performed {
		if call.name != "GetWorkItem" {
			t.Errorf("dismissing performed %q", call.name)
		}
	}
}

// A combination this build does not serve is told it is not built, not that the suggestion is
// invalid - the distinction the deferred automation kinds draw.
func TestAnAcceptanceThisBuildDoesNotServeSaysSo(t *testing.T) {
	cases, world := newWorld()
	stored := proposal()
	// Breaking a jumble entry down is a combination nothing produces and nothing applies: an entry
	// becomes one item, and what sits under it is proposed afterwards.
	stored.TargetType, stored.Kind = domain.TargetJumbleEntry, domain.KindDecomposition
	world.store.proposals[proposalID] = stored

	_, err := AcceptSuggestion{Cases: cases}.
		Execute(context.Background(), person(), proposalID, nil)
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("the answer was %v", err)
	}
	if got := shared.AsError(err).DetailCode; got != "suggestions.acceptance_not_built" {
		t.Errorf("detail code %q", got)
	}
}

// The audit entry carries the provenance - "who decided this" is half an answer without "and on
// whose suggestion" - and never the payload, which is somebody's content.
func TestTheDecisionIsAuditedWithProvenanceAndWithoutContent(t *testing.T) {
	cases, world := newWorld()
	world.store.proposals[proposalID] = proposal()

	if _, err := (AcceptSuggestion{Cases: cases}).
		Execute(context.Background(), person(), proposalID, nil); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	if len(world.entries) != 1 {
		t.Fatalf("%d audit entries", len(world.entries))
	}

	entry := world.entries[0]
	if entry.Action != SuggestionAcceptedAction {
		t.Errorf("action %q", entry.Action)
	}
	recorded := map[string]string{}
	for field, masked := range entry.Changes {
		shape, held := masked.(map[string]any)
		if !held {
			t.Fatalf("%s is recorded as %T", field, masked)
		}
		recorded[field], _ = shape["to"].(string)
	}
	for field, want := range map[string]string{
		"status": string(domain.StatusAccepted), "model": "a-model", "prompt_version": "v1",
	} {
		if recorded[field] != want {
			t.Errorf("the entry records %s = %q, want %q", field, recorded[field], want)
		}
	}
	for field, value := range recorded {
		if value == "Buy oat milk" {
			t.Errorf("the entry carries the proposed content in %s", field)
		}
	}
}

// Reading a suggestion reads its target first, which is what makes a proposal exactly as readable
// as the entry it is about.
func TestListingReadsTheTargetFirst(t *testing.T) {
	cases, world := newWorld()
	world.readFails = shared.ErrNotFound.WithDetail("items.not_found")

	_, err := ListSuggestions{Cases: cases}.Execute(context.Background(), person(), ListQuery{
		TargetType: domain.TargetWorkItem, TargetID: targetID,
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("listing against an unreadable entry answered %v", err)
	}
	if world.store.listed {
		t.Error("the suggestions were read although the entry could not be")
	}
}

func TestListingRefusesAValueOutsideAClosedSet(t *testing.T) {
	cases, _ := newWorld()

	for _, testCase := range []struct {
		name  string
		query ListQuery
		code  string
	}{
		{"a target type nobody defined", ListQuery{TargetType: "COMMENT", TargetID: targetID},
			"suggestions.target_type_unknown"},
		{"a status nobody defined",
			ListQuery{TargetType: domain.TargetWorkItem, TargetID: targetID, Status: "PENDING"},
			"suggestions.status_unknown"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ListSuggestions{Cases: cases}.
				Execute(context.Background(), person(), testCase.query)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("the refusal was %v", err)
			}
			if got := shared.AsError(err).DetailCode; got != testCase.code {
				t.Errorf("detail code %q, want %q", got, testCase.code)
			}
		})
	}
}

// Nothing reaches the store or the catalogue unless the actor may act (rule 2).
func TestAnUnauthorisedActorReachesNothing(t *testing.T) {
	cases, world := newWorld()
	cases.Authorizer = refuse{}
	world.store.proposals[proposalID] = proposal()

	if _, err := (AcceptSuggestion{Cases: cases}).
		Execute(context.Background(), person(), proposalID, nil); err == nil {
		t.Fatal("an unauthorised actor accepted a suggestion")
	}
	if len(world.performed) != 0 || len(world.entries) != 0 {
		t.Error("a refused caller reached the catalogue or the trail")
	}
}

// The fixtures.

var (
	proposalID = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	targetID   = shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	tenantID   = shared.MustParseID("0192f000-0000-7000-8000-0000000000f3")
	accountID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000f4")
	now        = time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
)

func person() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
	}
}

func proposal() domain.Suggestion {
	recorded, err := domain.New(domain.NewInput{
		ID: proposalID, TenantID: tenantID,
		TargetType: domain.TargetWorkItem, TargetID: targetID,
		Kind:    domain.KindFields,
		Payload: map[string]any{"title": "Buy oat milk"},
		Provenance: domain.Provenance{
			Model: "a-model", PromptID: "suggest-fields", PromptVersion: "v1",
			ProducedAt: now.Add(-time.Minute),
		},
		InputDigest: domain.Digest("Buy milk", ""),
		Now:         now,
	})
	if err != nil {
		panic(err)
	}
	return recorded
}

// world is the catalogue, the store, the trail and the target's current state in one, so that a
// test can assert "nothing happened" from a single place.
type world struct {
	store        *suggestionStore
	performed    []performed
	entries      []audit.Entry
	title        string
	performFails error
	readFails    error
	// creates counts CreateWorkItem calls; failCreateAfter refuses from the nth onwards, and -1
	// never refuses.
	creates         int
	failCreateAfter int
}

type performed struct {
	name  string
	actor appshared.ActorContext
	in    usecase.Input
}

func newWorld() (Cases, *world) {
	w := &world{
		store: &suggestionStore{proposals: map[shared.ID]domain.Suggestion{}},
		title: "Buy milk", failCreateAfter: -1,
	}
	return Cases{
		Suggestions: w.store,
		Targets:     EntryTargets{Catalogue: w},
		Authorizer:  allow{}, Catalogue: w, Audit: w,
		UnitOfWork: direct{}, Clock: fixed{},
	}, w
}

func (w *world) Invoke(
	_ context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	w.performed = append(w.performed, performed{name: name, actor: actor, in: in})
	switch name {
	case "GetWorkItem":
		if w.readFails != nil {
			return nil, w.readFails
		}
		return usecase.Output{"title": w.title, "notes": ""}, nil
	case "ListJumbleEntries":
		if w.readFails != nil {
			return nil, w.readFails
		}
		// The same two words the work item answers, so a proposal about an entry is fresh against
		// it and a test about the *applier* is not stopped by the fingerprint.
		return usecase.Output{"items": []usecase.Output{{
			"id": targetID.String(), "raw_subject": w.title, "raw_body": "",
		}}}, nil
	}
	if name == "CreateWorkItem" {
		w.creates++
		// failCreateAfter refuses from the nth create onwards. Zero refuses the first, which is
		// the "nothing was created at all" case; a test that wants none to fail leaves it at -1.
		if w.failCreateAfter >= 0 && w.creates > w.failCreateAfter {
			return nil, shared.ErrForbidden
		}
		return usecase.Output{"id": fmt.Sprintf(
			"0192f000-0000-7000-8000-0000000001%02x", w.creates)}, nil
	}
	if w.performFails != nil {
		return nil, w.performFails
	}
	return usecase.Output{}, nil
}

func (w *world) Append(_ context.Context, entry audit.Entry) error {
	w.entries = append(w.entries, entry)
	return nil
}

type suggestionStore struct {
	proposals    map[shared.ID]domain.Suggestion
	listed       bool
	versionMoved bool
}

func (s *suggestionStore) Record(_ context.Context, proposal domain.Suggestion) error {
	s.proposals[proposal.ID] = proposal
	return nil
}

func (s *suggestionStore) Find(_ context.Context, id shared.ID) (domain.Suggestion, error) {
	held, found := s.proposals[id]
	if !found {
		return domain.Suggestion{}, shared.ErrNotFound.WithDetail("suggestions.not_found")
	}
	return held, nil
}

func (s *suggestionStore) List(context.Context, repository.Query) (repository.Page, error) {
	s.listed = true
	proposals := make([]domain.Suggestion, 0, len(s.proposals))
	for _, held := range s.proposals {
		proposals = append(proposals, held)
	}
	return repository.Page{Items: proposals}, nil
}

func (s *suggestionStore) Decide(
	_ context.Context, decided domain.Suggestion, _ int,
) (bool, error) {
	if s.versionMoved {
		return false, nil
	}
	s.proposals[decided.ID] = decided
	return true, nil
}

type allow struct{}

func (allow) Authorize(context.Context, appshared.ActorContext, access.Request) error { return nil }

type refuse struct{}

func (refuse) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return shared.ErrForbidden
}

type direct struct{}

func (direct) Within(ctx context.Context, _ persistence.Scope, run func(context.Context) error) error {
	return run(ctx)
}

func (direct) WithinReadOnly(ctx context.Context, _ persistence.Scope, run func(context.Context) error) error {
	return run(ctx)
}

type fixed struct{}

func (fixed) Now() time.Time { return now }

// The descriptors are how REST, MCP and automation all reach these three, so the mapping from an
// input map to the typed call is exercised here rather than only through an adapter.

func TestTheDescriptorsCarryWhatTheRegistryNeeds(t *testing.T) {
	cases, _ := newWorld()

	for _, descriptor := range []usecase.Descriptor{
		ListSuggestions{Cases: cases}.Descriptor(),
		AcceptSuggestion{Cases: cases}.Descriptor(),
		DismissSuggestion{Cases: cases}.Descriptor(),
	} {
		t.Run(descriptor.Name, func(t *testing.T) {
			if descriptor.Summary == "" || descriptor.SideEffects == "" {
				t.Error("an agent is told nothing about what this does")
			}
			if descriptor.TokenScope == "" || descriptor.Handler == nil {
				t.Error("the descriptor is not usable by the registry")
			}
			if !descriptor.ReadOnly && !descriptor.Audit.Required {
				t.Error("a writing use case declares no audit obligation (gate SG-13)")
			}
		})
	}
}

// The enums a descriptor declares come from the domain's own closed sets. A declared value the
// domain refuses is a written refusal nobody can reach - the trap a contract enum and a descriptor
// enum fall into when they are two lists.
func TestTheDeclaredEnumsAreTheDomainsOwn(t *testing.T) {
	cases, _ := newWorld()
	fields := map[string][]string{}
	for _, field := range (ListSuggestions{Cases: cases}).Descriptor().Input {
		fields[field.Name] = field.Enum
	}

	for _, declared := range fields["target_type"] {
		if !domain.TargetType(declared).Valid() {
			t.Errorf("the descriptor declares the target type %q and the domain refuses it", declared)
		}
	}
	for _, declared := range fields["status"] {
		if !domain.Status(declared).Valid() {
			t.Errorf("the descriptor declares the status %q and the domain refuses it", declared)
		}
	}
	if len(fields["target_type"]) != len(domain.TargetTypes()) ||
		len(fields["status"]) != len(domain.Statuses()) {
		t.Error("a closed set grew and the descriptor did not")
	}
}

func TestListingThroughTheRegistryAnswersAPage(t *testing.T) {
	cases, world := newWorld()
	world.store.proposals[proposalID] = proposal()

	out, err := ListSuggestions{Cases: cases}.invoke(context.Background(), person(), usecase.Input{
		"target_type": string(domain.TargetWorkItem),
		"target_id":   targetID.String(),
	})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}

	items, held := out["items"].([]usecase.Output)
	if !held || len(items) != 1 {
		t.Fatalf("the page is %v", out["items"])
	}
	// The provenance is what a suggestion is for, so it is in the shape every channel renders.
	for _, field := range []string{"source", "model", "prompt_id", "prompt_version", "produced_at"} {
		if _, carried := items[0][field]; !carried {
			t.Errorf("the read shape carries no %s", field)
		}
	}
	if items[0]["status"] != string(domain.StatusProposed) {
		t.Errorf("status %v", items[0]["status"])
	}
}

func TestAnsweringThroughTheRegistryCarriesTheDecisionBack(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		invoke func(Cases) (usecase.Output, error)
		want   domain.Status
	}{
		{"accept", func(c Cases) (usecase.Output, error) {
			return AcceptSuggestion{Cases: c}.invoke(context.Background(), person(),
				usecase.Input{"suggestion_id": proposalID.String()})
		}, domain.StatusAccepted},
		{"dismiss", func(c Cases) (usecase.Output, error) {
			return DismissSuggestion{Cases: c}.invoke(context.Background(), person(),
				usecase.Input{"suggestion_id": proposalID.String()})
		}, domain.StatusDismissed},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cases, world := newWorld()
			world.store.proposals[proposalID] = proposal()

			out, err := testCase.invoke(cases)
			if err != nil {
				t.Fatalf("answering: %v", err)
			}
			if out["status"] != string(testCase.want) {
				t.Errorf("status %v, want %q", out["status"], testCase.want)
			}
			if out["decided_by"] != accountID.String() {
				t.Errorf("decided_by %v", out["decided_by"])
			}
			if _, carried := out["decided_at"]; !carried {
				t.Error("the answer carries no moment")
			}
		})
	}
}

// An identifier that is not one is the caller's mistake, and every channel gets the same answer.
func TestAMalformedIdentifierIsRefusedBeforeAnythingIsRead(t *testing.T) {
	cases, world := newWorld()

	if _, err := (AcceptSuggestion{Cases: cases}).invoke(context.Background(), person(),
		usecase.Input{"suggestion_id": "not-an-identifier"}); err == nil {
		t.Fatal("a malformed identifier was accepted")
	}
	if len(world.performed) != 0 {
		t.Error("a malformed identifier reached the catalogue")
	}
}

// An entry the inbox does not list - settled, gone, or in a workspace the actor cannot see - is one
// answer and not three, which is what the inbox itself would say.
func TestAnEntryTheInboxDoesNotListIsNotFound(t *testing.T) {
	cases, _ := newWorld()

	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000fe")
	_, err := EntryTargets{Catalogue: cases.Catalogue}.
		Digest(context.Background(), person(), domain.TargetJumbleEntry, elsewhere)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the answer was %v, want a not-found", err)
	}
}

// A target kind this build has no reader for answers not-found for the same reason: from a
// caller's side there is no such suggestion, which is exactly true.
func TestATargetKindWithNoReaderIsNotFound(t *testing.T) {
	cases, _ := newWorld()

	_, err := EntryTargets{Catalogue: cases.Catalogue}.
		Digest(context.Background(), person(), domain.TargetType("COMMENT"), targetID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the answer was %v, want a not-found", err)
	}
}

// Accepting a breakdown is a walk: one ordinary create per node, depth first, in the order a
// person read them - a breakdown whose pieces arrived shuffled is not the one they saw.
func TestAcceptingABreakdownCreatesEachPieceInOrder(t *testing.T) {
	cases, world := newWorld()
	world.store.proposals[proposalID] = breakdown()

	if _, err := (AcceptSuggestion{Cases: cases}).
		Execute(context.Background(), person(), proposalID, nil); err != nil {
		t.Fatalf("accepting: %v", err)
	}

	var created []string
	for _, call := range world.performed {
		if call.name != "CreateWorkItem" {
			continue
		}
		created = append(created, call.in["title"].(string))
	}
	want := []string{"Draft", "Outline", "Send"}
	if len(created) != len(want) {
		t.Fatalf("created %v, want %v", created, want)
	}
	for i := range want {
		if created[i] != want[i] {
			t.Errorf("piece %d is %q, want %q", i, created[i], want[i])
		}
	}
}

// The parent is the walk's, never the node's. A node naming its own parent would be a proposal
// about one entry able to grow children under another.
func TestABreakdownCannotChooseItsOwnParent(t *testing.T) {
	cases, world := newWorld()
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000fd")
	stored := breakdown()
	children := stored.Payload["children"].([]any)
	children[0].(map[string]any)["parent_id"] = elsewhere.String()
	world.store.proposals[proposalID] = stored

	if _, err := (AcceptSuggestion{Cases: cases}).
		Execute(context.Background(), person(), proposalID, nil); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	for _, call := range world.performed {
		if call.name == "CreateWorkItem" && call.in["parent_id"] == elsewhere.String() {
			t.Error("a node grew a child under an entry the suggestion is not about")
		}
	}
}

// A refusal partway through leaves what was created standing. Each piece is its own create with
// its own permission check, and losing a whole breakdown to one title that was too long is not
// what somebody who accepted it meant.
func TestARefusalPartwayThroughLeavesWhatWasCreated(t *testing.T) {
	cases, world := newWorld()
	world.store.proposals[proposalID] = breakdown()
	world.failCreateAfter = 2

	accepted, err := AcceptSuggestion{Cases: cases}.
		Execute(context.Background(), person(), proposalID, nil)
	if err != nil {
		t.Fatalf("a partial acceptance answered %v, want the two pieces that stood", err)
	}
	if accepted.Status != domain.StatusAccepted {
		t.Errorf("status %q", accepted.Status)
	}
	if world.creates != 3 {
		t.Errorf("%d creates attempted, want the two that worked and the one that did not",
			world.creates)
	}
}

// Nothing created at all is a different answer: the acceptance did nothing, so the refusal is what
// the caller gets and the proposal is still standing.
func TestABreakdownRefusedAtItsFirstPieceIsRefusedWhole(t *testing.T) {
	cases, world := newWorld()
	world.store.proposals[proposalID] = breakdown()
	world.failCreateAfter = 0

	if _, err := (AcceptSuggestion{Cases: cases}).
		Execute(context.Background(), person(), proposalID, nil); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the answer was %v, want the create's own refusal", err)
	}
	if world.store.proposals[proposalID].Status != domain.StatusProposed {
		t.Error("a breakdown that created nothing was marked accepted")
	}
}

// What a person changes before accepting a breakdown applies to every piece: the collection it
// lands in is a property of the breakdown, not of one node of it.
func TestOverridesApplyToEveryPieceOfABreakdown(t *testing.T) {
	cases, world := newWorld()
	world.store.proposals[proposalID] = breakdown()
	collection := shared.MustParseID("0192f000-0000-7000-8000-0000000000fc").String()

	if _, err := (AcceptSuggestion{Cases: cases}).Execute(context.Background(), person(),
		proposalID, map[string]any{"collection_id": collection}); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	for _, call := range world.performed {
		if call.name == "CreateWorkItem" && call.in["collection_id"] != collection {
			t.Errorf("a piece landed in %v", call.in["collection_id"])
		}
	}
}

func breakdown() domain.Suggestion {
	stored := proposal()
	stored.Kind = domain.KindDecomposition
	stored.Payload = map[string]any{"children": []any{
		map[string]any{"type": "WORK_PACKAGE", "title": "Draft", "children": []any{
			map[string]any{"type": "ACTIVITY", "title": "Outline"},
		}},
		map[string]any{"type": "ACTIVITY", "title": "Send"},
	}}
	return stored
}
