// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strconv"
	"time"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The series on the command line (D-04, D-05, D-09).

func recurrenceGroup() group {
	return group{
		name:    "recur",
		summary: "work that comes back - the rule, and what it produces",
		commands: []command{
			{
				name:    "show",
				usage:   "<item-id>",
				summary: "read the series on an entry",
				run:     recurrenceShow,
			},
			{
				name: "set",
				usage: "<item-id> --rule <RRULE> --zone <iana> [--mode ON_SCHEDULE|ON_COMPLETION]" +
					" [--horizon <days>] [--until <timestamp>] [--count <n>]",
				summary: "make an entry recur, or change how it does",
				run:     recurrenceSet,
			},
			{
				name:    "skip",
				usage:   "<item-id>",
				summary: "skip the next occurrence the series has not produced yet",
				run:     recurrenceSkip,
			},
			{
				name:    "rm",
				usage:   "<item-id> [--expect-version <n>]",
				summary: "stop an entry recurring; what it produced stays",
				run:     recurrenceRemove,
			},
		},
	}
}

func recurrenceShow(ctx context.Context, cli *CLI, args []string) error {
	const usage = "recur show <item-id>"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "recur", "show", "<item-id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var rule openapi.Recurrence
	if err := client.Get(ctx, recurrencePath(item), nil, &rule); err != nil {
		return err
	}
	return cli.Emit(rule, recurrenceTable([]openapi.Recurrence{rule}))
}

func recurrenceSet(ctx context.Context, cli *CLI, args []string) error {
	const usage = "recur set <item-id> --rule <RRULE> --zone <iana> [--mode <mode>]" +
		" [--horizon <days>] [--until <timestamp>] [--count <n>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "recur", "set", usage)
	rule := flags.String("rule", "", "an RFC 5545 rule, such as FREQ=WEEKLY;BYDAY=MO")
	zone := flags.String("zone", "", "the IANA zone the rule is read in, such as Europe/Berlin")
	mode := flags.String("mode", "ON_SCHEDULE",
		"ON_SCHEDULE puts the next one in place at its time; ON_COMPLETION waits for this one to be done")
	horizon := flags.Int("horizon", 0, "how many days ahead occurrences are materialised")
	until := flags.String("until", "", "when the series stops, as an RFC 3339 moment")
	count := flags.Int("count", 0, "how many occurrences the series produces at most")
	expected := flags.Int("expect-version", 0, "refuse unless the series is still at this version")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *rule == "" || *zone == "" {
		return usagef("a series needs --rule and --zone")
	}
	if *until != "" && *count > 0 {
		// The API refuses the pair, and saying so here saves the round trip.
		return usagef("--until and --count are two ways to end a series; send one")
	}

	body := openapi.RecurrenceInput{
		Rrule: *rule, TimeZone: *zone, Mode: openapi.RecurrenceMode(*mode),
	}
	if *horizon > 0 {
		body.HorizonDays = horizon
	}
	if *count > 0 {
		body.MaxCount = count
	}
	if *until != "" {
		moment, err := time.Parse(time.RFC3339, *until)
		if err != nil {
			message, _ := cli.Catalogue.Message(
				"items.due_at_malformed", map[string]string{"value": *until})
			return usageError{error: errorString("--until: " + message)}
		}
		body.EndsAt = &moment
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var stored openapi.Recurrence
	if err := client.PutVersioned(
		ctx, recurrencePath(item), body, versionPrecondition(*expected), &stored,
	); err != nil {
		return err
	}
	return cli.Emit(stored, recurrenceTable([]openapi.Recurrence{stored}))
}

func recurrenceSkip(ctx context.Context, cli *CLI, args []string) error {
	const usage = "recur skip <item-id>"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "recur", "skip", "<item-id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var skipped openapi.Recurrence
	if err := client.Post(ctx, recurrencePath(item)+":skip", nil, &skipped); err != nil {
		return err
	}
	return cli.Emit(skipped, recurrenceTable([]openapi.Recurrence{skipped}))
}

func recurrenceRemove(ctx context.Context, cli *CLI, args []string) error {
	const usage = "recur rm <item-id> [--expect-version <n>]"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "recur", "rm", "<item-id> [--expect-version <n>]")
	expected := flags.Int("expect-version", 0, "refuse unless the series is still at this version")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, recurrencePath(item), versionPrecondition(*expected)); err != nil {
		return err
	}
	// What the series produced is not touched, and the confirmation says so: a person removing a
	// rule wants to know that yesterday's occurrence is still on their list.
	printf(cli.Err, "series removed: %s (the occurrences it produced stay)\n", item)
	return nil
}

func recurrencePath(item openapitypes.UUID) string {
	return itemsPath + "/" + item.String() + "/recurrence"
}

// recurrenceTable shows the rule and the two things a person asks about it: when it stops, and how
// far the server has got.
func recurrenceTable(rules []openapi.Recurrence) Table {
	rows := make([][]string, 0, len(rules))
	for _, rule := range rules {
		ends := shortTime(rule.EndsAt)
		if rule.MaxCount != nil {
			ends = strconv.Itoa(*rule.MaxCount) + " times"
		}
		rows = append(rows, []string{
			rule.Id.String(),
			rule.Rrule,
			rule.TimeZone,
			string(rule.Mode),
			strconv.Itoa(rule.HorizonDays),
			ends,
			shortTime(rule.LastMaterializedAt),
			strconv.Itoa(rule.Version),
		})
	}
	return Table{
		Columns: []string{"id", "rule", "zone", "mode", "horizon", "ends", "materialised", "version"},
		Rows:    rows,
	}
}
