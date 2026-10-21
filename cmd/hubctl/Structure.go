// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
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
