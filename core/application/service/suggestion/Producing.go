// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Providers answers which provider a workspace uses (J-03). An interface here rather than the
// adapter, because the application layer may not import one (ADR-0001).
type Providers interface {
	For(ctx context.Context, actor appshared.ActorContext) (aiprovider.Provider, error)
}

// Material is what a suggestion is made from: the text to describe, and the fingerprint of the
// state it describes.
//
// The two travel together on purpose. A producer that read the text and computed the digest
// separately could read one state and fingerprint another, and the staleness check would then pass
// for a suggestion made from something else - which is the exact failure the digest exists to
// prevent, arrived at through the back door.
type Material struct {
	// Content is what goes to the provider, as user content and never as instruction.
	Content string
	// Digest is the fingerprint of the state Content was read from.
	Digest []byte
}

// Sources reads the material for one target kind.
type Sources interface {
	Material(ctx context.Context, actor appshared.ActorContext, targetType domain.TargetType, targetID shared.ID) (Material, error)
}

// Produce asks a provider and records what it answered (J-06).
//
// It is not a use case and is deliberately not in the catalogue: nobody asks for a suggestion to be
// *recorded*, they ask for one to be *made* (`SuggestFromJumbleEntry`), and this is the job that
// runs afterwards. Which also means it has no actor of its own - it acts for the person who asked,
// so the read it performs and the consent it is subject to are theirs.
type Produce struct {
	Providers   Providers
	Prompts     aiprovider.Prompts
	Sources     Sources
	Suggestions repository.Suggestions
	UnitOfWork  persistence.UnitOfWork
	Clock       clock.Clock
	IDs         clock.IDGenerator
}

// The prompt each kind is asked with, named once. A map for the reason `appliers` is one: what
// this package can ask is exactly what it can name.
var prompts = map[domain.Kind]string{
	domain.KindFields: "suggest-fields",
}

// Execute asks, and records what came back.
//
// Nothing partial is stored. A provider that answers something unparseable, or proposes nothing,
// leaves no row: an empty suggestion is one somebody would accept to no effect, and a list of them
// is an inbox of noise. The job then finishes rather than retrying - a model that answered badly
// will answer badly again, and the person asks again if they want to.
func (h Produce) Execute(
	ctx context.Context, actor appshared.ActorContext,
	targetType domain.TargetType, targetID shared.ID, kind domain.Kind,
) error {
	promptID, named := prompts[kind]
	if !named {
		return shared.ErrInternal.WithDetail("suggestions.kind_unknown").
			WithParams(map[string]string{"value": string(kind)})
	}
	prompt, err := h.Prompts.Get(promptID)
	if err != nil {
		return err
	}

	provider, err := h.Providers.For(ctx, actor)
	if err != nil {
		return err
	}
	if !provider.Capabilities().Completion {
		// The workspace switched AI off, or withdrew consent, between the asking and the running.
		// The same refusal the asking would have given, which is what makes the two consistent.
		return aiprovider.ErrUnavailable
	}

	// The material is read inside a transaction and the provider is called outside one: an AI call
	// is somebody else's machine, and a transaction waiting on one holds a connection for as long
	// as they feel like taking (observability-reliability.md §8).
	var material Material
	if err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			read, err := h.Sources.Material(ctx, actor, targetType, targetID)
			material = read
			return err
		}); err != nil {
		return err
	}
	if strings.TrimSpace(material.Content) == "" {
		// Nothing to describe. Not an error and not a suggestion: an empty entry produces an
		// empty proposal, and recording one would be recording noise.
		return nil
	}

	// Prompt.Ask is the only place the instruction and the content are put together, and it puts
	// them in two messages with two roles. That is ai-first.md §1.3 as a shape rather than as a
	// rule somebody remembers: this code cannot merge them if it tries.
	answer, err := provider.Complete(ctx, prompt.Ask(material.Content))
	if err != nil {
		return err
	}

	payload, ok := fieldsFrom(answer.Text)
	if !ok || len(payload) == 0 {
		// A model that answered something this cannot read has answered nothing useful. Finished
		// rather than retried: the next attempt asks the same question of the same model.
		return nil
	}

	proposal, err := domain.New(domain.NewInput{
		ID: h.IDs.NewID(), TenantID: actor.TenantID,
		TargetType: targetType, TargetID: targetID, Kind: kind,
		Payload: payload,
		Provenance: domain.Provenance{
			Model: answer.Model, PromptID: answer.PromptID,
			PromptVersion: answer.PromptVersion, ProducedAt: answer.ProducedAt,
		},
		InputDigest: material.Digest,
		Now:         h.Clock.Now(),
	})
	if err != nil {
		return err
	}

	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Suggestions.Record(ctx, proposal)
	})
}

// suggestedFields are the keys a FIELDS proposal may carry, and the only ones.
//
// An allow list rather than "whatever the model said", and it is the security half of parsing an
// answer: the payload is later merged into a use case's input, so a key nobody expected would be a
// field a model chose to set. The registry would refuse an undeclared one (C-07) - but it would
// *accept* a declared one nobody meant to offer, and `collection_id` is exactly such a field.
// Filtering here is what keeps "a model proposes text" from becoming "a model proposes a
// destination".
var suggestedFields = map[string]bool{
	"title": true, "notes": true, "due_date": true, "labels": true,
}

// fieldsFrom reads a model's answer as the fields it was asked for.
//
// Tolerant of the two things every model does - a fenced code block around the JSON, and prose
// before it - and intolerant of everything else. What it will not do is repair: a half-formed
// answer produces no suggestion rather than a suggestion with a guess in it.
func fieldsFrom(text string) (map[string]any, bool) {
	trimmed := strings.TrimSpace(text)
	if fenced := strings.Index(trimmed, "```"); fenced >= 0 {
		rest := trimmed[fenced+3:]
		if line := strings.IndexByte(rest, '\n'); line >= 0 {
			rest = rest[line+1:]
		}
		if end := strings.Index(rest, "```"); end >= 0 {
			rest = rest[:end]
		}
		trimmed = strings.TrimSpace(rest)
	}
	start, end := strings.IndexByte(trimmed, '{'), strings.LastIndexByte(trimmed, '}')
	if start < 0 || end <= start {
		return nil, false
	}

	var answered map[string]any
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &answered); err != nil {
		return nil, false
	}

	kept := make(map[string]any, len(answered))
	for key, value := range answered {
		if !suggestedFields[key] {
			continue
		}
		// An empty value proposes nothing and would only clutter the shape a person reads.
		if text, isText := value.(string); isText && strings.TrimSpace(text) == "" {
			continue
		}
		kept[key] = value
	}
	return kept, true
}

// IsUnavailable reports whether an error is the AI port's one refusal, so a caller can tell "the
// provider is out of reach" from "something went wrong" without importing the port's sentinel.
func IsUnavailable(err error) bool {
	return errors.Is(err, shared.ErrUnavailable) &&
		shared.AsError(err).DetailCode == "ai.unavailable"
}
