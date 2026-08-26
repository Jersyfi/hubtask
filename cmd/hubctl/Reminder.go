// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strconv"
	"strings"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The reminders on the command line (D-02, D-03, D-09).

func reminderGroup() group {
	return group{
		name:    "remind",
		summary: "being told about work before it is late",
		commands: []command{
			{
				name:    "ls",
				usage:   "<item-id>",
				summary: "list what an entry will remind about",
				run:     reminderList,
			},
			{
				name:    "add",
				usage:   "<item-id> --at <offset> [--to <account-ids>] [--channel <names>]",
				summary: "add a reminder to an entry",
				run:     reminderAdd,
			},
			{
				name:    "rm",
				usage:   "<item-id> <reminder-id> [--expect-version <n>]",
				summary: "remove a reminder",
				run:     reminderRemove,
			},
		},
	}
}

// reminderOffset is the one convenience this group adds. The contract spells an offset
// `REL:-PT30M` or `ABS:2026-09-10T09:00:00Z`, and a person typing `--at -PT30M` or
// `--at 2026-09-10T09:00:00Z` means exactly those - so the prefix is filled in rather than
// demanded. A value that already carries one is passed through untouched, because the contract's
// spelling has to stay typeable.
func reminderOffset(raw string) string {
	if strings.HasPrefix(raw, "REL:") || strings.HasPrefix(raw, "ABS:") {
		return raw
	}
	if strings.HasPrefix(raw, "P") || strings.HasPrefix(raw, "-P") {
		return "REL:" + raw
	}
	return "ABS:" + raw
}

func reminderList(ctx context.Context, cli *CLI, args []string) error {
	const usage = "remind ls <item-id>"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "remind", "ls", "<item-id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var reminders []openapi.Reminder
	if err := client.Get(ctx, remindersPath(item), nil, &reminders); err != nil {
		return err
	}
	return cli.Emit(reminders, reminderTable(reminders))
}

func reminderAdd(ctx context.Context, cli *CLI, args []string) error {
	const usage = "remind add <item-id> --at <offset> [--to <account-ids>] [--channel <names>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "remind", "add",
		"<item-id> --at <offset> [--to <account-ids>] [--channel <names>]")
	at := flags.String("at", "",
		"-PT30M for half an hour before the due date, or an RFC 3339 moment for a fixed one")
	to := flags.String("to", "",
		"comma-separated accounts to remind; unset means the assignee and the entry's members")
	channels := flags.String("channel", "", "comma-separated channels; unset means EMAIL")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *at == "" {
		return usagef("a reminder needs --at")
	}

	body := openapi.ReminderInput{OffsetSpec: reminderOffset(*at)}
	if *to != "" {
		recipients, err := cli.parseIDs("--to", *to)
		if err != nil {
			return err
		}
		body.Recipients = &recipients
	}
	if *channels != "" {
		named := make([]openapi.ReminderChannel, 0, 1)
		for _, raw := range strings.Split(*channels, ",") {
			named = append(named, openapi.ReminderChannel(strings.TrimSpace(raw)))
		}
		body.Channels = &named
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.Reminder
	if err := client.Post(ctx, remindersPath(item), body, &created); err != nil {
		return err
	}
	return cli.Emit(created, reminderTable([]openapi.Reminder{created}))
}

func reminderRemove(ctx context.Context, cli *CLI, args []string) error {
	const usage = "remind rm <item-id> <reminder-id> [--expect-version <n>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	reminder, rest, err := cli.takeID(rest, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "remind", "rm", "<item-id> <reminder-id> [--expect-version <n>]")
	expected := flags.Int("expect-version", 0, "refuse unless the reminder is still at this version")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	target := remindersPath(item) + "/" + reminder.String()
	if err := client.Delete(ctx, target, versionPrecondition(*expected)); err != nil {
		return err
	}
	printf(cli.Err, "reminder removed: %s\n", reminder)
	return nil
}

func remindersPath(item openapitypes.UUID) string {
	return itemsPath + "/" + item.String() + "/reminders"
}

// reminderTable shows what a person asks a list for: when it fires, what it was asked for, and
// where it stands.
func reminderTable(reminders []openapi.Reminder) Table {
	rows := make([][]string, 0, len(reminders))
	for _, reminder := range reminders {
		channels := make([]string, 0, len(reminder.Channels))
		for _, channel := range reminder.Channels {
			channels = append(channels, string(channel))
		}
		rows = append(rows, []string{
			reminder.Id.String(),
			reminder.OffsetSpec,
			shortTime(reminder.FireAt),
			string(reminder.State),
			strings.Join(channels, ","),
			strconv.Itoa(len(reminder.Recipients)),
			strconv.Itoa(reminder.Version),
		})
	}
	return Table{
		Columns: []string{"id", "offset", "fires", "state", "channels", "recipients", "version"},
		Rows:    rows,
	}
}
