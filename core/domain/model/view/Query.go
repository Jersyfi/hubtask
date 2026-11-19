// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Query is a Hubtask query read from text (ADR-0102, api-guidelines.md §3.1): the filter document
// and sort list ParseFilter and ParseSort take, plus what the tree has no node for - the free words
// a search ranks, the kinds of object asked for, and the two flags that widen what is seen.
type Query struct {
	// Filter is the filter document, nil where the text holds none.
	Filter any
	// Sort is the sort list, nil where the text holds none.
	Sort []any
	// Words are the free words and phrases of the top level, in the syntax websearch_to_tsquery
	// reads. Words inside a group are MATCHES leaves of the filter instead.
	Words string
	// Kinds are the objects `type:` asks for beyond the entry levels; empty asks for every kind.
	Kinds           []string
	IncludeArchived bool
	IncludeTrashed  bool
}

// ObjectKinds are the kinds `type:` names. The first three are entry levels and filter on the
// entry's `type`; the others choose which kinds a search reads.
var ObjectKinds = []string{
	"task", "workpackage", "activity", "ticket", "page", "whiteboard", "post", "goal", "dashboard",
	"asset", "file", "person", "comment", "collection", "hub",
}

var entryLevels = map[string]string{"task": "TASK", "workpackage": "WORK_PACKAGE", "activity": "ACTIVITY"}

// readerKeys are the fields as a person types them; every catalogue name is a key as well, so a
// tree with a field no reader key names is still written back.
var readerKeys = map[string]string{
	"status": FieldStatus, "type": FieldType, "who": FieldAssigneeID, "with": FieldMembers,
	"by": FieldCreatedBy, "in": FieldCollection, "label": FieldLabels, "created": FieldCreatedAt,
	"updated": FieldUpdatedAt, "due": FieldDueAt, "start": FieldStartAt, "done": FieldCompletedAt,
	"title": FieldTitle, "text": FieldText, "note": FieldNotes, "linked": FieldLinkedTo,
	"parent": FieldParentID, "desk.sla": FieldSLADue, "desk.standing": FieldSLAStanding,
	"desk.satisfaction": FieldSatisfaction, "hub": FieldHub, "under": FieldUnder, "blocks": FieldBlocks,
	"affects": FieldAffects, "plan.sprint": FieldSprint, "plan.release": FieldRelease, "plan.slip": FieldSlip,
	"plan.critical": FieldCritical, "time.logged": FieldTimeLogged,
}

// fieldKeys is readerKeys turned round, for the formatter.
var fieldKeys = func() map[string]string {
	keys := map[string]string{}
	for key, name := range readerKeys {
		keys[name] = key
	}
	return keys
}()

// CustomKeyPrefix is a custom field's prefix in the text; the tree says custom_fields.<key>.
const CustomKeyPrefix = "cf."

// isWords are the values of `is:`: a state of the entry, or a flag that widens the search.
var isWords = map[string]struct {
	field string
	value bool
}{
	"open": {FieldIsCompleted, false}, "done": {FieldIsCompleted, true},
	"blocked": {FieldBlocked, true}, "blocking": {FieldBlocking, true},
}

// periods are the named spans a time field takes bare.
var periods = map[string][2]string{
	"today":      {"@today", "@end_of_day"},
	"this-week":  {"@start_of_week", "@end_of_week"},
	"last-week":  {"@start_of_week-P1W", "@start_of_week-PT1S"},
	"next-week":  {"@start_of_week+P1W", "@end_of_week+P1W"},
	"this-month": {"@start_of_month", "@end_of_month"},
	"last-month": {"@start_of_month-P1M", "@start_of_month-PT1S"},
}

// anchors are the time words that stand for one moment.
var anchors = map[string]string{
	"now": "@now", "today": "@today", "end-of-day": "@end_of_day", "start-of-week": "@start_of_week",
	"end-of-week": "@end_of_week", "start-of-month": "@start_of_month", "end-of-month": "@end_of_month",
}

var (
	fieldMarker = regexp.MustCompile(`^(-?)([A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)*):`)
	relative    = regexp.MustCompile(`^([+-])(\d{1,4})([dwmy])$`)
	dateOnly    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	todayOffset = regexp.MustCompile(`^@today([+-])P(\d{1,4})([DWMY])$`)
)

// ParseQuery reads a query in either spelling: the `field: value` text or the `=`/`AND`/`ORDER
// BY` line (ParseLine). A text that marks a field with a colon is the first; one that compares
// with a sign or a keyword and has no such mark is the second; free words alone are the first.
func ParseQuery(text, path string) (Query, error) {
	if len(text) > MaxLineLength {
		return Query{}, fieldError(path, "query.line_too_long", map[string]string{"maximum": strconv.Itoa(MaxLineLength)})
	}
	if !utf8.ValidString(text) {
		return Query{}, lineError(path, "", 0)
	}
	if strings.TrimSpace(text) == "" {
		return Query{}, nil
	}
	if comparisonLine(text) {
		filter, sort, err := ParseLine(text, path)
		return Query{Filter: filter, Sort: sort}, err
	}
	tokens, at, ok := lexQuery(text)
	if !ok {
		return Query{}, queryError(path, text, at, "query.line_malformed", nil)
	}
	p := &queryParser{tokens: tokens, path: path, text: text}
	q, err := p.query()
	if err != nil {
		return Query{}, err
	}
	return q, nil
}

// comparisonLine reports whether a text is the `=` line: it lexes as one, compares at least once
// with a sign or a comparison keyword, and marks no field with a colon.
func comparisonLine(text string) bool {
	tokens, _, ok := lex(text)
	if !ok {
		// A line that stops being readable is answered by the grammar it looks like, so the
		// error names the character there.
		for _, word := range strings.Fields(text) {
			if fieldMarker.MatchString(word) {
				return false
			}
		}
		return strings.ContainsAny(text, "=<>~'")
	}
	compares := false
	for _, t := range tokens {
		switch {
		case t.kind == tokenWord && fieldMarker.MatchString(t.text):
			return false
		case t.kind == tokenSymbol && symbols[t.text] != "":
			compares = true
		case t.kind == tokenWord && slices.Contains([]string{"IN", "IS", "BETWEEN", "CONTAINS", "STARTS", "MATCHES", "ORDER"}, strings.ToUpper(t.text)):
			compares = true
		}
	}
	return compares
}

type qToken struct {
	kind tokenKind
	text string
	at   int
	// joined says the token follows the one before it with no space: `-(` and `-"…"`.
	joined bool
}

// lexQuery splits a text into words, quoted texts and the symbols ( ) and comma.
func lexQuery(text string) ([]qToken, int, bool) {
	var tokens []qToken
	end := -1
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		joined := i == end
		switch {
		case unicode.IsSpace(r):
			i += size
			continue
		case r == '"':
			var b strings.Builder
			closed := false
			j := i + 1
			for ; j < len(text); j++ {
				if text[j] == '\\' && j+1 < len(text) {
					j++
					b.WriteByte(text[j])
					continue
				}
				if text[j] == '"' {
					closed = true
					break
				}
				b.WriteByte(text[j])
			}
			if !closed {
				return nil, i, false
			}
			tokens = append(tokens, qToken{kind: tokenText, text: b.String(), at: i, joined: joined})
			i = j + 1
		case r == '(' || r == ')' || r == ',':
			tokens = append(tokens, qToken{kind: tokenSymbol, text: string(r), at: i, joined: joined})
			i++
		default:
			start := i
			for i < len(text) {
				r, size := utf8.DecodeRuneInString(text[i:])
				if unicode.IsSpace(r) || strings.ContainsRune(`(),"`, r) {
					break
				}
				i += size
			}
			tokens = append(tokens, qToken{kind: tokenWord, text: text[start:i], at: start, joined: joined})
		}
		end = i
	}
	return tokens, 0, true
}

type queryParser struct {
	tokens []qToken
	pos    int
	path   string
	text   string
	q      Query
}

// item is one operand of an AND: a node of the tree, or a free word as websearch reads it.
type item struct {
	node  any
	piece string
}

func (p *queryParser) done() bool { return p.pos >= len(p.tokens) }

func (p *queryParser) peek() qToken {
	if p.done() {
		return qToken{kind: tokenSymbol, at: len(p.text)}
	}
	return p.tokens[p.pos]
}

func (p *queryParser) atWord(word string) bool {
	t := p.peek()
	return !p.done() && t.kind == tokenWord && t.text == word
}

func (p *queryParser) atSymbol(symbol string) bool {
	t := p.peek()
	return !p.done() && t.kind == tokenSymbol && t.text == symbol
}

func (p *queryParser) fail(at int, code string, params map[string]string) error {
	return queryError(p.path, p.text, at, code, params)
}

// queryError names the character a person counts, the first being 1, and what fixes it.
func queryError(path, text string, at int, code string, params map[string]string) error {
	if params == nil {
		params = map[string]string{}
	}
	params["at"] = strconv.Itoa(utf8.RuneCountInString(text[:at]) + 1)
	return fieldError(path, code, params)
}

func (p *queryParser) query() (Query, error) {
	items, err := p.and(0, true)
	if err != nil {
		return Query{}, err
	}
	if !p.done() {
		return Query{}, p.fail(p.peek().at, "query.line_malformed", nil)
	}
	var nodes []any
	var words []string
	for _, it := range items {
		if it.node != nil {
			nodes = append(nodes, it.node)
		} else {
			words = append(words, it.piece)
		}
	}
	p.q.Words = strings.Join(words, " ")
	if p.q.Filter == nil {
		p.q.Filter = joinAnd(nodes)
	}
	return p.q, nil
}

func joinAnd(nodes []any) any {
	switch len(nodes) {
	case 0:
		return nil
	case 1:
		return nodes[0]
	}
	return map[string]any{"op": "AND", "nodes": nodes}
}

// or reads `and (OR and)*`; a top-level OR makes the whole text one node.
func (p *queryParser) or(nesting int) (any, error) {
	nodes, err := p.branches(nesting)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 1 {
		return nodes[0], nil
	}
	return map[string]any{"op": "OR", "nodes": nodes}, nil
}

// branches reads the operands of an OR.
func (p *queryParser) branches(nesting int) ([]any, error) {
	var nodes []any
	for {
		items, err := p.and(nesting, false)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, joinAnd(nodesOf(items)))
		if !p.atWord("OR") {
			break
		}
		p.pos++
	}
	return nodes, nil
}

// nodesOf turns free words inside a group into MATCHES leaves of the full-text field.
func nodesOf(items []item) []any {
	nodes := make([]any, 0, len(items))
	for _, it := range items {
		if it.node != nil {
			nodes = append(nodes, it.node)
			continue
		}
		if negated, found := strings.CutPrefix(it.piece, "-"); found {
			nodes = append(nodes, not(matches(negated)))
			continue
		}
		nodes = append(nodes, matches(it.piece))
	}
	return nodes
}

func matches(piece string) any {
	return map[string]any{"field": FieldText, "op": string(OpMatches), "value": piece}
}

func not(node any) any { return map[string]any{"op": "NOT", "nodes": []any{node}} }

// and reads operands until OR, a closing bracket or the end; an explicit AND is allowed and adds
// nothing. At the top, a top-level OR turns the operands read so far into its first branch.
func (p *queryParser) and(nesting int, top bool) ([]item, error) {
	var items []item
	for !p.done() && !p.atSymbol(")") {
		if p.atWord("OR") {
			if !top || len(items) == 0 {
				break
			}
			p.pos++
			rest, err := p.branches(nesting)
			if err != nil {
				return nil, err
			}
			nodes := append([]any{joinAnd(nodesOf(items))}, rest...)
			return []item{{node: map[string]any{"op": "OR", "nodes": nodes}}}, nil
		}
		if p.atWord("AND") {
			p.pos++
			continue
		}
		it, err := p.unary(nesting, top)
		if err != nil {
			return nil, err
		}
		if it.node != nil || it.piece != "" {
			items = append(items, it)
		}
	}
	if len(items) == 0 && !top {
		return nil, p.fail(p.peek().at, "query.line_malformed", nil)
	}
	return items, nil
}

func (p *queryParser) unary(nesting int, top bool) (item, error) {
	t := p.peek()
	if p.atWord("NOT") || (t.kind == tokenWord && t.text == "-" && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].joined && p.tokens[p.pos+1].text == "(") {
		p.pos++
		if nesting >= maxLineNesting {
			return item{}, p.fail(t.at, "query.filter_too_deep", map[string]string{"maximum": strconv.Itoa(MaxFilterDepth)})
		}
		operand, err := p.unary(nesting+1, false)
		if err != nil {
			return item{}, err
		}
		node := operand.node
		if node == nil {
			node = nodesOf([]item{operand})[0]
		}
		return item{node: not(node)}, nil
	}
	if p.atSymbol("(") {
		p.pos++
		if nesting >= maxLineNesting {
			return item{}, p.fail(t.at, "query.filter_too_deep", map[string]string{"maximum": strconv.Itoa(MaxFilterDepth)})
		}
		node, err := p.or(nesting + 1)
		if err != nil {
			return item{}, err
		}
		if !p.atSymbol(")") {
			return item{}, p.fail(p.peek().at, "query.bracket_unclosed", map[string]string{"suggestion": ")"})
		}
		p.pos++
		return item{node: node}, nil
	}
	if t.kind == tokenSymbol {
		if t.text == "," {
			p.pos++
			return item{}, nil
		}
		return item{}, p.fail(t.at, "query.line_malformed", nil)
	}
	if t.kind == tokenText {
		p.pos++
		return item{piece: phrase(t.text)}, nil
	}
	if marker := fieldMarker.FindStringSubmatch(t.text); marker != nil {
		return p.term(marker, top)
	}
	p.pos++
	if t.text == "-" && !p.done() && p.peek().joined && p.peek().kind == tokenText {
		next := p.peek()
		p.pos++
		return item{piece: "-" + phrase(next.text)}, nil
	}
	return item{piece: t.text}, nil
}

// phrase writes a quoted text as websearch reads a phrase; a quote inside has no spelling there.
func phrase(text string) string { return `"` + strings.ReplaceAll(text, `"`, "") + `"` }

// raw is one value as typed, with where it starts; group is set for `members("…")`, whose
// argument text is.
type raw struct {
	text   string
	quoted bool
	at     int
	group  bool
}

// membersFunction is the one function value: the people of a group, `members("Support")`.
const membersFunction = "members"

// atJoinedSymbol reports whether the next token is the symbol, written against the word before.
func (p *queryParser) atJoinedSymbol(symbol string) bool {
	return p.atSymbol(symbol) && p.tokens[p.pos].joined
}

// groupArgument reads `("name")` or `(name)` after `members`.
func (p *queryParser) groupArgument(at int) (raw, error) {
	p.pos++
	if p.done() || p.peek().kind == tokenSymbol {
		return raw{}, p.fail(at, "query.group_required", nil)
	}
	arg := p.tokens[p.pos]
	p.pos++
	if !p.atSymbol(")") {
		return raw{}, p.fail(at, "query.group_required", nil)
	}
	p.pos++
	return raw{text: arg.text, at: at, group: true}, nil
}

// term reads `[-]key: value[, value…]`.
func (p *queryParser) term(marker []string, top bool) (item, error) {
	t := p.tokens[p.pos]
	p.pos++
	negated, key := marker[1] == "-", marker[2]
	keyAt := t.at + len(marker[1])
	var values []raw
	all := false
	rest := t.text[len(marker[0]):]
	expect := true
	if rest != "" {
		values = append(values, raw{text: rest, at: t.at + len(marker[0])})
		expect = false
		if rest == membersFunction && p.atJoinedSymbol("(") {
			group, err := p.groupArgument(t.at + len(marker[0]))
			if err != nil {
				return item{}, err
			}
			values[0] = group
		}
	}
	for {
		if expect {
			v := p.peek()
			if p.done() || v.kind == tokenSymbol || (v.kind == tokenWord && (v.text == "OR" || v.text == "AND" || v.text == "NOT" || fieldMarker.MatchString(v.text))) {
				return item{}, p.fail(v.at, "query.value_required", map[string]string{"field": key})
			}
			p.pos++
			if v.kind == tokenWord && slices.Contains([]string{"<", "<=", ">", ">=", "="}, v.text) && !p.done() && p.peek().kind == tokenWord {
				next := p.tokens[p.pos]
				p.pos++
				values = append(values, raw{text: v.text + next.text, at: v.at})
			} else if v.kind == tokenWord && v.text == membersFunction && p.atJoinedSymbol("(") {
				group, err := p.groupArgument(v.at)
				if err != nil {
					return item{}, err
				}
				values = append(values, group)
			} else {
				values = append(values, raw{text: v.text, quoted: v.kind == tokenText, at: v.at})
			}
		}
		switch {
		case p.atSymbol(","):
			p.pos++
			expect = true
			continue
		case p.atWord("+") && len(values) > 0:
			p.pos++
			all, expect = true, true
			continue
		}
		break
	}
	if all && len(values) < 2 {
		return item{}, p.fail(t.at, "query.line_malformed", nil)
	}
	return p.leaf(key, keyAt, negated, values, all, top)
}

func (p *queryParser) leaf(key string, keyAt int, negated bool, values []raw, all, top bool) (item, error) {
	switch key {
	case "sort":
		if !top || negated {
			return item{}, p.fail(keyAt, "query.sort_in_group", nil)
		}
		for _, v := range values {
			name, dir := v.text, ""
			if trimmed, desc := strings.CutPrefix(name, "-"); desc {
				name, dir = trimmed, "DESC"
			}
			fieldName, ok := resolveKey(name)
			if _, ofAKind := kindField(fieldName); !ok || ofAKind {
				return item{}, p.fail(v.at, "query.field_unknown", suggestion(name, sortKeys()))
			}
			term := map[string]any{"field": fieldName}
			if dir != "" {
				term["dir"] = dir
			}
			p.q.Sort = append(p.q.Sort, term)
		}
		return item{}, nil
	case "is":
		return p.isTerm(keyAt, negated, values, top)
	case "type":
		return p.typeTerm(keyAt, negated, values, top)
	}
	name, ok := resolveKey(key)
	if !ok {
		return item{}, p.fail(keyAt, "query.field_unknown", suggestion(key, queryKeys()))
	}
	target, err := queryField(name, p.path)
	if err != nil {
		return item{}, p.fail(keyAt, "query.field_unknown", suggestion(key, queryKeys()))
	}
	node, err := p.compare(target, key, negated, values, all)
	if err != nil {
		return item{}, err
	}
	return item{node: node}, nil
}

func (p *queryParser) isTerm(keyAt int, negated bool, values []raw, top bool) (item, error) {
	var nodes []any
	for _, v := range values {
		word := strings.ToLower(v.text)
		switch word {
		case "archived", "trashed":
			if !top || negated || len(values) > 1 {
				return item{}, p.fail(v.at, "query.flag_in_group", map[string]string{"value": word})
			}
			if word == "archived" {
				p.q.IncludeArchived = true
			} else {
				p.q.IncludeTrashed = true
			}
			return item{}, nil
		}
		state, known := isWords[word]
		if !known {
			return item{}, p.fail(v.at, "query.value_unknown", suggestionFor("is", v.text, []string{"open", "done", "blocked", "blocking", "archived", "trashed"}))
		}
		op := "EQ"
		if negated {
			op = "NEQ"
		}
		nodes = append(nodes, map[string]any{"field": state.field, "op": op, "value": state.value})
	}
	if len(nodes) == 1 {
		return item{node: nodes[0]}, nil
	}
	if negated {
		return item{node: map[string]any{"op": "AND", "nodes": nodes}}, nil
	}
	return item{node: map[string]any{"op": "OR", "nodes": nodes}}, nil
}

func (p *queryParser) typeTerm(keyAt int, negated bool, values []raw, top bool) (item, error) {
	var levels []any
	var kinds []string
	for _, v := range values {
		word := strings.ToLower(strings.ReplaceAll(v.text, "_", ""))
		if !slices.Contains(ObjectKinds, word) {
			return item{}, p.fail(v.at, "query.value_unknown", suggestionFor("type", v.text, ObjectKinds))
		}
		if level, isLevel := entryLevels[word]; isLevel {
			levels = append(levels, level)
		}
		if !slices.Contains(kinds, word) {
			kinds = append(kinds, word)
		}
	}
	if len(levels) < len(kinds) {
		// Another kind than an entry level chooses what is read, which only the top can say.
		if !top || negated {
			return item{}, p.fail(keyAt, "query.kind_in_group", nil)
		}
		p.q.Kinds = append(p.q.Kinds, kinds...)
		return item{}, nil
	}
	op := map[[2]bool]string{{false, false}: "EQ", {false, true}: "IN", {true, false}: "NEQ", {true, true}: "NOT_IN"}[[2]bool{negated, len(levels) > 1}]
	node := map[string]any{"field": FieldType, "op": op}
	if len(levels) == 1 {
		node["value"] = levels[0]
	} else {
		node["value"] = levels
	}
	return item{node: node}, nil
}

// compare builds the leaf for one field: the comparison a value spells, or the field's default -
// a set contains, the full text matches, a title or a note contains, every other field equals.
func (p *queryParser) compare(target Field, key string, negated bool, values []raw, all bool) (any, error) {
	if len(values) == 1 && !values[0].quoted {
		v := values[0]
		if op, operand, found := comparator(v.text); found {
			bound := ""
			switch op {
			case "LT", "GTE":
				bound = "low"
			case "LTE", "GT":
				bound = "high"
			}
			node, err := p.single(target, op, raw{text: operand, at: v.at}, bound)
			return negate(node, err, negated)
		}
		if low, high, isRange := strings.Cut(v.text, ".."); isRange {
			node, err := p.rangeOf(target, low, high, v.at)
			return negate(node, err, negated)
		}
	}
	none := false
	var rest []raw
	for _, v := range values {
		if !v.quoted && v.text == "none" {
			none = true
			continue
		}
		rest = append(rest, v)
	}
	var nodes []any
	if len(rest) > 0 {
		node, err := p.defaultCompare(target, rest, all)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	if none {
		nodes = append(nodes, map[string]any{"field": target.Name, "op": string(OpIsNull)})
	}
	if len(nodes) > 1 {
		return negate(map[string]any{"op": "OR", "nodes": nodes}, nil, negated)
	}
	return negate(nodes[0], nil, negated)
}

// negate writes a leading `-`: into the operator where one says it (EQ, IN), around the node
// everywhere else.
func negate(node any, err error, negated bool) (any, error) {
	if err != nil || !negated {
		return node, err
	}
	leaf, isLeaf := node.(map[string]any)
	if !isLeaf {
		return not(node), nil
	}
	switch leaf["op"] {
	case "EQ":
		leaf["op"] = "NEQ"
		return leaf, nil
	case "IN":
		leaf["op"] = "NOT_IN"
		return leaf, nil
	}
	return not(node), nil
}

func defaultOp(target Field) string {
	switch {
	case target.Kind == KindIDSet:
		return string(OpContains)
	case target.Name == FieldText:
		return string(OpMatches)
	case target.Name == FieldTitle || target.Name == FieldNotes:
		return string(OpContains)
	}
	return string(OpEq)
}

func (p *queryParser) defaultCompare(target Field, values []raw, all bool) (any, error) {
	op := defaultOp(target)
	if len(values) == 1 {
		return p.single(target, op, values[0], "")
	}
	switch {
	case target.Kind == KindIDSet && all:
		op = string(OpContainsAll)
	case target.Kind == KindIDSet:
		op = string(OpContainsAny)
	case all || op != "EQ":
		return nil, p.fail(values[1].at, "query.values_not_allowed", map[string]string{"field": target.Name})
	default:
		op = string(OpIn)
	}
	list := make([]any, 0, len(values))
	for _, v := range values {
		value, err := p.value(target, v, "")
		if err != nil {
			return nil, err
		}
		list = append(list, value)
	}
	return map[string]any{"field": target.Name, "op": op, "value": list}, nil
}

// single builds one comparison; a bare time word or a day without a comparison is a span.
func (p *queryParser) single(target Field, op string, v raw, bound string) (any, error) {
	if target.Kind == KindTimestamp && !v.quoted && op == "EQ" && bound == "" {
		word := strings.ToLower(v.text)
		if span, isPeriod := periods[word]; isPeriod {
			return map[string]any{"field": target.Name, "op": "BETWEEN", "value": []any{span[0], span[1]}}, nil
		}
		if word == "overdue" {
			return map[string]any{"field": target.Name, "op": "LT", "value": "@today"}, nil
		}
		if m := relative.FindStringSubmatch(word); m != nil {
			op = "LTE"
			if m[1] == "-" {
				op = "GTE"
			}
			return map[string]any{"field": target.Name, "op": op, "value": offsetPlaceholder(m)}, nil
		}
		if dateOnly.MatchString(v.text) {
			return map[string]any{"field": target.Name, "op": "BETWEEN", "value": []any{v.text + "T00:00:00Z", v.text + "T23:59:59Z"}}, nil
		}
	}
	value, err := p.value(target, v, bound)
	if err != nil {
		return nil, err
	}
	return map[string]any{"field": target.Name, "op": op, "value": value}, nil
}

func (p *queryParser) rangeOf(target Field, low, high string, at int) (any, error) {
	switch {
	case low == "" && high == "":
		return nil, p.fail(at, "query.value_required", map[string]string{"field": target.Name})
	case low == "":
		return p.single(target, "LTE", raw{text: high, at: at}, "high")
	case high == "":
		return p.single(target, "GTE", raw{text: low, at: at}, "low")
	}
	from, err := p.value(target, raw{text: low, at: at}, "low")
	if err != nil {
		return nil, err
	}
	to, err := p.value(target, raw{text: high, at: at}, "high")
	if err != nil {
		return nil, err
	}
	return map[string]any{"field": target.Name, "op": "BETWEEN", "value": []any{from, to}}, nil
}

// comparator splits a leading sign off a value: `<today`, `>=3`, `=Exact`, `~part`, `begin*`.
func comparator(text string) (op, operand string, found bool) {
	for _, sign := range []struct{ prefix, op string }{{"<=", "LTE"}, {">=", "GTE"}, {"<", "LT"}, {">", "GT"}, {"=", "EQ"}, {"~", "CONTAINS"}} {
		if rest, ok := strings.CutPrefix(text, sign.prefix); ok && rest != "" {
			return sign.op, rest, true
		}
	}
	if rest, ok := strings.CutSuffix(text, "*"); ok && rest != "" {
		return "STARTS_WITH", rest, true
	}
	return "", "", false
}

func offsetPlaceholder(m []string) string {
	return "@today" + m[1] + "P" + m[2] + strings.ToUpper(m[3])
}

// value types one value for the tree as the document would hold it; ParseFilter checks it.
func (p *queryParser) value(target Field, v raw, bound string) (any, error) {
	if v.group {
		if target.Kind != KindID && target.Kind != KindIDSet {
			return nil, p.fail(v.at, "query.value_unknown", map[string]string{"field": target.Name})
		}
		return string(PlaceholderGroup) + ":" + v.text, nil
	}
	if v.quoted {
		return v.text, nil
	}
	word := v.text
	switch target.Kind {
	case KindID, KindIDSet:
		switch word {
		case "me":
			return "@me", nil
		case "my-groups":
			return string(PlaceholderMyGroups), nil
		case "here":
			return nil, p.fail(v.at, "query.value_needs_place", map[string]string{"field": target.Name})
		}
	case KindTimestamp:
		lower := strings.ToLower(word)
		if anchor, isAnchor := anchors[lower]; isAnchor {
			return anchor, nil
		}
		if m := relative.FindStringSubmatch(lower); m != nil {
			return offsetPlaceholder(m), nil
		}
		if dateOnly.MatchString(word) {
			if bound == "high" {
				return word + "T23:59:59Z", nil
			}
			return word + "T00:00:00Z", nil
		}
	case KindBool:
		switch strings.ToLower(word) {
		case "true", "yes":
			return true, nil
		case "false", "no":
			return false, nil
		}
		return nil, p.fail(v.at, "query.value_unknown", suggestionFor(target.Name, word, []string{"true", "false"}))
	case KindInt:
		if number, err := strconv.ParseInt(word, 10, 64); err == nil {
			return float64(number), nil
		}
	case KindEnum:
		for _, allowed := range target.Values {
			if strings.EqualFold(allowed, word) {
				return allowed, nil
			}
		}
		lower := make([]string, len(target.Values))
		for i, allowed := range target.Values {
			lower[i] = strings.ToLower(allowed)
		}
		return nil, p.fail(v.at, "query.value_unknown", suggestionFor(target.Name, word, lower))
	case KindCustom:
		return bareValue(word, KindCustom), nil
	}
	return word, nil
}

// resolveKey turns a key as typed into the tree's field name.
func resolveKey(key string) (string, bool) {
	if name, ok := readerKeys[key]; ok {
		return name, true
	}
	if custom, ok := strings.CutPrefix(key, CustomKeyPrefix); ok {
		name := CustomFieldPrefix + custom
		_, valid := CustomField(name)
		return name, valid
	}
	if _, ok := FieldByName(key); ok {
		return key, true
	}
	if _, ok := kindField(key); ok {
		return key, true
	}
	return "", false
}

func queryKeys() []string {
	keys := []string{"is", "type", "sort"}
	for key := range readerKeys {
		keys = append(keys, key)
	}
	for _, f := range kindCatalogue {
		keys = append(keys, f.Name)
	}
	slices.Sort(keys)
	return keys
}

func sortKeys() []string {
	var keys []string
	for key, name := range readerKeys {
		if f, ok := FieldByName(name); ok && f.Sortable {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func suggestion(typed string, candidates []string) map[string]string {
	params := map[string]string{"field": typed}
	if best := nearest(strings.ToLower(typed), candidates); best != "" {
		params["suggestion"] = best
	}
	return params
}

// suggestionFor names the field, not the value: a value is the person's content (rule 10), the
// suggestion is the vocabulary's.
func suggestionFor(fieldName, typed string, candidates []string) map[string]string {
	params := map[string]string{"field": fieldName}
	if best := nearest(strings.ToLower(typed), candidates); best != "" {
		params["suggestion"] = best
	}
	return params
}

// nearest is the candidate fewest edits away, at most two, or one the typed text begins.
func nearest(typed string, candidates []string) string {
	best, distance := "", 3
	for _, candidate := range candidates {
		if typed != "" && strings.HasPrefix(candidate, typed) {
			return candidate
		}
		if d := editDistance(typed, candidate); d < distance {
			best, distance = candidate, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	row := make([]int, len(rb)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		previous := row[0]
		row[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			current := min(row[j]+1, row[j-1]+1, previous+cost)
			previous, row[j] = row[j], current
		}
	}
	return row[len(rb)]
}

// FormatQuery writes a query in the `field: value` spelling ParseQuery reads back to the same
// query: the free words, the kinds, the filter, the flags, the sort. ok is false for a tree no
// such text spells (an operator a field's spelling has no form for, a malformed node).
func FormatQuery(q Query) (string, bool) {
	var parts []string
	if q.Words != "" {
		parts = append(parts, q.Words)
	}
	if len(q.Kinds) > 0 {
		parts = append(parts, "type: "+strings.Join(q.Kinds, ", "))
	}
	if q.Filter != nil {
		node, isObject := q.Filter.(map[string]any)
		if !isObject {
			return "", false
		}
		if node["op"] == "AND" {
			children, _ := node["nodes"].([]any)
			if len(children) < 2 {
				return "", false
			}
			for _, child := range children {
				text, ok := formatQueryNode(child, true, true)
				if !ok {
					return "", false
				}
				parts = append(parts, text)
			}
		} else {
			text, ok := formatQueryNode(node, true, false)
			if !ok {
				return "", false
			}
			parts = append(parts, text)
		}
	}
	if q.IncludeArchived {
		parts = append(parts, "is: archived")
	}
	if q.IncludeTrashed {
		parts = append(parts, "is: trashed")
	}
	if len(q.Sort) > 0 {
		var terms []string
		for _, rawTerm := range q.Sort {
			term, _ := rawTerm.(map[string]any)
			name, _ := term["field"].(string)
			if name == "" || term["nulls"] != nil {
				return "", false
			}
			key := keyOf(name)
			switch term["dir"] {
			case "DESC":
				key = "-" + key
			case nil:
			default:
				return "", false
			}
			terms = append(terms, key)
		}
		parts = append(parts, "sort: "+strings.Join(terms, ", "))
	}
	return strings.Join(parts, " "), true
}

// keyOf is the key a field is typed by: the reader's where there is one.
func keyOf(name string) string {
	if key, ok := fieldKeys[name]; ok {
		return key
	}
	if custom, ok := strings.CutPrefix(name, CustomFieldPrefix); ok {
		return CustomKeyPrefix + custom
	}
	return name
}

// formatQueryNode writes one node. top says it is an operand of the outermost AND, where a free
// word would be read as ranked words, so a MATCHES leaf is written as `text:` there; inAnd says
// a nested AND needs brackets to stay a node of its own.
func formatQueryNode(rawNode any, top, inAnd bool) (string, bool) {
	node, isObject := rawNode.(map[string]any)
	if !isObject {
		return "", false
	}
	op, _ := node["op"].(string)
	children, _ := node["nodes"].([]any)
	switch op {
	case "AND":
		if len(children) < 2 {
			return "", false
		}
		parts := make([]string, len(children))
		for i, child := range children {
			text, ok := formatQueryNode(child, false, true)
			if !ok {
				return "", false
			}
			parts[i] = text
		}
		text := strings.Join(parts, " ")
		if inAnd {
			text = "(" + text + ")"
		}
		return text, true
	case "OR":
		if text, ok := formatNoneList(node); ok {
			return text, true
		}
		if len(children) < 2 {
			return "", false
		}
		parts := make([]string, len(children))
		for i, child := range children {
			text, ok := formatQueryNode(child, false, false)
			if !ok {
				return "", false
			}
			parts[i] = text
		}
		return "(" + strings.Join(parts, " OR ") + ")", true
	case "NOT":
		if len(children) != 1 {
			return "", false
		}
		if text, ok := formatNegated(children[0], top); ok {
			return text, true
		}
		text, ok := formatQueryNode(children[0], false, true)
		if !ok {
			return "", false
		}
		return "NOT " + text, true
	}
	return formatLeaf(node, top, false)
}

// formatNoneList writes OR(EQ|IN f, IS_NULL f) as `f: a, none`.
func formatNoneList(node map[string]any) (string, bool) {
	children, _ := node["nodes"].([]any)
	if len(children) != 2 {
		return "", false
	}
	first, _ := children[0].(map[string]any)
	second, _ := children[1].(map[string]any)
	if first == nil || second == nil || second["op"] != "IS_NULL" || first["field"] != second["field"] || (first["op"] != "EQ" && first["op"] != "IN") {
		return "", false
	}
	name, _ := first["field"].(string)
	if name == FieldType || name == FieldIsCompleted || name == FieldBlocked || name == FieldBlocking {
		return "", false
	}
	text, ok := formatLeaf(first, false, false)
	if !ok {
		return "", false
	}
	return text + ", none", true
}

// formatNegated writes NOT over a leaf whose negation `-key:` spells: a set, a contains, a match,
// an empty check, a comparison - every leaf whose `-` the parser wraps in NOT.
func formatNegated(rawNode any, top bool) (string, bool) {
	node, _ := rawNode.(map[string]any)
	if node == nil {
		return "", false
	}
	op, _ := node["op"].(string)
	if op == "OR" {
		if text, ok := formatNoneList(node); ok {
			return "-" + text, true
		}
		return "", false
	}
	if op == "AND" || op == "NOT" || op == "EQ" || op == "IN" {
		return "", false
	}
	name, _ := node["field"].(string)
	if name == FieldText && op == "MATCHES" && !top {
		if word, plain := node["value"].(string); plain && freeWord(word) {
			return "-" + word, true
		}
	}
	text, ok := formatLeaf(node, top, true)
	if !ok {
		return "", false
	}
	return "-" + text, true
}

// freeWord reports whether a MATCHES value reads back from a bare word or phrase unchanged.
func freeWord(word string) bool {
	if strings.HasPrefix(word, `"`) && strings.HasSuffix(word, `"`) && len(word) > 1 {
		return !strings.Contains(word[1:len(word)-1], `"`)
	}
	return plainWord(word) && !strings.HasPrefix(word, "-") && !fieldMarker.MatchString(word) && word != "OR" && word != "AND" && word != "NOT"
}

func formatLeaf(node map[string]any, top, negated bool) (string, bool) {
	op, _ := node["op"].(string)
	name, _ := node["field"].(string)
	value := node["value"]
	target, err := queryField(name, "")
	if err != nil {
		return "", false
	}
	switch name {
	case FieldIsCompleted, FieldBlocked, FieldBlocking:
		flag, isBool := value.(bool)
		if isBool && !negated && (op == "EQ" || op == "NEQ") {
			for word, state := range isWords {
				if state.field == name && state.value == flag {
					if op == "NEQ" {
						return "-is: " + word, true
					}
					return "is: " + word, true
				}
			}
		}
	case FieldType:
		var levels []string
		values, isList := value.([]any)
		if !isList {
			values = []any{value}
		}
		for _, v := range values {
			text, _ := v.(string)
			level := ""
			for word, l := range entryLevels {
				if l == text {
					level = word
				}
			}
			if level == "" {
				return "", false
			}
			levels = append(levels, level)
		}
		if negated {
			return "", false
		}
		switch {
		case op == "EQ" && !isList, op == "IN" && isList:
			return "type: " + strings.Join(levels, ", "), true
		case op == "NEQ" && !isList, op == "NOT_IN" && isList:
			return "-type: " + strings.Join(levels, ", "), true
		}
		return "", false
	}
	key := keyOf(name)
	if name == FieldText && op == "MATCHES" && !top && !negated {
		if word, plain := value.(string); plain && freeWord(word) {
			return word, true
		}
	}
	prefix := key + ": "
	defaultOperator := defaultOp(target)
	switch op {
	case "IS_NULL":
		return prefix + "none", true
	case "BETWEEN":
		values, _ := value.([]any)
		if len(values) != 2 {
			return "", false
		}
		if target.Kind == KindTimestamp {
			for word, span := range periods {
				if values[0] == span[0] && values[1] == span[1] {
					return prefix + word, true
				}
			}
			lowText, _ := values[0].(string)
			highText, _ := values[1].(string)
			if day, ok := strings.CutSuffix(lowText, "T00:00:00Z"); ok && dateOnly.MatchString(day) && highText == day+"T23:59:59Z" {
				return prefix + day, true
			}
		}
		low, okLow := formatQueryValue(values[0], target, "low", true)
		high, okHigh := formatQueryValue(values[1], target, "high", true)
		if !okLow || !okHigh {
			return "", false
		}
		return prefix + low + ".." + high, true
	case "LT", "LTE", "GT", "GTE":
		bound := "low"
		if op == "LTE" || op == "GT" {
			bound = "high"
		}
		if target.Kind == KindTimestamp && !negated {
			if text, isText := value.(string); isText {
				if m := todayOffset.FindStringSubmatch(text); m != nil && ((m[1] == "-" && op == "GTE") || (m[1] == "+" && op == "LTE")) {
					return prefix + m[1] + m[2] + strings.ToLower(m[3]), true
				}
			}
		}
		text, ok := formatQueryValue(value, target, bound, true)
		if !ok {
			return "", false
		}
		sign := map[string]string{"LT": "<", "LTE": "<=", "GT": ">", "GTE": ">="}[op]
		return prefix + sign + text, true
	case "IN", "NOT_IN", "CONTAINS_ANY", "CONTAINS_ALL":
		values, isList := value.([]any)
		if !isList || len(values) < 2 {
			return "", false
		}
		if (op == "IN" || op == "NOT_IN") && defaultOperator != "EQ" {
			return "", false
		}
		if (op == "CONTAINS_ANY" || op == "CONTAINS_ALL") && target.Kind != KindIDSet {
			return "", false
		}
		parts := make([]string, len(values))
		for i, v := range values {
			text, ok := formatQueryValue(v, target, "", false)
			if !ok || text == "none" {
				return "", false
			}
			parts[i] = text
		}
		separator := ", "
		if op == "CONTAINS_ALL" {
			separator = " + "
		}
		if op == "NOT_IN" {
			if negated {
				return "", false
			}
			prefix = "-" + prefix
		}
		return prefix + strings.Join(parts, separator), true
	case "EQ", "NEQ", "CONTAINS", "STARTS_WITH", "MATCHES":
		if op == "NEQ" {
			if negated {
				return "", false
			}
			prefix = "-" + prefix
		}
		text, ok := formatQueryValue(value, target, "", false)
		if !ok {
			return "", false
		}
		if target.Kind == KindTimestamp && (op == "EQ" || op == "NEQ") {
			return "", false
		}
		effective := op
		if op == "NEQ" {
			effective = "EQ"
		}
		if effective == defaultOperator {
			if text == "none" || (target.Kind == KindTimestamp) {
				return "", false
			}
			return prefix + text, true
		}
		switch effective {
		case "EQ":
			return prefix + "=" + text, true
		case "CONTAINS":
			return prefix + "~" + text, true
		case "STARTS_WITH":
			if strings.HasPrefix(text, `"`) {
				return "", false
			}
			return prefix + text + "*", true
		}
	}
	return "", false
}

// formatQueryValue writes a value so the parser reads it back for the field: a plain word bare,
// anything that could be read as something else quoted.
func formatQueryValue(rawValue any, target Field, bound string, inRange bool) (string, bool) {
	switch value := rawValue.(type) {
	case bool:
		return strconv.FormatBool(value), true
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), true
	case []any:
		return "", false
	case string:
		if strings.HasPrefix(value, "@") && (target.Kind == KindTimestamp || target.Kind == KindID || target.Kind == KindIDSet) {
			return formatPlaceholder(value), true
		}
		if target.Kind == KindTimestamp {
			if day, ok := strings.CutSuffix(value, "T00:00:00Z"); ok && dateOnly.MatchString(day) && bound != "high" {
				return day, true
			}
			if day, ok := strings.CutSuffix(value, "T23:59:59Z"); ok && dateOnly.MatchString(day) && bound == "high" {
				return day, true
			}
			if _, err := time.Parse(time.RFC3339, value); err == nil && plainWord(value) {
				return value, true
			}
			return "", false
		}
		if target.Kind == KindEnum {
			if target.Name == FieldSLAStanding || target.Name == FieldAssetStage || target.Name == FieldGoalState {
				return strings.ToLower(value), true
			}
			return value, true
		}
		if target.Kind == KindBool {
			return "", false
		}
		if bareSafe(value, target) {
			return value, true
		}
		if inRange {
			return "", false
		}
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`, true
	}
	return "", false
}

// bareSafe reports whether a text value reads back as itself without quotes.
func bareSafe(value string, target Field) bool {
	if !plainWord(value) || strings.Contains(value, "..") || strings.HasSuffix(value, "*") ||
		strings.ContainsAny(value[:1], "<>=~-+@") || fieldMarker.MatchString(value) {
		return false
	}
	switch value {
	case "none", "me", "my-groups", "here", "OR", "AND", "NOT", "+":
		return false
	}
	if target.Kind == KindCustom {
		if _, isNumber := bareValue(value, KindCustom).(string); !isNumber {
			return false
		}
	}
	if target.Kind == KindInt {
		return false
	}
	return true
}

func plainWord(word string) bool {
	return word != "" && !strings.ContainsFunc(word, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`(),"\`, r)
	})
}

// formatPlaceholder writes a computed value as its time word, or as itself where it has none.
func formatPlaceholder(value string) string {
	if value == "@me" {
		return "me"
	}
	if value == string(PlaceholderMyGroups) {
		return "my-groups"
	}
	if group, isGroup := strings.CutPrefix(value, string(PlaceholderGroup)+":"); isGroup {
		return membersFunction + `("` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(group) + `")`
	}
	for word, anchor := range anchors {
		if anchor == value {
			return word
		}
	}
	if m := todayOffset.FindStringSubmatch(value); m != nil {
		return m[1] + m[2] + strings.ToLower(m[3])
	}
	return value
}
