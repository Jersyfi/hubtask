// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package automation holds the use cases that write the rules (G-05, automation.md §1).
//
// Running one is somebody else's task. What is here is everything that has to be true before a rule
// is allowed to exist, which is most of the work: a rule is data that will later be executed with a
// service account's rights, so the moment it is written is the last moment anybody looks at it.
package automation

import (
	"errors"
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
)

// Catalogue is the slice of the use case registry this package reads: which action kinds exist, and
// what each one declares.
//
// Narrow rather than the whole registry, for the reason every slice in this codebase is - a
// validator that held Invoke could perform the action it is checking.
type Catalogue interface {
	ByAutomationAction(kind string) (usecase.Descriptor, bool)
}

// deferredActions are the kinds automation.md §1.3 documents and no release serves yet.
//
// They are refused by name and with a code that says so, rather than falling through to "no such
// action". The difference matters to whoever is reading the answer: `HTTP_REQUEST` is in the
// documentation, so being told it does not exist would send them looking for a typo they did not
// make, and being told it is not built yet sends them to the milestone.
//
// The list shrinks as the tasks land - the flow kinds with G-09's first step, the outbound pair
// with the steps that build them, the AI kinds with their own milestone - and
// TestNoDeferredActionIsAlreadyServed fails the build if a kind is left here after the catalogue
// grew one, so removing the entry is not something anybody has to remember.
var deferredActions = []string{
	// Outbound (automation.md §1.3): both call somebody else's server, which is the guarded
	// client's business and lands later in this same task.
	"SEND_WEBHOOK", "HTTP_REQUEST",
	// AI, optional and configured explicitly. The AI port arrives at 0.7.0, and a rule naming one
	// of these is refused the way a retention rule naming a missing notification category was.
	"AI_SUGGEST_FIELDS", "AI_SUMMARIZE", "AI_CLASSIFY",
}

// DeferredActions is the list, for the test that keeps it honest and for the manifest that will
// want it.
func DeferredActions() []string { return slices.Clone(deferredActions) }

// checkedAction is one action of a rule with the use case behind it resolved.
type checkedAction struct {
	action     domain.Action
	descriptor usecase.Descriptor
}

// checkActions resolves every action of a rule against the catalogue, and refuses what cannot be
// run.
//
// Three refusals, each with the field path of the action it is about, so an editor can point at the
// line rather than at the rule:
//
//   - a kind the documentation names and no release serves - not built yet, and the code says so;
//   - a kind nobody has ever named - a typo, and the code says that instead;
//   - a parameter the use case does not declare, which is the same refusal the call itself would
//     give (C-07). A rule saved with a misspelled `parent_id` would fail at a moment nobody is
//     watching, and the writer is the one person who could still fix it.
//
// What is *not* checked here: whether a required parameter is present, and whether its value has
// the right type. A rule supplies some parameters and the run supplies the rest - the entry an
// event is about is not a value a rule can carry - so requiring them at write time would refuse
// every rule that is correct. The run is where the whole input exists and where the registry
// validates it in full.
func checkActions(catalogue Catalogue, actions []domain.Action) ([]checkedAction, error) {
	checked := make([]checkedAction, 0, len(actions))
	var findings []shared.FieldError

	for i, action := range actions {
		path := "/actions/" + itoa(i)

		if slices.Contains(deferredActions, action.Kind) {
			findings = append(findings, shared.FieldError{
				Path: path + "/kind", Code: "automation.action_not_available_yet",
			})
			continue
		}

		if domain.IsFlowAction(action.Kind) {
			// A flow action is the engine's own control structure and is in no catalogue: its
			// parameters were checked by the aggregate, where every other shape question about a
			// rule is answered. It declares no token scope, because it performs nothing - which is
			// what makes `STOP` free of the composition rule that binds every other kind.
			nested, err := branchActions(catalogue, action, path)
			if err != nil {
				return nil, err
			}
			checked = append(checked, nested...)
			continue
		}

		descriptor, found := catalogue.ByAutomationAction(action.Kind)
		if !found {
			findings = append(findings, shared.FieldError{
				Path: path + "/kind", Code: "automation.action_unknown",
			})
			continue
		}

		declared := map[string]bool{}
		for _, field := range descriptor.Input {
			declared[field.Name] = true
		}
		for _, name := range sortedKeys(action.Params) {
			if !declared[name] {
				findings = append(findings, shared.FieldError{
					Path: path + "/params/" + name, Code: "automation.param_unknown",
				})
			}
		}

		checked = append(checked, checkedAction{action: action, descriptor: descriptor})
	}

	if len(findings) > 0 {
		return nil, shared.ErrValidation.
			WithDetail("automation.actions_invalid").
			WithFields(findings...)
	}
	return checked, nil
}

// branchActions resolves what a branch would do, on both of its arms.
//
// Both arms, not the one a condition happens to take: the composition rule (automation.md §2.1) is
// about what a rule *may* do, and a rule whose `else` performs something its writer may not do is
// laundering the same rights the day the condition turns false. A branch that checked only the arm
// it takes would be a check that passes on Monday and fails on Tuesday.
//
// A non-branch flow action resolves to nothing at all, which is right: `WAIT` and `STOP` perform
// no use case and need no right.
func branchActions(
	catalogue Catalogue, action domain.Action, path string,
) ([]checkedAction, error) {
	if action.Kind != domain.ActionBranch {
		return nil, nil
	}

	branch, err := domain.ReadBranch(action.Params, path, 0)
	if err != nil {
		// Unreachable through the aggregate, which read the same parameters when the rule was
		// built. Reported rather than swallowed, because a branch this layer could not read is a
		// branch whose rights it has not checked.
		return nil, err
	}

	var checked []checkedAction
	for _, arm := range [][]domain.Action{branch.Then, branch.Else} {
		nested, err := checkActions(catalogue, arm)
		if err != nil {
			return nil, err
		}
		checked = append(checked, nested...)
	}
	return checked, nil
}

// checkConditions compiles a rule's conditions, and its dedupe key with them.
//
// This is the flip G-06 promised: until the language existed, a non-empty condition was refused
// with a code that said so, because a rule whose owner believes it is filtering and whose behaviour
// says otherwise is worse than one they could not save (E-08). What replaced the refusal is a real
// check - the expression is parsed and type-checked against exactly the names automation.md §1.2
// declares, so a typo is answered to its author with a line and a column while they are still
// looking at it, rather than to a log at three in the morning.
//
// Compiled and discarded. What is being asked here is "would this run", and the engine that will
// run it compiles its own (G-07) - keeping the program would mean caching a rule's compilation in
// the use case that wrote it, which is the wrong place for a cache and the wrong lifetime.
func checkConditions(compiler expression.Compiler, rule domain.Rule) error {
	if compiler == nil {
		// Fail closed. A build with no evaluator wired cannot promise that a condition means what
		// it says, and storing one on that promise is exactly the failure the refusal existed for.
		if len(rule.Conditions) > 0 || rule.Throttle.DedupeKeyExpr != "" {
			return shared.ErrInternal.WithDetail("automation.expression_engine_unavailable")
		}
		return nil
	}

	environment := condition.RuleEnvironment()
	var findings []shared.FieldError
	for i, each := range rule.Conditions {
		if _, err := compiler.Compile(each.Expr, environment, expression.Boolean); err != nil {
			findings = append(findings, findingFor("/conditions/"+itoa(i)+"/expr", err))
		}
	}
	// A branch's condition is a condition, and it is compiled here for the reason the rule's own
	// are: a branch whose expression cannot be read would take the same arm for ever, which is a
	// rule whose author believes it is deciding something (E-08's lesson, applied one level down).
	findings = append(findings, branchFindings(compiler, environment, rule.Actions, "/actions")...)
	if rule.Throttle.DedupeKeyExpr != "" {
		if _, err := // A dedupe key renders a value that collapses runs meaning the same thing, so it is a
			// template rather than a condition - `item.id` is a string, not a decision.
			compiler.Compile(rule.Throttle.DedupeKeyExpr, environment, expression.Text); err != nil {
			findings = append(findings, findingFor("/throttle/dedupe_key_expr", err))
		}
	}
	if len(findings) == 0 {
		return nil
	}
	return shared.ErrValidation.
		WithDetail("automation.condition_invalid").
		WithFields(findings...)
}

// branchFindings compiles every branch condition in a list, and in the branches below it.
//
// Depth-first and all the way down, because the aggregate has already bounded both the depth and
// the count: what is left is to compile what is there.
func branchFindings(
	compiler expression.Compiler, environment expression.Environment,
	actions []domain.Action, path string,
) []shared.FieldError {
	var findings []shared.FieldError
	for i, action := range actions {
		if action.Kind != domain.ActionBranch {
			continue
		}
		at := path + "/" + itoa(i)

		branch, err := domain.ReadBranch(action.Params, at, 0)
		if err != nil {
			// Unreachable through the aggregate, which read the same parameters. Left to the
			// caller's own refusal rather than turned into a second one here.
			continue
		}
		if _, err := compiler.Compile(
			branch.Condition, environment, expression.Boolean); err != nil {
			findings = append(findings, findingFor(at+"/params/condition", err))
		}
		findings = append(findings,
			branchFindings(compiler, environment, branch.Then, at+"/params/then")...)
		findings = append(findings,
			branchFindings(compiler, environment, branch.Else, at+"/params/else")...)
	}
	return findings
}

// findingFor renders the compiler's refusal at the field it is about, carrying the position it
// reported so an editor can put the cursor there.
func findingFor(path string, err error) shared.FieldError {
	finding := shared.FieldError{Path: path, Code: expression.CodeSyntax}

	var coded *shared.Error
	if errors.As(err, &coded) {
		finding.Code = coded.DetailCode
		if len(coded.Params) > 0 {
			finding.Params = coded.Params
		}
	}
	return finding
}

// requiredScopes is what the rule's actions would need of whoever performs them, in the order they
// are first asked for.
//
// The token scope the catalogue already declares, rather than a second list of rights per action: a
// use case states what a credential needs in exactly one place, and reading it here is what keeps
// the composition rule below in step with the catalogue rather than with somebody's memory of it.
func requiredScopes(actions []checkedAction) []string {
	var scopes []string
	for _, checked := range actions {
		scope := checked.descriptor.TokenScope
		if scope != "" && !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	return scopes
}

func sortedKeys(params map[string]any) []string {
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	// Sorted, so that two identical requests produce identical answers - a client that compares
	// responses, and a test, both depend on it.
	slices.Sort(names)
	return names
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

// trimmed is the one string helper this package needs, and it exists so that `strings` is imported
// once rather than in every file of it.
func trimmed(value string) string { return strings.TrimSpace(value) }
