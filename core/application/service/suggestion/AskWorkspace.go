// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AskWorkspaceName is the catalogue name.
const AskWorkspaceName = "AskWorkspace"

// QuestionAskedAction records that somebody asked the workspace a question: that content was sent
// to a provider and for what (ADR-0018 decision 7), never the words.
const QuestionAskedAction audit.Action = "ai.question_asked"

// accountTarget is what the asking's audit entry is about: the account that asked, which is what
// the answer will be about.
const accountTarget = "account"

// AskWorkspace asks the workspace's provider one question in the asker's own words (UC-AI-08).
//
// Anybody with a role in the workspace may ask, under the suggestions' token scope: the answer is
// made from the entries the ordinary reads answer the asker, so a question asks no permission of
// its own beyond being theirs. What comes back is an ANSWER suggestion about the asker's own
// account, which performs no action. Each question stands alone - nothing of an earlier one is
// carried into it (UC-AI-08 check 5).
type AskWorkspace struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// Execute checks, holds the words, then queues.
func (h AskWorkspace) Execute(
	ctx context.Context, actor appshared.ActorContext, question string,
) (shared.ID, error) {
	question = strings.TrimSpace(question)
	if question == "" || len([]rune(question)) > MaxTemplateDescription {
		return shared.ID(""), shared.ErrValidation.WithDetail("suggestions.question_invalid").
			WithFields(shared.FieldError{Path: "/question", Code: "suggestions.question_invalid"})
	}
	return Ask(h).queue(ctx, actor, asking{
		TargetType: domain.TargetAccount, TargetID: actor.AccountID,
		Action: QuestionAskedAction, Kind: domain.KindAnswer, PromptID: answerPrompt,
		Text:    question,
		JobKind: queue.KindAiAnswer, Attempts: queue.AiDraftRuleAttempts,
	})
}

// Descriptor is the catalogue entry.
func (h AskWorkspace) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AskWorkspaceName,
		Summary: "Asks the workspace's AI provider one question in the caller's own words. The " +
			"provider is shown only entries the caller may read, found through the ordinary search " +
			"and listings; content of people who keep theirs out of AI is never among them. The " +
			"answer is an ANSWER suggestion about the caller's own account, which only they read: " +
			"the answer text, at most five entries it rests on and the filter it implies, each " +
			"naming only what the provider was shown. It performs no action and keeps no history.",
		SideEffects: "Holds the question for the job that will read it, queues one question to the " +
			"provider and writes an audit entry. Changes nothing in the workspace.",
		TokenScope: suggestionsWrite,
		Input: []usecase.Field{
			{Name: "question", Kind: usecase.KindString, Required: true,
				Description: "The question, in the caller's own words. It travels to the provider as " +
					"content, never as instruction."},
		},
		Audit: usecase.AuditDeclaration{
			Action: QuestionAskedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Asking changes nothing in the workspace.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AskWorkspace) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	jobID, err := h.Execute(ctx, actor, in.String("question"))
	if err != nil {
		return nil, err
	}
	return queued(jobID), nil
}
