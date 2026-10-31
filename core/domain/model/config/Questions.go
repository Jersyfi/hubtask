// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// QuestionKind is what a template's question asks for.
type QuestionKind string

const (
	// QuestionText is a line of text, at most MaxAnswerLength characters.
	QuestionText QuestionKind = "TEXT"
	// QuestionGroup is one of the workspace's groups, answered by its identifier.
	QuestionGroup QuestionKind = "GROUP"
	// QuestionChoice is one of the question's options.
	QuestionChoice QuestionKind = "CHOICE"
)

// MaxAnswerLength bounds a TEXT answer, a title's bound.
const MaxAnswerLength = 200

// answerMark prefixes a string value of a template's document that its answer replaces:
// `"answer:first_group"`. Only a whole value is replaced, so a word inside a text is never
// rewritten.
const answerMark = "answer:"

// Question is what a template asks when it is applied (configuration.md §6), "which group answers
// first". Its key is what the document's marks name.
type Question struct {
	Key      string
	Kind     QuestionKind
	Required bool
	// Default answers a question left open; a GROUP has none, since no group is every workspace's.
	Default string
	Options []string
}

// ValidQuestions holds a template's questions to their declaration: a key each, once, a known
// kind, a choice's options with its default among them.
func ValidQuestions(questions []Question) error {
	seen := map[string]bool{}
	var refusals []shared.FieldError
	for _, q := range questions {
		path := "questions." + q.Key
		switch {
		case !ValidKey(q.Key) || seen[q.Key]:
			refusals = append(refusals, Refusal("config.question_key_invalid", path, map[string]string{"key": q.Key}))
		case q.Kind != QuestionText && q.Kind != QuestionGroup && q.Kind != QuestionChoice:
			refusals = append(refusals, Refusal("config.question_kind_unknown", path, map[string]string{"kind": string(q.Kind)}))
		case q.Kind == QuestionChoice && len(q.Options) == 0:
			refusals = append(refusals, Refusal("config.question_without_options", path, nil))
		case q.Default != "":
			if _, code := q.answer(q.Default); code != "" {
				refusals = append(refusals, Refusal("config.question_default_invalid", path, nil))
			}
		}
		seen[q.Key] = true
	}
	if len(refusals) > 0 {
		return ErrRefused(refusals)
	}
	return nil
}

// answer reads one answer to the question, or the code that refuses it.
func (q Question) answer(raw string) (string, string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "config.answer_required"
	}
	switch q.Kind {
	case QuestionGroup:
		id, err := shared.ParseID(value)
		if err != nil {
			return "", "config.answer_not_a_group"
		}
		return id.String(), ""
	case QuestionChoice:
		if !slices.Contains(q.Options, value) {
			return "", "config.answer_not_an_option"
		}
	default:
		if utf8.RuneCountInString(value) > MaxAnswerLength || strings.ContainsAny(value, "\r\n") {
			return "", "config.answer_too_long"
		}
	}
	return value, ""
}

// Answer is the document with each mark replaced by its question's answer, or its default. A block
// that names a question left open with no default is left out - "escalate to the first group" is
// not taken where nobody named the group - and the template's own document is not changed. Every
// wrong answer is refused at once, by the question's key.
func Answer(questions []Question, document Document, answers map[string]string) (Document, error) {
	byKey := map[string]Question{}
	for _, q := range questions {
		byKey[q.Key] = q
	}
	var refusals []shared.FieldError
	keys := make([]string, 0, len(answers))
	for key := range answers {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if _, asked := byKey[key]; !asked {
			refusals = append(refusals, Refusal("config.answer_unknown", "answers."+key, nil))
		}
	}
	values := map[string]string{}
	for _, q := range questions {
		raw, given := answers[q.Key]
		if !given {
			raw = q.Default
		}
		if !given && raw == "" {
			if q.Required {
				refusals = append(refusals, Refusal("config.answer_required", "answers."+q.Key, nil))
			}
			continue
		}
		value, code := q.answer(raw)
		if code != "" {
			refusals = append(refusals, Refusal(code, "answers."+q.Key, nil))
			continue
		}
		values[q.Key] = value
	}
	if len(refusals) > 0 {
		return nil, ErrRefused(refusals)
	}

	out := make(Document, len(document))
	for name, section := range document {
		if section == nil {
			out[name] = nil
			continue
		}
		kept := make(Section, len(section))
		for key, block := range section {
			if block == nil {
				kept[key] = nil
				continue
			}
			answered, whole := answerValue(map[string]any(block), values)
			if whole {
				kept[key] = Block(answered.(map[string]any))
			}
		}
		out[name] = kept
	}
	return out, nil
}

// answerValue copies a value with its marks answered, and reports false when a mark names a
// question without an answer.
func answerValue(value any, values map[string]string) (any, bool) {
	switch v := value.(type) {
	case string:
		key, marked := strings.CutPrefix(v, answerMark)
		if !marked {
			return v, true
		}
		answer, held := values[key]
		return answer, held
	case map[string]any:
		out := make(map[string]any, len(v))
		for name, each := range v {
			answered, whole := answerValue(each, values)
			if !whole {
				return nil, false
			}
			out[name] = answered
		}
		return out, true
	case []any:
		out := make([]any, 0, len(v))
		for _, each := range v {
			answered, whole := answerValue(each, values)
			if !whole {
				return nil, false
			}
			out = append(out, answered)
		}
		return out, true
	default:
		return v, true
	}
}
