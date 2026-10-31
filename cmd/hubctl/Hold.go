// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const holdsPath = "/legal-holds"

func holdGroup() group {
	return group{
		name:    "hold",
		summary: "an instruction not to delete something, and the lifting of it",
		commands: []command{
			{
				name:    "place",
				usage:   "--scope TENANT|CONTAINER|ITEM|ACCOUNT [--id <id>] --reason <why>",
				summary: "stop anything under it being deleted",
				run:     holdPlace,
			},
		},
	}
}

func holdPlace(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "hold", "place", "--scope <kind> [--id <id>] --reason <why>")
	scope := flags.String("scope", "", "TENANT, CONTAINER, ITEM or ACCOUNT")
	scopeID := flags.String("id", "", "what it covers; TENANT names nothing, because it covers everything")
	reason := flags.String("reason", "", "why, in your own words - an auditor reads this")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *scope == "" || *reason == "" {
		return usagef("hold place needs --scope and --reason")
	}

	create := openapi.LegalHoldCreate{Reason: *reason}
	create.Scope.Kind = openapi.LegalHoldCreateScopeKind(*scope)
	if *scopeID != "" {
		parsed, err := cli.parseID("--id", *scopeID)
		if err != nil {
			return err
		}
		create.Scope.Id = &parsed
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var placed openapi.LegalHold
	if err := client.Post(ctx, holdsPath, create, &placed); err != nil {
		return err
	}
	return cli.Emit(placed, holdTable([]openapi.LegalHold{placed}))
}

func holdTable(holds []openapi.LegalHold) Table {
	rows := make([][]string, 0, len(holds))
	for _, hold := range holds {
		rows = append(rows, []string{
			hold.Id.String(),
			string(hold.Scope.Kind),
			id(hold.Scope.Id),
			hold.Reason,
			shortTime(hold.PlacedAt),
			releasedState(hold),
		})
	}
	return Table{
		Columns: []string{"id", "scope", "covers", "reason", "placed", "released"},
		Rows:    rows,
	}
}

// releasedState is the column that says whether a hold is still doing anything. A hold in force
// reads as "in force" rather than as a dash: a blank there is the one thing somebody reading a
// list of holds must not have to guess at.
func releasedState(hold openapi.LegalHold) string {
	if hold.ReleasedAt == nil {
		return "in force"
	}
	if hold.ReleasedReason == nil || *hold.ReleasedReason == "" {
		return shortTime(hold.ReleasedAt)
	}
	return shortTime(hold.ReleasedAt) + ": " + *hold.ReleasedReason
}
