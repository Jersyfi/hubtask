// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package odata renders a data feed (UC-INT-15) as an OData v4 service a reporting tool reads -
// Power BI's and Excel's *OData feed*: the service document, the CSDL metadata and the one entity
// set, `Entries`, with server-driven paging. It renders what it is handed and decides nothing: which
// rows a feed holds is the application layer's answer.
package odata

import (
	"encoding/xml"
	"strconv"
	"strings"
	"time"
)

// Namespace is the schema's namespace in the metadata.
const Namespace = "Hubtask"

// EntitySet is the one set a feed serves.
const EntitySet = "Entries"

// PageSize is how many rows one answer carries before `@odata.nextLink` points at the rest.
const PageSize = 1000

// Entry is one row as the feed carries it: never its notes.
type Entry struct {
	ID           string
	Number       int64
	Level        string
	EntryType    string
	Title        string
	Status       string
	Completed    bool
	CompletedAt  *time.Time
	StartAt      *time.Time
	DueAt        *time.Time
	AssigneeID   string
	CollectionID string
	ParentID     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// property is one column of the entity type.
type property struct {
	name, kind string
	nullable   bool
}

var properties = []property{
	{"Id", "Edm.Guid", false},
	{"Number", "Edm.Int64", false},
	{"Level", "Edm.String", false},
	{"EntryType", "Edm.String", true},
	{"Title", "Edm.String", false},
	{"Status", "Edm.String", true},
	{"Completed", "Edm.Boolean", false},
	{"CompletedAt", "Edm.DateTimeOffset", true},
	{"StartAt", "Edm.DateTimeOffset", true},
	{"DueAt", "Edm.DateTimeOffset", true},
	{"AssigneeId", "Edm.Guid", true},
	{"CollectionId", "Edm.Guid", false},
	{"ParentId", "Edm.Guid", true},
	{"CreatedAt", "Edm.DateTimeOffset", false},
	{"UpdatedAt", "Edm.DateTimeOffset", false},
}

// Service is the service document under the feed's root.
func Service(root string) map[string]any {
	return map[string]any{
		"@odata.context": root + "/$metadata",
		"value":          []map[string]any{{"name": EntitySet, "kind": "EntitySet", "url": EntitySet}},
	}
}

// Metadata is the CSDL document describing the entity type `Entry` and the set `Entries`.
func Metadata() []byte {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<edmx:Edmx xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx" Version="4.0">`)
	b.WriteString(`<edmx:DataServices><Schema xmlns="http://docs.oasis-open.org/odata/ns/edm" Namespace="` + Namespace + `">`)
	b.WriteString(`<EntityType Name="Entry"><Key><PropertyRef Name="Id"/></Key>`)
	for _, p := range properties {
		b.WriteString(`<Property Name="` + p.name + `" Type="` + p.kind + `" Nullable="` + strconv.FormatBool(p.nullable) + `"/>`)
	}
	b.WriteString(`</EntityType><EntityContainer Name="Feed">`)
	b.WriteString(`<EntitySet Name="` + EntitySet + `" EntityType="` + Namespace + `.Entry"/>`)
	b.WriteString(`</EntityContainer></Schema></edmx:DataServices></edmx:Edmx>`)
	return []byte(b.String())
}

// Page is the window of rows an answer carries: `$skip` and `$top` as asked, at most PageSize.
type Page struct {
	Skip, Top int
	// HasTop says `$top` was asked; without it the client takes every row, page by page.
	HasTop bool
}

func optional(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func orNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func row(e Entry) map[string]any {
	return map[string]any{
		"Id": e.ID, "Number": e.Number, "Level": e.Level, "EntryType": orNull(e.EntryType), "Title": e.Title,
		"Status": orNull(e.Status), "Completed": e.Completed, "CompletedAt": optional(e.CompletedAt),
		"StartAt": optional(e.StartAt), "DueAt": optional(e.DueAt), "AssigneeId": orNull(e.AssigneeID),
		"CollectionId": e.CollectionID, "ParentId": orNull(e.ParentID),
		"CreatedAt": e.CreatedAt.UTC().Format(time.RFC3339), "UpdatedAt": e.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// Entries is the entity set's answer for the page asked: its rows and, where rows remain that the
// client asked for, the next link.
func Entries(root string, all []Entry, page Page) map[string]any {
	skip := max(page.Skip, 0)
	if skip > len(all) {
		skip = len(all)
	}
	wanted := len(all) - skip
	if page.HasTop && page.Top < wanted {
		wanted = max(page.Top, 0)
	}
	take := min(wanted, PageSize)
	rows := make([]map[string]any, 0, take)
	for _, e := range all[skip : skip+take] {
		rows = append(rows, row(e))
	}
	out := map[string]any{"@odata.context": root + "/$metadata#" + EntitySet, "value": rows}
	if take < wanted {
		next := root + "/" + EntitySet + "?$skip=" + strconv.Itoa(skip+take)
		if page.HasTop {
			next += "&$top=" + strconv.Itoa(wanted-take)
		}
		out["@odata.nextLink"] = next
	}
	return out
}
