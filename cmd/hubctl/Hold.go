// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"

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
			{
				name:    "ls",
				usage:   "[--include-released]",
				summary: "the holds in force, and the lifted ones when asked for",
				run:     holdList,
			},
			{
				name:    "release",
				usage:   "<id> --reason <why>",
				summary: "lift one, with the reason it was lifted",
				run:     holdRelease,
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

func holdList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "hold", "ls", "[--include-released]")
	includeReleased := flags.Bool("include-released", false, "the lifted ones too, which is what shows that they were")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	query := url.Values{}
	if *includeReleased {
		query.Set("include_released", "true")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var holds []openapi.LegalHold
	if err := client.Get(ctx, holdsPath, query, &holds); err != nil {
		return err
	}
	return cli.Emit(holds, holdTable(holds))
}

// holdRelease requires the reason the API requires, and refuses without one here rather than
// letting the server say so: the round trip adds nothing, and the sentence is the same either way.
func holdRelease(ctx context.Context, cli *CLI, args []string) error {
	const usage = "hold release <id> --reason <why>"
	holdID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "hold", "release", "<id> --reason <why>")
	reason := flags.String("reason", "", "why it is being lifted - \"released\" with no reason is an entry nobody can act on")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *reason == "" {
		return usagef("hold release needs --reason: lifting a hold is the moment the data under it becomes deletable")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var released openapi.LegalHold
	if err := client.Post(ctx, holdsPath+"/"+holdID.String()+":release",
		openapi.LegalHoldRelease{Reason: *reason}, &released); err != nil {
		return err
	}
	return cli.Emit(released, holdTable([]openapi.LegalHold{released}))
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
