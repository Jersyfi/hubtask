// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strconv"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const containersPath = "/containers"

func containerGroup() group {
	return group{
		name:    "container",
		summary: "hubs and collections - the structure work sits in",
		commands: []command{
			{
				name:    "ls",
				usage:   "[--parent <id>] [--type HUB|COLLECTION]",
				summary: "list hubs and collections",
				run:     containerList,
			},
			{
				name:    "create",
				usage:   "--type HUB|COLLECTION --name <name> [--parent <id>]",
				summary: "create a hub or a collection",
				run:     containerCreate,
			},
		},
	}
}

func containerList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "container", "ls", "[--parent <id>] [--type HUB|COLLECTION]")
	parent := flags.String("parent", "", "list what sits directly in this hub")
	kind := flags.String("type", "", "HUB or COLLECTION")
	includeArchived := flags.Bool("include-archived", false, "include archived entries")
	size := flags.Int("size", 0, "how many entries per page (the server decides when unset)")
	cursor := flags.String("cursor", "", "continue the previous page")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	query := url.Values{}
	if *parent != "" {
		parsed, err := cli.parseID("--parent", *parent)
		if err != nil {
			return err
		}
		query.Set("parent_id", parsed.String())
	}
	if *kind != "" {
		containerType, err := cli.containerType(*kind)
		if err != nil {
			return err
		}
		query.Set("type", string(containerType))
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
	var page openapi.ContainerPage
	if err := client.Get(ctx, containersPath, query, &page); err != nil {
		return err
	}

	if err := cli.Emit(page, containerTable(page.Data)); err != nil {
		return err
	}
	cli.reportMore(page.Page)
	return nil
}

func containerCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "container", "create", "--type HUB|COLLECTION --name <name> [--parent <id>]")
	kind := flags.String("type", "", "HUB or COLLECTION")
	name := flags.String("name", "", "what it is called")
	parent := flags.String("parent", "", "the hub a collection goes into")
	description := flags.String("description", "", "a longer description")
	icon := flags.String("icon", "", "an icon token")
	colour := flags.String("color", "", "a colour token")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *kind == "" || *name == "" {
		return usagef("a container needs --type and --name")
	}

	containerType, err := cli.containerType(*kind)
	if err != nil {
		return err
	}
	body := openapi.ContainerCreate{Type: containerType, Name: *name}
	if *parent != "" {
		parsed, err := cli.parseID("--parent", *parent)
		if err != nil {
			return err
		}
		body.ParentId = &parsed
	}
	body.Description, body.Icon, body.ColorToken = optional(*description), optional(*icon), optional(*colour)

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.Container
	if err := client.Post(ctx, containersPath, body, &created); err != nil {
		return err
	}
	return cli.Emit(created, containerTable([]openapi.Container{created}))
}

// containerType checks what somebody typed against the enum the specification declares, so that a
// misspelling is refused here with the catalogue's own sentence rather than after a round trip.
func (cli *CLI) containerType(raw string) (openapi.ContainerType, error) {
	containerType := openapi.ContainerType(raw)
	if !containerType.Valid() {
		message, _ := cli.Catalogue.Message("containers.type_unknown", map[string]string{"value": raw})
		return "", usageError{error: errorString(message)}
	}
	return containerType, nil
}

func containerTable(containers []openapi.Container) Table {
	rows := make([][]string, 0, len(containers))
	for _, container := range containers {
		rows = append(rows, []string{
			container.Id.String(),
			string(container.Type),
			container.Name,
			id(container.ParentId),
			yesNo(container.EffectiveArchived),
		})
	}
	return Table{Columns: []string{"id", "type", "name", "parent", "archived"}, Rows: rows}
}

// reportMore says how to get the rest, on standard error so that it never lands in a pipe. Under
// --json the cursor is in the payload already, where a script reads it.
func (cli *CLI) reportMore(page openapi.PageInfo) {
	if cli.JSON || !page.HasMore || page.NextCursor == nil {
		return
	}
	printf(cli.Err, "more entries follow; continue with --cursor %s\n", *page.NextCursor)
}
