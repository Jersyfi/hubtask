// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The pull half of the event stream (G-13, G-04): the endpoint a platform without a URL polls.
//
// Zapier and n8n cannot receive a webhook on a free plan, so they ask instead - and the whole of
// what makes that safe is the cursor. A poll without one asks "what has happened", which is a
// question whose honest answer grows without bound; a poll with one asks "what has happened since
// this", which is a page. So `--since` is printed after every call, on standard error where a pipe
// does not carry it, and the answer's own next cursor is what it prints.

const triggersPath = "/integrations/triggers/"

func eventsGroup() group {
	return group{
		name:    "events",
		summary: "the pull half of the event stream, for platforms without a URL",
		commands: []command{
			{
				name:    "poll",
				usage:   "<event type> [--since <cursor>] [--limit <n>]",
				summary: "what has happened since a cursor, deduplicable by the event's own id",
				run:     eventsPoll,
			},
		},
	}
}

func eventsPoll(ctx context.Context, cli *CLI, args []string) error {
	const usage = "events poll <event type> [--since <cursor>] [--limit <n>]"
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return usagef("an event type comes first: hubctl %s", usage)
	}
	eventType := args[0]

	flags := commandFlags(cli, "events", "poll", "<event type> [--since <cursor>] [--limit <n>]")
	since := flags.String("since", "",
		"where the last poll stopped; without one the first page is the oldest inside the window")
	limit := flags.Int("limit", 0, "how many at most (the server decides when unset)")
	if err := parseOnlyFlags(flags, args[1:], usage); err != nil {
		return err
	}

	query := url.Values{}
	if *since != "" {
		query.Set("since", *since)
	}
	if *limit > 0 {
		query.Set("limit", strconv.Itoa(*limit))
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var page openapi.TriggerEventPage
	if err := client.Get(ctx, triggersPath+url.PathEscape(eventType), query, &page); err != nil {
		return err
	}
	if err := cli.Emit(page, eventTable(page.Data)); err != nil {
		return err
	}

	// The cursor, always and on standard error: a poller that does not carry one forward asks the
	// unbounded question every time, and a poller that pipes the payload must not find the cursor
	// mixed into it.
	if page.Page.NextCursor != nil && *page.Page.NextCursor != "" {
		printf(cli.Err, "continue with --since %s\n", *page.Page.NextCursor)
	}
	return nil
}

// eventTable prints what a poller decides from: which event, when, and what it was about. The
// payload itself is `--json`'s, because an event's data is somebody's content and a table is not
// where content goes.
func eventTable(events []openapi.TriggerEvent) Table {
	rows := make([][]string, 0, len(events))
	for _, event := range events {
		rows = append(rows, []string{
			field(event, "id"),
			field(event, "type"),
			field(event, "time"),
			field(event, "subject"),
		})
	}
	return Table{Columns: []string{"id", "type", "time", "subject"}, Rows: rows}
}

// field reads one envelope member as text, and answers a dash for one the envelope does not carry.
// A CloudEvent is a map here rather than a struct - the contract types it as one, because `data`
// is whatever the event is about - so reading it is a lookup rather than a field access.
func field(event openapi.TriggerEvent, name string) string {
	value, present := event[name]
	if !present || value == nil {
		return "-"
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}
