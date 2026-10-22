// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

// The codes a rule draft names what it could not draft with (UC-AUT-10 check 4). Message codes,
// never sentences (rule 8): the client says them.
const (
	draftKindNotOffered       = "automation.draft.kind_not_offered"
	draftReferenceNotShown    = "automation.draft.reference_not_shown"
	draftFieldNotDrafted      = "automation.draft.field_not_drafted"
	draftBranchNotDrafted     = "automation.draft.branch_not_drafted"
	draftScheduleNotDrafted   = "automation.draft.schedule_not_drafted"
	draftConditionUnexpressed = "automation.draft.condition_not_expressible"
	draftNoTrigger            = "automation.draft.no_trigger"
	draftNoAction             = "automation.draft.no_action"
)

// The bounds a draft is held to: tighter than a rule's (automation.md §1), because a draft is a
// start a person finishes on the canvas, not a program.
const (
	maxDraftedActions   = 10
	maxDraftedSentences = 10
	maxDraftedText      = 500
	maxDraftedQuotes    = 10
)

// notDraftedKey is what a rule draft says it could not draft: shown to the person, never applied.
const notDraftedKey = "not_drafted"

// ruleDraft reads a model's answer as part of a rule's input, held to what the material showed.
//
// Everything is checked against the sets the material carried and nothing else: a kind outside the
// allowlist, an identifier nobody was shown, a field the table does not draft - each is dropped
// with a code saying so, and the rest stands. A condition arrives as sentences and is compiled here
// (automation.Sentence): no byte of a model's answer becomes expression text except as a literal
// that compiler escaped (rule 9's reasoning, applied to the rules' language).
//
// What it never writes is `run_as`, the name, the throttle or what a failure does: the person who
// saves the draft decides those (automation.md §2.1).
type ruleDraft struct {
	sets       *RuleSets
	conditions expression.Compiler
	notDrafted []any
}

// ruleFrom answers the payload and how many parts were dropped, or false where the answer cannot be
// read at all.
func ruleFrom(text string, sets *RuleSets, conditions expression.Compiler) (map[string]any, int, bool) {
	answered, ok := objectFrom(text)
	if !ok || sets == nil {
		return nil, 0, false
	}
	draft := ruleDraft{sets: sets, conditions: conditions}
	payload := map[string]any{"scope": draft.scope(answered["scope"])}

	if trigger, ok := draft.trigger(answered["trigger"]); ok {
		payload["trigger"] = trigger
	} else {
		draft.note(draftNoTrigger, nil)
	}
	if expr := draft.condition(answered["conditions"]); expr != "" {
		payload["conditions"] = []any{map[string]any{"expr": expr}}
	}
	if actions := draft.actions(answered["actions"]); len(actions) > 0 {
		payload["actions"] = actions
	} else {
		draft.note(draftNoAction, nil)
	}
	dropped := len(draft.notDrafted)
	draft.quotes(answered[notDraftedKey])
	if len(draft.notDrafted) > 0 {
		payload[notDraftedKey] = draft.notDrafted
	}
	return payload, dropped, true
}

func (d *ruleDraft) note(code string, params map[string]any) {
	entry := map[string]any{"code": code}
	if len(params) > 0 {
		entry["params"] = params
	}
	d.notDrafted = append(d.notDrafted, entry)
}

// scope is the place a model chose from the places shown, or the place asked about.
func (d *ruleDraft) scope(value any) map[string]any {
	asked := scopeMap(d.sets.Scope)
	chosen, isObject := value.(map[string]any)
	if !isObject {
		return asked
	}
	kind, _ := chosen["type"].(string)
	id, _ := chosen["id"].(string)
	if kind == string(d.sets.Scope.Type) && id == d.sets.Scope.ID.String() {
		return asked
	}
	for _, place := range d.sets.Scopes {
		if string(place.Scope.Type) == kind && place.Scope.ID.String() == id {
			return scopeMap(place.Scope)
		}
	}
	d.note(draftReferenceNotShown, map[string]any{"field": "scope"})
	return asked
}

func scopeMap(scope automation.Scope) map[string]any {
	out := map[string]any{"type": string(scope.Type)}
	if !scope.ID.IsZero() {
		out["id"] = scope.ID.String()
	}
	return out
}

// trigger reads a trigger of a kind a draft may start on, with only the fields that kind uses.
func (d *ruleDraft) trigger(value any) (map[string]any, bool) {
	answered, isObject := value.(map[string]any)
	if !isObject {
		return nil, false
	}
	kind, _ := answered["kind"].(string)
	switch {
	case kind == string(automation.TriggerSchedule):
		d.note(draftScheduleNotDrafted, nil)
		return nil, false
	case !slices.Contains(draftedTriggers, automation.TriggerKind(kind)):
		d.note(draftKindNotOffered, map[string]any{"kind": kind})
		return nil, false
	}

	trigger := automation.Trigger{Kind: automation.TriggerKind(kind)}
	switch trigger.Kind {
	case automation.TriggerEvent:
		eventType, _ := answered["event_type"].(string)
		if !slices.Contains(event.Types(), event.Type(eventType)) {
			d.note(draftReferenceNotShown, map[string]any{"field": "trigger.event_type"})
			return nil, false
		}
		trigger.EventType = event.Type(eventType)
		fields, _ := answered["changed_fields"].([]any)
		for _, field := range fields {
			name, _ := field.(string)
			if !slices.Contains(changedFields, name) {
				d.note(draftFieldNotDrafted, map[string]any{"field": "trigger.changed_fields"})
				trigger.ChangedFields = nil
				break
			}
			if !slices.Contains(trigger.ChangedFields, name) {
				trigger.ChangedFields = append(trigger.ChangedFields, name)
			}
		}
	case automation.TriggerRelativeDate:
		anchor, _ := answered["anchor"].(string)
		offset, _ := answered["offset"].(string)
		trigger.Anchor = automation.DateAnchor(anchor)
		trigger.Offset = offset
	}
	valid, err := automation.ValidTrigger(trigger)
	if err != nil {
		d.note(draftKindNotOffered, map[string]any{"kind": kind})
		return nil, false
	}

	out := map[string]any{"kind": string(valid.Kind)}
	if valid.EventType != "" {
		out["event_type"] = string(valid.EventType)
	}
	if len(valid.ChangedFields) > 0 {
		fields := make([]any, 0, len(valid.ChangedFields))
		for _, field := range valid.ChangedFields {
			fields = append(fields, field)
		}
		out["changed_fields"] = fields
	}
	if valid.Anchor != "" {
		out["anchor"] = string(valid.Anchor)
		out["offset"] = valid.Offset
	}
	return out, true
}

// condition compiles the answered sentences into one expression, the composer's shape, or answers
// nothing. A sentence that is not one a draft may write is dropped and named; the rest stand.
func (d *ruleDraft) condition(value any) string {
	if value == nil {
		return ""
	}
	answered, isObject := value.(map[string]any)
	if !isObject {
		// An expression written as text, or anything else: a model writes no expression.
		d.note(draftConditionUnexpressed, nil)
		return ""
	}
	mode, _ := answered["mode"].(string)
	items, _ := answered["items"].([]any)
	if mode != automation.ModeAll && mode != automation.ModeAny && mode != automation.ModeNone {
		d.note(draftConditionUnexpressed, nil)
		return ""
	}
	group := automation.SentenceGroup{Mode: mode}
	for _, item := range items {
		sentence, ok := d.sentence(item)
		if !ok || len(group.Items) >= maxDraftedSentences {
			d.note(draftConditionUnexpressed, nil)
			continue
		}
		group.Items = append(group.Items, automation.ConditionNode{Sentence: &sentence})
	}
	expr, err := automation.CompileCondition(automation.ConditionNode{Group: &group})
	if err != nil || expr == "" {
		return ""
	}
	if d.conditions != nil {
		// Once more through the engine's own compiler: what the composer's shapes compile to is
		// held to the editor by the shared vectors, and this holds it to the engine.
		if _, err := d.conditions.Compile(expr, condition.RuleEnvironment(), expression.Boolean); err != nil {
			d.note(draftConditionUnexpressed, nil)
			return ""
		}
	}
	return expr
}

// sentence reads one sentence: a subject a draft may use, its values typed as the operator takes
// them, and every identifier from a set the material showed.
func (d *ruleDraft) sentence(value any) (automation.Sentence, bool) {
	answered, isObject := value.(map[string]any)
	if !isObject {
		return automation.Sentence{}, false
	}
	subject, _ := answered["subject"].(string)
	op, _ := answered["op"].(string)
	if !slices.Contains(draftedSubjects, subject) {
		return automation.Sentence{}, false
	}
	sentence := automation.Sentence{Subject: subject, Op: op}
	switch automation.SentenceTakes(subject, op) {
	case "none":
	case "number", "days":
		number, ok := wholeNumber(answered["a"])
		if !ok {
			return automation.Sentence{}, false
		}
		sentence.A = number
	case "range":
		from, okFrom := wholeNumber(answered["a"])
		to, okTo := wholeNumber(answered["b"])
		if !okFrom || !okTo {
			return automation.Sentence{}, false
		}
		sentence.A, sentence.B = from, to
	default:
		text, isText := answered["a"].(string)
		if !isText {
			return automation.Sentence{}, false
		}
		sentence.A = strings.TrimSpace(text)
	}
	switch subject {
	case "type":
		if !slices.Contains(work.ItemTypes(), work.ItemType(sentence.A)) {
			return automation.Sentence{}, false
		}
	case "label":
		if !offered(d.sets.Labels, sentence.A) {
			return automation.Sentence{}, false
		}
	case "assignee", "actor":
		if (op == "is" || op == "is_not") && !offered(d.sets.People, sentence.A) {
			return automation.Sentence{}, false
		}
	}
	if _, err := automation.CompileSentence(sentence); err != nil {
		return automation.Sentence{}, false
	}
	return sentence, true
}

// wholeNumber reads a JSON integer as its decimal digits; a fraction, a string or a negative
// number is not one.
func wholeNumber(value any) (string, bool) {
	number, isNumber := value.(float64)
	if !isNumber || number < 0 || number != math.Trunc(number) || number > 1000 {
		return "", false
	}
	return strconv.Itoa(int(number)), true
}

func offered(options []Option, id string) bool {
	for _, option := range options {
		if option.ID == id {
			return true
		}
	}
	return false
}

// actions reads the steps: kinds a draft may use, each with only the fields the table drafts.
func (d *ruleDraft) actions(value any) []any {
	steps, _ := value.([]any)
	kept := make([]any, 0, len(steps))
	for index, step := range steps {
		answered, isObject := step.(map[string]any)
		if !isObject {
			continue
		}
		kind, _ := answered["kind"].(string)
		params, _ := answered["params"].(map[string]any)
		if len(kept) >= maxDraftedActions {
			d.note(draftKindNotOffered, map[string]any{"kind": kind})
			continue
		}
		switch kind {
		case "BRANCH":
			d.note(draftBranchNotDrafted, nil)
			continue
		case "STOP":
			if index == len(steps)-1 {
				kept = append(kept, map[string]any{"kind": "STOP"})
			}
			continue
		case "WAIT":
			if _, err := automation.WaitFor(params, ""); err != nil {
				d.note(draftFieldNotDrafted, map[string]any{"field": "WAIT.duration"})
				continue
			}
			kept = append(kept, map[string]any{"kind": "WAIT", "params": map[string]any{"duration": params["duration"]}})
			continue
		}
		if !slices.Contains(d.sets.Actions, kind) {
			d.note(draftKindNotOffered, map[string]any{"kind": kind})
			continue
		}
		if action, ok := d.action(kind, params); ok {
			kept = append(kept, action)
		}
	}
	return kept
}

// action reads one step's params as the table classifies them. A reference to anything not shown
// drops the step: an assignment to somebody else, or a move somewhere else, is not the step asked
// for. A field the table does not draft is dropped alone and named.
func (d *ruleDraft) action(kind string, params map[string]any) (map[string]any, bool) {
	how := draftableActions[kind]
	kept := map[string]any{}
	for field, value := range params {
		use, drafted := how.Fields[field]
		if !drafted {
			d.note(draftFieldNotDrafted, map[string]any{"field": kind + "." + field})
			continue
		}
		text, isText := value.(string)
		if !isText {
			d.note(draftFieldNotDrafted, map[string]any{"field": kind + "." + field})
			continue
		}
		text = strings.TrimSpace(text)
		switch use {
		case usePerson, useCollection, useLabel:
			if !offered(d.shown(use), text) {
				d.note(draftReferenceNotShown, map[string]any{"field": kind + "." + field})
				return nil, false
			}
		case useItemType:
			if !slices.Contains(work.ItemTypes(), work.ItemType(text)) {
				d.note(draftFieldNotDrafted, map[string]any{"field": kind + "." + field})
				continue
			}
		case useText:
			if !draftedText(text, field == "notes" || field == "body") {
				d.note(draftFieldNotDrafted, map[string]any{"field": kind + "." + field})
				continue
			}
		}
		kept[field] = text
	}
	step := map[string]any{"kind": kind}
	if len(kept) > 0 {
		step["params"] = kept
	}
	return step, true
}

func (d *ruleDraft) shown(use fieldUse) []Option {
	switch use {
	case usePerson:
		return d.sets.People
	case useCollection:
		return d.sets.Collections
	default:
		return d.sets.Labels
	}
}

// draftedText is text a step may carry: present, bounded, and on one line unless it is prose.
func draftedText(text string, prose bool) bool {
	if text == "" || len([]rune(text)) > maxDraftedText {
		return false
	}
	for _, r := range text {
		if r == '\n' && prose {
			continue
		}
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// quotes keeps the parts a model says it could not draft, where they are the person's own words:
// a quote that is not in what they wrote is dropped, so that a model cannot put a sentence of its
// own in front of them.
func (d *ruleDraft) quotes(value any) {
	quoted, _ := value.([]any)
	said := folded(d.sets.Words)
	count := 0
	for _, entry := range quoted {
		part, isText := entry.(string)
		part = strings.TrimSpace(part)
		if !isText || part == "" || count >= maxDraftedQuotes {
			continue
		}
		if !strings.Contains(said, folded(part)) {
			continue
		}
		d.notDrafted = append(d.notDrafted, map[string]any{"part": part})
		count++
	}
}

// folded is text compared without case and with every run of white space as one space.
func folded(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}
