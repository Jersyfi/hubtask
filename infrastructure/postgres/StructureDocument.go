// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// layerDocument is a structure layer as `policies.structure` and `settings.structure` store it. The
// settings keep the contract's spelling (StructureSettings), so the wire and the row say a setting
// the same way. This file is the only place that knows the stored shape.
type layerDocument struct {
	Preset   string                     `json:"preset,omitempty"`
	Settings *structureSettingsDocument `json:"settings,omitempty"`
	// The workspace's own keys: who locked what, and the preset a new hub starts from.
	Locks         map[string]string `json:"locks,omitempty"`
	DefaultPreset string            `json:"default_preset,omitempty"`
}

// structureSettingsDocument is the eight settings; a key that is absent is a setting this layer leaves to
// the one above.
type structureSettingsDocument struct {
	Levels      []levelDocument                `json:"levels,omitempty"`
	Fields      map[string]levelFieldsDocument `json:"fields,omitempty"`
	Views       *viewsDocument                 `json:"views,omitempty"`
	Counts      *countsDocument                `json:"counts,omitempty"`
	Completion  string                         `json:"completion,omitempty"`
	Capture     []string                       `json:"capture,omitempty"`
	Celebration string                         `json:"celebration,omitempty"`
	// Automation is a pointer, because an empty list ("nothing runs by itself") is a decision and
	// an absent key is none.
	Automation *[]string `json:"automation,omitempty"`
	// Capture likewise: an empty list recognises nothing, which is not "inherit".
	CaptureSet bool `json:"capture_set,omitempty"`
}

type levelDocument struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type levelFieldsDocument struct {
	Capabilities []string `json:"capabilities"`
}

type viewsDocument struct {
	Offered []string `json:"offered"`
	Default string   `json:"default"`
}

type countsDocument struct {
	Figure   string `json:"figure"`
	FieldKey string `json:"field_key,omitempty"`
}

// valuesDocument writes the values a layer decides.
func valuesDocument(values work.StructureValues) *structureSettingsDocument {
	if len(values.Decided()) == 0 {
		return nil
	}
	document := &structureSettingsDocument{
		Completion:  string(values.Completion),
		Celebration: string(values.Celebration),
	}
	for _, level := range values.Levels {
		document.Levels = append(document.Levels, levelDocument{Type: string(level.Type), Name: level.Name})
	}
	if values.Fields != nil {
		document.Fields = make(map[string]levelFieldsDocument, len(values.Fields))
		for itemType, level := range values.Fields {
			capabilities := make([]string, 0, len(level.Capabilities))
			for _, capability := range level.Capabilities {
				capabilities = append(capabilities, string(capability))
			}
			document.Fields[string(itemType)] = levelFieldsDocument{Capabilities: capabilities}
		}
	}
	if values.Views != nil {
		offered := make([]string, 0, len(values.Views.Offered))
		for _, layout := range values.Views.Offered {
			offered = append(offered, string(layout))
		}
		document.Views = &viewsDocument{Offered: offered, Default: string(values.Views.Default)}
	}
	if values.Counts != nil {
		document.Counts = &countsDocument{Figure: string(values.Counts.Figure), FieldKey: values.Counts.FieldKey}
	}
	if values.Capture != nil {
		document.CaptureSet = true
		for _, token := range values.Capture {
			document.Capture = append(document.Capture, string(token))
		}
	}
	if values.Automation != nil {
		behaviours := make([]string, 0, len(values.Automation))
		for _, behaviour := range values.Automation {
			behaviours = append(behaviours, string(behaviour))
		}
		document.Automation = &behaviours
	}
	return document
}

// valuesFrom reads stored values back. Nothing is judged here: the domain clips what it reads to
// the installation's bound, so a value stored before the installation narrowed reads as narrowed.
func valuesFrom(document *structureSettingsDocument) work.StructureValues {
	if document == nil {
		return work.StructureValues{}
	}
	values := work.StructureValues{
		Completion:  work.CompletionPolicy(document.Completion),
		Celebration: work.CelebrationStyle(document.Celebration),
	}
	if document.Levels != nil {
		values.Levels = make([]work.Level, 0, len(document.Levels))
		for _, level := range document.Levels {
			values.Levels = append(values.Levels, work.Level{Type: work.ItemType(level.Type), Name: level.Name})
		}
	}
	if document.Fields != nil {
		values.Fields = make(map[work.ItemType]work.LevelFields, len(document.Fields))
		for itemType, level := range document.Fields {
			capabilities := make([]work.Capability, 0, len(level.Capabilities))
			for _, capability := range level.Capabilities {
				capabilities = append(capabilities, work.Capability(capability))
			}
			values.Fields[work.ItemType(itemType)] = work.LevelFields{Capabilities: capabilities}
		}
	}
	if document.Views != nil {
		offered := make([]work.ViewLayout, 0, len(document.Views.Offered))
		for _, layout := range document.Views.Offered {
			offered = append(offered, work.ViewLayout(layout))
		}
		values.Views = &work.Views{Offered: offered, Default: work.ViewLayout(document.Views.Default)}
	}
	if document.Counts != nil {
		values.Counts = &work.Counts{Figure: work.Figure(document.Counts.Figure), FieldKey: document.Counts.FieldKey}
	}
	if document.CaptureSet {
		values.Capture = make([]work.CaptureToken, 0, len(document.Capture))
		for _, token := range document.Capture {
			values.Capture = append(values.Capture, work.CaptureToken(token))
		}
	}
	if document.Automation != nil {
		values.Automation = make([]work.AutomationBehaviour, 0, len(*document.Automation))
		for _, behaviour := range *document.Automation {
			values.Automation = append(values.Automation, work.AutomationBehaviour(behaviour))
		}
	}
	return values
}

// containerLayerDocument is a container's layer as `policies.structure` stores it. A collection's own
// completion is not in it: that is the `completion_policy` key beside it, the one store of that value
// (UpdateContainerPolicies writes it too).
func containerLayerDocument(container work.Container) ([]byte, string, error) {
	values := container.Structure.Values
	completion := ""
	if container.Type == work.ContainerCollection {
		completion = string(values.Completion)
		values = values.Without(work.SettingCompletion)
	}
	encoded, err := json.Marshal(layerDocument{Preset: string(container.Structure.Preset), Settings: valuesDocument(values)})
	if err != nil {
		return nil, "", shared.ErrInternal.WithDetail("postgres.document_unwritable").
			WithCause(fmt.Errorf("writing a structure layer: %w", err))
	}
	return encoded, completion, nil
}

// containerLayerFrom reads a container's layer, a collection's own completion folded back in.
func containerLayerFrom(containerType work.ContainerType, raw, ownCompletion string) (work.StructureLayer, error) {
	var document layerDocument
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &document); err != nil {
			return work.StructureLayer{}, shared.ErrInternal.WithDetail("postgres.document_unreadable").
				WithCause(fmt.Errorf("reading a structure layer: %w", err))
		}
	}
	layer := work.StructureLayer{Preset: work.PresetKey(document.Preset), Values: valuesFrom(document.Settings)}
	if containerType == work.ContainerCollection {
		layer.Values.Completion = work.CompletionPolicy(ownCompletion)
	}
	return layer, nil
}

// workspaceLayerDocument is the workspace's layer as `settings.structure` stores it.
func workspaceLayerDocument(workspace work.WorkspaceStructure) ([]byte, error) {
	document := layerDocument{
		Preset:        string(workspace.Layer.Preset),
		Settings:      valuesDocument(workspace.Layer.Values),
		DefaultPreset: string(workspace.DefaultPreset),
	}
	if len(workspace.Locks) > 0 {
		document.Locks = make(map[string]string, len(workspace.Locks))
		for key, by := range workspace.Locks {
			document.Locks[string(key)] = by.String()
		}
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("postgres.document_unwritable").
			WithCause(fmt.Errorf("writing the workspace's structure: %w", err))
	}
	return encoded, nil
}

// workspaceLayerFrom reads the workspace's layer. A lock whose author no longer parses is kept with
// no author rather than dropped: losing a lock would widen what every collection may do.
func workspaceLayerFrom(raw string) (work.WorkspaceStructure, error) {
	var document layerDocument
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &document); err != nil {
			return work.WorkspaceStructure{}, shared.ErrInternal.WithDetail("postgres.document_unreadable").
				WithCause(fmt.Errorf("reading the workspace's structure: %w", err))
		}
	}
	workspace := work.WorkspaceStructure{
		Layer:         work.StructureLayer{Preset: work.PresetKey(document.Preset), Values: valuesFrom(document.Settings)},
		DefaultPreset: work.PresetKey(document.DefaultPreset),
	}
	if len(document.Locks) > 0 {
		workspace.Locks = make(map[work.LockKey]shared.ID, len(document.Locks))
		for key, by := range document.Locks {
			author, err := shared.ParseID(by)
			if err != nil {
				author = ""
			}
			workspace.Locks[work.LockKey(key)] = author
		}
	}
	return workspace, nil
}
