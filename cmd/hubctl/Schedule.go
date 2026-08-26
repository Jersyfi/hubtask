// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The schedule on the command line (D-09): when something is due, and in which zone that means
// what it says.

func dueGroup() group {
	return group{
		name:    "due",
		summary: "when work is due - a day, or a moment",
		commands: []command{
			{
				name:    "set",
				usage:   "<id> --at <date|timestamp> [--zone <iana>] [--expect-version <n>]",
				summary: "date an entry",
				run:     dueSet,
			},
			{
				name:    "clear",
				usage:   "<id> [--expect-version <n>]",
				summary: "take the date off an entry",
				run:     dueClear,
			},
		},
	}
}

// dueValue is what --at accepts, and the one piece of grammar this file adds.
//
// Two spellings, and the shorter one is not a shortcut for the longer: `2026-09-10` is a day, and
// it is stored as an all-day due date in the entry's zone; `2026-09-10T09:00:00Z` is a moment.
// That is the distinction the product makes (D-01, i18n-l10n.md §4), and a client that turned a
// date into midnight would be deciding it for the person typing.
type dueValue struct {
	at       time.Time
	dateOnly bool
}

func parseDue(cli *CLI, flag, raw string) (dueValue, error) {
	if day, err := time.Parse(time.DateOnly, raw); err == nil {
		return dueValue{at: day, dateOnly: true}, nil
	}
	moment, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		message, _ := cli.Catalogue.Message("items.due_unreadable", map[string]string{"value": raw})
		return dueValue{}, usageError{error: errorString(flag + ": " + message)}
	}
	return dueValue{at: moment}, nil
}

func dueSet(ctx context.Context, cli *CLI, args []string) error {
	const usage = "due set <id> --at <date|timestamp> [--zone <iana>] [--expect-version <n>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "due", "set", "<id> --at <date|timestamp> [--zone <iana>] [--expect-version <n>]")
	at := flags.String("at", "", "the day (2026-09-10) or the moment (2026-09-10T09:00:00Z) it is due")
	zone := flags.String("zone", "", "the IANA zone the date is read in, such as Europe/Berlin")
	expected := flags.Int("expect-version", 0, "refuse unless the entry is still at this version")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *at == "" {
		return usagef("a date needs --at")
	}

	due, err := parseDue(cli, "--at", *at)
	if err != nil {
		return err
	}
	body := openapi.DueDateInput{DueAt: due.at}
	if due.dateOnly {
		body.DueDateOnly = &due.dateOnly
	}
	if *zone != "" {
		body.DueTimeZone = zone
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var updated openapi.WorkItem
	if err := client.PutVersioned(
		ctx, itemsPath+"/"+item.String()+"/due", body, versionPrecondition(*expected), &updated,
	); err != nil {
		return err
	}
	return cli.Emit(updated, dueTable([]openapi.WorkItem{updated}))
}

func dueClear(ctx context.Context, cli *CLI, args []string) error {
	const usage = "due clear <id> [--expect-version <n>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "due", "clear", "<id> [--expect-version <n>]")
	expected := flags.Int("expect-version", 0, "refuse unless the entry is still at this version")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(
		ctx, itemsPath+"/"+item.String()+"/due", versionPrecondition(*expected),
	); err != nil {
		return err
	}
	// No payload, so the confirmation goes to standard error and `--json` writes nothing into a
	// pipe (the shape `item rm` set).
	printf(cli.Err, "date cleared: %s\n", item)
	return nil
}

// versionPrecondition spells an expected version as an entity tag, or nothing at all.
func versionPrecondition(expected int) string {
	if expected <= 0 {
		return ""
	}
	return `"` + strconv.Itoa(expected) + `"`
}

// dueTable shows the schedule rather than the entry: what a person who just dated something wants
// to read back is the date, the flag and the zone.
func dueTable(items []openapi.WorkItem) Table {
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.Id.String(),
			item.Title,
			shortTime(item.DueAt),
			yesNo(item.DueDateOnly),
			text(item.DueTimeZone),
			strconv.Itoa(item.Version),
		})
	}
	return Table{
		Columns: []string{"id", "title", "due", "all day", "zone", "version"},
		Rows:    rows,
	}
}
