// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"regexp"
	"slices"
	"strconv"

	configrepo "github.com/Jersyfi/hubtask/core/application/repository/config"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListSampleEntriesName   = "ListSampleEntries"
	AddSampleEntryName      = "AddSampleEntry"
	RemoveSampleEntriesName = "RemoveSampleEntries"

	SampleAddedAction    audit.Action = "config.sample_added"
	SamplesRemovedAction audit.Action = "config.samples_removed"

	// A sample is an entry: creating and trashing it is what items:write admits.
	scopeItemsWrite = "items:write"
)

var sampleKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// ListSampleEntries answers a hub's sample entries (configuration.md §6).
type ListSampleEntries struct{ Desk Desk }

func (h ListSampleEntries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListSampleEntriesName,
		Permission: service.PermissionStructure,
		Summary: "Lists the entries a template brought to a hub to try its way of working on, marked " +
			"as samples and not in the trash, by their key: the hub document's samples section.",
		SideEffects: "None.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."}},
		Audit:       usecase.AuditDeclaration{},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			var samples []configrepo.Sample
			err = h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				samples, err = h.Desk.Samples.List(ctx, hub)
				return err
			})
			if err != nil {
				return nil, err
			}
			return samplesOutput(samples), nil
		}),
	}
}

func samplesOutput(samples []configrepo.Sample) usecase.Output {
	rows := make([]any, 0, len(samples))
	for _, s := range samples {
		var entryType any
		if s.EntryType != "" {
			entryType = s.EntryType
		}
		rows = append(rows, map[string]any{
			"key": s.Key, "item_id": s.ItemID.String(), "title": s.Title, "entry_type": entryType,
			"collection_id": s.CollectionID.String(),
		})
	}
	return usecase.Output{"samples": rows}
}

// AddSampleEntry creates an entry in one of the hub's collections, marked as a sample.
type AddSampleEntry struct{ Desk Desk }

func (h AddSampleEntry) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       AddSampleEntryName,
		Permission: service.PermissionStructure,
		Summary: "Creates a task in one of the hub's collections and marks it as the hub's sample " +
			"under its key, as a template's sample entries are taken. A key the hub marks already is " +
			"refused, and so is a collection of another hub. The entry is an ordinary entry: " +
			"creating it asks what creating any entry asks.",
		SideEffects: "Creates the entry with everything creating an entry writes, marks it, and " +
			"writes an audit entry.",
		TokenScope: scopeItemsWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The sample's key in the hub, a lower-case word."},
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "A collection of the hub."},
			{Name: "title", Kind: usecase.KindString, Required: true, Description: "The entry's title."},
			{Name: "entry_type", Kind: usecase.KindString, Description: "The key of a type of the task level; omitted is the level's own."},
		},
		Audit:    usecase.AuditDeclaration{Action: SampleAddedAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "the entry's creation is its own activity."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, SampleAddedAction, scopeItemsWrite, false); err != nil {
				return nil, err
			}
			return h.Desk.addSample(ctx, actor, hub, in)
		}),
	}
}

func (d Desk) addSample(ctx context.Context, actor appshared.ActorContext, hub shared.ID, in usecase.Input) (usecase.Output, error) {
	key := in.String("key")
	if !sampleKeyPattern.MatchString(key) {
		return nil, refused("config.key_invalid", "/key", nil)
	}
	collection, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}
	var out usecase.Output
	err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		place, err := d.preview(ctx, "GetContainer", actor, usecase.Input{"container_id": collection.String()})
		if err != nil || text(place["type"]) != "COLLECTION" || text(place["parent_id"]) != hub.String() {
			return refused("config.collection_unknown", "/collection_id", nil)
		}
		create := usecase.Input{"type": "TASK", "collection_id": collection.String(), "title": in.String("title")}
		if entryType := in.String("entry_type"); entryType != "" {
			create["entry_type"] = entryType
		}
		item, err := d.Catalogue.Invoke(ctx, "CreateWorkItem", actor, create)
		if err != nil {
			return err
		}
		id, err := shared.ParseID(text(item["id"]))
		if err != nil {
			return err
		}
		now := d.Clock.Now()
		marked, err := d.Samples.Mark(ctx, hub, id, key, now)
		if err != nil {
			return err
		}
		if !marked {
			// The whole write rolls back: no unmarked entry is left behind a refusal.
			return refused("config.sample_taken", "/key", map[string]string{"key": key})
		}
		var entryType any
		if t := text(item["entry_type"]); t != "" {
			entryType = t
		}
		out = usecase.Output{
			"key": key, "item_id": id.String(), "title": text(item["title"]), "entry_type": entryType,
			"collection_id": collection.String(),
		}
		return d.trail(ctx, actor, SampleAddedAction, now, audit.Change{Field: "hub_id", Classification: audit.Open, To: hub.String()})
	})
	return out, err
}

// RemoveSampleEntries moves every sample entry of a hub into the trash at once.
type RemoveSampleEntries struct{ Desk Desk }

func (h RemoveSampleEntries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       RemoveSampleEntriesName,
		Permission: service.PermissionStructure,
		Summary: "Moves every sample entry of the hub into the trash in one act, each as trashing it " +
			"alone would: restorable until the trash period ends. An entry nobody marked as a sample " +
			"is never touched. Answers how many moved.",
		SideEffects: "Trashes each sample entry with everything trashing an entry writes, and writes " +
			"an audit entry with the count.",
		TokenScope:  scopeItemsWrite,
		Destructive: true,
		Input:       []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."}},
		Audit:       usecase.AuditDeclaration{Action: SamplesRemovedAction, TargetType: target, Severity: audit.SeverityNotice, Required: true},
		Activity:    usecase.ActivityDeclaration{Exempt: "each entry's trashing is its own activity."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Desk.authorizeAt(ctx, actor, hub, SamplesRemovedAction, scopeItemsWrite, false); err != nil {
				return nil, err
			}
			removed := 0
			err = h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				samples, err := h.Desk.Samples.List(ctx, hub)
				if err != nil {
					return err
				}
				for _, s := range samples {
					if _, err := h.Desk.Catalogue.Invoke(ctx, "TrashWorkItem", actor, usecase.Input{"item_id": s.ItemID.String()}); err != nil {
						return err
					}
					removed++
				}
				return h.Desk.trail(ctx, actor, SamplesRemovedAction, h.Desk.Clock.Now(),
					audit.Change{Field: "hub_id", Classification: audit.Open, To: hub.String()},
					audit.Change{Field: "removed", Classification: audit.Open, To: strconv.Itoa(removed)})
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"removed": removed}, nil
		}),
	}
}

// sectionSamples is a hub's sample entries in its document: the one kind of content a document
// carries, marked (configuration.md §1).
const sectionSamples = "samples"

// hubSamplesSection reads the hub's sample entries and adds or trashes them. A block names its
// collection by handle; without one it lands in the hub's first collection. Its words are fixed:
// once added, a sample is the hub's entry to change like any other.
func hubSamplesSection(hub shared.ID) section {
	id := hub.String()
	return section{
		name: sectionSamples, list: ListSampleEntriesName, listInput: usecase.Input{"hub_id": id},
		rows: "samples", key: "key", create: AddSampleEntryName,
		attributes: []string{"title", "entry_type", "collection"},
		fixed:      []string{"title", "entry_type", "collection"},
		block: func(row map[string]any) (string, domain.Block) {
			block := domain.Block{"title": row["title"], "collection": row["collection_id"]}
			if t := text(row["entry_type"]); t != "" {
				block["entry_type"] = t
			}
			return text(row["key"]), block
		},
		present: func(p places, block domain.Block) domain.Block {
			block["collection"] = p.out(block["collection"])
			return block
		},
		opsAt: func(now current, change domain.BlockChange, row map[string]any) ([]operation, error) {
			path := change.Path()
			if change.Op == domain.OpRemove {
				return []operation{{UseCase: "TrashWorkItem", Path: path, Input: usecase.Input{"item_id": row["item_id"]}}}, nil
			}
			collection := text(now.Places.in(change.After["collection"]))
			if change.After["collection"] == nil && len(now.Places.order) > 0 {
				collection = now.Places.order[0]
			}
			if collection == "" || !now.Places.children[collection] {
				return nil, refused("config.collection_unknown", path+".collection", nil)
			}
			in := usecase.Input{"hub_id": id, "key": change.Key, "collection_id": collection, "title": change.After["title"]}
			if t := text(change.After["entry_type"]); t != "" {
				in["entry_type"] = t
			}
			return []operation{{UseCase: AddSampleEntryName, Path: path, Input: in}}, nil
		},
		hides: removing,
	}
}

// withoutSamples is a document without its sample entries, the original untouched.
func withoutSamples(d domain.Document) domain.Document {
	if _, held := d[sectionSamples]; !held {
		return d
	}
	out := make(domain.Document, len(d))
	for name, s := range d {
		if name != sectionSamples {
			out[name] = s
		}
	}
	return out
}

// sampleRefusals checks a template's sample blocks as far as no hub is needed: a key, a title
// on one line, and only the attributes a sample has.
func sampleRefusals(samples domain.Section) []shared.FieldError {
	var out []shared.FieldError
	for key, block := range samples {
		path := sectionSamples + "." + key
		if !sampleKeyPattern.MatchString(key) {
			out = append(out, domain.Refusal("config.key_invalid", path, nil))
		}
		if block == nil {
			continue
		}
		if title := text(block["title"]); title == "" || len(title) > 500 || !domain.OneLine(title) {
			out = append(out, domain.Refusal("config.sample_title_invalid", path+".title", nil))
		}
		for attribute := range block {
			if !slices.Contains([]string{"title", "entry_type", "collection"}, attribute) {
				out = append(out, domain.Refusal("config.attribute_unknown", path+"."+attribute, nil))
			}
		}
	}
	return out
}
