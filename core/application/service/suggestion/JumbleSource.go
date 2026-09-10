// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"slices"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
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
	if !slices.Contains(choiceSets[promptID], bucketKey) {
		return material, nil
	}
	board, err := s.board(ctx, actor, out)
	if err != nil {
		return Material{}, err
	}
	if len(board.Options) > 0 {
		material.Choices = append(material.Choices, board)
	}
	return material, nil
}

// bucketKey is the answer key a board column is chosen under, and the input key `MoveWorkItem`
// would be given at acceptance.
const bucketKey = "bucket_id"

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
