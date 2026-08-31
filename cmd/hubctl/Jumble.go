// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strconv"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The inbox as a person meets it (G-13): what arrived, what it became, and what was decided
// against.
//
// The listing prints the subject and not the body. An entry's raw content is the least trusted
// text in the system and a terminal is where an escape sequence would land: what a listing is for
// is deciding which entry to look at, and `--json` is where the whole of one is available for
// something that can handle it.

const (
	jumblePath = "/jumble/entries"
	intakePath = "/jumble/intake"
)

func jumbleGroup() group {
	return group{
		name:    "jumble",
		summary: "the inbox: what arrived, and what became of it",
		commands: []command{
			{
				name:    "ls",
				usage:   "[--status NEW|PROCESSED|DISMISSED] [--channel <c>] [--cursor <c>] [--size <n>]",
				summary: "the entries, newest first",
				run:     jumbleList,
			},
			{
				name:    "submit",
				usage:   "[--subject <s>] [--body <b>] [--channel API|QUICK_CAPTURE] [--attachment <id>…]",
				summary: "put something in the jumble over a near channel",
				run:     jumbleSubmit,
			},
			{
				name:    "convert",
				usage:   "<id> --collection <id> [--bucket <id>] [--title <t>] [--type TASK|WORK_PACKAGE|ACTIVITY]",
				summary: "turn an entry into work, once",
				run:     jumbleConvert,
			},
			{
				name:    "dismiss",
				usage:   "<id>",
				summary: "decide against it - a state, not a deletion",
				run:     jumbleDismiss,
			},
			{
				name:    "intake",
				usage:   "rotate-token",
				summary: "mint the address the jumble accepts deliveries on - shown once",
				run:     jumbleIntake,
			},
		},
	}
}

func jumbleList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "jumble", "ls",
		"[--status <s>] [--channel <c>] [--cursor <c>] [--size <n>]")
	status := flags.String("status", "", "NEW, PROCESSED or DISMISSED")
	channel := flags.String("channel", "", "EMAIL, WEBHOOK, QUICK_CAPTURE or API")
	cursor := flags.String("cursor", "", "continue the previous page")
	size := flags.Int("size", 0, "how many at most")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	query := url.Values{}
	for name, value := range map[string]string{"status": *status, "channel": *channel} {
		if value != "" {
			query.Set(name, value)
		}
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
	var page openapi.JumbleEntryPage
	if err := client.Get(ctx, jumblePath, query, &page); err != nil {
		return err
	}
	if err := cli.Emit(page, jumbleTable(page.Data)); err != nil {
		return err
	}
	cli.reportMore(page.Page)
	return nil
}

func jumbleSubmit(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "jumble", "submit",
		"[--subject <s>] [--body <b>] [--channel <c>] [--attachment <id>…]")
	subject := flags.String("subject", "", "the short half")
	body := flags.String("body", "", "the long half")
	channel := flags.String("channel", "", "API or QUICK_CAPTURE; the far channels authenticate their own way")
	var attachments stringList
	flags.Var(&attachments, "attachment", "a sealed media object to carry; repeat for several")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *subject == "" && *body == "" && len(attachments) == 0 {
		return usagef("jumble submit needs a subject, a body, or an attachment")
	}

	submit := openapi.JumbleEntrySubmit{}
	if *subject != "" {
		submit.RawSubject = subject
	}
	if *body != "" {
		submit.RawBody = body
	}
	if *channel != "" {
		kind := openapi.JumbleEntrySubmitChannel(*channel)
		submit.Channel = &kind
	}
	if len(attachments) > 0 {
		ids, err := cli.parseIDs("--attachment", joinList(attachments))
		if err != nil {
			return err
		}
		submit.Attachments = &ids
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var entry openapi.JumbleEntry
	if err := client.Post(ctx, jumblePath, submit, &entry); err != nil {
		return err
	}
	return cli.Emit(entry, jumbleTable([]openapi.JumbleEntry{entry}))
}

func jumbleConvert(ctx context.Context, cli *CLI, args []string) error {
	const usage = "jumble convert <id> --collection <id> [--bucket <id>] [--title <t>] [--type <t>]"
	entryID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "jumble", "convert", "<id> --collection <id> [--title <t>]")
	collection := flags.String("collection", "", "where the entry becomes work")
	bucket := flags.String("bucket", "", "the board column, when the destination has a board")
	title := flags.String("title", "", "what the item is called (defaults to the entry's subject)")
	kind := flags.String("type", "", "TASK, WORK_PACKAGE or ACTIVITY")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *collection == "" {
		return usagef("jumble convert needs --collection: an entry becomes work somewhere")
	}

	destination, err := cli.parseID("--collection", *collection)
	if err != nil {
		return err
	}
	convert := openapi.JumbleEntryConvert{CollectionId: destination}
	if *bucket != "" {
		parsed, err := cli.parseID("--bucket", *bucket)
		if err != nil {
			return err
		}
		convert.BucketId = &parsed
	}
	if *title != "" {
		convert.Title = title
	}
	if *kind != "" {
		itemType := openapi.JumbleEntryConvertType(*kind)
		convert.Type = &itemType
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var converted openapi.JumbleEntry
	if err := client.Post(ctx,
		jumblePath+"/"+entryID.String()+":convert", convert, &converted); err != nil {
		return err
	}
	if !cli.JSON && converted.TargetItemId != nil {
		printf(cli.Err, "the item it became: %s\n", converted.TargetItemId.String())
	}
	return cli.Emit(converted, jumbleTable([]openapi.JumbleEntry{converted}))
}

func jumbleDismiss(ctx context.Context, cli *CLI, args []string) error {
	entryID, err := cli.onlyID(args, "jumble dismiss <id>")
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var dismissed openapi.JumbleEntry
	if err := client.Post(ctx, jumblePath+"/"+entryID.String()+":dismiss", nil, &dismissed); err != nil {
		return err
	}
	if !cli.JSON {
		// A state rather than a deletion, and worth saying: somebody dismissing an entry has not
		// destroyed it, and the retention rule is what eventually does (data-retention.md §3).
		printf(cli.Err, "dismissed; the entry stays readable and ages out by retention rule\n")
	}
	return cli.Emit(dismissed, jumbleTable([]openapi.JumbleEntry{dismissed}))
}

// jumbleIntake is `jumble intake rotate-token`: the sub-verb exists because minting and revoking
// are one act - there is one address per workspace, and rotating replaces it in one statement.
func jumbleIntake(ctx context.Context, cli *CLI, args []string) error {
	if len(args) != 1 || args[0] != "rotate-token" {
		return usagef("hubctl jumble intake rotate-token")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var minted openapi.JumbleIntakeToken
	if err := client.Post(ctx, intakePath+":rotate-token", nil, &minted); err != nil {
		return err
	}
	if !cli.JSON {
		printf(cli.Err,
			"that token is the whole credential and is shown once: store it, and rotate again if it leaks\n")
	}
	return cli.Emit(minted, Table{
		Columns: []string{"token", "rotated"},
		Rows:    [][]string{{minted.Token, shortTime(&minted.RotatedAt)}},
	})
}

// jumbleTable prints the subject and not the body: what a listing is for is deciding which entry to
// look at, and the body is the least trusted text in the system landing in a terminal.
func jumbleTable(entries []openapi.JumbleEntry) Table {
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{
			entry.Id.String(),
			string(entry.Channel),
			text(entry.Sender),
			text(entry.RawSubject),
			string(entry.Status),
			strconv.Itoa(len(entry.Attachments)),
			id(entry.TargetItemId),
		})
	}
	return Table{
		Columns: []string{"id", "channel", "from", "subject", "status", "files", "became"},
		Rows:    rows,
	}
}

// joinList is the comma-separated form parseIDs takes. The flag is repeatable and the helper is
// not, so the two meet here rather than in a second parser.
func joinList(values []string) string {
	joined := ""
	for index, value := range values {
		if index > 0 {
			joined += ","
		}
		joined += value
	}
	return joined
}
