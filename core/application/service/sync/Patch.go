// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"sort"
	"strings"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// ITEM_PATCH (N-05, offline-sync.md §4.2): a map of fields, each with its value and its own
// reading, decided one by one against the server's clock per field - the later reading wins, a
// tie broken by the device identifier the way HLC.Compare breaks it - and applied through the use
// case that owns each field, as the pushing person, under the device's reading.

// clockEntityItem is the entity name an entry's change log entries carry, and therefore the one
// its clocks are filed under.
const clockEntityItem = "item"

// The fields a device may patch, and which use case owns each. A table in code rather than a
// name on the mutation, for the applier table's reason. `custom_fields.<key>` is matched by
// prefix, one key at a time (§4.2's maps row).
var (
	// updateFields are UpdateWorkItem's: one call carries every winner among them.
	updateFields = map[string]bool{
		work.FieldTitle: true, work.FieldNotes: true, work.FieldBucketID: true,
		work.FieldContentLanguage: true, work.FieldStartAt: true,
		work.FieldDueAt: true, work.FieldDueDateOnly: true, work.FieldDueTimeZone: true,
	}
	// dueFields travel as a trio: a qualifier without its date is refused by the use case, so a
	// winner among them is completed with the server's current values of the others.
	dueFields = []string{work.FieldDueAt, work.FieldDueDateOnly, work.FieldDueTimeZone}
	// laterFields are the ones a patch does not carry. The hierarchy travels as a MOVE, its own
	// kind, because a parent is decided with cycle detection and a destination rather than as a
	// value; a patch naming it is refused with a code that says so.
	laterFields = map[string]bool{work.FieldParentID: true}
	// ownedFields are the server's, never in a patch (§4.2: derived, provenance, retention, the
	// lifecycle stamps, the search's columns).
	ownedFields = map[string]bool{
		"id": true, "type": true, work.FieldCollectionID: true, "path": true, "depth": true,
		"version": true, "updated_at": true, "created_at": true, "created_by": true,
		"recurrence_rule_id": true, "recurrence_source_id": true, "origin_jumble_id": true,
		work.FieldCalendarUID: true, "retention": true, "archived_at": true, "deleted_at": true, "trash_batch_id": true,
		"search_document": true, "search_configuration": true,
	}
)

// patch applies an ITEM_PATCH: decides every field, applies the winners, and answers with the
// server's state.
func (p PushChanges) patch(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error) {
	if m.ItemID.IsZero() {
		return Result{}, shared.ErrValidation.WithDetail("sync.item_required")
	}
	if len(m.Fields) == 0 {
		return Result{}, shared.ErrValidation.
			WithDetail("sync.fields_required").
			WithFields(shared.FieldError{Path: "/fields", Code: "sync.fields_required"})
	}
	for field := range m.Fields {
		if err := patchable(field); err != nil {
			return Result{}, err
		}
	}

	// The server's copy, through the use case that reads it - so that a device that may not read
	// the entry gets that refusal and not a merge.
	before, err := p.Catalogue.Invoke(ctx, "GetWorkItem", actor, usecase.Input{"item_id": m.ItemID.String()})
	if err != nil {
		return Result{}, err
	}
	clocks, err := p.clocksOf(ctx, actor, m.ItemID)
	if err != nil {
		return Result{}, err
	}

	// The decision, per field. A field with no reading on the server loses to any reading: it
	// was written before the clocks existed, and last writer wins over a value nobody stamped.
	winners := map[string]shared.HLC{}
	losers := map[string]shared.HLC{}
	for field, change := range m.Fields {
		reading, err := shared.ParseHLC(change.HLC)
		if err != nil {
			return Result{}, err
		}
		if stored, stamped := clocks[field]; stamped && !reading.After(stored) {
			losers[field] = reading
			continue
		}
		winners[field] = reading
	}

	if len(winners) > 0 {
		// Applied under the device's readings: the change log adapter records each winning
		// field under the reading that decided it, so the next device compares against the
		// clock that won and not against a fresh server reading.
		applied := appshared.ContextWithReadings(ctx, winners)
		if err := p.applyWinners(applied, actor, m, winners, before); err != nil {
			return Result{}, err
		}
	}
	// What lost, and had a rule about losing (§4.2's status row, §5): a completion that lost
	// is a visible step in the history, and free text that lost is filed as a comment.
	conflict, err := p.answerLosers(ctx, actor, m, losers, before)
	if err != nil {
		return Result{}, err
	}

	after, err := p.Catalogue.Invoke(ctx, "GetWorkItem", actor, usecase.Input{"item_id": m.ItemID.String()})
	if err != nil {
		return Result{}, err
	}
	// APPLIED when everything the device sent won and nothing else had moved since its base;
	// MERGED otherwise, with the server's state, which the device adopts (§9, requirement 5);
	// CONFLICT when free text lost, with both values so a client can offer the choice (§5).
	result := domain.Merged
	if len(winners) == len(m.Fields) && (m.BaseVersion == nil || versionOf(before) == *m.BaseVersion) {
		result = domain.Applied
	}
	if conflict != nil {
		result = domain.Conflict
	}
	return Result{OpID: m.OpID, Result: result, EntityID: m.ItemID, ServerState: after, Conflict: conflict}, nil
}

// answerLosers gives the fields with a rule about losing their answer, and returns the conflict
// detail when free text lost.
func (p PushChanges) answerLosers(
	ctx context.Context, actor appshared.ActorContext, m Mutation, losers map[string]shared.HLC,
	before usecase.Output,
) (*ConflictDetail, error) {
	deviceID := appshared.DeviceFrom(ctx)
	now := p.Stream.Clock.Now()
	tenantID, itemID, collectionID := actor.TenantID, m.ItemID, shared.ID(stringOf(before[work.FieldCollectionID]))

	if reading, lost := losers[work.FieldCompletion]; lost && p.Activity != nil {
		// A reopen is never silently discarded, and neither is a completion (§4.2): the history
		// says somebody tried, from which device, under which reading.
		if err := p.Activity.RecordStep(ctx, actor, tenantID, itemID, collectionID, activity.ItemChangeLost,
			map[string]any{
				"field": work.FieldCompletion, "device_id": deviceID.String(), "hlc": reading.String(),
				"is_completed": completionWanted(m.Fields[work.FieldCompletion].Value),
			}, now); err != nil {
			return nil, err
		}
	}

	reading, lost := losers[work.FieldNotes]
	if !lost {
		return nil, nil
	}
	mine, _ := m.Fields[work.FieldNotes].Value.(string)
	conflict := &ConflictDetail{Field: work.FieldNotes, Mine: mine, Theirs: before[work.FieldNotes]}
	if p.Displaced != nil && mine != "" {
		// The displaced version is filed as a comment in the pushing person's name, with the
		// same permission as any comment: refused, the conflict still carries both values.
		comment, err := p.Displaced.FileDisplaced(ctx, actor, itemID, mine, map[string]string{
			"field": work.FieldNotes, "device": deviceID.String(),
			"at": reading.Physical.UTC().Format(time.RFC3339),
		})
		switch {
		case err == nil:
			conflict.PreservedCommentID = comment.ID
		case !refusal(err):
			return nil, err
		}
	}
	if p.Activity != nil {
		if err := p.Activity.RecordStep(ctx, actor, tenantID, itemID, collectionID, activity.ItemMerged,
			map[string]any{
				"field": work.FieldNotes, "device_id": deviceID.String(), "hlc": reading.String(),
				"preserved_comment_id": idOrNilOf(conflict.PreservedCommentID),
			}, now); err != nil {
			return nil, err
		}
	}
	return conflict, nil
}

// completionWanted reads the done/open state a device sent, whichever of the contract's shapes it
// used: the object `GET /items/{id}` answers with, or the flag alone.
func completionWanted(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case map[string]any:
		done, _ := v["is_completed"].(bool)
		return done
	}
	return false
}

func stringOf(value any) string {
	text, _ := value.(string)
	return text
}

func idOrNilOf(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}

// applyWinners performs the use case that owns each winning field, grouped by use case.
func (p PushChanges) applyWinners(
	ctx context.Context, actor appshared.ActorContext, m Mutation, winners map[string]shared.HLC,
	before usecase.Output,
) error {
	itemID := m.ItemID.String()
	update := usecase.Input{}
	dueTouched := false

	// In a fixed order, so that two runs over one mutation perform the same calls in the same
	// sequence - which is what makes a test readable and a log comparable.
	fields := make([]string, 0, len(winners))
	for field := range winners {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		value := m.Fields[field].Value
		switch {
		case updateFields[field]:
			update[field] = value
			if field == work.FieldDueAt || field == work.FieldDueDateOnly || field == work.FieldDueTimeZone {
				dueTouched = true
			}
		case field == work.FieldAssigneeID:
			if err := p.applyAssignee(ctx, actor, itemID, value); err != nil {
				return err
			}
		case field == work.FieldCover:
			if err := p.applyCover(ctx, actor, itemID, value); err != nil {
				return err
			}
		case field == work.FieldCompletion:
			name := "ReopenWorkItem"
			if completionWanted(value) {
				name = "CompleteWorkItem"
			}
			// Idempotent in the use case: an entry already in the state asked for writes nothing
			// and seeds nothing, which is what keeps a double completion from producing a second
			// occurrence (D-05, SY-8).
			if _, err := p.Catalogue.Invoke(ctx, name, actor, usecase.Input{"item_id": itemID}); err != nil {
				return err
			}
		case field == work.FieldOrderKey:
			key, _ := value.(string)
			if _, err := p.Catalogue.Invoke(ctx, "ReorderWorkItem", actor, usecase.Input{
				"item_id": itemID, "order_key": key,
			}); err != nil {
				return err
			}
		case strings.HasPrefix(field, work.FieldCustomFields+"."):
			key := strings.TrimPrefix(field, work.FieldCustomFields+".")
			if _, err := p.Catalogue.Invoke(ctx, "SetCustomField", actor, usecase.Input{
				"item_id": itemID, "key": key, "value": value,
			}); err != nil {
				return err
			}
		}
	}

	if len(update) == 0 {
		return nil
	}
	if dueTouched {
		// The trio travels together: a member the device did not win keeps the server's value,
		// read from the copy the merge started from.
		for _, field := range dueFields {
			if _, sent := update[field]; !sent {
				update[field] = before[field]
			}
		}
		if update[work.FieldDueAt] == nil {
			// Clearing: the qualifiers go with the date, and UpdateWorkItem reads an empty
			// due_at as the clearing.
			delete(update, work.FieldDueDateOnly)
			delete(update, work.FieldDueTimeZone)
			update[work.FieldDueAt] = ""
		}
	}
	// A null the device sent is the field's clearing, which the use case reads as the empty
	// string - the same reading the REST route gives a JSON null.
	for field, value := range update {
		if value == nil {
			update[field] = ""
		}
	}
	update["item_id"] = itemID
	_, err := p.Catalogue.Invoke(ctx, "UpdateWorkItem", actor, update)
	return err
}

// applyAssignee is AssignWorkItem for a person and UnassignWorkItem for a null.
func (p PushChanges) applyAssignee(ctx context.Context, actor appshared.ActorContext, itemID string, value any) error {
	account, _ := value.(string)
	if account == "" {
		_, err := p.Catalogue.Invoke(ctx, "UnassignWorkItem", actor, usecase.Input{"item_id": itemID})
		return err
	}
	_, err := p.Catalogue.Invoke(ctx, "AssignWorkItem", actor, usecase.Input{"item_id": itemID, "account_id": account})
	return err
}

// applyCover is SetCover for an object and ClearCover for a null. The object is the contract's
// `Cover`: kind, colour token, media identifier.
func (p PushChanges) applyCover(ctx context.Context, actor appshared.ActorContext, itemID string, value any) error {
	cover, isObject := value.(map[string]any)
	if value == nil || (isObject && len(cover) == 0) {
		_, err := p.Catalogue.Invoke(ctx, "ClearCover", actor, usecase.Input{"item_id": itemID})
		return err
	}
	if !isObject {
		return shared.ErrValidation.
			WithDetail("sync.field_malformed").
			WithParams(map[string]string{"field": work.FieldCover}).
			WithFields(shared.FieldError{Path: "/fields/cover", Code: "sync.field_malformed"})
	}
	in := usecase.Input{"item_id": itemID}
	for _, key := range []string{"kind", "color_token", "media_id"} {
		if v, present := cover[key]; present && v != nil {
			in[key] = v
		}
	}
	_, err := p.Catalogue.Invoke(ctx, "SetCover", actor, in)
	return err
}

// clocksOf reads the server's clock per field of the entry.
func (p PushChanges) clocksOf(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (map[string]shared.HLC, error) {
	if p.Clocks == nil {
		return nil, nil
	}
	var clocks map[string]shared.HLC
	err := p.Stream.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		clocks, err = p.Clocks.Of(ctx, clockEntityItem, itemID)
		return err
	})
	return clocks, err
}

// patchable judges one field name of a patch.
func patchable(field string) error {
	switch {
	case updateFields[field], field == work.FieldAssigneeID, field == work.FieldCover,
		field == work.FieldOrderKey, field == work.FieldCompletion,
		strings.HasPrefix(field, work.FieldCustomFields+"."):
		return nil
	case laterFields[field]:
		return shared.ErrUnavailable.
			WithDetail("sync.field_unavailable").
			WithParams(map[string]string{"field": field})
	case ownedFields[field]:
		return shared.ErrValidation.
			WithDetail("sync.field_not_mergeable").
			WithParams(map[string]string{"field": field}).
			WithFields(shared.FieldError{Path: "/fields/" + field, Code: "sync.field_not_mergeable"})
	}
	return shared.ErrValidation.
		WithDetail("sync.field_unknown").
		WithParams(map[string]string{"field": field}).
		WithFields(shared.FieldError{Path: "/fields/" + field, Code: "sync.field_unknown"})
}

// versionOf reads the version out of an entry's output, whichever integer type it came back as.
func versionOf(out usecase.Output) int {
	switch v := out["version"].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return -1
}
