// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	suggestionrepo "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// recordedArrival is an arrival with words in it, remembering what it was asked about.
type recordedArrival struct{ asked *suggestion.Request }

func (r recordedArrival) Arrival(_ context.Context, request suggestion.Request) (suggestion.Arrival, error) {
	*r.asked = request
	return suggestion.Arrival{Prepare: true, Material: suggestion.Material{Content: "Subject: Order 42", Digest: []byte{1}}}, nil
}

// systemOnly is a provider resolver that refuses anybody but the system acting for a workspace,
// and answers with its provenance.
type systemOnly struct{ text string }

func (s systemOnly) For(_ context.Context, actor appshared.ActorContext) (aiprovider.Provider, error) {
	if actor.Kind != appshared.ActorSystem || !actor.AccountID.IsZero() {
		return refusingProvider{}, nil
	}
	return provenancedProvider(s), nil
}

type provenancedProvider struct{ text string }

func (provenancedProvider) Capabilities() aiprovider.ProviderCapabilities {
	return aiprovider.ProviderCapabilities{Kind: "stub", Completion: true}
}

func (p provenancedProvider) Complete(context.Context, aiprovider.CompletionRequest) (aiprovider.CompletionResult, error) {
	return aiprovider.CompletionResult{
		Text: p.text, Model: "stub", PromptID: "suggest-fields", PromptVersion: "v4", ProducedAt: fixedClock{}.Now(),
	}, nil
}

func (provenancedProvider) Embed(context.Context, []string) (aiprovider.EmbeddingResult, error) {
	return aiprovider.EmbeddingResult{}, nil
}

// recordingSuggestions keeps what was recorded.
type recordingSuggestions struct{ recorded []domain.Suggestion }

func (r *recordingSuggestions) Record(_ context.Context, proposal domain.Suggestion) error {
	r.recorded = append(r.recorded, proposal)
	return nil
}
func (r *recordingSuggestions) Find(context.Context, shared.ID) (domain.Suggestion, error) {
	return domain.Suggestion{}, shared.ErrNotFound
}
func (r *recordingSuggestions) List(context.Context, suggestionrepo.Query) (suggestionrepo.Page, error) {
	return suggestionrepo.Page{}, nil
}
func (r *recordingSuggestions) Decide(context.Context, domain.Suggestion, int) (bool, error) {
	return false, nil
}

func (r *recordingSuggestions) Narrow(context.Context, domain.Suggestion, int) (bool, error) {
	return false, nil
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC) }

func arrivalJob() queue.Job {
	return queue.Job{
		ID: jobID, TenantID: tenantID, Kind: queue.KindAiSuggestArrival,
		Payload: map[string]any{
			"target_id":    "0192f000-0000-7000-8000-0000000000e1",
			"submitted_by": "0192f000-0000-7000-8000-0000000000d1",
		},
	}
}

// The arrival's job is asked as the system for the workspace, about the entry it names, with who
// made the arrival carried through to the run that judges whose content stays out.
func TestAnArrivalsJobAsksAsTheWorkspace(t *testing.T) {
	asked := &suggestion.Request{}
	handler := AiSuggestionArrival{Produce: suggestion.Produce{
		Providers: systemOnly{text: `{"title":"Ship order 42"}`}, Prompts: anyPrompt{},
		Arrivals: recordedArrival{asked: asked}, Suggestions: &recordingSuggestions{},
		UnitOfWork: &unitOfWork{}, Clock: fixedClock{}, IDs: &fixedIDs{},
	}}

	result, err := handler.Run(t.Context(), arrivalJob())
	if err != nil {
		t.Fatalf("the job failed: %v", err)
	}
	if result.Outcome != suggestion.OutcomeProposed {
		t.Errorf("the job ended with %+v", result)
	}
	if !asked.OnArrival || asked.TargetID != shared.ID("0192f000-0000-7000-8000-0000000000e1") ||
		asked.SubmittedBy != shared.ID("0192f000-0000-7000-8000-0000000000d1") || asked.TargetType != domain.TargetJumbleEntry {
		t.Errorf("the run was asked %+v", *asked)
	}
}

// UC-AI-07 check 3 on arrival: a budget spent since ends the job refused with the sentence's
// parameters, which the entry's card reads once.
func TestAnArrivalAfterTheBudgetIsSpentEndsRefused(t *testing.T) {
	renews := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	handler := AiSuggestionArrival{Produce: suggestion.Produce{
		Providers: spentBudget{renews: renews}, Prompts: anyPrompt{},
		Arrivals: recordedArrival{asked: &suggestion.Request{}}, UnitOfWork: &unitOfWork{},
	}}

	result, err := handler.Run(t.Context(), arrivalJob())
	if err != nil {
		t.Fatalf("the job failed rather than ending refused: %v", err)
	}
	if result.Refusal == nil || result.Refusal.Code != "ai.unavailable" ||
		result.Refusal.Params["reason"] != aiprovider.ReasonBudgetSpent {
		t.Errorf("the job ended with %+v", result)
	}
}
