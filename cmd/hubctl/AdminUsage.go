// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"slices"
	"strconv"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The use per workspace, metric and day, for billing (UC-INS-14 checks 4 and 7): offered only while
// the operator has switched metering on (`admin settings set metering on`). One row per workspace,
// metric and day; the next page's cursor on standard error, as `admin events` prints its own.

const adminUsagePath = "/admin/usage"

func adminUsage(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin usage --from <YYYY-MM-DD> --to <YYYY-MM-DD> [--metric <name>] [--workspace <id>] [--cursor <cursor>] [--size <n>]"
	flags := commandFlags(cli, "admin", "usage",
		"--from <YYYY-MM-DD> --to <YYYY-MM-DD> [--metric <name>] [--workspace <id>] [--cursor <cursor>] [--size <n>]")
	from := flags.String("from", "", "the first day")
	to := flags.String("to", "", "the last day; 366 days at most, both included")
	metric := flags.String("metric", "", "one metric's name; every metric when unset")
	workspace := flags.String("workspace", "", "one workspace's id; every workspace when unset")
	cursor := flags.String("cursor", "", "where the last page stopped")
	size := flags.Int("size", 0, "workspaces per page (the server decides when unset)")
	if err := parseOnlyFlags(flags, args, usage); err != nil {
		return err
	}
	if *from == "" || *to == "" {
		return usagef("admin usage needs --from and --to")
	}

	query := url.Values{"from": {*from}, "to": {*to}}
	for name, value := range map[string]string{"metric": *metric, "workspace": *workspace, "cursor": *cursor} {
		if value != "" {
			query.Set(name, value)
		}
	}
	if *size > 0 {
		query.Set("size", strconv.Itoa(*size))
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var page openapi.UsagePage
	if err := client.Get(ctx, adminUsagePath, query, &page); err != nil {
		return err
	}
	if err := cli.Emit(page, usageTable(page)); err != nil {
		return err
	}
	printf(cli.Err, "final through %s\n", page.FinalThrough.String())
	if page.Page.NextCursor != nil && *page.Page.NextCursor != "" {
		printf(cli.Err, "continue with --cursor %s\n", *page.Page.NextCursor)
	}
	return nil
}

// usageTable prints one row per workspace, metric and day, the metrics in the answer's order.
func usageTable(page openapi.UsagePage) Table {
	rows := [][]string{}
	for _, workspace := range page.Data {
		purged := ""
		if workspace.Purged {
			purged = "purged"
		}
		names := make([]string, 0, len(page.Metrics))
		for _, metric := range page.Metrics {
			names = append(names, metric.Name)
		}
		for name := range workspace.Metrics {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
		for _, name := range names {
			for _, day := range workspace.Metrics[name] {
				rows = append(rows, []string{
					workspace.WorkspaceId.String(), purged, name, day.Day.String(), strconv.FormatInt(day.Value, 10),
				})
			}
		}
	}
	return Table{Columns: []string{"workspace", "state", "metric", "day", "value"}, Rows: rows}
}
