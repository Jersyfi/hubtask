// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"sort"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// operation is one use case a plan runs: the registry's door, so that every block is changed
// through the use case a screen, `hubctl` and MCP reach and held to what it checks (P-18).
type operation struct {
	UseCase string
	Input   usecase.Input
	// Path is the key path the operation is for, named in a refusal.
	Path string
	// StepUp marks an operation that changes the sign-in rule: it carries the apply's fresh proof.
	StepUp bool
}

// section is one family of building blocks the document holds, declared by the use cases that
// read and write it. The order of the sections is the order a plan applies them in - a module
// before the blocks it carries, a role before what names it - and removals run in reverse.
type section struct {
	name string
	// module is the module the section belongs to: where it is off, the section is not read and
	// may not be written.
	module module.Key
	// list reads the section; rows is the field of its answer holding the rows, key the row's
	// field naming the block.
	list      string
	listInput usecase.Input
	// scope is what every operation of the section carries besides its block: a hub's section
	// names the hub in each.
	scope usecase.Input
	rows  string
	key   string
	// rowsOf replaces rows where the answer is one object rather than a list of rows.
	rowsOf func(answer map[string]any) []any
	// attributes is what the document keeps of a row; fixed are those set at creation only.
	attributes []string
	fixed      []string
	// system is the row field marking a shipped block, which is changed but never removed.
	system string
	// create, change and remove are the use cases; locate is the input field change and remove
	// name the block by, read from the row's field locateFrom.
	create, change, remove string
	locate, locateFrom     string
	// changeable is what change takes; whole means it takes every one of them each time. separate
	// names an attribute changed by a use case of its own, taking the locator and the attribute.
	changeable []string
	whole      bool
	separate   map[string]string
	// block and ops replace the generic reading and writing where a section needs its own.
	block func(row map[string]any) (string, domain.Block)
	ops   func(change domain.BlockChange, row map[string]any) ([]operation, error)
	// hides reports whether a change hides data from somebody; permissions whether the section
	// changes what somebody may do.
	hides       func(change domain.BlockChange) bool
	permissions bool
}

const (
	sectionModules   = "modules"
	sectionRoles     = "roles"
	sectionFields    = "fields"
	sectionLinkTypes = "link_types"
	sectionTypes     = "types"
	sectionWorkflows = "workflows"
	sectionSettings  = "settings"
	sectionStructure = "structure"
	// The service desk's blocks (M5): calendars before the policies that name them, both after the
	// types a policy measures.
	sectionCalendars  = "business_calendars"
	sectionPolicies   = "sla_policies"
	sectionReplies    = "reply_templates"
	switchAttribute   = "switch"
	settingValueField = "value"
)

// sections is the document's shape, in apply order.
func sections() []section {
	return []section{
		{
			name: sectionModules, list: "ListModules", rows: "modules", attributes: []string{switchAttribute},
			block: func(row map[string]any) (string, domain.Block) {
				state := "INHERIT"
				if text(row["source"]) == "WORKSPACE" {
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
					"module": strings.ToUpper(change.Key), "switch": text(change.After[switchAttribute]),
				}}}, nil
			},
		},
		{
			name: sectionRoles, list: "ListRoles", rows: "custom", key: "key",
			attributes: []string{"name", "permissions"},
			create:     "CreateRole", change: "ChangeRole", remove: "DeleteRole", locate: "key", locateFrom: "key",
			changeable: []string{"name", "permissions"}, whole: true, permissions: true,
		},
		{
			name: sectionFields, list: "ListCustomFields", rows: "data", key: "key",
			attributes: []string{"kind", "options", "currency", "formula", "is_required", "restricted", "read_restricted", "applies_to"},
			fixed:      []string{"kind", "currency", "formula"},
			create:     "DefineCustomField", change: "UpdateCustomField", remove: "DeleteCustomField",
			locate: "field_id", locateFrom: "id",
			changeable: []string{"options", "is_required", "restricted", "read_restricted", "applies_to"},
			hides: func(change domain.BlockChange) bool {
				if change.Op == domain.OpRemove {
					return true
				}
				hidden, _ := change.After["read_restricted"].(bool)
				was, _ := change.Before["read_restricted"].(bool)
				return hidden && !was
			},
		},
		{
			name: sectionLinkTypes, list: "ListLinkTypes", rows: "data", key: "key", system: "system",
			attributes: []string{"name", "inverse_name", "direction", "cardinality", "cycles", "waits", "position", "archived"},
			fixed:      []string{"direction", "cardinality", "cycles", "waits"},
			create:     "CreateLinkType", change: "ChangeLinkType", remove: "DeleteLinkType", locate: "key", locateFrom: "key",
			changeable: []string{"name", "inverse_name", "position", "archived"},
			hides:      removing,
		},
		{
			name: sectionTypes, module: module.Workflows, list: "ListEntryTypes", rows: "data", key: "key", system: "system",
			attributes: []string{
				"level", "name", "icon", "color_token", "completable", "workflow", "record", "schedulable",
				"estimable", "allowed_children", "position", "archived", "form",
			},
			fixed:  []string{"level"},
			create: "CreateEntryType", change: "ChangeEntryType", remove: "DeleteEntryType", locate: "key", locateFrom: "key",
			changeable: []string{
				"name", "icon", "color_token", "archived", "completable", "workflow", "record", "schedulable",
				"estimable", "allowed_children", "position",
			},
			separate: map[string]string{"form": "SetEntryTypeForm"},
			hides:    removing,
		},
		{
			// The workspace's own structure, one block per setting it decides itself; a setting
			// left to the installation's or Hubtask's default has no block (configuration/model.md).
			name: sectionStructure, list: ReadWorkspaceName, key: "setting", attributes: []string{settingValueField, presetIDAttribute},
			rowsOf: structureRows,
			block: func(row map[string]any) (string, domain.Block) {
				if text(row["source"]) != "WORKSPACE" {
					return "", nil
				}
				block := domain.Block{}
				for _, attribute := range []string{settingValueField, presetIDAttribute} {
					if value := row[attribute]; value != nil {
						block[attribute] = value
					}
				}
				return text(row["setting"]), block
			},
			ops: structureOps,
		},
		{
			name: sectionWorkflows, module: module.Workflows, list: "ListWorkflows", rows: "data", key: "key", system: "system",
			attributes: []string{"name", "shape"},
			remove:     "DeleteWorkflow", locate: "key", locateFrom: "key",
			block: func(row map[string]any) (string, domain.Block) {
				block := domain.Block{"shape": row["published"]}
				if name := row["name"]; name != nil {
					block["name"] = name
				}
				return text(row["key"]), block
			},
			ops: workflowOps,
		},
		// Each is defined whole under its key - the same use case creates and replaces it.
		{
			name: sectionCalendars, module: module.ServiceDesk, list: "ListBusinessCalendars", rows: "data", key: "key",
			attributes: []string{"name", "zone", "hours", "holidays"},
			create:     "DefineBusinessCalendar", change: "DefineBusinessCalendar", remove: "DeleteBusinessCalendar",
			locate: "key", locateFrom: "key", changeable: []string{"name", "zone", "hours", "holidays"}, whole: true,
		},
		{
			name: sectionPolicies, module: module.ServiceDesk, list: "ListSlaPolicies", rows: "data", key: "key",
			attributes: []string{"name", "calendar", "entry_types", "priority_field", "goals", "position", "enabled"},
			create:     "DefineSlaPolicy", change: "DefineSlaPolicy", remove: "DeleteSlaPolicy",
			locate: "key", locateFrom: "key",
			changeable: []string{"name", "calendar", "entry_types", "priority_field", "goals", "position", "enabled"}, whole: true,
		},
		{
			name: sectionReplies, module: module.ServiceDesk, list: "ListReplyTemplates", rows: "data", key: "key",
			attributes: []string{"name", "body"},
			create:     "DefineReplyTemplate", change: "DefineReplyTemplate", remove: "DeleteReplyTemplate",
			locate: "key", locateFrom: "key", changeable: []string{"name", "body"}, whole: true,
		},
	}
}

// workflowOps writes a workflow: a new one is created with its shape; a changed shape is saved
// as the draft and published, so that the publish's own checks - a status in use removed without
// a mapping - refuse the apply as they refuse the designer.
func workflowOps(change domain.BlockChange, _ map[string]any) ([]operation, error) {
	path := change.Path()
	switch change.Op {
	case domain.OpAdd:
		return []operation{{UseCase: "CreateWorkflow", Path: path, Input: usecase.Input{
			"key": change.Key, "name": change.After["name"], "shape": change.After["shape"],
		}}}, nil
	case domain.OpChange:
		in := usecase.Input{"key": change.Key}
		for _, field := range change.Fields {
			in[field] = change.After[field]
		}
		ops := []operation{{UseCase: "SaveWorkflowDraft", Path: path, Input: in}}
		if _, shaped := in["shape"]; shaped {
			ops = append(ops, operation{UseCase: "PublishWorkflow", Path: path, Input: usecase.Input{"key": change.Key}})
		}
		return ops, nil
	default:
		return []operation{{UseCase: "DeleteWorkflow", Path: path, Input: usecase.Input{"key": change.Key}}}, nil
	}
}

// The structure section's two blocks beside the settings: the workspace's own locks on every hub
// and collection, and the preset a new hub starts from - Hubtask's by its key, or the workspace's
// own by its identifier, which is the one place a preset has no stable key.
const (
	structureLocks         = "locks"
	structureDefaultPreset = "default_preset"
	presetIDAttribute      = "preset_id"
)

// structureRows is the workspace's answer as one row per structure setting: its value in force
// and the level it comes from; then its locks and its default preset where it set them.
func structureRows(answer map[string]any) []any {
	structure, _ := answer["structure"].(map[string]any)
	values, _ := structure["settings"].(map[string]any)
	sources, _ := structure["sources"].(map[string]any)
	rows := make([]any, 0, len(sources)+2)
	for _, name := range work.StructureSettingNames() {
		rows = append(rows, map[string]any{
			"setting": string(name), "source": sources[string(name)], settingValueField: values[string(name)],
		})
	}
	locks, _ := structure["locks"].(map[string]any)
	var own []any
	for _, key := range sortedKeysOf(locks) {
		if lock, _ := locks[key].(map[string]any); text(lock["origin"]) == "WORKSPACE" {
			own = append(own, key)
		}
	}
	if len(own) > 0 {
		rows = append(rows, map[string]any{"setting": structureLocks, "source": "WORKSPACE", settingValueField: own})
	}
	if preset := text(structure["default_preset"]); preset != "" {
		rows = append(rows, map[string]any{"setting": structureDefaultPreset, "source": "WORKSPACE", settingValueField: preset})
	} else if id := text(structure["default_preset_id"]); id != "" {
		rows = append(rows, map[string]any{"setting": structureDefaultPreset, "source": "WORKSPACE", presetIDAttribute: id})
	}
	return rows
}

func sortedKeysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// structureOps sets or resets one structure setting of the workspace through the use case its
// settings screen calls, so the bound and the installation's locks refuse an apply as they refuse
// the screen.
func structureOps(change domain.BlockChange, _ map[string]any) ([]operation, error) {
	path := change.Path()
	switch change.Key {
	case structureLocks:
		// The use case takes the whole set; a removed block lifts every lock of the workspace's.
		locks := []any{}
		if change.Op != domain.OpRemove {
			list, isList := change.After[settingValueField].([]any)
			if !isList {
				return nil, refused("config.document_invalid", path, nil)
			}
			locks = list
		}
		return []operation{{UseCase: "SetWorkspaceStructure", Path: path, Input: usecase.Input{"locks": locks}}}, nil
	case structureDefaultPreset:
		in := usecase.Input{"clear_default_preset": true}
		if change.Op != domain.OpRemove {
			if id := text(change.After[presetIDAttribute]); id != "" {
				in = usecase.Input{"default_preset_id": id}
			} else {
				in = usecase.Input{"default_preset": strings.ToUpper(text(change.After[settingValueField]))}
			}
		}
		return []operation{{UseCase: "SetWorkspaceStructure", Path: path, Input: in}}, nil
	}
	if _, has := change.After[presetIDAttribute]; has {
		return nil, refused("config.attribute_unknown", path+"."+presetIDAttribute, nil)
	}
	if _, err := work.ParseStructureSetting(change.Key); err != nil {
		return nil, refused("config.key_unknown", change.Path(), nil)
	}
	in := usecase.Input{"reset": []any{change.Key}}
	if change.Op != domain.OpRemove {
		in = usecase.Input{"settings": map[string]any{change.Key: change.After[settingValueField]}}
	}
	return []operation{{UseCase: "SetWorkspaceStructure", Path: change.Path(), Input: in}}, nil
}

func removing(change domain.BlockChange) bool { return change.Op == domain.OpRemove }

// ops is the use cases one change runs, by the section's declaration.
func (s section) operations(change domain.BlockChange, row map[string]any) ([]operation, error) {
	if s.ops != nil {
		return s.ops(change, row)
	}
	ops, err := s.blockOperations(change, row)
	for _, op := range ops {
		for name, value := range s.scope {
			op.Input[name] = value
		}
	}
	return ops, err
}

func (s section) blockOperations(change domain.BlockChange, row map[string]any) ([]operation, error) {
	path := change.Path()
	switch change.Op {
	case domain.OpAdd:
		in := usecase.Input{s.key: change.Key}
		var later []operation
		for _, attribute := range change.Fields {
			if useCase, held := s.separate[attribute]; held {
				later = append(later, operation{UseCase: useCase, Path: path, Input: usecase.Input{s.locate: change.Key, attribute: change.After[attribute]}})
				continue
			}
			if attribute == "archived" {
				if archived, _ := change.After[attribute].(bool); archived {
					later = append(later, operation{UseCase: s.change, Path: path, Input: usecase.Input{s.locate: change.Key, "archived": true}})
				}
				continue
			}
			in[attribute] = change.After[attribute]
		}
		return append([]operation{{UseCase: s.create, Path: path, Input: in}}, later...), nil
	case domain.OpChange:
		locator := row[s.locateFrom]
		in := usecase.Input{s.locate: locator}
		var ops []operation
		for _, attribute := range change.Fields {
			if useCase, held := s.separate[attribute]; held {
				ops = append(ops, operation{UseCase: useCase, Path: path, Input: usecase.Input{s.locate: locator, attribute: change.After[attribute]}})
				continue
			}
			in[attribute] = change.After[attribute]
		}
		if s.whole {
			for _, attribute := range s.changeable {
				in[attribute] = change.After[attribute]
			}
		}
		if len(in) > 1 {
			ops = append([]operation{{UseCase: s.change, Path: path, Input: in}}, ops...)
		}
		return ops, nil
	default:
		return []operation{{UseCase: s.remove, Path: path, Input: usecase.Input{s.locate: row[s.locateFrom]}}}, nil
	}
}

// writable is every attribute a document may name for the section.
func (s section) writable(attribute string) bool {
	for _, name := range s.attributes {
		if name == attribute {
			return true
		}
	}
	return false
}

func (s section) isFixed(attribute string) bool {
	for _, name := range s.fixed {
		if name == attribute {
			return true
		}
	}
	return false
}

func text(v any) string {
	s, _ := v.(string)
	return s
}
