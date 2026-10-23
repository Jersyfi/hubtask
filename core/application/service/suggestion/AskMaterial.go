// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// answerPrompt answers a person's question about the workspace (UC-AI-08).
const answerPrompt = "answer-question"

// The bounds on what a question is answered from: a page of entries per place the question names,
// a page of entries its words find, and what the material may hold of both together. Enough for
// any question a person asks in a sentence, and far short of reading a workspace.
const (
	maxAskedPerPlace  = 15
	maxAskedByWords   = 20
	maxAskedEntries   = 30
	maxAskedNotes     = 300
	maxAskedWordQuery = 200
)

// AnswerSets are what an answer may name, exactly what its material showed: the narrowing checks
// the answer against these, so a model cannot cite an entry or point a filter at a place, a person
// or a label it was not shown (ai-first.md §2).
type AnswerSets struct {
	// Entries are the identifiers of the entries shown, in the order shown.
	Entries []string
	// Places are the hubs and collections the question names, each with its type.
	Places []AnswerPlace
	// People and Labels are those the question names, the asker always among the people.
	People []Option
	Labels []Option
}

// AnswerPlace is one hub or collection a filter may be anchored at.
type AnswerPlace struct {
	ID   string
	Type work.ContainerType
	Name string
}

// askMaterial is what a question is answered from, read through the ordinary reads as the asker:
// what they may not read is not found by those reads, so it is not in the material (UC-AI-08 check
// 3). Work entries only - no inbox entry is read.
func (s CatalogueSources) askMaterial(
	ctx context.Context, actor appshared.ActorContext, question Question, kept integration.KeptOut,
) (Material, error) {
	if question.TargetID != actor.AccountID {
		// An answer is the asker's own; a question about somebody else's account is not one.
		return Material{}, shared.ErrNotFound.WithDetail("suggestions.not_found")
	}
	words := s.nfc(question.Words)
	matcher := newNameMatcher(words)

	containers, err := s.containers(ctx, actor)
	if err != nil {
		return Material{}, err
	}
	names := make(map[string]string, len(containers))
	var sets AnswerSets
	var namedCollections []container
	for _, each := range containers {
		names[each.id] = each.name
		if !matcher.names(s.nfc(each.name)) {
			continue
		}
		sets.Places = append(sets.Places, AnswerPlace{ID: each.id, Type: each.kind, Name: kept.Scrub(each.name)})
		if each.kind == work.ContainerCollection {
			namedCollections = append(namedCollections, each)
		}
	}
	sets.Places = firstShown(sets.Places)

	sets.People, err = s.people(ctx, actor, automation.Scope{Type: automation.ScopeTenant}, containers, matcher, kept)
	if err != nil {
		return Material{}, err
	}
	sets.Labels, err = s.labels(ctx, actor, namedCollections, matcher, kept)
	if err != nil {
		return Material{}, err
	}
	sets.Labels = firstShown(sortOptions(sets.Labels))

	var searches []usecase.Input
	for _, place := range sets.Places {
		searches = append(searches, usecase.Input{
			"container_id": place.ID, "size": maxAskedPerPlace,
			"filter": map[string]any{"field": "is_completed", "op": "EQ", "value": false},
		})
	}
	if query := wordQuery(words); query != "" {
		searches = append(searches, usecase.Input{"q": query, "size": maxAskedByWords})
	}
	var entries []usecase.Output
	seen := map[string]bool{}
	for _, in := range searches {
		found, err := s.searchEntries(ctx, actor, in)
		if err != nil {
			return Material{}, err
		}
		for _, entry := range found {
			id := entry.String("id")
			if id == "" || seen[id] || len(entries) >= maxAskedEntries {
				continue
			}
			seen[id] = true
			// A kept-out person's entries, and those a restricted person is on, are not material.
			if keptEntry(kept, entry) {
				continue
			}
			entries = append(entries, entry)
			sets.Entries = append(sets.Entries, id)
		}
	}
	if len(entries) == 0 && len(sets.Places) == 0 {
		// Nothing the question names and nothing its words find: nothing to answer from, which
		// the asking refuses at once rather than queueing for nothing.
		return Material{Digest: domain.Digest(actor.AccountID.String(), "")}, nil
	}

	return Material{
		Content: s.writtenAskMaterial(sets, entries, names, kept),
		// The asker's own identifier: an answer is read and dismissed, never applied, so there is
		// no state of the workspace it could go stale against.
		Digest: domain.Digest(actor.AccountID.String(), ""),
		Answer: &sets,
	}, nil
}

// searchEntries is one page of SearchItems as the asker. A search the query grammar refuses
// finds nothing rather than failing the question: the words are the person's, not a query.
func (s CatalogueSources) searchEntries(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) ([]usecase.Output, error) {
	out, err := s.Catalogue.Invoke(ctx, "SearchItems", actor, in)
	if errors.Is(err, shared.ErrValidation) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, _ := out["data"].([]usecase.Output)
	return rows, nil
}

// wordQuery is the question as a search: any of its words, so that "what is open for Müller
// GmbH" finds an entry that names Müller. The grammar's own signs - quotes, a leading minus, the
// word "or" - are left out, because the person wrote a sentence and not a query.
func wordQuery(question string) string {
	var kept []string
	length := 0
	for _, word := range wordsOf(question) {
		if strings.EqualFold(word, "or") {
			continue
		}
		next := utf8.RuneCountInString(word)
		if len(kept) > 0 {
			next += len(" or ")
		}
		if length+next > maxAskedWordQuery {
			break
		}
		kept = append(kept, word)
		length += next
	}
	return strings.Join(kept, " or ")
}

func (s CatalogueSources) writtenAskMaterial(
	sets AnswerSets, entries []usecase.Output, names map[string]string, kept integration.KeptOut,
) string {
	var written strings.Builder
	people := make(map[string]string, len(sets.People))
	for _, person := range sets.People {
		people[person.ID] = person.Name
	}
	if len(sets.Places) > 0 {
		written.WriteString("Places (place_id):\n")
		for _, place := range sets.Places {
			written.WriteString("- " + place.ID + " - " + string(place.Type) + " " + place.Name + "\n")
		}
	}
	for _, set := range []struct {
		label   string
		options []Option
	}{
		{"People (assignee_id)", sets.People}, {"Labels (label_ids)", sets.Labels},
	} {
		if len(set.options) == 0 {
			continue
		}
		written.WriteString("\n" + set.label + ":\n")
		for _, option := range set.options {
			written.WriteString("- " + option.ID + " - " + option.Name + "\n")
		}
	}
	written.WriteString("\nEntries (source_ids):\n")
	if len(entries) == 0 {
		written.WriteString("none found\n")
	}
	for _, entry := range entries {
		written.WriteString("\n- " + entry.String("id") + " - " + kept.Scrub(entry.String("title")) +
			" [" + standing(entry))
		if place := names[entry.String("collection_id")]; place != "" {
			written.WriteString(", in " + kept.Scrub(place))
		}
		if assignee := people[entry.String("assignee_id")]; assignee != "" {
			written.WriteString(", assigned to " + assignee)
		}
		written.WriteString("]\n")
		if notes := excerpt(entry.String("notes")); notes != "" {
			written.WriteString("  " + kept.Scrub(notes) + "\n")
		}
	}
	return written.String()
}

// excerpt is the start of an entry's notes on one line: enough to tell what it is about.
func excerpt(notes string) string {
	notes = strings.Join(strings.Fields(notes), " ")
	if utf8.RuneCountInString(notes) <= maxAskedNotes {
		return notes
	}
	return string([]rune(notes)[:maxAskedNotes]) + "…"
}
