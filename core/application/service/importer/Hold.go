// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// FormHolder holds an entry an import would create in an existing collection to its type's form,
// through the import door: nil where the type has no form or the entry meets it.
type FormHolder interface {
	HoldImported(ctx context.Context, actor appshared.ActorContext, collection work.Container, item work.WorkItem) error
}

// hold takes out of the converter's records every entry its type's form refuses, with what hangs
// on it, and answers a refusal per entry (accepted.md, second answers 4; UC-IMP-04 check 8). Only an
// import into an existing collection is held: there the types and forms are the workspace's own,
// while an import that creates its collections brings types with an empty form, and holding
// another tool's history to rules it never knew would refuse what the source accepted.
//
// Refusal instead of corruption: an entry is dropped whole, never landed half-filled, and nothing
// that names it - the work under it, a label, a link, a comment, an attachment - lands without it.
func (r Runner) hold(
	ctx context.Context, scope persistence.Scope, run domain.Run, collection work.Container, result *repository.Result,
) error {
	if r.Forms == nil || len(result.Records["work_items"]) == 0 {
		return nil
	}
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: run.TenantID, AccountID: run.RequestedBy,
		Channel: string(work.ChannelImport),
	}
	dropped := map[string]bool{}
	var refused []domain.Refusal
	err := r.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		for _, record := range result.Records["work_items"] {
			if parent, _ := record.Data["parent_id"].(string); parent != "" && dropped[parent] {
				dropped[record.ID] = true
				refused = append(refused, domain.Refusal{Row: result.Rows[record.ID], Code: domain.CodeRowParentRefused})
				continue
			}
			item, err := importedItem(record, collection.ID)
			if err != nil {
				return err
			}
			err = r.Forms.HoldImported(ctx, actor, collection, item)
			if err == nil {
				continue
			}
			refusal, ok := formRefusal(err)
			if !ok {
				return err
			}
			refusal.Row = result.Rows[record.ID]
			dropped[record.ID] = true
			refused = append(refused, refusal)
		}
		return nil
	})
	if err != nil || len(dropped) == 0 {
		return err
	}
	withoutDropped(result, dropped)
	result.Refused = append(result.Refused, refused...)
	slices.SortStableFunc(result.Refused, func(a, b domain.Refusal) int { return a.Row - b.Row })
	return nil
}

// formRefusal reads the form's refusal of an entry as the import's, or answers false for anything
// that is no refusal of the entry - the database away, a broken condition - which fails the run.
func formRefusal(err error) (domain.Refusal, bool) {
	var typed *shared.Error
	if !errors.As(err, &typed) {
		return domain.Refusal{}, false
	}
	switch typed.DetailCode {
	case "forms.channel_closed":
		return domain.Refusal{Code: domain.CodeRowChannelClosed}, true
	case "forms.refused", "fields.restricted":
		refusal := domain.Refusal{Code: domain.CodeRowFormRefused}
		for _, field := range typed.Fields {
			key := field.Params["key"]
			if field.Code == "fields.restricted" {
				// A restricted field is named by its definition's key, a form by its own.
				key = work.FormCustomPrefix + key
			} else if key == "" {
				key = field.Params["validation"]
			}
			if key != "" && !slices.Contains(refusal.Fields, key) {
				refusal.Fields = append(refusal.Fields, key)
			}
		}
		return refusal, true
	}
	return domain.Refusal{}, false
}

// importedItem reads an entry record back into the entry it would create, as far as a form reads
// one: level, type, title, notes, dates, completion and the custom fields as stored.
func importedItem(record archive.Record, collection shared.ID) (work.WorkItem, error) {
	data := record.Data
	text := func(key string) string { value, _ := data[key].(string); return value }
	item := work.WorkItem{
		ID: shared.ID(record.ID), CollectionID: collection, Type: work.ItemType(text("type")),
		EntryType: text("entry_type"), Path: text("path"), Title: text("title"), Notes: text("notes"),
	}
	item.Completion.IsCompleted, _ = data["is_completed"].(bool)
	if at, ok := instant(text("due_at")); ok {
		item.Due = &work.DueDate{At: at, TimeZone: text("due_time_zone")}
		item.Due.DateOnly, _ = data["due_date_only"].(bool)
	}
	if at, ok := instant(text("start_at")); ok {
		item.StartAt = &at
	}
	// Through JSON, so that a value reads as the stored entry's does - a list as a list of any.
	if fields, ok := data["custom_fields"]; ok && fields != nil {
		raw, err := json.Marshal(fields)
		if err != nil {
			return work.WorkItem{}, shared.Internalf("import: an entry's fields could not be read: %w", err)
		}
		if err := json.Unmarshal(raw, &item.CustomFields); err != nil {
			return work.WorkItem{}, shared.Internalf("import: an entry's fields could not be read: %w", err)
		}
	}
	return item, nil
}

func instant(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, value)
	return at, err == nil
}

// withoutDropped removes the dropped entries and every record naming one, until nothing more
// names what was removed, then recounts the attachments' objects and drops those left unused.
func withoutDropped(result *repository.Result, dropped map[string]bool) {
	names := func(record archive.Record) bool {
		if dropped[record.ID] {
			return true
		}
		for _, part := range strings.Split(record.ID, "/") {
			if dropped[part] {
				return true
			}
		}
		for _, value := range record.Data {
			if id, ok := value.(string); ok && dropped[id] {
				return true
			}
		}
		return false
	}
	for changed := true; changed; {
		changed = false
		for entity, records := range result.Records {
			kept := records[:0:0]
			for _, record := range records {
				if names(record) {
					if !dropped[record.ID] && entity != "media_objects" {
						dropped[record.ID] = true
						changed = true
					}
					continue
				}
				kept = append(kept, record)
			}
			result.Records[entity] = kept
		}
	}
	used := map[string]int{}
	for _, entity := range []string{"item_attachments", "page_media"} {
		for _, record := range result.Records[entity] {
			if id, ok := record.Data["media_id"].(string); ok {
				used[id]++
			}
		}
	}
	objects := result.Records["media_objects"][:0:0]
	for _, record := range result.Records["media_objects"] {
		if used[record.ID] == 0 {
			for _, blob := range record.Blobs {
				delete(result.Media, blob.Digest)
			}
			continue
		}
		record.Data["ref_count"] = used[record.ID]
		objects = append(objects, record)
	}
	if len(result.Records["media_objects"]) > 0 {
		result.Records["media_objects"] = objects
	}
}
