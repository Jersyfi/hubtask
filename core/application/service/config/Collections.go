// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A hub's document holds its collections' own configuration too (configuration.md §1): what a
// collection decides itself - its structure settings, its time tolerance, its shared views (a
// queue is one) and its rules - in sections whose block key is the collection's handle, a dot and
// the block's own key (`collection_views.network.open`). A handle names a collection of the
// document's hub; any other is refused, never guessed. The hub's own tolerance is a section beside
// them.
const (
	sectionTolerance           = "tolerance"
	sectionCollectionStructure = "collection_structure"
	sectionCollectionTolerance = "collection_tolerance"
	sectionCollectionViews     = "collection_views"
	sectionCollectionRules     = "collection_rules"
	sectionCollectionFields    = "collection_fields"
	// toleranceKey is the tolerance's one block, under the catalogue's name.
	toleranceKey   = "time_tolerance_days"
	sourceLevel    = "COLLECTION"
	readTolerance  = "ReadPlanTolerance"
	writeTolerance = "SetPlanTolerance"
)

// toleranceAttributes are a tolerance block's: its days and whether it is locked for below.
var toleranceAttributes = []string{settingValueField, lockAttribute}

// hubToleranceSection is the hub's own tolerance: a block where the hub decides it, none where it
// is left to the workspace.
func hubToleranceSection(hub shared.ID) section {
	id := hub.String()
	return section{
		name: sectionTolerance, module: module.Planning, list: readTolerance,
		listInput: usecase.Input{"plan_id": id, "tolerance_only": true}, key: "setting", attributes: toleranceAttributes,
		rowsOf: func(answer map[string]any) []any { return toleranceRowsAt(answer, sourceHub) },
		block:  toleranceBlock,
		ops: func(change domain.BlockChange, _ map[string]any) ([]operation, error) {
			if change.Key != toleranceKey {
				return nil, refused("config.key_unknown", change.Path(), nil)
			}
			return []operation{toleranceWrite(change, id)}, nil
		},
	}
}

// collectionSections are a hub document's sections of its collections' own blocks.
func collectionSections() []section {
	ruleAttributes := []string{"name", "enabled", "run_as", "trigger", "conditions", "actions", "throttle", "on_error"}
	return []section{
		{
			name: sectionCollectionStructure, list: "GetContainer", collection: true,
			listFor: func(c string) usecase.Input { return usecase.Input{"container_id": c} },
			rowsFor: func(answer map[string]any, _ string) []any { return hubStructureRows(answer) },
			key:     "setting", attributes: []string{settingValueField},
			create: "SetContainerStructure", change: "SetContainerStructure", remove: "SetContainerStructure",
			changeable: []string{settingValueField},
			block: func(row map[string]any) (string, domain.Block) {
				if text(row["source"]) != sourceLevel {
					return "", nil
				}
				return text(row["setting"]), domain.Block{settingValueField: row[settingValueField]}
			},
			opsAt: func(now current, change domain.BlockChange, _ map[string]any) ([]operation, error) {
				collection, inner := now.Places.collection(change.Key)
				if _, err := work.ParseStructureSetting(inner); err != nil {
					return nil, refused("config.key_unknown", change.Path(), nil)
				}
				in := usecase.Input{"container_id": collection, "reset": []any{inner}}
				if change.Op != domain.OpRemove {
					in = usecase.Input{"container_id": collection, "settings": map[string]any{inner: change.After[settingValueField]}}
				}
				return []operation{{UseCase: "SetContainerStructure", Path: change.Path(), Input: in}}, nil
			},
			hides: removing,
		},
		{
			name: sectionCollectionTolerance, module: module.Planning, list: readTolerance, collection: true,
			listFor: func(c string) usecase.Input { return usecase.Input{"plan_id": c, "tolerance_only": true} },
			rowsFor: func(answer map[string]any, _ string) []any { return toleranceRowsAt(answer, sourceLevel) },
			key:     "setting", attributes: toleranceAttributes,
			create: writeTolerance, change: writeTolerance, remove: writeTolerance, changeable: toleranceAttributes,
			block: toleranceBlock,
			opsAt: func(now current, change domain.BlockChange, _ map[string]any) ([]operation, error) {
				collection, inner := now.Places.collection(change.Key)
				if inner != toleranceKey {
					return nil, refused("config.key_unknown", change.Path(), nil)
				}
				return []operation{toleranceWrite(change, collection)}, nil
			},
		},
		{
			name: sectionCollectionViews, list: "ListSavedViews", collection: true,
			listFor: func(c string) usecase.Input { return usecase.Input{"container_id": c} },
			rowsFor: func(answer map[string]any, c string) []any {
				return rowsWhere(answer, func(row map[string]any) bool {
					return text(row["sharing"]) == "SCOPE" && text(row["scope_type"]) == sourceLevel && text(row["scope_id"]) == c
				})
			},
			key: "key", attributes: viewAttributes, changeable: viewAttributes,
			create: "CreateSavedView", change: "UpdateSavedView", remove: "DeleteSavedView", locateFrom: "id",
			present: func(p places, block domain.Block) domain.Block {
				block["query"] = mapQuery(block["query"], p.out)
				return block
			},
			opsAt: func(now current, change domain.BlockChange, row map[string]any) ([]operation, error) {
				collection, inner := now.Places.collection(change.Key)
				return viewOps(now, change, row, inner, sourceLevel, collection)
			},
			hides: removing,
		},
		{
			// One listing serves every collection: the rules are the workspace's, by their scope.
			name: sectionCollectionRules, list: "ListRules", listInput: usecase.Input{"size": rulePageSize}, paged: true, collection: true,
			rowsFor: func(answer map[string]any, c string) []any {
				return rowsWhere(answer, func(row map[string]any) bool {
					scope, _ := row["scope"].(map[string]any)
					return text(scope["type"]) == sourceLevel && text(scope["id"]) == c
				})
			},
			key: "key", attributes: ruleAttributes, changeable: ruleAttributes,
			create: "CreateRule", change: "UpdateRule", remove: "DeleteRule", locateFrom: "id",
			present: func(p places, block domain.Block) domain.Block {
				block["actions"] = mapActions(block["actions"], p.out)
				block["conditions"] = mapConditions(block["conditions"], p.literalOut)
				return block
			},
			opsAt: func(now current, change domain.BlockChange, row map[string]any) ([]operation, error) {
				collection, inner := now.Places.collection(change.Key)
				return ruleOps(now, change, row, inner, sourceLevel, collection)
			},
			hides: removing, permissions: true,
		},
		collectionFieldsSection(),
	}
}

// collectionFieldsSection is a collection's own custom fields: the workspace's fields section
// read at the collection and written with its identifier, the block's own key the field's.
func collectionFieldsSection() section {
	fields := workspaceFieldsSection()
	own := fields
	own.name, own.collection = sectionCollectionFields, true
	own.listFor = func(c string) usecase.Input { return usecase.Input{"collection_id": c} }
	own.rowsFor = func(answer map[string]any, c string) []any {
		return rowsWhere(answer, func(row map[string]any) bool { return text(row["collection_id"]) == c })
	}
	own.opsAt = func(now current, change domain.BlockChange, row map[string]any) ([]operation, error) {
		collection, inner := now.Places.collection(change.Key)
		at := change
		at.Key = inner
		ops, err := fields.blockOperations(at, row)
		for i := range ops {
			ops[i].Path = change.Path()
			if ops[i].UseCase == fields.create {
				ops[i].Input["collection_id"] = collection
			}
		}
		return ops, err
	}
	return own
}

func rowsWhere(answer map[string]any, keep func(map[string]any) bool) []any {
	rows, _ := answer["data"].([]any)
	var out []any
	for _, raw := range rows {
		if row, _ := raw.(map[string]any); row != nil && keep(row) {
			out = append(out, row)
		}
	}
	return out
}

// toleranceRowsAt is a tolerance read as its one row where level decides it, none otherwise.
func toleranceRowsAt(answer map[string]any, level string) []any {
	if text(answer["source"]) != level {
		return nil
	}
	return []any{map[string]any{
		"setting": toleranceKey, settingValueField: answer["time_tolerance_days"],
		lockAttribute: text(answer["locked_by"]) == level,
	}}
}

func toleranceBlock(row map[string]any) (string, domain.Block) {
	block := domain.Block{settingValueField: row[settingValueField]}
	if locked, _ := row[lockAttribute].(bool); locked {
		block[lockAttribute] = true
	}
	return text(row["setting"]), block
}

// toleranceWrite sets a hub's or a collection's tolerance, locked when the block says so; a
// removed block leaves it to the level above.
func toleranceWrite(change domain.BlockChange, plan string) operation {
	in := usecase.Input{"plan_id": plan, "time_tolerance_days": nil}
	if change.Op != domain.OpRemove {
		in["time_tolerance_days"] = change.After[settingValueField]
		in[lockAttribute] = change.After[lockAttribute] == true
	}
	return operation{UseCase: writeTolerance, Path: change.Path(), Input: in}
}

// collection is the identifier and the block's own key a collection block's key names; an empty
// identifier where the handle names no collection of the document's hub.
func (p places) collection(key string) (string, string) {
	handle, inner, _ := strings.Cut(key, ".")
	id := p.id[handle]
	if id == "" || !p.children[id] {
		return "", inner
	}
	return id, inner
}

// collectionKeyRefusal is why a collection block's key cannot be applied, or nil.
func (p places) collectionKeyRefusal(section, key string) *shared.FieldError {
	path := section + "." + key
	handle, inner, cut := strings.Cut(key, ".")
	if !cut || handle == "" || !domain.ValidKey(inner) {
		refusal := domain.Refusal("config.key_invalid", path, nil)
		return &refusal
	}
	if id, _ := p.collection(key); id == "" {
		refusal := domain.Refusal("config.collection_unknown", path, map[string]string{"handle": handle})
		return &refusal
	}
	return nil
}

// exportCollections reads a collection section: once per collection of the hub, or one listing
// for all of them.
func (d Desk) exportCollections(ctx context.Context, actor appshared.ActorContext, s section, out *current) error {
	blocks := domain.Section{}
	byKey := map[string]map[string]any{}
	var shared map[string]any
	for _, id := range out.Places.order {
		answer := shared
		if answer == nil {
			read := s
			if s.listFor != nil {
				read.listInput = s.listFor(id)
			}
			raw, err := d.list(ctx, actor, read)
			if err != nil {
				return err
			}
			normal, err := normalise(map[string]any(raw))
			if err != nil {
				return err
			}
			answer, _ = normal.(map[string]any)
			if s.listFor == nil {
				shared = answer
			}
		}
		handle := out.Places.key[id]
		for _, raw := range s.rowsFor(answer, id) {
			row, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			key, block := s.read(row)
			if key == "" {
				continue
			}
			if s.present != nil {
				block = s.present(out.Places, block)
			}
			blocks[handle+"."+key] = block
			byKey[handle+"."+key] = row
		}
	}
	out.Document[s.name] = blocks
	out.Rows[s.name] = byKey
	return nil
}
