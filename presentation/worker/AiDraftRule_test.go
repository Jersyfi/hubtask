// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

const (
	drafterID   = "0192f000-0000-7000-8000-0000000000d7"
	draftedAtID = "0192f000-0000-7000-8000-0000000000c7"
)

// A rule's draft job reads its scope back from the references the asking wrote, and counts what it
// could not draft - a number, never the parts of the sentence.
func TestARuleDraftJobReadsItsScopeAndCountsWhatItCouldNotDraft(t *testing.T) {
	material := &scopeMaterial{}
	recorded := &recordedSuggestions{}
	handler := AiSuggestion{Produce: suggestion.Produce{
		Providers: draftingProvider{}, Prompts: anyPrompt{}, Sources: material,
		Suggestions: recorded, UnitOfWork: &unitOfWork{}, Clock: fixedNow{}, IDs: oneID{},
	}}

	result, err := handler.Run(t.Context(), queue.Job{
		ID: jobID, TenantID: tenantID, Kind: queue.KindAiDraftRule,
		Payload: map[string]any{
			"target_type": "ACCOUNT", "target_id": drafterID, "asked_by": drafterID,
			"kind": "RULE", "prompt": "draft-rule",
			"scope_type": "COLLECTION", "scope_id": draftedAtID,
		},
	})
	if err != nil {
		t.Fatalf("the job failed: %v", err)
	}
	if material.asked.Scope != (automation.Scope{Type: automation.ScopeCollection, ID: shared.ID(draftedAtID)}) {
		t.Errorf("the material was asked for %+v", material.asked.Scope)
	}
	if result.Outcome != suggestion.OutcomeProposed || result.Counts["parts_not_drafted"] != 1 {
		t.Errorf("the job ended with %+v", result)
	}
	if len(recorded.proposals) != 1 || recorded.proposals[0].Kind != domain.KindRule {
		t.Fatalf("recorded %+v", recorded.proposals)
	}
}

type scopeMaterial struct{ asked suggestion.Question }

func (m *scopeMaterial) Material(
	_ context.Context, _ appshared.ActorContext, question suggestion.Question,
) (suggestion.Material, error) {
	m.asked = question
	return suggestion.Material{
		Content: "What a rule here may be made of.", Digest: domain.Digest(drafterID, ""),
		Rule: &suggestion.RuleSets{
			Scope: question.Scope, Actions: []string{"COMPLETE_WORK_ITEM"},
			Words: "complete what arrives",
		},
	}, nil
}

// draftingProvider answers one step it may use and one it may not.
type draftingProvider struct{}

func (draftingProvider) For(context.Context, appshared.ActorContext) (aiprovider.Provider, error) {
	return draftingProvider{}, nil
}

func (draftingProvider) Capabilities() aiprovider.ProviderCapabilities {
	return aiprovider.ProviderCapabilities{Kind: "stub", Completion: true}
}

func (draftingProvider) Complete(context.Context, aiprovider.CompletionRequest) (aiprovider.CompletionResult, error) {
	return aiprovider.CompletionResult{
		Text: `{"trigger": {"kind": "JUMBLE_ENTRY"}, "actions": [{"kind": "COMPLETE_WORK_ITEM"}, ` +
			`{"kind": "HTTP_REQUEST", "params": {"url": "https://example.invalid"}}]}`,
		Model: "a-model", PromptID: "draft-rule", PromptVersion: "v1",
		ProducedAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
	}, nil
}

func (draftingProvider) Embed(context.Context, []string) (aiprovider.EmbeddingResult, error) {
	return aiprovider.EmbeddingResult{}, nil
}

type recordedSuggestions struct {
	repository.Suggestions
	proposals []domain.Suggestion
}

func (r *recordedSuggestions) Record(_ context.Context, proposal domain.Suggestion) error {
	r.proposals = append(r.proposals, proposal)
	return nil
}

type fixedNow struct{}

func (fixedNow) Now() time.Time { return time.Date(2030, 1, 1, 0, 1, 0, 0, time.UTC) }

type oneID struct{}

func (oneID) NewID() shared.ID { return shared.ID("0192f000-0000-7000-8000-0000000000e9") }
