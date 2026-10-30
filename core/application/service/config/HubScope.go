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

// A hub's own document (config-document.md §1) holds what the hub decides itself - its structure
// settings, its module switches, which workflow it runs for which type and its own versions of the
// workspace's business calendars - each block by a key
// that is the same in every hub, so a hub's document applies to a second hub unchanged. The hub
// is named by the request, never inside the document: a hub has no stable key.
const (
	sectionScheme = "scheme"
	// schemeEveryType is the scheme row without an entry type: the workflow for every type the hub
	// names no row for.
	schemeEveryType   = "every_type"
	workflowAttribute = "workflow"
	sourceHub         = "HUB"
)

// hubSections is a hub document's shape, in apply order: a module before the blocks it carries.
func hubSections(hub shared.ID) []section {
	id := hub.String()
	return []section{
		{
			name: sectionModules, list: "ListModules", listInput: usecase.Input{"hub_id": id}, rows: "modules",
			attributes: []string{switchAttribute},
			block: func(row map[string]any) (string, domain.Block) {
				state := "INHERIT"
				if text(row["source"]) == sourceHub {
					state = "OFF"
					if on, _ := row["on"].(bool); on {
						state = "ON"
					}
				}
				return strings.ToLower(text(row["module"])), domain.Block{switchAttribute: state}
			},
			ops: func(change domain.BlockChange, _ map[string]any) ([]operation, error) {
				if change.Op != domain.OpChange {
					return nil, refused("config.not_removable", change.Path(), nil)
				}
				return []operation{{UseCase: "SetModule", Path: change.Path(), Input: usecase.Input{
					"module": strings.ToUpper(change.Key), "switch": text(change.After[switchAttribute]), "hub_id": id,
				}}}, nil
			},
		},
		{
			// One block per structure setting the hub decides itself; one left to the workspace has
			// no block, as in the workspace's own structure section.
			name: sectionStructure, list: "GetContainer", listInput: usecase.Input{"container_id": id},
			key: "setting", attributes: []string{settingValueField},
			rowsOf: hubStructureRows,
			block: func(row map[string]any) (string, domain.Block) {
				if text(row["source"]) != sourceHub {
					return "", nil
				}
				return text(row["setting"]), domain.Block{settingValueField: row[settingValueField]}
			},
			ops: func(change domain.BlockChange, _ map[string]any) ([]operation, error) {
				if _, err := work.ParseStructureSetting(change.Key); err != nil {
					return nil, refused("config.key_unknown", change.Path(), nil)
				}
				in := usecase.Input{"container_id": id, "reset": []any{change.Key}}
				if change.Op != domain.OpRemove {
					in = usecase.Input{"container_id": id, "settings": map[string]any{change.Key: change.After[settingValueField]}}
				}
				return []operation{{UseCase: "SetContainerStructure", Path: change.Path(), Input: in}}, nil
			},
			hides: removing,
		},
		{
			// Which workflow the hub runs, by entry type; `every_type` is the row for every type
			// the hub names none for.
			name: sectionScheme, module: module.Workflows, list: "ListWorkflows", key: "entry_type",
			attributes: []string{workflowAttribute},
			rowsOf: func(answer map[string]any) []any {
				rows, _ := answer["scheme"].([]any)
				var out []any
				for _, raw := range rows {
					if row, _ := raw.(map[string]any); text(row["container_id"]) == id {
						out = append(out, row)
					}
				}
				return out
			},
			block: func(row map[string]any) (string, domain.Block) {
				key := text(row["entry_type"])
				if key == "" {
					key = schemeEveryType
				}
				return key, domain.Block{workflowAttribute: row[workflowAttribute]}
			},
			ops: func(change domain.BlockChange, _ map[string]any) ([]operation, error) {
				entryType := change.Key
				if entryType == schemeEveryType {
					entryType = ""
				}
				workflow := ""
				if change.Op != domain.OpRemove {
					workflow = text(change.After[workflowAttribute])
					if workflow == "" {
						return nil, refused("usecase.field_required", change.Path()+"."+workflowAttribute, nil)
					}
				}
				// Confirmed: the plan's reach is the preview a person saw before applying it.
				return []operation{{UseCase: "SetWorkflowScheme", Path: change.Path(), Input: usecase.Input{
					"container_id": id, "entry_type": entryType, "workflow": workflow, "confirm": true,
				}}}, nil
			},
		},
		{
			// The hub's own version of a workspace calendar, under the workspace's key (UC-SVC-01
			// check 5); after the scheme, as the workspace's calendars follow its types.
			name: sectionCalendars, module: module.ServiceDesk, list: "ListHubCalendars", listInput: usecase.Input{"hub_id": id},
			rows: "data", key: "key", scope: usecase.Input{"hub_id": id},
			attributes: []string{"name", "zone", "hours", "holidays"},
			create:     "DefineHubCalendar", change: "DefineHubCalendar", remove: "DeleteHubCalendar",
			locate: "key", locateFrom: "key", changeable: []string{"name", "zone", "hours", "holidays"}, whole: true,
		},
	}
}

// hubStructureRows is a hub's answer as one row per structure setting: its value in force and
// the level it comes from.
func hubStructureRows(answer map[string]any) []any {
	structure, _ := answer["structure"].(map[string]any)
	values, _ := structure["settings"].(map[string]any)
	sources, _ := structure["sources"].(map[string]any)
	rows := make([]any, 0, len(sources))
	for _, name := range work.StructureSettingNames() {
		rows = append(rows, map[string]any{
			"setting": string(name), "source": sources[string(name)], settingValueField: values[string(name)],
		})
	}
	return rows
}

// isHub reads the place a hub document is asked for: a hub of the workspace, or not found - a
// collection, another workspace's hub and a private hub the reader holds nothing in alike (T-04).
func (d Desk) isHub(ctx context.Context, actor appshared.ActorContext, hub shared.ID) error {
	answer, err := d.Catalogue.Invoke(ctx, "GetContainer", actor, usecase.Input{"container_id": hub.String()})
	if err != nil {
		return err
	}
	if text(answer["type"]) != string(work.ContainerHub) {
		return shared.ErrNotFound.WithDetail("config.hub_unknown")
	}
	return nil
}

// hubOf reads the scope a call names: a hub's document for `hub_id`, the workspace's otherwise.
func hubOf(in usecase.Input) (shared.ID, error) {
	if in.String("hub_id") == "" {
		return "", nil
	}
	return in.ID("hub_id")
}

// hubField is the input naming a hub's document.
func hubField() usecase.Field {
	return usecase.Field{Name: "hub_id", Kind: usecase.KindID, Description: "A hub of the workspace, for the hub's own document; omitted is the workspace's."}
}
