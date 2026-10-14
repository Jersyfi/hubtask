// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
)

// silentProvider cannot complete and says nothing about why - an absent provider.
type silentProvider struct{}

func (silentProvider) Capabilities() aiprovider.ProviderCapabilities {
	return aiprovider.ProviderCapabilities{Kind: "noop"}
}
func (silentProvider) Complete(context.Context, aiprovider.CompletionRequest) (aiprovider.CompletionResult, error) {
	return aiprovider.CompletionResult{}, aiprovider.ErrUnavailable
}
func (silentProvider) Embed(context.Context, []string) (aiprovider.EmbeddingResult, error) {
	return aiprovider.EmbeddingResult{}, aiprovider.ErrUnavailable
}

// spentProvider is a workspace whose budget is used up: it says so before anybody calls.
type spentProvider struct{ silentProvider }

func (spentProvider) Refusal() error {
	return aiprovider.BudgetSpent(time.Date(2026, 9, 9, 22, 0, 0, 0, time.UTC))
}

type oneProvider struct{ provider aiprovider.Provider }

func (o oneProvider) For(context.Context, appshared.ActorContext) (aiprovider.Provider, error) {
	return o.provider, nil
}

// UC-AI-07 check 3 at the asking: a spent budget refuses with its own parameters - the reason and
// when it renews - before anything is queued, still the one refusal; an absent provider refuses
// as before, with nothing to add.
func TestAskingWithASpentBudgetIsToldWhenItRenews(t *testing.T) {
	available, err := Availability{Providers: oneProvider{spentProvider{}}}.CanSuggest(t.Context(), appshared.ActorContext{})
	if available || !errors.Is(err, aiprovider.ErrUnavailable) {
		t.Fatalf("a spent budget answered %v, %v; want no and the one refusal", available, err)
	}
	answer := shared.AsError(err)
	if answer.DetailCode != "ai.unavailable" || answer.Params["reason"] != aiprovider.ReasonBudgetSpent ||
		answer.Params["renews_at"] != "2026-09-09T22:00:00Z" {
		t.Errorf("refusal %s %v, want the budget's reason and renewal", answer.DetailCode, answer.Params)
	}

	available, err = Availability{Providers: oneProvider{silentProvider{}}}.CanSuggest(t.Context(), appshared.ActorContext{})
	if available || err != nil {
		t.Errorf("an absent provider answered %v, %v; want no and no error", available, err)
	}
}
