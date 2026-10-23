// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// The keys of an answer to a question, as the prompt asks for them and the payload holds them.
const (
	answerKey  = "answer"
	sourcesKey = "source_ids"
	filterKey  = "filter"
)

// The bounds on an answer: a few sentences, and the handful of entries it rests on.
const (
	maxAnswerText    = 2000
	maxAnswerSources = 5
)

// answerDues are the spans a filter's due date may name, each resolved by whoever reads the filter
// against their own today: a closed set, so a model chooses a span rather than writing a date.
var answerDues = []string{"overdue", "today", "next_7_days", "this_month"}

// answerFrom answers the payload and how many references were dropped, or false where the answer
// cannot be read or says nothing. What it may name is exactly what the material showed
// (ai-first.md §2): an entry, a place, a person or a label it was not shown is dropped, and a
// place's type is the one the material recorded, never the answer's.
func answerFrom(text string, sets *AnswerSets) (map[string]any, int, bool) {
	answered, ok := objectFrom(text)
	if !ok || sets == nil {
		return nil, 0, false
	}
	words, _ := answered[answerKey].(string)
	words = strings.TrimSpace(words)
	if words == "" {
		return nil, 0, false
	}
	if utf8.RuneCountInString(words) > maxAnswerText {
		words = string([]rune(words)[:maxAnswerText])
	}
	payload := map[string]any{answerKey: words}
	dropped := 0

	listed, _ := answered[sourcesKey].([]any)
	sources := []any{}
	for _, raw := range listed {
		id, _ := raw.(string)
		if !slices.Contains(sets.Entries, id) || slices.Contains(sources, any(id)) || len(sources) >= maxAnswerSources {
			dropped++
			continue
		}
		sources = append(sources, id)
	}
	if len(sources) > 0 {
		payload[sourcesKey] = sources
	}

	if asked, isObject := answered[filterKey].(map[string]any); isObject {
		filter, notShown := answerFilter(asked, sets)
		dropped += notShown
		if len(filter) > 0 {
			payload[filterKey] = filter
		}
	}
	return payload, dropped, true
}

// answerFilter keeps of a filter the closed values the material allows, and counts the rest.
func answerFilter(asked map[string]any, sets *AnswerSets) (map[string]any, int) {
	filter := map[string]any{}
	dropped := 0
	for key, value := range asked {
		switch key {
		case "place_id":
			id, _ := value.(string)
			index := slices.IndexFunc(sets.Places, func(place AnswerPlace) bool { return place.ID == id })
			if index < 0 {
				dropped++
				continue
			}
			filter["place_id"] = id
			filter["place_type"] = string(sets.Places[index].Type)
		case "is_completed":
			done, isBool := value.(bool)
			if !isBool {
				dropped++
				continue
			}
			filter["is_completed"] = done
		case "due":
			span, _ := value.(string)
			if !slices.Contains(answerDues, span) {
				dropped++
				continue
			}
			filter["due"] = span
		case "assignee_id":
			id, _ := value.(string)
			if !shownOption(sets.People, id) {
				dropped++
				continue
			}
			filter["assignee_id"] = id
		case "label_ids":
			listed, _ := value.([]any)
			labels := []any{}
			for _, raw := range listed {
				id, _ := raw.(string)
				if !shownOption(sets.Labels, id) || slices.Contains(labels, any(id)) {
					dropped++
					continue
				}
				labels = append(labels, id)
			}
			if len(labels) > 0 {
				filter["label_ids"] = labels
			}
		default:
			// A key no filter of an answer holds - an expression, a date, a field nobody offered.
			dropped++
		}
	}
	return filter, dropped
}

func shownOption(options []Option, id string) bool {
	return id != "" && slices.ContainsFunc(options, func(option Option) bool { return option.ID == id })
}
