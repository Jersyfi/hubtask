// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package search

import (
	"context"
	"errors"
	"strings"
	"time"

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
// per search (MeaningMemo).
type Entries struct {
	Invoker    Invoker
	Containers workrepo.Containers
	UnitOfWork persistence.UnitOfWork
}

// Find asks one page.
func (e Entries) Find(ctx context.Context, actor appshared.ActorContext, q Question) (Found, error) {
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
	if err != nil {
		if errors.Is(err, shared.ErrForbidden) {
			return Found{}, nil
		}
		return Found{}, err
	}
	rows, _ := out["data"].([]usecase.Output)
	page, _ := out["page"].(map[string]any)
	more, _ := page["has_more"].(bool)
	found := Found{Count: len(rows), More: more}
	collections := map[string]work.Container{}
	for _, row := range rows[:min(len(rows), q.Size)] {
		id, _ := row["id"].(string)
		title, _ := row["title"].(string)
		level, _ := row["type"].(string)
		collection, _ := row["collection_id"].(string)
		state := "OPEN"
		if done, _ := row["is_completed"].(bool); done {
			state = "DONE"
		}
		hit := Hit{ID: shared.ID(id), Title: title, Tile: level, State: state, CollectionID: shared.ID(collection), UpdatedAt: momentOf(row["updated_at"])}
		held, err := e.container(ctx, actor, collection, collections)
		if err != nil {
			return Found{}, err
		}
		hit.HubID = held.ParentID
		if number, ok := row["number"].(int64); ok && held.Handle != "" {
			hit.Key = mail.Key{Prefix: held.KeyPrefix, Handle: held.Handle, Number: number}.String()
		}
		found.Hits = append(found.Hits, hit)
	}
	return found, nil
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
