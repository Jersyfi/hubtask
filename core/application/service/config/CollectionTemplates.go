// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"slices"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A template's collection part (configuration.md §6) is the collection sections of a hub's
// document with each block keyed without a handle, so one part sets up any collection: taking it,
// a collection's handle is put in front of every key and the part is planned as the collection's
// hub's document. The collection records what it took under its own identifier, beside its hub's.

// lintCollection and lintHandle stand for the collection a collection part is linted against.
const (
	lintCollection shared.ID = "00000000-0000-7000-8000-000000000001"
	lintHandle               = "collection"
)

// collectionField is the input naming a collection taking a template's collection part.
func collectionField() usecase.Field {
	return usecase.Field{Name: "collection_id", Kind: usecase.KindID, Description: "A collection, for a template's collection part; its document is its hub's."}
}

// scopeOf reads the place a template call names: with `collection_id` the collection and its hub -
// a hub_id beside it must be that hub -, otherwise the hub of `hub_id` or the workspace. The input
// then names the hub, so everything downstream plans the hub's document.
func (d Desk) scopeOf(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Input, shared.ID, shared.ID, error) {
	hub, err := hubOf(in)
	if err != nil || in.String("collection_id") == "" {
		return in, hub, "", err
	}
	collection, err := in.ID("collection_id")
	if err != nil {
		return nil, "", "", err
	}
	answer, err := d.Catalogue.Invoke(ctx, "GetContainer", actor, usecase.Input{"container_id": collection.String()})
	if err != nil {
		return nil, "", "", err
	}
	parent := shared.ID(text(answer["parent_id"]))
	if text(answer["type"]) != string(work.ContainerCollection) || parent.IsZero() {
		return nil, "", "", shared.ErrNotFound.WithDetail("config.collection_not_found")
	}
	if !hub.IsZero() && hub != parent {
		return nil, "", "", refused("config.collection_not_in_hub", "/collection_id", nil)
	}
	scoped := make(usecase.Input, len(in)+1)
	for k, v := range in {
		scoped[k] = v
	}
	scoped["hub_id"] = parent.String()
	return scoped, parent, collection, nil
}

// collectionOf is the collection an input scoped by scopeOf names, or empty.
func collectionOf(in usecase.Input) shared.ID {
	return shared.ID(in.String("collection_id"))
}

// collectionSectionNames are the sections a collection part may hold.
func collectionSectionNames() []string {
	var names []string
	for _, s := range hubSections(lintHub) {
		if s.collection {
			names = append(names, s.name)
		}
	}
	return names
}

// keyedUnder is a collection part with the collection's handle in front of every key.
func keyedUnder(part domain.Document, handle string) domain.Document {
	out := domain.Document{}
	for name, section := range part {
		keyed := domain.Section{}
		for key, block := range section {
			keyed[handle+"."+key] = block
		}
		out[name] = keyed
	}
	return out
}

// collectionPart reads a template's collection part, or nil where none is named: collection
// sections only, keys without a handle, every block one the collection's own use cases accept -
// linted as if an empty collection of an empty hub took it.
func (d Desk) collectionPart(in usecase.Input) (domain.Document, error) {
	if !in.Present("collection_document") {
		return nil, nil
	}
	part, refusals := documentOf(in["collection_document"])
	if len(refusals) > 0 {
		return nil, domain.ErrRefused(refusals)
	}
	if part == nil {
		part = domain.Document{}
	}
	allowed := collectionSectionNames()
	for name, section := range part {
		if !slices.Contains(allowed, name) {
			refusals = append(refusals, domain.Refusal("config.section_not_for_collections", "/collection_document/"+name, nil))
			continue
		}
		for key := range section {
			if !domain.ValidKey(key) {
				refusals = append(refusals, domain.Refusal("config.key_invalid", "/collection_document/"+name+"."+key, nil))
			}
		}
	}
	if len(refusals) > 0 {
		return nil, domain.ErrRefused(refusals)
	}
	lintPlaces := places{
		key: map[string]string{lintCollection.String(): lintHandle}, id: map[string]string{lintHandle: lintCollection.String()},
		hub: lintHub.String(), children: map[string]bool{lintCollection.String(): true}, order: []string{lintCollection.String()},
	}
	lint := d.planOf(current{Document: lintBase(), Hub: lintHub, Places: lintPlaces}, withoutRemovals(keyedUnder(part, lintHandle)), domain.ModeMerge)
	if len(lint.Refusals) > 0 {
		for i := range lint.Refusals {
			path := strings.TrimPrefix(lint.Refusals[i].Path, "/")
			lint.Refusals[i].Path = "/collection_document/" + strings.Replace(path, "."+lintHandle+".", ".", 1)
		}
		return nil, domain.ErrRefused(lint.Refusals)
	}
	return part, nil
}

// collectionTemplate is what a collection takes of a template: its collection part under the
// collection's handle, planned against what the collection took before. As for a hub, the
// template's workspace part comes first.
func (d Desk) collectionTemplate(ctx context.Context, chosen domain.Template, workspace []domain.TemplateUse, now current, collection shared.ID) (domain.Document, []domain.TemplateUse, string, error) {
	if !chosen.ForCollections() {
		return nil, nil, "", refused("config.template_not_for_collections", "/key", map[string]string{"template": chosen.Key})
	}
	if len(chosen.Document) > 0 && !slices.ContainsFunc(workspace, func(u domain.TemplateUse) bool { return u.TemplateKey == chosen.Key }) {
		return nil, nil, "", refused("config.template_workspace_first", "/key", map[string]string{"template": chosen.Key})
	}
	handle := now.Places.key[collection.String()]
	if handle == "" || !now.Places.children[collection.String()] {
		return nil, nil, "", shared.ErrNotFound.WithDetail("config.collection_unknown")
	}
	uses, err := d.Store.TemplateUses(ctx, collection)
	if err != nil {
		return nil, nil, "", err
	}
	return keyedUnder(chosen.Collection, handle), uses, handle, nil
}
