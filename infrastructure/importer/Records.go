// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package importer holds the converters: one per foreign format, each turning a file
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
	"github.com/Jersyfi/hubtask/infrastructure/text"
)

// builder accumulates records in the archive's entity order.
//
// Every identity is DuplicateID(salt, entity, source key): the same derivation the restore's
// DUPLICATE rule uses, with the salt derived from the hub - or the collection the import lands
// in - and the source's own identity - the file's digest, or the board's identifier - so that the
// same source imported into the same place twice produces the same identifiers and MERGE with
// skip makes the second import a no-op, while the same file into another place is another set of
// rows.
type builder struct {
	source  service.Source
	salt    shared.ID
	records map[string][]archive.Record
	// named keeps each column's and label's identity by its collection and folded name: the
	// collection's unique names would refuse a second one under another identity, so a name met
	// again - in the file, or already in the target collection - is the one there.
	named map[string]shared.ID
	// linked keeps the links written, so that a label a row names twice - in its labels and as
	// its list - is one link.
	linked map[string]bool
	// orders keeps the last order key handed out per parent, so siblings rank after one another.
	orders map[string]string
	// entries remembers each entry's identity by the key its source used, for a parent reference;
	// byTitle the same by the title's lower case, for a source that names a parent by its title.
	entries map[string]entry
	byTitle map[string]string
	// itemData is each entry's record by its identity, for what a converter settles after the
	// entry is added - its type, status and field values.
	itemData map[string]map[string]any
	// typed keeps the entry types and link types written, by key.
	typed map[string]bool
	// media is the attachments' bytes by digest, and mediaRows their records.
	media     map[string][]byte
	mediaRows map[string]map[string]any
	// rows is each entry's row in the source, by its identity.
	rows map[string]int
}

type entry struct {
	id    shared.ID
	path  string
	depth int
	row   int
}

// newBuilder starts a builder salted by the hub and the source's identity: the digest of the
// file, or a key the converter read out of it.
func newBuilder(source service.Source, identity string) *builder {
	place := source.Hub
	if source.Into != nil {
		place = source.Into.Collection
	}
	b := &builder{
		source:   source,
		salt:     backup.DuplicateID(place, "import", identity),
		records:  map[string][]archive.Record{},
		named:    map[string]shared.ID{},
		linked:   map[string]bool{},
		orders:   map[string]string{},
		entries:  map[string]entry{},
		byTitle:  map[string]string{},
		itemData: map[string]map[string]any{},
	}
	if into := source.Into; into != nil {
		for _, bucket := range into.Buckets {
			b.named[nameKey("buckets", into.Collection, bucket.Name)] = bucket.ID
		}
		for _, label := range into.Labels {
			b.named[nameKey("labels", into.Collection, label.Name)] = label.ID
		}
		// What lands ranks after what was there.
		b.orders["bucket:"+into.Collection.String()] = into.LastBucketOrder
		b.orders["item:"+into.Collection.String()+":"] = into.LastEntryOrder
	}
	return b
}

// nameKey is a column's or a label's name as the collection's unique index compares it: lower
// case, without accents (`lower(imm_unaccent(name))`).
func nameKey(entity string, collection shared.ID, name string) string {
	return entity + "/" + collection.String() + "/" + text.Folded(name)
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
// Into a collection, that collection is the file's: nothing is added, and what the source kept in
// its own collection lands there.
func (b *builder) collection(key, name, description string) shared.ID {
	if b.source.Into != nil {
		return b.source.Into.Collection
	}
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
	name = clip(strings.TrimSpace(name), 120)
	if name == "" {
		name = "Bucket"
	}
	named := nameKey("buckets", collection, name)
	if id, ok := b.named[named]; ok {
		return id
	}
	id := b.id("buckets", key)
	b.named[named] = id
	b.add("buckets", id, map[string]any{
		"id": id.String(), "collection_id": collection.String(), "name": name,
		"order_key": b.nextOrder("bucket:" + collection.String()), "wip_limit": nil,
		"is_done_bucket": done, "color_token": nil, "deleted_at": nil, "version": 1,
	})
	return id
}

// label adds a label to a collection, in one of the ten tokens.
func (b *builder) label(key string, collection shared.ID, name, token string) shared.ID {
	name = clip(strings.TrimSpace(name), 120)
	if name == "" {
		name = "Label"
	}
	named := nameKey("labels", collection, name)
	if id, ok := b.named[named]; ok {
		return id
	}
	id := b.id("labels", key)
	b.named[named] = id
	b.add("labels", id, map[string]any{
		"id": id.String(), "collection_id": collection.String(), "name": name, "color_token": token, "version": 1,
	})
	return id
}

// listLabel is the label a list of the file becomes where the file lands in one collection: a
// source that keeps several lists - a CSV's `collection` column, Google Tasks' and Microsoft To
// Do's lists - keeps them apart by it, and each top-level entry of the list carries it. Empty where
// the import creates collections, which keep the lists apart themselves.
func (b *builder) listLabel(key, name string) shared.ID {
	if b.source.Into == nil || strings.TrimSpace(name) == "" {
		return shared.ID("")
	}
	return b.label("list:"+key, b.source.Into.Collection, name, tokenFor(name))
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
	// Row is the entry's place in the source, for a refusal of it after conversion; zero takes
	// the parent's, so that a card's checklist is reported by the card's row.
	Row int
}

// item adds an entry. The parent, where the source names one, must have been added before it -
// the archive is applied in order and the path is materialised here from the parent's.
func (b *builder) item(in Item) (shared.ID, bool) {
	id := b.id("work_items", in.Key)
	path, depth, row := "/"+id.String()+"/", 0, in.Row
	if in.ParentKey != "" {
		parent, ok := b.entries[in.ParentKey]
		if !ok {
			return shared.ID(""), false
		}
		path, depth = parent.path+id.String()+"/", parent.depth+1
		if row == 0 {
			row = parent.row
		}
	}
	title := clip(strings.TrimSpace(in.Title), 500)
	if title == "" {
		return shared.ID(""), false
	}
	created := b.source.Now
	if in.CreatedAt != nil {
		created = *in.CreatedAt
	}
	// A root is a task and a child is the work under it, unless the converter said otherwise:
	// the model holds `type = TASK` and `parent_id IS NULL` to be one fact, and a default that
	// ignored the parent would write a row the database refuses.
	kind := in.Type
	if kind == "" {
		kind = "TASK"
		if in.ParentKey != "" {
			kind = "WORK_PACKAGE"
		}
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
	b.itemData[id.String()] = data
	b.entries[in.Key] = entry{id: id, path: path, depth: depth, row: row}
	if row > 0 {
		if b.rows == nil {
			b.rows = map[string]int{}
		}
		b.rows[id.String()] = row
	}
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
	key := item.String() + "/" + label.String()
	if b.linked[key] {
		return
	}
	b.linked[key] = true
	b.records["item_labels"] = append(b.records["item_labels"], archive.Record{
		ID: item.String() + "/" + label.String(), Op: archive.OpUpsert, UpdatedAt: b.source.Now,
		Data: map[string]any{"item_id": item.String(), "label_id": label.String()},
	})
}

// comment adds a comment by the importing person, with the source's author named in the text:
// a Trello member is not an account here, and a comment's author must be one.
func (b *builder) comment(key string, item shared.ID, author, body string, at *time.Time) {
	id := b.id("comments", key)
	text := clip(strings.TrimSpace(body), 20000)
	if text == "" {
		return
	}
	if author != "" {
		text = clip(author+": "+text, 20000)
	}
	created := b.source.Now
	if at != nil {
		created = *at
	}
	b.add("comments", id, map[string]any{
		"id": id.String(), "item_id": item.String(), "author_id": b.source.Actor.String(),
		"parent_comment_id": nil, "body": text, "created_at": stamp(created), "edited_at": nil,
		"deleted_at": nil, "version": 1, "kind": "USER", "system_code": nil, "system_params": nil,
	})
}

// reminder adds an absolute reminder at the moment another system kept, by e-mail to whoever
// the entry concerns; one whose moment has passed is marked lapsed by the restore that applies it.
func (b *builder) reminder(key string, item shared.ID, at time.Time) {
	id := b.id("reminders", key)
	b.add("reminders", id, map[string]any{
		"id": id.String(), "item_id": item.String(), "offset_spec": "ABS:" + at.UTC().Format(time.RFC3339),
		"channels": []string{"EMAIL"}, "recipients": []string{}, "state": "PENDING", "fire_at": stamp(at),
		"created_at": stamp(b.source.Now), "updated_at": nil, "version": 1,
	})
}

func (b *builder) result(refused []refusal) service.Result {
	out := service.Result{Records: b.records, Media: b.media, Rows: b.rows}
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

// tokenNamed maps a colour word another system uses onto the nearest of the ten tokens, and
// falls back to the hash where the word is unknown.
func tokenNamed(colour, name string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(colour, "_")[0])) {
	case "green":
		return "green"
	case "yellow", "lime":
		return "lime"
	case "orange":
		return "orange"
	case "red":
		return "red"
	case "purple", "violet":
		return "violet"
	case "blue", "sky":
		return "blue"
	case "pink", "magenta":
		return "magenta"
	case "black", "gray", "grey", "slate":
		return "slate"
	case "teal", "cyan":
		return "teal"
	case "amber", "gold":
		return "amber"
	}
	return tokenFor(name)
}
