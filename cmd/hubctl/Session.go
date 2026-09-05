// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The sign-ins a person can see and end (H-01).
//
// One's own and nobody else's, whatever the role - "an administrator who suspects one acts by
// disabling the account, not by reading its sessions". So this group needs no scope and no
// argument beyond an identifier: the credential making the call is the whole of who it is about.

func sessionGroup() group {
	return group{
		name:    "session",
		summary: "the sign-ins on this account, and ending them",
		commands: []command{
			{
				name:    "ls",
				summary: "the account's own sessions, newest first",
				run:     sessionList,
			},
			{
				name:    "revoke",
				usage:   "<id> | --all",
				summary: "end one session, or every one including this shell's",
				run:     sessionRevoke,
			},
		},
	}
}

func sessionList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "session", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var sessions []openapi.Session
	if err := client.Get(ctx, sessionsPath, nil, &sessions); err != nil {
		return err
	}
	return cli.Emit(sessions, sessionTable(sessions))
}

// sessionRevoke ends one session or all of them, and forgets the local one where it was among
// them.
//
// Forgetting matters: the pair in the profile refuses on its next request, and a profile still
// holding it would make the next command fail with an answer about a credential rather than with
// the plain fact that this shell signed itself out.
func sessionRevoke(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "session", "revoke", "<id> | --all")
	all := flags.Bool("all", false, "end every session of this account, this shell's included")

	identifier := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		identifier, args = args[0], args[1:]
	}
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	switch {
	case identifier == "" && !*all:
		return usagef("say which session: hubctl session revoke <id>, or --all")
	case identifier != "" && *all:
		return usagef("give an identifier or --all, not both")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}

	if *all {
		if err := client.Delete(ctx, sessionsPath, ""); err != nil {
			return err
		}
		cli.forgetSession()
		printf(cli.Err, "every session of this account is over, this one included\n")
		return nil
	}

	sessionID, err := cli.parseID("the identifier", identifier)
	if err != nil {
		return err
	}
	// Somebody else's session is not found rather than forbidden, and revoking twice is not an
	// error - both of which are the server's to decide, so nothing is checked here first.
	if err := client.Delete(ctx, sessionRevokePrefix+sessionID.String(), ""); err != nil {
		return err
	}
	if sessionID.String() == cli.Profile.Session.ID {
		cli.forgetSession()
		printf(cli.Err, "that was this shell's own session; sign in again with `hubctl login`\n")
		return nil
	}
	printf(cli.Err, "session %s is over\n", sessionID)
	return nil
}

func sessionTable(sessions []openapi.Session) Table {
	rows := make([][]string, 0, len(sessions))
	for _, session := range sessions {
		rows = append(rows, []string{
			session.Id.String(),
			map[bool]string{true: "yes", false: "-"}[session.Current],
			shortTime(&session.CreatedAt),
			shortTime(session.LastUsedAt),
			text(session.UserAgent),
			// The network, coarsened where it was recorded - a /24 or a /48, never an address
			// (T-01). The column is called what it holds so that nobody reads it as one.
			text(session.IpClass),
		})
	}
	return Table{
		Columns: []string{"id", "this one", "opened", "last used", "client", "network"},
		Rows:    rows,
	}
}
