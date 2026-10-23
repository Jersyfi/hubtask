// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

const (
	shownEntryID   = "0192f000-0000-7000-8000-0000000000f1"
	unshownEntryID = "0192f000-0000-7000-8000-0000000000f2"
)

// An answer's job records the answer about its asker, narrowed to the entries it was shown, and
// counts what it dropped - a number, never the references themselves.
func TestAnAnswerJobRecordsTheAnswerNarrowedToWhatWasShown(t *testing.T) {
	recorded := &recordedSuggestions{}
	handler := AiSuggestion{Produce: suggestion.Produce{
		Providers: questionProvider{}, Prompts: anyPrompt{}, Sources: answerMaterial{},
		Suggestions: recorded, UnitOfWork: &unitOfWork{}, Clock: fixedNow{}, IDs: oneID{},
	}}

	result, err := handler.Run(t.Context(), queue.Job{
		ID: jobID, TenantID: tenantID, Kind: queue.KindAiAnswer,
		Payload: map[string]any{
			"target_type": "ACCOUNT", "target_id": drafterID, "asked_by": drafterID,
			"kind": "ANSWER", "prompt": "answer-question",
		},
	})
	if err != nil {
		t.Fatalf("the job failed: %v", err)
	}
	if result.Outcome != suggestion.OutcomeProposed || result.Counts["references_dropped"] != 1 {
		t.Errorf("the job ended with %+v", result)
	}
	if len(recorded.proposals) != 1 || recorded.proposals[0].Kind != domain.KindAnswer {
		t.Fatalf("recorded %+v", recorded.proposals)
	}
	sources, _ := recorded.proposals[0].Payload["source_ids"].([]any)
	if len(sources) != 1 || sources[0] != shownEntryID {
		t.Errorf("sources %v", sources)
	}
}

type answerMaterial struct{}

func (answerMaterial) Material(
	context.Context, appshared.ActorContext, suggestion.Question,
) (suggestion.Material, error) {
	return suggestion.Material{
		Content: "Entries (source_ids): ...", Digest: domain.Digest(drafterID, ""),
		Answer: &suggestion.AnswerSets{Entries: []string{shownEntryID}},
	}, nil
}

// questionProvider cites one entry it was shown and one it was not.
type questionProvider struct{ draftingProvider }

func (questionProvider) For(context.Context, appshared.ActorContext) (aiprovider.Provider, error) {
	return questionProvider{}, nil
}

func (questionProvider) Complete(context.Context, aiprovider.CompletionRequest) (aiprovider.CompletionResult, error) {
	return aiprovider.CompletionResult{
		Text:  `{"answer": "One offer is open.", "source_ids": ["` + shownEntryID + `", "` + unshownEntryID + `"]}`,
		Model: "a-model", PromptID: "answer-question", PromptVersion: "v1",
		ProducedAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
	}, nil
}
