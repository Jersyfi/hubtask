// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"os"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The calendar feeds on the command line (D-08, D-09).
//
// This group is the one that handles a credential, so it is the one with a rule of its own: the
// token is printed exactly once, by the command that mints it, and never again. `ls` shows that a
// feed exists and what it serves; it cannot show the token, because the server does not keep one.

const calendarFeedsPath = "/integrations/calendar-feeds"

func calendarGroup() group {
	return group{
		name:    "calendar",
		summary: "subscriptions - a view, as a calendar",
		commands: []command{
			{
				name:    "ls",
				usage:   "",
				summary: "list the caller's own feeds",
				run:     calendarList,
			},
			{
				name:    "mint",
				usage:   "--view <id>",
				summary: "mint a feed over a view, and print its URL once",
				run:     calendarMint,
			},
			{
				name:    "fetch",
				usage:   "--url <feed-url> [--out <file>]",
				summary: "fetch a feed the way a calendar client would",
				run:     calendarFetch,
			},
			{
				name:    "revoke",
				usage:   "<feed-id>",
				summary: "stop a feed; every fetch after this answers nothing",
				run:     calendarRevoke,
			},
		},
	}
}

func calendarList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "calendar", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var feeds []openapi.CalendarFeed
	if err := client.Get(ctx, calendarFeedsPath, nil, &feeds); err != nil {
		return err
	}
	return cli.Emit(feeds, calendarTable(feeds))
}

func calendarMint(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "calendar", "mint", "--view <id>")
	view := flags.String("view", "", "the saved view the feed serves")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *view == "" {
		return usagef("a feed needs --view")
	}

	target, err := cli.parseID("--view", *view)
	if err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var minted openapi.CalendarFeedSecret
	if err := client.Post(ctx, calendarFeedsPath,
		openapi.CalendarFeedCreate{ViewId: target}, &minted); err != nil {
		return err
	}

	// The URL is the credential, and this is the only moment it exists outside the caller's
	// hands. Under --json it goes to standard output, because a script that just asked for a
	// feed has to be able to read it; in a table it goes to standard output too, with the warning
	// beside it on standard error - a person needs to see it, and the warning must not end up in
	// whatever they pipe it into.
	if cli.JSON {
		return cli.Emit(minted, Table{})
	}
	if err := cli.Emit(minted, calendarTable([]openapi.CalendarFeed{feedOf(minted)})); err != nil {
		return err
	}
	printf(cli.Out, "%s\n", minted.Url)
	printf(cli.Err,
		"that URL is the whole credential and is shown once: store it, and revoke the feed if it leaks\n")
	return nil
}

// calendarFetch reads a feed the way a calendar client does: the URL and nothing else, with no
// Authorization header. It exists so that "does this feed actually work" is one command rather
// than a detour through curl - and so the end-to-end session can prove the document is valid ICS.
func calendarFetch(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "calendar", "fetch", "--url <feed-url> [--out <file>]")
	target := flags.String("url", "", "the feed URL, as `calendar mint` printed it")
	out := flags.String("out", "", "write the calendar here instead of to standard output")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *target == "" {
		return usagef("a fetch needs --url")
	}
	if !strings.HasPrefix(*target, "http://") && !strings.HasPrefix(*target, "https://") {
		return usagef("--url takes the address `calendar mint` printed, including the scheme")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	document, err := client.FetchPublic(ctx, *target)
	if err != nil {
		return err
	}

	if *out == "" {
		_, _ = cli.Out.Write(document)
		return nil
	}
	if err := os.WriteFile(*out, document, 0o600); err != nil {
		return err
	}
	printf(cli.Err, "written: %s (%d bytes)\n", *out, len(document))
	return nil
}

func calendarRevoke(ctx context.Context, cli *CLI, args []string) error {
	const usage = "calendar revoke <feed-id>"
	feed, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "calendar", "revoke", "<feed-id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, calendarFeedsPath+"/"+feed.String(), ""); err != nil {
		return err
	}
	printf(cli.Err, "feed revoked: %s (every fetch from now on answers nothing)\n", feed)
	return nil
}

// feedOf drops the credential from a minted feed so the table can be built from the same shape
// the list uses.
func feedOf(minted openapi.CalendarFeedSecret) openapi.CalendarFeed {
	return openapi.CalendarFeed{
		Id: minted.Id, AccountId: minted.AccountId, ViewId: minted.ViewId,
		CreatedAt: minted.CreatedAt, RevokedAt: minted.RevokedAt,
	}
}

func calendarTable(feeds []openapi.CalendarFeed) Table {
	rows := make([][]string, 0, len(feeds))
	for _, feed := range feeds {
		rows = append(rows, []string{
			feed.Id.String(),
			id(feed.ViewId),
			shortTime(&feed.CreatedAt),
			shortTime(feed.RevokedAt),
		})
	}
	return Table{Columns: []string{"id", "view", "created", "revoked"}, Rows: rows}
}
