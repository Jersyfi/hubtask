// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The query as one line of text (api-guidelines.md §3, "The query as a line"): what a person or an
// agent types or shares instead of the filter tree - `status = open AND custom_fields.priority >= 3
// ORDER BY due_at`. A line is only another spelling of the tree. ParseLine turns it into the same
// filter document and sort list the structured query sends, so the field catalogue, the operators a
// field permits, the bounds and the price are ParseFilter's and ParseSort's, never this grammar's.
const (
	// MaxLineLength bounds a line in bytes. Fifty nodes of a field, an operator and a value of
	// 500 characters do not need more.
	MaxLineLength = 4000
	// maxLineNesting bounds the parentheses before the tree's own depth is counted, so a line of
	// ten thousand `(` ends here instead of in the recursion.
	maxLineNesting = 2 * MaxFilterDepth
)

// ParseLine reads a query line into a filter document and a sort list, each nil where the line
// holds none. path is where the line sits in the request; a refusal names it with the character
// it stopped at.
func ParseLine(line, path string) (filter any, sort []any, err error) {
	if len(line) > MaxLineLength {
		return nil, nil, fieldError(path, "query.line_too_long", map[string]string{"maximum": strconv.Itoa(MaxLineLength)})
	}
	if !utf8.ValidString(line) {
		return nil, nil, lineError(path, "", 0)
	}
	tokens, at, ok := lex(line)
	if !ok {
		return nil, nil, lineError(path, line, at)
	}
	p := &lineParser{tokens: tokens, path: path, line: line, end: len(line)}
	if !p.atKeyword("ORDER") && !p.done() {
		node, err := p.or(0)
		if err != nil {
			return nil, nil, err
		}
		filter = node
	}
	if p.atKeyword("ORDER") {
		p.next()
		if !p.atKeyword("BY") {
			return nil, nil, p.fail()
		}
		p.next()
		if sort, err = p.sortList(); err != nil {
			return nil, nil, err
		}
	}
	if !p.done() {
		return nil, nil, p.fail()
	}
	return filter, sort, nil
}

// lineError names where the line stops being readable as the character a person counts, the first
// being 1.
func lineError(path, line string, at int) error {
	return fieldError(path, "query.line_malformed", map[string]string{"at": strconv.Itoa(utf8.RuneCountInString(line[:at]) + 1)})
}

type tokenKind int

const (
	tokenWord tokenKind = iota
	tokenText
	tokenSymbol
)

type token struct {
	kind tokenKind
	text string
	at   int
}

// lex splits a line into words (a field, a keyword, a bare value), quoted texts and symbols. It
// answers the offset of what it cannot read.
func lex(line string) ([]token, int, bool) {
	var tokens []token
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		switch {
		case unicode.IsSpace(r):
			i += size
		case r == '\'' || r == '"':
			text, end, ok := quoted(line, i)
			if !ok {
				return nil, i, false
			}
			tokens = append(tokens, token{kind: tokenText, text: text, at: i})
			i = end
		case r == '(' || r == ')' || r == ',' || r == '~':
			tokens = append(tokens, token{kind: tokenSymbol, text: string(r), at: i})
			i++
		case r == '=' || r == '!' || r == '<' || r == '>':
			symbol := string(r)
			if i+1 < len(line) && line[i+1] == '=' {
				symbol += "="
			}
			if symbol == "!" {
				return nil, i, false
			}
			tokens = append(tokens, token{kind: tokenSymbol, text: symbol, at: i})
			i += len(symbol)
		default:
			start := i
			for i < len(line) {
				r, size := utf8.DecodeRuneInString(line[i:])
				if unicode.IsSpace(r) || strings.ContainsRune("()'\",~=!<>", r) {
					break
				}
				i += size
			}
			if i == start {
				return nil, i, false
			}
			tokens = append(tokens, token{kind: tokenWord, text: line[start:i], at: start})
		}
	}
	return tokens, 0, true
}

// quoted reads a text in the quote at start; a backslash takes the next character as it is.
func quoted(line string, start int) (string, int, bool) {
	quote := line[start]
	var b strings.Builder
	for i := start + 1; i < len(line); i++ {
		switch c := line[i]; {
		case c == '\\' && i+1 < len(line):
			i++
			b.WriteByte(line[i])
		case c == quote:
			return b.String(), i + 1, true
		default:
			b.WriteByte(c)
		}
	}
	return "", start, false
}

type lineParser struct {
	tokens []token
	pos    int
	path   string
	line   string
	end    int
}

func (p *lineParser) done() bool { return p.pos >= len(p.tokens) }

func (p *lineParser) peek() token {
	if p.done() {
		return token{kind: tokenSymbol, at: p.end}
	}
	return p.tokens[p.pos]
}

func (p *lineParser) next() token {
	t := p.peek()
	p.pos++
	return t
}

func (p *lineParser) fail() error { return lineError(p.path, p.line, p.peek().at) }

func (p *lineParser) atKeyword(word string) bool {
	t := p.peek()
	return t.kind == tokenWord && strings.EqualFold(t.text, word)
}

func (p *lineParser) atSymbol(symbol string) bool {
	t := p.peek()
	return !p.done() && t.kind == tokenSymbol && t.text == symbol
}

// or, and and not are the precedence levels: NOT binds tighter than AND, AND tighter than OR. A
// run of one operator is one combination, so `a AND b AND c` costs one level of depth, not two.
func (p *lineParser) or(nesting int) (any, error) {
	return p.combination("OR", nesting, p.and)
}

func (p *lineParser) and(nesting int) (any, error) {
	return p.combination("AND", nesting, p.not)
}

func (p *lineParser) combination(op string, nesting int, operand func(int) (any, error)) (any, error) {
	first, err := operand(nesting)
	if err != nil {
		return nil, err
	}
	nodes := []any{first}
	for p.atKeyword(op) {
		p.next()
		node, err := operand(nesting)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 1 {
		return first, nil
	}
	return map[string]any{"op": op, "nodes": nodes}, nil
}

func (p *lineParser) not(nesting int) (any, error) {
	if p.atKeyword("NOT") {
		p.next()
		if nesting >= maxLineNesting {
			return nil, p.fail()
		}
		node, err := p.not(nesting + 1)
		if err != nil {
			return nil, err
		}
		return map[string]any{"op": "NOT", "nodes": []any{node}}, nil
	}
	if p.atSymbol("(") {
		p.next()
		if nesting >= maxLineNesting {
			return nil, p.fail()
		}
		node, err := p.or(nesting + 1)
		if err != nil {
			return nil, err
		}
		if !p.atSymbol(")") {
			return nil, p.fail()
		}
		p.next()
		return node, nil
	}
	return p.comparison()
}

// symbols are the comparisons written as a sign.
var symbols = map[string]Operator{
	"=": OpEq, "!=": OpNeq, "<": OpLt, "<=": OpLte, ">": OpGt, ">=": OpGte, "~": OpContains,
}

// comparison reads `field op value` in one of its spellings.
func (p *lineParser) comparison() (any, error) {
	name := p.peek()
	if name.kind != tokenWord || isKeyword(name.text) {
		return nil, p.fail()
	}
	p.next()
	leaf := map[string]any{"field": name.text}
	target, _ := field(name.text, p.path)
	op := p.peek()
	switch {
	case op.kind == tokenSymbol && symbols[op.text] != "":
		p.next()
		value, err := p.value(target)
		if err != nil {
			return nil, err
		}
		leaf["op"], leaf["value"] = string(symbols[op.text]), value
	case p.atKeyword("IN"):
		p.next()
		return p.list(leaf, OpIn, target)
	case p.atKeyword("NOT"):
		p.next()
		if !p.atKeyword("IN") {
			return nil, p.fail()
		}
		p.next()
		return p.list(leaf, OpNotIn, target)
	case p.atKeyword("IS"):
		p.next()
		negated := p.atKeyword("NOT")
		if negated {
			p.next()
		}
		if !p.atKeyword("EMPTY") && !p.atKeyword("NULL") {
			return nil, p.fail()
		}
		p.next()
		leaf["op"] = string(OpIsNull)
		if negated {
			return map[string]any{"op": "NOT", "nodes": []any{leaf}}, nil
		}
	case p.atKeyword("BETWEEN"):
		p.next()
		low, err := p.value(target)
		if err != nil {
			return nil, err
		}
		if !p.atKeyword("AND") {
			return nil, p.fail()
		}
		p.next()
		high, err := p.value(target)
		if err != nil {
			return nil, err
		}
		leaf["op"], leaf["value"] = string(OpBetween), []any{low, high}
	case p.atKeyword("CONTAINS"):
		p.next()
		switch {
		case p.atKeyword("ANY"):
			p.next()
			return p.list(leaf, OpContainsAny, target)
		case p.atKeyword("ALL"):
			p.next()
			return p.list(leaf, OpContainsAll, target)
		}
		value, err := p.value(target)
		if err != nil {
			return nil, err
		}
		leaf["op"], leaf["value"] = string(OpContains), value
	case p.atKeyword("STARTS"):
		p.next()
		if !p.atKeyword("WITH") {
			return nil, p.fail()
		}
		p.next()
		value, err := p.value(target)
		if err != nil {
			return nil, err
		}
		leaf["op"], leaf["value"] = string(OpStartsWith), value
	case p.atKeyword("MATCHES"):
		p.next()
		value, err := p.value(target)
		if err != nil {
			return nil, err
		}
		leaf["op"], leaf["value"] = string(OpMatches), value
	default:
		return nil, p.fail()
	}
	return leaf, nil
}

// list reads `( value, value … )` for a list operator.
func (p *lineParser) list(leaf map[string]any, op Operator, target Field) (any, error) {
	if !p.atSymbol("(") {
		return nil, p.fail()
	}
	p.next()
	var values []any
	for {
		if len(values) > MaxValues {
			return nil, p.fail()
		}
		value, err := p.value(target)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		if p.atSymbol(",") {
			p.next()
			continue
		}
		if !p.atSymbol(")") {
			return nil, p.fail()
		}
		p.next()
		break
	}
	leaf["op"], leaf["value"] = string(op), values
	return leaf, nil
}

// value reads one value as the document would hold it for the field: a quoted text is a text,
// and a bare word is read by the field's kind - a number for a number, true or false for a flag,
// and for a custom field, whose kind no column pins, by what it looks like. An unknown field reads
// it as text; ParseFilter refuses the field by name afterwards.
func (p *lineParser) value(target Field) (any, error) {
	t := p.peek()
	switch t.kind {
	case tokenText:
		p.next()
		return t.text, nil
	case tokenWord:
		if isKeyword(t.text) {
			return nil, p.fail()
		}
		p.next()
		return bareValue(t.text, target.Kind), nil
	}
	return nil, p.fail()
}

func bareValue(word string, kind Kind) any {
	switch kind {
	case KindInt:
		if number, err := strconv.ParseInt(word, 10, 64); err == nil {
			return float64(number)
		}
	case KindBool:
		if flag, err := strconv.ParseBool(word); err == nil && (word == "true" || word == "false") {
			return flag
		}
	case KindCustom:
		if word == "true" || word == "false" {
			return word == "true"
		}
		if number, err := strconv.ParseFloat(word, 64); err == nil {
			return number
		}
	}
	return word
}

// sortList reads `field [ASC|DESC] [NULLS FIRST|LAST], …`.
func (p *lineParser) sortList() ([]any, error) {
	var terms []any
	for {
		name := p.peek()
		if name.kind != tokenWord || isKeyword(name.text) || len(terms) > MaxSortTerms {
			return nil, p.fail()
		}
		p.next()
		term := map[string]any{"field": name.text}
		if p.atKeyword("ASC") || p.atKeyword("DESC") {
			term["dir"] = strings.ToUpper(p.next().text)
		}
		if p.atKeyword("NULLS") {
			p.next()
			if !p.atKeyword("FIRST") && !p.atKeyword("LAST") {
				return nil, p.fail()
			}
			term["nulls"] = strings.ToUpper(p.next().text)
		}
		terms = append(terms, term)
		if !p.atSymbol(",") {
			return terms, nil
		}
		p.next()
	}
}

// keywords are the words a line's grammar reserves; a field or a bare value is never one of them,
// and a text that is one is quoted.
var keywords = map[string]bool{
	"AND": true, "OR": true, "NOT": true, "IN": true, "IS": true, "EMPTY": true, "NULL": true,
	"BETWEEN": true, "CONTAINS": true, "ANY": true, "ALL": true, "STARTS": true, "WITH": true,
	"MATCHES": true, "ORDER": true, "BY": true, "ASC": true, "DESC": true, "NULLS": true,
	"FIRST": true, "LAST": true,
}

func isKeyword(word string) bool { return keywords[strings.ToUpper(word)] }

// operatorWords are the comparisons as a line spells them back.
var operatorWords = map[Operator]string{
	OpEq: "=", OpNeq: "!=", OpLt: "<", OpLte: "<=", OpGt: ">", OpGte: ">=", OpContains: "CONTAINS",
	OpStartsWith: "STARTS WITH", OpMatches: "MATCHES", OpIn: "IN", OpNotIn: "NOT IN",
	OpContainsAny: "CONTAINS ANY", OpContainsAll: "CONTAINS ALL",
}

// FormatLine writes a filter document and a sort list as a line ParseLine reads back to the same
// document - a saved filter shared as one line. ok is false for a document no line spells (an
// unknown operator, a malformed node).
func FormatLine(filter any, sort []any) (string, bool) {
	var b strings.Builder
	if filter != nil {
		if !formatNode(&b, filter, false) {
			return "", false
		}
	}
	if len(sort) > 0 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("ORDER BY ")
		for i, raw := range sort {
			term, isObject := raw.(map[string]any)
			name, _ := term["field"].(string)
			if !isObject || !bareField(name) {
				return "", false
			}
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(name)
			if dir, _ := term["dir"].(string); dir != "" {
				b.WriteString(" " + dir)
			}
			if nulls, _ := term["nulls"].(string); nulls != "" {
				b.WriteString(" NULLS " + nulls)
			}
		}
	}
	return b.String(), true
}

func formatNode(b *strings.Builder, raw any, nested bool) bool {
	node, isObject := raw.(map[string]any)
	if !isObject {
		return false
	}
	op, _ := node["op"].(string)
	switch Operator(op) {
	case OpAnd, OpOr:
		nodes, _ := node["nodes"].([]any)
		if len(nodes) < 2 {
			return false
		}
		if nested {
			b.WriteByte('(')
		}
		for i, child := range nodes {
			if i > 0 {
				b.WriteString(" " + op + " ")
			}
			if !formatNode(b, child, true) {
				return false
			}
		}
		if nested {
			b.WriteByte(')')
		}
		return true
	case OpNot:
		nodes, _ := node["nodes"].([]any)
		if len(nodes) != 1 {
			return false
		}
		b.WriteString("NOT ")
		return formatNode(b, nodes[0], true)
	}
	name, _ := node["field"].(string)
	if !bareField(name) {
		return false
	}
	b.WriteString(name)
	value := node["value"]
	switch Operator(op) {
	case OpIsNull:
		b.WriteString(" IS EMPTY")
		return true
	case OpBetween:
		values, _ := value.([]any)
		if len(values) != 2 {
			return false
		}
		b.WriteString(" BETWEEN " + formatValue(values[0]) + " AND " + formatValue(values[1]))
		return true
	case OpIn, OpNotIn, OpContainsAny, OpContainsAll:
		values, isList := value.([]any)
		if !isList {
			values = []any{value}
		}
		parts := make([]string, len(values))
		for i, v := range values {
			parts[i] = formatValue(v)
		}
		b.WriteString(" " + operatorWords[Operator(op)] + " (" + strings.Join(parts, ", ") + ")")
		return true
	}
	word, known := operatorWords[Operator(op)]
	if !known {
		return false
	}
	b.WriteString(" " + word + " " + formatValue(value))
	return true
}

// formatValue quotes a text always, so a text that looks like a number or a keyword stays a text.
func formatValue(raw any) string {
	switch value := raw.(type) {
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case string:
		escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value)
		return "'" + escaped + "'"
	}
	return "''"
}

// bareField reports whether a field name is written as a word of the line.
func bareField(name string) bool {
	if name == "" || isKeyword(name) {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("()'\",~=!<>", r)
	})
}
