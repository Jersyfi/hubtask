// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// AskQueryName is the catalogue name.
const AskQueryName = "AskQuery"

// QueryAskedAction is audited for every question sent to a provider - never the question (rule 10).
const QueryAskedAction audit.Action = "ai.query_asked"

const (
	writeQueryPrompt = "write-query"
	// MaxQuestionLength bounds a question in characters, as the contract does.
	MaxQuestionLength = 500
)

var writeQueryAnswerKeys = []string{"query"}

// QueryAnswerKeys is the allow list of the question's prompt, read beside the translation's.
func QueryAnswerKeys() map[string][]string {
	return map[string][]string{writeQueryPrompt: append([]string(nil), writeQueryAnswerKeys...)}
}

// AskQuery writes a question in words as a Hubtask query through the workspace's AI provider
// (Q-6): the answer is read by view.ParseQuery and answered as canonical text the person sees and
// changes. Nothing is run and nothing is stored.
type AskQuery struct {
	Providers  AiProviders
	Prompts    aiprovider.Prompts
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Timeout bounds the provider call: the person is waiting. Zero is the translation's.
	Timeout time.Duration
	// Kept refuses a person who keeps their own content out of AI. Optional.
	Kept KeptOutReader
}

// AskedQuery is the query and its provenance.
type AskedQuery struct {
	Query         string
	Model         string
	PromptID      string
	PromptVersion string
	ProducedAt    time.Time
}

// Execute asks the provider; every way of not getting a query is ai.unavailable.
func (h AskQuery) Execute(ctx context.Context, actor appshared.ActorContext, question string) (AskedQuery, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return AskedQuery{}, shared.ErrValidation.WithDetail("query.question_required").
			WithFields(shared.FieldError{Path: "/question", Code: "query.question_required"})
	}
	if utf8.RuneCountInString(question) > MaxQuestionLength {
		return AskedQuery{}, shared.ErrValidation.WithDetail("query.question_too_long").
			WithParams(map[string]string{"max": "500"}).
			WithFields(shared.FieldError{Path: "/question", Code: "query.question_too_long"})
	}
	if h.Providers == nil || h.Prompts == nil || h.UnitOfWork == nil {
		return AskedQuery{}, aiUnavailable
	}
	if h.Kept != nil {
		var refused error
		err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			kept, err := h.Kept.Read(ctx, actor.TenantID)
			refused = kept.Refusal(actor.AccountID)
			return err
		})
		if err != nil {
			return AskedQuery{}, err
		}
		if refused != nil {
			return AskedQuery{}, refused
		}
	}
	provider, err := h.Providers.For(ctx, actor)
	if err != nil {
		return AskedQuery{}, aiUnavailable
	}
	if !provider.Capabilities().Completion {
		return AskedQuery{}, aiprovider.RefusalOf(provider)
	}
	prompt, err := h.Prompts.Get(writeQueryPrompt)
	if err != nil {
		return AskedQuery{}, err
	}
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: h.Clock.Now(),
			Action: QueryAskedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: "tenant", TargetID: actor.TenantID,
		})
	})
	if err != nil {
		return AskedQuery{}, err
	}

	timeout := h.Timeout
	if timeout <= 0 {
		timeout = defaultTranslateTimeout
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	answer, err := provider.Complete(bounded, prompt.Ask(queryMaterial(view.QueryKeys(), question)))
	if err != nil {
		return AskedQuery{}, aiUnavailable
	}
	text, ok := queryFrom(answer.Text)
	if !ok {
		return AskedQuery{}, aiUnavailable
	}
	return AskedQuery{
		Query: text, Model: answer.Model, PromptID: answer.PromptID,
		PromptVersion: answer.PromptVersion, ProducedAt: answer.ProducedAt,
	}, nil
}

// queryMaterial is the catalogue the parser reads and the question, so the model writes only keys
// this installation answers.
func queryMaterial(keys []view.QueryKey, question string) string {
	var material strings.Builder
	material.WriteString("Keys:\n")
	for _, key := range keys {
		material.WriteString("- " + key.Key + " (" + key.Value + ")")
		if len(key.Values) > 0 {
			material.WriteString(" values: " + strings.Join(key.Values, ", "))
		}
		if len(key.Examples) > 0 {
			material.WriteString(" e.g. " + strings.Join(key.Examples, " | "))
		}
		material.WriteString("\n")
	}
	material.WriteString("\nQuestion:\n" + question + "\n")
	return material.String()
}

// queryFrom reads the answer's query through the parser and writes it back canonical; an answer
// that is not a query the parser reads is no answer.
func queryFrom(text string) (string, bool) {
	var answer struct {
		Query *string `json:"query"`
	}
	if err := json.Unmarshal([]byte(objectIn(text)), &answer); err != nil || answer.Query == nil {
		return "", false
	}
	parsed, err := view.ParseQuery(strings.TrimSpace(*answer.Query), "/query")
	if err != nil {
		return "", false
	}
	canonical, ok := view.FormatQuery(parsed)
	if !ok {
		return "", false
	}
	return canonical, true
}

// Descriptor is the catalogue entry.
func (h AskQuery) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       AskQueryName,
		Permission: service.PermissionRead,
		Summary: "Writes a question in words as a Hubtask query through the workspace's AI " +
			"provider, for the person to see, change and run. Nothing is run or stored.",
		SideEffects: "Sends the question and the query's keys to the provider and writes an " +
			"audit entry that a query was asked for, never the question.",
		TokenScope: itemsRead,
		ReadOnly:   true,
		Input: []usecase.Field{
			{
				Name: "question", Kind: usecase.KindString, Required: true,
				Description: "What the person wants to find, in words; at most 500 characters.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: QueryAskedAction, TargetType: "tenant",
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AskQuery) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	asked, err := h.Execute(ctx, actor, in.String("question"))
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"query": asked.Query, "source": "AI", "model": asked.Model,
		"prompt_id": asked.PromptID, "prompt_version": asked.PromptVersion,
		"produced_at": asked.ProducedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}
