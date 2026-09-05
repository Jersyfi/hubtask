// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// capabilitiesPath is what a sign-in calls to find out whether the credential is any good.
//
// It is chosen because it is the one operation that needs no scope: openapi.yaml declares it
// public, and the middleware still verifies a credential that was presented (presentation/rest,
// Auth.go). So a token with `items:read` and a token with everything both get the same answer -
// valid or not - which is exactly the question a sign-in asks.
const capabilitiesPath = "/meta/capabilities"

// Credential sources, as `hubctl auth status` reports them.
const (
	sourceEnvironment = "environment"
	sourceSession     = "session"
	sourceProfile     = "profile"
	sourceNone        = "none"
)

func authGroup() group {
	return group{
		name:    "auth",
		summary: "sign in to an installation, and see which credential is in use",
		commands: []command{
			{
				name:    "login",
				usage:   "--url <address>",
				summary: "verify a personal access token and store it",
				run:     authLogin,
			},
			{
				name:    "status",
				summary: "show which installation and which credential this shell would use",
				run:     authStatus,
			},
			{
				name:    "logout",
				summary: "forget the stored credential",
				run:     authLogout,
			},
		},
	}
}

// AuthStatus is hubctl's answer about its own state. It is the CLI's shape rather than the API's
// - there is no endpoint behind it - and it is documented here because `--json` makes it something
// a script can depend on.
type AuthStatus struct {
	// BaseURL is the installation this shell would talk to, empty if none is configured.
	BaseURL string `json:"base_url"`
	// SignedIn is whether a credential is available at all. It says nothing about whether the
	// installation would accept it; only a call can say that.
	SignedIn bool `json:"signed_in"`
	// TokenSource is `environment`, `session`, `profile` or `none` - a variable in the shell, the
	// session `hubctl login` holds, the personal access token `hubctl auth login` stored, or
	// nothing at all.
	TokenSource string `json:"token_source"`
	// ProfilePath is where the stored profile lives, whether or not it exists.
	ProfilePath string `json:"profile_path"`
}

func authLogin(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "auth", "login", "--url <address>")
	address := flags.String("url", cli.Profile.BaseURL, "the installation to sign in to")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *address == "" {
		return usagef("say which installation to sign in to: --url https://hub.example.com")
	}
	normalised, err := NormaliseBaseURL(*address)
	if err != nil {
		return err
	}

	raw, err := readToken(cli)
	if err != nil {
		return err
	}
	// The shape is checked before the call, so that a truncated paste is reported as a truncated
	// paste rather than as a rejected credential.
	if _, err := identity.ParseToken(raw); err != nil {
		return cli.renderDomainError(err)
	}

	// Built from the profile being signed in with rather than from the resolved one, which is
	// the whole point: the credential being verified is the one that was just typed.
	verifying := *cli
	verifying.Profile = Profile{BaseURL: normalised, Token: secret.New(raw)}
	client, err := verifying.client()
	if err != nil {
		return err
	}
	// Verified before it is stored. A profile holding a credential the installation refuses is
	// worse than no profile: every later command fails with an answer about the token rather
	// than about the sign-in that accepted it.
	if err := client.Get(ctx, capabilitiesPath, nil, nil); err != nil {
		return err
	}

	if err := SaveProfile(cli.ProfilePath, verifying.Profile); err != nil {
		return err
	}
	// On standard error: signing in produces no payload, and a script that pipes standard output
	// should get nothing rather than a sentence it has to skip.
	printf(cli.Err, "signed in to %s; the credential is stored in %s\n", normalised, cli.ProfilePath)
	return nil
}

func authStatus(_ context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "auth", "status", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	stored, err := LoadProfile(cli.ProfilePath)
	if err != nil {
		return err
	}

	status := AuthStatus{
		BaseURL:     cli.Profile.BaseURL,
		SignedIn:    !cli.Profile.Credential().IsEmpty(),
		TokenSource: sourceNone,
		ProfilePath: cli.ProfilePath,
	}
	switch {
	case cli.Env(envToken) != "":
		status.TokenSource = sourceEnvironment
	case !cli.Profile.Session.IsEmpty():
		status.TokenSource = sourceSession
	case !stored.Token.IsEmpty():
		status.TokenSource = sourceProfile
	}

	return cli.Emit(status, Table{
		Columns: []string{"installation", "signed in", "token from", "profile"},
		Rows: [][]string{{
			firstNonEmpty(status.BaseURL, "-"),
			map[bool]string{true: "yes", false: "no"}[status.SignedIn],
			status.TokenSource,
			status.ProfilePath,
		}},
	})
}

func authLogout(_ context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "auth", "logout", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	// Every credential, the session included: the file is the profile, and forgetting half of it
	// would leave `hubctl auth status` reporting a sign-in nothing can use.
	if err := ForgetProfile(cli.ProfilePath); err != nil {
		return err
	}
	printf(cli.Err, "the stored credential is gone (%s)\n", cli.ProfilePath)
	if cli.Env(envToken) != "" {
		// Otherwise the next command still works and nobody can see why.
		printf(cli.Err, "note: %s is still set in this shell, and it takes precedence over the profile\n", envToken)
	}
	return nil
}

// readToken takes the credential from the environment or from standard input, and never from an
// argument: an argument is visible in `ps` and lands in the shell history.
func readToken(cli *CLI) (string, error) {
	if fromEnv := cli.Env(envToken); fromEnv != "" {
		return strings.TrimSpace(fromEnv), nil
	}

	// A prompt only where there is somebody to read it. Piped input gets none, so that
	// `echo "$TOKEN" | hubctl auth login` writes nothing it did not ask for.
	if file, ok := cli.In.(*os.File); ok {
		if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			// The token is echoed as it is typed. Turning the terminal's echo off needs a
			// dependency this binary does not have, and a prompt that lies about being hidden
			// would be worse than one that does not claim to be - hence the pipe in the hint.
			printf(cli.Err, "Paste the personal access token (it will be visible), then press Ctrl-D:\n")
		}
	}

	raw, err := io.ReadAll(cli.In)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", errors.New("no token to store: pipe one into `hubctl auth login`, or set " + envToken)
	}
	return token, nil
}

// renderDomainError turns a refusal the domain made here, on this machine, into the same sentence
// the server's refusal would have produced. A token rejected before the call and one rejected
// after it should not read as though they came from different programs.
func (cli *CLI) renderDomainError(err error) error {
	domainErr := shared.AsError(err)
	if domainErr == nil {
		return err
	}
	if message, known := cli.Catalogue.Message(domainErr.DetailCode, domainErr.Params); known {
		return errors.New(message)
	}
	message, _ := cli.Catalogue.Message("errors."+domainErr.Code, domainErr.Params)
	return errors.New(message)
}
