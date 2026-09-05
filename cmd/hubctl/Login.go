// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The sign-in of a person, as opposed to `hubctl auth login`, which stores a machine's credential
// (H-01, H-02).
//
// The difference is worth the second verb. A personal access token is a credential somebody minted
// once and pasted; a session is a person who proved themselves just now, and it is the only
// credential this API accepts for the acts that demand proving it again - a step-up cannot be
// satisfied by a token, and consenting to an app is "never a token, a person".

const (
	sessionsPath        = "/auth/sessions"
	sessionRefreshPath  = "/auth/sessions:refresh"
	sessionVerifyPath   = "/auth/sessions:verify"
	sessionRevokePrefix = "/auth/sessions/"
)

func loginGroup() group {
	return group{
		name:    "login",
		summary: "sign in as a person, with a password and - where one is armed - a code",
		usage:   "--url <address> --email <address> [--tenant <workspace>] [--recovery]",
		run:     signIn,
	}
}

// signIn opens the session and holds it in the profile.
//
// Held rather than printed: the pair is answered once, the access token is good for fifteen
// minutes, and a person types more than one command in that time. What makes holding it honest is
// the renewal below - the profile carries the refresh token too, and a command that finds the
// access token nearly out buys the next pair before it calls anything.
func signIn(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "login", "",
		"--url <address> --email <address> [--tenant <workspace>] [--recovery]")
	address := flags.String("url", cli.Profile.BaseURL, "the installation to sign in to")
	email := flags.String("email", "", "the address the account signs in with")
	tenant := flags.String("tenant", cli.Profile.Tenant,
		"the workspace, in an installation that runs more than one")
	recovery := flags.Bool("recovery", false,
		"answer the second step with one of the ten recovery codes instead of the authenticator's")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *address == "" {
		return usagef("say which installation to sign in to: --url https://hub.example.com")
	}
	if *email == "" {
		return usagef("say who is signing in: --email somebody@example.com")
	}
	normalised, err := NormaliseBaseURL(*address)
	if err != nil {
		return err
	}

	// Anonymous, and not because there is no credential to hand: the sign-in is public by
	// contract, and a credential presented beside the body is verified all the same - so an
	// expired session in the profile would refuse the very call that replaces it.
	client, err := cli.anonymousClient(Profile{BaseURL: normalised, Tenant: *tenant})
	if err != nil {
		return err
	}

	password, err := cli.readCredential(envPassword, "Password (it will be visible): ")
	if err != nil {
		return err
	}

	status, answer, err := client.PostStatus(ctx, sessionsPath, openapi.SignIn{
		Email: openapitypes.Email(*email), Password: password,
	})
	if err != nil {
		return err
	}

	if status == http.StatusAccepted {
		return cli.secondStep(ctx, client, normalised, *tenant, answer, *recovery)
	}
	tokens, err := readSessionTokens(answer)
	if err != nil {
		return err
	}
	return cli.holdSession(normalised, *tenant, tokens)
}

// secondStep answers the challenge a two-step sign-in owes.
//
// Two shapes of owing, and they end differently. A code the caller can produce is asked for and
// presented here, so `hubctl login` finishes what it started. An enrolment the tenant demands
// cannot be: the secret has to be shown, read into an authenticator and confirmed with a code
// computed from it, which is two more commands with a person in between. So that case ends by
// handing over the pending credential and naming those two commands - and `hubctl mfa confirm`
// is what stores the session, because that is where the pair finally arrives.
func (cli *CLI) secondStep(
	ctx context.Context, client *Client, baseURL, tenant string, answer []byte, recovery bool,
) error {
	var challenge openapi.MfaChallenge
	if err := json.Unmarshal(answer, &challenge); err != nil {
		return fmt.Errorf("the installation owed a second step in a shape this client cannot read: %w", err)
	}

	if !offers(challenge, openapi.MfaChallengeMethodsTOTP) &&
		!offers(challenge, openapi.MfaChallengeMethodsRECOVERY) {
		return cli.enrolmentOwed(challenge)
	}

	completion := openapi.SignInCompletion{PendingToken: challenge.PendingToken}
	if recovery {
		code, err := cli.readCredential("", "Recovery code: ")
		if err != nil {
			return err
		}
		completion.RecoveryCode = &code
	} else {
		code, err := cli.readCredential(envTotp, "The authenticator's current code: ")
		if err != nil {
			return err
		}
		completion.Code = &code
	}

	_, verified, err := client.PostStatus(ctx, sessionVerifyPath, completion)
	if err != nil {
		return err
	}
	tokens, err := readSessionTokens(verified)
	if err != nil {
		return err
	}
	if tokens.RecoveryCodesRemaining != nil {
		printf(cli.Err, "hubctl: a recovery code was spent; %d remain\n", *tokens.RecoveryCodesRemaining)
	}
	return cli.holdSession(baseURL, tenant, tokens)
}

// enrolmentOwed ends a sign-in that cannot finish here, with the credential that lets the next two
// commands finish it. The pending token is a credential and is answered once, so it is printed
// like every other one this client meets: on standard output, where a script can read it, with
// the warning beside it on standard error.
func (cli *CLI) enrolmentOwed(challenge openapi.MfaChallenge) error {
	if err := cli.Emit(challenge, Table{
		Columns: []string{"second step", "pending credential", "expires"},
		Rows:    [][]string{{"enrolment", challenge.PendingToken, shortTime(&challenge.ExpiresAt)}},
	}); err != nil {
		return err
	}
	printf(cli.Err,
		"this workspace requires a second factor of its administrators, and this account has none yet.\n"+
			"The credential above is shown once and lives minutes:\n"+
			"  hubctl mfa enroll --pending <credential>\n"+
			"  hubctl mfa confirm --pending <credential>\n")
	return nil
}

func offers(challenge openapi.MfaChallenge, method openapi.MfaChallengeMethods) bool {
	for _, offered := range challenge.Methods {
		if offered == method {
			return true
		}
	}
	return false
}

// holdSession writes the pair into the profile, replacing whatever credential was there.
//
// One credential at a time, deliberately: a profile holding both a token and a session would leave
// which one a command used to the order of two fields, and "which credential was that call made
// with" is not a question a person should have to ask their configuration file.
func (cli *CLI) holdSession(baseURL, tenant string, tokens openapi.SessionTokens) error {
	profile := Profile{
		BaseURL: baseURL,
		Tenant:  tenant,
		Session: Session{
			ID:               tokens.Session.Id.String(),
			AccessToken:      secret.New(tokens.AccessToken),
			AccessExpiresAt:  tokens.AccessTokenExpiresAt,
			RefreshToken:     secret.New(tokens.RefreshToken),
			RefreshExpiresAt: tokens.RefreshTokenExpiresAt,
		},
	}
	if err := SaveProfile(cli.ProfilePath, profile); err != nil {
		return err
	}
	cli.Profile = profile
	// On standard error, for `hubctl auth login`'s reason: a sign-in produces no payload, and a
	// script that pipes standard output should get nothing rather than a sentence it has to skip.
	printf(cli.Err, "signed in to %s; the session is held in %s until %s\n",
		baseURL, cli.ProfilePath, tokens.RefreshTokenExpiresAt.Local().Format("2006-01-02 15:04"))
	return nil
}

// renewSession buys the next pair when the held one is nearly out, before the command runs.
//
// It happens here, once, rather than as a retry on a 401: the rotation retires the presented
// refresh token in the same moment it mints the next one, so a client that raced two renewals
// against each other would present a retired token, and a retired token presented again is read as
// theft - the whole family dies. One place, before anything else calls anything.
//
// A renewal that fails is not fatal. The family may be gone - revoked, expired, or invalidated by
// exactly the reuse above - and in that case the useful next command is `hubctl login`, which a
// fatal error here would prevent. So the dead session is forgotten and the run goes on with
// whatever is left, which is usually nothing and says so on the first call.
func (cli *CLI) renewSession(ctx context.Context) {
	session := cli.Profile.Session
	if session.IsEmpty() || !session.refreshDue(time.Now()) {
		return
	}

	renewed, err := cli.exchangeRefresh(ctx, session)
	if err != nil {
		printf(cli.Err, "hubctl: the session could not be renewed (%s); sign in again with `hubctl login`\n", err)
		cli.forgetSession()
		return
	}
	cli.Profile.Session = renewed
}

func (cli *CLI) exchangeRefresh(ctx context.Context, session Session) (Session, error) {
	bounded, cancel := context.WithTimeout(ctx, cli.Timeout)
	defer cancel()

	client, err := cli.anonymousClient(cli.Profile)
	if err != nil {
		return Session{}, err
	}
	var tokens openapi.SessionTokens
	if err := client.Post(bounded, sessionRefreshPath,
		openapi.SessionRefresh{RefreshToken: session.RefreshToken.Reveal()}, &tokens); err != nil {
		return Session{}, err
	}

	renewed := Session{
		ID:               tokens.Session.Id.String(),
		AccessToken:      secret.New(tokens.AccessToken),
		AccessExpiresAt:  tokens.AccessTokenExpiresAt,
		RefreshToken:     secret.New(tokens.RefreshToken),
		RefreshExpiresAt: tokens.RefreshTokenExpiresAt,
	}
	// Written before it is used. The exchange has already retired the old token on the server, so
	// a run that used the new pair and then failed to save it would leave the profile holding a
	// credential that is not merely stale but poisonous to present.
	stored, err := LoadProfile(cli.ProfilePath)
	if err != nil {
		return Session{}, err
	}
	stored.Session = renewed
	if err := SaveProfile(cli.ProfilePath, stored); err != nil {
		return Session{}, err
	}
	return renewed, nil
}

// forgetSession drops a session nothing can use any more, in memory and on disk. A failure to
// write is not worth reporting over the renewal failure that caused it: the credential is dead
// either way, and the sentence a person needs is the one already printed.
func (cli *CLI) forgetSession() {
	cli.Profile.Session = Session{}
	stored, err := LoadProfile(cli.ProfilePath)
	if err != nil {
		return
	}
	stored.Session = Session{}
	_ = SaveProfile(cli.ProfilePath, stored)
}

func (cli *CLI) anonymousClient(profile Profile) (*Client, error) {
	client, err := NewAnonymousClient(profile, cli.Catalogue, cli.Timeout)
	if err != nil {
		return nil, err
	}
	client.Notice = func(format string, args ...any) {
		printf(cli.Err, "hubctl: "+format+"\n", args...)
	}
	return client, nil
}

func readSessionTokens(answer []byte) (openapi.SessionTokens, error) {
	var tokens openapi.SessionTokens
	if err := json.Unmarshal(answer, &tokens); err != nil {
		return openapi.SessionTokens{}, fmt.Errorf(
			"the installation answered with something that is not a session: %w", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return openapi.SessionTokens{}, errors.New("the installation answered a session with half a pair")
	}
	return tokens, nil
}

// readCredential takes a password or a code from the environment or from one line of standard
// input, and never from an argument - the reason there is no `--token` flag either: an argument is
// visible in `ps` and lands in the shell history.
//
// One line rather than everything, which is what separates this from `auth login`'s reader: a
// sign-in under a second factor asks twice, and a reader that swallowed the whole of standard
// input the first time would leave the second question with nothing to read.
func (cli *CLI) readCredential(variable, question string) (string, error) {
	if variable != "" {
		if fromEnv := cli.Env(variable); fromEnv != "" {
			return strings.TrimSpace(fromEnv), nil
		}
	}

	// A prompt only where there is somebody to read it, and the honest warning `auth login`
	// carries: turning the terminal's echo off needs a dependency this binary does not have, and
	// a prompt that lied about being hidden would be worse than one that does not claim to be.
	if file, ok := cli.In.(*os.File); ok {
		if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			printf(cli.Err, "%s", question)
		}
	}
	if cli.lines == nil {
		cli.lines = bufio.NewReader(cli.In)
	}
	line, err := cli.lines.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("nothing to read for %q: pipe it in, or set %s",
			strings.TrimSpace(question), firstNonEmpty(variable, "it in the environment"))
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return "", errors.New("nothing was given for " + strings.TrimSpace(strings.TrimSuffix(question, ": ")))
	}
	return answer, nil
}
