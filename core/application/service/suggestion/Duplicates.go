// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strconv"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// SuggestDuplicatesName is the catalogue name, as domain-model.md §5 writes it.
const SuggestDuplicatesName = "SuggestDuplicates"

// DuplicatesAskedAction records that somebody asked which entries look like this one.
//
// Its own action for the reason each of the others has one: "what was asked, and about what" is
// the question ADR-0018 decision 7 asks, and this is the one AI feature that sends nothing
// anywhere - which is worth being able to tell apart in a trail rather than inferring.
const DuplicatesAskedAction audit.Action = "ai.duplicates_asked"

// Neighbours is the slice of the embedding store this use case needs (K-04).
type Neighbours interface {
	Near(ctx context.Context, itemID shared.ID, floor float64, limit int) (workrepo.Nearby, error)
}

// SemanticStore answers whether this installation carries an embedding store at all.
//
// The same question `SearchMeaning` asks before it spends a provider call, asked here before a
// statement is run against a table that may not exist: pgvector is detected rather than demanded
// (ADR-0050), and an installation without it has no store to query.
type SemanticStore interface {
	Available(ctx context.Context) (bool, error)
}

// Readers narrows a list of candidates to what one actor may see.
//
// Declared here rather than borrowed from the work service, because an interface belongs to
// whoever calls it - and because what this needs is one question about many paths, which is
// exactly the shape a search needs and nothing else about a search.
type Readers interface {
	Permitted(
		ctx context.Context, actor appshared.ActorContext, request access.Request,
		paths [][]identity.Scope,
	) ([]bool, error)
}

// SuggestDuplicates answers which entries look like one entry (K-04).
//
// **The one AI feature in this product that asks nothing of a provider.** Two entries are near
// each other in the embedding space or they are not, and J-10 already maintains the vector that
// says so - so this is a query with a threshold, it costs no tokens, it spends no budget, and it
// works on an installation whose provider can embed and cannot complete. There is no `Providers`
// field on this struct, which is how that stays true rather than being asserted.
//
// Synchronous, for the same reason: the others queue because a provider is somebody else's
// machine, and this one reads an index that is already there.
type SuggestDuplicates struct {
	Cases      Cases
	Neighbours Neighbours
	Semantic   SemanticStore
	Reader     Readers
	IDs        clock.IDGenerator
	// Floor is the similarity two entries have to reach before either is proposed as the other's
	// duplicate. Zero takes DefaultDuplicateFloor.
	Floor float64
	// Limit bounds how many are proposed. Zero takes the default below.
	Limit int
}

// DefaultDuplicateFloor is the similarity below which two entries are not the same thing.
//
// A number somebody has to choose, and one chosen by a session is one nobody can defend later - so
// it is configuration (`HUBTASK_AI_DUPLICATE_THRESHOLD`) and this is only its default.
//
// 0.9 rather than something lower, because the cost of the two mistakes is not symmetric. A
// duplicate that is not proposed is a duplicate somebody finds by searching, which is what they do
// today; a proposal that is not a duplicate is an inbox of noise, and an inbox of noise is how a
// feature stops being read at all. The embedding models this product speaks to produce vectors
// normalised for cosine similarity, where two texts about the same subject in different words sit
// well above 0.8 and two paraphrases of one sentence sit above 0.95 - so 0.9 is the band where
// "the same piece of work, written twice" lives rather than "about the same subject".
const DefaultDuplicateFloor = 0.9

// defaultDuplicateLimit is how many neighbours one answer carries. Small: this is a question about
// whether something already exists, and a person reads two or three candidates. A list of twenty is
// a search result, and search is where that belongs.
const defaultDuplicateLimit = 5

// Execute answers the entries near one entry, and records what it found.
//
// Nothing near, no store, no vector: the same answer to all three - no suggestion, no error. That
// is J-10's degradation reused rather than reinvented, and it is what makes this feature safe to
// offer on an installation that cannot support it.
func (h SuggestDuplicates) Execute(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (domain.Suggestion, bool, error) {
	c := h.Cases
	if err := c.Authorizer.Authorize(ctx, actor, access.Request{
		// Asking records a suggestion against the workspace, so it asks for what writing asks
		// for - the same question the other four ask, arrived at without spending anything.
		Permission: service.PermissionWriteItems,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     DuplicatesAskedAction,
		TokenScope: suggestionsWrite,
		TargetType: suggestionTarget,
		TargetID:   itemID,
	}); err != nil {
		return domain.Suggestion{}, false, err
	}

	var (
		digest []byte
		near   workrepo.Nearby
	)
	if err := c.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			// The cheapest question first, and the one that decides whether the statement below
			// can be run at all: an installation with no pgvector has no store to read.
			available, err := h.Semantic.Available(ctx)
			if err != nil || !available {
				return err
			}
			// The entry is read through its own use case, which is the permission check and the
			// existence check at once - and it is the fingerprint the suggestion is judged stale
			// against.
			if digest, err = c.Targets.Digest(ctx, actor, domain.TargetWorkItem, itemID); err != nil {
				return err
			}
			near, err = h.Neighbours.Near(ctx, itemID, h.floor(), h.limit())
			return err
		}); err != nil {
		return domain.Suggestion{}, false, err
	}
	if !near.Embedded || len(near.Candidates) == 0 {
		return domain.Suggestion{}, false, nil
	}

	// The narrowing, and it is the security-critical half rather than a detail. The rows came from
	// everywhere in the workspace at once, so no scope check upstream could have bounded them: an
	// answer naming an entry in a collection the actor cannot open would be T-04 with a different
	// verb (SearchItems.visible, rule 2).
	visible, err := h.visible(ctx, actor, near.Candidates)
	if err != nil {
		return domain.Suggestion{}, false, err
	}
	if len(visible) == 0 {
		return domain.Suggestion{}, false, nil
	}

	proposal, err := domain.New(domain.NewInput{
		ID: h.IDs.NewID(), TenantID: actor.TenantID,
		TargetType: domain.TargetWorkItem, TargetID: itemID,
		Kind:    domain.KindDuplicates,
		Payload: duplicatesPayload(visible),
		Provenance: domain.Provenance{
			// The embedding model, because that is what produced the vectors that were compared -
			// and no prompt, because none was asked (K-04).
			Model: near.Model, ProducedAt: c.Clock.Now(),
		},
		InputDigest: digest,
		Now:         c.Clock.Now(),
	})
	if err != nil {
		return domain.Suggestion{}, false, err
	}

	if err := c.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := c.Suggestions.Record(ctx, proposal); err != nil {
			return err
		}
		return h.record(ctx, actor, proposal, len(visible))
	}); err != nil {
		return domain.Suggestion{}, false, err
	}
	return proposal, true, nil
}

func (h SuggestDuplicates) floor() float64 {
	if h.Floor <= 0 {
		return DefaultDuplicateFloor
	}
	return h.Floor
}

func (h SuggestDuplicates) limit() int {
	if h.Limit <= 0 {
		return defaultDuplicateLimit
	}
	return h.Limit
}

// visible drops the candidates the actor may not read, one question for the whole list.
//
// The path ends on the entry rather than on its collection, which is what makes an entry shared
// with the actor individually visible here: a membership held anywhere along the path counts
// (domain-model.md §3.2).
func (h SuggestDuplicates) visible(
	ctx context.Context, actor appshared.ActorContext, candidates []workrepo.Neighbour,
) ([]workrepo.Neighbour, error) {
	paths := make([][]identity.Scope, 0, len(candidates))
	for _, candidate := range candidates {
		path := []identity.Scope{identity.TenantScope()}
		if !candidate.HubID.IsZero() {
			path = append(path, identity.HubScope(candidate.HubID))
		}
		paths = append(paths,
			append(path, identity.CollectionScope(candidate.CollectionID),
				identity.ItemScope(candidate.ItemID)))
	}

	allowed, err := h.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Action:     DuplicatesAskedAction,
		TokenScope: suggestionsRead,
		TargetType: suggestionTarget,
	}, paths)
	if err != nil {
		return nil, err
	}

	visible := make([]workrepo.Neighbour, 0, len(candidates))
	for index, candidate := range candidates {
		if index < len(allowed) && allowed[index] {
			visible = append(visible, candidate)
		}
	}
	return visible, nil
}

// duplicatesPayload is what the suggestion carries: identifiers and similarities, nearest first.
//
// Identifiers rather than titles, deliberately. A title in the payload would be a copy of somebody
// else's entry inside a record about this one - readable after the entry changed, after it was
// trashed, and by anybody who can read this suggestion. What a client renders it reads from the
// entries themselves, where the permission is asked again.
func duplicatesPayload(candidates []workrepo.Neighbour) map[string]any {
	found := make([]any, 0, len(candidates))
	for _, candidate := range candidates {
		found = append(found, map[string]any{
			"item_id": candidate.ItemID.String(),
			// Rounded to three places: it is a similarity a person reads as "how alike", and the
			// seventeen digits a float carries say nothing a reader can use.
			"similarity": rounded(candidate.Similarity),
		})
	}
	return map[string]any{"duplicates": found}
}

func rounded(value float64) float64 {
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(value, 'f', 3, 64), 64)
	if err != nil {
		return value
	}
	return rounded
}

// record writes the trail entry: that somebody asked, about what, and how many were found.
//
// No title, no similarity of any named entry, no identifier of a neighbour - what a duplicate is
// *of* is content, and an audit trail that listed them would be a copy of somebody's workspace in
// a place with a different retention (rule 10, ADR-0017).
func (h SuggestDuplicates) record(
	ctx context.Context, actor appshared.ActorContext, proposal domain.Suggestion, found int,
) error {
	return h.Cases.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: h.Cases.Clock.Now(),
		Action:     DuplicatesAskedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: suggestionTarget,
		TargetID:   proposal.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "model", Classification: audit.Open, To: proposal.Model},
			audit.Change{Field: "found", Classification: audit.Open, To: strconv.Itoa(found)},
		),
	})
}

// Descriptor is the catalogue entry - and the automation action SUGGEST_DUPLICATES with it, which
// is a rule that can ask "does this already exist" the moment an entry arrives.
func (h SuggestDuplicates) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SuggestDuplicatesName,
		Summary: "Answers which entries look like this one, from the embedding index the product " +
			"already maintains. No provider is asked and no budget is spent: two entries are near " +
			"each other or they are not. An entry's own parent and children are never among the " +
			"answers, and the answers are narrowed to what the caller may read. An installation " +
			"without pgvector, a workspace whose provider cannot embed, and an entry the " +
			"embedding pass has not reached each answer nothing at all rather than an error.",
		SideEffects: "Records a suggestion where something is near, and writes an audit entry. " +
			"Changes no entry and sends nothing anywhere.",
		TokenScope: suggestionsWrite,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry to look for duplicates of. A rule leaves this out and the " +
					"run supplies the entry it is about."},
		},
		Audit: usecase.AuditDeclaration{
			Action: DuplicatesAskedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Looking changes no entry, and what somebody does about a duplicate writes " +
				"its own history.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SuggestDuplicates) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	proposal, found, err := h.Execute(ctx, actor, itemID)
	if err != nil {
		return nil, err
	}
	if !found {
		// Nothing near, or nothing that could look. An empty answer rather than a refusal: the
		// caller asked a question with an answer, and the answer is "none".
		return usecase.Output{}, nil
	}
	return suggestionOutput(proposal), nil
}
