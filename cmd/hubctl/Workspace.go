// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const workspaceStructurePath = workspacePath + "/structure"

const workspaceStructureUsage = "[--preset <key>] [--settings <json>] [--reset <setting>…] " +
	"[--lock <key>… | --no-locks] [--default-preset <key> | --no-default-preset] [--version <n>]"

func workspaceGroup() group {
	return group{
		name:    "workspace",
		summary: "the workspace the credential belongs to",
		commands: []command{
			{
				name:    "structure",
				usage:   workspaceStructureUsage,
				summary: "show or change the structure every hub and collection inherits, its locks and the default preset",
				run:     workspaceStructure,
			},
		},
	}
}

// workspaceStructure shows the workspace's structure, or changes it. `--lock` names the whole set
// of locks after the change, a setting (views) or one capability of one type
// (fields.TASK.DUE_DATE); `--no-locks` removes them all.
func workspaceStructure(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "workspace", "structure", workspaceStructureUsage)
	preset := flags.String("preset", "", "copy this preset's settings into the workspace's")
	settings := flags.String("settings", "", "settings to set, as JSON")
	defaultPreset := flags.String("default-preset", "", "the preset a new hub starts from")
	noDefault := flags.Bool("no-default-preset", false, "new hubs start from the workspace's own settings")
	noLocks := flags.Bool("no-locks", false, "remove every lock")
	version := flags.String("version", "", "the version you read; refused if it moved")
	var reset, locks stringList
	flags.Var(&reset, "reset", "a setting that goes back to Hubtask's default; repeat for several")
	flags.Var(&locks, "lock", "a setting or fields.<TYPE>.<CAPABILITY> to lock; repeat for each - the whole set")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	unchanged := *preset == "" && *settings == "" && len(reset) == 0 && len(locks) == 0 &&
		!*noLocks && *defaultPreset == "" && !*noDefault
	if unchanged {
		var workspace openapi.Workspace
		if err := client.Get(ctx, workspacePath, nil, &workspace); err != nil {
			return err
		}
		return cli.Emit(workspace.Structure, workspaceStructureTable(workspace.Structure))
	}

	change := map[string]any{}
	if *preset != "" {
		change["preset"] = *preset
	}
	if *settings != "" {
		var document openapi.StructureSettings
		decoder := json.NewDecoder(strings.NewReader(*settings))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&document); err != nil {
			return usagef("--settings is the settings as JSON: %v", err)
		}
		change["settings"] = document
	}
	if len(reset) > 0 {
		for _, name := range reset {
			if !openapi.StructureSettingName(name).Valid() {
				return usagef("--reset takes a setting: levels, fields, views, counts, completion, capture, celebration or automation")
			}
		}
		change["reset"] = []string(reset)
	}
	switch {
	case *noLocks && len(locks) > 0:
		return usagef("--lock and --no-locks cannot both be given")
	case *noLocks:
		change["locks"] = []string{}
	case len(locks) > 0:
		change["locks"] = []string(locks)
	}
	switch {
	case *noDefault && *defaultPreset != "":
		return usagef("--default-preset and --no-default-preset cannot both be given")
	case *noDefault:
		change["default_preset"] = nil
	case *defaultPreset != "":
		change["default_preset"] = *defaultPreset
	}

	ifMatch := ""
	if *version != "" {
		ifMatch = `"` + *version + `"`
	}
	var structure openapi.WorkspaceStructure
	if err := client.PatchVersioned(ctx, workspaceStructurePath, change, ifMatch, &structure); err != nil {
		return err
	}
	return cli.Emit(structure, workspaceStructureTable(&structure))
}

// workspaceStructureTable is the settings with their sources and locks, and the default preset.
func workspaceStructureTable(structure *openapi.WorkspaceStructure) Table {
	if structure == nil {
		return Table{Columns: []string{"setting", "source", "locked", "value"}}
	}
	table := structureTable(&openapi.EffectiveStructure{
		Settings: structure.Settings,
		Sources:  structure.Sources, Locks: structure.Locks,
	})
	table.Rows = append(table.Rows, []string{"default preset", "WORKSPACE", "", stringOf(structure.DefaultPreset)})
	return table
}

func stringOf[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
