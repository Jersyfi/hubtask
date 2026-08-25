// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const searchPath = "/search"

func searchGroup() group {
	return group{
		name:    "search",
		summary: "find entries by the words in their titles and notes, best match first",
		usage: "<words> [--container <id>] [--language <bcp47>]" +
			" [--include-archived] [--include-trashed] [--size <n>] [--cursor <c>]",
		run: searchRun,
	}
}

func searchRun(ctx context.Context, cli *CLI, args []string) error {
	// The words come first, before any flag, for the reason an identifier does: the flag package
	// stops at the first argument that is not a flag. Everything up to the first flag is the
	// query, so `hubctl search buy milk --container X` asks for both words.
	words := 0
	for words < len(args) && !strings.HasPrefix(args[words], "-") {
		words++
	}
	query := strings.Join(args[:words], " ")

	flags := commandFlags(cli, "search", "",
		"<words> [--container <id>] [--language <bcp47>] [--include-archived] [--include-trashed] [--size <n>] [--cursor <c>]")
	container := flags.String("container", "", "the hub or collection to search in; unset searches everything visible")
	language := flags.String("language", "", "the language the words are in, as BCP-47; unset takes the account's locale")
	includeArchived := flags.Bool("include-archived", false, "find archived entries too")
	includeTrashed := flags.Bool("include-trashed", false, "find trashed entries too")
	size := flags.Int("size", 0, "how many hits per page (the server decides when unset)")
	cursor := flags.String("cursor", "", "continue the previous page")
	if err := parseCommand(flags, args[words:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return usagef("the words come before the flags: hubctl search %s", flags.Arg(0))
	}
	if query == "" {
		message, _ := cli.Catalogue.Message("search.words_required", nil)
		return usageError{error: errorString(message)}
	}

	body := openapi.ItemSearchQuery{Q: query}
	if *container != "" {
		parsed, err := cli.parseID("--container", *container)
		if err != nil {
			return err
		}
		body.ContainerId = &parsed
	}
	body.Language = optional(*language)
	if *includeArchived {
		body.IncludeArchived = includeArchived
	}
	if *includeTrashed {
		body.IncludeTrashed = includeTrashed
	}
	if *size > 0 || *cursor != "" {
		var page struct {
			Cursor *string `json:"cursor,omitempty"`
			Size   *int    `json:"size,omitempty"`
		}
		if *size > 0 {
			page.Size = size
		}
		page.Cursor = optional(*cursor)
		body.Page = &page
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var hits openapi.WorkItemPage
	if err := client.Post(ctx, searchPath, body, &hits); err != nil {
		return err
	}

	if err := cli.Emit(hits, itemTable(hits.Data)); err != nil {
		return err
	}
	cli.reportMore(hits.Page)
	return nil
}
