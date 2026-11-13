// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package search

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Invoker runs another use case by its name, as the reader.
type Invoker interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// maxEntryScan is the page of entries read for one search: the entry search's own largest page.
const maxEntryScan = 200

// Entries finds entries through the entry search as the reader (SearchItems), which ranks them by
// words and meaning and narrows them to what the reader may read; the vector is embedded once
// per search (MeaningMemo). With Tickets it finds the entries of the collections that act as a
// desk; without, the others where the question keeps them apart, else every entry. A query that is
// one ticket key finds that entry first (UC-WRK-18 check 10).
type Entries struct {
	Invoker    Invoker
	Containers workrepo.Containers
	UnitOfWork persistence.UnitOfWork
	// Keys reads the entry a ticket key names as the reader; nil finds none by its key.
	Keys KeyReader
	// Desks says which collections act as a desk; nil is none.
	Desks Desks
	// Tickets finds the entries of the desks.
	Tickets bool
}

// KeyReader reads the entry a ticket key names, where the reader may read it.
type KeyReader interface {
	EntryByKey(ctx context.Context, actor appshared.ActorContext, key searchrepo.Key, hubID shared.ID) (Hit, bool, error)
}

// Desks answers which of the collections act as a service desk.
type Desks interface {
	Desks(ctx context.Context, actor appshared.ActorContext, collections []work.Container) (map[shared.ID]bool, error)
}

// entryAnswer is the entry search's answer within one search, read once for both kinds.
type entryAnswer struct {
	read        bool
	hits        []Hit
	more        bool
	collections map[string]work.Container
	desks       map[shared.ID]bool
}

// KeyOf reads words that are one ticket key, "SA-12": a word that names a collection, a dash and
// a number.
func KeyOf(words string) (searchrepo.Key, bool) {
	word, number, found := strings.Cut(strings.TrimSpace(words), "-")
	if !found || word == "" || len(word) > 20 || number == "" || number[0] == '0' {
		return searchrepo.Key{}, false
	}
	for i, r := range word {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		digit := i > 0 && r >= '0' && r <= '9'
		if !letter && !digit {
			return searchrepo.Key{}, false
		}
	}
	n, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return searchrepo.Key{}, false
	}
	return searchrepo.Key{Word: word, Number: n}, true
}

// Find asks the entry search once per search and keeps the entries of its kind.
func (e Entries) Find(ctx context.Context, actor appshared.ActorContext, q Question) (Found, error) {
	answer := q.entries
	if answer == nil {
		answer = &entryAnswer{}
	}
	if !answer.read {
		if err := e.read(ctx, actor, q, answer); err != nil {
			return Found{}, err
		}
	}
	found := Found{More: answer.more}
	for _, hit := range answer.hits {
		if (q.Apart || e.Tickets) && answer.desks[hit.CollectionID] != e.Tickets {
			continue
		}
		found.Count++
		if len(found.Hits) < q.Size {
			found.Hits = append(found.Hits, hit)
		}
	}
	return found, nil
}

// read asks the entry search and the key, and reads the hits' collections and which act as a desk.
func (e Entries) read(ctx context.Context, actor appshared.ActorContext, q Question, answer *entryAnswer) error {
	answer.read = true
	answer.collections = map[string]work.Container{}
	if key, ok := KeyOf(q.Words); ok && e.Keys != nil {
		hit, found, err := e.Keys.EntryByKey(ctx, actor, key, q.HubID)
		if err != nil {
			return err
		}
		if found {
			answer.hits = append(answer.hits, hit)
		}
	}
	in := usecase.Input{"size": maxEntryScan, "include_archived": q.IncludeArchived, "include_trashed": q.IncludeTrashed}
	if strings.TrimSpace(q.Words) != "" {
		in["q"] = q.Words
	}
	if q.Filter != nil {
		in["filter"] = q.Filter
	}
	if q.Sort != nil {
		in["sort"] = q.Sort
	}
	if !q.HubID.IsZero() {
		in["container_id"] = q.HubID.String()
	}
	out, err := e.Invoker.Invoke(ctx, "SearchItems", actor, in)
	if err != nil && !errors.Is(err, shared.ErrForbidden) {
		return err
	}
	rows, _ := out["data"].([]usecase.Output)
	page, _ := out["page"].(map[string]any)
	answer.more, _ = page["has_more"].(bool)
	for _, row := range rows {
		id, _ := row["id"].(string)
		if len(answer.hits) > 0 && answer.hits[0].ID == shared.ID(id) {
			continue
		}
		title, _ := row["title"].(string)
		level, _ := row["type"].(string)
		collection, _ := row["collection_id"].(string)
		state := "OPEN"
		if done, _ := row["is_completed"].(bool); done {
			state = "DONE"
		}
		hit := Hit{ID: shared.ID(id), Title: title, Tile: level, State: state, CollectionID: shared.ID(collection), UpdatedAt: momentOf(row["updated_at"])}
		held, err := e.container(ctx, actor, collection, answer.collections)
		if err != nil {
			return err
		}
		hit.HubID = held.ParentID
		if number, ok := row["number"].(int64); ok && (held.Handle != "" || held.KeyPrefix != "") {
			hit.Key = mail.Key{Prefix: held.KeyPrefix, Handle: held.Handle, Number: number}.String()
		}
		answer.hits = append(answer.hits, hit)
	}
	return e.desks(ctx, actor, answer)
}

// desks reads which of the hits' collections act as a desk.
func (e Entries) desks(ctx context.Context, actor appshared.ActorContext, answer *entryAnswer) error {
	if e.Desks == nil || len(answer.hits) == 0 {
		return nil
	}
	if len(answer.hits) > 0 && answer.collections[answer.hits[0].CollectionID.String()].ID.IsZero() {
		if _, err := e.container(ctx, actor, answer.hits[0].CollectionID.String(), answer.collections); err != nil {
			return err
		}
	}
	collections := make([]work.Container, 0, len(answer.collections))
	for _, c := range answer.collections {
		if !c.ID.IsZero() {
			collections = append(collections, c)
		}
	}
	return e.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		answer.desks, err = e.Desks.Desks(ctx, actor, collections)
		return err
	})
}

// container reads an entry's collection once per search, for its hub and its ticket key.
func (e Entries) container(ctx context.Context, actor appshared.ActorContext, id string, held map[string]work.Container) (work.Container, error) {
	if c, read := held[id]; read || e.Containers == nil || id == "" {
		return c, nil
	}
	err := e.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		c, err := e.Containers.Find(ctx, shared.ID(id))
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		held[id] = c
		return nil
	})
	return held[id], err
}

func momentOf(v any) *time.Time {
	switch at := v.(type) {
	case time.Time:
		return &at
	case *time.Time:
		return at
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, at); err == nil {
			return &parsed
		}
	}
	return nil
}
