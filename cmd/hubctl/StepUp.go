// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"errors"
	"strings"

	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The step-up, once, for every act that demands one (H-03, security.md §5).
//
// The server mints the demand in exactly one place - `stepup.Required`, "so that two operations
// cannot describe the same demand differently" - and this file is the client's half of that
// bargain: one place that recognises the refusal, asks the person, and repeats the call. A
// re-implementation per command is how `hubctl admin tenant delete` and `hubctl restore run` end
// up prompting differently for the same proof.
//
// What differs between the acts is only where the proof travels, and that is the caller's one
// line: `X-Hubtask-Step-Up` for the operations that take a header, and the request field for the
// restore, which has carried one since 0.4.5.

// envStepUp carries a proof that was minted elsewhere.
//
// It exists because the two halves of a privileged act need not be the same credential: the proof
// lands on the account (`StepUps.Consume` asks whether the token is fresh "on a live session of
// this account"), while the act itself may be made with a personal access token. That is not an
// oddity, it is the control plane's only shape - `admin:tenants` is a scope no session carries,
// and ending a workspace still demands proving the person afresh.
const envStepUp = "HUBTASK_STEP_UP"

const (
	stepUpPath = "/auth/step-up"
	// stepUpHeader is where the proof travels for every act but the restore. Spelled here rather
	// than imported from the server's package for `restTenantHeader`'s reason: a header name is
	// part of the published contract, and this binary reads the contract.
	stepUpHeader = "X-Hubtask-Step-Up"
)

// stepUpProof is the header carrying a proof, or no header at all where there is none yet.
func stepUpProof(token string) map[string][]string {
	if token == "" {
		return nil
	}
	return map[string][]string{stepUpHeader: {token}}
}

// proveAgain runs an act, and runs it once more with a fresh proof of identity if that is what the
// act was refused for.
//
// Once more, not in a loop: the proof is consumed by the one privileged action it is presented to,
// so a second refusal means the proof was rejected rather than missing, and asking a person for
// their password again on the same command would be a client arguing with a server.
func (cli *CLI) proveAgain(ctx context.Context, client *Client, act func(stepUp string) error) error {
	// A proof minted elsewhere travels on the first attempt rather than after a refusal. It is
	// single-use and lives minutes, and spending a round trip to discover it was wanted would
	// waste a good part of that.
	if carried := strings.TrimSpace(cli.Env(envStepUp)); carried != "" {
		return act(carried)
	}

	err := act("")
	methods, demanded := stepUpDemand(err)
	if !demanded {
		return err
	}

	token, err := cli.stepUp(ctx, client, methods)
	if err != nil {
		return err
	}
	return act(token)
}

// stepUpDemand reads the one refusal, and the methods it named. A 403 carrying any other code is
// somebody's answer to give, not this file's.
func stepUpDemand(err error) (string, bool) {
	var refusal APIError
	if !errors.As(err, &refusal) || refusal.DetailCode != stepup.CodeRequired {
		return "", false
	}
	return refusal.Params["methods"], true
}

// stepUp proves the person afresh and hands back the token the retry carries.
func (cli *CLI) stepUp(ctx context.Context, client *Client, methods string) (string, error) {
	// A step-up is a session's act - "a personal access token cannot prove a person afresh"
	// (auth.step_up_session_required). Said here rather than after the round trip, because the
	// answer is about this machine's configuration and a person who reads it has to sign in
	// rather than retype anything.
	if cli.Profile.Session.IsEmpty() {
		message, _ := cli.Catalogue.Message("auth.step_up_session_required", nil)
		return "", errorString(message + "\n  Mint one where you are signed in - `hubctl step-up` - " +
			"and pass it to this command in " + envStepUp + ".")
	}

	request, err := cli.stepUpRequest(methods)
	if err != nil {
		return "", err
	}
	var grant openapi.StepUpGrant
	if err := client.Post(ctx, stepUpPath, request, &grant); err != nil {
		return "", err
	}
	// On standard error, like every diagnostic: what happened is worth seeing, and it is not part
	// of the answer the command was asked for.
	printf(cli.Err, "hubctl: proved again with the %s; retrying\n", strings.ToLower(string(grant.Method)))
	return grant.StepUpToken, nil
}

// stepUpRequest asks for one of the two proofs the contract accepts - "one of the two, never
// both".
//
// The authenticator's code wins where there is one to hand, because it is the stronger of the two
// and because a session under an armed factor is one whose password has already been typed today.
// Where there is none, the password is what is left, and the demand's own list is what says
// whether that is accepted.
func (cli *CLI) stepUpRequest(methods string) (openapi.StepUpRequest, error) {
	offersTOTP := methods == "" || strings.Contains(methods, "TOTP")
	if offersTOTP && (cli.Env(envTotp) != "" || !strings.Contains(methods, "PASSWORD")) {
		code, err := cli.readCredential(envTotp, "This needs proving again. The authenticator's current code: ")
		if err != nil {
			return openapi.StepUpRequest{}, err
		}
		return openapi.StepUpRequest{Code: &code}, nil
	}

	password, err := cli.readCredential(envPassword, "This needs proving again. Password (it will be visible): ")
	if err != nil {
		return openapi.StepUpRequest{}, err
	}
	return openapi.StepUpRequest{Password: &password}, nil
}

func stepUpGroup() group {
	return group{
		name: "step-up",
		summary: "prove yourself again, and print the proof for a command that cannot ask for it " +
			"itself",
		usage: "",
		run:   mintStepUp,
	}
}

// mintStepUp is the proof as its own command, for the act whose credential cannot mint one.
//
// Every other privileged call asks for itself: it is refused, this client prompts, and it goes
// again. The control plane cannot, because it is reached with a personal access token and a token
// "cannot prove a person afresh" - so the proof is minted here, in the session, and carried to
// that command in the environment. The account is what ties the two together.
func mintStepUp(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "step-up", "", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	// No methods named, because nothing refused anything here: the demand's own list is what
	// narrows the choice, and there is no demand to read.
	token, err := cli.stepUp(ctx, client, "")
	if err != nil {
		return err
	}

	printf(cli.Out, "%s\n", token)
	printf(cli.Err,
		"that proof is single-use, lives minutes, and is shown once: pass it in %s to the one "+
			"command it is for\n", envStepUp)
	return nil
}
