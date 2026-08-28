// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package expression_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/infrastructure/expression"
)

// values is an activation over a fixed map, and it counts what was asked for - which is how the
// laziness the port promises is proved rather than asserted.
type values struct {
	rows  map[string]any
	asked map[string]int
	err   error
}

func newValues(rows map[string]any) *values {
	return &values{rows: rows, asked: map[string]int{}}
}

func (v *values) Resolve(_ context.Context, name string) (any, bool, error) {
	v.asked[name]++
	if v.err != nil {
		return nil, false, v.err
	}
	value, found := v.rows[name]
	return value, found, nil
}

func ruleEnvironment() port.Environment {
	return port.Environment{Variables: []port.Variable{
		{Name: "event", Kind: port.KindMap},
		{Name: "item", Kind: port.KindMap},
		{Name: "now", Kind: port.KindTimestamp},
	}}
}

func compile(t *testing.T, text string) port.Program {
	t.Helper()

	program, err := expression.New().Compile(text, ruleEnvironment())
	if err != nil {
		t.Fatalf("compiling %q: %v", text, err)
	}
	return program
}

func TestAConditionAnswersTrueOrFalse(t *testing.T) {
	cases := []struct {
		expr string
		want bool
	}{
		{`item.type == 'TASK'`, true},
		{`item.type == 'ACTIVITY'`, false},
		{`item.labels.exists(l, l == 'approval')`, true},
		{`item.labels.exists(l, l == 'nope')`, false},
		{`item.type == 'TASK' && size(item.labels) == 2`, true},
		{`now.getHours() >= 8 && now.getHours() < 18`, true},
		{`event.type.startsWith('de.hubtask.work.item')`, true},
		{`has(item.missing) == false`, true},
	}
	rows := map[string]any{
		"item": map[string]any{
			"type":   "TASK",
			"labels": []any{"approval", "urgent"},
		},
		"event": map[string]any{"type": "de.hubtask.work.item.overdue.v1"},
		"now":   time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC),
	}

	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			out, err := compile(t, tc.expr).Evaluate(context.Background(), newValues(rows))
			if err != nil {
				t.Fatalf("evaluating: %v", err)
			}
			if out.Bool != tc.want {
				t.Errorf("answered %v, want %v", out.Bool, tc.want)
			}
		})
	}
}

// The check that makes automation.md §1.2's list a contract rather than a suggestion.
func TestAnExpressionNamingSomethingUndeclaredIsRefusedAtCompileTime(t *testing.T) {
	for _, text := range []string{
		`secret == 1`,
		`tenant.settings.anything`,
		`payload.body != ''`,
	} {
		t.Run(text, func(t *testing.T) {
			_, err := expression.New().Compile(text, ruleEnvironment())
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want ErrValidation", err)
			}
			if code := detailOf(t, err); code != port.CodeUnknownName {
				t.Errorf("code %q, want %q", code, port.CodeUnknownName)
			}
		})
	}
}

// A mistake is answered to its author, with a position, while they are still looking at it.
func TestASyntaxErrorCarriesItsPosition(t *testing.T) {
	_, err := expression.New().Compile("item.type == ", ruleEnvironment())
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want ErrValidation", err)
	}

	var coded *shared.Error
	if !errors.As(err, &coded) {
		t.Fatalf("the refusal carries no code: %v", err)
	}
	if coded.DetailCode != port.CodeSyntax {
		t.Errorf("code %q, want %q", coded.DetailCode, port.CodeSyntax)
	}
	if coded.Params["line"] == "" || coded.Params["column"] == "" {
		t.Errorf("the refusal carries no position: %v", coded.Params)
	}
}

// A condition that answered a number is a condition whose author believes it filters.
func TestAnExpressionThatIsNotABooleanIsRefused(t *testing.T) {
	program := compile(t, `size(item.labels)`)

	_, err := program.Evaluate(context.Background(),
		newValues(map[string]any{"item": map[string]any{"labels": []any{"a"}}}))
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want ErrValidation", err)
	}
	if code := detailOf(t, err); code != port.CodeNotBoolean {
		t.Errorf("code %q, want %q", code, port.CodeNotBoolean)
	}
}

// The first of the three limits: the length, checked before the parser rather than by it.
func TestAnExpressionPastTheLengthLimitIsRefusedUnparsed(t *testing.T) {
	long := "item.type == 'TASK'" + strings.Repeat(" || item.type == 'TASK'", 400)
	if len(long) <= port.MaxLength {
		t.Fatalf("the fixture is %d bytes, which is inside the limit", len(long))
	}

	_, err := expression.New().Compile(long, ruleEnvironment())
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want ErrValidation", err)
	}
	if code := detailOf(t, err); code != port.CodeTooLong {
		t.Errorf("code %q, want %q", code, port.CodeTooLong)
	}
}

// The second: an expression whose worst case is expensive is refused rather than allowed to spend
// the timeout. A concatenation inside a comprehension over a large list is the cheapest way to
// write an expensive-but-finite program.
func TestAnExpensiveExpressionHitsALimitRatherThanACPU(t *testing.T) {
	// Nested comprehensions: finite, terminating, and quadratic in a list the activation supplies.
	text := `item.labels.all(a, item.labels.all(b, item.labels.all(c, a + b + c != '')))`

	big := make([]any, 0, 400)
	for i := range 400 {
		big = append(big, strings.Repeat("x", 32)+itoa(i))
	}
	rows := map[string]any{"item": map[string]any{"labels": big}}

	program, err := expression.New().Compile(text, ruleEnvironment())
	if err != nil {
		// Refused statically is the better of the two outcomes and satisfies the criterion: the
		// expression never ran.
		if code := detailOf(t, err); code != port.CodeSyntax && code != port.CodeTooExpensive {
			t.Errorf("code %q, want a cost or syntax refusal", code)
		}
		return
	}

	started := time.Now()
	if _, err := program.Evaluate(context.Background(), newValues(rows)); err == nil {
		t.Fatal("a quadratic expression over four hundred elements was evaluated to completion")
	} else if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("error %v, want ErrUnavailable", err)
	}

	// Whichever limit caught it, it caught it quickly. A generous bound: what is being proved is
	// that something stopped it, not how fast.
	if spent := time.Since(started); spent > 5*time.Second {
		t.Errorf("the expression ran for %v before a limit stopped it", spent)
	}
}

// The third: the timeout, proved with a context that is already past its deadline so the assertion
// does not race a machine's speed.
func TestAnEvaluationStopsAtTheDeadline(t *testing.T) {
	program := compile(t, `item.labels.all(a, item.labels.all(b, a + b != ''))`)

	big := make([]any, 0, 300)
	for i := range 300 {
		big = append(big, itoa(i))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	_, err := program.Evaluate(ctx, newValues(map[string]any{"item": map[string]any{"labels": big}}))
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("error %v, want ErrUnavailable", err)
	}
	if code := detailOf(t, err); code != port.CodeTimedOut && code != port.CodeTooExpensive {
		t.Errorf("code %q, want a timeout or cost refusal", code)
	}
}

// A regex is the classic way to write a bomb, and CEL's `matches` is the only door to one.
func TestARegexBombDoesNotRunAway(t *testing.T) {
	program := compile(t, `item.title.matches('^(a+)+$')`)

	rows := map[string]any{
		"item": map[string]any{"title": strings.Repeat("a", 40) + "!"},
	}

	started := time.Now()
	// Go's RE2 has no backtracking, so this is linear rather than exponential - which is the point
	// worth recording: the bomb is defused by the engine's regex implementation rather than by a
	// limit, and the limits are there for the cases RE2 does not cover.
	if _, err := program.Evaluate(context.Background(), newValues(rows)); err != nil {
		t.Fatalf("evaluating: %v", err)
	}
	if spent := time.Since(started); spent > time.Second {
		t.Errorf("a catastrophic-looking regex took %v", spent)
	}
}

// The promise that makes the port worth its shape: an expression naming only `event` costs no
// reads, and one naming `item` three times reads it once.
func TestTheActivationIsResolvedLazilyAndOnce(t *testing.T) {
	rows := map[string]any{
		"event": map[string]any{"type": "de.hubtask.work.item.created.v1"},
		"item":  map[string]any{"type": "TASK"},
	}

	t.Run("an untouched name is never asked for", func(t *testing.T) {
		activation := newValues(rows)
		if _, err := compile(t, `event.type != ''`).
			Evaluate(context.Background(), activation); err != nil {
			t.Fatalf("evaluating: %v", err)
		}
		if activation.asked["item"] != 0 {
			t.Errorf("item was resolved %d times for an expression that does not name it",
				activation.asked["item"])
		}
	})

	t.Run("a repeated name is asked for once", func(t *testing.T) {
		activation := newValues(rows)
		if _, err := compile(t, `item.type == 'TASK' && item.type != '' && size(item.type) > 0`).
			Evaluate(context.Background(), activation); err != nil {
			t.Fatalf("evaluating: %v", err)
		}
		if activation.asked["item"] != 1 {
			t.Errorf("item was resolved %d times, want once", activation.asked["item"])
		}
	})
}

// A name the environment declared and the activation cannot produce is a failure, not a false: a
// condition that quietly read unavailable as false would match the opposite of what it says.
func TestAValueTheActivationCannotProduceFailsTheEvaluation(t *testing.T) {
	activation := newValues(nil)
	activation.err = shared.ErrUnavailable.WithDetail("postgres.query_failed")

	_, err := compile(t, `item.type == 'TASK'`).Evaluate(context.Background(), activation)
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("error %v, want the activation's own failure", err)
	}
}

// An absent value is not the same as an unreadable one: `has()` is how an expression asks, and it
// answers false rather than failing.
func TestAnAbsentValueIsAskable(t *testing.T) {
	out, err := compile(t, `has(item.cover) == false`).
		Evaluate(context.Background(), newValues(map[string]any{"item": map[string]any{}}))
	if err != nil {
		t.Fatalf("evaluating: %v", err)
	}
	if !out.Bool {
		t.Error("an absent field did not answer absent")
	}
}

// Templating is the same environment applied to strings (automation.md §1.3), and it goes through
// the same compiler and the same limits.
func TestATemplateRendersText(t *testing.T) {
	out, err := compile(t, `'Reminder: ' + item.title`).
		Evaluate(context.Background(),
			newValues(map[string]any{"item": map[string]any{"title": "Pay the invoice"}}))
	if err != nil {
		t.Fatalf("evaluating: %v", err)
	}
	if out.Text != "Reminder: Pay the invoice" {
		t.Errorf("rendered %q", out.Text)
	}
}

// The library functions automation.md §1.2 names: dates, sets and strings. The library's own rather
// than functions written here - a custom function is one whose termination nobody has proved.
func TestTheDocumentedLibraryFunctionsAreAvailable(t *testing.T) {
	rows := map[string]any{
		"item": map[string]any{
			"title":  "  Pay the invoice  ",
			"labels": []any{"a", "b"},
			"due":    time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
		},
		"now": time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC),
	}
	for _, text := range []string{
		// Dates.
		`now < timestamp('2027-01-01T00:00:00Z')`,
		`now + duration('24h') > now`,
		`now.getDayOfWeek() >= 0`,
		// Sets.
		`item.labels.exists(l, l == 'a')`,
		`item.labels.all(l, l != '')`,
		`size(item.labels) == 2`,
		// Strings.
		`item.title.contains('invoice')`,
		`item.title.startsWith('  Pay')`,
		`item.title.matches('invoice')`,
	} {
		t.Run(text, func(t *testing.T) {
			out, err := compile(t, text).Evaluate(context.Background(), newValues(rows))
			if err != nil {
				t.Fatalf("evaluating: %v", err)
			}
			if !out.Bool {
				t.Errorf("answered false")
			}
		})
	}
}

func detailOf(t *testing.T, err error) string {
	t.Helper()

	var coded *shared.Error
	if !errors.As(err, &coded) {
		t.Fatalf("the error carries no code: %v", err)
	}
	return coded.DetailCode
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
