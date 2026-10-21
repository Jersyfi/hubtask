// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const presetsPath = "/presets"

func presetGroup() group {
	return group{
		name:    "preset",
		summary: "the presets a hub or collection starts from",
		commands: []command{
			{
				name:    "ls",
				usage:   "",
				summary: "list the presets and what each offers",
				run:     presetList,
			},
		},
	}
}

func presetList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "preset", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var presets []openapi.Preset
	if err := client.Get(ctx, presetsPath, nil, &presets); err != nil {
		return err
	}

	rows := make([][]string, 0, len(presets))
	for _, preset := range presets {
		rows = append(rows, []string{
			string(preset.Key), levelsColumn(preset.Settings), viewsColumn(preset.Settings),
		})
	}
	return cli.Emit(presets, Table{Columns: []string{"key", "levels", "views"}, Rows: rows})
}

// levelsColumn is the offered levels, top-down, each with its own name where it has one.
func levelsColumn(settings openapi.StructureSettings) string {
	if settings.Levels == nil {
		return ""
	}
	names := make([]string, 0, len(*settings.Levels))
	for _, level := range *settings.Levels {
		name := string(level.Type)
		if level.Name != nil {
			name += " (" + *level.Name + ")"
		}
		names = append(names, name)
	}
	return strings.Join(names, " / ")
}

// viewsColumn is the offered layouts, the default first and marked.
func viewsColumn(settings openapi.StructureSettings) string {
	if settings.Views == nil {
		return ""
	}
	layouts := []string{string(settings.Views.Default) + "*"}
	for _, layout := range settings.Views.Offered {
		if string(layout) != string(settings.Views.Default) {
			layouts = append(layouts, string(layout))
		}
	}
	return strings.Join(layouts, ", ")
}

const structureUsage = "<container id> [--preset LIST|TASKS|PROJECT|FLOW] [--settings <json>] [--reset <setting>…] [--version <n>]"

// containerStructure shows a container's structure in force, or changes its own: a preset, settings
// as the contract's JSON document, settings reset to the layer above. The server's refusals - a
// widening, a lock - come back as its message codes, the same as on every other door.
func containerStructure(ctx context.Context, cli *CLI, args []string) error {
	const usage = "container structure " + structureUsage
	containerID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "container", "structure", structureUsage)
	preset := flags.String("preset", "", "copy this preset's settings, replacing the container's own")
	settings := flags.String("settings", "", `settings to set, as JSON: {"views":{"offered":["KANBAN"],"default":"KANBAN"}}`)
	version := flags.String("version", "", "the version you read; refused if it moved")
	var reset stringList
	flags.Var(&reset, "reset", "a setting that goes back to the hub's or the workspace's; repeat for several")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	path := containersPath + "/" + containerID.String()
	var container openapi.Container
	if *preset == "" && *settings == "" && len(reset) == 0 {
		if err := client.Get(ctx, path, nil, &container); err != nil {
			return err
		}
		return cli.Emit(container.Structure, structureTable(container.Structure))
	}

	change := openapi.StructureChange{}
	if *preset != "" {
		key := openapi.StructureChangePreset(*preset)
		if !key.Valid() {
			message, _ := cli.Catalogue.Message("presets.unknown", map[string]string{"preset": *preset})
			return usageError{error: errorString(message)}
		}
		change.Preset = &key
	}
	if *settings != "" {
		var document openapi.StructureSettings
		decoder := json.NewDecoder(strings.NewReader(*settings))
		// A misspelt setting is refused here, as the server would refuse it, rather than dropped.
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&document); err != nil {
			return usagef("--settings is the settings as JSON: %v", err)
		}
		change.Settings = &document
	}
	if len(reset) > 0 {
		names := make([]openapi.StructureSettingName, 0, len(reset))
		for _, name := range reset {
			setting := openapi.StructureSettingName(name)
			if !setting.Valid() {
				return usagef("--reset takes a setting: levels, fields, views, counts, completion, capture, celebration or automation")
			}
			names = append(names, setting)
		}
		change.Reset = &names
	}
	ifMatch := ""
	if *version != "" {
		ifMatch = `"` + *version + `"`
	}
	if err := client.PatchVersioned(ctx, path+"/structure", change, ifMatch, &container); err != nil {
		return err
	}
	if container.KeptByLock != nil && len(*container.KeptByLock) > 0 {
		printf(cli.Err, "kept by a lock: %s\n", strings.Join(*container.KeptByLock, ", "))
	}
	return cli.Emit(container, structureTable(container.Structure))
}

// structureTable is one row per setting: its source, whether it is locked, and its value.
func structureTable(structure *openapi.EffectiveStructure) Table {
	table := Table{Columns: []string{"setting", "source", "locked", "value"}}
	if structure == nil {
		return table
	}
	values := map[string]any{}
	if encoded, err := json.Marshal(structure.Settings); err == nil {
		_ = json.Unmarshal(encoded, &values)
	}
	for _, setting := range []string{"levels", "fields", "views", "counts", "completion", "capture", "celebration", "automation"} {
		locked := ""
		for key, lock := range structure.Locks {
			if key == setting || strings.HasPrefix(key, setting+".") {
				locked = string(lock.Origin)
			}
		}
		value, _ := json.Marshal(values[setting])
		table.Rows = append(table.Rows, []string{setting, string(structure.Sources[setting]), locked, string(value)})
	}
	return table
}
