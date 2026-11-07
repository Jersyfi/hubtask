// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strings"
)

// The sign-ins a person can see and end.
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
				name:    "revoke",
				usage:   "<id> | --all",
				summary: "end one session, or every one including this shell's",
				run:     sessionRevoke,
			},
		},
	}
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
