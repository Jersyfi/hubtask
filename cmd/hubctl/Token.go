// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The credentials on the command line (G-01).
//
// The second group that handles a secret, and it follows the calendar's rule: the token is
// printed exactly once, by the command that mints it, and never again. `ls` shows that a
// credential exists, what it may do and when it was last used; it cannot show the token, because
// the server does not keep one.

const (
	tokensPath         = "/auth/tokens" //nolint:gosec // G101: a route, not a credential.
	serviceAccountPath = "/auth/service-accounts"
)

func tokenGroup() group {
	return group{
		name:    "token",
		summary: "personal access tokens - mint, list, revoke",
		commands: []command{
			{
				name:    "ls",
				usage:   "[--account <id>]",
				summary: "list one's own tokens, or a service account's",
				run:     tokenList,
			},
			{
				name:    "create",
				usage:   "--name <text> --scope <a,b> (--days <n> | --expires <RFC3339>) [--account <id>]",
				summary: "mint a token and print it once",
				run:     tokenCreate,
			},
			{
				name:    "revoke",
				usage:   "<token-id>",
				summary: "stop a token; the call after this one is refused",
				run:     tokenRevoke,
			},
		},
	}
}

func serviceAccountGroup() group {
	return group{
		name:    "service-account",
		summary: "accounts that exist only to be acted through",
		commands: []command{
			{
				name:    "ls",
				summary: "list the workspace's service accounts",
				run:     serviceAccountList,
			},
			{
				name:    "create",
				usage:   "--name <text>",
				summary: "create a service account an integration or a rule can act as",
				run:     serviceAccountCreate,
			},
		},
	}
}

func tokenList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "token", "ls", "[--account <id>]")
	account := flags.String("account", "", "whose tokens; omitted means one's own")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	query := url.Values{}
	if *account != "" {
		owner, err := cli.parseID("--account", *account)
		if err != nil {
			return err
		}
		query.Set("account_id", owner.String())
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var tokens []openapi.AccessToken
	if err := client.Get(ctx, tokensPath, query, &tokens); err != nil {
		return err
	}
	return cli.Emit(tokens, tokenTable(tokens))
}

func tokenCreate(ctx context.Context, cli *CLI, args []string) error {
	const usage = "--name <text> --scope <a,b> (--days <n> | --expires <RFC3339>) [--account <id>]"
	flags := commandFlags(cli, "token", "create", usage)
	name := flags.String("name", "", "what the token is for, in your own words")
	scopes := flags.String("scope", "", "the scopes it may exercise, comma separated")
	days := flags.String("days", "", "how many days it should live, at most 365")
	expires := flags.String("expires", "", "when it stops working, RFC 3339")
	account := flags.String("account", "", "whose token; omitted means one's own")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	if *name == "" {
		return usagef("a token needs --name: it is what you will read in a year")
	}
	requested, err := scopeArgument(*scopes)
	if err != nil {
		return err
	}
	expiresAt, err := expiryArgument(*days, *expires)
	if err != nil {
		return err
	}

	body := openapi.AccessTokenCreate{Name: *name, Scopes: requested, ExpiresAt: expiresAt}
	if *account != "" {
		owner, err := cli.parseID("--account", *account)
		if err != nil {
			return err
		}
		body.AccountId = &owner
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var minted openapi.AccessTokenSecret
	if err := client.Post(ctx, tokensPath, body, &minted); err != nil {
		return err
	}

	// The one moment the credential exists outside the server's hash. Under --json it goes to
	// standard output, because a script that just asked for a token has to read it; in a table it
	// goes to standard output too, with the warning beside it on standard error - a person needs
	// to see the warning, and it must not end up in whatever they pipe the token into.
	if cli.JSON {
		return cli.Emit(minted, Table{})
	}
	if err := cli.Emit(minted, tokenTable([]openapi.AccessToken{tokenOf(minted)})); err != nil {
		return err
	}
	printf(cli.Out, "%s\n", minted.Token)
	printf(cli.Err,
		"that is the whole credential and is shown once: store it, and revoke the token if it leaks\n")
	return nil
}

func tokenRevoke(ctx context.Context, cli *CLI, args []string) error {
	const usage = "token revoke <token-id>"
	token, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "token", "revoke", "<token-id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, tokensPath+"/"+token.String(), ""); err != nil {
		return err
	}
	printf(cli.Err, "token revoked: %s (the next call presenting it is refused)\n", token)
	return nil
}

func serviceAccountList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "service-account", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var accounts []openapi.Account
	if err := client.Get(ctx, serviceAccountPath, nil, &accounts); err != nil {
		return err
	}
	return cli.Emit(accounts, serviceAccountTable(accounts))
}

func serviceAccountCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "service-account", "create", "--name <text>")
	name := flags.String("name", "", "what it does - \"the nightly export\", not \"svc1\"")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *name == "" {
		return usagef("a service account needs --name: it is what the audit trail shows")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.Account
	if err := client.Post(ctx, serviceAccountPath,
		openapi.ServiceAccountCreate{DisplayName: *name}, &created); err != nil {
		return err
	}
	return cli.Emit(created, serviceAccountTable([]openapi.Account{created}))
}

// scopeArgument reads the comma separated list. Empty is refused here rather than sent: the
// server would refuse it too, and a round trip to be told "ask for something" is a round trip
// that did not have to happen.
func scopeArgument(raw string) ([]string, error) {
	scopes := make([]string, 0, 4)
	for _, scope := range strings.Split(raw, ",") {
		if scope = strings.TrimSpace(scope); scope != "" {
			scopes = append(scopes, scope)
		}
	}
	if len(scopes) == 0 {
		return nil, usagef("a token needs --scope: ask for what it needs, not for everything")
	}
	return scopes, nil
}

// expiryArgument turns either spelling into the instant the contract wants.
//
// There is deliberately no default, on the server's own reasoning: every default is either short
// enough that people work around it or long enough to be the eternal credential the rule exists to
// prevent. --days exists because "90" is what a person means and computing a date by hand at a
// prompt is how somebody ends up with a token that expires in 2034.
func expiryArgument(days, expires string) (time.Time, error) {
	switch {
	case days != "" && expires != "":
		return time.Time{}, usagef("--days and --expires say the same thing; give one")
	case days != "":
		count, err := strconv.Atoi(days)
		if err != nil || count < 1 {
			return time.Time{}, usagef("--days takes a whole number of days, at least one")
		}
		return time.Now().UTC().Add(time.Duration(count) * 24 * time.Hour), nil
	case expires != "":
		at, err := time.Parse(time.RFC3339, expires)
		if err != nil {
			return time.Time{}, usagef("--expires takes RFC 3339, such as 2027-01-31T09:00:00Z")
		}
		return at, nil
	default:
		return time.Time{}, usagef("a token needs --days or --expires: there is no default")
	}
}

// tokenOf drops the credential from a minted token so the table is built from the same shape the
// listing uses.
func tokenOf(minted openapi.AccessTokenSecret) openapi.AccessToken {
	return openapi.AccessToken{
		Id: minted.Id, AccountId: minted.AccountId, Name: minted.Name, Scopes: minted.Scopes,
		ExpiresAt: minted.ExpiresAt, CreatedAt: minted.CreatedAt,
		LastUsedAt: minted.LastUsedAt, RevokedAt: minted.RevokedAt,
	}
}

func tokenTable(tokens []openapi.AccessToken) Table {
	rows := make([][]string, 0, len(tokens))
	for _, token := range tokens {
		rows = append(rows, []string{
			token.Id.String(),
			token.Name,
			strings.Join(token.Scopes, " "),
			shortTime(&token.ExpiresAt),
			shortTime(token.LastUsedAt),
			shortTime(token.RevokedAt),
		})
	}
	return Table{
		Columns: []string{"id", "name", "scopes", "expires", "last used", "revoked"},
		Rows:    rows,
	}
}

func serviceAccountTable(accounts []openapi.Account) Table {
	rows := make([][]string, 0, len(accounts))
	for _, account := range accounts {
		rows = append(rows, []string{
			account.Id.String(),
			account.DisplayName,
			string(account.Status),
		})
	}
	return Table{Columns: []string{"id", "name", "status"}, Rows: rows}
}
