// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package automation holds the use cases that write the rules (G-05, automation.md §1).
//
// Running one is somebody else's task. What is here is everything that has to be true before a rule
// is allowed to exist, which is most of the work: a rule is data that will later be executed with a
// service account's rights, so the moment it is written is the last moment anybody looks at it.
package automation

import (
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
// The list shrinks as the tasks land - the outbound and flow kinds with G-09, the AI kinds with
// their own milestone - and TestNoDeferredActionIsAlreadyServed fails the build if a kind is left
// here after the catalogue grew one, so removing the entry is not something anybody has to
// remember.
var deferredActions = []string{
	// Outbound (automation.md §1.3): both call somebody else's server, which is the guarded
	// client's business and G-09's task.
	"SEND_WEBHOOK", "HTTP_REQUEST",
	// Flow: none of the three is a use case at all. They are the engine's own control structures,
	// and they mean nothing until there is an engine to control.
	"WAIT", "BRANCH", "STOP",
	// AI, optional and configured explicitly.
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
