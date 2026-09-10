// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// CatalogueSources reads what a suggestion is made from, through the ordinary reads.
//
// The same decision `EntryTargets` makes, and it has to be the same one: the material and the
// fingerprint must come from a single read, or a producer could read one state and fingerprint
// another and the staleness check would pass for a suggestion made from something else.
type CatalogueSources struct {
	Catalogue Catalogue
}

var _ Sources = CatalogueSources{}

// Material answers the text to describe, the fingerprint of the state it came from, and the closed
// sets the question about to be asked may choose from.
func (s CatalogueSources) Material(
	ctx context.Context, actor appshared.ActorContext,
	targetType domain.TargetType, targetID shared.ID, promptID string,
) (Material, error) {
	switch targetType {
	case domain.TargetJumbleEntry:
		// An entry in the inbox is in no collection yet, so there is no board to choose from: the
		// person converting names the destination, and its columns are theirs to pick.
		return s.entry(ctx, actor, targetID)
	case domain.TargetWorkItem:
		return s.item(ctx, actor, targetID, promptID)
	default:
		return Material{}, shared.ErrNotFound.WithDetail("suggestions.not_found")
	}
}

// entry is the subject and the body of one inbox arrival - the least trusted text in the system
// (G-10), which travels to a provider as user content and never as instruction.
//
// The listing is unfiltered, for `entryDigest`'s reason: a `status: NEW` filter here meant that
// asking about an entry somebody had already converted answered nothing at all, rather than
// answering what the entry says.
//
// Unfiltered, for `entryDigest`'s reason: a `status: NEW` filter here meant that asking about an
// entry somebody had already converted answered nothing at all, rather than answering what the
// entry says.
func (s CatalogueSources) entry(
	ctx context.Context, actor appshared.ActorContext, entryID shared.ID,
) (Material, error) {
	out, err := s.Catalogue.Invoke(ctx, "ListJumbleEntries", actor, usecase.Input{})
	if err != nil {
		return Material{}, err
	}
	// `data`, which is what a page answers under (api-guidelines.md §4). It was `items` until
	// J-16: the key was wrong, so the loop below always saw an empty list and every suggestion
	// about a jumble entry was refused `suggestions.not_found` - and the test fakes invented the
	// wrong key too, so nothing but a real registry could say so.
	entries, _ := out["data"].([]usecase.Output)
	for _, entry := range entries {
		if entry.String("id") != entryID.String() {
			continue
		}
		subject, body := entry.String("raw_subject"), entry.String("raw_body")
		if strings.TrimSpace(subject) == "" && strings.TrimSpace(body) == "" {
			// Nothing to describe. The labels below would otherwise make an empty entry look like
			// content to the caller's emptiness check, and a provider would be asked about
			// "Subject:" and nothing else.
			return Material{Digest: domain.Digest(subject, body)}, nil
		}
		return Material{
			// Two fields joined for the model with a label each, because a subject and a body are
			// different things and a model told only "here is some text" describes the wrong one.
			// The labels are the instruction's, not the content's: nothing here is interpolated
			// into the system message (Prompt.Ask).
			Content: "Subject: " + subject + "\n\n" + body,
			Digest:  domain.Digest(subject, body),
		}, nil
	}
	return Material{}, shared.ErrNotFound.WithDetail("suggestions.not_found")
}

// item is an entry's title and notes, the same two fields the fingerprint is taken over - and, for
// a question that chooses from one, the board it sits on.
func (s CatalogueSources) item(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID, promptID string,
) (Material, error) {
	out, err := s.Catalogue.Invoke(ctx, "GetWorkItem", actor, usecase.Input{
		"item_id": itemID.String(),
	})
	if err != nil {
		return Material{}, err
	}
	title, notes := out.String("title"), out.String("notes")
	if strings.TrimSpace(title) == "" && strings.TrimSpace(notes) == "" {
		return Material{Digest: domain.Digest(title, notes)}, nil
	}

	material := Material{
		Content: "Title: " + title + "\n\n" + notes,
		Digest:  domain.Digest(title, notes),
	}
	if promptFields[promptID][fieldsKey] {
		declared, err := s.declared(ctx, actor, out)
		if err != nil {
			return Material{}, err
		}
		material.Declared = declared
	}
	for _, key := range choiceSets[promptID] {
		set, err := s.set(ctx, actor, key, out)
		if err != nil {
			return Material{}, err
		}
		if len(set.Options) > 0 {
			material.Choices = append(material.Choices, set)
		}
	}
	return material, nil
}

// set reads one closed set this entry could be classified into.
func (s CatalogueSources) set(
	ctx context.Context, actor appshared.ActorContext, key string, item usecase.Output,
) (Choices, error) {
	switch key {
	case bucketKey:
		return s.board(ctx, actor, item)
	case labelsKey:
		return s.vocabulary(ctx, actor, item)
	default:
		// A key named in `choiceSets` and read by nothing would be a set a model is asked to
		// choose from and never shown - which is the leak the whole milestone is about.
		return Choices{}, shared.ErrInternal.
			WithDetail("suggestions.choices_unknown").
			WithParams(map[string]string{"key": key})
	}
}

// The answer keys a closed set is chosen under, and what applies them at acceptance:
// `MoveWorkItem`'s `target_bucket_id`, and one `AddLabel` per chosen label.
const (
	bucketKey = "bucket_id"
	labelsKey = "label_ids"
	fieldsKey = "custom_fields"
)

// closedKinds are the custom field kinds a value can be *chosen* for (K-03).
//
// The open ones - TEXT, NUMBER, DATE, URL - are deliberately absent, and so is USER. The row says
// "classification", and classifying into an open set is not classification: a model writing a
// number or a sentence into a workspace's field is not choosing from what was declared, it is
// filling in a form nobody checked. USER is the same refusal wearing a closed set's clothes - the
// members of a collection are people, and putting one of them on an entry is naming rather than
// choosing.
var closedKinds = map[work.CustomFieldKind]bool{
	work.CustomFieldSelect:      true,
	work.CustomFieldMultiSelect: true,
	work.CustomFieldBool:        true,
}

// declared answers the custom fields in force for this entry, narrowed to the ones a value can be
// chosen for and to the ones its own type carries.
//
// The entry's own value travels beside each, so that a model can leave a field that is already
// right alone. No other entry's value ever does: what another entry holds under the same key is
// that entry's content.
func (s CatalogueSources) declared(
	ctx context.Context, actor appshared.ActorContext, item usecase.Output,
) ([]Declared, error) {
	collectionID := item.String("collection_id")
	if collectionID == "" {
		return nil, nil
	}

	out, err := s.Catalogue.Invoke(ctx, "ListCustomFields", actor, usecase.Input{
		"collection_id": collectionID,
	})
	if err != nil {
		if errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	itemType := work.ItemType(item.String("type"))
	values, _ := item["custom_fields"].(map[string]any)
	rows, _ := out["data"].([]usecase.Output)

	declared := make([]Declared, 0, len(rows))
	for _, row := range rows {
		definition, ok := definitionFrom(row)
		if !ok || !closedKinds[definition.Kind] || !definition.Carries(itemType) {
			continue
		}
		declared = append(declared, Declared{
			Definition: definition, Current: values[definition.Key],
		})
	}
	return declared, nil
}

// definitionFrom reads a definition back out of the listing, as far as validating a value needs it:
// the key, the kind, the options and whether it is required. Nothing else is read, because nothing
// else decides what a value may be.
func definitionFrom(row usecase.Output) (work.CustomFieldDefinition, bool) {
	kind, err := work.ParseCustomFieldKind(row.String("kind"))
	if err != nil || row.String("key") == "" {
		return work.CustomFieldDefinition{}, false
	}

	required, _ := row["is_required"].(bool)
	definition := work.CustomFieldDefinition{
		Key: row.String("key"), Kind: kind, IsRequired: required,
	}
	definition.Options = texts(row["options"])
	for _, carried := range texts(row["applies_to"]) {
		definition.AppliesTo = append(definition.AppliesTo, work.ItemType(carried))
	}
	return definition, true
}

// texts reads a list of strings out of a use case's answer, which carries them as themselves
// in-process and as `[]any` once anything has been through JSON.
func texts(value any) []string {
	switch held := value.(type) {
	case []string:
		return append([]string(nil), held...)
	case []any:
		read := make([]string, 0, len(held))
		for _, entry := range held {
			if text, isText := entry.(string); isText {
				read = append(read, text)
			}
		}
		return read
	default:
		return nil
	}
}

// vocabulary answers the labels this entry's collection has agreed on.
//
// A label is a set entry rather than a field, and it exists in a collection's vocabulary or it does
// not - so this is the same closed set a board is, and for a stronger reason: words a model
// invented could never be applied. `AddLabel` takes a label of the entry's own collection, and a
// workspace that has not agreed on "urgent" does not acquire it because a model wrote it down.
//
// A collection with an empty vocabulary is offered nothing and is classified by its board alone,
// which is the honest answer: there is nothing to choose.
func (s CatalogueSources) vocabulary(
	ctx context.Context, actor appshared.ActorContext, item usecase.Output,
) (Choices, error) {
	collectionID := item.String("collection_id")
	if collectionID == "" {
		return Choices{}, nil
	}

	out, err := s.Catalogue.Invoke(ctx, "ListLabels", actor, usecase.Input{
		"collection_id": collectionID,
	})
	if err != nil {
		if errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrNotFound) {
			return Choices{}, nil
		}
		return Choices{}, err
	}

	rows, _ := out["data"].([]usecase.Output)
	options := make([]Option, 0, len(rows))
	for _, row := range rows {
		options = append(options, Option{ID: row.String("id"), Name: row.String("name")})
	}
	return Choices{Key: labelsKey, Label: "Labels this collection uses", Options: options}, nil
}

// board answers the columns this entry could be moved between, with the one it is in now marked.
//
// Read through the ordinary listing, as everything in this package is, so the columns offered are
// the columns the person asking may see. Two entries produce no set at all rather than an empty
// one:
//
//   - An entry that is not directly in a collection. A board belongs to a collection and only the
//     entries directly in it have a place on one (domain-model.md §2), so offering its columns
//     would be offering a choice the domain refuses at acceptance.
//   - A collection with no board. Nothing to choose from is not an error - the classification is
//     the labels, and it is produced exactly as it was before this existed.
func (s CatalogueSources) board(
	ctx context.Context, actor appshared.ActorContext, item usecase.Output,
) (Choices, error) {
	if parent, held := item["parent_id"].(string); held && parent != "" {
		return Choices{}, nil
	}
	collectionID := item.String("collection_id")
	if collectionID == "" {
		return Choices{}, nil
	}

	out, err := s.Catalogue.Invoke(ctx, "ListBuckets", actor, usecase.Input{
		"collection_id": collectionID,
	})
	if err != nil {
		// A board the asker may not read is not a reason to refuse them a classification: they
		// get the labels, which is what this question answered before it could offer a column at
		// all. Anything else is a defect and travels.
		if errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrNotFound) {
			return Choices{}, nil
		}
		return Choices{}, err
	}

	rows, _ := out["data"].([]usecase.Output)
	current := item.String(bucketKey)
	options := make([]Option, 0, len(rows))
	for _, row := range rows {
		id := row.String("id")
		options = append(options, Option{
			ID: id, Name: row.String("name"), Current: id != "" && id == current,
		})
	}
	return Choices{Key: bucketKey, Label: "Board columns", Options: options}, nil
}
