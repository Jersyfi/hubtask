// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Operator is one comparison, or one way of joining comparisons. The list is the contract's
// (api-guidelines.md §3); which of them a given field permits is the field's own business.
type Operator string

const (
	OpAnd Operator = "AND"
	OpOr  Operator = "OR"
	OpNot Operator = "NOT"

	OpEq          Operator = "EQ"
	OpNeq         Operator = "NEQ"
	OpIn          Operator = "IN"
	OpNotIn       Operator = "NOT_IN"
	OpLt          Operator = "LT"
	OpLte         Operator = "LTE"
	OpGt          Operator = "GT"
	OpGte         Operator = "GTE"
	OpBetween     Operator = "BETWEEN"
	OpIsNull      Operator = "IS_NULL"
	OpContains    Operator = "CONTAINS"
	OpContainsAny Operator = "CONTAINS_ANY"
	OpContainsAll Operator = "CONTAINS_ALL"
	OpStartsWith  Operator = "STARTS_WITH"
	OpMatches     Operator = "MATCHES"
)

// operators is the set a name is resolved against - all of them, combinations included, because
// "AND is not an operator" and "AND is not a leaf operator" are different answers and a client
// deserves the second one.
var operators = map[Operator]bool{
	OpAnd: true, OpOr: true, OpNot: true,
	OpEq: true, OpNeq: true, OpIn: true, OpNotIn: true, OpLt: true, OpLte: true, OpGt: true,
	OpGte: true, OpBetween: true, OpIsNull: true, OpContains: true, OpContainsAny: true,
	OpContainsAll: true, OpStartsWith: true, OpMatches: true,
}

// Combines reports whether the operator joins nodes rather than comparing a field.
func (o Operator) Combines() bool { return o == OpAnd || o == OpOr || o == OpNot }

// The bounds a filter has to stay inside (api-guidelines.md §3, "protection against expensive
// queries").
//
// They are part of the grammar rather than a configuration value: a query that a client may send
// is a query every installation has to be able to plan, and an operator who could raise the limit
// would be an operator who could make one tenant's request cost another tenant's latency.
const (
	// MaxFilterDepth is how deeply nodes may nest, counting the outermost node as the first level.
	MaxFilterDepth = 5
	// MaxFilterNodes is the whole tree, leaves and combinations together.
	MaxFilterNodes = 50
	// MaxValues bounds one list operator. A hundred identifiers is a generous selection on a
	// board; ten thousand is a way of turning one request into a scan.
	MaxValues = 100
	// MaxValueLength bounds a text a query compares against. The title column stops at 500, so
	// nothing longer can match anything.
	MaxValueLength = 500
)

// Node is one node of a filter: a leaf comparing a field, or a combination joining nodes.
//
// Built by ParseFilter and by nothing else in practice - but the compiler that reads it still
// refuses what it does not recognise rather than trusting the type. Two layers of defence for the
// same rule is the whole arrangement of ADR-0026, and it costs one switch statement.
type Node struct {
	Op Operator
	// Field is the field a leaf compares. The zero value on a combination.
	Field Field
	// Values are what the leaf compares against: none for IS_NULL, one for most operators, two for
	// BETWEEN, and up to MaxValues for the list operators.
	Values []Value
	// Nodes are what a combination joins, and are empty on a leaf.
	Nodes []Node
}

// IsLeaf reports whether the node compares a field.
func (n Node) IsLeaf() bool { return !n.Op.Combines() }

// Value is one comparison value, already typed against the field it belongs to.
//
// A tagged struct rather than an `any`, because the adapter binds it as a query parameter and has
// to know which one: a value that arrived as a JSON string and is bound as text where the column
// is a uuid is an error the database reports at run time, on a path a fuzz test cannot reach if
// the type is decided by a type assertion.
type Value struct {
	Kind Kind
	Text string
	ID   shared.ID
	Int  int64
	Bool bool
	Time time.Time
	// Placeholder is the unresolved form of a value the server computes: `@me`, `@today+P3D`. It
	// is empty in a resolved value, and a value that still carries one has not been through
	// Resolve - which the compiler refuses, because `@me` is not a comparable value.
	Placeholder Placeholder
}

// IsPlaceholder reports whether the value still has to be resolved.
func (v Value) IsPlaceholder() bool { return v.Placeholder.Kind != "" }

// ParseFilter reads a filter document and refuses anything the grammar does not allow.
//
// The document is untyped on purpose: it arrives as a decoded body, as MCP tool arguments or as an
// automation action's parameters, and all three are maps and slices by the time they reach a use
// case. Nothing here decodes anything - the grammar is the domain's, and giving it a serialisation
// format would put one in the core (project-structure.md §3).
//
// A nil document is no filter, which is a legitimate query: "everything in this collection".
func ParseFilter(raw any, path string) (*Node, error) {
	if raw == nil {
		return nil, nil
	}
	var count int
	node, err := parseNode(raw, path, 1, &count)
	if err != nil {
		return nil, err
	}
	return &node, nil
}

func parseNode(raw any, path string, depth int, count *int) (Node, error) {
	if depth > MaxFilterDepth {
		return Node{}, fieldError(path, "query.filter_too_deep", map[string]string{
			"maximum": strconv.Itoa(MaxFilterDepth),
		})
	}
	if *count++; *count > MaxFilterNodes {
		return Node{}, fieldError(path, "query.filter_too_large", map[string]string{
			"maximum": strconv.Itoa(MaxFilterNodes),
		})
	}

	document, ok := raw.(map[string]any)
	if !ok {
		return Node{}, fieldError(path, "query.node_malformed", nil)
	}

	op, err := parseOperator(document["op"], path+"/op")
	if err != nil {
		return Node{}, err
	}
	if op.Combines() {
		return parseCombination(document, op, path, depth, count)
	}
	return parseLeaf(document, op, path)
}

func parseOperator(raw any, path string) (Operator, error) {
	name, ok := raw.(string)
	if !ok || strings.TrimSpace(name) == "" {
		return "", fieldError(path, "query.operator_required", nil)
	}
	op := Operator(strings.TrimSpace(name))
	if !operators[op] {
		return "", fieldError(path, "query.operator_unknown", map[string]string{"operator": string(op)})
	}
	return op, nil
}

// parseCombination reads AND, OR and NOT.
//
// A combination that also names a field is refused rather than read as one or the other. A client
// that sent `{"op": "AND", "field": "type", "nodes": [...]}` has two ideas about what it wants,
// and picking one of them for it is how a query comes to answer a question nobody asked.
func parseCombination(document map[string]any, op Operator, path string, depth int, count *int) (Node, error) {
	if _, named := document["field"]; named {
		return Node{}, fieldError(path, "query.node_ambiguous", nil)
	}
	if _, valued := document["value"]; valued {
		return Node{}, fieldError(path, "query.node_ambiguous", nil)
	}

	raw, present := document["nodes"]
	children, ok := raw.([]any)
	if !present || !ok || len(children) == 0 {
		return Node{}, fieldError(path+"/nodes", "query.nodes_required", nil)
	}
	if op == OpNot && len(children) != 1 {
		return Node{}, fieldError(path+"/nodes", "query.not_takes_one", nil)
	}

	node := Node{Op: op, Nodes: make([]Node, 0, len(children))}
	for index, child := range children {
		parsed, err := parseNode(child, path+"/nodes/"+strconv.Itoa(index), depth+1, count)
		if err != nil {
			return Node{}, err
		}
		node.Nodes = append(node.Nodes, parsed)
	}
	return node, nil
}

// parseLeaf reads a comparison: which field, which operator, and what to compare against.
func parseLeaf(document map[string]any, op Operator, path string) (Node, error) {
	if _, present := document["nodes"]; present {
		return Node{}, fieldError(path, "query.node_ambiguous", nil)
	}

	name, _ := document["field"].(string)
	target, err := field(name, path+"/field")
	if err != nil {
		return Node{}, err
	}
	if !target.Permits(op) {
		// Two different refusals reach this line, and they are one answer on purpose: an operator
		// that exists but not for this field, and an operator this installation serves for nothing
		// at all. Both are answered by the same list in `/meta/capabilities`, and a client that
		// reads it has what it needs either way.
		return Node{}, fieldError(path+"/op", "query.operator_unsupported", map[string]string{
			"field": target.Name, "operator": string(op),
		})
	}
	if op == OpIsNull && !target.Nullable {
		return Node{}, fieldError(path+"/op", "query.field_not_nullable", map[string]string{
			"field": target.Name,
		})
	}

	values, err := parseValues(document, target, op, path)
	if err != nil {
		return Node{}, err
	}
	return Node{Op: op, Field: target, Values: values}, nil
}

// parseValues reads what a leaf compares against, in the arity its operator demands.
func parseValues(document map[string]any, target Field, op Operator, path string) ([]Value, error) {
	raw, present := document["value"]
	valuePath := path + "/value"

	switch op {
	case OpIsNull:
		// IS_NULL takes no value. A client that sent one meant something else - `IS_NULL: false`
		// reads as "is not null", which is NOT wrapped around IS_NULL and not this.
		if present && raw != nil {
			return nil, fieldError(valuePath, "query.value_not_allowed", nil)
		}
		return nil, nil

	case OpIn, OpNotIn, OpContainsAny, OpContainsAll:
		return parseList(raw, target, valuePath, 1, MaxValues)

	case OpBetween:
		values, err := parseList(raw, target, valuePath, 2, 2)
		if err != nil {
			return nil, err
		}
		return values, nil

	default:
		if !present || raw == nil {
			return nil, fieldError(valuePath, "query.value_required", nil)
		}
		value, err := parseValue(raw, target, valuePath)
		if err != nil {
			return nil, err
		}
		return []Value{value}, nil
	}
}

// parseList reads an array of values and holds it to the arity the operator has.
//
// A single value where a list belongs is accepted for a list operator with no lower bound of two,
// for the reason Input.IDList accepts one: every channel's caller writes `"value": "x"` instead of
// `["x"]` at least once, and the two mean the same thing for IN.
func parseList(raw any, target Field, path string, minimum, maximum int) ([]Value, error) {
	elements, ok := raw.([]any)
	if !ok {
		if raw == nil || minimum > 1 {
			return nil, fieldError(path, "query.values_required", map[string]string{
				"minimum": strconv.Itoa(minimum),
			})
		}
		elements = []any{raw}
	}
	if len(elements) < minimum || len(elements) > maximum {
		return nil, fieldError(path, "query.values_arity_invalid", map[string]string{
			"minimum": strconv.Itoa(minimum), "maximum": strconv.Itoa(maximum),
		})
	}

	values := make([]Value, 0, len(elements))
	for index, element := range elements {
		value, err := parseValue(element, target, path+"/"+strconv.Itoa(index))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

// parseValue types one value against the field's kind.
//
// A placeholder is recognised for the kinds that can have one - an identifier, a set of
// identifiers, and a moment - and nowhere else: on a text field an `@` is the first character of
// somebody's title, and reading it as a placeholder would make a legitimate search unexpressible.
//
// A set takes one because `members CONTAINS @me` is the query api-guidelines.md §3 names in its own
// example: what the placeholder stands for is one identifier either way, and whether the field
// holds one of them or several is the operator's business rather than the value's.
func parseValue(raw any, target Field, path string) (Value, error) {
	if text, ok := raw.(string); ok && strings.HasPrefix(text, placeholderPrefix) {
		switch target.Kind {
		case KindID, KindIDSet, KindTimestamp:
			placeholder, err := parsePlaceholder(text, target.Kind, path)
			if err != nil {
				return Value{}, err
			}
			return Value{Kind: target.Kind, Placeholder: placeholder}, nil
		}
	}

	switch target.Kind {
	case KindID, KindIDSet:
		text, ok := raw.(string)
		if !ok {
			return Value{}, typeError(path, target)
		}
		id, err := shared.ParseID(strings.TrimSpace(text))
		if err != nil {
			return Value{}, fieldError(path, "shared.id_malformed", nil)
		}
		return Value{Kind: KindID, ID: id}, nil

	case KindBool:
		value, ok := raw.(bool)
		if !ok {
			return Value{}, typeError(path, target)
		}
		return Value{Kind: KindBool, Bool: value}, nil

	case KindInt:
		number, ok := integerOf(raw)
		if !ok {
			return Value{}, typeError(path, target)
		}
		return Value{Kind: KindInt, Int: number}, nil

	case KindTimestamp:
		text, ok := raw.(string)
		if !ok {
			return Value{}, typeError(path, target)
		}
		moment, err := time.Parse(time.RFC3339, strings.TrimSpace(text))
		if err != nil {
			return Value{}, fieldError(path, "query.timestamp_malformed", nil)
		}
		return Value{Kind: KindTimestamp, Time: moment.UTC()}, nil

	case KindEnum:
		text, ok := raw.(string)
		if !ok {
			return Value{}, typeError(path, target)
		}
		text = strings.TrimSpace(text)
		if !containsString(target.Values, text) {
			return Value{}, fieldError(path, "query.value_not_in_enum", map[string]string{
				"field": target.Name, "allowed": strings.Join(target.Values, ", "),
			})
		}
		return Value{Kind: KindEnum, Text: text}, nil

	case KindCustom:
		return parseCustomValue(raw, path)

	default: // KindString, KindText
		text, ok := raw.(string)
		if !ok {
			return Value{}, typeError(path, target)
		}
		// Trimmed but not otherwise touched, and length-bounded: a comparison value is content,
		// and the only thing worth deciding about it here is that it cannot be long enough to make
		// the comparison itself expensive.
		text = strings.TrimSpace(text)
		if text == "" {
			return Value{}, fieldError(path, "query.value_required", nil)
		}
		if len([]rune(text)) > MaxValueLength {
			return Value{}, fieldError(path, "query.value_too_long", map[string]string{
				"maximum": strconv.Itoa(MaxValueLength),
			})
		}
		return Value{Kind: target.Kind, Text: text}, nil
	}
}

// parseCustomValue reads a value for a custom field, whose type no column pins.
//
// The three JSON scalars a definition can produce, and the value's own type is what says which
// comparison is meant: a NUMBER field holds a JSON number, and a filter that had to spell it as a
// string would match nothing on every entry that holds one. A list is not among them - a
// MULTI_SELECT is asked with CONTAINS, one element at a time, which is the same shape the labels
// take.
func parseCustomValue(raw any, path string) (Value, error) {
	switch typed := raw.(type) {
	case bool:
		return Value{Kind: KindBool, Bool: typed}, nil

	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return Value{}, fieldError(path, "query.value_required", nil)
		}
		if len([]rune(text)) > MaxValueLength {
			return Value{}, fieldError(path, "query.value_too_long", map[string]string{
				"maximum": strconv.Itoa(MaxValueLength),
			})
		}
		return Value{Kind: KindString, Text: text}, nil

	default:
		number, ok := numberTextOf(raw)
		if !ok {
			return Value{}, fieldError(path, "query.value_type_invalid", nil)
		}
		// The canonical spelling rather than the float, because the comparison is against a JSON
		// scalar and the adapter casts it back to a number. A float carried through Go's default
		// formatting would spell 1e+06 for a million.
		return Value{Kind: KindNumber, Text: number}, nil
	}
}

// numberTextOf renders a JSON number in the spelling the database will parse. The shapes are the
// ones a decoded document produces across the three channels.
func numberTextOf(raw any) (string, bool) {
	switch number := raw.(type) {
	case float64:
		return strconv.FormatFloat(number, 'f', -1, 64), true
	case int:
		return strconv.Itoa(number), true
	case int64:
		return strconv.FormatInt(number, 10), true
	default:
		return "", false
	}
}

// typeError says what shape the field wanted, without echoing what arrived: the value is content
// (CLAUDE.md rule 10), and the field's kind is what a client needs in order to correct itself.
func typeError(path string, target Field) error {
	return fieldError(path, "query.value_type_invalid", map[string]string{
		"field": target.Name, "kind": string(target.Kind),
	})
}

// integerOf reads a whole number. JSON numbers arrive as float64, so a plain assertion would
// refuse every integer that came over the wire; a float with a fraction is refused, because
// `depth < 2.5` is a question about a column that holds whole numbers.
func integerOf(raw any) (int64, bool) {
	switch number := raw.(type) {
	case int:
		return int64(number), true
	case int64:
		return number, true
	case float64:
		if number != float64(int64(number)) {
			return 0, false
		}
		return int64(number), true
	}
	return 0, false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
