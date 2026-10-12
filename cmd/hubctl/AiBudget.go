// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strconv"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The AI budgets of a workspace, per source (ADR-0072 §4): what the offered model may spend a day,
// which the installation sets, and what the workspace's own model may, which the workspace sets
// here - and nobody else.

const aiBudgetPath = "/ai-budget"

// aiBudget shows the two AI rows, or sets or clears the own model's budget.
func aiBudget(ctx context.Context, cli *CLI, args []string) error {
	verb := "show"
	if len(args) > 0 {
		verb, args = args[0], args[1:]
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	switch verb {
	case "show":
		flags := commandFlags(cli, "ai budget", "show", "")
		if err := parseCommand(flags, args); err != nil {
			return err
		}
		var standings []openapi.QuotaStanding
		if err := client.Get(ctx, quotasPath, nil, &standings); err != nil {
			return err
		}
		ai := make([]openapi.QuotaStanding, 0, 2)
		for _, standing := range standings {
			if standing.Quota == openapi.AiTokensPerDay ||
				standing.Quota == openapi.AiOwnTokensPerDay {
				ai = append(ai, standing)
			}
		}
		return cli.Emit(ai, quotaTable(ai))
	case "set", "clear":
		flags := commandFlags(cli, "ai budget", verb, "<tokens a day>")
		if err := parseCommand(flags, args); err != nil {
			return err
		}
		body := openapi.OwnAiBudget{}
		if verb == "set" {
			if flags.NArg() != 1 {
				return usagef("hubctl ai budget set takes the own model's tokens a day")
			}
			tokens, err := strconv.ParseInt(flags.Arg(0), 10, 64)
			if err != nil || tokens < 1 {
				return usagef("%q is not a number of tokens of at least 1", flags.Arg(0))
			}
			body.OwnTokensPerDay = &tokens
		} else if flags.NArg() != 0 {
			return usagef("hubctl ai budget clear takes nothing")
		}
		var standings []openapi.QuotaStanding
		if err := client.Put(ctx, aiBudgetPath, body, &standings); err != nil {
			return err
		}
		return cli.Emit(standings, quotaTable(standings))
	default:
		return usagef("hubctl ai budget takes `show`, `set <tokens>` or `clear`")
	}
}
