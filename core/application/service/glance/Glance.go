// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package glance is the two reads another product's dashboard and search box show Hubtask in
// (UC-INT-20): the reader's open entries, and one search across entries and pages. Both run the
// searches of the registry as the reader, so they answer exactly what the reader's roles and the
// token's scopes let those searches answer, bounded and in a small shape with absolute links.
package glance

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
	domainwork "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The use case names.
const (
	ListMyOpenEntriesName = "ListMyOpenEntries"
	SearchEverywhereName  = "SearchEverywhere"
)

const (
	searchItems = "SearchItems"
	searchPages = "SearchPages"
	readScope   = "items:read"
	readAction  = audit.Action("glance.read")
	target      = "glance"
	// MaxOpenEntries and MaxFound bound one answer; a page of the inner search may be short,
	// so at most maxRounds of them are read to fill it.
	MaxOpenEntries     = 25
	defaultOpenEntries = 7
	MaxFound           = 25
	defaultFound       = 10
	maxRounds          = 4
)

// Invoker runs another use case by its name, as the reader.
type Invoker interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Addresses answers the workspace's address, which the links begin with.
type Addresses interface {
	Of(ctx context.Context) (string, error)
}

// Glance is what both reads share.
type Glance struct {
	Invoker    Invoker
	Addresses  Addresses
	Containers workrepo.Containers
	UnitOfWork persistence.UnitOfWork
}

// Found is one line of an answer: an entry or a page.
type Found struct {
	Kind      string // ENTRY or PAGE
	ID        string
	Title     string
	Key       string // an entry's ticket key, empty without a handle
	DueAt     any    // an entry's due moment as the entry answers it, nil for none
	Excerpt   string // a page's passage that matched
	UpdatedAt any
	Link      string
}

func bounded(size, fallback, most int) int {
	if size <= 0 {
		return fallback
	}
	return min(size, most)
}

// entries runs SearchItems until size entries are gathered or nothing more is there.
func (g Glance) entries(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input, size int,
) ([]usecase.Output, string, error) {
	var rows []usecase.Output
	cursor, _ := in["cursor"].(string)
	for round := 0; round < maxRounds && len(rows) < size; round++ {
		ask := usecase.Input{}
		for k, v := range in {
			ask[k] = v
		}
		ask["size"], ask["cursor"] = size-len(rows), nil
		if cursor != "" {
			ask["cursor"] = cursor
		}
		out, err := g.Invoker.Invoke(ctx, searchItems, actor, ask)
		if err != nil {
			return nil, "", err
		}
		data, _ := out["data"].([]usecase.Output)
		rows = append(rows, data...)
		page, _ := out["page"].(map[string]any)
		next, _ := page["next_cursor"].(string)
		if more, _ := page["has_more"].(bool); !more || next == "" {
			return rows, "", nil
		}
		cursor = next
	}
	return rows, cursor, nil
}

// lines turns the entries into lines with their keys and links, and answers the workspace's
// address the links begin with - read inside the transaction, which the address needs in multi mode.
func (g Glance) lines(ctx context.Context, actor appshared.ActorContext, rows []usecase.Output) ([]Found, string, error) {
	var base string
	collections := map[string]domainwork.Container{}
	err := g.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if base, err = g.Addresses.Of(ctx); err != nil {
			return err
		}
		base = strings.TrimSuffix(base, "/")
		for _, row := range rows {
			id, _ := row["collection_id"].(string)
			if _, read := collections[id]; read || id == "" {
				continue
			}
			collection, err := g.Containers.Find(ctx, shared.ID(id))
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			collections[id] = collection
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	out := make([]Found, 0, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		title, _ := row["title"].(string)
		line := Found{Kind: "ENTRY", ID: id, Title: title, DueAt: row["due_at"], UpdatedAt: row["updated_at"],
			Link: base + "/items/" + id}
		collection, _ := row["collection_id"].(string)
		if number, ok := row["number"].(int64); ok && collections[collection].Handle != "" {
			held := collections[collection]
			line.Key = mail.Key{Prefix: held.KeyPrefix, Handle: held.Handle, Number: number}.String()
		}
		out = append(out, line)
	}
	return out, base, nil
}

// FoundOutput is a line as the contract names it (schema GlanceLine).
func FoundOutput(line Found) usecase.Output {
	out := usecase.Output{
		"kind": line.Kind, "id": line.ID, "title": line.Title, "key": nil, "due_at": line.DueAt,
		"excerpt": nil, "updated_at": line.UpdatedAt, "link": line.Link,
	}
	if line.Key != "" {
		out["key"] = line.Key
	}
	if line.Excerpt != "" {
		out["excerpt"] = line.Excerpt
	}
	return out
}

func answer(lines []Found, cursor string) usecase.Output {
	data := make([]usecase.Output, 0, len(lines))
	for _, line := range lines {
		data = append(data, FoundOutput(line))
	}
	out := usecase.Output{"data": data, "next_cursor": nil}
	if cursor != "" {
		out["next_cursor"] = cursor
	}
	return out
}

// ListMyOpenEntries answers the reader's open entries, due first (UC-INT-20 check 1).
type ListMyOpenEntries struct{ Glance Glance }

// Execute answers one page.
func (h ListMyOpenEntries) Execute(ctx context.Context, actor appshared.ActorContext, size int, cursor string) ([]Found, string, error) {
	size = bounded(size, defaultOpenEntries, MaxOpenEntries)
	in := usecase.Input{
		"filter": map[string]any{"op": "AND", "nodes": []any{
			map[string]any{"op": "EQ", "field": "assignee_id", "value": "@me"},
			map[string]any{"op": "EQ", "field": "is_completed", "value": false},
		}},
		"sort": []any{map[string]any{"field": "due_at", "direction": "ASC"}},
	}
	if cursor != "" {
		in["cursor"] = cursor
	}
	rows, next, err := h.Glance.entries(ctx, actor, in, size)
	if err != nil {
		return nil, "", err
	}
	lines, _, err := h.Glance.lines(ctx, actor, rows)
	return lines, next, err
}

// Descriptor is the catalogue entry.
func (h ListMyOpenEntries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListMyOpenEntriesName,
		Permission: service.PermissionRead,
		Summary: "The open entries assigned to the caller anywhere in the workspace, due first, each " +
			"with its title, ticket key, due moment and a link - what another product's dashboard " +
			"shows of Hubtask. Read as the caller, so only what they may see.",
		SideEffects: "None. Reads only.", TokenScope: readScope, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "size", Kind: usecase.KindInt, Description: "At most this many, up to 25; 7 when omitted."},
			{Name: "cursor", Kind: usecase.KindString, Description: "The next_cursor of the page before."},
		},
		Audit: usecase.AuditDeclaration{Action: readAction, TargetType: target, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			lines, next, err := h.Execute(ctx, actor, in.Int("size"), in.String("cursor"))
			if err != nil {
				return nil, err
			}
			return answer(lines, next), nil
		}),
	}
}

// SearchEverywhere searches entries and pages at once (UC-INT-20 check 2): the entries a page at a
// time, the pages on the first page only, both as the reader.
type SearchEverywhere struct{ Glance Glance }

// Execute answers one page.
func (h SearchEverywhere) Execute(
	ctx context.Context, actor appshared.ActorContext, words string, size int, cursor string,
) ([]Found, string, error) {
	words = strings.TrimSpace(words)
	if words == "" || len([]rune(words)) > 200 {
		return nil, "", shared.ErrValidation.WithDetail("search.words_required").
			WithFields(shared.FieldError{Path: "/q", Code: "search.words_required"})
	}
	size = bounded(size, defaultFound, MaxFound)
	in := usecase.Input{"q": words}
	if cursor != "" {
		in["cursor"] = cursor
	}
	rows, next, err := h.Glance.entries(ctx, actor, in, size)
	if err != nil {
		return nil, "", err
	}
	lines, base, err := h.Glance.lines(ctx, actor, rows)
	if err != nil || cursor != "" {
		return lines, next, err
	}
	pages, err := h.Glance.Invoker.Invoke(ctx, searchPages, actor, usecase.Input{"text": words, "limit": size})
	if err != nil {
		return nil, "", err
	}
	data, _ := pages["data"].([]usecase.Output)
	for _, page := range data {
		id, _ := page["id"].(string)
		title, _ := page["title"].(string)
		excerpt, _ := page["excerpt"].(string)
		lines = append(lines, Found{
			Kind: "PAGE", ID: id, Title: title, Excerpt: excerpt, UpdatedAt: timeOf(page["updated_at"]),
			Link: base + "/pages/" + id,
		})
	}
	return lines, next, nil
}

func timeOf(v any) any {
	if at, ok := v.(time.Time); ok {
		return at.UTC()
	}
	return v
}

// Descriptor is the catalogue entry.
func (h SearchEverywhere) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       SearchEverywhereName,
		Permission: service.PermissionRead,
		Summary: "One search across the caller's entries and pages, best first, each with its title, " +
			"ticket key or the passage that matched, and a link - what another product's search box " +
			"lists of Hubtask. The entries come a page at a time; the pages, at most size of them, on " +
			"the first page only. Read as the caller, so only what they may see.",
		SideEffects: "None. Reads only.", TokenScope: readScope, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "q", Kind: usecase.KindString, Required: true, Description: "What to look for, at most 200 characters."},
			{Name: "size", Kind: usecase.KindInt, Description: "At most this many entries and this many pages, up to 25; 10 when omitted."},
			{Name: "cursor", Kind: usecase.KindString, Description: "The next_cursor of the page before."},
		},
		Audit: usecase.AuditDeclaration{Action: readAction, TargetType: target, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			lines, next, err := h.Execute(ctx, actor, in.String("q"), in.Int("size"), in.String("cursor"))
			if err != nil {
				return nil, err
			}
			return answer(lines, next), nil
		}),
	}
}
