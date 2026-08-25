// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strconv"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const itemsPath = "/items"

func itemGroup() group {
	return group{
		name:    "item",
		summary: "the work itself - tasks, work packages and activities",
		commands: []command{
			{
				name:    "ls",
				usage:   "--collection <id> [--parent <id>]",
				summary: "list the entries of one level, in their manual order",
				run:     itemList,
			},
			{
				name:    "create",
				usage:   "--collection <id> --type TASK|WORK_PACKAGE|ACTIVITY --title <title>",
				summary: "create an entry",
				run:     itemCreate,
			},
			{
				name:    "complete",
				usage:   "<id> [--cascade]",
				summary: "mark an entry done",
				run:     itemComplete,
			},
			{
				name:    "move",
				usage:   "<id> [--parent <id>] [--collection <id>] [--before <id>]",
				summary: "move an entry, and its subtree with it",
				run:     itemMove,
			},
			{
				name:    "assign",
				usage:   "<id> --account <id>",
				summary: "hand an entry to an account",
				run:     itemAssign,
			},
			{
				name:    "unassign",
				usage:   "<id>",
				summary: "take the assignee off an entry",
				run:     itemUnassign,
			},
			{
				name:    "rm",
				usage:   "<id> [--expect-version <n>]",
				summary: "move an entry to the trash",
				run:     itemTrash,
			},
		},
	}
}

func itemList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "item", "ls", "--collection <id> [--parent <id>]")
	collection := flags.String("collection", "", "the collection to list (required)")
	parent := flags.String("parent", "", "list the children of this entry instead of the top level")
	includeArchived := flags.Bool("include-archived", false, "include archived entries")
	size := flags.Int("size", 0, "how many entries per page (the server decides when unset)")
	cursor := flags.String("cursor", "", "continue the previous page")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *collection == "" {
		return usagef("say which collection to list: --collection <id>")
	}

	query := url.Values{}
	for _, filter := range []struct{ flag, name, value string }{
		{"--collection", "collection_id", *collection},
		{"--parent", "parent_id", *parent},
	} {
		if filter.value == "" {
			continue
		}
		parsed, err := cli.parseID(filter.flag, filter.value)
		if err != nil {
			return err
		}
		query.Set(filter.name, parsed.String())
	}
	if *includeArchived {
		query.Set("include_archived", "true")
	}
	if *size > 0 {
		query.Set("size", strconv.Itoa(*size))
	}
	if *cursor != "" {
		query.Set("cursor", *cursor)
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var page openapi.WorkItemPage
	if err := client.Get(ctx, itemsPath, query, &page); err != nil {
		return err
	}

	if err := cli.Emit(page, itemTable(page.Data)); err != nil {
		return err
	}
	cli.reportMore(page.Page)
	return nil
}

func itemCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "item", "create", "--collection <id> --type TASK|WORK_PACKAGE|ACTIVITY --title <title>")
	collection := flags.String("collection", "", "the collection it goes into")
	kind := flags.String("type", "", "TASK, WORK_PACKAGE or ACTIVITY")
	title := flags.String("title", "", "what it is called")
	parent := flags.String("parent", "", "the entry it sits inside")
	notes := flags.String("notes", "", "the notes body")
	before := flags.String("before", "", "the sibling to place it before; appended when absent")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *title == "" || *kind == "" {
		return usagef("an entry needs --type and --title")
	}
	if *collection == "" && *parent == "" {
		return usagef("say where the entry goes: --collection <id>, or --parent <id> for a child")
	}

	itemType, err := cli.itemType(*kind)
	if err != nil {
		return err
	}
	body := openapi.WorkItemCreate{Type: itemType, Title: *title, Notes: optional(*notes)}
	for _, reference := range []struct {
		flag, value string
		target      **openapitypes.UUID
	}{
		{"--collection", *collection, &body.CollectionId},
		{"--parent", *parent, &body.ParentId},
		{"--before", *before, &body.BeforeItemId},
	} {
		if reference.value == "" {
			continue
		}
		parsed, err := cli.parseID(reference.flag, reference.value)
		if err != nil {
			return err
		}
		*reference.target = &parsed
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.WorkItem
	if err := client.Post(ctx, itemsPath, body, &created); err != nil {
		return err
	}
	return cli.Emit(created, itemTable([]openapi.WorkItem{created}))
}

func itemComplete(ctx context.Context, cli *CLI, args []string) error {
	const usage = "item complete <id> [--cascade]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "item", "complete", "<id> [--cascade]")
	cascade := flags.Bool("cascade", false, "complete everything under it as well")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var completed openapi.WorkItem
	body := openapi.CompleteWorkItemJSONBody{CascadeChildren: cascade}
	if err := client.Post(ctx, itemsPath+"/"+item.String()+":complete", body, &completed); err != nil {
		return err
	}
	return cli.Emit(completed, itemTable([]openapi.WorkItem{completed}))
}

func itemMove(ctx context.Context, cli *CLI, args []string) error {
	const usage = "item move <id> [--parent <id>] [--collection <id>] [--before <id>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "item", "move", "<id> [--parent <id>] [--collection <id>] [--before <id>]")
	parent := flags.String("parent", "", "the entry to move it under")
	collection := flags.String("collection", "", "the collection to move it into")
	bucket := flags.String("bucket", "", "the bucket at the destination")
	before := flags.String("before", "", "the sibling to place it before; appended when absent")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *parent == "" && *collection == "" {
		return usagef("say where to move it: --parent <id>, --collection <id>, or both")
	}

	body := openapi.MoveWorkItemJSONBody{}
	for _, reference := range []struct {
		flag, value string
		target      **openapitypes.UUID
	}{
		{"--parent", *parent, &body.TargetParentId},
		{"--collection", *collection, &body.TargetCollectionId},
		{"--bucket", *bucket, &body.TargetBucketId},
		{"--before", *before, &body.BeforeItemId},
	} {
		if reference.value == "" {
			continue
		}
		parsed, err := cli.parseID(reference.flag, reference.value)
		if err != nil {
			return err
		}
		*reference.target = &parsed
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var result openapi.MoveResult
	if err := client.Post(ctx, itemsPath+"/"+item.String()+":move", body, &result); err != nil {
		return err
	}

	if err := cli.Emit(result, itemTable([]openapi.WorkItem{result.Item})); err != nil {
		return err
	}
	// Invariant I-W6: what could not be resolved in the destination is reported, never dropped in
	// silence. Under --json it is already in the payload; in a table it would otherwise be lost.
	if !cli.JSON {
		for _, dropped := range result.DroppedReferences {
			reason, known := cli.Catalogue.Message(dropped.Code, nil)
			if !known {
				reason = dropped.Code
			}
			printf(cli.Err, "dropped in the destination: %s %s - %s\n", dropped.Kind, dropped.Id, reason)
		}
	}
	return nil
}

func itemAssign(ctx context.Context, cli *CLI, args []string) error {
	const usage = "item assign <id> --account <id>"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "item", "assign", "<id> --account <id>")
	account := flags.String("account", "", "the account to hand the entry to")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *account == "" {
		return usagef("say who to assign: --account <id>")
	}
	assignee, err := cli.parseID("--account", *account)
	if err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var assigned openapi.WorkItem
	body := openapi.Assignment{AccountId: assignee}
	if err := client.Post(ctx, itemsPath+"/"+item.String()+":assign", body, &assigned); err != nil {
		return err
	}
	return cli.Emit(assigned, itemTable([]openapi.WorkItem{assigned}))
}

func itemUnassign(ctx context.Context, cli *CLI, args []string) error {
	const usage = "item unassign <id>"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "item", "unassign", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var unassigned openapi.WorkItem
	if err := client.Post(ctx, itemsPath+"/"+item.String()+":unassign", nil, &unassigned); err != nil {
		return err
	}
	return cli.Emit(unassigned, itemTable([]openapi.WorkItem{unassigned}))
}

func itemTrash(ctx context.Context, cli *CLI, args []string) error {
	const usage = "item rm <id> [--expect-version <n>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "item", "rm", "<id> [--expect-version <n>]")
	// Named for what it does rather than --version, which is the flag that prints the build.
	expected := flags.Int("expect-version", 0, "refuse unless the entry is still at this version")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	ifMatch := ""
	if *expected > 0 {
		ifMatch = `"` + strconv.Itoa(*expected) + `"`
	}
	if err := client.Delete(ctx, itemsPath+"/"+item.String(), ifMatch); err != nil {
		return err
	}
	// A deletion has no payload. The confirmation goes to standard error, so that
	// `hubctl --json item rm` writes nothing at all to a pipe rather than an invented document.
	printf(cli.Err, "moved to the trash: %s\n", item)
	return nil
}

// itemType checks what somebody typed against the enum the specification declares.
func (cli *CLI) itemType(raw string) (openapi.ItemType, error) {
	itemType := openapi.ItemType(raw)
	if !itemType.Valid() {
		message, _ := cli.Catalogue.Message("items.type_unknown", map[string]string{"value": raw})
		return "", usageError{error: errorString(message)}
	}
	return itemType, nil
}

func itemTable(items []openapi.WorkItem) Table {
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.Id.String(),
			string(item.Type),
			item.Title,
			yesNo(item.Completion.IsCompleted),
			id(item.AssigneeId),
			strconv.Itoa(item.Version),
		})
	}
	return Table{Columns: []string{"id", "type", "title", "done", "assignee", "version"}, Rows: rows}
}
