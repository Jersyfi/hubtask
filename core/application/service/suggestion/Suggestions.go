// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package suggestion is the application layer of what AI proposed (J-05).
//
// Three use cases and one rule that shapes all of them: **accepting is an ordinary write**. There
// is no path in this package that changes an entry — what `AcceptSuggestion` does is call the use
// case a person would call, with that person as the actor, so the permission check, the validation,
// the event, the history entry and the metric are the ones that would have happened anyway. A
// suggestion cannot grant anybody anything, which is not a promise this package makes but a
// consequence of it having no other way to write.
package suggestion

import (
	"context"
	"sort"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ListSuggestionsName   = "ListSuggestions"
	AcceptSuggestionName  = "AcceptSuggestion"
	DismissSuggestionName = "DismissSuggestion"

	// suggestionsScope is the registry's scope. A suggestion is about an entry, so reading and
	// answering them travels with the entry's own scope rather than having a third one: somebody
	// who may write items may answer proposals about them.
	suggestionsRead  = "items:read"
	suggestionsWrite = "items:write"

	suggestionTarget = "ai_suggestion"
)

// The audit codes. Creating one is the producer's (J-06); answering one is a person's, and the two
// answers are separate actions because "accepted" and "turned down" are different facts about a
// workspace's relationship with its AI.
const (
	SuggestionRecordedAction  audit.Action = "ai.suggestion_recorded"
	SuggestionsReadAction     audit.Action = "ai.suggestions_read"
	SuggestionAcceptedAction  audit.Action = "ai.suggestion_accepted"
	SuggestionDismissedAction audit.Action = "ai.suggestion_dismissed"
)

// Catalogue is the slice of the use case registry accepting needs: run one use case by name.
//
// An interface rather than the registry, for the bulk's reason exactly - the registry is built
// *from* these descriptors, so a direct dependency would be a cycle the composition root cannot
// wire - and for one more that matters here: what this package can do to a workspace is exactly
// what it can name, and a narrow seam is what makes that reviewable.
type Catalogue interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Authorizer is the application layer's one question.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Targets is how a suggestion's target is checked: does it exist, may this actor see it, and what
// does it look like now.
//
// The digest is the target's own, computed by whoever produced the suggestion and recomputed here.
// It lives behind a port rather than in this package because what makes a work item's state is the
// work item's business - a suggestion has no opinion about which fields count.
type Targets interface {
	// Digest answers the fingerprint of the target's current state, or an error wrapping
	// shared.ErrNotFound when it is gone. Reading it is what proves the actor may see the target
	// at all, which is why there is no separate visibility question.
	Digest(ctx context.Context, actor appshared.ActorContext, targetType domain.TargetType, targetID shared.ID) ([]byte, error)
}

// Cases is what the three use cases share.
type Cases struct {
	Suggestions repository.Suggestions
	Targets     Targets
	Authorizer  Authorizer
	// Catalogue is how an acceptance writes. The only way this package writes anything.
	Catalogue  Catalogue
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// ListSuggestions answers what stands against one entry.
type ListSuggestions struct{ Cases Cases }

// ListQuery is the input, typed.
type ListQuery struct {
	TargetType domain.TargetType
	TargetID   shared.ID
	Status     domain.Status
	Cursor     string
	Size       int
}

// Execute answers one page.
func (h ListSuggestions) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListQuery,
) (repository.Page, error) {
	c := h.Cases
	if err := c.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     SuggestionsReadAction,
		TokenScope: suggestionsRead,
		TargetType: suggestionTarget,
		TargetID:   query.TargetID,
	}); err != nil {
		return repository.Page{}, err
	}
	if !query.TargetType.Valid() {
		return repository.Page{}, shared.ErrValidation.
			WithDetail("suggestions.target_type_unknown").
			WithFields(shared.FieldError{Path: "/target_type", Code: "suggestions.target_type_unknown"})
	}
	if query.Status != "" && !query.Status.Valid() {
		return repository.Page{}, shared.ErrValidation.
			WithDetail("suggestions.status_unknown").
			WithFields(shared.FieldError{Path: "/status", Code: "suggestions.status_unknown"})
	}

	var page repository.Page
	err := c.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// The target is read first, and that read is the visibility check: a suggestion about an
		// entry is exactly as readable as the entry, and asking the entry is how that stays one
		// rule rather than two. An entry the actor cannot see answers not-found, which is what it
		// would answer for the entry itself.
		if _, err := c.Targets.Digest(ctx, actor, query.TargetType, query.TargetID); err != nil {
			return err
		}
		found, err := c.Suggestions.List(ctx, repository.Query{
			TargetType: query.TargetType, TargetID: query.TargetID,
			Status: query.Status, Cursor: query.Cursor, Size: query.Size,
		})
		page = found
		return err
	})
	if err != nil {
		return repository.Page{}, err
	}
	return page, nil
}

// AcceptSuggestion applies what was proposed, as the accepting person's own write.
type AcceptSuggestion struct{ Cases Cases }

// Execute accepts one.
func (h AcceptSuggestion) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, overrides map[string]any,
) (domain.Suggestion, error) {
	return h.Cases.decide(ctx, actor, id, domain.StatusAccepted, overrides)
}

// DismissSuggestion turns one down.
type DismissSuggestion struct{ Cases Cases }

// Execute dismisses one.
func (h DismissSuggestion) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Suggestion, error) {
	return h.Cases.decide(ctx, actor, id, domain.StatusDismissed, nil)
}

// decide is both answers, because everything except the one line that applies is shared: the
// permission, the freshness, the once-only rule, the audit entry.
func (c Cases) decide(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, status domain.Status,
	overrides map[string]any,
) (domain.Suggestion, error) {
	action := SuggestionAcceptedAction
	if status == domain.StatusDismissed {
		action = SuggestionDismissedAction
	}
	if err := c.Authorizer.Authorize(ctx, actor, access.Request{
		// Answering a proposal about an entry needs what changing the entry needs. Dismissing
		// asks the same, deliberately: a workspace where anybody who can read can clear somebody
		// else's inbox of proposals is one where the inbox is not theirs.
		Permission: service.PermissionWriteItems,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     action,
		TokenScope: suggestionsWrite,
		TargetType: suggestionTarget,
		TargetID:   id,
	}); err != nil {
		return domain.Suggestion{}, err
	}

	var answered domain.Suggestion
	err := c.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		proposal, err := c.Suggestions.Find(ctx, id)
		if err != nil {
			return err
		}

		digest, err := c.Targets.Digest(ctx, actor, proposal.TargetType, proposal.TargetID)
		if err != nil {
			return err
		}
		// Staleness is checked for both answers and not only for acceptance. Dismissing a stale
		// proposal is harmless, but telling somebody it was stale is what stops them wondering
		// why the one they meant to accept was refused.
		if !proposal.Fresh(digest) {
			return domain.ErrStale
		}

		decided, err := proposal.Decide(status, actor.AccountID, c.Clock.Now())
		if err != nil {
			return err
		}

		if status == domain.StatusAccepted {
			// The whole design, in one call. The ordinary use case, the accepting person as the
			// actor, and therefore the ordinary permission check - somebody who could not make
			// this change by hand cannot make it by accepting.
			if err := c.apply(ctx, actor, proposal, overrides); err != nil {
				return err
			}
		}

		written, err := c.Suggestions.Decide(ctx, decided, proposal.Version)
		if err != nil {
			return err
		}
		if !written {
			// Two people answering one proposal. The first wins and the second is told, rather
			// than the second silently overwriting a decision somebody has already acted on.
			return domain.ErrAlreadyDecided
		}
		answered = decided
		return c.record(ctx, actor, action, decided)
	})
	if err != nil {
		return domain.Suggestion{}, err
	}
	return answered, nil
}

// appliers maps what a suggestion is about, and what accepting it does, onto the use case that
// does it.
//
// A table in code rather than a use case name on the row, and that is a security decision: a stored
// use-case name would be a stored capability, and a producer that could write one could point an
// acceptance at anything within the accepter's rights. Here the payload is data and the use case is
// code, which is the only arrangement in which "a suggestion grants nothing" is structural.
//
// The combinations this build does not serve yet are absent rather than wrong. J-06 adds the jumble
// entry's, J-07 the decomposition's, and a caller meeting a gap is told it is not built rather than
// that the suggestion is invalid - the distinction `deferredActions` draws for automation kinds.
var appliers = map[applierKey]string{
	{domain.TargetWorkItem, domain.KindFields}: "UpdateWorkItem",
	// Accepting a proposal about a jumble entry is converting it (J-06), which is why the
	// acceptance takes overrides: a model cannot name a destination collection, and
	// ConvertJumbleEntry requires one.
	{domain.TargetJumbleEntry, domain.KindFields}: "ConvertJumbleEntry",
	// A decomposition is not one call but a walk (J-07): one CreateWorkItem per node, in order,
	// each with the accepting person's rights at its destination. The name is here all the same,
	// because what this package can do is still exactly what it can name.
	{domain.TargetWorkItem, domain.KindDecomposition}: createWorkItemName,
}

// The use cases this package calls for something other than applying a payload: J-07's walk and
// K-01's, which are the same walk over two shapes, and K-02's move.
//
// Named here rather than written at the call site for `appliers`' reason: what this package can do
// to a workspace is exactly what it can name, and a short list is what makes that reviewable.
const (
	createWorkItemName = "CreateWorkItem"
	moveWorkItemName   = "MoveWorkItem"
	addLabelName       = "AddLabel"
	setCustomFieldName = "SetCustomField"
)

// under is the level a proposed subtask lands at: the default profile's CHILDREN row
// (domain-model.md §2), read downwards.
//
// A table rather than a read of the workspace's own profiles, because it is a *default* and not an
// authority: a workspace that narrowed what may sit under a task gets its refusal from the domain
// where every other create gets it, with the entry and whatever was created before it standing.
// Asking the profiles here would be this package deciding what the domain decides.
//
// An activity takes no children at all, which is why it is absent rather than mapped to itself.
var under = map[work.ItemType]work.ItemType{
	work.ItemTask:        work.ItemWorkPackage,
	work.ItemWorkPackage: work.ItemActivity,
}

type applierKey struct {
	target domain.TargetType
	kind   domain.Kind
}

// apply performs the acceptance, and does nothing else.
func (c Cases) apply(
	ctx context.Context, actor appshared.ActorContext, proposal domain.Suggestion,
	overrides map[string]any,
) error {
	name, served := appliers[applierKey{proposal.TargetType, proposal.Kind}]
	if !served {
		return shared.ErrUnavailable.
			WithDetail("suggestions.acceptance_not_built").
			WithParams(map[string]string{
				"target_type": string(proposal.TargetType), "kind": string(proposal.Kind),
			})
	}

	if proposal.Kind == domain.KindDecomposition {
		return c.plant(ctx, actor, name, proposal, overrides)
	}

	// What the acceptance grows itself never reaches the applier's input: `subtasks` is a walk
	// (K-01), and the registry would refuse a key `ConvertJumbleEntry` does not declare.
	grows := grown[applierKey{proposal.TargetType, proposal.Kind}]

	in := usecase.Input{}
	for field, value := range proposal.Payload {
		if grows[field] {
			continue
		}
		in[field] = value
	}
	// What the person changed or added before accepting, laid over the proposal. It is their own
	// input, checked by the use case with their own rights - exactly as if they had made the call
	// themselves - and it is what lets a jumble proposal be accepted at all, since a model cannot
	// name a destination collection.
	for field, value := range overrides {
		in[field] = value
	}
	// The target is the suggestion's, and it is written *after* both, so neither the payload nor
	// the overrides can move it. A proposal about one entry able to change another would be a
	// stored capability, and the registry would refuse nothing about it: `item_id` is a field
	// UpdateWorkItem declares, and `entry_id` one ConvertJumbleEntry does.
	in[targetKeys[proposal.TargetType]] = proposal.TargetID.String()

	out, err := c.Catalogue.Invoke(ctx, name, actor, in)
	if err != nil {
		return err
	}
	// What the acceptance performs itself, once the applier has written the rest.
	if grows["subtasks"] {
		if err := c.grow(ctx, actor, proposal, in, out); err != nil {
			return err
		}
	}
	if grows[labelsKey] {
		if err := c.tag(ctx, actor, proposal); err != nil {
			return err
		}
	}
	if grows[fieldsKey] {
		if err := c.fill(ctx, actor, proposal); err != nil {
			return err
		}
	}
	if grows[bucketKey] {
		return c.place(ctx, actor, proposal)
	}
	return nil
}

// fill writes the values a classification proposed for the fields a collection declared (K-03).
//
// One ordinary `SetCustomField` per key, as the accepting person, because that is how a custom
// field is written: one key per call, since the merge rule is per key. `UpdateWorkItem` has no such
// input, and the value was already checked against the definition by the same code this call runs -
// so what is written here is what a person filling the form by hand would have been allowed to
// write.
//
// In key order, so that two acceptances of the same proposal do the same thing in the same
// sequence: a map's own order is nobody's, and an audit trail that shuffles is one nobody can
// compare.
//
// A refusal is the answer, as it is for the column and the labels.
func (c Cases) fill(
	ctx context.Context, actor appshared.ActorContext, proposal domain.Suggestion,
) error {
	values, held := proposal.Payload[fieldsKey].(map[string]any)
	if !held {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if _, err := c.Catalogue.Invoke(ctx, setCustomFieldName, actor, usecase.Input{
			// The entry is the suggestion's, for `place`'s reason.
			"item_id": proposal.TargetID.String(),
			"key":     key,
			"value":   values[key],
		}); err != nil {
			return err
		}
	}
	return nil
}

// tag puts the labels a classification chose on the entry (K-02).
//
// One ordinary `AddLabel` each, as the accepting person, because a label is a set entry and not a
// field: `UpdateWorkItem` - the use case that applies the rest of a FIELDS proposal - has no such
// input, and never should. The identifiers are ones the material offered, so each is a label of the
// entry's own collection, which is what `AddLabel` requires.
//
// A refusal is the answer, as it is for the column: the labels are part of what was accepted.
func (c Cases) tag(
	ctx context.Context, actor appshared.ActorContext, proposal domain.Suggestion,
) error {
	chosen, held := proposal.Payload[labelsKey].([]any)
	if !held {
		return nil
	}
	for _, entry := range chosen {
		labelID, isText := entry.(string)
		if !isText || strings.TrimSpace(labelID) == "" {
			continue
		}
		if _, err := c.Catalogue.Invoke(ctx, addLabelName, actor, usecase.Input{
			// The entry is the suggestion's, for `place`'s reason.
			"item_id":  proposal.TargetID.String(),
			"label_id": labelID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// place puts the entry in the column a classification chose (K-02).
//
// Through `MoveWorkItem`, which is the use case that owns putting an entry somewhere: a card going
// into another column is a move, and the permission check, the history entry and the event a
// person reads should be a move's. `UpdateWorkItem` declares `bucket_id` and would have written it
// - that is why the key is in `grown` rather than left to the applier, and this is the whole of the
// difference.
//
// **A refusal is the answer**, which is where this differs from the walk under a converted entry.
// The column is part of what was accepted rather than something the acceptance grew afterwards, so
// somebody who may not move the entry has not half-accepted a classification - they have been
// refused one, and the labels go back with it.
func (c Cases) place(
	ctx context.Context, actor appshared.ActorContext, proposal domain.Suggestion,
) error {
	bucketID, chosen := proposal.Payload[bucketKey].(string)
	if !chosen || strings.TrimSpace(bucketID) == "" {
		return nil
	}
	_, err := c.Catalogue.Invoke(ctx, moveWorkItemName, actor, usecase.Input{
		// The entry is the suggestion's, never the payload's: a proposal able to name the entry it
		// moves would be a proposal about one entry moving another.
		"item_id":          proposal.TargetID.String(),
		"target_bucket_id": bucketID,
	})
	return err
}

// grow creates the work a jumble entry implied, under the item the conversion just made (K-01).
//
// J-07's walk over a different shape: one ordinary `CreateWorkItem` per title, in order, with the
// accepting person's rights and the ordering keys `Ordering.go` produces. What it is not is a
// second write path - the entry was converted by the use case that owns that, and each child is
// created by the use case that owns creating.
//
// **A refusal here leaves the conversion standing**, which is where this differs from `plant`. A
// breakdown is the whole proposal, so a first node refused means nothing was accepted and the
// refusal is the answer; a converted entry is the acceptance, and the titles under it are what the
// material implied. Somebody who converted a note and got the conversion undone because the third
// title was too long would reasonably think the feature broken. What they get instead is the entry,
// the children that were created, and the rest on their next read.
func (c Cases) grow(
	ctx context.Context, actor appshared.ActorContext, proposal domain.Suggestion,
	in usecase.Input, out usecase.Output,
) error {
	titles, _ := proposal.Payload["subtasks"].([]any)
	if len(titles) == 0 {
		return nil
	}

	// The item the conversion made, read off its answer. Never the payload's and never the
	// overrides': a proposal able to name the parent of what it grows would be a proposal about
	// one entry growing children under another.
	parentID, err := shared.ParseID(out.String("target_item_id"))
	if err != nil {
		return nil
	}

	// What the entry became decides what may sit under it. TASK unless the person accepting said
	// otherwise, which is `ConvertJumbleEntry`'s own default.
	became := work.ItemTask
	if named, held := in["type"].(string); held && strings.TrimSpace(named) != "" {
		became = work.ItemType(named)
	}
	childType, takesChildren := under[became]
	if !takesChildren {
		// An entry converted to an activity has nowhere to put them. Not an error: the conversion
		// is what was accepted, and inventing a level for the titles would put work where nobody
		// proposed it.
		return nil
	}

	nodes := make([]any, 0, len(titles))
	for _, title := range titles {
		nodes = append(nodes, map[string]any{"type": string(childType), "title": title})
	}
	// No overrides. What a person changed before accepting - the destination collection, the
	// title, the board column - is the *entry's*, and laying it over every child would give them
	// all one title and put children on a board only the entry belongs to.
	created := 0
	_ = c.plantUnder(ctx, actor, createWorkItemName, parentID, nodes, nil, &created)
	return nil
}

// plant creates the tree a decomposition proposes, one ordinary create at a time.
//
// Depth first and in order, because the order is what a person read when they accepted: a
// breakdown whose pieces arrived shuffled is not the breakdown they saw.
//
// **A partial result is a success, not a rollback**, and that is the decision worth naming. Each
// node is its own create with its own permission check, and a refusal at the third child is a
// fact about that child rather than about the two already standing - a person who accepted a
// breakdown and got nothing because the fifth activity had a title one character too long would
// reasonably think the feature broken. What they get instead is what was created, and the refusal
// that stopped it. Nothing is created at all only when the *first* node is refused, and then the
// refusal is the answer.
func (c Cases) plant(
	ctx context.Context, actor appshared.ActorContext, name string,
	proposal domain.Suggestion, overrides map[string]any,
) error {
	children, _ := proposal.Payload["children"].([]any)
	if len(children) == 0 {
		return shared.ErrInternal.WithDetail("suggestions.payload_empty")
	}

	created := 0
	err := c.plantUnder(ctx, actor, name, proposal.TargetID, children, overrides, &created)
	if err != nil && created == 0 {
		return err
	}
	// A refusal after something was created is reported through the log rather than the answer:
	// the acceptance happened, and what did not is the caller's next read of the entry's children.
	return nil
}

// plantUnder creates one level and recurses into what each node carries.
func (c Cases) plantUnder(
	ctx context.Context, actor appshared.ActorContext, name string, parentID shared.ID,
	nodes []any, overrides map[string]any, created *int,
) error {
	for _, entry := range nodes {
		node, isNode := entry.(map[string]any)
		if !isNode {
			return shared.ErrInternal.WithDetail("suggestions.payload_malformed")
		}

		in := usecase.Input{
			"type":  node["type"],
			"title": node["title"],
		}
		if notes, held := node["notes"]; held {
			in["notes"] = notes
		}
		// The overrides apply to every node - what a person changes before accepting a breakdown
		// is a property of the breakdown, like the collection it lands in, rather than of one
		// piece of it.
		for field, value := range overrides {
			in[field] = value
		}
		// The parent is this walk's, never the node's. A node naming its own parent would be a
		// proposal about one entry able to grow children under another.
		in["parent_id"] = parentID.String()

		out, err := c.Catalogue.Invoke(ctx, name, actor, in)
		if err != nil {
			return err
		}
		*created++

		grandchildren, _ := node["children"].([]any)
		if len(grandchildren) == 0 {
			continue
		}
		childID, err := shared.ParseID(out.String("id"))
		if err != nil {
			return err
		}
		if err := c.plantUnder(ctx, actor, name, childID, grandchildren, overrides, created); err != nil {
			return err
		}
	}
	return nil
}

// targetKeys is what each target kind is called in the input of the use case that acts on it.
var targetKeys = map[domain.TargetType]string{
	domain.TargetWorkItem:    "item_id",
	domain.TargetJumbleEntry: "entry_id",
}

// record writes the trail entry for a decision.
//
// The provenance travels with it - the model, the prompt version, the kind - because "who decided
// this" is only half an answer without "and on whose suggestion". What never travels is the
// payload: it is a proposal about somebody's entry and therefore their content (rule 10), and the
// change it caused is in the entry's own history, where it belongs.
func (c Cases) record(
	ctx context.Context, actor appshared.ActorContext,
	action audit.Action, decided domain.Suggestion,
) error {
	return c.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: c.Clock.Now(),
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: suggestionTarget,
		TargetID:   decided.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "status", Classification: audit.Open,
				To: string(decided.Status), From: string(domain.StatusProposed)},
			audit.Change{Field: "kind", Classification: audit.Open, To: string(decided.Kind)},
			audit.Change{Field: "model", Classification: audit.Open, To: decided.Model},
			audit.Change{Field: "prompt_version", Classification: audit.Open,
				To: decided.PromptVersion},
		),
	})
}

// suggestionOutput is the read shape, and it carries the provenance because the provenance is what
// a suggestion is for.
func suggestionOutput(proposal domain.Suggestion) usecase.Output {
	out := usecase.Output{
		"id":             proposal.ID.String(),
		"target_type":    string(proposal.TargetType),
		"target_id":      proposal.TargetID.String(),
		"kind":           string(proposal.Kind),
		"status":         string(proposal.Status),
		"payload":        proposal.Payload,
		"source":         string(proposal.Source),
		"model":          proposal.Model,
		"prompt_id":      proposal.PromptID,
		"prompt_version": proposal.PromptVersion,
		"produced_at":    proposal.ProducedAt,
		"created_at":     proposal.CreatedAt,
		"version":        proposal.Version,
	}
	if !proposal.DecidedAt.IsZero() {
		out["decided_at"] = proposal.DecidedAt
		out["decided_by"] = proposal.DecidedBy.String()
	}
	return out
}

func (h ListSuggestions) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListSuggestionsName,
		Summary: "What AI has proposed about one entry, newest first. Nothing here has changed " +
			"anything: a suggestion is a record carrying the model, the prompt version and the " +
			"moment it was produced, and it becomes a change only when somebody accepts it.",
		SideEffects: "None. Reads only.",
		TokenScope:  suggestionsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "target_type", Kind: usecase.KindString, Required: true,
				Enum:        targetTypeNames(),
				Description: "What the suggestions are about."},
			{Name: "target_id", Kind: usecase.KindID, Required: true,
				Description: "The entry. It is read first, so a suggestion is exactly as readable as its target."},
			{Name: "status", Kind: usecase.KindString, Enum: statusNames(),
				Description: "Narrows to one state. Omitted answers what is still standing."},
			{Name: "cursor", Kind: usecase.KindString, Description: "The page to continue from."},
			{Name: "page_size", Kind: usecase.KindInt, Description: "How many to answer."},
		},
		Audit: usecase.AuditDeclaration{
			Action: SuggestionsReadAction, TargetType: suggestionTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListSuggestions) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	targetID, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	page, err := h.Execute(ctx, actor, ListQuery{
		TargetType: domain.TargetType(in.String("target_type")),
		TargetID:   targetID,
		Status:     domain.Status(in.String("status")),
		Cursor:     in.String("cursor"),
		Size:       in.Int("page_size"),
	})
	if err != nil {
		return nil, err
	}

	items := make([]usecase.Output, 0, len(page.Items))
	for _, proposal := range page.Items {
		items = append(items, suggestionOutput(proposal))
	}
	return usecase.Output{
		"items": items, "next_cursor": page.NextCursor, "has_more": page.HasMore,
	}, nil
}

func (h AcceptSuggestion) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AcceptSuggestionName,
		Summary: "Applies what was proposed, as the accepting person's own write: the ordinary " +
			"use case, with the ordinary permission check and the ordinary history entry. A " +
			"suggestion made against an entry that has since been rewritten is stale and is " +
			"refused rather than applied to a state it was not made from.",
		SideEffects: "Performs the change through the use case that owns it, marks the " +
			"suggestion accepted, and writes two audit entries: the acceptance and the write.",
		TokenScope: suggestionsWrite,
		Input: []usecase.Field{
			{Name: "suggestion_id", Kind: usecase.KindID, Required: true,
				Description: "The proposal to accept."},
			{Name: "overrides", Kind: usecase.KindObject,
				Description: "What the person changed or added before accepting, laid over the " +
					"proposal. A jumble proposal needs the destination collection here, because " +
					"a model cannot know which collections a workspace has."},
		},
		Audit: usecase.AuditDeclaration{
			Action: SuggestionAcceptedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The change the acceptance performs writes the entry's history itself; a " +
				"second entry would describe the same change twice.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AcceptSuggestion) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("suggestion_id")
	if err != nil {
		return nil, err
	}
	// The registry has already checked that an object arrived, so what is left is reading it -
	// what is *in* it is the target use case's judgement, exactly as a query's filter tree is the
	// grammar's (usecase.KindObject).
	overrides, _ := in["overrides"].(map[string]any)
	accepted, err := h.Execute(ctx, actor, id, overrides)
	if err != nil {
		return nil, err
	}
	return suggestionOutput(accepted), nil
}

func (h DismissSuggestion) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DismissSuggestionName,
		Summary: "Turns a proposal down. A state and not a deletion, so \"what was proposed and " +
			"turned down\" has an answer, and the retention engine ages it out by rule.",
		SideEffects: "Marks the suggestion dismissed and writes an audit entry. Changes nothing else.",
		TokenScope:  suggestionsWrite,
		Input: []usecase.Field{
			{Name: "suggestion_id", Kind: usecase.KindID, Required: true,
				Description: "The proposal to dismiss."},
		},
		Audit: usecase.AuditDeclaration{
			Action: SuggestionDismissedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Turning a proposal down changes no entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DismissSuggestion) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("suggestion_id")
	if err != nil {
		return nil, err
	}
	dismissed, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return suggestionOutput(dismissed), nil
}

// The enums the descriptors declare come from the domain's own closed sets rather than from a copy
// of them: a declared value the domain refuses is a written refusal nobody can reach.
func targetTypeNames() []string {
	names := make([]string, 0, len(domain.TargetTypes()))
	for _, value := range domain.TargetTypes() {
		names = append(names, string(value))
	}
	return names
}

func statusNames() []string {
	names := make([]string, 0, len(domain.Statuses()))
	for _, value := range domain.Statuses() {
		names = append(names, string(value))
	}
	return names
}
