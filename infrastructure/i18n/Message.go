// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

// ICU MessageFormat, the subset the product's catalogues use, rendered on the server.
//
// The same subset apps/webapp/src/lib/i18n/format.ts implements, construct for construct, so that
// a message a translator writes renders the same in an email as on a screen: simple arguments,
// `plural` with `offset:` and `=n`, `selectordinal`, `select`, `#` inside a plural, nested
// messages in every branch, and ICU's apostrophe quoting. Everything else - `number`, `date`,
// `time` and any other argument type - is refused by name when the message is parsed, which is
// what lets M-03's gate hear the refusal when the message is written rather than when somebody
// reads Polish (i18n-l10n.md §3).
//
// The categories come from golang.org/x/text/feature/plural, the CLDR data Intl.PluralRules
// carries in a browser (ADR-0056): the one part of this that cannot be got right by hand.
//
// One deliberate difference from the client: a number is written as it was given. The client
// groups digits with Intl.NumberFormat; this side has no number symbols and does not pretend to
// (milestone-0.8.0.md, decision 8), and the few numbers the server renders are counts and limits.

// MessageSyntaxError is what parsing refuses with. The message names the construct, because
// "unsupported syntax" sends a reader looking and "`{n, number}` is not implemented" sends them to
// the one line to change.
type MessageSyntaxError struct {
	What    string
	At      int
	Pattern string
}

func (e *MessageSyntaxError) Error() string {
	return fmt.Sprintf("%s (at %d in `%s`)", e.What, e.At, e.Pattern)
}

type nodeKind int

const (
	nodeText nodeKind = iota
	nodeArgument
	nodeNumber
	nodePlural
	nodeSelect
)

type node struct {
	kind nodeKind
	// value is the text of a text node.
	value string
	// name is the argument's name for every other kind.
	name string
	// offset is the plural's `offset:`, carried by the plural and by every `#` inside it.
	offset int
	// ordinal says whether a plural node selects by the ordinal rules.
	ordinal bool
	// exact holds a plural's `=n` branches; categories its CLDR ones; options a select's.
	exact      map[float64][]node
	categories map[string][]node
	options    map[string][]node
}

// pluralCategories is what a plural branch may be named. An unknown keyword is a typo, and typos
// are loud.
var pluralCategories = map[string]bool{
	"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true,
}

type parser struct {
	pattern []rune
	at      int
	source  string
}

type pluralContext struct {
	name   string
	offset int
}

// parseMessage parses a pattern, or refuses it.
func parseMessage(pattern string) ([]node, error) {
	p := &parser{pattern: []rune(pattern), source: pattern}
	nodes, err := p.message(nil, 0)
	if err != nil {
		return nil, err
	}
	if p.at < len(p.pattern) {
		return nil, p.fail("a `}` with no `{` before it")
	}
	return nodes, nil
}

func (p *parser) fail(what string) error {
	return &MessageSyntaxError{What: what, At: p.at, Pattern: p.source}
}

func (p *parser) peek() (rune, bool) {
	if p.at >= len(p.pattern) {
		return 0, false
	}
	return p.pattern[p.at], true
}

// message is the top level, and every branch body: text until an unmatched `}` or the end.
func (p *parser) message(inside *pluralContext, depth int) ([]node, error) {
	var nodes []node
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			nodes = append(nodes, node{kind: nodeText, value: text.String()})
			text.Reset()
		}
	}

	for {
		char, more := p.peek()
		if !more {
			break
		}
		switch {
		case char == '}':
			if depth == 0 {
				return nil, p.fail("a `}` with no `{` before it")
			}
			flush()
			return nodes, nil
		case char == '\'':
			text.WriteString(p.quoted())
		case char == '#' && inside != nil:
			flush()
			nodes = append(nodes, node{kind: nodeNumber, name: inside.name, offset: inside.offset})
			p.at++
		case char == '{':
			flush()
			argument, err := p.argument(depth)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, argument)
		default:
			text.WriteRune(char)
			p.at++
		}
	}
	flush()
	return nodes, nil
}

// quoted is ICU's apostrophe rule: `”` is one apostrophe, an apostrophe before `{`, `}` or `#`
// quotes everything up to the next one, and an apostrophe anywhere else is just an apostrophe.
func (p *parser) quoted() string {
	p.at++ // the opening apostrophe
	next, more := p.peek()
	if !more {
		return "'"
	}
	if next == '\'' {
		p.at++
		return "'"
	}
	if next != '{' && next != '}' && next != '#' {
		return "'"
	}

	var out strings.Builder
	for {
		char, more := p.peek()
		if !more {
			// An unterminated run reads to the end, which is what ICU does.
			return out.String()
		}
		if char == '\'' {
			if p.at+1 < len(p.pattern) && p.pattern[p.at+1] == '\'' {
				out.WriteRune('\'')
				p.at += 2
				continue
			}
			p.at++
			return out.String()
		}
		out.WriteRune(char)
		p.at++
	}
}

func isNameRune(r rune) bool {
	return !unicode.IsSpace(r) && r != ',' && r != '{' && r != '}' && r != '#'
}

func (p *parser) argument(depth int) (node, error) {
	p.at++ // `{`
	rawName, err := p.until(',', '}')
	if err != nil {
		return node{}, err
	}
	name := strings.TrimSpace(rawName)
	if name == "" || strings.ContainsFunc(name, func(r rune) bool { return !isNameRune(r) }) {
		return node{}, p.fail(fmt.Sprintf("`%s` is not an argument name", name))
	}

	if char, _ := p.peek(); char == '}' {
		p.at++
		return node{kind: nodeArgument, name: name}, nil
	}

	p.at++ // `,`
	rawType, err := p.until(',', '}')
	if err != nil {
		return node{}, err
	}
	kind := strings.TrimSpace(rawType)

	switch kind {
	case "select":
		if err := p.expect(','); err != nil {
			return node{}, err
		}
		branches, err := p.branches(nil, depth)
		if err != nil {
			return node{}, err
		}
		if _, ok := branches["other"]; !ok {
			return node{}, p.fail(fmt.Sprintf("the select `%s` has no `other` branch", name))
		}
		return node{kind: nodeSelect, name: name, options: branches}, nil

	case "plural", "selectordinal":
		if err := p.expect(','); err != nil {
			return node{}, err
		}
		offset, err := p.offset()
		if err != nil {
			return node{}, err
		}
		branches, err := p.branches(&pluralContext{name: name, offset: offset}, depth)
		if err != nil {
			return node{}, err
		}
		exact := map[float64][]node{}
		categories := map[string][]node{}
		for key, body := range branches {
			switch {
			case strings.HasPrefix(key, "="):
				value, err := strconv.ParseFloat(key[1:], 64)
				if err != nil || math.IsInf(value, 0) {
					return node{}, p.fail(fmt.Sprintf("`%s` is not an exact plural match", key))
				}
				exact[value] = body
			case pluralCategories[key]:
				categories[key] = body
			default:
				return node{}, p.fail(fmt.Sprintf("`%s` is not a CLDR plural category", key))
			}
		}
		if _, ok := categories["other"]; !ok {
			return node{}, p.fail(fmt.Sprintf("the plural `%s` has no `other` branch", name))
		}
		return node{
			kind: nodePlural, name: name, ordinal: kind == "selectordinal", offset: offset,
			exact: exact, categories: categories,
		}, nil
	}

	// The refusal the client makes too, and for the same reason: naming the type is the point.
	return node{}, p.fail(fmt.Sprintf("`{%s, %s}` is not implemented by this renderer", name, kind))
}

func (p *parser) offset() (int, error) {
	save := p.at
	p.skipSpace()
	const keyword = "offset:"
	if !strings.HasPrefix(string(p.pattern[p.at:]), keyword) {
		p.at = save
		return 0, nil
	}
	p.at += len(keyword)
	digits, err := p.until(' ', '\t', '\n', '{')
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(strings.TrimSpace(digits))
	if err != nil {
		return 0, p.fail(fmt.Sprintf("`%s` is not an offset", digits))
	}
	return value, nil
}

func (p *parser) branches(inside *pluralContext, depth int) (map[string][]node, error) {
	branches := map[string][]node{}
	for {
		p.skipSpace()
		char, more := p.peek()
		if !more {
			return nil, p.fail("the argument is not closed")
		}
		if char == '}' {
			p.at++
			return branches, nil
		}
		rawKey, err := p.until('{', ' ', '\t', '\n')
		if err != nil {
			return nil, err
		}
		key := strings.TrimSpace(rawKey)
		p.skipSpace()
		if char, _ := p.peek(); char != '{' {
			return nil, p.fail(fmt.Sprintf("the branch `%s` has no body", key))
		}
		p.at++
		body, err := p.message(inside, depth+1)
		if err != nil {
			return nil, err
		}
		if char, _ := p.peek(); char != '}' {
			return nil, p.fail(fmt.Sprintf("the branch `%s` is not closed", key))
		}
		p.at++
		branches[key] = body
	}
}

func (p *parser) until(stops ...rune) (string, error) {
	start := p.at
	for p.at < len(p.pattern) {
		for _, stop := range stops {
			if p.pattern[p.at] == stop {
				return string(p.pattern[start:p.at]), nil
			}
		}
		p.at++
	}
	return "", p.fail("the argument is not closed")
}

func (p *parser) expect(char rune) error {
	p.skipSpace()
	if got, _ := p.peek(); got != char {
		return p.fail(fmt.Sprintf("expected `%c`", char))
	}
	p.at++
	return nil
}

func (p *parser) skipSpace() {
	for p.at < len(p.pattern) && unicode.IsSpace(p.pattern[p.at]) {
		p.at++
	}
}

// render walks the nodes. A parameter that is not there leaves its placeholder standing rather
// than blanking it: `{request_id}` on the screen says a value went missing, an empty gap says
// nothing at all.
func render(nodes []node, params map[string]string, tag language.Tag) string {
	var out strings.Builder
	for _, n := range nodes {
		switch n.kind {
		case nodeText:
			out.WriteString(n.value)
		case nodeArgument:
			if value, ok := params[n.name]; ok {
				out.WriteString(value)
			} else {
				out.WriteString("{" + n.name + "}")
			}
		case nodeNumber:
			value, ok := numberOf(params[n.name])
			if !ok {
				out.WriteString("{" + n.name + "}")
				continue
			}
			out.WriteString(formatNumber(value - float64(n.offset)))
		case nodePlural:
			value, ok := numberOf(params[n.name])
			if !ok {
				out.WriteString("{" + n.name + "}")
				continue
			}
			body, matched := n.exact[value]
			if !matched {
				body, matched = n.categories[category(tag, value-float64(n.offset), n.ordinal)]
			}
			if !matched {
				body = n.categories["other"]
			}
			out.WriteString(render(body, params, tag))
		case nodeSelect:
			body, matched := n.options[params[n.name]]
			if !matched {
				body = n.options["other"]
			}
			out.WriteString(render(body, params, tag))
		}
	}
	return out.String()
}

// numberOf reads a parameter as a number. The port carries strings, so a plural's operand is
// parsed here; a value that is not a number leaves the placeholder standing, like an absent one.
func numberOf(raw string) (float64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, false
	}
	return value, true
}

// formatNumber writes a number as it was given: an integer without a decimal point, anything
// else with the digits it has. No grouping - see the note at the top of the file.
func formatNumber(value float64) string {
	if value == math.Trunc(value) && math.Abs(value) < 1e15 {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// category answers the CLDR plural category of a number in a language, from the operands TR 35
// defines: i the integer digits, v the count of visible fraction digits, f the fraction digits
// as an integer. w and t are the same without trailing zeros, which a parsed float has none of.
func category(tag language.Tag, value float64, ordinal bool) string {
	text := strconv.FormatFloat(math.Abs(value), 'f', -1, 64)
	integerPart, fractionPart, _ := strings.Cut(text, ".")
	i, _ := strconv.Atoi(integerPart)
	v := len(fractionPart)
	f, _ := strconv.Atoi(fractionPart)

	rules := plural.Cardinal
	if ordinal {
		rules = plural.Ordinal
	}
	switch rules.MatchPlural(tag, i, v, v, f, f) {
	case plural.Zero:
		return "zero"
	case plural.One:
		return "one"
	case plural.Two:
		return "two"
	case plural.Few:
		return "few"
	case plural.Many:
		return "many"
	default:
		return "other"
	}
}
