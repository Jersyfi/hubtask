// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package search is the one search over every kind of object (UC-WRK-18 check 9):
// entries, pages, whiteboards, goals, dashboards, inventory items, people, file names and
// comments in one answer, with a count per kind. Each kind is found by a Finder that asks that
// kind's own permission question of every hit, so the answer and its counts hold only what the
// reader may read (rule 2, Q-5).
package search

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// SearchAllName is the catalogue name.
const SearchAllName = "SearchAll"

// The kinds, in the order an answer groups them when none is asked.
const (
	KindEntry      = "ENTRY"
	KindPage       = "PAGE"
	KindWhiteboard = "WHITEBOARD"
	KindGoal       = "GOAL"
	KindDashboard  = "DASHBOARD"
	KindAsset      = "ASSET"
	KindPerson     = "PERSON"
	KindFile       = "FILE"
	KindComment    = "COMMENT"
)

// Kinds is every kind, in the default order.
var Kinds = []string{KindEntry, KindPage, KindWhiteboard, KindGoal, KindDashboard, KindAsset, KindPerson, KindFile, KindComment}

const (
	maxWords    = 200
	maxTerms    = 8
	defaultSize = 5
	maxSize     = 25
	// Scan is how many candidates of a kind are read before the reader's permission narrows
	// them; a kind with Scan candidates says its count may be more.
	Scan       = 100
	readAction = audit.Action("search.read")
	target     = "search"
)

// Question is what every finder is asked.
type Question struct {
	// Words is what was typed; Terms the words lowered, every one of which a name must hold.
	Words string
	Terms []string
	HubID shared.ID
	Size  int
}

// Ask is the store's question for the candidates of a kind.
func (q Question) Ask() searchrepo.Ask {
	return searchrepo.Ask{Terms: q.Terms, HubID: q.HubID, Batch: Scan}
}

// Hit is one object as a hit row draws it.
type Hit struct {
	Kind, Title, Tile, Snippet, State, Key string
	ID, HubID, CollectionID                shared.ID
	UpdatedAt                              *time.Time
	OnKind                                 string
	OnID                                   shared.ID
}

// Found is one kind's answer: at most Size hits, how many the reader may read and whether there
// may be more.
type Found struct {
	Hits  []Hit
	Count int
	More  bool
}

// Finder finds one kind as the reader. A reader who may read none of a kind - a token without
// its scope included - gets nothing rather than a refusal: a search narrows, it does not refuse.
type Finder interface {
	Find(ctx context.Context, actor appshared.ActorContext, q Question) (Found, error)
}

// Narrowed is the answer of a kind found by candidates and narrowed by a permission per row.
func Narrowed(rows []searchrepo.Row, allowed []bool, size int, hit func(searchrepo.Row) Hit) Found {
	found := Found{More: len(rows) >= Scan}
	for i, row := range rows {
		if i >= len(allowed) || !allowed[i] {
			continue
		}
		found.Count++
		if len(found.Hits) < size {
			found.Hits = append(found.Hits, hit(row))
		}
	}
	return found
}

// Within reads candidates inside a read-only transaction.
func Within(
	ctx context.Context, uow persistence.UnitOfWork, actor appshared.ActorContext,
	read func(context.Context, searchrepo.Ask) ([]searchrepo.Row, error), ask searchrepo.Ask,
) ([]searchrepo.Row, error) {
	var rows []searchrepo.Row
	err := uow.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		rows, err = read(ctx, ask)
		return err
	})
	return rows, err
}

// Snippet cuts the passage around the first word found, the word between « and », as a page's
// passage is marked.
func Snippet(text string, terms []string) string {
	text = strings.Join(strings.Fields(text), " ")
	lowered := strings.ToLower(text)
	if len(terms) == 0 || len(lowered) != len(text) {
		return cut(text, 0, 160)
	}
	at := strings.Index(lowered, terms[0])
	if at < 0 {
		return cut(text, 0, 160)
	}
	end := at + len(terms[0])
	return strings.TrimSpace(cut(text, max(0, at-60), at) + "«" + text[at:end] + "»" + cut(text, end, end+80))
}

func cut(text string, from, to int) string {
	to = min(to, len(text))
	for from > 0 && !utf8.RuneStart(text[from]) {
		from--
	}
	for to < len(text) && !utf8.RuneStart(text[to]) {
		to++
	}
	return text[from:to]
}

// SearchAll is the use case.
type SearchAll struct {
	Finders map[string]Finder
}

// SearchAllQuery is the input, typed.
type SearchAllQuery struct {
	Words string
	Kinds []string
	HubID shared.ID
	Size  int
}

// Answer is the hits grouped by kind, and the counts.
type Answer struct {
	Hits   []Hit
	Facets []Facet
}

// Facet is one kind's count.
type Facet struct {
	Kind  string
	Count int
	More  bool
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

// Terms reads the words a name must hold: lowered, without quotes, a leading minus's word left
// out of the names (an entry and a page still read it), each once, at most eight.
func Terms(words string) ([]string, error) {
	var terms []string
	seen := map[string]bool{}
	for _, field := range strings.Fields(strings.ToLower(strings.ReplaceAll(words, `"`, " "))) {
		if strings.HasPrefix(field, "-") || field == "or" || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
	}
	if len(terms) > maxTerms {
		return nil, invalid("/q", "search.too_many_words")
	}
	return terms, nil
}

// Execute asks every kind asked for, in order.
func (h SearchAll) Execute(ctx context.Context, actor appshared.ActorContext, query SearchAllQuery) (Answer, error) {
	words := strings.TrimSpace(query.Words)
	if words == "" || utf8.RuneCountInString(words) > maxWords {
		return Answer{}, invalid("/q", "search.words_required")
	}
	terms, err := Terms(words)
	if err != nil {
		return Answer{}, err
	}
	kinds := query.Kinds
	if len(kinds) == 0 {
		kinds = Kinds
	}
	size := query.Size
	if size <= 0 {
		size = defaultSize
	}
	if size > maxSize {
		return Answer{}, invalid("/size", "search.size_out_of_range")
	}
	answer := Answer{Hits: []Hit{}, Facets: []Facet{}}
	asked := map[string]bool{}
	for i, kind := range kinds {
		finder, known := h.Finders[kind]
		if !known && !isKind(kind) {
			return Answer{}, invalid("/kinds/"+strconv.Itoa(i), "search.kind_unknown")
		}
		if asked[kind] {
			continue
		}
		asked[kind] = true
		found := Found{}
		// A kind without a finder is a module not running here: it finds nothing.
		if known && (kind != KindPerson || query.HubID.IsZero()) && (len(terms) > 0 || kind == KindEntry || kind == KindPage) {
			if found, err = finder.Find(ctx, actor, Question{Words: words, Terms: terms, HubID: query.HubID, Size: size}); err != nil {
				return Answer{}, err
			}
		}
		for _, hit := range found.Hits {
			hit.Kind = kind
			answer.Hits = append(answer.Hits, hit)
		}
		answer.Facets = append(answer.Facets, Facet{Kind: kind, Count: found.Count, More: found.More})
	}
	return answer, nil
}

func isKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// HitOutput is a hit as the contract names it (schema SearchAllHit).
func HitOutput(hit Hit) usecase.Output {
	text := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	id := func(v shared.ID) any {
		if v.IsZero() {
			return nil
		}
		return v.String()
	}
	out := usecase.Output{
		"kind": hit.Kind, "id": hit.ID.String(), "title": hit.Title, "hub_id": id(hit.HubID),
		"collection_id": id(hit.CollectionID), "tile": text(hit.Tile), "snippet": text(hit.Snippet),
		"state": text(hit.State), "key": text(hit.Key), "updated_at": nil, "on": nil,
	}
	if hit.UpdatedAt != nil {
		out["updated_at"] = hit.UpdatedAt.UTC()
	}
	if hit.OnKind != "" {
		out["on"] = map[string]any{"kind": hit.OnKind, "id": hit.OnID.String()}
	}
	return out
}

// Descriptor is the catalogue entry.
func (h SearchAll) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       SearchAllName,
		Permission: service.PermissionRead,
		Summary: "One search over every kind of object the caller may read - entries, pages, " +
			"whiteboards, goals, dashboards, inventory items, people, file names and comments - " +
			"each hit with its kind, title, place, tile, the passage that matched where there is " +
			"one and its state, grouped by kind and best first within a kind, at most size per " +
			"kind; facets counts each kind's hits the caller may read, a count with more being a " +
			"lower bound. People are found by their display name only, files by their name only. " +
			"Among equally good pages a verified one comes first.",
		SideEffects: "None. Reads only.",
		TokenScope:  "items:read",
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "q", Kind: usecase.KindString, Required: true, Description: "The words, at most 200 characters; every word must occur."},
			{Name: "kinds", Kind: usecase.KindList, Description: "The kinds to search, in the order the answer groups them: ENTRY, PAGE, WHITEBOARD, GOAL, DASHBOARD, ASSET, PERSON, FILE, COMMENT. Omitted is every kind."},
			{Name: "hub_id", Kind: usecase.KindID, Description: "Only this hub's objects; people stand in no hub and are then left out."},
			{Name: "size", Kind: usecase.KindInt, Description: "At most this many hits per kind, 1 to 25; 5 when omitted."},
		},
		Audit: usecase.AuditDeclaration{Action: readAction, TargetType: target, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			var kinds []string
			if list, ok := in["kinds"].([]any); ok {
				for i, k := range list {
					text, ok := k.(string)
					if !ok {
						return nil, invalid("/kinds/"+strconv.Itoa(i), "search.kind_unknown")
					}
					kinds = append(kinds, text)
				}
			}
			answer, err := h.Execute(ctx, actor, SearchAllQuery{Words: in.String("q"), Kinds: kinds, HubID: hub, Size: in.Int("size")})
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(answer.Hits))
			for _, hit := range answer.Hits {
				data = append(data, HitOutput(hit))
			}
			facets := make([]usecase.Output, 0, len(answer.Facets))
			for _, f := range answer.Facets {
				facets = append(facets, usecase.Output{"kind": f.Kind, "count": f.Count, "more": f.More})
			}
			return usecase.Output{"data": data, "facets": facets}, nil
		}),
	}
}
