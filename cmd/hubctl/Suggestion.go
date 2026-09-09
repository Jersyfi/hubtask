// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// What AI has proposed, and what became of it (J-05, J-16).
//
// Nothing here has changed anything: a suggestion is a record with a name on it - the model, the
// prompt and its version, the moment - and it becomes a change only when somebody accepts it, as
// their own write with their own rights. That is why `accept` is a command of its own rather than
// a flag on the listing.
//
// The listing prints the provenance and not the payload. A payload is model output about somebody's
// content, which is a terminal's worst case and a wide column besides; `--json` is where the whole
// of one is available to something that can handle it.

const suggestionPath = "/suggestions"

func suggestionGroup() group {
	return group{
		name:    "suggestion",
		summary: "what AI proposed about an entry, and accepting or turning it down",
		commands: []command{
			{
				name:    "ls",
				usage:   "--target <id> [--target-type ITEM|JUMBLE_ENTRY] [--status <s>] [--cursor <c>] [--size <n>]",
				summary: "the suggestions standing against one entry, newest first",
				run:     suggestionList,
			},
			{
				name:    "ask",
				usage:   "--target <id> [--target-type ITEM|JUMBLE_ENTRY]",
				summary: "ask the workspace's provider to propose something about an entry",
				run:     suggestionAsk,
			},
			{
				name:    "accept",
				usage:   "<id>",
				summary: "apply it as your own write, with your own rights",
				run:     suggestionAccept,
			},
			{
				name:    "dismiss",
				usage:   "<id>",
				summary: "turn it down - a state, not a deletion",
				run:     suggestionDismiss,
			},
		},
	}
}

func suggestionList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "suggestion", "ls",
		"--target <id> [--target-type <t>] [--status <s>] [--cursor <c>] [--size <n>]")
	target := flags.String("target", "", "the entry the suggestions are about")
	targetType := flags.String("target-type", "ITEM", "ITEM or JUMBLE_ENTRY")
	status := flags.String("status", "", "PROPOSED, ACCEPTED or DISMISSED; unset answers what is still standing")
	cursor := flags.String("cursor", "", "continue the previous page")
	size := flags.Int("size", 0, "how many at most")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *target == "" {
		return usagef("hubctl suggestion ls needs --target")
	}
	targetID, err := cli.parseID("--target", *target)
	if err != nil {
		return err
	}

	query := url.Values{"target_type": {*targetType}, "target_id": {targetID.String()}}
	if *status != "" {
		query.Set("status", *status)
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
	var page openapi.SuggestionPage
	if err := client.Get(ctx, suggestionPath, query, &page); err != nil {
		return err
	}
	if err := cli.Emit(page, suggestionTable(page.Items)); err != nil {
		return err
	}
	// This page is flat rather than nested, unlike most - so the continuation is assembled here
	// rather than passed through.
	cli.reportMore(openapi.PageInfo{HasMore: page.HasMore, NextCursor: page.NextCursor})
	return nil
}

// suggestionAsk asks, and says so: the answer is `202` and a suggestion appears under
// `suggestion ls` when the provider has answered.
//
// The route differs by target because the two asks are different use cases - a jumble entry is
// asked what it should *become*, an entry what its fields should be - and this dispatches rather
// than making the caller know which path is which. What it does not do is wait: an AI call reaches
// somebody else's machine, so it never sits in a request, and a client that blocked here would be
// inventing a synchronous shape the API deliberately does not have.
func suggestionAsk(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "suggestion", "ask", "--target <id> [--target-type <t>]")
	target := flags.String("target", "", "the entry to ask about")
	targetType := flags.String("target-type", "ITEM", "ITEM or JUMBLE_ENTRY")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *target == "" {
		return usagef("hubctl suggestion ask needs --target")
	}
	targetID, err := cli.parseID("--target", *target)
	if err != nil {
		return err
	}

	var path string
	switch strings.ToUpper(*targetType) {
	case "JUMBLE_ENTRY":
		path = jumblePath + "/" + targetID.String() + ":suggest"
	case "ITEM":
		path = "/items/" + targetID.String() + ":suggest-fields"
	default:
		return usagef("--target-type takes ITEM or JUMBLE_ENTRY, not %q", *targetType)
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Post(ctx, path, nil, nil); err != nil {
		return err
	}
	if !cli.JSON {
		printf(cli.Err,
			"asked; the suggestion appears under `hubctl suggestion ls --target %s` when the provider answers\n",
			targetID)
	}
	return nil
}

func suggestionAccept(ctx context.Context, cli *CLI, args []string) error {
	return decideSuggestion(ctx, cli, args, "accept")
}

func suggestionDismiss(ctx context.Context, cli *CLI, args []string) error {
	return decideSuggestion(ctx, cli, args, "dismiss")
}

// decideSuggestion is both decisions, because they differ in one word on the wire and in nothing
// here - and two copies would be two places for the identifier parsing to drift.
func decideSuggestion(ctx context.Context, cli *CLI, args []string, verb string) error {
	id, err := cli.onlyID(args, "suggestion "+verb+" <id>")
	if err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var decided openapi.Suggestion
	if err := client.Post(ctx,
		suggestionPath+"/"+id.String()+":"+verb, nil, &decided); err != nil {
		return err
	}
	return cli.Emit(decided, suggestionTable([]openapi.Suggestion{decided}))
}

// suggestionTable prints where a proposal came from rather than what it says: the model, the prompt
// and its version are what make a suggestion traceable a year later (ai-first.md §2), and the
// payload is model output about somebody's content.
func suggestionTable(suggestions []openapi.Suggestion) Table {
	rows := make([][]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		rows = append(rows, []string{
			suggestion.Id.String(),
			string(suggestion.Kind),
			string(suggestion.Status),
			suggestion.Model,
			suggestion.PromptId + " " + suggestion.PromptVersion,
			shortTime(&suggestion.ProducedAt),
			shortTime(suggestion.DecidedAt),
		})
	}
	return Table{
		Columns: []string{"id", "kind", "status", "model", "prompt", "produced", "decided"},
		Rows:    rows,
	}
}
