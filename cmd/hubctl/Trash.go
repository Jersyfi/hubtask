// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const trashPath = "/trash"

func trashGroup() group {
	return group{
		name:    "trash",
		summary: "what was deleted, and how to get it back",
		commands: []command{
			{
				name:    "restore",
				usage:   "<id> --kind ITEM|CONTAINER",
				summary: "take one deletion back out of the trash, whole",
				run:     trashRestore,
			},
		},
	}
}

// trashRestore needs the kind because the two levels have two endpoints: the trash mixes hubs,
// collections and entries by design, and which of the two restores a row is what `kind` says.
// The listing prints it in a column of its own, so it is a value to copy rather than to know.
func trashRestore(ctx context.Context, cli *CLI, args []string) error {
	const usage = "trash restore <id> --kind ITEM|CONTAINER"
	deleted, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "trash", "restore", "<id> --kind ITEM|CONTAINER")
	kind := flags.String("kind", "", "ITEM or CONTAINER, as the kind column of `hubctl trash ls` says")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	entryKind := openapi.TrashEntryKind(*kind)
	if !entryKind.Valid() {
		return usagef("--kind is ITEM or CONTAINER; `hubctl trash ls` prints it in the kind column")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if entryKind == openapi.TrashEntryKindCONTAINER {
		var restored openapi.Container
		if err := client.Post(ctx, containersPath+"/"+deleted.String()+":restore", nil, &restored); err != nil {
			return err
		}
		return cli.Emit(restored, containerTable([]openapi.Container{restored}))
	}

	var restored openapi.WorkItem
	if err := client.Post(ctx, itemsPath+"/"+deleted.String()+":restore", nil, &restored); err != nil {
		return err
	}
	return cli.Emit(restored, itemTable([]openapi.WorkItem{restored}))
}
