// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package importer holds the converters (P-08…P-10): one per foreign format, each turning a file
// into the archive records the restore applies. What they share is here - how a collection, a
// bucket, a label, an entry, a link and a comment are spelled as records under identities derived
// from the source's own, so that four converters cannot come to disagree about what a row is.
package importer

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	service "github.com/Jersyfi/hubtask/core/application/repository/importer"
	backup "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
)

// builder accumulates records in the archive's entity order.
//
// Every identity is DuplicateID(salt, entity, source key): the same derivation the restore's
// DUPLICATE rule uses, with the salt derived from the hub and the source's own identity - the
// file's digest, or the board's identifier - so that the same source imported into the same hub
// twice produces the same identifiers and MERGE with skip makes the second import a no-op, while
// the same file into another hub is another set of rows.
type builder struct {
	source  service.Source
	salt    shared.ID
	records map[string][]archive.Record
	// orders keeps the last order key handed out per parent, so siblings rank after one another.
	orders map[string]string
	// entries remembers each entry's identity by the key its source used, for a parent reference;
	// byTitle the same by the title's lower case, for a source that names a parent by its title.
	entries map[string]entry
	byTitle map[string]string
}

type entry struct {
	id    shared.ID
	path  string
	depth int
}

// newBuilder starts a builder salted by the hub and the source's identity: the digest of the
// file, or a key the converter read out of it.
func newBuilder(source service.Source, identity string) *builder {
	return &builder{
		source:  source,
		salt:    backup.DuplicateID(source.Hub, "import", identity),
		records: map[string][]archive.Record{},
		orders:  map[string]string{},
		entries: map[string]entry{},
		byTitle: map[string]string{},
	}
}

// titles remembers an entry by its title, first one wins: a parent named by title is the first
// entry that carried it.
func (b *builder) titles(title, key string) {
	lower := strings.ToLower(strings.TrimSpace(title))
	if _, taken := b.byTitle[lower]; !taken {
		b.byTitle[lower] = key
	}
}

func (b *builder) id(entity, key string) shared.ID {
	return backup.DuplicateID(b.salt, entity, key)
}

func (b *builder) nextOrder(parent string) string {
	next, err := domainservice.OrderKeyAfter(b.orders[parent])
	if err != nil {
		// The generator only fails on a malformed previous key, and every previous key here is
		// one it produced.
		next = b.orders[parent] + "V"
	}
	b.orders[parent] = next
	return next
}

func (b *builder) add(entity string, id shared.ID, data map[string]any) {
	b.records[entity] = append(b.records[entity], archive.Record{
		ID: id.String(), Op: archive.OpUpsert, UpdatedAt: b.source.Now, Data: data,
	})
}

func stamp(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }

func optional(at *time.Time) any {
	if at == nil {
		return nil
	}
	return stamp(*at)
}

// collection adds a collection under the hub. The key is the source's own identifier for it.
func (b *builder) collection(key, name, description string) shared.ID {
	id := b.id("containers", key)
	name = clip(strings.TrimSpace(name), 200)
	if name == "" {
		name = "Imported"
	}
	data := map[string]any{
		"id": id.String(), "type": "COLLECTION", "parent_id": b.source.Hub.String(), "name": name,
		"description": nilIfEmpty(clip(description, 2000)), "icon": nil, "color_token": nil,
		"order_key": b.nextOrder("hub"), "policies": map[string]any{},
		"archived_at": nil, "deleted_at": nil, "trash_batch_id": nil, "deleted_by_type": nil, "deleted_by_id": nil,
		"created_by": b.source.Actor.String(), "created_at": stamp(b.source.Now), "updated_at": stamp(b.source.Now),
		"version": 1,
	}
	b.add("containers", id, data)
	return id
}

// bucket adds a bucket to a collection.
func (b *builder) bucket(key string, collection shared.ID, name string, done bool) shared.ID {
	id := b.id("buckets", key)
	name = clip(strings.TrimSpace(name), 120)
	if name == "" {
		name = "Bucket"
	}
	b.add("buckets", id, map[string]any{
		"id": id.String(), "collection_id": collection.String(), "name": name,
		"order_key": b.nextOrder("bucket:" + collection.String()), "wip_limit": nil,
		"is_done_bucket": done, "color_token": nil, "deleted_at": nil, "version": 1,
	})
	return id
}

// label adds a label to a collection, in one of the ten tokens.
func (b *builder) label(key string, collection shared.ID, name, token string) shared.ID {
	id := b.id("labels", key)
	name = clip(strings.TrimSpace(name), 120)
	if name == "" {
		name = "Label"
	}
	b.add("labels", id, map[string]any{
		"id": id.String(), "collection_id": collection.String(), "name": name, "color_token": token, "version": 1,
	})
	return id
}

// Item is what a converter knows about one entry.
type Item struct {
	Key         string
	ParentKey   string
	Collection  shared.ID
	Type        string
	Title       string
	Notes       string
	Bucket      shared.ID
	DueAt       *time.Time
	DueDateOnly bool
	DueZone     string
	CompletedAt *time.Time
	ArchivedAt  *time.Time
	CreatedAt   *time.Time
}

// item adds an entry. The parent, where the source names one, must have been added before it -
// the archive is applied in order and the path is materialised here from the parent's.
func (b *builder) item(in Item) (shared.ID, bool) {
	id := b.id("work_items", in.Key)
	path, depth := "/"+id.String()+"/", 0
	if in.ParentKey != "" {
		parent, ok := b.entries[in.ParentKey]
		if !ok {
			return shared.ID(""), false
		}
		path, depth = parent.path+id.String()+"/", parent.depth+1
	}
	title := clip(strings.TrimSpace(in.Title), 500)
	if title == "" {
		return shared.ID(""), false
	}
	created := b.source.Now
	if in.CreatedAt != nil {
		created = *in.CreatedAt
	}
	kind := in.Type
	if kind == "" {
		kind = "TASK"
	}
	data := map[string]any{
		"id": id.String(), "collection_id": in.Collection.String(), "type": kind,
		"parent_id": nilIfEmptyID(b.parentID(in.ParentKey)), "path": path, "depth": depth,
		"title": title, "notes": nilIfEmpty(clip(in.Notes, 20000)),
		"is_completed": in.CompletedAt != nil, "completed_at": optional(in.CompletedAt),
		"completed_by": completedBy(in.CompletedAt, b.source.Actor),
		"bucket_id":    nilIfEmptyID(in.Bucket), "order_key": b.nextOrder("item:" + in.Collection.String() + ":" + in.ParentKey),
		"start_at": nil, "due_at": optional(in.DueAt), "due_date_only": in.DueDateOnly,
		"due_time_zone": nilIfEmpty(in.DueZone), "assignee_id": nil,
		"cover_kind": nil, "cover_color_token": nil, "cover_media_id": nil,
		"custom_fields": map[string]any{}, "recurrence_rule_id": nil, "recurrence_source_id": nil,
		"origin_jumble_id": nil, "content_language": nilIfEmpty(b.source.Language),
		"archived_at": optional(in.ArchivedAt), "deleted_at": nil, "trash_batch_id": nil,
		"created_by": b.source.Actor.String(), "created_at": stamp(created), "updated_at": stamp(b.source.Now),
		"version": 1, "custom_field_refs": map[string]any{}, "search_document": nil,
		"due_soon_announced_at": nil, "overdue_announced_at": nil,
		"retention_pending_until": nil, "retention_rule_id": nil, "retention_action": nil, "retention_blocked_by": nil,
	}
	b.add("work_items", id, data)
	b.entries[in.Key] = entry{id: id, path: path, depth: depth}
	return id, true
}

func (b *builder) parentID(key string) shared.ID {
	if key == "" {
		return shared.ID("")
	}
	return b.entries[key].id
}

func completedBy(at *time.Time, actor shared.ID) any {
	if at == nil {
		return nil
	}
	return actor.String()
}

// link attaches a label to an entry.
func (b *builder) link(item, label shared.ID) {
	b.records["item_labels"] = append(b.records["item_labels"], archive.Record{
		ID: item.String() + "/" + label.String(), Op: archive.OpUpsert, UpdatedAt: b.source.Now,
		Data: map[string]any{"item_id": item.String(), "label_id": label.String()},
	})
}

func (b *builder) result(refused []refusal) service.Result {
	out := service.Result{Records: b.records}
	for _, r := range refused {
		out.Refused = append(out.Refused, r.refusal())
	}
	return out
}

func clip(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nilIfEmptyID(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}

// tokenFor picks a label colour for a name that came without one: the ten tokens, walked by a
// hash of the name, so that the same label lands in the same colour on every import.
func tokenFor(name string) string {
	tokens := shared.LabelTokens
	sum := 0
	for _, r := range strings.ToLower(name) {
		sum = (sum*31 + int(r)) % len(tokens)
	}
	return string(tokens[sum])
}
