// SPDX-License-Identifier: Apache-2.0
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

// The installation's events, read forward: what a purchase platform learns about the workspaces
// (UC-INS-14). The cursor is printed after every read on standard error, as `events poll` prints
// its own, so a pipe carries the events and the operator carries the cursor forward.

const adminEventsPath = "/admin/events"

func adminEvents(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin events [--cursor <cursor>] [--size <n>]"
	flags := commandFlags(cli, "admin", "events", "[--cursor <cursor>] [--size <n>]")
	cursor := flags.String("cursor", "",
		"where the last read stopped; without one the read starts at the oldest event kept")
	size := flags.Int("size", 0, "how many at most (the server decides when unset)")
	if err := parseOnlyFlags(flags, args, usage); err != nil {
		return err
	}

	query := url.Values{}
	if *cursor != "" {
		query.Set("cursor", *cursor)
	}
	if *size > 0 {
		query.Set("size", strconv.Itoa(*size))
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var page openapi.PlatformEventPage
	if err := client.Get(ctx, adminEventsPath, query, &page); err != nil {
		return err
	}
	if err := cli.Emit(page, platformEventTable(page.Data)); err != nil {
		return err
	}
	if page.Page.NextCursor != nil && *page.Page.NextCursor != "" {
		printf(cli.Err, "continue with --cursor %s\n", *page.Page.NextCursor)
	}
	return nil
}

// platformEventTable prints what the event says about its workspace: identifiers and states, which
// is all an installation event carries.
func platformEventTable(events []openapi.PlatformEvent) Table {
	rows := make([][]string, 0, len(events))
	for _, e := range events {
		state := map[string]any{}
		if data, ok := e["data"].(map[string]any); ok {
			if workspace, ok := data["workspace"].(map[string]any); ok {
				state = workspace
			}
		}
		rows = append(rows, []string{
			memberText(e["time"]),
			strings.TrimSuffix(strings.TrimPrefix(memberText(e["type"]), "de.hubtask.installation.workspace."), ".v1"),
			memberText(state["id"]), memberText(state["status"]), memberText(state["plan"]), memberText(state["version"]),
			memberText(state["external_reference"]),
		})
	}
	return Table{
		Columns: []string{"time", "event", "workspace", "status", "plan", "version", "reference"},
		Rows:    rows,
	}
}

// memberText renders one member, a dash for one that is absent or null.
func memberText(value any) string {
	switch v := value.(type) {
	case nil:
		return "-"
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}
