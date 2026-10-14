// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// spentBudget answers a provider that may not be asked, the way the budget counter does once the
// day's tokens are gone.
type spentBudget struct{ renews time.Time }

func (s spentBudget) For(context.Context, appshared.ActorContext) (aiprovider.Provider, error) {
	return refusingProvider(s), nil
}

type refusingProvider struct{ renews time.Time }

func (refusingProvider) Capabilities() aiprovider.ProviderCapabilities {
	return aiprovider.ProviderCapabilities{Kind: "noop"}
}

func (p refusingProvider) Refusal() error { return aiprovider.BudgetSpent(p.renews) }

func (p refusingProvider) Complete(
	context.Context, aiprovider.CompletionRequest,
) (aiprovider.CompletionResult, error) {
	return aiprovider.CompletionResult{}, p.Refusal()
}

func (p refusingProvider) Embed(context.Context, []string) (aiprovider.EmbeddingResult, error) {
	return aiprovider.EmbeddingResult{}, p.Refusal()
}

type anyPrompt struct{}

func (anyPrompt) Get(id string) (aiprovider.Prompt, error) {
	return aiprovider.Prompt{ID: id, Version: "v1", Instruction: "describe what follows"}, nil
}

func (anyPrompt) IDs() []string { return nil }

// UC-AI-07 check 3 for a question already accepted: a budget spent between the 202 and the job is
// not a job that ends silently - it ends refused, with the code and the parameters the sentence
// needs, so the person who asked reads that the budget is spent and when it renews.
func TestABudgetSpentAfterTheAskingEndsTheJobRefused(t *testing.T) {
	renews := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	handler := AiSuggestion{Produce: suggestion.Produce{
		Providers: spentBudget{renews: renews}, Prompts: anyPrompt{},
	}}

	result, err := handler.Run(t.Context(), queue.Job{
		ID: jobID, TenantID: tenantID, Kind: queue.KindAiSuggest,
		Payload: map[string]any{
			"target_type": "WORK_ITEM", "target_id": "0192f000-0000-7000-8000-0000000000e1",
			"asked_by": "0192f000-0000-7000-8000-0000000000d1", "kind": "FIELDS",
			"prompt": "suggest-fields",
		},
	})
	if err != nil {
		t.Fatalf("the job failed rather than ending refused: %v", err)
	}
	if result.Refusal == nil {
		t.Fatal("the job ended without saying it was refused")
	}
	if result.Refusal.Code != "ai.unavailable" ||
		result.Refusal.Params["reason"] != aiprovider.ReasonBudgetSpent ||
		result.Refusal.Params["renews_at"] != renews.Format(time.RFC3339) {
		t.Errorf("the refusal is %+v", result.Refusal)
	}
}
