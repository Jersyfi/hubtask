// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// FindDuplicatesByTitleName is the catalogue name.
const FindDuplicatesByTitleName = "FindDuplicatesByTitle"

// duplicatesByTitleAction names the read where a refusal of the token's scope is recorded. A read
// writes no audit entry of its own, as a search writes none.
const duplicatesByTitleAction audit.Action = "ai.duplicates_by_title_read"

// TitleMeaning turns typed words into a vector, or answers nothing: the search's own embedding of
// what somebody typed (work.SearchMeaning), declared here because the application layer may not
// import a sibling service.
type TitleMeaning interface {
	Of(ctx context.Context, actor appshared.ActorContext, words string) ([]float32, string, error)
}

// VectorNeighbours is the slice of the embedding store this read needs.
type VectorNeighbours interface {
	NearVector(ctx context.Context, vector []float32, model string, floor float64, limit int) ([]workrepo.Neighbour, error)
}

// FindDuplicatesByTitle answers which entries look like a title that is not an entry yet
// (UC-AI-04 check 7): what somebody creating "Book the tiler" should learn is already there.
//
// **A read, not a suggestion.** Nothing is recorded - there is no entry yet to record a suggestion
// against - and nothing is audited, as a search is not. The words reach the provider the way a
// search's words do, through the same embedding with the same consent, the same rule for people who
// keep their content out and the same short wait; every way of not getting a vector is an empty
// answer, never an error.
//
// **No question is asked at the workspace.** A role held on one hub is enough to create there, and
// a workspace-scope question would refuse that person the lookalikes they may read. What decides is
// the token's scope, and then the read permission along each candidate's own path - which is also
// what keeps an entry in a private hub from somebody outside it (T-04).
//
// Titles travel in the answer, unlike in a stored DUPLICATES suggestion: the answer goes to the
// asker alone, after that narrowing, and is stored nowhere - a search hit carries its title for the
// same reason.
type FindDuplicatesByTitle struct {
	Meaning    TitleMeaning
	Neighbours VectorNeighbours
	Reader     Readers
	Items      ItemFinder
	UnitOfWork persistence.UnitOfWork
	// Floor is the similarity a lookalike has to reach. Zero takes DefaultDuplicateFloor.
	Floor float64
	// Limit bounds the answer. Zero takes the default every duplicates answer has.
	Limit int
}

// ItemFinder reads one entry the caller was already found to be allowed to read: the title is all
// that is taken from it.
type ItemFinder interface {
	Find(ctx context.Context, id shared.ID) (work.WorkItem, error)
}

// Lookalike is one entry near the words.
type Lookalike struct {
	ItemID       shared.ID
	CollectionID shared.ID
	Title        string
	Similarity   float64
}

// Execute answers the lookalikes, nearest first.
func (h FindDuplicatesByTitle) Execute(
	ctx context.Context, actor appshared.ActorContext, title string,
) ([]Lookalike, error) {
	words := strings.TrimSpace(title)
	switch {
	case words == "":
		return nil, shared.ErrValidation.WithDetail("items.title_empty").
			WithFields(shared.FieldError{Path: "/title", Code: "items.title_empty"})
	case utf8.RuneCountInString(words) > work.MaxItemTitleLength:
		return nil, shared.ErrValidation.WithDetail("items.title_too_long").
			WithParams(map[string]string{"maximum": "500"}).
			WithFields(shared.FieldError{Path: "/title", Code: "items.title_too_long"})
	}

	// The token's scope first, and before the provider: a token that may not read entries is
	// refused before anything is spent. No path yet, so no membership is asked.
	request := access.Request{
		Permission: service.PermissionRead,
		Action:     duplicatesByTitleAction,
		TokenScope: suggestionsRead,
		TargetType: suggestionTarget,
	}
	if _, err := h.Reader.Permitted(ctx, actor, request, nil); err != nil {
		return nil, err
	}

	if h.Meaning == nil || h.Neighbours == nil {
		return []Lookalike{}, nil
	}
	vector, model, err := h.Meaning.Of(ctx, actor, words)
	if err != nil {
		return nil, err
	}
	if len(vector) == 0 {
		return []Lookalike{}, nil
	}

	var candidates []workrepo.Neighbour
	if err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		candidates, err = h.Neighbours.NearVector(ctx, vector, model, h.floor(), h.limit())
		return err
	}); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []Lookalike{}, nil
	}

	paths := make([][]identity.Scope, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, pathOfNeighbour(candidate))
	}
	allowed, err := h.Reader.Permitted(ctx, actor, request, paths)
	if err != nil {
		return nil, err
	}
	visible := make([]workrepo.Neighbour, 0, len(candidates))
	ids := make([]shared.ID, 0, len(candidates))
	for index, candidate := range candidates {
		if index < len(allowed) && allowed[index] {
			visible = append(visible, candidate)
			ids = append(ids, candidate.ItemID)
		}
	}
	if len(visible) == 0 {
		return []Lookalike{}, nil
	}

	titles := make(map[shared.ID]string, len(ids))
	if err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		for _, id := range ids {
			item, err := h.Items.Find(ctx, id)
			if errors.Is(err, shared.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if !item.IsTrashed() {
				titles[id] = item.Title
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	found := make([]Lookalike, 0, len(visible))
	for _, candidate := range visible {
		title, known := titles[candidate.ItemID]
		if !known {
			// Trashed between the two reads: it is no longer anything to look like.
			continue
		}
		found = append(found, Lookalike{
			ItemID: candidate.ItemID, CollectionID: candidate.CollectionID,
			Title: title, Similarity: rounded(candidate.Similarity),
		})
	}
	return found, nil
}

func (h FindDuplicatesByTitle) floor() float64 {
	if h.Floor <= 0 {
		return DefaultDuplicateFloor
	}
	return h.Floor
}

func (h FindDuplicatesByTitle) limit() int {
	if h.Limit <= 0 {
		return defaultDuplicateLimit
	}
	return h.Limit
}

// pathOfNeighbour is the path a candidate's read permission is asked along, ending on the entry so
// that an entry shared with the actor on its own counts (domain-model.md §3.2).
func pathOfNeighbour(candidate workrepo.Neighbour) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !candidate.HubID.IsZero() {
		path = append(path, identity.HubScope(candidate.HubID))
	}
	return append(path, identity.CollectionScope(candidate.CollectionID), identity.ItemScope(candidate.ItemID))
}

// Descriptor registers the read in all three channels.
func (h FindDuplicatesByTitle) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       FindDuplicatesByTitleName,
		Permission: service.PermissionRead,
		Summary: "Answers the entries whose meaning sits near the words of a title that is not an " +
			"entry yet - at most five, nearest first, at or above the duplicate threshold, each " +
			"narrowed to what the caller may read along its own path. The words are embedded the " +
			"way a search's are; without a store, a provider that embeds, consent, or an answer in " +
			"time, the answer is empty rather than an error.",
		SideEffects: "Sends the words to the workspace's AI provider for their embedding, as a search " +
			"does. Stores nothing and writes no audit entry.",
		TokenScope: suggestionsRead,
		ReadOnly:   true,
		Input: []usecase.Field{
			{Name: "title", Kind: usecase.KindString, Required: true,
				Description: "The words of the title being typed."},
		},
		Audit: usecase.AuditDeclaration{
			Action: duplicatesByTitleAction, TargetType: suggestionTarget,
			Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A read: there is no entry yet, and nothing changes.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h FindDuplicatesByTitle) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	found, err := h.Execute(ctx, actor, in.String("title"))
	if err != nil {
		return nil, err
	}
	items := make([]usecase.Output, 0, len(found))
	for _, lookalike := range found {
		items = append(items, usecase.Output{
			"item_id":       lookalike.ItemID.String(),
			"collection_id": lookalike.CollectionID.String(),
			"title":         lookalike.Title,
			"similarity":    lookalike.Similarity,
		})
	}
	return usecase.Output{"items": items}, nil
}
