// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// MakeEntriesName is the catalogue name.
const MakeEntriesName = "MakeEntries"

// EntriesAskedAction records that somebody asked for a piece of text to be turned into entries:
// that something was sent to a provider and for which collection (ADR-0018 decision 7), never the
// words.
const EntriesAskedAction audit.Action = "ai.entries_asked"

// makeEntriesPrompt turns a piece of text into entries for one collection (UC-AI-09).
const makeEntriesPrompt = "make-entries"

// MaxEntriesText bounds what may be pasted: a recipe, an e-mail, a page of notes - not a document.
const MaxEntriesText = 8000

// maxProposedEntries bounds a proposal. The prompt asks for fifty at most; the entries a model
// answers beyond are dropped and counted, because a list is many proposals at once and the first
// fifty stand on their own - unlike a tree, which `keptTree` refuses whole.
const maxProposedEntries = 50

// maxEntryTitle bounds a proposed title at the length the domain allows one: a longer title would
// be refused by CreateWorkItem at acceptance, so it is dropped here rather than offered.
const maxEntryTitle = 500

// maxEntryAmount is the domain's bound on an amount, for the same reason.
const maxEntryAmount = 40

// The answer keys an entry may carry beside its title, and the capability each needs at the
// collection's level. Only these, and only where the collection carries the capability.
const (
	entriesKey = "entries"
	amountKey  = "amount"
	takeKey    = "key"
)

var entryFields = map[string]work.Capability{
	notesKey:  work.CapabilityNotes,
	amountKey: work.CapabilityAmount,
	dueKey:    work.CapabilityDueDate,
}

// entryFieldOrder is the order the material names them in, so that two readings of one collection
// send the same words.
var entryFieldOrder = []string{notesKey, amountKey, dueKey}

// EntriesShape is what an entry made for one collection may be: the collection, the level its
// entries are created at, and the fields that level carries there.
type EntriesShape struct {
	CollectionID shared.ID
	Type         work.ItemType
	Fields       map[string]bool
}

// MakeEntries asks the workspace's provider which entries a piece of pasted text holds, for one
// collection (UC-AI-09).
//
// It asks what creating entries there asks, along the collection's own path, because what it
// proposes is entries there. What comes back is an ENTRIES suggestion about the asker's own
// account - the text is theirs, as a rule's sentence is - which only they read, take in whole or
// in part, or dismiss.
type MakeEntries struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// Execute checks, holds the words, then queues.
func (h MakeEntries) Execute(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, text string,
) (shared.ID, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > MaxEntriesText {
		return shared.ID(""), shared.ErrValidation.WithDetail("suggestions.text_invalid").
			WithFields(shared.FieldError{Path: "/text", Code: "suggestions.text_invalid"})
	}
	return Ask(h).queue(ctx, actor, asking{
		TargetType: domain.TargetAccount, TargetID: actor.AccountID,
		Action: EntriesAskedAction, Kind: domain.KindEntries, PromptID: makeEntriesPrompt,
		Text: text,
		// The collection travels as the scope's reference, the way a rule's place does: a kind
		// and a container, and nothing of what it holds.
		Scope: automation.Scope{Type: automation.ScopeCollection, ID: collectionID},
		Access: &access.Request{
			Permission: service.PermissionWriteItems,
			Path:       collectionPath(collectionID),
			TokenScope: suggestionsWrite,
			TargetType: suggestionTarget,
			TargetID:   collectionID,
		},
		JobKind: queue.KindAiMakeEntries, Attempts: queue.AiMakeEntriesAttempts,
	})
}

// Descriptor is the catalogue entry.
func (h MakeEntries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       MakeEntriesName,
		Permission: service.PermissionWriteItems,
		Summary: "Asks the workspace's AI provider which entries a piece of pasted text holds - a " +
			"recipe, an e-mail, meeting notes - for one collection. Each proposed entry carries a " +
			"title and only the fields the collection carries at the level its entries are " +
			"created at: notes, an amount, a due date. Nothing of the collection's entries and " +
			"nobody else's content is sent. It is an ENTRIES suggestion about the caller's own " +
			"account, which only they read; accepting it creates every entry, or those named in " +
			"take, with CreateWorkItem as the caller.",
		SideEffects: "Holds the text for the job that will read it, queues one question to the " +
			"provider and writes an audit entry. Creates no entry until somebody takes one.",
		TokenScope: suggestionsWrite,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true,
				Description: "The collection the entries would be created in."},
			{Name: "text", Kind: usecase.KindString, Required: true,
				Description: "What was pasted, in the caller's own words, at most 8000 characters. " +
					"It travels to the provider as content, never as instruction."},
		},
		Audit: usecase.AuditDeclaration{
			Action: EntriesAskedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Asking changes nothing; the entries a taking creates write their own history.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h MakeEntries) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	collectionID, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}
	jobID, err := h.Execute(ctx, actor, collectionID, in.String("text"))
	if err != nil {
		return nil, err
	}
	return queued(jobID), nil
}

// entriesMaterial is what entries are made against: the fields an entry of the collection carries,
// and nothing else of the workspace. The text itself is the request's, read by the producer beside
// this, scrubbed of the people who keep their content out, and appended as content.
//
// Not the collection's name and not its entries: what the entries are to be is in the text, and
// whether one already stands is a read the person makes against what they may see, title by title
// (FindDuplicatesByTitle) - so no entry of anybody else's travels for it (UC-AI-09 check 4).
func (s CatalogueSources) entriesMaterial(
	ctx context.Context, actor appshared.ActorContext, question Question,
) (Material, error) {
	if question.TargetID != actor.AccountID {
		// A proposal is the asker's own; a question naming somebody else is not one this answers.
		return Material{}, shared.ErrNotFound.WithDetail("suggestions.not_found")
	}
	if question.Scope.Type != automation.ScopeCollection || question.Scope.ID.IsZero() {
		return Material{}, entriesNeedACollection
	}
	out, err := s.Catalogue.Invoke(ctx, "GetContainer", actor, usecase.Input{
		"container_id": question.Scope.ID.String(),
	})
	if err != nil {
		return Material{}, err
	}
	if out.String("type") != string(work.ContainerCollection) {
		return Material{}, entriesNeedACollection
	}

	shape := shapeFrom(out)
	entries := &EntriesShape{
		CollectionID: question.Scope.ID, Type: topLevel(out), Fields: map[string]bool{},
	}
	var profile *work.CapabilityProfile
	if s.Profiles != nil {
		hierarchy, err := s.hierarchy(ctx)
		if err != nil {
			return Material{}, err
		}
		if found, err := hierarchy.Profile(entries.Type); err == nil {
			profile = &found
		}
	}
	carried := []string{"title"}
	for _, key := range entryFieldOrder {
		capability := entryFields[key]
		if !shape.Carries(entries.Type, capability) || profile != nil && !profile.Allows(capability) {
			continue
		}
		entries.Fields[key] = true
		carried = append(carried, key)
	}

	return Material{
		Content: "Fields an entry of this list carries: " + strings.Join(carried, ", ") + ".",
		// The asker's own identifier, as for a rule draft: a proposal of entries has no state of
		// the workspace it was made from, and each create checks everything again.
		Digest:  domain.Digest(actor.AccountID.String(), ""),
		Entries: entries,
	}, nil
}

// entriesNeedACollection refuses entries for anything but a collection: a hub holds collections,
// not entries.
var entriesNeedACollection = shared.ErrValidation.WithDetail("suggestions.collection_required").
	WithFields(shared.FieldError{Path: "/collection_id", Code: "suggestions.collection_required"})

// topLevel is the level a collection's entries are created at: the first its structure offers,
// or a task where it decides none - Hubtask's default, every level, entered at the top.
func topLevel(container usecase.Output) work.ItemType {
	structure, _ := container["structure"].(map[string]any)
	settings, _ := structure["settings"].(map[string]any)
	levels, _ := settings["levels"].([]any)
	if len(levels) > 0 {
		if first, held := levels[0].(map[string]any); held {
			if itemType, isText := first["type"].(string); isText && itemType != "" {
				return work.ItemType(itemType)
			}
		}
	}
	return work.ItemTask
}

// entriesFrom reads an ENTRIES answer: a list of entries, each with a title and only the fields the
// collection carries, in the shape the acceptance creates them in. An entry without a usable title
// is dropped alone and counted; a field the collection does not carry, or one in the wrong shape,
// is dropped from its entry. Every entry gets a key, its place in the proposal, which is what a
// person names when taking some of them.
//
// Nothing the model answers names the collection or the level: both are the material's, written
// here, so an answer cannot move its entries anywhere the asker did not ask for.
func entriesFrom(text string, shape *EntriesShape) (map[string]any, int, bool) {
	if shape == nil {
		return nil, 0, false
	}
	answered, ok := objectFrom(text)
	if !ok {
		return nil, 0, false
	}
	list, isList := answered[entriesKey].([]any)
	if !isList {
		return nil, 0, false
	}
	kept := make([]any, 0, min(len(list), maxProposedEntries))
	dropped := 0
	for _, value := range list {
		entry, ok := proposedEntry(value, shape)
		if !ok || len(kept) == maxProposedEntries {
			dropped++
			continue
		}
		entry[takeKey] = strconv.Itoa(len(kept) + 1)
		kept = append(kept, entry)
	}
	if len(kept) == 0 {
		return nil, dropped, true
	}
	return map[string]any{
		collectionKey: shape.CollectionID.String(),
		"type":        string(shape.Type),
		entriesKey:    kept,
	}, dropped, true
}

// proposedEntry is one answered entry narrowed to what the collection carries.
func proposedEntry(value any, shape *EntriesShape) (map[string]any, bool) {
	answered, isEntry := value.(map[string]any)
	if !isEntry {
		return nil, false
	}
	title, _ := answered["title"].(string)
	title = strings.Join(strings.Fields(title), " ")
	if title == "" || utf8.RuneCountInString(title) > maxEntryTitle {
		return nil, false
	}
	entry := map[string]any{"title": title}
	if notes, isText := answered[notesKey].(string); isText && shape.Fields[notesKey] &&
		strings.TrimSpace(notes) != "" {
		entry[notesKey] = strings.TrimSpace(notes)
	}
	if amount, isText := answered[amountKey].(string); isText && shape.Fields[amountKey] {
		amount = strings.TrimSpace(amount)
		if amount != "" && !strings.ContainsAny(amount, "\r\n") &&
			utf8.RuneCountInString(amount) <= maxEntryAmount {
			entry[amountKey] = amount
		}
	}
	if due, isText := answered[dueKey].(string); isText && shape.Fields[dueKey] {
		if _, err := time.Parse(dateLayout, strings.TrimSpace(due)); err == nil {
			entry[dueKey] = strings.TrimSpace(due)
		}
	}
	return entry, true
}

// make creates the entries of an ENTRIES proposal that are taken - all, or those `take` names - in
// the proposal's collection, at its level, in its order, as the accepting person. What is left is
// handed back for the proposal to be narrowed to; nothing left means the proposal is accepted.
//
// **All or nothing, unlike a breakdown.** A breakdown keeps what it created before a refusal,
// because its pieces hang under one entry and the rest is the next read of its children; entries
// side by side have no such place, and a person who took three and got two would have to find out
// which. The acceptance is one transaction, so a refusal of any entry is the answer and none is
// created.
func (c Cases) make(
	ctx context.Context, actor appshared.ActorContext, name string, proposal domain.Suggestion,
	overrides map[string]any, taking *taking,
) error {
	entries, _ := proposal.Payload[entriesKey].([]any)
	collection, _ := proposal.Payload[collectionKey].(string)
	itemType, _ := proposal.Payload["type"].(string)
	if len(entries) == 0 || collection == "" || itemType == "" {
		return shared.ErrInternal.WithDetail("suggestions.payload_malformed")
	}

	var wanted map[string]bool
	if taking != nil && taking.Take != nil {
		wanted = make(map[string]bool, len(taking.Take))
		for _, key := range taking.Take {
			wanted[key] = true
		}
		if len(wanted) == 0 {
			// Naming no entry is not taking all of them: absent is all, empty is a mistake.
			return shared.ErrValidation.WithDetail("suggestions.entry_unknown").
				WithFields(shared.FieldError{Path: "/take", Code: "suggestions.entry_unknown"})
		}
	}
	held := make(map[string]bool, len(entries))
	for _, value := range entries {
		entry, isEntry := value.(map[string]any)
		if !isEntry {
			return shared.ErrInternal.WithDetail("suggestions.payload_malformed")
		}
		key, _ := entry[takeKey].(string)
		held[key] = true
	}
	for key := range wanted {
		if !held[key] {
			// Before anything is created: an entry somebody took a moment ago, or never there.
			return shared.ErrValidation.WithDetail("suggestions.entry_unknown").
				WithFields(shared.FieldError{Path: "/take", Code: "suggestions.entry_unknown"})
		}
	}

	left := make([]any, 0, len(entries))
	var created []shared.ID
	for _, value := range entries {
		entry, _ := value.(map[string]any)
		if key, _ := entry[takeKey].(string); wanted != nil && !wanted[key] {
			left = append(left, entry)
			continue
		}
		in, err := c.entryInput(actor, entry)
		if err != nil {
			return err
		}
		// What the person changed before taking applies to every entry taken, as to every piece
		// of a breakdown; the collection and the level are the proposal's, written after both.
		for field, override := range overrides {
			in[field] = override
		}
		in[collectionKey] = collection
		in["type"] = itemType
		delete(in, "parent_id")

		out, err := c.Catalogue.Invoke(ctx, name, actor, in)
		if err != nil {
			return err
		}
		id, err := shared.ParseID(out.String("id"))
		if err != nil {
			return shared.Internalf("reading a created entry's identifier: %w", err)
		}
		created = append(created, id)
	}

	if taking != nil {
		taking.CreatedIDs = created
		if len(left) > 0 {
			remaining := make(map[string]any, len(proposal.Payload))
			for field, value := range proposal.Payload {
				remaining[field] = value
			}
			remaining[entriesKey] = left
			taking.Left = remaining
		}
	}
	return nil
}

// entryInput is one proposed entry as CreateWorkItem's input: its title and fields, and a date the
// way a person's own date-only due date is written - the day, in the accepting person's zone.
func (c Cases) entryInput(actor appshared.ActorContext, entry map[string]any) (usecase.Input, error) {
	in := usecase.Input{"title": entry["title"]}
	for _, key := range []string{notesKey, amountKey} {
		if value, held := entry[key]; held {
			in[key] = value
		}
	}
	written, held := entry[dueKey].(string)
	if !held {
		return in, nil
	}
	zone := time.UTC
	if actor.TimeZone != "" {
		if loaded, err := time.LoadLocation(actor.TimeZone); err == nil {
			zone = loaded
		}
	}
	day, err := time.ParseInLocation(dateLayout, written, zone)
	if err != nil {
		return nil, shared.ErrValidation.WithDetail("suggestions.due_date_unreadable")
	}
	in["due_at"] = day.Format(time.RFC3339)
	in["due_date_only"] = true
	in["due_time_zone"] = zone.String()
	return in, nil
}
