// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A condition composed as sentences (automation.md §1.5): a bounded set of subjects, the operators
// each takes, and groups of them under all of, any of or none of. The rule editor composes them and
// compiles them to the CEL a rule stores; this is the same compiler on the server, for the one
// writer that may not write an expression - a model drafting a rule from a sentence (UC-AUT-10).
//
// It writes exactly what the editor writes, byte for byte, because the editor draws a condition as
// sentences only where it can read the expression back as the shapes it compiles to; anything else
// is drawn as raw text. A shared vector file holds the two compilers together
// (`test/architecture/conditionsentences_test.go` and the editor's `model.test.ts`).
//
// What makes this safe to hand a model's answer to is that no byte of a value becomes expression
// text except inside a string literal it escapes itself, or as a number it parsed and bounded: the
// reasoning of rule 9, applied to the rules' language.

// Sentence is one condition: a subject, an operator and up to two values, as the editor holds it.
type Sentence struct {
	Subject string
	Op      string
	A       string
	B       string
}

// SentenceGroup is sentences and groups under one mode.
type SentenceGroup struct {
	Mode  string
	Items []ConditionNode
}

// ConditionNode is a sentence or a group, exactly one of the two.
type ConditionNode struct {
	Sentence *Sentence
	Group    *SentenceGroup
}

// The group modes.
const (
	ModeAll  = "all"
	ModeAny  = "any"
	ModeNone = "none"
)

// SentenceOperators is the subjects and the operators each takes, as the editor offers them.
var sentenceOperators = map[string][]string{
	"type":      {"is", "is_not"},
	"title":     {"contains", "not_contains", "starts_with"},
	"notes":     {"empty", "not_empty", "contains"},
	"completed": {"yes", "no"},
	"archived":  {"yes", "no"},
	"due":       {"has", "lacks", "past", "future", "within"},
	"label":     {"on", "not_on"},
	"assignee":  {"has", "lacks", "is", "is_not"},
	"bucket":    {"is", "is_not"},
	"parent":    {"has", "lacks"},
	"depth":     {"is", "at_most"},
	"actor":     {"is", "is_not"},
	"hour":      {"between"},
	"field":     {"is", "is_not"},
}

// SentenceOperators answers the operators a subject takes, in the order the editor offers them;
// nothing for a subject the composer does not know.
func SentenceOperators(subject string) []string {
	return append([]string(nil), sentenceOperators[subject]...)
}

// SentenceSubjects answers every subject the composer knows, for a caller narrowing them.
func SentenceSubjects() []string {
	subjects := make([]string, 0, len(sentenceOperators))
	for subject := range sentenceOperators {
		subjects = append(subjects, subject)
	}
	return subjects
}

// The bounds a sentence's values are held to. A number outside them is refused rather than
// clamped: a clamped hour is a rule nobody asked for.
const (
	// MaxSentenceText bounds a written value - a word a title contains, a label's identifier.
	MaxSentenceText = 200
	maxDepth        = 4
	maxDays         = 365
	maxHour         = 24
)

// SentenceTakes says which values an operator takes: "none", "value", "number", "days", "range"
// or "key_value" - the editor's `takes`.
func SentenceTakes(subject, op string) string {
	switch subject {
	case "hour":
		return "range"
	case "field":
		return "key_value"
	case "depth":
		return "number"
	case "due":
		if op == "within" {
			return "days"
		}
		return "none"
	case "label":
		return "value"
	}
	switch op {
	case "has", "lacks", "yes", "no", "empty", "not_empty":
		return "none"
	}
	return "value"
}

// sentenceInvalidCode refuses a sentence the composer could not have written, or one whose value is
// out of its bounds.
const sentenceInvalidCode = "automation.sentence_invalid"

func sentenceInvalid() error { return shared.ErrValidation.WithDetail(sentenceInvalidCode) }

// CompileSentence answers the expression one sentence is, or refuses it.
func CompileSentence(s Sentence) (string, error) {
	operators, known := sentenceOperators[s.Subject]
	if !known || !contains(operators, s.Op) {
		return "", sentenceInvalid()
	}
	switch SentenceTakes(s.Subject, s.Op) {
	case "none":
		if s.A != "" || s.B != "" {
			return "", sentenceInvalid()
		}
	case "value":
		if !writtenValue(s.A) || s.B != "" {
			return "", sentenceInvalid()
		}
	case "key_value":
		if !writtenValue(s.A) || !writtenValue(s.B) {
			return "", sentenceInvalid()
		}
	case "number":
		if _, ok := bounded(s.A, 0, maxDepth); !ok || s.B != "" {
			return "", sentenceInvalid()
		}
	case "days":
		if _, ok := bounded(s.A, 0, maxDays); !ok || s.B != "" {
			return "", sentenceInvalid()
		}
	case "range":
		from, okFrom := bounded(s.A, 0, maxHour)
		to, okTo := bounded(s.B, 0, maxHour)
		if !okFrom || !okTo || from >= to {
			return "", sentenceInvalid()
		}
	}
	return compileSentence(s), nil
}

// compileSentence is the editor's `compileSentence`, for a sentence already checked.
func compileSentence(s Sentence) string {
	is := func(yes, no string) string {
		if s.Op == "is" {
			return yes
		}
		return no
	}
	switch s.Subject {
	case "type":
		return "item.type " + is("==", "!=") + " " + quote(s.A)
	case "title":
		if s.Op == "starts_with" {
			return "item.title.matches(" + insensitive(s.A, true) + ")"
		}
		negation := ""
		if s.Op == "not_contains" {
			negation = "!"
		}
		return negation + "item.title.matches(" + insensitive(s.A, false) + ")"
	case "notes":
		if s.Op == "contains" {
			return "item.notes.matches(" + insensitive(s.A, false) + ")"
		}
		if s.Op == "empty" {
			return "item.notes == ''"
		}
		return "item.notes != ''"
	case "completed", "archived":
		value := "false"
		if s.Op == "yes" {
			value = "true"
		}
		return "item." + s.Subject + " == " + value
	case "due":
		switch s.Op {
		case "has":
			return "has(item.due_at)"
		case "lacks":
			return "!has(item.due_at)"
		case "past":
			return "has(item.due_at) && item.due_at < now"
		case "future":
			return "has(item.due_at) && item.due_at > now"
		}
		days, _ := strconv.Atoi(s.A)
		return "has(item.due_at) && item.due_at < now + duration(" + quote(strconv.Itoa(days*24)+"h") + ")"
	case "label":
		negation := ""
		if s.Op == "not_on" {
			negation = "!"
		}
		return negation + "item.labels.exists(l, l == " + quote(s.A) + ")"
	case "parent":
		if s.Op == "has" {
			return "has(item.parent_id)"
		}
		return "!has(item.parent_id)"
	case "depth":
		depth, _ := strconv.Atoi(s.A)
		op := "<="
		if s.Op == "is" {
			op = "=="
		}
		return "item.depth " + op + " " + strconv.Itoa(depth)
	case "assignee":
		switch s.Op {
		case "has":
			return "has(item.assignee_id)"
		case "lacks":
			return "!has(item.assignee_id)"
		}
		return "item.assignee_id " + is("==", "!=") + " " + quote(s.A)
	case "bucket":
		return "item.bucket_id " + is("==", "!=") + " " + quote(s.A)
	case "actor":
		return "actor.id " + is("==", "!=") + " " + quote(s.A)
	case "hour":
		from, _ := strconv.Atoi(s.A)
		to, _ := strconv.Atoi(s.B)
		return "now.getHours() >= " + strconv.Itoa(from) + " && now.getHours() < " + strconv.Itoa(to)
	default: // field
		return "item.custom_fields[" + quote(s.A) + "] " + is("==", "!=") + " " + quote(s.B)
	}
}

// CompileCondition answers the expression a tree of sentences is - the editor's `compileNode` -
// or refuses it. An empty group compiles to the empty string, which is no condition.
func CompileCondition(node ConditionNode) (string, error) {
	switch {
	case node.Sentence != nil && node.Group == nil:
		return CompileSentence(*node.Sentence)
	case node.Group != nil && node.Sentence == nil:
	default:
		return "", sentenceInvalid()
	}
	group := node.Group
	if group.Mode != ModeAll && group.Mode != ModeAny && group.Mode != ModeNone {
		return "", sentenceInvalid()
	}
	parts := make([]string, 0, len(group.Items))
	for _, item := range group.Items {
		compiled, err := CompileCondition(item)
		if err != nil {
			return "", err
		}
		// A nested group and a sentence that is itself a join are parenthesised, so what the
		// writer grouped is what the engine groups.
		if item.Group != nil || hasTopLevelJoin(compiled) {
			compiled = "(" + compiled + ")"
		}
		if compiled == "" || compiled == "()" {
			continue
		}
		parts = append(parts, compiled)
	}
	if len(parts) == 0 {
		return "", nil
	}
	switch group.Mode {
	case ModeNone:
		return "!(" + strings.Join(parts, " || ") + ")", nil
	case ModeAll:
		return strings.Join(parts, " && "), nil
	default:
		return strings.Join(parts, " || "), nil
	}
}

// writtenValue is a value a sentence may carry: present, bounded, and on one line - a CEL string
// literal cannot hold a raw line break, and the editor's fields cannot type one.
func writtenValue(value string) bool {
	if value == "" || len([]rune(value)) > MaxSentenceText {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// bounded reads a whole number written in decimal digits and holds it to its range.
func bounded(value string, low, high int) (int, bool) {
	if value == "" || len(value) > 3 {
		return 0, false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < low || n > high {
		return 0, false
	}
	return n, true
}

func quote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return "'" + value + "'"
}

// regexMeta is the editor's escapeRegex set: an RE2 metacharacter in a value matches itself.
const regexMeta = `\^$.|?*+()[]{}`

func escapeRegex(value string) string {
	var escaped strings.Builder
	for _, r := range value {
		if strings.ContainsRune(regexMeta, r) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(r)
	}
	return escaped.String()
}

func insensitive(value string, anchored bool) string {
	anchor := ""
	if anchored {
		anchor = "^"
	}
	return quote("(?i)" + anchor + escapeRegex(value))
}

// hasTopLevelJoin reports whether an expression has `&&` or `||` outside every pair of parentheses
// and quotes. Bytes rather than runes: the delimiters are ASCII, and no byte of a multi-byte rune
// equals one.
func hasTopLevelJoin(expr string) bool {
	return len(splitTopLevel(expr, "&&")) > 1 || len(splitTopLevel(expr, "||")) > 1
}

func splitTopLevel(expr, join string) []string {
	var parts []string
	depth, start, quoted := 0, 0, false
	for at := 0; at < len(expr); at++ {
		char := expr[at]
		if quoted {
			switch char {
			case '\\':
				at++
			case '\'':
				quoted = false
			}
			continue
		}
		switch {
		case char == '\'':
			quoted = true
		case char == '(':
			depth++
		case char == ')':
			depth--
		case depth == 0 && strings.HasPrefix(expr[at:], join):
			parts = append(parts, expr[start:at])
			start = at + 2
			at++
		}
	}
	parts = append(parts, expr[start:])
	return parts
}
