// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package expression is what a condition is, and nothing about how one is evaluated.
//
// A rule's condition and a retention rule's condition are the same language (ADR-0009,
// automation.md §1.2, data-retention.md §2), and this port is what makes that one sentence rather
// than two implementations that drift. The engine is CEL and lives in an adapter: the core states
// what it may ask and what it may be told, and never learns that a third-party evaluator exists
// (rule 1).
//
// The three limits automation.md §1.2 names - a maximum expression length, the evaluator's cost
// limit, and a timeout per evaluation - are the adapter's to enforce, because they are properties of
// the engine rather than of the language. What is here is the vocabulary for their refusals, so that
// a caller can tell "this expression is too expensive" from "this expression is wrong" without
// reading a sentence somebody wrote.
package expression

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Compiler turns the text of a condition into something that can be evaluated, or refuses it.
//
// Compiling is separate from evaluating because the two happen at different moments and for
// different people. A condition is compiled when somebody writes a rule - so that a mistake is
// answered to its author, with a position, while they are still looking at it - and evaluated
// later, thousands of times, by nobody. A design that only checked at evaluation would report a
// typo to a log at three in the morning.
type Compiler interface {
	// Compile parses and type-checks one expression against the environment it will be evaluated
	// in, and reports an error wrapping shared.ErrValidation when the text is not a valid
	// expression, names a variable the environment does not declare, is longer than the engine
	// accepts, or does not produce what the caller asked for.
	//
	// The last of those is why `want` is a parameter rather than a property of the result. A
	// condition that evaluates to a string and a template that evaluates to a boolean are both
	// programs somebody wrote by mistake, and both are silently wrong at run time - the first
	// filters nothing and the second renders "true". CEL knows the output type after the check, so
	// the mistake is answerable when it is written rather than discoverable when it runs.
	//
	// The refusal carries the position, because an editor points at a column and a person reads
	// one - see Position.
	Compile(text string, env Environment, want Result) (Program, error)
}

// Result is what an expression has to produce.
type Result string

const (
	// Boolean is a condition: it decides whether something happens.
	Boolean Result = "boolean"
	// Text is a template: it renders a value into a parameter (automation.md §1.3).
	Text Result = "text"
)

// Program is a compiled expression, ready to be evaluated as often as necessary.
//
// Reusable and safe for concurrent use: compiling is the expensive half, and an engine that
// recompiled per event would spend a rule's whole budget on parsing.
type Program interface {
	// Evaluate answers the expression against one set of values.
	//
	// The context bounds it. The adapter additionally applies its own timeout and cost limit, and
	// exceeding either is an error wrapping shared.ErrUnavailable with a `expression.` detail -
	// not a validation error, because the expression was valid when it was written and what failed
	// is this evaluation of it.
	Evaluate(ctx context.Context, in Activation) (Value, error)
}

// Environment is the set of names an expression may use, and what each one is.
//
// Declared rather than inferred, so that an expression naming something the engine will not provide
// fails when it is written rather than when it runs. automation.md §1.2 lists the names; a
// retention rule uses a smaller set of the same ones, which is what makes this a parameter rather
// than a constant.
type Environment struct {
	// Variables are the names and their kinds. A name absent from here is a compile error.
	Variables []Variable
}

// Kind is what an expression may assume about a variable.
//
// Coarse on purpose. CEL can carry a full type graph, and modelling this system's aggregates in it
// would make every field of every entry part of the expression language's contract - so a rule
// written today would break when a field is renamed. A map of dynamic values keeps the contract at
// the names in automation.md §1.2, and a condition reaching into one gets a runtime answer rather
// than a compile-time promise.
type Kind string

const (
	// KindMap is a document an expression reads fields out of: `item`, `event`, `payload`.
	KindMap Kind = "map"
	// KindTimestamp is an instant: `now`.
	KindTimestamp Kind = "timestamp"
	// KindString, KindInt and KindBool are the scalars.
	KindString Kind = "string"
	KindInt    Kind = "int"
	KindBool   Kind = "bool"
)

// Variable is one name an expression may use.
type Variable struct {
	Name string
	Kind Kind
	// Description is what a rule editor shows beside the name. Protocol documentation rather than
	// display text, like a use case's summary - it never reaches an end user's screen as prose the
	// backend wrote (ADR-0011).
	Description string
}

// Activation is the values an evaluation reads, resolved when the expression asks for them.
//
// A function per name rather than a map of values, because an expression that touches only `event`
// must cost no reads: a rule engine evaluates every enabled rule against every event, and building
// `collection`, `hub` and `parent` for each of them would turn one event into four queries per rule.
// The adapter calls the function at most once per evaluation and remembers the answer, so an
// expression naming `item` three times reads it once.
type Activation interface {
	// Resolve answers one name, or reports that it has no value here. A name the environment
	// declared and the activation cannot produce is an evaluation error rather than a silent null:
	// a condition that quietly read absent as false would match the opposite of what it says.
	Resolve(ctx context.Context, name string) (any, bool, error)
}

// Value is what an evaluation produced.
type Value struct {
	// Bool is the answer a condition gives. False when the expression produced something that is
	// not a boolean, which Program.Evaluate reports as an error rather than as false.
	Bool bool
	// Text is the answer a template gives.
	Text string
}

// Position is where in the text a refusal is about, as a compiler reports it.
//
// One-based, because that is how an editor counts and how every compiler anybody has used reports
// it. Zero means the refusal is about the expression as a whole - too long, or empty - rather than
// about a place in it.
type Position struct {
	Line   int
	Column int
}

// Refusal is a compile error with its position, so that a caller can turn it into a field error
// pointing at the right part of the right condition.
//
// A type rather than a coded error alone, because the position is structured and a caller has to
// put it somewhere: G-05's rule conditions render it into `/conditions/0/expr` with the line and
// column as parameters, and a retention rule renders it into `/condition`.
type Refusal struct {
	// Code is the message code, `expression.` something.
	Code string
	// Message is the engine's own words, for the log. Never answered to a caller: it is a
	// third-party library's English, and no display text comes out of the backend (ADR-0011).
	Message string
	Position
}

// Limits are what the adapter enforces, and they are here so that the numbers automation.md §1.2
// names have one home rather than being repeated in a configuration file and a comment.
//
// The values are the document's. An installation does not tune them: they are what makes "no I/O,
// no loops, terminating" true in practice rather than in principle, and an operator who raised them
// would be lifting a safety property rather than a performance one.
const (
	// MaxLength is the longest expression the compiler accepts, in bytes. Generous for a condition
	// somebody writes and far short of a payload somebody pastes.
	MaxLength = 4096

	// Timeout bounds one evaluation. Fifty milliseconds is automation.md §1.2's number, and it is
	// per expression rather than per rule: a rule with three conditions has three of them.
	Timeout = 50 * time.Millisecond

	// CostLimit is the evaluator's own budget, in CEL's cost units. It is the limit that catches
	// what a timeout catches too late - a deeply nested expression or a long concatenation is
	// refused before it has been allowed to spend the fifty milliseconds.
	CostLimit = 1_000_000
)

// The message codes this port's refusals carry. Here rather than in each adapter, because a caller
// switching on them is switching on the port's contract.
const (
	// CodeSyntax is an expression that is not one.
	CodeSyntax = "expression.syntax"
	// CodeUnknownName is an expression naming something the environment does not declare - which is
	// the check that makes the documented variable list a contract rather than a suggestion.
	CodeUnknownName = "expression.unknown_name"
	// CodeTooLong is an expression past MaxLength.
	CodeTooLong = "expression.too_long"
	// CodeNotBoolean is a condition that compiles and does not answer true or false. A condition
	// that returned a number would be a condition whose author believes it filters.
	CodeNotBoolean = "expression.not_boolean"
	// CodeNotString is a template that does not produce text.
	CodeNotString = "expression.not_string"
	// CodeTooExpensive is an evaluation that exceeded the cost limit.
	CodeTooExpensive = "expression.too_expensive"
	// CodeTimedOut is an evaluation that exceeded the timeout.
	CodeTimedOut = "expression.timed_out"
	// CodeUnavailable is a name the environment declared and the activation could not produce.
	CodeUnavailable = "expression.value_unavailable"
)

// Error builds a refusal as the port's callers see it: a validation error, coded, carrying the
// position as parameters so that a field error can name it without this package knowing what a
// field error is.
func (r Refusal) Error() error {
	params := map[string]string{}
	if r.Line > 0 {
		params["line"] = itoa(r.Line)
		params["column"] = itoa(r.Column)
	}
	return shared.ErrValidation.WithDetail(r.Code).WithParams(params)
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
