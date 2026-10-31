// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strconv"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The inbox as a person meets it: what arrived, what it became, and what was decided
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
				name:    "show",
				usage:   "<id>",
				summary: "one entry, whatever became of it",
				run:     jumbleShow,
			},
			{
				name:    "submit",
				usage:   "[--subject <s>] [--body <b>] [--channel API|QUICK_CAPTURE] [--attachment <id>…]",
				summary: "put something in the jumble over a near channel",
				run:     jumbleSubmit,
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

// jumbleShow is `jumble show <id>`: one entry in the listing's columns, the body left to `--json`
// for the listing's reason.
func jumbleShow(ctx context.Context, cli *CLI, args []string) error {
	entryID, err := cli.onlyID(args, "jumble show <id>")
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var entry openapi.JumbleEntry
	if err := client.Get(ctx, jumblePath+"/"+entryID.String(), nil, &entry); err != nil {
		return err
	}
	return cli.Emit(entry, jumbleTable([]openapi.JumbleEntry{entry}))
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
