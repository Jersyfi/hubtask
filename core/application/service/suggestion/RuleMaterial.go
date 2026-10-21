// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// draftRulePrompt drafts an automation rule from a sentence (UC-AUT-10).
const draftRulePrompt = "draft-rule"

// automationScope is the token scope rule drafts are read and answered under: drafting a rule is
// automation work, whatever door it arrives through.
const automationScope = "automation:manage"

// Permitter is the authoriser's quiet question - may this actor, without an audit entry - for a
// material that narrows a set to where the asker may act rather than refusing the whole question.
type Permitter interface {
	Permits(ctx context.Context, actor appshared.ActorContext, request access.Request) (bool, error)
}

// RuleSets are what a rule drafted for this question may name, already narrowed to what the person
// asking may read and to what their sentence names (UC-AUT-10 check 2). The narrowing checks an
// answer against exactly these: what a model was not shown, it cannot pick.
type RuleSets struct {
	// Scope is where the rule applies unless the answer narrows it to one of Scopes.
	Scope automation.Scope
	// Scopes are the hubs and collections the asker may write rules in that the sentence names.
	Scopes []ScopeOption
	// People, Collections and Labels are the identifiers an action or a condition may name.
	People      []Option
	Collections []Option
	Labels      []Option
	// Actions are the kinds a draft may use: the allowlist, less what this build does not serve.
	Actions []string
	// Words are the person's sentence as they wrote it, against which a quoted part is checked.
	Words string
}

// ScopeOption is one place a rule may apply.
type ScopeOption struct {
	Scope automation.Scope
	Name  string
}

// The bound on each set: enough for any sentence a person writes, and far short of a directory.
const maxShown = 20

// The bounds on what is read to build the sets. A workspace larger than this is narrowed by its
// first pages; a name further down is not shown, and its part of the sentence is named as not
// drafted - a refusal to guess rather than a silent miss.
const (
	maxMembershipsRead = 200
	maxContainersRead  = 100
)

// ruleMaterial is what a rule is drafted against: the vocabulary this workspace offers, and the
// people, places and labels the sentence names that the person asking may see.
func (s CatalogueSources) ruleMaterial(
	ctx context.Context, actor appshared.ActorContext, question Question, kept integration.KeptOut,
) (Material, error) {
	if question.TargetID != actor.AccountID {
		// A draft is the asker's own; a question naming somebody else is not one this answers.
		return Material{}, shared.ErrNotFound.WithDetail("suggestions.not_found")
	}
	scope, err := automation.ValidScope(question.Scope)
	if err != nil {
		return Material{}, err
	}
	words := question.Words
	if s.Text != nil {
		words = s.Text.NFC(words)
	}
	matcher := newNameMatcher(words)

	sets := RuleSets{Scope: scope, Words: question.Words, Actions: s.draftable()}

	containers, err := s.containers(ctx, actor)
	if err != nil {
		return Material{}, err
	}
	scopeName := ""
	for _, container := range containers {
		if scope.Type != automation.ScopeTenant && container.id == scope.ID.String() {
			scopeName = container.name
		}
	}
	var shownCollections []container
	for _, each := range containers {
		own := scope.Type != automation.ScopeTenant && each.id == scope.ID.String()
		if !own && !matcher.names(s.nfc(each.name)) {
			continue
		}
		if each.kind == work.ContainerCollection {
			shownCollections = append(shownCollections, each)
			sets.Collections = append(sets.Collections, Option{ID: each.id, Name: kept.Scrub(each.name)})
		}
		permitted, err := s.mayAutomate(ctx, actor, each)
		if err != nil {
			return Material{}, err
		}
		if permitted {
			sets.Scopes = append(sets.Scopes, ScopeOption{Scope: each.scope(), Name: kept.Scrub(each.name)})
		}
	}

	sets.People, err = s.people(ctx, actor, scope, containers, matcher, kept)
	if err != nil {
		return Material{}, err
	}
	sets.Labels, err = s.labels(ctx, actor, shownCollections, matcher, kept)
	if err != nil {
		return Material{}, err
	}
	sets.Scopes = firstShown(sortScopes(sets.Scopes))
	sets.Collections = firstShown(sortOptions(sets.Collections))
	sets.Labels = firstShown(sortOptions(sets.Labels))

	return Material{
		Content: writtenRuleMaterial(sets, scopeName, kept),
		// The asker's own identifier: a draft has no state of the workspace it was made from, and
		// the save checks everything again (CreateRule).
		Digest: domain.Digest(actor.AccountID.String(), ""),
		Rule:   &sets,
	}, nil
}

func (s CatalogueSources) nfc(text string) string {
	if s.Text == nil {
		return text
	}
	return s.Text.NFC(text)
}

// draftable is the allowlist less what this build does not serve.
func (s CatalogueSources) draftable() []string {
	served := map[string]bool{}
	for _, kind := range s.Served {
		served[kind] = true
	}
	kinds := make([]string, 0, len(draftableActions))
	for kind := range draftableActions {
		if served[kind] {
			kinds = append(kinds, kind)
		}
	}
	sort.Strings(kinds)
	return kinds
}

// container is one hub or collection as the listing answered it.
type container struct {
	id, name, parent string
	kind             work.ContainerType
}

func (c container) scope() automation.Scope {
	id, _ := shared.ParseID(c.id)
	if c.kind == work.ContainerHub {
		return automation.Scope{Type: automation.ScopeHub, ID: id}
	}
	return automation.Scope{Type: automation.ScopeCollection, ID: id}
}

// containers reads the hubs and their collections the asker may see, through the ordinary listing.
func (s CatalogueSources) containers(ctx context.Context, actor appshared.ActorContext) ([]container, error) {
	hubs, err := s.listContainers(ctx, actor, usecase.Input{"type": string(work.ContainerHub)})
	if err != nil {
		return nil, err
	}
	all := append([]container(nil), hubs...)
	for _, hub := range hubs {
		if len(all) >= maxContainersRead {
			break
		}
		collections, err := s.listContainers(ctx, actor, usecase.Input{"parent_id": hub.id})
		if err != nil {
			return nil, err
		}
		all = append(all, collections...)
	}
	if len(all) > maxContainersRead {
		all = all[:maxContainersRead]
	}
	return all, nil
}

func (s CatalogueSources) listContainers(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) ([]container, error) {
	in["size"] = maxContainersRead
	out, err := s.Catalogue.Invoke(ctx, "ListContainers", actor, in)
	if err != nil {
		return nil, err
	}
	rows, _ := out["data"].([]usecase.Output)
	read := make([]container, 0, len(rows))
	for _, row := range rows {
		read = append(read, container{
			id: row.String("id"), name: row.String("name"), parent: row.String("parent_id"),
			kind: work.ContainerType(row.String("type")),
		})
	}
	return read, nil
}

// mayAutomate asks quietly whether the asker may write rules at a container: a draft may narrow
// its scope only to where its writer could save it.
func (s CatalogueSources) mayAutomate(
	ctx context.Context, actor appshared.ActorContext, place container,
) (bool, error) {
	if s.Permits == nil {
		return false, nil
	}
	return s.Permits.Permits(ctx, actor, access.Request{
		Permission: service.PermissionAutomation,
		Path:       place.scope().Path(),
		TokenScope: automationScope,
	})
}

// people reads who holds a role on the scope's path, resolves their names, and keeps those the
// sentence names. Someone who holds a role only through a group is not listed by ListMemberships,
// so is never shown: an assignment to them is named as not drafted rather than guessed at.
func (s CatalogueSources) people(
	ctx context.Context, actor appshared.ActorContext, scope automation.Scope,
	containers []container, matcher nameMatcher, kept integration.KeptOut,
) ([]Option, error) {
	path := []identity.Scope{identity.TenantScope()}
	switch scope.Type {
	case automation.ScopeHub:
		path = append(path, identity.HubScope(scope.ID))
	case automation.ScopeCollection:
		for _, each := range containers {
			if each.id == scope.ID.String() && each.parent != "" {
				if hub, err := shared.ParseID(each.parent); err == nil {
					path = append(path, identity.HubScope(hub))
				}
			}
		}
		path = append(path, identity.CollectionScope(scope.ID))
	}

	seen := map[string]bool{}
	var accounts []string
	for _, at := range path {
		if s.Permits != nil {
			// Only where the asker may read: a scope they hold nothing on answers not-found, and
			// the workspace a refusal with an audit entry (domain-model.md §5) - neither of which
			// a draft should cost them.
			readable, err := s.Permits.Permits(ctx, actor, access.Request{
				Permission: service.PermissionRead, Path: pathTo(at),
			})
			if err != nil {
				return nil, err
			}
			if !readable {
				continue
			}
		}
		in := usecase.Input{"scope_type": string(at.Type), "size": maxMembershipsRead}
		if !at.ID.IsZero() {
			in["scope_id"] = at.ID.String()
		}
		out, err := s.Catalogue.Invoke(ctx, "ListMemberships", actor, in)
		if err != nil {
			return nil, err
		}
		rows, _ := out["data"].([]usecase.Output)
		for _, row := range rows {
			id := row.String("account_id")
			if id == "" || seen[id] || len(accounts) >= maxMembershipsRead {
				continue
			}
			seen[id] = true
			accounts = append(accounts, id)
		}
	}
	if !seen[actor.AccountID.String()] {
		accounts = append(accounts, actor.AccountID.String())
	}

	var shown []Option
	for _, id := range accounts {
		accountID, err := shared.ParseID(id)
		if err != nil || kept.Has(accountID) {
			// Whoever keeps their content out of AI, or whose processing is restricted, is not
			// offered to a model by name (data-protection.md §4.1).
			continue
		}
		out, err := s.Catalogue.Invoke(ctx, "GetAccount", actor, usecase.Input{"account_id": id})
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				continue
			}
			return nil, err
		}
		if out.String("status") != "ACTIVE" {
			continue
		}
		name := out.String("display_name")
		own := id == actor.AccountID.String()
		if !own && !matcher.names(s.nfc(name)) {
			continue
		}
		if own {
			name += " (you)"
		}
		shown = append(shown, Option{ID: id, Name: name, Current: own})
	}
	return firstShown(sortOptions(shown)), nil
}

func pathTo(at identity.Scope) []identity.Scope {
	if at.Type == identity.ScopeTenant {
		return []identity.Scope{at}
	}
	return []identity.Scope{identity.TenantScope(), at}
}

// labels reads the vocabulary of every shown collection and keeps the labels the sentence names.
func (s CatalogueSources) labels(
	ctx context.Context, actor appshared.ActorContext, collections []container,
	matcher nameMatcher, kept integration.KeptOut,
) ([]Option, error) {
	var shown []Option
	for _, collection := range collections {
		out, err := s.Catalogue.Invoke(ctx, "ListLabels", actor, usecase.Input{"collection_id": collection.id})
		if err != nil {
			return nil, err
		}
		rows, _ := out["data"].([]usecase.Output)
		for _, row := range rows {
			if !matcher.names(s.nfc(row.String("name"))) {
				continue
			}
			shown = append(shown, Option{
				ID:   row.String("id"),
				Name: kept.Scrub(row.String("name")) + " (in " + kept.Scrub(collection.name) + ")",
			})
		}
	}
	return shown, nil
}

func sortOptions(options []Option) []Option {
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].Name != options[j].Name {
			return options[i].Name < options[j].Name
		}
		return options[i].ID < options[j].ID
	})
	return options
}

func sortScopes(options []ScopeOption) []ScopeOption {
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].Name != options[j].Name {
			return options[i].Name < options[j].Name
		}
		return options[i].Scope.ID.String() < options[j].Scope.ID.String()
	})
	return options
}

func firstShown[T any](options []T) []T {
	if len(options) > maxShown {
		return options[:maxShown]
	}
	return options
}

// nameMatcher says whether a sentence names something: the whole name, a word of it of three
// letters or more, or a word of the sentence of four letters or more that begins a word of the
// name - each found in the sentence after case folding. For a name in a script written without
// spaces (Han, Thai, kana and their neighbours), the whole name or a part a space sets apart, of
// two characters or more, since there are no words to compare.
type nameMatcher struct {
	folded string
	words  []string
}

func newNameMatcher(sentence string) nameMatcher {
	folded := strings.ToLower(sentence)
	return nameMatcher{folded: folded, words: wordsOf(folded)}
}

func (m nameMatcher) names(name string) bool {
	folded := strings.TrimSpace(strings.ToLower(name))
	if folded == "" {
		return false
	}
	if unspaced(folded) {
		// No words to compare: the name, or a part of it a space sets apart (a given name), as
		// written in the sentence.
		for _, part := range append([]string{folded}, strings.Fields(folded)...) {
			if len([]rune(part)) >= 2 && strings.Contains(m.folded, part) {
				return true
			}
		}
		return false
	}
	if strings.Contains(m.folded, folded) && len([]rune(folded)) >= 3 {
		return true
	}
	nameWords := wordsOf(folded)
	for _, word := range nameWords {
		// A substring rather than a word of the sentence: an inflection or a particle written
		// onto the name (a dative ending, an Arabic preposition) still names it.
		if len([]rune(word)) >= 3 && strings.Contains(m.folded, word) {
			return true
		}
	}
	for _, said := range m.words {
		if len([]rune(said)) < 4 {
			continue
		}
		for _, word := range nameWords {
			if strings.HasPrefix(word, said) {
				return true
			}
		}
	}
	return false
}

// wordsOf splits at everything that is not a letter, a mark or a digit - so a Devanagari vowel
// sign stays inside its word.
func wordsOf(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsDigit(r)
	})
}

var unspacedScripts = []*unicode.RangeTable{
	unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai, unicode.Lao, unicode.Khmer,
	unicode.Myanmar,
}

func unspaced(text string) bool {
	for _, r := range text {
		if unicode.IsOneOf(unspacedScripts, r) {
			return true
		}
	}
	return false
}

// changedFields are the fields an `item.updated` trigger may narrow to in a draft: the ones its
// change set names.
var changedFields = []string{
	work.FieldTitle, work.FieldNotes, work.FieldBucketID, work.FieldAssigneeID, work.FieldStartAt,
	work.FieldDueAt,
}

// draftedTriggers are the trigger kinds a draft may start on. SCHEDULE is not among them: its
// recurrence is a language of its own, and a draft names it as not drafted.
var draftedTriggers = []automation.TriggerKind{
	automation.TriggerEvent, automation.TriggerRelativeDate, automation.TriggerManual,
	automation.TriggerInboundWebhook, automation.TriggerJumbleEntry,
}

// draftedSubjects are the condition subjects a draft may use: the composer's, less the two whose
// values come from sets no draft is shown (a board's columns, a custom field's values).
var draftedSubjects = []string{
	"type", "title", "notes", "completed", "archived", "due", "label", "assignee", "parent",
	"depth", "actor", "hour",
}

// writtenRuleMaterial is the material as a model reads it. Every name in it is somebody's words
// and travels as content (ai-first.md §1.3).
func writtenRuleMaterial(sets RuleSets, scopeName string, kept integration.KeptOut) string {
	var written strings.Builder
	where := "the whole workspace"
	if scopeName != "" {
		where = kept.Scrub(scopeName)
	}
	written.WriteString("The rule applies to: " + where + "\n")
	if len(sets.Scopes) > 0 {
		written.WriteString("\nPlaces the rule may apply to instead (scope):\n")
		for _, place := range sets.Scopes {
			written.WriteString("- " + string(place.Scope.Type) + " " + place.Scope.ID.String() + " - " + place.Name + "\n")
		}
	}

	written.WriteString("\nWhat starts a run (trigger.kind): ")
	kinds := make([]string, 0, len(draftedTriggers))
	for _, kind := range draftedTriggers {
		kinds = append(kinds, string(kind))
	}
	written.WriteString(strings.Join(kinds, ", ") + "\n")
	written.WriteString("Event types for EVENT (trigger.event_type):\n")
	for _, each := range event.Types() {
		written.WriteString("- " + string(each) + "\n")
	}
	written.WriteString("Fields an item update may be narrowed to (trigger.changed_fields): " +
		strings.Join(changedFields, ", ") + "\n")
	written.WriteString("Anchors for RELATIVE_DATE (trigger.anchor): DUE_DATE, CREATED_AT; " +
		"trigger.offset is an ISO 8601 duration, negative for before.\n")

	written.WriteString("\nWhat a rule may do (actions[].kind), and the params each takes:\n")
	for _, kind := range sets.Actions {
		written.WriteString("- " + kind)
		var params []string
		for _, field := range draftableActions[kind].fieldNames() {
			params = append(params, field+" ("+draftableActions[kind].Fields[field].describe()+")")
		}
		if len(params) > 0 {
			written.WriteString(": " + strings.Join(params, ", "))
		}
		written.WriteString("\n")
	}
	written.WriteString("- WAIT: duration (an ISO 8601 duration)\n- STOP: nothing; only as the last step\n")

	written.WriteString("\nCondition subjects and their operators:\n")
	for _, subject := range draftedSubjects {
		written.WriteString("- " + subject + ": " + strings.Join(conditionOperators(subject), ", ") + "\n")
	}
	written.WriteString("Entry types: ")
	types := make([]string, 0, len(work.ItemTypes()))
	for _, each := range work.ItemTypes() {
		types = append(types, string(each))
	}
	written.WriteString(strings.Join(types, ", ") + "\n")

	for _, set := range []struct {
		label   string
		options []Option
	}{
		{"People", sets.People}, {"Collections", sets.Collections}, {"Labels", sets.Labels},
	} {
		if len(set.options) == 0 {
			continue
		}
		written.WriteString("\n" + set.label + ":\n")
		for _, option := range set.options {
			written.WriteString("- " + option.ID + " - " + option.Name + "\n")
		}
	}
	return written.String()
}

// conditionOperators answers a subject's operators with what each takes, for the material.
func conditionOperators(subject string) []string {
	var described []string
	for _, op := range automation.SentenceOperators(subject) {
		takes := automation.SentenceTakes(subject, op)
		switch takes {
		case "none":
			described = append(described, op)
		default:
			described = append(described, op+" ("+takes+")")
		}
	}
	return described
}
