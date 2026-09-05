// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"flag"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The second factor from a terminal (H-02).
//
// This group handles credentials the way `calendar mint` does, and for the same reason: the
// secret, the provisioning URI and the ten recovery codes exist outside the server for exactly one
// answer. They go to standard output, where a script can read them, with the warning beside them
// on standard error - the warning must not end up in whatever the output is piped into.
//
// The QR image is not this client's job and never will be. What a terminal can offer is the
// base32 secret for typing by hand and the `otpauth://` URI for anything that renders one.

const (
	totpEnrollPath  = "/auth/mfa/totp:enroll"
	totpConfirmPath = "/auth/mfa/totp:confirm"
	mfaDisablePath  = "/auth/mfa:disable"
)

func mfaGroup() group {
	return group{
		name:    "mfa",
		summary: "the second factor: enrolling, arming it, and taking it off",
		commands: []command{
			{
				name:    "enroll",
				usage:   "[--pending <credential>]",
				summary: "mint the secret and the recovery codes, shown once",
				run:     mfaEnroll,
			},
			{
				name:    "confirm",
				usage:   "[--pending <credential>]",
				summary: "arm the enrolment with a first valid code",
				run:     mfaConfirm,
			},
			{
				name:    "disable",
				summary: "remove the factor and burn the recovery codes; it asks for the password",
				run:     mfaDisable,
			},
		},
	}
}

// mfaEnroll begins the enrolment. It arms nothing: until a code confirms it, sign-in is unchanged,
// "because an unconfirmed enrolment protects nobody and locks nobody out".
func mfaEnroll(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "mfa", "enroll", "[--pending <credential>]")
	pending := pendingFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.enrolmentClient(*pending)
	if err != nil {
		return err
	}
	var enrolment openapi.TotpEnrollment
	if err := client.Post(ctx, totpEnrollPath,
		openapi.TotpEnrollmentStart{PendingToken: optional(*pending)}, &enrolment); err != nil {
		return err
	}

	if cli.JSON {
		if err := cli.Emit(enrolment, Table{}); err != nil {
			return err
		}
	} else {
		cli.emitTable(Table{
			Columns: []string{"secret", "provisioning uri"},
			Rows:    [][]string{{enrolment.Secret, enrolment.OtpauthUri}},
		})
		printf(cli.Out, "\n")
		cli.emitTable(Table{
			Columns: []string{"recovery codes"},
			Rows:    recoveryRows(enrolment.RecoveryCodes),
		})
	}

	printf(cli.Err,
		"the secret, the URI and the recovery codes above are shown once: put the secret in an "+
			"authenticator and the codes somewhere safe.\n"+
			"Nothing is armed until `hubctl mfa confirm` presents a code from it.\n")
	return nil
}

// mfaConfirm arms the enrolment, and - for the enforcement flow - finishes the sign-in that sent
// the person here, because by then they have proved both factors.
func mfaConfirm(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "mfa", "confirm", "[--pending <credential>]")
	pending := pendingFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.enrolmentClient(*pending)
	if err != nil {
		return err
	}
	code, err := cli.readCredential(envTotp, "The authenticator's current code: ")
	if err != nil {
		return err
	}

	var confirmed openapi.TotpConfirmed
	if err := client.Post(ctx, totpConfirmPath, openapi.TotpConfirmation{
		PendingToken: optional(*pending), Code: code,
	}, &confirmed); err != nil {
		return err
	}

	printf(cli.Err, "the second factor is armed; sign-in asks for a code from now on\n")
	if confirmed.Tokens == nil {
		return nil
	}
	// The enforcement flow ends here rather than at `hubctl login`: the sign-in that was routed
	// into enrolment is completed by this very call, and the pair it answers is the session.
	return cli.holdSession(cli.Profile.BaseURL, cli.Profile.Tenant, *confirmed.Tokens)
}

// mfaDisable takes the factor off. It demands the password afresh - "the one case where 'recently
// signed in' is not enough, because a stolen session removing the second factor is exactly the
// attack the factor exists against".
func mfaDisable(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "mfa", "disable", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	password, err := cli.readCredential(envPassword,
		"Removing the second factor asks for the password (it will be visible): ")
	if err != nil {
		return err
	}
	if err := client.Post(ctx, mfaDisablePath, openapi.MfaDisable{Password: password}, nil); err != nil {
		return err
	}
	printf(cli.Err, "the second factor is gone, and the remaining recovery codes with it\n")
	return nil
}

func pendingFlag(flags *flag.FlagSet) *string {
	return flags.String("pending", "",
		"the credential a sign-in under tenant enforcement answered, instead of being signed in")
}

// enrolmentClient picks which credential the call carries. Both routes accept either, and which
// one is being used is exactly what `--pending` says: a person routed into enrolment instead of
// into a session has no bearer credential to present, and presenting a stale one beside the
// pending credential would be verified and refused.
func (cli *CLI) enrolmentClient(pending string) (*Client, error) {
	if pending != "" {
		return cli.anonymousClient(cli.Profile)
	}
	return cli.client()
}

// recoveryRows prints the ten codes one to a line. A single cell holding all of them would be a
// line no terminal wraps usefully, and these are read aloud and typed one at a time.
func recoveryRows(codes []string) [][]string {
	rows := make([][]string, 0, len(codes))
	for _, code := range codes {
		rows = append(rows, []string{strings.TrimSpace(code)})
	}
	return rows
}
