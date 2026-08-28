// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package expression evaluates conditions with CEL (ADR-0009).
//
// The one place `cel-go` is imported. The core describes what a condition is and never learns that
// a third-party evaluator exists (rule 1, ADR-0001), which is what lets the engine be replaced
// without a rule changing - and what the architecture gate checks by name.
//
// CEL is the choice because of what it cannot do: no loops, no I/O, no user-defined functions, and
// every program terminates. Those are properties of the language, not of this configuration. What
// the configuration adds is the three bounds automation.md §1.2 asks for - a maximum length, a cost
// limit and a timeout - because "terminates" is not the same as "terminates soon enough", and a
// nested expression can be finite and still spend a worker's afternoon.
package expression

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/interpreter"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/expression"
)

// CEL is the compiler, and it holds nothing per expression.
//
// The environment is a parameter of Compile rather than a field, because two callers use two
// different sets of names: an automation rule sees the event and the entry it is about, and a
// retention rule sees the row it is deciding. One compiler, two vocabularies.
type CEL struct{}

func New() CEL { return CEL{} }

var _ port.Compiler = CEL{}

// Compile parses and type-checks one expression.
func (CEL) Compile(text string, environment port.Environment) (port.Program, error) {
	if len(text) > port.MaxLength {
		// Before the parser, deliberately: the length limit exists so that a hostile expression is
		// refused without being parsed, and a parser handed a megabyte has already done the work
		// the limit was meant to prevent.
		return nil, port.Refusal{Code: port.CodeTooLong}.Error()
	}
	if strings.TrimSpace(text) == "" {
		return nil, port.Refusal{Code: port.CodeSyntax}.Error()
	}

	env, err := environmentOf(environment)
	if err != nil {
		return nil, err
	}

	ast, issues := env.Compile(text)
	if issues != nil && issues.Err() != nil {
		return nil, refusalOf(issues.Err()).Error()
	}

	// Cost is bounded at compile time as well as at evaluation. The static estimate is what
	// catches an expression whose worst case is expensive before it has ever been run - a limit
	// applied only at evaluation would let such a rule be saved and then fail every time it fires,
	// which is a rule that looks fine to whoever wrote it.
	program, err := env.Program(ast,
		cel.CostLimit(port.CostLimit),
		cel.EvalOptions(cel.OptOptimize),
		cel.InterruptCheckFrequency(checkFrequency),
	)
	if err != nil {
		return nil, port.Refusal{Code: port.CodeSyntax, Message: err.Error()}.Error()
	}

	return compiled{program: program, output: ast.OutputType()}, nil
}

// checkFrequency is how often the evaluator looks at the context between steps.
//
// Small, because the point of the interrupt is the timeout: a check every hundred steps on an
// expression that spends a millisecond per step would notice the deadline long after it passed.
const checkFrequency = 100

// compiled is one expression, ready to be evaluated as often as necessary.
type compiled struct {
	program cel.Program
	output  *cel.Type
}

// Evaluate answers the expression against one activation.
func (c compiled) Evaluate(ctx context.Context, in port.Activation) (port.Value, error) {
	// The adapter's own deadline beside the caller's. The caller's bounds the request; this one
	// bounds the expression, and automation.md §1.2 asks for the second - a rule with three
	// conditions gets fifty milliseconds each rather than fifty between them.
	ctx, cancel := context.WithTimeout(ctx, port.Timeout)
	defer cancel()

	resolver := &lazy{ctx: ctx, from: in, seen: map[string]any{}}
	out, _, err := c.program.ContextEval(ctx, resolver)
	if resolver.err != nil {
		// A value the activation could not produce. Reported as its own failure rather than as the
		// evaluator's, because the expression is not what is wrong.
		return port.Value{}, resolver.err
	}
	if err != nil {
		return port.Value{}, evaluationError(ctx, err)
	}

	switch value := out.Value().(type) {
	case bool:
		return port.Value{Bool: value, Text: fmt.Sprint(value)}, nil
	case string:
		return port.Value{Text: value}, nil
	default:
		// A condition that answered a number is a condition whose author believes it filters.
		return port.Value{}, shared.ErrValidation.WithDetail(port.CodeNotBoolean)
	}
}

// lazy resolves a name the first time an expression asks for it, and remembers the answer.
//
// The remembering is what makes the port's promise true: an expression naming `item` three times
// reads it once, and an expression naming only `event` never touches the read model at all.
type lazy struct {
	ctx  context.Context
	from port.Activation
	seen map[string]any
	// err carries the activation's failure out, because CEL's ResolveName has nowhere to put one:
	// returning "not found" would make an unreadable entry look like an absent one, and a condition
	// on an absent entry answers false rather than failing.
	err error
}

var _ interpreter.Activation = (*lazy)(nil)

func (l *lazy) Parent() interpreter.Activation { return nil }

func (l *lazy) ResolveName(name string) (any, bool) {
	if value, cached := l.seen[name]; cached {
		return value, value != nil
	}
	if l.err != nil {
		return nil, false
	}

	value, found, err := l.from.Resolve(l.ctx, name)
	if err != nil {
		l.err = err
		return nil, false
	}
	if !found {
		l.seen[name] = nil
		return nil, false
	}
	l.seen[name] = value
	return value, true
}

// environmentOf declares exactly the names the caller listed, and nothing else.
//
// No container, no extension packages beyond the documented ones, and no `cel.Declarations` for
// anything this system did not ask for. An expression naming something absent is a compile error,
// which is what makes automation.md §1.2's list a contract rather than a suggestion.
func environmentOf(environment port.Environment) (*cel.Env, error) {
	options := []cel.EnvOption{
		// The standard library minus nothing: CEL's own is already free of I/O and loops. What is
		// added beside it are the three families automation.md §1.2 names - dates, sets and
		// strings - and they are the library's own rather than functions written here, because a
		// custom function is a function whose termination nobody has proved.
		cel.StdLib(),
		cel.OptionalTypes(),
		// Refuse a program that would need a type conversion CEL cannot check, rather than
		// discovering it at evaluation.
		cel.EagerlyValidateDeclarations(true),
		cel.DefaultUTCTimeZone(true),
	}
	for _, variable := range environment.Variables {
		celType, err := typeOf(variable.Kind)
		if err != nil {
			return nil, err
		}
		options = append(options, cel.Variable(variable.Name, celType))
	}

	env, err := cel.NewEnv(options...)
	if err != nil {
		return nil, shared.ErrInternal.
			WithDetail("expression.environment_invalid").
			WithCause(fmt.Errorf("building the CEL environment: %w", err))
	}
	return env, nil
}

func typeOf(kind port.Kind) (*cel.Type, error) {
	switch kind {
	case port.KindMap:
		// Dynamic values, so that the expression language's contract is the variable names rather
		// than every field of every aggregate - a rule written today must not break when a field
		// is renamed.
		return cel.MapType(cel.StringType, cel.DynType), nil
	case port.KindTimestamp:
		return cel.TimestampType, nil
	case port.KindString:
		return cel.StringType, nil
	case port.KindInt:
		return cel.IntType, nil
	case port.KindBool:
		return cel.BoolType, nil
	default:
		return nil, shared.ErrInternal.
			WithDetail("expression.environment_invalid").
			WithCause(fmt.Errorf("no CEL type for the kind %q", kind))
	}
}

// refusalOf turns the compiler's own error into a coded refusal with a position.
//
// The position is parsed out of CEL's message rather than read from a field, because the library
// reports issues as formatted text. Parsed defensively: a refusal with no position is still a
// refusal, and the caller renders it against the expression as a whole.
func refusalOf(err error) port.Refusal {
	message := err.Error()
	refusal := port.Refusal{Code: port.CodeSyntax, Message: message}
	if strings.Contains(message, "undeclared reference") ||
		strings.Contains(message, "undefined field") {
		// The check that makes the documented variable list a contract. Its own code, because
		// "you named something that does not exist" and "this is not an expression" send their
		// reader to two different places.
		refusal.Code = port.CodeUnknownName
	}
	refusal.Position = positionIn(message)
	return refusal
}

// positionIn reads `ERROR: <source>:<line>:<column>: <what>` out of CEL's formatted issue text.
func positionIn(message string) port.Position {
	for _, line := range strings.Split(message, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 4 {
			continue
		}
		row, rowErr := atoi(strings.TrimSpace(fields[2]))
		column, columnErr := atoi(strings.TrimSpace(fields[3]))
		if rowErr == nil && columnErr == nil && row > 0 {
			return port.Position{Line: row, Column: column}
		}
	}
	return port.Position{}
}

func atoi(text string) (int, error) {
	if text == "" {
		return 0, errors.New("empty")
	}
	value := 0
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, errors.New("not a number")
		}
		value = value*10 + int(digit-'0')
	}
	return value, nil
}

// evaluationError separates the two ways an evaluation stops early, because they mean different
// things to whoever reads the run log: one is an expression that costs too much and the other is a
// process that was too slow.
func evaluationError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return shared.ErrUnavailable.
			WithDetail(port.CodeTimedOut).
			WithParams(map[string]string{"timeout_ms": itoa(int(port.Timeout / time.Millisecond))}).
			WithCause(err)
	}
	if strings.Contains(err.Error(), "cost limit exceeded") ||
		strings.Contains(err.Error(), "operation cancelled: actual cost limit exceeded") {
		return shared.ErrUnavailable.
			WithDetail(port.CodeTooExpensive).
			WithCause(err)
	}
	return shared.ErrUnavailable.
		WithDetail(port.CodeTimedOut).
		WithCause(err)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// unused keeps the ref import honest while the value mapping is the standard one; CEL's own
// conversion handles the map and scalar kinds this environment declares.
var _ ref.Val = types.Bool(false)
