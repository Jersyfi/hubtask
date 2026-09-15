// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// AiTranslateName is the catalogue name (domain-model.md §5).
const AiTranslateName = "AiTranslate"

// TranslationAskedAction records that somebody read an entry in another language: the entry and
// the language, never the text (rule 10). Its own action, because "what was sent to a provider,
// and about what" is the question a data protection officer asks, and this sends an entry's
// content somewhere without recording anything the other AI actions record.
const TranslationAskedAction audit.Action = "ai.translation_asked"

// translatePrompt is the instruction in the store (ADR-0049 decision 3).
const translatePrompt = "translate"

// translateAnswerKeys is what the prompt asks for and the code keeps, compared by the gate K-01
// left behind (test/architecture/promptanswers_test.go). Both halves are deliberate.
var translateAnswerKeys = []string{"notes", "title"}

// TranslationAnswerKeys is the allow list the gate reads beside the suggestion service's.
func TranslationAnswerKeys() map[string][]string {
	return map[string][]string{translatePrompt: append([]string(nil), translateAnswerKeys...)}
}

// defaultTranslateTimeout bounds the provider call.
//
// Longer than the search's 800 ms, because a translation is not something the product can answer
// without the provider - there is no lexical fallback to prefer - and shorter than a job's,
// because a person is waiting for it. Every way of not getting an answer inside it is the same
// refusal (ai.unavailable), which is the port's discipline applied to a read.
const defaultTranslateTimeout = 8 * time.Second

// AiTranslate reads one entry in another language (ai-first.md §2's translation row, M-11).
//
// A read, not a record: the row says "display only, not persisted" and i18n-l10n.md §7 says user
// content is never translated in place. So there is no AiSuggestion, no acceptance and nothing
// written - the answer is handed back and gone. It is the second AI call somebody waits for, after
// the search's meaning, and bounded the same way.
type AiTranslate struct {
	// Reader is the ordinary read of the entry: the permission question and the existence
	// question at once, asked before any provider is called.
	Reader    GetWorkItem
	Providers AiProviders
	Prompts   aiprovider.Prompts
	Audit     audit.Sink
	Clock     clock.Clock
	// Timeout bounds the provider call. Zero takes the default above.
	Timeout time.Duration
}

// TranslateCommand is the input, typed.
type TranslateCommand struct {
	ItemID shared.ID
	// TargetLocale is BCP 47. Empty takes the caller's own.
	TargetLocale string
}

// Translation is the answer: the two texts and the provenance every AI output carries.
type Translation struct {
	TargetLocale   string
	SourceLanguage string
	Title          string
	Notes          string
	Model          string
	PromptID       string
	PromptVersion  string
	ProducedAt     time.Time
}

// aiUnavailable is the one refusal for every way of not getting an answer (ADR-0049, QS-09).
var aiUnavailable = aiprovider.ErrUnavailable

// Execute asks, or refuses.
//
// The order is the one every AI use case keeps: the entry's own permission first, so that this
// cannot become a way to learn what a workspace has configured; the consent and the provider
// after it, so that a workspace with AI switched off is told so whatever the entry is.
func (h AiTranslate) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd TranslateCommand,
) (Translation, error) {
	target, ok := shared.LanguageTag(cmd.TargetLocale)
	if !ok {
		return Translation{}, shared.ErrValidation.
			WithDetail("items.content_language_invalid").
			WithParams(map[string]string{"value": cmd.TargetLocale}).
			WithFields(shared.FieldError{Path: "/target_locale", Code: "items.content_language_invalid"})
	}
	if target == "" {
		target = actor.Locale
	}
	if target == "" {
		return Translation{}, shared.ErrValidation.
			WithDetail("ai.target_locale_required").
			WithFields(shared.FieldError{Path: "/target_locale", Code: "ai.target_locale_required"})
	}

	item, err := h.Reader.Execute(ctx, actor, GetWorkItemQuery{ItemID: cmd.ItemID})
	if err != nil {
		return Translation{}, err
	}

	if h.Providers == nil || h.Prompts == nil {
		return Translation{}, aiUnavailable
	}
	provider, err := h.Providers.For(ctx, actor)
	if err != nil {
		// A provider that cannot be resolved is one that cannot answer; the resolver already
		// answers NoopAi for every ordinary reason - no configuration, no consent, a spent budget.
		return Translation{}, aiUnavailable
	}
	if !provider.Capabilities().Completion {
		return Translation{}, aiUnavailable
	}
	prompt, err := h.Prompts.Get(translatePrompt)
	if err != nil {
		return Translation{}, err
	}

	// Recorded before the call rather than after it, for the reason the asking use cases record
	// theirs: what is auditable is that content was sent, and a call that timed out sent it.
	if err := h.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: h.Clock.Now(),
		Action: TranslationAskedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: itemTarget, TargetID: item.ID,
		Changes: audit.Changes(
			audit.Change{Field: "target_locale", Classification: audit.Open, To: target},
		),
	}); err != nil {
		return Translation{}, err
	}

	timeout := h.Timeout
	if timeout <= 0 {
		timeout = defaultTranslateTimeout
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Prompt.Ask is the only place the instruction and the content meet, and the target language
	// travels at the head of the content - a parameter, validated as a tag above, never a free
	// sentence - the way the asking use cases carry the date (ai-first.md §1.3).
	answer, err := provider.Complete(bounded, prompt.Ask(translationMaterial(item, target)))
	if err != nil {
		return Translation{}, aiUnavailable
	}

	title, notes, ok := translationFrom(answer.Text)
	if !ok {
		// A model that answered something this cannot read has answered nothing useful, and a
		// person is waiting: refused rather than retried, because the next attempt asks the same
		// question of the same model.
		return Translation{}, aiUnavailable
	}
	return Translation{
		TargetLocale: target, SourceLanguage: item.ContentLanguage,
		Title: title, Notes: notes,
		Model: answer.Model, PromptID: answer.PromptID, PromptVersion: answer.PromptVersion,
		ProducedAt: answer.ProducedAt,
	}, nil
}

// translationMaterial is the user message: the target on its first line, the source language
// where the entry states one, then the title and the notes - fenced as content by the prompt.
func translationMaterial(item domain.WorkItem, target string) string {
	var content strings.Builder
	content.WriteString("Target language: " + target + "\n")
	if item.ContentLanguage != "" {
		content.WriteString("Source language: " + item.ContentLanguage + "\n")
	}
	content.WriteString("\nTitle:\n" + item.Title + "\n")
	content.WriteString("\nNotes:\n" + item.Notes + "\n")
	return content.String()
}

// translationFrom reads the answer: one object, the two keys the prompt asks for, and nothing a
// model volunteered under another name.
func translationFrom(text string) (title, notes string, ok bool) {
	var answer struct {
		Title *string `json:"title"`
		Notes *string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(objectIn(text)), &answer); err != nil || answer.Title == nil {
		return "", "", false
	}
	if answer.Notes != nil {
		notes = *answer.Notes
	}
	return strings.TrimSpace(*answer.Title), notes, true
}

// objectIn answers the first JSON object in a text, so that a model which wrapped its answer in a
// sentence or a fence still answers.
func objectIn(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return ""
	}
	return text[start : end+1]
}

// Descriptor registers the translation in all three channels.
func (h AiTranslate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AiTranslateName,
		Summary: "Reads one entry's title and notes in another language, through the workspace's " +
			"AI provider. Display only: nothing is stored, no suggestion is recorded, and " +
			"nothing is accepted - the original stays as it is.",
		SideEffects: "Sends the entry's title and notes to the provider and writes an audit " +
			"entry naming the entry and the language, never the text.",
		TokenScope:  itemsRead,
		Destructive: false,
		// A read, and declared as one: nothing about the entry changes. The audit obligation
		// stands all the same - what is recorded is that content was sent to a provider.
		ReadOnly: true,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry to read in another language.",
			},
			{
				Name: "target_locale", Kind: usecase.KindString,
				Description: "BCP 47. Absent, the caller's own locale.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TranslationAskedAction, TargetType: itemTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AiTranslate) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	translation, err := h.Execute(ctx, actor, TranslateCommand{
		ItemID: itemID, TargetLocale: in.String("target_locale"),
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"target_locale":   translation.TargetLocale,
		"source_language": stringOrNil(translation.SourceLanguage),
		"title":           translation.Title,
		"notes":           translation.Notes,
		"source":          "AI",
		"model":           translation.Model,
		"prompt_id":       translation.PromptID,
		"prompt_version":  translation.PromptVersion,
		"produced_at":     translation.ProducedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}
