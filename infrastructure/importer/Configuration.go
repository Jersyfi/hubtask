// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Jersyfi/hubtask/core/application/archive"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/text"
)

// The records of a source that carries its own configuration (D-10): entry types, workflows and
// the scheme that binds one to a collection, custom fields with their values, link types, an
// entry's history and attachments, and pages. Spelled here beside the rest, for the reason
// Records.go gives.

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// keyOf turns a name another system uses into a key: lower case, accents folded, every run of
// other characters one underscore, a leading digit prefixed. Empty where nothing is left.
func keyOf(name string) string {
	var out strings.Builder
	gap := false
	for _, r := range strings.ToLower(text.Folded(name)) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			if gap && out.Len() > 0 {
				out.WriteByte('_')
			}
			gap = false
			out.WriteRune(r)
		default:
			gap = true
		}
	}
	key := out.String()
	if key == "" {
		return ""
	}
	if key[0] >= '0' && key[0] <= '9' {
		key = "f_" + key
	}
	if len(key) > 50 {
		key = strings.TrimRight(key[:50], "_")
	}
	if !keyPattern.MatchString(key) {
		return ""
	}
	return key
}

// entryType adds a workspace entry type at a level. A key the workspace already holds is kept by
// the restore's skip rule, and the entries then name that type.
func (b *builder) entryType(key, name, icon string, level work.ItemType, workflow bool, position int) {
	if b.typed == nil {
		b.typed = map[string]bool{}
	}
	if b.typed[key] || key == work.SystemEntryTypeKey(level) {
		return
	}
	b.typed[key] = true
	id := b.id("entry_types", key)
	b.add("entry_types", id, map[string]any{
		"id": id.String(), "key": key, "level": string(level), "system": false,
		"name": clip(strings.TrimSpace(name), 120), "icon": icon, "color_token": string(shared.LabelTokenSlate),
		"completable": true, "workflow": workflow, "record": false, "schedulable": true, "estimable": false,
		"allowed_children": []string{}, "form": map[string]any{}, "position": position,
		"archived_at": nil, "created_at": stamp(b.source.Now), "updated_at": stamp(b.source.Now), "version": 1,
	})
}

// workflow adds a published workflow under a key, and binds it to a collection for every type.
// The shape has been validated by the converter; a key the workspace holds is kept by the skip
// rule, and the scheme then names that workflow.
func (b *builder) workflow(key, name string, shape work.WorkflowShape, collection shared.ID) {
	statuses := make([]map[string]any, 0, len(shape.Statuses))
	for _, status := range shape.Statuses {
		statuses = append(statuses, map[string]any{
			"key": status.Key, "name": status.Name, "category": string(status.Category), "color_token": status.ColorToken,
		})
	}
	steps := make([]map[string]any, 0, len(shape.Steps))
	for _, step := range shape.Steps {
		from := step.From
		if from == nil {
			from = []string{}
		}
		steps = append(steps, map[string]any{"key": step.Key, "name": step.Name, "from": from, "to": step.To, "who": map[string]any{}})
	}
	id := b.id("workflows", key)
	b.add("workflows", id, map[string]any{
		"id": id.String(), "key": key, "system": false, "name": clip(strings.TrimSpace(name), 120),
		"published":    map[string]any{"statuses": statuses, "initial": shape.Initial, "steps": steps},
		"draft":        nil,
		"published_at": stamp(b.source.Now), "published_by": b.source.Actor.String(),
		"created_at": stamp(b.source.Now), "updated_at": stamp(b.source.Now), "version": 1,
	})
	b.records["workflow_schemes"] = append(b.records["workflow_schemes"], archive.Record{
		ID: collection.String() + "/", Op: archive.OpUpsert, UpdatedAt: b.source.Now,
		Data: map[string]any{
			"container_id": collection.String(), "entry_type": "", "workflow_key": key, "created_at": stamp(b.source.Now),
		},
	})
}

// Field is a custom field definition a converter derived from the source's own.
type Field struct {
	Key     string
	Kind    work.CustomFieldKind
	Options []string
}

// field adds a collection's custom field definition carried by every level, and answers its
// identity for the entries' `custom_field_refs`.
func (b *builder) field(collection shared.ID, in Field) shared.ID {
	id := b.id("custom_field_definitions", collection.String()+"/"+in.Key)
	options := in.Options
	if options == nil {
		options = []string{}
	}
	b.add("custom_field_definitions", id, map[string]any{
		"id": id.String(), "collection_id": collection.String(), "key": in.Key, "kind": string(in.Kind),
		"options": options, "currency": nil, "formula": nil, "is_required": false, "restricted": false,
		"applies_to": []string{string(work.ItemTask), string(work.ItemWorkPackage), string(work.ItemActivity)},
		"deleted_at": nil, "created_at": stamp(b.source.Now), "updated_at": stamp(b.source.Now), "version": 1,
	})
	return id
}

// classify sets an entry's type and status and its custom field values, after item added it.
// A status of a DONE category completes the entry, as a workflow collection does (D-02).
func (b *builder) classify(item shared.ID, entryType, status string, done bool, values map[string]any, refs map[string]shared.ID) {
	data, ok := b.itemData[item.String()]
	if !ok {
		return
	}
	if entryType != "" {
		data["entry_type"] = entryType
	}
	if status != "" {
		data["status"] = status
	}
	if done && data["completed_at"] == nil {
		data["is_completed"] = true
		data["completed_at"] = stamp(b.source.Now)
		data["completed_by"] = b.source.Actor.String()
	}
	fields := map[string]any{}
	named := map[string]any{}
	for key, value := range values {
		if id, ok := refs[key]; ok {
			fields[key] = value
			named[key] = id.String()
		}
	}
	data["custom_fields"] = fields
	data["custom_field_refs"] = named
}

// Change is one field of a change another system recorded.
type Change struct {
	Field string
	From  string
	To    string
}

// history adds a change the source recorded, in the entry's history as the importing person's
// with the source's author named in the change set: the author is no account here.
func (b *builder) history(key string, item, collection shared.ID, author string, at time.Time, changes []Change) {
	set := map[string]any{}
	for _, change := range changes {
		field := keyOf(change.Field)
		if field == "" {
			continue
		}
		set[field] = map[string]any{"from": nilIfEmpty(clip(change.From, 500)), "to": nilIfEmpty(clip(change.To, 500))}
	}
	if len(set) == 0 {
		return
	}
	if author != "" {
		set["changed_by"] = map[string]any{"from": nil, "to": clip(author, 200)}
	}
	id := b.id("activity_entries", key)
	b.add("activity_entries", id, map[string]any{
		"id": id.String(), "item_id": item.String(), "container_id": collection.String(),
		"actor_type": string(shared.ActorUser), "actor_id": b.source.Actor.String(),
		"verb": string(activity.ItemHistoryImported), "change_set": set, "occurred_at": stamp(at),
		"correlation_id": nil, "causation_id": nil,
	})
}

// attachment adds a file to an entry: one media object per content, so that the same bytes
// attached twice are one object referenced twice, its bytes handed to the archive by digest.
func (b *builder) attachment(item shared.ID, name string, content []byte, at time.Time) {
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	id := b.id("media_objects", digest)
	if b.media == nil {
		b.media = map[string][]byte{}
		b.mediaRows = map[string]map[string]any{}
	}
	row, seen := b.mediaRows[digest]
	if !seen {
		head := content
		if len(head) > 512 {
			head = head[:512]
		}
		kind, err := media.AcceptClaim("", http.DetectContentType(head))
		if err != nil || kind == "" {
			kind = "application/octet-stream"
		}
		file := clip(strings.TrimSpace(strings.Map(func(r rune) rune {
			if r == '/' || r == '\\' || unicode.IsControl(r) {
				return '_'
			}
			return r
		}, name)), media.MaxFileNameLength)
		if file == "" {
			file = "attachment"
		}
		row = map[string]any{
			"id": id.String(), "storage_key": "", "mime_type": kind, "byte_size": len(content),
			"checksum": digest, "usage": string(media.UsageAttachment), "ref_count": 0,
			"created_by": b.source.Actor.String(), "created_at": stamp(at), "deleted_at": nil,
			"status": string(media.StatusReady), "file_name": file,
		}
		b.mediaRows[digest] = row
		b.media[digest] = content
		b.records["media_objects"] = append(b.records["media_objects"], archive.Record{
			ID: id.String(), Op: archive.OpUpsert, UpdatedAt: b.source.Now, Data: row,
			Blobs: []archive.Blob{{Digest: digest, Bytes: int64(len(content))}},
		})
	}
	key := item.String() + "/" + id.String()
	if b.linked[key] {
		return
	}
	b.linked[key] = true
	row["ref_count"] = row["ref_count"].(int) + 1
	b.records["item_attachments"] = append(b.records["item_attachments"], archive.Record{
		ID: key, Op: archive.OpUpsert, UpdatedAt: b.source.Now,
		Data: map[string]any{"item_id": item.String(), "media_id": id.String()},
	})
}

// linkType adds a workspace link type for a relation the shipped six do not name.
func (b *builder) linkType(key, name, inverse string, position int) {
	if b.typed == nil {
		b.typed = map[string]bool{}
	}
	if b.typed["link:"+key] {
		return
	}
	b.typed["link:"+key] = true
	direction := string(work.LinkDirected)
	if inverse == "" || strings.EqualFold(inverse, name) {
		direction, inverse = string(work.LinkSymmetric), ""
	}
	id := b.id("link_types", key)
	b.add("link_types", id, map[string]any{
		"id": id.String(), "key": key, "name": clip(strings.TrimSpace(name), 120),
		"inverse_name": nilIfEmpty(clip(strings.TrimSpace(inverse), 120)), "direction": direction,
		"cardinality": string(work.LinkMany), "cycles": string(work.LinkCyclesAllowed), "waits": string(work.LinkWaitsNone),
		"position": position, "archived_at": nil, "created_at": stamp(b.source.Now), "updated_at": stamp(b.source.Now), "version": 1,
	})
}

// entryLink adds a link of a type between two entries the builder holds.
func (b *builder) entryLink(linkType string, source, target shared.ID) {
	key := linkType + "/" + source.String() + "/" + target.String()
	if b.linked[key] || source == target {
		return
	}
	b.linked[key] = true
	id := b.id("entry_links", key)
	b.add("entry_links", id, map[string]any{
		"id": id.String(), "link_type": linkType, "source_id": source.String(), "target_id": target.String(),
		"created_by": b.source.Actor.String(), "created_at": stamp(b.source.Now), "dependency_kind": "FS", "lag_days": 0,
	})
}

// Page is what a converter knows about one page of a wiki.
type Page struct {
	Key       string
	ParentKey string
	Title     string
	Body      string
	CreatedAt *time.Time
	UpdatedAt *time.Time
}

// page adds a hub's page, after its parent. A body over the column's bound is cut at it.
func (b *builder) page(in Page) (shared.ID, bool) {
	title := clip(strings.TrimSpace(in.Title), 200)
	if title == "" {
		return shared.ID(""), false
	}
	parent := shared.ID("")
	if in.ParentKey != "" {
		known, ok := b.entries["page:"+in.ParentKey]
		if !ok {
			return shared.ID(""), false
		}
		parent = known.id
	}
	body := in.Body
	if len(body) > 1<<20 {
		body = strings.ToValidUTF8(body[:1<<20], "")
	}
	created, updated := b.source.Now, b.source.Now
	if in.CreatedAt != nil {
		created = *in.CreatedAt
	}
	if in.UpdatedAt != nil {
		updated = *in.UpdatedAt
	}
	id := b.id("pages", in.Key)
	b.add("pages", id, map[string]any{
		"id": id.String(), "hub_id": b.source.Hub.String(), "parent_id": nilIfEmptyID(parent), "title": title,
		"body": body, "icon": "", "order_key": b.nextOrder("page:" + parent.String()), "template": false,
		"content_language": nilIfEmpty(b.source.Language), "owner_id": nil,
		"verified_until": nil, "verified_at": nil, "verified_by": nil, "archived_at": nil, "deleted_at": nil,
		"created_by": b.source.Actor.String(), "created_at": stamp(created),
		"updated_by": b.source.Actor.String(), "updated_at": stamp(updated), "version": 1,
	})
	b.records["page_versions"] = append(b.records["page_versions"], archive.Record{
		ID: id.String() + "/1", Op: archive.OpUpsert, UpdatedAt: b.source.Now,
		Data: map[string]any{
			"page_id": id.String(), "seq": 1, "title": title, "body": body, "author_id": b.source.Actor.String(),
			"name": "", "restored": 0, "created_at": stamp(updated),
		},
	})
	b.entries["page:"+in.Key] = entry{id: id}
	return id, true
}
