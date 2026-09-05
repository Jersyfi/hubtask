// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strconv"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// What a workspace may hold, and how close it is to the wall (H-08).
//
// A read from inside the workspace rather than from the control plane: a quota is workspace
// configuration, and who is near a wall is exactly what its administrators and its auditors read.
// Moving a wall is the operator's write and lives elsewhere.

const quotasPath = "/quotas"

func quotaGroup() group {
	return group{
		name:    "quota",
		summary: "the workspace's ceilings, and how close it is to them",
		commands: []command{
			{
				name:    "show",
				summary: "every limit as it applies to this workspace",
				run:     quotaShow,
			},
		},
	}
}

func quotaShow(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "quota", "show", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var standings []openapi.QuotaStanding
	if err := client.Get(ctx, quotasPath, nil, &standings); err != nil {
		return err
	}
	return cli.Emit(standings, quotaTable(standings))
}

func quotaTable(standings []openapi.QuotaStanding) Table {
	rows := make([][]string, 0, len(standings))
	for _, standing := range standings {
		rows = append(rows, []string{
			string(standing.Quota),
			ceiling(standing.Limit),
			count64(standing.Used),
			approach(standing.Ratio),
			// Whether the workspace carries its own ceiling or the mode's default. It is the
			// difference between "somebody decided this" and "nobody has", which is the first
			// question asked of a limit that is in the way.
			yesNo(standing.Configured),
		})
	}
	return Table{
		Columns: []string{"quota", "limit", "used", "approach", "set for this workspace"},
		Rows:    rows,
	}
}

// ceiling renders the limit, and nought as what it means. A column of numbers with a nought in it
// reads as "nothing allowed", which is the opposite of what the contract says.
func ceiling(limit int64) string {
	if limit == 0 {
		return "unlimited"
	}
	return strconv.FormatInt(limit, 10)
}

// count64 is the live count where one exists. A dash where none does: the request rate is
// enforced by the limiter and reported in its own headers, so a nought here would be a lie about
// a wall nobody is near.
func count64(used *int64) string {
	if used == nil {
		return "-"
	}
	return strconv.FormatInt(*used, 10)
}

func approach(ratio *float32) string {
	if ratio == nil {
		return "-"
	}
	return strconv.Itoa(int(*ratio*100)) + "%"
}
