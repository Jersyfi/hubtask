// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	ruleID   = shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	tenantID = shared.ID("01936f2a-7c1e-7000-8000-0000000000a2")
	authorID = shared.ID("01936f2a-7c1e-7000-8000-0000000000a3")
	runAsID  = shared.ID("01936f2a-7c1e-7000-8000-0000000000a4")
	hubID    = shared.ID("01936f2a-7c1e-7000-8000-0000000000a5")
	written  = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
)

func validInput() automation.NewRuleInput {
	return automation.NewRuleInput{
		ID: ruleID, TenantID: tenantID, Name: "Escalate overdue approvals",
		Scope: automation.Scope{Type: automation.ScopeTenant},
		RunAs: runAsID,
		Trigger: automation.Trigger{
			Kind: automation.TriggerEvent, EventType: event.ItemOverdue,
		},
		Actions:   []automation.Action{{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "x"}}},
		CreatedBy: authorID, Now: written,
	}
}

// fieldCodes reads the per-field findings out of a refusal, which is what an editor points at and
// what an agent acts on.
func fieldCodes(t *testing.T, err error) map[string]string {
	t.Helper()

	var coded *shared.Error
	if !errors.As(err, &coded) {
		t.Fatalf("the refusal carries no fields: %v", err)
	}
	found := map[string]string{}
	for _, finding := range coded.Fields {
		found[finding.Path] = finding.Code
	}
	return found
}

func TestARuleIsWrittenSwitchedOff(t *testing.T) {
	rule, err := automation.NewRule(validInput())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	// The decision the acceptance criteria call "enabling is separate from creating": a rule that
	// acted the moment it was saved would give nobody the chance to read it back first.
	if rule.Enabled {
		t.Error("a newly written rule is switched on")
	}
	if rule.Version != 1 {
		t.Errorf("version %d, want 1", rule.Version)
	}
	if rule.OnError != automation.OnErrorStop {
		t.Errorf("on_error %q, want the STOP default", rule.OnError)
	}
	if rule.Conditions == nil {
		t.Error("conditions came back null rather than empty - what is stored is what is read back")
	}
	if !rule.CreatedAt.Equal(written) || !rule.UpdatedAt.Equal(written) {
		t.Errorf("stamped %v/%v, want %v", rule.CreatedAt, rule.UpdatedAt, written)
	}
}

// The flip G-05 wrote this test for. It refused a non-empty condition while there was no language;
// with G-06 the aggregate keeps it, and whether the text is an expression is the compiler's
// question - asked in the application layer, because core/domain may not hold an evaluator.
func TestAConditionIsKeptAndLeftForTheCompiler(t *testing.T) {
	in := validInput()
	in.Conditions = []automation.Condition{{Expr: "  item.type == 'TASK'  "}}

	rule, err := automation.NewRule(in)
	if err != nil {
		t.Fatalf("a condition was refused by the aggregate: %v", err)
	}
	if len(rule.Conditions) != 1 {
		t.Fatalf("%d conditions, want one", len(rule.Conditions))
	}
	// Trimmed, so that what is stored is what a compiler was handed.
	if rule.Conditions[0].Expr != "item.type == 'TASK'" {
		t.Errorf("kept %q", rule.Conditions[0].Expr)
	}
}

// An empty expression is not a condition at all, and is still refused as the empty field it is.
func TestAnEmptyConditionIsStillRefused(t *testing.T) {
	in := validInput()
	in.Conditions = []automation.Condition{{Expr: "   "}}

	_, err := automation.NewRule(in)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want ErrValidation", err)
	}
	if code := fieldCodes(t, err)["/conditions/0/expr"]; code != "automation.condition_empty" {
		t.Errorf("the refusal says %q, want automation.condition_empty", code)
	}
}

// Each condition is evaluated with its own timeout, so the count is what bounds a match's cost.
func TestTooManyConditionsAreRefused(t *testing.T) {
	in := validInput()
	for range automation.MaxConditions + 1 {
		in.Conditions = append(in.Conditions, automation.Condition{Expr: "true"})
	}

	_, err := automation.NewRule(in)
	if code := fieldCodes(t, err)["/conditions"]; code != "automation.too_many_conditions" {
		t.Errorf("the refusal says %q, want automation.too_many_conditions", code)
	}
}

// An empty list is not a condition at all: it is a rule that runs on every match, which is what the
// model has always meant and needs no evaluator.
func TestNoConditionsIsAccepted(t *testing.T) {
	in := validInput()
	in.Conditions = []automation.Condition{}

	if _, err := automation.NewRule(in); err != nil {
		t.Fatalf("a rule with no conditions was refused: %v", err)
	}
}

// A dedupe key is an expression by another name, and is kept with the conditions and compiled with
// them - the other half of the same flip.
func TestADedupeKeyIsKeptWithTheConditions(t *testing.T) {
	in := validInput()
	in.Throttle = automation.Throttle{MaxRunsPerHour: 100, DedupeKeyExpr: " item.id "}

	rule, err := automation.NewRule(in)
	if err != nil {
		t.Fatalf("a dedupe key was refused by the aggregate: %v", err)
	}
	if rule.Throttle.DedupeKeyExpr != "item.id" {
		t.Errorf("kept %q", rule.Throttle.DedupeKeyExpr)
	}
	if rule.Throttle.MaxRunsPerHour != 100 {
		t.Errorf("max_runs_per_hour %d", rule.Throttle.MaxRunsPerHour)
	}
}

func TestAThrottleWithoutAnExpressionIsKept(t *testing.T) {
	in := validInput()
	in.Throttle = automation.Throttle{MaxRunsPerHour: 100}

	rule, err := automation.NewRule(in)
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
	if rule.Throttle.MaxRunsPerHour != 100 {
		t.Errorf("max_runs_per_hour %d, want 100", rule.Throttle.MaxRunsPerHour)
	}
}

func TestTheTriggerIsCheckedPerKind(t *testing.T) {
	cases := []struct {
		name    string
		trigger automation.Trigger
		path    string
		code    string
	}{
		{
			name:    "an event trigger with no type",
			trigger: automation.Trigger{Kind: automation.TriggerEvent},
			path:    "/trigger/event_type", code: "automation.event_type_required",
		},
		{
			name: "an event type nobody publishes",
			trigger: automation.Trigger{
				Kind: automation.TriggerEvent, EventType: event.Type("de.hubtask.work.item.invented.v1"),
			},
			path: "/trigger/event_type", code: "automation.event_type_unknown",
		},
		{
			name:    "a schedule with no rule",
			trigger: automation.Trigger{Kind: automation.TriggerSchedule, Timezone: "Europe/Berlin"},
			path:    "/trigger/rrule", code: "automation.rrule_required",
		},
		{
			name:    "a schedule with no zone",
			trigger: automation.Trigger{Kind: automation.TriggerSchedule, RRule: "FREQ=WEEKLY;BYDAY=MO"},
			path:    "/trigger/timezone", code: "automation.timezone_required",
		},
		{
			name: "a zone the platform does not know",
			trigger: automation.Trigger{
				Kind: automation.TriggerSchedule, RRule: "FREQ=WEEKLY", Timezone: "Middle/Earth",
			},
			path: "/trigger/timezone", code: "automation.timezone_unknown",
		},
		{
			name:    "a relative date with no anchor",
			trigger: automation.Trigger{Kind: automation.TriggerRelativeDate, Offset: "-PT24H"},
			path:    "/trigger/anchor", code: "automation.anchor_required",
		},
		{
			name: "a relative date with no offset",
			trigger: automation.Trigger{
				Kind: automation.TriggerRelativeDate, Anchor: automation.AnchorDueDate,
			},
			path: "/trigger/offset", code: "automation.offset_required",
		},
		{
			name:    "no kind at all",
			trigger: automation.Trigger{},
			path:    "/trigger/kind", code: "automation.trigger_kind_required",
		},
		{
			name:    "a kind nobody declares",
			trigger: automation.Trigger{Kind: automation.TriggerKind("TELEPATHY")},
			path:    "/trigger/kind", code: "automation.trigger_kind_unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			in.Trigger = tc.trigger

			_, err := automation.NewRule(in)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want ErrValidation", err)
			}
			if code := fieldCodes(t, err)[tc.path]; code != tc.code {
				t.Errorf("%s says %q, want %q", tc.path, code, tc.code)
			}
		})
	}
}

// A trigger carrying a cron expression and an event type is a rule nobody can read, and the day
// somebody edits one from a schedule into an event the leftover field would decide something.
func TestAFieldOfAnotherKindIsRefused(t *testing.T) {
	cases := map[string]struct {
		trigger automation.Trigger
		path    string
	}{
		"a cron on an event trigger": {
			trigger: automation.Trigger{
				Kind: automation.TriggerEvent, EventType: event.ItemOverdue,
				RRule: "FREQ=WEEKLY",
			},
			path: "/trigger/rrule",
		},
		"an event type on a schedule": {
			trigger: automation.Trigger{
				Kind: automation.TriggerSchedule, RRule: "FREQ=WEEKLY", Timezone: "Europe/Berlin",
				EventType: event.ItemOverdue,
			},
			path: "/trigger/event_type",
		},
		"an offset on a manual trigger": {
			trigger: automation.Trigger{Kind: automation.TriggerManual, Offset: "-PT24H"},
			path:    "/trigger/offset",
		},
		"changed fields on a jumble trigger": {
			trigger: automation.Trigger{
				Kind: automation.TriggerJumbleEntry, ChangedFields: []string{"title"},
			},
			path: "/trigger/changed_fields",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput()
			in.Trigger = tc.trigger

			_, err := automation.NewRule(in)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want ErrValidation", err)
			}
			if code := fieldCodes(t, err)[tc.path]; code != "automation.trigger_field_not_for_kind" {
				t.Errorf("%s says %q, want automation.trigger_field_not_for_kind", tc.path, code)
			}
		})
	}
}

func TestTheKindsThatConfigureNothingAreAccepted(t *testing.T) {
	for _, kind := range []automation.TriggerKind{
		automation.TriggerInboundWebhook, automation.TriggerManual, automation.TriggerJumbleEntry,
	} {
		t.Run(string(kind), func(t *testing.T) {
			in := validInput()
			in.Trigger = automation.Trigger{Kind: kind}

			rule, err := automation.NewRule(in)
			if err != nil {
				t.Fatalf("writing the rule: %v", err)
			}
			if rule.Trigger.Kind != kind {
				t.Errorf("kind %q, want %q", rule.Trigger.Kind, kind)
			}
		})
	}
}

func TestAnOffsetIsRead(t *testing.T) {
	accepted := []string{"-PT24H", "PT30M", "P3D", "-P1W", "PT1H30M", "-P1DT12H"}
	for _, offset := range accepted {
		t.Run("accepts "+offset, func(t *testing.T) {
			if _, err := automation.ValidOffset(offset); err != nil {
				t.Errorf("%q was refused: %v", offset, err)
			}
		})
	}

	// Years and months are refused rather than converted: "one month before it is due" is a
	// different instant depending on which month.
	refused := []string{"", "24h", "P", "PT", "P1M", "P1Y", "PT1X", "3D", "P1DT", "-P400D"}
	for _, offset := range refused {
		t.Run("refuses "+offset, func(t *testing.T) {
			if _, err := automation.ValidOffset(offset); err == nil {
				t.Errorf("%q was accepted", offset)
			}
		})
	}
}

func TestTheScopeNamesWhatItsLevelNeeds(t *testing.T) {
	cases := []struct {
		name  string
		scope automation.Scope
		path  string
		code  string
	}{
		{
			name:  "a hub with no identifier",
			scope: automation.Scope{Type: automation.ScopeHub},
			path:  "/scope/id", code: "automation.scope_id_required",
		},
		{
			name:  "a collection with no identifier",
			scope: automation.Scope{Type: automation.ScopeCollection},
			path:  "/scope/id", code: "automation.scope_id_required",
		},
		{
			// Refused rather than ignored: a caller that named both means one of them, and
			// guessing which would be guessing at what the rule watches.
			name:  "a tenant scope naming a hub",
			scope: automation.Scope{Type: automation.ScopeTenant, ID: hubID},
			path:  "/scope/id", code: "automation.scope_id_not_allowed",
		},
		{
			name:  "a level nobody declares",
			scope: automation.Scope{Type: automation.ScopeType("GALAXY")},
			path:  "/scope/type", code: "automation.scope_type_unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			in.Scope = tc.scope

			_, err := automation.NewRule(in)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want ErrValidation", err)
			}
			if code := fieldCodes(t, err)[tc.path]; code != tc.code {
				t.Errorf("%s says %q, want %q", tc.path, code, tc.code)
			}
		})
	}
}

// The path is what the authoriser reads, and it is written once here rather than at each call site.
func TestTheScopePathRunsFromTheTenantDownwards(t *testing.T) {
	cases := map[string][]identity.Scope{
		"TENANT": {identity.TenantScope()},
		"HUB":    {identity.TenantScope(), identity.HubScope(hubID)},
		"COLLECTION": {
			identity.TenantScope(), identity.CollectionScope(hubID),
		},
	}
	for level, want := range cases {
		t.Run(level, func(t *testing.T) {
			scope := automation.Scope{Type: automation.ScopeType(level), ID: hubID}
			if level == "TENANT" {
				scope.ID = ""
			}

			got := scope.Path()
			if len(got) != len(want) {
				t.Fatalf("path %v, want %v", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("step %d is %v, want %v", i, got[i], want[i])
				}
			}
		})
	}
}

func TestARuleNeedsAtLeastOneActionAndNotTooMany(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		in := validInput()
		in.Actions = nil

		_, err := automation.NewRule(in)
		if code := fieldCodes(t, err)["/actions"]; code != "automation.actions_required" {
			t.Errorf("the refusal says %q, want automation.actions_required", code)
		}
	})

	t.Run("too many", func(t *testing.T) {
		in := validInput()
		in.Actions = make([]automation.Action, automation.MaxActions+1)
		for i := range in.Actions {
			in.Actions[i] = automation.Action{Kind: "ADD_LABEL"}
		}

		_, err := automation.NewRule(in)
		if code := fieldCodes(t, err)["/actions"]; code != "automation.too_many_actions" {
			t.Errorf("the refusal says %q, want automation.too_many_actions", code)
		}
	})

	t.Run("one with no kind", func(t *testing.T) {
		in := validInput()
		in.Actions = []automation.Action{{Kind: "  "}}

		_, err := automation.NewRule(in)
		if code := fieldCodes(t, err)["/actions/0/kind"]; code != "automation.action_kind_required" {
			t.Errorf("the refusal says %q, want automation.action_kind_required", code)
		}
	})
}

// What is stored is what is read back, and a null would come back as one.
func TestAnActionWithoutParametersCarriesAnEmptyMap(t *testing.T) {
	in := validInput()
	in.Actions = []automation.Action{{Kind: "COMPLETE"}}

	rule, err := automation.NewRule(in)
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
	if rule.Actions[0].Params == nil {
		t.Error("the parameters came back null")
	}
}

func TestTheNameIsBounded(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		in := validInput()
		in.Name = "   "

		_, err := automation.NewRule(in)
		if code := fieldCodes(t, err)["/name"]; code != "automation.name_required" {
			t.Errorf("the refusal says %q, want automation.name_required", code)
		}
	})

	t.Run("too long", func(t *testing.T) {
		in := validInput()
		in.Name = strings.Repeat("a", automation.MaxNameLength+1)

		_, err := automation.NewRule(in)
		if code := fieldCodes(t, err)["/name"]; code != "automation.name_too_long" {
			t.Errorf("the refusal says %q, want automation.name_too_long", code)
		}
	})
}

func TestOnErrorTakesItsThreeValuesAndTheDefault(t *testing.T) {
	for _, value := range []automation.OnError{
		automation.OnErrorStop, automation.OnErrorContinue, automation.OnErrorRetry,
	} {
		t.Run(string(value), func(t *testing.T) {
			in := validInput()
			in.OnError = value

			rule, err := automation.NewRule(in)
			if err != nil {
				t.Fatalf("writing the rule: %v", err)
			}
			if rule.OnError != value {
				t.Errorf("on_error %q, want %q", rule.OnError, value)
			}
		})
	}

	t.Run("unknown", func(t *testing.T) {
		in := validInput()
		in.OnError = automation.OnError("PANIC")

		_, err := automation.NewRule(in)
		if code := fieldCodes(t, err)["/on_error"]; code != "automation.on_error_unknown" {
			t.Errorf("the refusal says %q, want automation.on_error_unknown", code)
		}
	})
}

// A rule somebody has looked at and turned back on is a rule whose run of failures has been dealt
// with. Leaving the count would mean the next single failure disables it again.
func TestEnablingClearsTheFailureCountAndDisablingDoesNot(t *testing.T) {
	rule, err := automation.NewRule(validInput())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
	rule.FailureCount = 3

	later := written.Add(time.Hour)
	enabled := rule.Enable(later)
	if !enabled.Enabled || enabled.FailureCount != 0 {
		t.Errorf("enabled=%v failures=%d, want on and zero", enabled.Enabled, enabled.FailureCount)
	}
	if enabled.Version != rule.Version+1 || !enabled.UpdatedAt.Equal(later) {
		t.Errorf("version %d at %v, want %d at %v",
			enabled.Version, enabled.UpdatedAt, rule.Version+1, later)
	}

	// What a run of failures concluded is still true, and somebody switching the rule off by hand
	// has not fixed it.
	disabled := enabled.Disable(later)
	disabled.FailureCount = 3
	again := disabled.Disable(later)
	if again.FailureCount != 3 {
		t.Errorf("disabling cleared the count to %d", again.FailureCount)
	}
}

func TestTheVersionIsTheOptimisticLock(t *testing.T) {
	rule, err := automation.NewRule(validInput())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	if err := rule.EnsureVersion(0); err != nil {
		// Zero is how a client that read nothing writes without pretending to have read something.
		t.Errorf("no expectation was refused: %v", err)
	}
	if err := rule.EnsureVersion(1); err != nil {
		t.Errorf("the current version was refused: %v", err)
	}
	if err := rule.EnsureVersion(2); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("error %v, want ErrConflict", err)
	}
}

func TestARuleWithoutItsIdentifiersIsAnInternalError(t *testing.T) {
	for _, missing := range []string{"id", "tenant", "author"} {
		t.Run(missing, func(t *testing.T) {
			in := validInput()
			switch missing {
			case "id":
				in.ID = ""
			case "tenant":
				in.TenantID = ""
			case "author":
				in.CreatedBy = ""
			}

			// Internal rather than a validation refusal: no caller sends these, so a missing one is
			// this system's mistake and not somebody's input.
			if _, err := automation.NewRule(in); !errors.Is(err, shared.ErrInternal) {
				t.Errorf("error %v, want ErrInternal", err)
			}
		})
	}
}

func TestARuleNeedsAnAccountToRunAs(t *testing.T) {
	in := validInput()
	in.RunAs = ""

	_, err := automation.NewRule(in)
	if code := fieldCodes(t, err)["/run_as"]; code != "automation.run_as_required" {
		t.Errorf("the refusal says %q, want automation.run_as_required", code)
	}
}
