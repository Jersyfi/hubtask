// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"flag"
	"net/url"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The workspace's entry types (UC-FLW-01 check 7): listed, created, changed, archived and deleted
// through the same REST use cases the screen, MCP and the rule engine reach, and an entry's type
// changed through its own operation.

const entryTypesPath = "/entry-types"

func entryTypeGroup() group {
	return group{
		name:    "type",
		summary: "the workspace's entry types: a bug, a feature, a record",
		commands: []command{
			{name: "ls", summary: "every type, the system types included", run: entryTypeList},
			{name: "create", summary: "add a type on a level", run: entryTypeCreate},
			{name: "set", summary: "rename, recolour, reorder or change a type's behaviours", run: entryTypeChange},
			{name: "archive", summary: "stop offering a type; its entries keep it", run: entryTypeArchive(true)},
			{name: "unarchive", summary: "offer an archived type again", run: entryTypeArchive(false)},
			{name: "rm", summary: "delete a type nothing uses", run: entryTypeDelete},
			{name: "assign", summary: "change an entry's type", run: entryTypeAssign},
		},
	}
}

func entryTypeList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "type", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var list openapi.EntryTypeList
	if err := client.Get(ctx, entryTypesPath, nil, &list); err != nil {
		return err
	}
	return cli.Emit(list, entryTypeTable(list.Data))
}

// behaviourFlags are the five switches create and set share. Each is sent only where it was given,
// so that set changes nothing the caller did not name and create leaves the server's defaults.
type behaviourFlags struct {
	completable, workflow, record, schedulable, estimable *bool
	children                                              *string
}

func addBehaviourFlags(flags *flag.FlagSet) behaviourFlags {
	return behaviourFlags{
		completable: flags.Bool("completable", true, "its entries can be completed"),
		workflow:    flags.Bool("workflow", false, "its entries move through a workflow"),
		record:      flags.Bool("record", false, "kept rather than done; never completable"),
		schedulable: flags.Bool("schedulable", true, "its entries carry a due date, reminders and a recurrence"),
		estimable:   flags.Bool("estimable", false, "its entries carry an estimate"),
		children:    flags.String("children", "", "comma-separated keys of the types allowed underneath; \"-\" allows every one"),
	}
}

// given answers the flags the caller named, so that a default is never mistaken for a choice.
func given(flags *flag.FlagSet) map[string]bool {
	named := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { named[f.Name] = true })
	return named
}

func pick[T any](named map[string]bool, name string, value *T) *T {
	if named[name] {
		return value
	}
	return nil
}

func childKeys(value string) *[]string {
	keys := []string{}
	if value != "-" {
		for _, key := range strings.Split(value, ",") {
			if key = strings.TrimSpace(key); key != "" {
				keys = append(keys, key)
			}
		}
	}
	return &keys
}

func entryTypeCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "type", "create",
		"--key <key> --level TASK|WORK_PACKAGE|ACTIVITY --name <name> --icon <icon> --color <token> [--record] [--schedulable=false] [--position <n>]")
	key := flags.String("key", "", "the stable name every door and a query use")
	level := flags.String("level", "", "TASK, WORK_PACKAGE or ACTIVITY")
	name := flags.String("name", "", "what the type is called")
	icon := flags.String("icon", "", "an icon of the design system's set")
	color := flags.String("color", "", "one of the label colour tokens")
	position := flags.Int("position", 0, "its place among the level's types")
	behaviours := addBehaviourFlags(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *key == "" || *level == "" || *name == "" || *icon == "" || *color == "" {
		return usagef("a type needs --key, --level, --name, --icon and --color")
	}
	itemType, err := cli.itemType(*level)
	if err != nil {
		return err
	}
	named := given(flags)
	body := openapi.EntryTypeCreate{
		Key: *key, Level: itemType, Name: *name, Icon: *icon, ColorToken: *color,
		Position:    pick(named, "position", position),
		Completable: pick(named, "completable", behaviours.completable),
		Workflow:    pick(named, "workflow", behaviours.workflow),
		Record:      pick(named, "record", behaviours.record),
		Schedulable: pick(named, "schedulable", behaviours.schedulable),
		Estimable:   pick(named, "estimable", behaviours.estimable),
	}
	if named["children"] {
		body.AllowedChildren = childKeys(*behaviours.children)
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.EntryType
	if err := client.Post(ctx, entryTypesPath, body, &created); err != nil {
		return err
	}
	return cli.Emit(created, entryTypeTable([]openapi.EntryType{created}))
}

func entryTypeChange(ctx context.Context, cli *CLI, args []string) error {
	const usage = "type set <key> [--name <name>] [--icon <icon>] [--color <token>] [--position <n>] [--expect-version <n>]"
	key, rest, err := takeKey(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "type", "set", "<key> [--name <name>] [--icon <icon>] [--color <token>] [--position <n>]")
	name := flags.String("name", "", "the new name")
	icon := flags.String("icon", "", "an icon of the design system's set")
	color := flags.String("color", "", "one of the label colour tokens")
	position := flags.Int("position", 0, "its place among the level's types")
	expected := flags.Int("expect-version", 0, "refuse unless the type is still at this version")
	behaviours := addBehaviourFlags(flags)
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	named := given(flags)
	body := openapi.EntryTypeChange{
		Name: pick(named, "name", name), Icon: pick(named, "icon", icon),
		ColorToken: pick(named, "color", color), Position: pick(named, "position", position),
		Completable: pick(named, "completable", behaviours.completable),
		Workflow:    pick(named, "workflow", behaviours.workflow),
		Record:      pick(named, "record", behaviours.record),
		Schedulable: pick(named, "schedulable", behaviours.schedulable),
		Estimable:   pick(named, "estimable", behaviours.estimable),
	}
	if named["children"] {
		body.AllowedChildren = childKeys(*behaviours.children)
	}
	return patchEntryType(ctx, cli, key, body, *expected)
}

func entryTypeArchive(archived bool) func(context.Context, *CLI, []string) error {
	verb := "archive"
	if !archived {
		verb = "unarchive"
	}
	return func(ctx context.Context, cli *CLI, args []string) error {
		usage := "type " + verb + " <key> [--expect-version <n>]"
		key, rest, err := takeKey(args, usage)
		if err != nil {
			return err
		}
		flags := commandFlags(cli, "type", verb, "<key> [--expect-version <n>]")
		expected := flags.Int("expect-version", 0, "refuse unless the type is still at this version")
		if err := parseOnlyFlags(flags, rest, usage); err != nil {
			return err
		}
		return patchEntryType(ctx, cli, key, openapi.EntryTypeChange{Archived: &archived}, *expected)
	}
}

func patchEntryType(ctx context.Context, cli *CLI, key string, body openapi.EntryTypeChange, expected int) error {
	client, err := cli.client()
	if err != nil {
		return err
	}
	var changed openapi.EntryType
	if err := client.PatchVersioned(ctx, entryTypesPath+"/"+url.PathEscape(key), body, versionTag(expected), &changed); err != nil {
		return err
	}
	return cli.Emit(changed, entryTypeTable([]openapi.EntryType{changed}))
}

func entryTypeDelete(ctx context.Context, cli *CLI, args []string) error {
	const usage = "type rm <key> [--expect-version <n>]"
	key, rest, err := takeKey(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "type", "rm", "<key> [--expect-version <n>]")
	expected := flags.Int("expect-version", 0, "refuse unless the type is still at this version")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	// A type in use is refused by the server with entry_types.in_use: it can only be archived.
	if err := client.Delete(ctx, entryTypesPath+"/"+url.PathEscape(key), versionTag(*expected)); err != nil {
		return err
	}
	printf(cli.Err, "deleted: %s\n", key)
	return nil
}

func entryTypeAssign(ctx context.Context, cli *CLI, args []string) error {
	const usage = "type assign <item-id> <key> [--expect-version <n>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	key, rest, err := takeKey(rest, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "type", "assign", "<item-id> <key> [--expect-version <n>]")
	expected := flags.Int("expect-version", 0, "refuse unless the entry is still at this version")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var changed openapi.WorkItem
	body := openapi.SetItemEntryTypeJSONBody{EntryType: key}
	if err := client.PutVersioned(ctx, itemsPath+"/"+item.String()+"/entry-type", body, versionTag(*expected), &changed); err != nil {
		return err
	}
	return cli.Emit(changed, itemTable([]openapi.WorkItem{changed}))
}

// takeKey reads the type key that leads the arguments, before any flag.
func takeKey(args []string, usage string) (string, []string, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") || strings.TrimSpace(args[0]) == "" {
		return "", nil, usagef("usage: hubctl %s", usage)
	}
	return args[0], args[1:], nil
}

// versionTag is the If-Match of a version the caller read; none where zero.
func versionTag(version int) string {
	if version <= 0 {
		return ""
	}
	return `"` + strconv.Itoa(version) + `"`
}

func entryTypeTable(types []openapi.EntryType) Table {
	rows := make([][]string, 0, len(types))
	for _, t := range types {
		name := ""
		if t.Name != nil {
			name = *t.Name
		}
		var behaviours []string
		for _, b := range []struct {
			on   bool
			name string
		}{
			{t.Completable, "completable"}, {t.Workflow, "workflow"}, {t.Record, "record"},
			{t.Schedulable, "schedulable"}, {t.Estimable, "estimable"},
		} {
			if b.on {
				behaviours = append(behaviours, b.name)
			}
		}
		rows = append(rows, []string{
			t.Key, string(t.Level), name, t.Icon, t.ColorToken, strings.Join(behaviours, ","),
			yesNo(&t.System), yesNo(&t.Archived), strconv.Itoa(t.Version),
		})
	}
	return Table{
		Columns: []string{"key", "level", "name", "icon", "colour", "behaviours", "system", "archived", "version"},
		Rows:    rows,
	}
}
