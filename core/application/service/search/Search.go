// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package search is the one search over every kind of object (UC-WRK-18 check 9):
// entries, pages, whiteboards, goals, dashboards, inventory items, people, file names,
// comments, portfolios, releases, sprints and posts in one answer, with a count per kind. Each kind is found by a Finder that asks that
// kind's own permission question of every hit, so the answer and its counts hold only what the
// reader may read (rule 2, Q-5).
package search

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// SearchAllName is the catalogue name.
const SearchAllName = "SearchAll"

// The kinds, in the order an answer groups them when none is asked.
const (
	KindEntry      = "ENTRY"
	KindTicket     = "TICKET"
	KindPage       = "PAGE"
	KindWhiteboard = "WHITEBOARD"
	KindGoal       = "GOAL"
	KindDashboard  = "DASHBOARD"
	KindAsset      = "ASSET"
	KindPerson     = "PERSON"
	KindFile       = "FILE"
	KindComment    = "COMMENT"
	KindPortfolio  = "PORTFOLIO"
	KindRelease    = "RELEASE"
	KindSprint     = "SPRINT"
	// KindPost is a published post, found as itself and never as a page: its audience and moment
	// are its own question (B-6).
	KindPost = "POST"
)

// Kinds is every kind, in the default order.
var Kinds = []string{
	KindEntry, KindTicket, KindPage, KindWhiteboard, KindGoal, KindDashboard, KindAsset, KindPerson, KindFile, KindComment,
	KindPortfolio, KindRelease, KindSprint, KindPost,
}

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
	// Words are the query's free words; Terms the words lowered, every one of which a name must
	// hold. Either may be empty where the query narrows by keys alone.
	Words string
	Terms []string
	HubID shared.ID
	Size  int
	// Conditions are the keys of the one kind the query asks about (asset., page., goal.), `@me`
	// already the reader's identifier.
	Conditions []view.KindCondition
	// Filter and Sort are the entry filter and order where the query's keys are entries'; the two
	// flags widen the entries read.
	Filter          any
	Sort            []any
	IncludeArchived bool
	IncludeTrashed  bool
	// Apart keeps tickets out of the entries: TICKET is asked beside ENTRY.
	Apart bool
	// entries is the entry search's answer, read once for ENTRY and TICKET.
	entries *entryAnswer
	// modules answers the modules on at each hub once per search.
	modules *modulesAt
}

// Modules answers the modules on at a hub as the module gate resolves them (UC-INS-21 check 5).
type Modules interface {
	On(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (module.Set, error)
}

type modulesAt struct {
	modules Modules
	at      map[shared.ID]module.Set
}

// On reports whether the module is on at the hub, so a finder leaves out what a hub has switched
// off, as every other door refuses it with module.off; a hub that is gone has nothing on. A search
// asked without the switches - a finder on its own - takes every module as on.
func (q Question) On(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, key module.Key) (bool, error) {
	if q.modules == nil || q.modules.modules == nil {
		return true, nil
	}
	set, read := q.modules.at[hubID]
	if !read {
		var err error
		set, err = q.modules.modules.On(ctx, actor, hubID)
		if errors.Is(err, shared.ErrNotFound) {
			set, err = module.Set{}, nil
		}
		if err != nil {
			return false, err
		}
		q.modules.at[hubID] = set
	}
	return set.Has(key), nil
}

// Ask is the store's question for the candidates of a kind.
func (q Question) Ask() searchrepo.Ask {
	return searchrepo.Ask{Terms: q.Terms, HubID: q.HubID, Conditions: q.Conditions, Batch: Scan}
}

// Narrows reports whether the question asks anything of a kind found by its names: a word, or a
// key of the kind.
func (q Question) Narrows() bool { return len(q.Terms) > 0 || len(q.Conditions) > 0 }

// Hit is one object as a hit row draws it.
type Hit struct {
	Kind, Title, Tile, Snippet, State, Key string
	ID, HubID, CollectionID                shared.ID
	UpdatedAt                              *time.Time
	// CreatedAt and AuthorID say when and by whom it was made, where its kind keeps both.
	CreatedAt *time.Time
	AuthorID  shared.ID
	OnKind    string
	// Status is an entry's workflow status key, empty where it stands in none.
	Status string
	OnID   shared.ID
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
	to, from = min(to, len(text)), min(from, len(text))
	for from > 0 && from < len(text) && !utf8.RuneStart(text[from]) {
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
	// Modules leaves out a hit at a hub where its kind's module is off; nil leaves out nothing.
	Modules Modules
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

// objectKinds are the kinds `type:` names that a search reads, by the query's word.
var objectKinds = map[string]string{
	"task": KindEntry, "workpackage": KindEntry, "activity": KindEntry, "ticket": KindTicket,
	"page": KindPage, "whiteboard": KindWhiteboard, "goal": KindGoal, "dashboard": KindDashboard,
	"asset": KindAsset, "person": KindPerson, "file": KindFile, "comment": KindComment, "post": KindPost,
}

// filterKinds are the kinds whose keys a query may narrow by.
var filterKinds = map[string]string{
	view.ObjectEntry: KindEntry, view.ObjectAsset: KindAsset, view.ObjectPage: KindPage, view.ObjectGoal: KindGoal,
}

// read is the query `q` as the search asks it: the question every finder gets and the one kind
// its keys keep the search to ("" for every kind); `type:` chooses kinds as well.
func read(actor appshared.ActorContext, text string) (Question, string, map[string]bool, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxWords {
		return Question{}, "", nil, invalid("/q", "search.words_required")
	}
	query, err := view.ParseQuery(text, "/q")
	if err != nil {
		return Question{}, "", nil, err
	}
	if strings.TrimSpace(query.Words) == "" && query.Filter == nil {
		return Question{}, "", nil, invalid("/q", "search.words_required")
	}
	terms, err := Terms(query.Words)
	if err != nil {
		return Question{}, "", nil, err
	}
	keys, err := query.KindFilter("/q")
	if err != nil {
		return Question{}, "", nil, err
	}
	q := Question{Words: query.Words, Terms: terms, IncludeArchived: query.IncludeArchived, IncludeTrashed: query.IncludeTrashed}
	if keys.Kind == view.ObjectEntry {
		q.Filter, q.Sort = query.Filter, query.Sort
	}
	for _, condition := range keys.Conditions {
		values := make([]string, 0, len(condition.Values))
		for _, value := range condition.Values {
			if value == "@me" {
				if actor.AccountID.IsZero() {
					return Question{}, "", nil, shared.ErrValidation.WithDetail("query.placeholder_needs_actor").
						WithParams(map[string]string{"placeholder": "@me"}).
						WithFields(shared.FieldError{Path: "/q", Code: "query.placeholder_needs_actor"})
				}
				value = actor.AccountID.String()
			}
			values = append(values, value)
		}
		condition.Values = values
		q.Conditions = append(q.Conditions, condition)
	}
	var typed map[string]bool
	if len(query.Kinds) > 0 {
		typed = map[string]bool{}
		for _, kind := range query.Kinds {
			if k, ok := objectKinds[kind]; ok {
				typed[k] = true
			}
		}
	}
	return q, filterKinds[keys.Kind], typed, nil
}

// Execute asks every kind asked for, in order. A key of one kind keeps the search to that kind:
// the other kinds are counted as none.
func (h SearchAll) Execute(ctx context.Context, actor appshared.ActorContext, query SearchAllQuery) (Answer, error) {
	question, only, typed, err := read(actor, query.Words)
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
	question.HubID, question.Size = query.HubID, size
	question.entries = &entryAnswer{}
	question.modules = &modulesAt{modules: h.Modules, at: map[shared.ID]module.Set{}}
	_, tickets := h.Finders[KindTicket]
	question.Apart = tickets && slices.Contains(kinds, KindTicket)
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
		if known && h.asks(kind, question, only, typed) {
			if found, err = finder.Find(ctx, actor, question); err != nil {
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

// asks reports whether a kind is searched: kept by the query's keys and its `type:`, a person
// and a portfolio only outside a hub, and asked something - entries, pages and posts read words a
// name does not; a post has no keys of its own, so words alone ask it.
func (h SearchAll) asks(kind string, q Question, only string, typed map[string]bool) bool {
	switch {
	case only != "" && kind != only && (only != KindEntry || kind != KindTicket),
		typed != nil && !typed[kind], (kind == KindPerson || kind == KindPortfolio) && !q.HubID.IsZero():
		return false
	case kind == KindEntry, kind == KindTicket:
		return strings.TrimSpace(q.Words) != "" || q.Filter != nil
	case kind == KindPage:
		return strings.TrimSpace(q.Words) != "" || len(q.Conditions) > 0
	case kind == KindPost:
		return strings.TrimSpace(q.Words) != ""
	}
	return q.Narrows()
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
		"state": text(hit.State), "status": text(hit.Status), "key": text(hit.Key), "updated_at": nil, "created_at": nil,
		"author_id": id(hit.AuthorID), "on": nil,
	}
	if hit.UpdatedAt != nil {
		out["updated_at"] = hit.UpdatedAt.UTC()
	}
	if hit.CreatedAt != nil {
		out["created_at"] = hit.CreatedAt.UTC()
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
			"whiteboards, goals, dashboards, inventory items, people, file names, comments, portfolios, " +
			"releases, sprints and posts - " +
			"tickets apart from the other entries where TICKET is asked, a ticket key finding its entry - " +
			"each hit with its kind, title, place, tile, the passage that matched where there is " +
			"one and its state, grouped by kind and best first within a kind, at most size per " +
			"kind; facets counts each kind's hits the caller may read, a count with more being a " +
			"lower bound. People and portfolios stand in no hub and are left out where hub_id narrows. " +
			"People are found by their display name only, files by their name only; " +
			"files and comments on entries, pages and whiteboards, where the caller reads what they are on. " +
			"Among equally good pages a verified one comes first. A post is found by its title and body once " +
			"it is published, by a person it is for alone - never as a page, never a draft or a scheduled one, never in a hub " +
			"where posts are switched off. q is a Hubtask query: its free " +
			"words are searched, its keys narrow - a key of one kind (asset.class, asset.stage, " +
			"asset.owner, asset.location, asset.tag, page.verified, page.owner, goal.state) keeps the " +
			"search to that kind, a key of entries to entries, type: chooses kinds; keys alone " +
			"need no words. GET /query/keys lists the keys.",
		SideEffects: "None. Reads only.",
		TokenScope:  "items:read",
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "q", Kind: usecase.KindString, Required: true, Description: "The words and keys as one Hubtask query, at most 200 characters; every free word must occur, every key narrows."},
			{Name: "kinds", Kind: usecase.KindList, Description: "The kinds to search, in the order the answer groups them: ENTRY, TICKET, PAGE, WHITEBOARD, GOAL, DASHBOARD, ASSET, PERSON, FILE, COMMENT, PORTFOLIO, RELEASE, SPRINT, POST. Omitted is every kind. TICKET is the entries of the collections that act as a desk; beside it, ENTRY holds the other entries."},
			{Name: "hub_id", Kind: usecase.KindID, Description: "Only this hub's objects; people and portfolios stand in no hub and are then left out."},
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

// Together is one kind found in several places - files and comments on entries, pages and
// boards - each by its own finder and read rule: the counts add up, and the hits are the latest
// first, at most Size.
type Together []Finder

// Find asks every finder.
func (t Together) Find(ctx context.Context, actor appshared.ActorContext, q Question) (Found, error) {
	var all Found
	for _, finder := range t {
		found, err := finder.Find(ctx, actor, q)
		if err != nil {
			return Found{}, err
		}
		all.Count += found.Count
		all.More = all.More || found.More
		all.Hits = append(all.Hits, found.Hits...)
	}
	sort.SliceStable(all.Hits, func(i, j int) bool {
		a, b := all.Hits[i].UpdatedAt, all.Hits[j].UpdatedAt
		return a != nil && (b == nil || a.After(*b))
	})
	all.Hits = all.Hits[:min(len(all.Hits), q.Size)]
	return all, nil
}
