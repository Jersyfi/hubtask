// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Providers answers which provider a workspace uses (J-03). An interface here rather than the
// adapter, because the application layer may not import one (ADR-0001).
type Providers interface {
	For(ctx context.Context, actor appshared.ActorContext) (aiprovider.Provider, error)
}

// Material is what a suggestion is made from: the text to describe, and the fingerprint of the
// state it describes.
//
// The two travel together on purpose. A producer that read the text and computed the digest
// separately could read one state and fingerprint another, and the staleness check would then pass
// for a suggestion made from something else - which is the exact failure the digest exists to
// prevent, arrived at through the back door.
type Material struct {
	// Content is what goes to the provider, as user content and never as instruction.
	Content string
	// Digest is the fingerprint of the state Content was read from.
	Digest []byte
	// Declared are the custom fields the entry's container asks for, as an answer may fill them
	// (K-03). Empty for a workspace that declared none, which is every workspace until somebody
	// declares one.
	Declared []Declared
	// Choices are the closed sets this answer may pick from, already narrowed to what the person
	// asking may see (K-02).
	//
	// They are the whole difference between a model *choosing* and a model *naming*.
	// `collection_id` stays filtered out of every answer because a model cannot know which
	// collections a workspace has, and one that names one is choosing a destination; a board's
	// columns are small, closed, and already in front of the person asking, so handed over as the
	// options and validated on the way back an answer is a choice from what it was shown. They
	// travel as content, never as instruction - a column's name is something somebody typed.
	Choices []Choices
}

// Declared is one custom field definition, with the entry's own value beside it (K-03).
//
// §2's Classification row has said "priority" since before this repository had custom fields, and
// the item model has never had such a column. What a workspace that works with priority does is
// *declare* it - usually a SELECT with its own options, in its own words, in its own language - and
// that declaration is exactly the closed set the decision above needs. So the classifier proposes
// values for the fields the container declared, which serves the row for the workspaces that meant
// it and adds nothing to the item model for the ones that did not.
//
// The definition is the domain's own type rather than a copy of its parts, because the validation
// has to be the definition's own: a value `SetCustomField` would refuse is one this must refuse,
// and the way to be sure of that is to ask the same code (`ValidateValue`).
type Declared struct {
	Definition work.CustomFieldDefinition
	// Current is the entry's own value for the key, or nil. Nobody else's ever travels: what
	// another entry holds under the same key is that entry's content, and a provider asked to
	// classify this one has no business reading it.
	Current any
}

// Choices is one closed set, with the option the target is on now marked.
type Choices struct {
	// Key is the answer key this set belongs to - the same key the allow list names.
	Key string
	// Label is what the set is called in the material, e.g. "Board columns".
	Label   string
	Options []Option
}

// Option is one thing that may be chosen: an identifier a model copies, and a name it reads.
//
// The identifier travels because the answer has to be unambiguous - two columns may be called the
// same thing - and because it costs nothing: whoever asked can already read every one of them.
type Option struct {
	ID      string
	Name    string
	Current bool
}

// offers reports whether this set contains the identifier answered.
func (c Choices) offers(id string) bool {
	for _, option := range c.Options {
		if option.ID == id {
			return true
		}
	}
	return false
}

// Sources reads the material for one target kind, for the question about to be asked.
//
// The prompt travels because what is worth reading depends on the question: a classification is
// offered the board's columns, and a summary of the same entry is not - reading them anyway would
// spend a query and put a workspace's board in front of a provider for no reason.
type Sources interface {
	Material(ctx context.Context, actor appshared.ActorContext, targetType domain.TargetType, targetID shared.ID, promptID string) (Material, error)
}

// Produce asks a provider and records what it answered (J-06).
//
// It is not a use case and is deliberately not in the catalogue: nobody asks for a suggestion to be
// *recorded*, they ask for one to be *made* (`SuggestFromJumbleEntry`), and this is the job that
// runs afterwards. Which also means it has no actor of its own - it acts for the person who asked,
// so the read it performs and the consent it is subject to are theirs.
type Produce struct {
	Providers   Providers
	Prompts     aiprovider.Prompts
	Sources     Sources
	Suggestions repository.Suggestions
	// Catalogue is how an applied answer is accepted: through the use case, never around it.
	Catalogue Catalogue
	// Fields narrows a proposal to what the use case that would apply it declares. Nil narrows
	// nothing, which is what a build wired before J-16 did - and what it produced was a suggestion
	// nobody could accept.
	Fields     Fields
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// The prompts this build can ask with, and what a node of each answer may carry.
//
// Keyed by prompt rather than by kind, because three of `automation.md` §1.3's actions produce a
// FIELDS suggestion and each asks a different question: suggesting fields, summarising and
// classifying differ in the prompt and in which fields the answer may set, and in nothing else.
// The allow list is per prompt for that reason - a summariser that came back with labels has
// answered a question nobody asked.
var promptFields = map[string]map[string]bool{
	"suggest-fields": {
		"title": true, "notes": true, "due_date": true, "labels": true, "subtasks": true,
	},
	"summarize": {"notes": true},
	// The other two thirds of §2's Summarisation row (K-05). Same answer shape, different
	// material: a discussion rather than an entry, and a collection rather than either.
	"summarize-thread":     {"notes": true},
	"summarize-collection": {"notes": true},
	// All three chosen from what the material carried, never named freely (K-02, K-03): the
	// columns of the entry's board, the vocabulary its collection agreed on, and the values of the
	// fields that collection declared.
	//
	// `labels` was here until K-02 and could not be applied by anything: a label is a set entry
	// added by identifier through `AddLabel`, not a field of the item, so `UpdateWorkItem` - the
	// use case that applies a FIELDS proposal about an entry - declares no such input. J-16's
	// narrowing therefore dropped the key, and every classification since has recorded an empty
	// payload, which is to say nothing at all. Words a model invented could not have been applied
	// anyway: a label a workspace has not agreed on is vocabulary, and inventing vocabulary is the
	// naming this milestone's second decision keeps a model out of.
	"classify": {"label_ids": true, "bucket_id": true, "custom_fields": true},
	// A decomposition's answer is one key at its own level and a tree underneath it, and what a
	// *node* may carry is `keptTree`'s business rather than this map's.
	"decompose": {"children": true},
}

// choiceSets names, per prompt, which of its answer keys are chosen from a closed set (K-02).
//
// A key named here is refused unless the answer copies one of the options the material carried -
// so a model that invents an identifier, or names a column from a board nobody showed it, proposes
// nothing under that key while the rest of its answer stands.
var choiceSets = map[string][]string{
	"classify": {"bucket_id", "label_ids"},
}

// AnswerKeys is this map, for the gate that reads it beside the prompt store (K-01).
//
// Exported for one caller and named for what it is: `test/architecture` compares what a prompt asks
// a provider for against what the code keeps, because the two are a markdown file and a Go map and
// nothing else reads both. That is how `subtasks` came to be asked for and discarded from J-06
// until 0.7.5 - a defect no compiler can see and no review reliably catches.
func AnswerKeys() map[string][]string {
	keys := make(map[string][]string, len(promptFields))
	for prompt, fields := range promptFields {
		named := make([]string, 0, len(fields))
		for field := range fields {
			named = append(named, field)
		}
		sort.Strings(named)
		keys[prompt] = named
	}
	return keys
}

// grown are the answer keys an acceptance performs itself rather than handing to the use case that
// applies the rest of the payload.
//
// Keyed by applier, because a key that is structural for one acceptance is a field nobody declared
// for another. `subtasks` under a jumble entry is the walk in `apply`: the entry is converted, and
// each title becomes a child through `CreateWorkItem`. The same key proposed about a work item
// would be handed to `UpdateWorkItem`, which declares no such input, and the registry would refuse
// the whole acceptance - J-16's defect from the other side. Breaking a work item down is what
// KindDecomposition is for.
var grown = map[applierKey]map[string]bool{
	{domain.TargetJumbleEntry, domain.KindFields}: {"subtasks": true},
	// `UpdateWorkItem` declares `bucket_id` and would take it, which is exactly why this entry is
	// here rather than absent: putting a card in another column is a *move*, and the history entry
	// and the event a person reads should say so (K-02). The acceptance calls `MoveWorkItem`.
	{domain.TargetWorkItem, domain.KindFields}: {
		"bucket_id": true, "label_ids": true, "custom_fields": true,
	},
}

// defaultPrompts is what a kind is asked with when a job does not say.
//
// It exists for one reason: a job written by the release before this one carries no prompt, and it
// still runs after an upgrade (core/port/queue - the payload outlives the process that wrote it).
var defaultPrompts = map[domain.Kind]string{
	domain.KindFields:        "suggest-fields",
	domain.KindDecomposition: "decompose",
}

// Execute asks, and records what came back.
//
// Nothing partial is stored. A provider that answers something unparseable, or proposes nothing,
// leaves no row: an empty suggestion is one somebody would accept to no effect, and a list of them
// is an inbox of noise. The job then finishes rather than retrying - a model that answered badly
// will answer badly again, and the person asks again if they want to.
func (h Produce) Execute(
	ctx context.Context, actor appshared.ActorContext, request Request,
) error {
	promptID := request.PromptID
	if promptID == "" {
		promptID = defaultPrompts[request.Kind]
	}
	if _, known := promptFields[promptID]; !known {
		return shared.ErrInternal.WithDetail("ai.prompt_unknown").
			WithParams(map[string]string{"prompt": promptID})
	}
	prompt, err := h.Prompts.Get(promptID)
	if err != nil {
		return err
	}
	targetType, targetID, kind := request.TargetType, request.TargetID, request.Kind

	// Where the person who asked lives. A job presents no credential, so nothing has resolved it:
	// the worker builds the actor from the identity the payload carries and stops there, and an
	// actor with no zone reads a date-only due date in UTC without complaining. That is a
	// suggestion applied by a rule landing on a different day from the same suggestion accepted by
	// a person, which is not a difference anybody could explain.
	actor, err = h.located(ctx, actor)
	if err != nil {
		return err
	}

	provider, err := h.Providers.For(ctx, actor)
	if err != nil {
		return err
	}
	if !provider.Capabilities().Completion {
		// The workspace switched AI off, or withdrew consent, between the asking and the running.
		// The same refusal the asking would have given, which is what makes the two consistent.
		return aiprovider.ErrUnavailable
	}

	// The material is read inside a transaction and the provider is called outside one: an AI call
	// is somebody else's machine, and a transaction waiting on one holds a connection for as long
	// as they feel like taking (observability-reliability.md §8).
	var material Material
	if err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			read, err := h.Sources.Material(ctx, actor, targetType, targetID, promptID)
			material = read
			return err
		}); err != nil {
		return err
	}
	if strings.TrimSpace(material.Content) == "" {
		// Nothing to describe. Not an error and not a suggestion: an empty entry produces an
		// empty proposal, and recording one would be recording noise.
		return nil
	}

	// Prompt.Ask is the only place the instruction and the content are put together, and it puts
	// them in two messages with two roles. That is ai-first.md §1.3 as a shape rather than as a
	// rule somebody remembers: this code cannot merge them if it tries.
	answer, err := provider.Complete(ctx, prompt.Ask(withOptions(material)))
	if err != nil {
		return err
	}

	payload, ok := payloadFrom(kind, promptID, answer.Text, h.applicable(request), material)
	if !ok || len(payload) == 0 {
		// A model that answered something this cannot read has answered nothing useful. Finished
		// rather than retried: the next attempt asks the same question of the same model.
		return nil
	}

	proposal, err := domain.New(domain.NewInput{
		ID: h.IDs.NewID(), TenantID: actor.TenantID,
		TargetType: targetType, TargetID: targetID, Kind: kind,
		Payload: payload,
		Provenance: domain.Provenance{
			Model: answer.Model, PromptID: answer.PromptID,
			PromptVersion: answer.PromptVersion, ProducedAt: answer.ProducedAt,
		},
		InputDigest: material.Digest,
		Now:         h.Clock.Now(),
	})
	if err != nil {
		return err
	}

	if err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Suggestions.Record(ctx, proposal)
	}); err != nil {
		return err
	}
	if !request.Apply {
		return nil
	}

	// Accepted through the use case, not around it. A rule that applies an answer directly is a
	// rule whose `run_as` account makes the change, with that account's rights checked where they
	// always are - so an action cannot reach further than the person the rule runs as
	// (automation.md §2).
	_, err = h.Catalogue.Invoke(ctx, "AcceptSuggestion", actor, usecase.Input{
		"suggestion_id": proposal.ID.String(),
	})
	return err
}

// The allow list is the security half of parsing an answer, and `promptFields` is where it lives.
//
// The payload is later merged into a use case's input, so a key nobody expected would be a field a
// model chose to set. The registry would refuse an *undeclared* one (C-07) - but it would accept a
// *declared* one nobody meant to offer, and `collection_id` is exactly such a field. Filtering is
// what keeps "a model proposes text" from becoming "a model proposes a destination".

// Request is one question to a provider.
type Request struct {
	TargetType domain.TargetType
	TargetID   shared.ID
	Kind       domain.Kind
	// PromptID names the question. Empty takes the kind's default, which is what a job written by
	// the previous release carries.
	PromptID string
	// Apply is `automation.md` §1.3's "or applied directly, configured explicitly": the answer is
	// accepted the moment it arrives, as the person the rule runs as, rather than waiting for
	// somebody to read it.
	//
	// It goes through AcceptSuggestion like every other acceptance, which is what keeps the record
	// honest: the suggestion exists first with its provenance, the acceptance is audited as its
	// own act, and the change is the ordinary use case with the ordinary permission check. An
	// applied answer is therefore not a shortcut past any of it - it is the same path with nobody
	// pausing in the middle.
	Apply bool
}

// The two ordinary reads that answer where the person who asked lives.
//
// Named here for `appliers`' reason: what this package can do to a workspace is exactly what it can
// name, and a short list is what makes that reviewable.
const (
	ownAccountName    = "GetOwnAccount"
	readWorkspaceName = "ReadWorkspace"
)

// located fills in the locale and the time zone an actor arrived without.
//
// The same chain `AuthenticateToken` walks for a request - the person's own preference, then the
// workspace's default - through the ordinary reads, as the person, because that is how everything
// else this job reads is read. Nothing is stored and nothing new travels: putting a time zone in
// the job payload would put a personal preference in a table with no row level security, to save a
// read that happens once per job.
//
// An actor that already has a zone is left alone, which is every actor that came through a request.
// A workspace that answers neither leaves the zone empty, and what depends on it says so where it
// depends on it rather than guessing here.
func (h Produce) located(
	ctx context.Context, actor appshared.ActorContext,
) (appshared.ActorContext, error) {
	if actor.TimeZone != "" || h.Catalogue == nil {
		return actor, nil
	}

	account, err := h.Catalogue.Invoke(ctx, ownAccountName, actor, usecase.Input{})
	if err != nil {
		return actor, err
	}
	actor.Locale = firstWritten(actor.Locale, account.String("locale"))
	actor.TimeZone = account.String("time_zone")
	if actor.TimeZone != "" && actor.Locale != "" {
		return actor, nil
	}

	workspace, err := h.Catalogue.Invoke(ctx, readWorkspaceName, actor, usecase.Input{})
	if err != nil {
		return actor, err
	}
	actor.Locale = firstWritten(actor.Locale, workspace.String("default_locale"))
	actor.TimeZone = firstWritten(actor.TimeZone, workspace.String("default_time_zone"))
	return actor, nil
}

func firstWritten(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// applicable is the set of fields the use case that would apply this suggestion declares, or
// nothing where this build cannot say - which narrows nothing rather than everything.
//
// It reads the descriptor rather than a second list beside `appliers`, so the day somebody adds a
// field to `ConvertJumbleEntry` the suggestions may propose it, with nothing to remember.
func (h Produce) applicable(request Request) map[string]bool {
	if h.Fields == nil {
		return nil
	}
	name, served := appliers[applierKey{request.TargetType, request.Kind}]
	if !served {
		return nil
	}
	declared, known := h.Fields.InputsOf(name)
	if !known {
		return nil
	}
	fields := make(map[string]bool, len(declared))
	for _, field := range declared {
		fields[field] = true
	}
	// And what the acceptance grows itself, which the applier never sees as an input and therefore
	// never declares.
	for field := range grown[applierKey{request.TargetType, request.Kind}] {
		fields[field] = true
	}
	return fields
}

// payloadFrom reads a model's answer in the shape its kind fixes, its prompt narrows, and - for a
// field set - the use case that would apply it can actually take.
//
// That last narrowing was missing until J-16, and what it produced was a suggestion nobody could
// ever accept. `suggest-fields` proposes a title, notes, a due date and labels; a proposal about a
// jumble entry is applied by `ConvertJumbleEntry`, which declares `title` and not the other three -
// and the registry refuses an input a descriptor does not declare. So the record was produced,
// stored and listed, and every acceptance of it answered `validation_failed`. Narrowing here rather
// than dropping fields at acceptance is the honest half of the choice: a person reading a proposal
// should be reading what they could actually accept.
func payloadFrom(
	kind domain.Kind, promptID, text string, applicable map[string]bool, material Material,
) (map[string]any, bool) {
	answered, ok := objectFrom(text)
	if !ok {
		return nil, false
	}
	switch kind {
	case domain.KindFields:
		kept := keptFields(answered, Narrowed(promptFields[promptID], applicable))
		kept = keptChoices(kept, promptID, material.Choices)
		return keptTitles(keptDeclared(kept, material.Declared)), true
	case domain.KindDecomposition:
		return keptTree(answered)
	default:
		return nil, false
	}
}

// withOptions is the material as the provider sees it: what was written, and then the sets the
// answer may choose from.
//
// Rendered here rather than by whoever read them, so that the text a model is shown and the set an
// answer is checked against cannot disagree - the failure that would produce is a model choosing
// correctly from what it was shown and being refused for it.
//
// After the emptiness check in Execute, deliberately: a board is not material. An entry with no
// text of its own is nothing to describe, and offering a provider a list of columns to classify
// nothing into would spend a call on it.
func withOptions(material Material) string {
	content := material.Content
	if declared := writtenFields(material.Declared); declared != "" {
		content += declared
	}
	for _, set := range material.Choices {
		if len(set.Options) == 0 {
			continue
		}
		var written strings.Builder
		written.WriteString("\n\n" + set.Label + ":\n")
		for _, option := range set.Options {
			written.WriteString("- " + option.ID + " - " + option.Name)
			if option.Current {
				written.WriteString(" (where the entry is now)")
			}
			written.WriteString("\n")
		}
		content += written.String()
	}
	return content
}

// writtenFields is the declared fields as the provider sees them: the key, what it permits, and
// what the entry holds today.
func writtenFields(declared []Declared) string {
	if len(declared) == 0 {
		return ""
	}
	var written strings.Builder
	written.WriteString("\n\nFields this collection asks for:\n")
	for _, field := range declared {
		written.WriteString("- " + field.Definition.Key + ": " + permitted(field.Definition))
		if field.Current != nil {
			written.WriteString(" (the entry holds: " + shown(field.Current) + ")")
		}
		written.WriteString("\n")
	}
	return written.String()
}

// permitted says what one field may hold, in the words the answer has to use.
func permitted(definition work.CustomFieldDefinition) string {
	switch definition.Kind {
	case work.CustomFieldMultiSelect:
		return "any of " + strings.Join(definition.Options, ", ")
	case work.CustomFieldSelect:
		return "one of " + strings.Join(definition.Options, ", ")
	default:
		// BOOL, and nothing else reaches here: `closedKinds` is what decides which definitions
		// travel at all.
		return "true or false"
	}
}

// shown renders a stored value for the material. Values are the workspace's own words, so a list
// is joined rather than described.
func shown(value any) string {
	switch held := value.(type) {
	case string:
		return held
	case bool:
		if held {
			return "true"
		}
		return "false"
	case []any:
		written := make([]string, 0, len(held))
		for _, entry := range held {
			written = append(written, shown(entry))
		}
		return strings.Join(written, ", ")
	default:
		return ""
	}
}

// keptDeclared refuses a field the container did not declare, and a value the declaration does not
// allow (K-03).
//
// The refusal is the definition's own - `ValidateValue` is the code `SetCustomField` runs - so a
// value this keeps is one the acceptance can write, and a value it drops is one that would have
// been refused with the person's name on it. What comes back is the *normalised* value, for the
// same reason: what is stored is what the definition says it is.
//
// A key at a time, and the rest of the answer stands. A model that filled three fields and invented
// a fourth has classified the entry three times correctly.
func keptDeclared(payload map[string]any, declared []Declared) map[string]any {
	answered, held := payload[fieldsKey]
	if !held {
		return payload
	}
	delete(payload, fieldsKey)

	values, isObject := answered.(map[string]any)
	if !isObject || len(values) == 0 {
		return payload
	}
	definitions := make(map[string]work.CustomFieldDefinition, len(declared))
	for _, field := range declared {
		definitions[field.Definition.Key] = field.Definition
	}

	kept := make(map[string]any, len(values))
	for key, value := range values {
		definition, declaredHere := definitions[key]
		if !declaredHere || value == nil {
			continue
		}
		checked, err := definition.ValidateValue(value)
		if err != nil || checked == nil {
			continue
		}
		kept[key] = checked
	}
	if len(kept) == 0 {
		return payload
	}
	payload[fieldsKey] = kept
	return payload
}

// keptChoices refuses a choice that was not offered (K-02).
//
// Both halves of the same rule: an answer naming something outside the set is dropped, and so is
// one answering a key whose set was never offered at all - an entry with no board is not an entry
// whose board a model may invent. The rest of the answer stands, because a classification is
// several proposals at once and labels are not made wrong by a bucket that was.
func keptChoices(payload map[string]any, promptID string, offered []Choices) map[string]any {
	for _, key := range choiceSets[promptID] {
		chosen, held := payload[key]
		if !held {
			continue
		}
		switch answered := chosen.(type) {
		case string:
			if !chosenFrom(offered, key, answered) {
				delete(payload, key)
			}
		case []any:
			// Several choices from one set. Each is checked on its own and the ones that were
			// offered stand, because this is the same filter a key gets rather than a repair of a
			// malformed answer: what a model chose from the list it was shown is a choice,
			// whatever it wrote beside it.
			kept := make([]any, 0, len(answered))
			seen := map[string]bool{}
			for _, entry := range answered {
				id, isText := entry.(string)
				if !isText || seen[id] || !chosenFrom(offered, key, id) {
					continue
				}
				seen[id] = true
				kept = append(kept, id)
			}
			if len(kept) == 0 {
				delete(payload, key)
				continue
			}
			payload[key] = kept
		default:
			delete(payload, key)
		}
	}
	return payload
}

func chosenFrom(offered []Choices, key, id string) bool {
	for _, set := range offered {
		if set.Key == key {
			return set.offers(id)
		}
	}
	return false
}

// maxProposedSubtasks bounds the titles a field set may carry. The prompt asks for ten; this is
// what happens when a model ignores it.
const maxProposedSubtasks = 10

// keptTitles reads `subtasks` as what it is - a list of titles, in the order the work would be
// done - and drops it whole where it is anything else.
//
// Dropped rather than repaired, and dropped *alone* rather than taking the suggestion with it,
// which is where this differs from `keptTree`. A tree is the whole proposal, so a malformed one
// leaves nothing to record; a field set is several proposals at once, and losing a good title
// because a model answered the last field badly would be the wrong trade. What a person then reads
// is a suggestion without subtasks, which is also what a note describing one indivisible thing
// produces.
func keptTitles(payload map[string]any) map[string]any {
	proposed, held := payload["subtasks"]
	if !held {
		return payload
	}
	delete(payload, "subtasks")

	list, isList := proposed.([]any)
	if !isList || len(list) == 0 || len(list) > maxProposedSubtasks {
		return payload
	}
	titles := make([]any, 0, len(list))
	for _, entry := range list {
		title, isText := entry.(string)
		if !isText || strings.TrimSpace(title) == "" {
			return payload
		}
		titles = append(titles, title)
	}
	// The length a title may be is the domain's, checked where every other title is: a title too
	// long is refused by `CreateWorkItem` at acceptance, with everything before it standing.
	payload["subtasks"] = titles
	return payload
}

// maxProposedNodes bounds a tree. The prompt asks for eight; this is what happens when a model
// ignores it, and it is a bound rather than a truncation - a tree cut in half is a breakdown
// nobody proposed, where a refusal is a proposal somebody asks for again.
const maxProposedNodes = 32

// keptTree reads the tree of a DECOMPOSITION answer, node by node, keeping only what a node may
// carry.
//
// Two levels and no more, which is what the item model allows underneath a task and what the
// prompt asks for. A deeper answer is refused rather than flattened: flattening would put
// activities where a person did not propose them.
func keptTree(answered map[string]any) (map[string]any, bool) {
	children, count, ok := keptChildren(answered["children"], 0)
	if !ok || count > maxProposedNodes {
		return nil, false
	}
	if len(children) == 0 {
		// A model that found nothing to break down has answered correctly, and there is nothing
		// to record: an empty proposal is one somebody would accept to no effect.
		return nil, true
	}
	return map[string]any{"children": children}, true
}

// nodeTypes are the two an item under a task may be. `TASK` is deliberately absent: a task under a
// task is a shape the domain refuses, and proposing one would be proposing a refusal.
var nodeTypes = map[string]bool{"WORK_PACKAGE": true, "ACTIVITY": true}

func keptChildren(value any, depth int) ([]any, int, bool) {
	if value == nil {
		return nil, 0, true
	}
	list, isList := value.([]any)
	if !isList {
		return nil, 0, false
	}
	if depth > 1 {
		// Nothing sits under an activity.
		return nil, 0, false
	}

	kept := make([]any, 0, len(list))
	total := 0
	for _, entry := range list {
		node, isNode := entry.(map[string]any)
		if !isNode {
			return nil, 0, false
		}
		kind, _ := node["type"].(string)
		title, _ := node["title"].(string)
		if !nodeTypes[kind] || strings.TrimSpace(title) == "" {
			return nil, 0, false
		}

		clean := map[string]any{"type": kind, "title": title}
		if notes, held := node["notes"].(string); held && strings.TrimSpace(notes) != "" {
			clean["notes"] = notes
		}
		grandchildren, under, ok := keptChildren(node["children"], depth+1)
		if !ok {
			return nil, 0, false
		}
		if len(grandchildren) > 0 {
			clean["children"] = grandchildren
		}
		kept = append(kept, clean)
		total += 1 + under
	}
	return kept, total, true
}

// keptFields reads a model's answer as the fields it was asked for.
//
// Tolerant of the two things every model does - a fenced code block around the JSON, and prose
// before it - and intolerant of everything else. What it will not do is repair: a half-formed
// answer produces no suggestion rather than a suggestion with a guess in it.
// Narrowed intersects the prompt's allow list with what the applier declares.
//
// An applier this build does not serve, or one whose declared inputs cannot be read, narrows
// nothing rather than everything: a suggestion with no fields at all is not stored, and answering
// "the model proposed nothing" for a lookup that failed would be a lie about the model.
func Narrowed(allowed, applicable map[string]bool) map[string]bool {
	if len(applicable) == 0 {
		return allowed
	}
	both := make(map[string]bool, len(allowed))
	for field := range allowed {
		if applicable[field] {
			both[field] = true
		}
	}
	return both
}

func keptFields(answered map[string]any, allowed map[string]bool) map[string]any {
	kept := make(map[string]any, len(answered))
	for key, value := range answered {
		if !allowed[key] {
			continue
		}
		// An empty value proposes nothing and would only clutter the shape a person reads.
		if text, isText := value.(string); isText && strings.TrimSpace(text) == "" {
			continue
		}
		kept[key] = value
	}
	return kept
}

// objectFrom finds the JSON object in a model's answer.
//
// Tolerant of the two things every model does - a fenced code block around the JSON, and prose
// before it - and intolerant of everything else. What it will not do is repair: a half-formed
// answer produces no suggestion rather than a suggestion with a guess in it.
func objectFrom(text string) (map[string]any, bool) {
	trimmed := strings.TrimSpace(text)
	if fenced := strings.Index(trimmed, "```"); fenced >= 0 {
		rest := trimmed[fenced+3:]
		if line := strings.IndexByte(rest, '\n'); line >= 0 {
			rest = rest[line+1:]
		}
		if end := strings.Index(rest, "```"); end >= 0 {
			rest = rest[:end]
		}
		trimmed = strings.TrimSpace(rest)
	}
	start, end := strings.IndexByte(trimmed, '{'), strings.LastIndexByte(trimmed, '}')
	if start < 0 || end <= start {
		return nil, false
	}

	var answered map[string]any
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &answered); err != nil {
		return nil, false
	}
	return answered, true
}

// IsUnavailable reports whether an error is the AI port's one refusal, so a caller can tell "the
// provider is out of reach" from "something went wrong" without importing the port's sentinel.
func IsUnavailable(err error) bool {
	return errors.Is(err, shared.ErrUnavailable) &&
		shared.AsError(err).DetailCode == "ai.unavailable"
}
