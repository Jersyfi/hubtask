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
	"github.com/Jersyfi/hubtask/core/application/usecase"
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
	// Catalogue is how an applied answer is accepted: through the use case, never around it.
	Catalogue Catalogue
	// Fields narrows a proposal to what the use case that would apply it declares. Nil narrows
	// nothing, which is what a build wired before J-16 did - and what it produced was a suggestion
	// nobody could accept.
	Fields     Fields
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// The prompts this build can ask with, and what a node of each answer may carry.
//
// Keyed by prompt rather than by kind, because three of `automation.md` §1.3's actions produce a
// FIELDS suggestion and each asks a different question: suggesting fields, summarising and
// classifying differ in the prompt and in which fields the answer may set, and in nothing else.
// The allow list is per prompt for that reason - a summariser that came back with labels has
// answered a question nobody asked.
var promptFields = map[string]map[string]bool{
	"suggest-fields": {"title": true, "notes": true, "due_date": true, "labels": true},
	"summarize":      {"notes": true},
	"classify":       {"labels": true},
	// A decomposition's shape is a tree rather than a field set, and `keptTree` is its allow list.
	"decompose": nil,
}

// defaultPrompts is what a kind is asked with when a job does not say.
//
// It exists for one reason: a job written by the release before this one carries no prompt, and it
// still runs after an upgrade (core/port/queue - the payload outlives the process that wrote it).
var defaultPrompts = map[domain.Kind]string{
	domain.KindFields:        "suggest-fields",
	domain.KindDecomposition: "decompose",
}

// Execute asks, and records what came back.
//
// Nothing partial is stored. A provider that answers something unparseable, or proposes nothing,
// leaves no row: an empty suggestion is one somebody would accept to no effect, and a list of them
// is an inbox of noise. The job then finishes rather than retrying - a model that answered badly
// will answer badly again, and the person asks again if they want to.
func (h Produce) Execute(
	ctx context.Context, actor appshared.ActorContext, request Request,
) error {
	promptID := request.PromptID
	if promptID == "" {
		promptID = defaultPrompts[request.Kind]
	}
	if _, known := promptFields[promptID]; !known {
		return shared.ErrInternal.WithDetail("ai.prompt_unknown").
			WithParams(map[string]string{"prompt": promptID})
	}
	prompt, err := h.Prompts.Get(promptID)
	if err != nil {
		return err
	}
	targetType, targetID, kind := request.TargetType, request.TargetID, request.Kind

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

	payload, ok := payloadFrom(kind, promptID, answer.Text, h.applicable(request))
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

	if err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Suggestions.Record(ctx, proposal)
	}); err != nil {
		return err
	}
	if !request.Apply {
		return nil
	}

	// Accepted through the use case, not around it. A rule that applies an answer directly is a
	// rule whose `run_as` account makes the change, with that account's rights checked where they
	// always are - so an action cannot reach further than the person the rule runs as
	// (automation.md §2).
	_, err = h.Catalogue.Invoke(ctx, "AcceptSuggestion", actor, usecase.Input{
		"suggestion_id": proposal.ID.String(),
	})
	return err
}

// The allow list is the security half of parsing an answer, and `promptFields` is where it lives.
//
// The payload is later merged into a use case's input, so a key nobody expected would be a field a
// model chose to set. The registry would refuse an *undeclared* one (C-07) - but it would accept a
// *declared* one nobody meant to offer, and `collection_id` is exactly such a field. Filtering is
// what keeps "a model proposes text" from becoming "a model proposes a destination".

// Request is one question to a provider.
type Request struct {
	TargetType domain.TargetType
	TargetID   shared.ID
	Kind       domain.Kind
	// PromptID names the question. Empty takes the kind's default, which is what a job written by
	// the previous release carries.
	PromptID string
	// Apply is `automation.md` §1.3's "or applied directly, configured explicitly": the answer is
	// accepted the moment it arrives, as the person the rule runs as, rather than waiting for
	// somebody to read it.
	//
	// It goes through AcceptSuggestion like every other acceptance, which is what keeps the record
	// honest: the suggestion exists first with its provenance, the acceptance is audited as its
	// own act, and the change is the ordinary use case with the ordinary permission check. An
	// applied answer is therefore not a shortcut past any of it - it is the same path with nobody
	// pausing in the middle.
	Apply bool
}

// applicable is the set of fields the use case that would apply this suggestion declares, or
// nothing where this build cannot say - which narrows nothing rather than everything.
//
// It reads the descriptor rather than a second list beside `appliers`, so the day somebody adds a
// field to `ConvertJumbleEntry` the suggestions may propose it, with nothing to remember.
func (h Produce) applicable(request Request) map[string]bool {
	if h.Fields == nil {
		return nil
	}
	name, served := appliers[applierKey{request.TargetType, request.Kind}]
	if !served {
		return nil
	}
	declared, known := h.Fields.InputsOf(name)
	if !known {
		return nil
	}
	fields := make(map[string]bool, len(declared))
	for _, field := range declared {
		fields[field] = true
	}
	return fields
}

// payloadFrom reads a model's answer in the shape its kind fixes, its prompt narrows, and - for a
// field set - the use case that would apply it can actually take.
//
// That last narrowing was missing until J-16, and what it produced was a suggestion nobody could
// ever accept. `suggest-fields` proposes a title, notes, a due date and labels; a proposal about a
// jumble entry is applied by `ConvertJumbleEntry`, which declares `title` and not the other three -
// and the registry refuses an input a descriptor does not declare. So the record was produced,
// stored and listed, and every acceptance of it answered `validation_failed`. Narrowing here rather
// than dropping fields at acceptance is the honest half of the choice: a person reading a proposal
// should be reading what they could actually accept.
func payloadFrom(kind domain.Kind, promptID, text string, applicable map[string]bool) (map[string]any, bool) {
	answered, ok := objectFrom(text)
	if !ok {
		return nil, false
	}
	switch kind {
	case domain.KindFields:
		return keptFields(answered, Narrowed(promptFields[promptID], applicable)), true
	case domain.KindDecomposition:
		return keptTree(answered)
	default:
		return nil, false
	}
}

// maxProposedNodes bounds a tree. The prompt asks for eight; this is what happens when a model
// ignores it, and it is a bound rather than a truncation - a tree cut in half is a breakdown
// nobody proposed, where a refusal is a proposal somebody asks for again.
const maxProposedNodes = 32

// keptTree reads the tree of a DECOMPOSITION answer, node by node, keeping only what a node may
// carry.
//
// Two levels and no more, which is what the item model allows underneath a task and what the
// prompt asks for. A deeper answer is refused rather than flattened: flattening would put
// activities where a person did not propose them.
func keptTree(answered map[string]any) (map[string]any, bool) {
	children, count, ok := keptChildren(answered["children"], 0)
	if !ok || count > maxProposedNodes {
		return nil, false
	}
	if len(children) == 0 {
		// A model that found nothing to break down has answered correctly, and there is nothing
		// to record: an empty proposal is one somebody would accept to no effect.
		return nil, true
	}
	return map[string]any{"children": children}, true
}

// nodeTypes are the two an item under a task may be. `TASK` is deliberately absent: a task under a
// task is a shape the domain refuses, and proposing one would be proposing a refusal.
var nodeTypes = map[string]bool{"WORK_PACKAGE": true, "ACTIVITY": true}

func keptChildren(value any, depth int) ([]any, int, bool) {
	if value == nil {
		return nil, 0, true
	}
	list, isList := value.([]any)
	if !isList {
		return nil, 0, false
	}
	if depth > 1 {
		// Nothing sits under an activity.
		return nil, 0, false
	}

	kept := make([]any, 0, len(list))
	total := 0
	for _, entry := range list {
		node, isNode := entry.(map[string]any)
		if !isNode {
			return nil, 0, false
		}
		kind, _ := node["type"].(string)
		title, _ := node["title"].(string)
		if !nodeTypes[kind] || strings.TrimSpace(title) == "" {
			return nil, 0, false
		}

		clean := map[string]any{"type": kind, "title": title}
		if notes, held := node["notes"].(string); held && strings.TrimSpace(notes) != "" {
			clean["notes"] = notes
		}
		grandchildren, under, ok := keptChildren(node["children"], depth+1)
		if !ok {
			return nil, 0, false
		}
		if len(grandchildren) > 0 {
			clean["children"] = grandchildren
		}
		kept = append(kept, clean)
		total += 1 + under
	}
	return kept, total, true
}

// keptFields reads a model's answer as the fields it was asked for.
//
// Tolerant of the two things every model does - a fenced code block around the JSON, and prose
// before it - and intolerant of everything else. What it will not do is repair: a half-formed
// answer produces no suggestion rather than a suggestion with a guess in it.
// Narrowed intersects the prompt's allow list with what the applier declares.
//
// An applier this build does not serve, or one whose declared inputs cannot be read, narrows
// nothing rather than everything: a suggestion with no fields at all is not stored, and answering
// "the model proposed nothing" for a lookup that failed would be a lie about the model.
func Narrowed(allowed, applicable map[string]bool) map[string]bool {
	if len(applicable) == 0 {
		return allowed
	}
	both := make(map[string]bool, len(allowed))
	for field := range allowed {
		if applicable[field] {
			both[field] = true
		}
	}
	return both
}

func keptFields(answered map[string]any, allowed map[string]bool) map[string]any {
	kept := make(map[string]any, len(answered))
	for key, value := range answered {
		if !allowed[key] {
			continue
		}
		// An empty value proposes nothing and would only clutter the shape a person reads.
		if text, isText := value.(string); isText && strings.TrimSpace(text) == "" {
			continue
		}
		kept[key] = value
	}
	return kept
}

// objectFrom finds the JSON object in a model's answer.
//
// Tolerant of the two things every model does - a fenced code block around the JSON, and prose
// before it - and intolerant of everything else. What it will not do is repair: a half-formed
// answer produces no suggestion rather than a suggestion with a guess in it.
func objectFrom(text string) (map[string]any, bool) {
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
	return answered, true
}

// IsUnavailable reports whether an error is the AI port's one refusal, so a caller can tell "the
// provider is out of reach" from "something went wrong" without importing the port's sentinel.
func IsUnavailable(err error) bool {
	return errors.Is(err, shared.ErrUnavailable) &&
		shared.AsError(err).DetailCode == "ai.unavailable"
}
