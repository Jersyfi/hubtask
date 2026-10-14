// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package expression_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/infrastructure/expression"
)

// SG-8 for CEL input: the condition an automation rule's author writes, compiled against the rule
// vocabulary and run against a delivery that may itself come from outside (`payload`).
//
// An ordinary test over its seeds in `make gate-unit`, a fuzz target in the nightly. Five
// properties:
//
//  1. Nothing written makes the compiler or the evaluator panic.
//  2. Every refusal is the port's own: a typed error with an `expression.*` detail code, which is
//     what a field error and a run log can name - never the library's raw error.
//  3. What compiles as a condition answers true or false or one of those refusals; it never answers
//     a value of another kind as if it were one.
//  4. One evaluation ends near the port's bound (automation.md §1.2): the cost limit and the
//     interrupt are what stand between a hostile expression and a stalled rule engine.
//  5. One compilation ends near its deadline (rule 7): the depth limit keeps the checker short,
//     and the deadline is the backstop.
func FuzzCompileCondition(f *testing.F) {
	for _, seed := range []string{
		`item.type == 'TASK'`,
		`item.labels.exists(l, l == 'approval')`,
		`has(item.due_at) && item.due_at < now`,
		`event.type.startsWith('de.hubtask.work.item')`,
		`payload.action == 'opened' && size(payload.items) > 2`,
		`actor.kind == 'USER' || tenant.settings.locale == 'de'`,
		`now.getHours() >= 8 && now.getHours() < 18`,
		`item.title.matches('^[a-z]+$')`,
		`[1,2,3].map(x, [1,2,3].map(y, [1,2,3].map(z, x*y*z))).size() > 0`,
		`payload.items.all(a, payload.items.all(b, payload.items.all(c, a != b || b != c)))`,
		`'a' * 3`,
		`1 / 0 == 1`,
		`item.missing.deeper == 1`,
		`item.title`,
		`unknown == 1`,
		`)`,
		``,
		`"` + strings.Repeat("a", port.MaxLength) + `"`,
		`item.title + item.title + item.title == ''`,
		"payload.items.exists(x, x.matches('(a+)+$'))",
		// Each `.map` wraps the type in another list; 240 of them took the checker 3.7 s.
		"size([1]" + strings.Repeat(".map(a,[a])", 240) + ") > 0",
	} {
		f.Add(seed)
	}

	compiler := expression.New()
	environment := condition.RuleEnvironment()
	activation := newValues(map[string]any{
		condition.VarEvent: map[string]any{"type": "de.hubtask.work.item.created.v1", "subject": "x"},
		condition.VarItem: map[string]any{
			"type": "TASK", "title": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa!", "labels": []any{"approval", "urgent"},
			"due_at": time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		},
		condition.VarActor:   map[string]any{"kind": "USER", "id": "0192f000-0000-7000-8000-000000000001"},
		condition.VarNow:     time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		condition.VarPayload: map[string]any{"action": "opened", "items": []any{"aaaaaaaaaaaaaaaaaaaaaaaa!", "b", "c", 1.5, true}},
		condition.VarTenant:  map[string]any{"settings": map[string]any{"locale": "de"}},
	})

	f.Fuzz(func(t *testing.T, text string) {
		compiling := time.Now()
		program, err := compiler.Compile(text, environment, port.Boolean)
		if took := time.Since(compiling); took > 2*port.CompileTimeout {
			t.Fatalf("compiling %q took %v, past the %v deadline", text, took, port.CompileTimeout)
		}
		if err != nil {
			portRefusal(t, "compiling", text, err)
			return
		}

		started := time.Now()
		value, err := program.Evaluate(context.Background(), activation)
		if took := time.Since(started); took > 20*port.Timeout {
			t.Fatalf("%q ran for %v, far past the %v bound", text, took, port.Timeout)
		}
		if err != nil {
			portRefusal(t, "evaluating", text, err)
			return
		}
		if value.Text != "true" && value.Text != "false" {
			t.Fatalf("%q answered %+v as a condition", text, value)
		}
	})
}

// portRefusal fails unless err is one the port's callers know how to name.
func portRefusal(t *testing.T, doing, text string, err error) {
	t.Helper()
	code := shared.AsError(err).DetailCode
	if !strings.HasPrefix(code, "expression.") {
		t.Fatalf("%s %q answered %v, not a coded refusal", doing, text, err)
	}
}
