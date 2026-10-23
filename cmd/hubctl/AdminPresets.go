// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The installation's presets (UC-INS-18) at the terminal: the verbs the API has, so an operator
// who drives an installation from a terminal keeps what every workspace is offered, the default a
// hub starts from and what the default locks.
//
// Settings travel as the StructureSettings document the API takes, so a preset's `settings` copied
// out of `admin presets ls --output json` goes back in as it is.

const adminPresetsPath = "/admin/presets"

func adminPresets(ctx context.Context, cli *CLI, args []string) error {
	const verbs = "ls, create, set, rm"
	if len(args) == 0 {
		return usagef("admin presets needs a command: %s", verbs)
	}
	switch args[0] {
	case "ls":
		return adminPresetsList(ctx, cli, args[1:])
	case "create":
		return adminPresetsCreate(ctx, cli, args[1:])
	case "set":
		return adminPresetsSet(ctx, cli, args[1:])
	case "rm":
		return adminPresetsRemove(ctx, cli, args[1:])
	default:
		return usagef("admin presets has no command %q: %s", args[0], verbs)
	}
}

func adminPresetsList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin presets", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var listed struct {
		Data []openapi.InstallationPreset `json:"data"`
	}
	if err := client.Get(ctx, adminPresetsPath, nil, &listed); err != nil {
		return err
	}
	return cli.Emit(listed.Data, presetsTable(listed.Data...))
}

func adminPresetsCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin presets", "create",
		"--name <name> [--from LIST|TASKS|PROJECT|FLOW] [--settings <json>] [--default] [--lock views,fields.TASK.DUE_DATE]")
	name := flags.String("name", "", "the name every workspace sees")
	from := flags.String("from", "", "one of Hubtask's presets whose settings it takes")
	settings := flags.String("settings", "", "settings on top, as the StructureSettings document")
	isDefault := flags.Bool("default", false, "make it the default a hub starts from where its workspace chose none")
	lock := flags.String("lock", "", "what the default locks for every workspace, comma-separated")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *name == "" {
		return usagef("a preset needs --name")
	}
	body := openapi.InstallationPresetCreate{Name: *name}
	if *from != "" {
		preset := openapi.InstallationPresetCreatePreset(strings.ToUpper(*from))
		body.Preset = &preset
	}
	if *settings != "" {
		parsed, err := settingsFrom(*settings)
		if err != nil {
			return err
		}
		body.Settings = parsed
	}
	if *isDefault {
		body.IsDefault = isDefault
	}
	if *lock != "" {
		locks := locksFrom(*lock)
		body.Locks = &locks
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.InstallationPreset
	if err := client.Post(ctx, adminPresetsPath, body, &created); err != nil {
		return err
	}
	return cli.Emit(created, presetsTable(created))
}

// adminPresetsSet changes a preset: only what a flag names, the rest as it stands.
func adminPresetsSet(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin presets", "set",
		"<id> [--name <name>] [--from <preset>] [--settings <json>] [--default yes|no] [--lock <locks>] [--expected-version <n>]")
	name := flags.String("name", "", "a new name")
	from := flags.String("from", "", "one of Hubtask's presets whose settings replace its own")
	settings := flags.String("settings", "", "settings on top, as the StructureSettings document")
	isDefault := flags.String("default", "", "yes makes it the default, no makes none the default")
	lock := flags.String("lock", "-", "the whole set of what the default locks, comma-separated; empty for none")
	expected := flags.Int("expected-version", 0, "refuse the change if the preset moved past this version")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return usagef("admin presets set needs the preset's id")
	}
	body := openapi.InstallationPresetUpdate{}
	if *name != "" {
		body.Name = name
	}
	if *from != "" {
		preset := openapi.InstallationPresetUpdatePreset(strings.ToUpper(*from))
		body.Preset = &preset
	}
	if *settings != "" {
		parsed, err := settingsFrom(*settings)
		if err != nil {
			return err
		}
		body.Settings = parsed
	}
	switch strings.ToLower(*isDefault) {
	case "":
	case "yes", "true":
		on := true
		body.IsDefault = &on
	case "no", "false":
		off := false
		body.IsDefault = &off
	default:
		return usagef("--default takes yes or no")
	}
	if *lock != "-" {
		locks := locksFrom(*lock)
		body.Locks = &locks
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	ifMatch := ""
	if *expected > 0 {
		ifMatch = strconv.Quote(strconv.Itoa(*expected))
	}
	var changed openapi.InstallationPreset
	if err := client.PatchVersioned(ctx, adminPresetsPath+"/"+flags.Arg(0), body, ifMatch, &changed); err != nil {
		return err
	}
	return cli.Emit(changed, presetsTable(changed))
}

func adminPresetsRemove(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin presets", "rm", "<id>")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return usagef("admin presets rm needs the preset's id")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, adminPresetsPath+"/"+flags.Arg(0), ""); err != nil {
		return err
	}
	printf(cli.Err, "preset %s withdrawn; what was made from it keeps its settings\n", flags.Arg(0))
	return nil
}

func settingsFrom(raw string) (*openapi.StructureSettings, error) {
	var settings openapi.StructureSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, usagef("--settings is not a StructureSettings document: %v", err)
	}
	return &settings, nil
}

func locksFrom(raw string) []string {
	locks := []string{}
	for _, key := range strings.Split(raw, ",") {
		if key = strings.TrimSpace(key); key != "" {
			locks = append(locks, key)
		}
	}
	return locks
}

func presetsTable(presets ...openapi.InstallationPreset) Table {
	rows := make([][]string, 0, len(presets))
	for _, preset := range presets {
		isDefault := ""
		if preset.IsDefault {
			isDefault = "default"
		}
		rows = append(rows, []string{
			preset.Id.String(), preset.Name, isDefault, strings.Join(preset.Locks, ","), strconv.Itoa(preset.Version),
		})
	}
	return Table{Columns: []string{"id", "name", "default", "locks", "version"}, Rows: rows}
}
