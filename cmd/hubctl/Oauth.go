// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The OAuth2 provider from a terminal (H-05).
//
// The authorization endpoint is headless (0.6.0 decision 5), which is what makes the dance
// scriptable at all: there is no browser to redirect, so the three moves are three commands -
// register the app, consent to it, exchange the code for the pair. What a consent *screen* will
// do when the client track builds one is call exactly the middle one.
//
// Consenting is a person's act - "never a token, a person" - so `hubctl oauth authorize` needs the
// session `hubctl login` holds. The exchange is the opposite: public, because what authenticates
// is in the body, and a bearer credential presented beside it would be a second identity the
// route does not accept.

const (
	oauthClientsPath = "/oauth/clients"
	oauthAuthorize   = "/oauth/authorize"
	oauthTokenPath   = "/oauth/token" //nolint:gosec // G101: a route, not a credential
	oauthGrantsPath  = "/oauth/grants"
)

// envClientSecret carries a confidential client's credential into the exchange, for the reason no
// credential is a flag anywhere else in this binary: an argument is visible in `ps`.
const envClientSecret = "HUBTASK_OAUTH_CLIENT_SECRET" //nolint:gosec // G101: the name of an environment variable

// verifierBytes is RFC 7636's verifier as this client draws it: 32 bytes of entropy, which is 43
// base64url characters - the minimum the specification allows and the maximum entropy that fits
// in it.
const verifierBytes = 32

func oauthGroup() group {
	return group{
		name:    "oauth",
		summary: "third-party apps: registering them, consenting, and what they were allowed",
		commands: []command{
			{
				name:    "client",
				usage:   "add|ls|rm …",
				summary: "the apps that may ask people here for bounded access",
				run:     oauthClient,
			},
			{
				name:    "authorize",
				usage:   "--client <id> --redirect <uri> --scope <a,b> [--state <s>] [--verifier <v>]",
				summary: "consent to an app, and receive the single-use code it exchanges",
				run:     oauthConsent,
			},
			{
				name:    "token",
				usage:   "--client <id> --redirect <uri> --code <c> --verifier <v> [--confidential]",
				summary: "exchange the code for the pair, as the app would",
				run:     oauthExchange,
			},
			{
				name:    "grant",
				usage:   "ls|revoke …",
				summary: "what this account allowed, and withdrawing it",
				run:     oauthGrant,
			},
		},
	}
}

func oauthClient(ctx context.Context, cli *CLI, args []string) error {
	if len(args) == 0 {
		return usagef("oauth client needs a command: add, ls, rm")
	}
	switch args[0] {
	case "add":
		return oauthClientAdd(ctx, cli, args[1:])
	case "ls":
		return oauthClientList(ctx, cli, args[1:])
	case "rm":
		return oauthClientRemove(ctx, cli, args[1:])
	default:
		return usagef("oauth client has no command %q: add, ls, rm", args[0])
	}
}

// oauthClientAdd registers an app. A confidential one is answered a secret, in clear, once.
func oauthClientAdd(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "oauth client", "add",
		"--name <name> --redirect <uri,uri> [--confidential]")
	name := flags.String("name", "", "what people read on the consent screen and in their grant list")
	redirects := flags.String("redirect", "", "the exact redirect URIs, comma separated")
	confidential := flags.Bool("confidential", false,
		"the app can keep a secret on a server; without this it is public and must bring PKCE")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *name == "" || *redirects == "" {
		return usagef("registering an app needs --name and at least one --redirect")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var registered openapi.OauthClientSecret
	if err := client.Post(ctx, oauthClientsPath, openapi.OauthClientCreate{
		Name:         *name,
		RedirectUris: splitList(*redirects),
		Confidential: *confidential,
	}, &registered); err != nil {
		return err
	}

	if cli.JSON {
		return cli.Emit(registered, Table{})
	}
	cli.emitTable(Table{
		Columns: []string{"id", "name", "confidential", "redirect uris"},
		Rows: [][]string{{
			registered.Id.String(), registered.Name,
			map[bool]string{true: "yes", false: "no"}[registered.Confidential],
			strings.Join(registered.RedirectUris, " "),
		}},
	})
	// A public client is answered none, and printing an empty line for it would look like a
	// secret that failed to arrive.
	if registered.ClientSecret != nil && *registered.ClientSecret != "" {
		printf(cli.Out, "%s\n", *registered.ClientSecret)
		printf(cli.Err, "that client secret is shown once: store it, and register again if it is lost\n")
	}
	return nil
}

func oauthClientList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "oauth client", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var clients []openapi.OauthClient
	if err := client.Get(ctx, oauthClientsPath, nil, &clients); err != nil {
		return err
	}
	rows := make([][]string, 0, len(clients))
	for _, registered := range clients {
		rows = append(rows, []string{
			registered.Id.String(), registered.Name,
			map[bool]string{true: "yes", false: "no"}[registered.Confidential],
			strings.Join(registered.RedirectUris, " "),
			shortTime(&registered.CreatedAt),
		})
	}
	return cli.Emit(clients, Table{
		Columns: []string{"id", "name", "confidential", "redirect uris", "registered"},
		Rows:    rows,
	})
}

// oauthClientRemove takes the app away, and every grant that pointed at it with it - "a grant to a
// client nobody can name is a door with no label".
func oauthClientRemove(ctx context.Context, cli *CLI, args []string) error {
	const usage = "oauth client rm <id>"
	clientID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "oauth client", "rm", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	api, err := cli.client()
	if err != nil {
		return err
	}
	if err := api.Delete(ctx, oauthClientsPath+"/"+clientID.String(), ""); err != nil {
		return err
	}
	printf(cli.Err, "the app is gone, and every grant that pointed at it is withdrawn\n")
	return nil
}

// OauthConsent is what `hubctl oauth authorize` answers: the contract's code, and the verifier
// this client drew in order to ask for it.
//
// The verifier is not in the contract because it never travels to the server - only its hash does
// (RFC 7636). But the exchange needs it, and the two calls are two commands here, so it has to
// come back out. It is documented as this client's own shape for `auth status`'s reason: `--json`
// makes it something a script depends on.
type OauthConsent struct {
	openapi.OauthCode
	// CodeVerifier is the other half of the code, and useless without it: the exchange sends the
	// verifier, the server hashes it, and the result has to equal the challenge the code was
	// minted under.
	CodeVerifier string `json:"code_verifier"`
}

func oauthConsent(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "oauth", "authorize",
		"--client <id> --redirect <uri> --scope <a,b> [--state <s>] [--verifier <v>]")
	clientID := flags.String("client", "", "the app being allowed")
	redirect := flags.String("redirect", "", "one of the app's registered redirect URIs, exactly")
	scopes := flags.String("scope", "", "the scopes to allow, comma separated")
	state := flags.String("state", "", "echoed back untouched, for the app's own CSRF binding")
	verifier := flags.String("verifier", "",
		"the PKCE verifier to use; one is drawn and printed when this is not given")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *clientID == "" || *redirect == "" || *scopes == "" {
		return usagef("consenting needs --client, --redirect and --scope")
	}
	app, err := cli.parseID("--client", *clientID)
	if err != nil {
		return err
	}

	secretVerifier := *verifier
	if secretVerifier == "" {
		secretVerifier, err = drawVerifier()
		if err != nil {
			return err
		}
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var code openapi.OauthCode
	if err := client.Post(ctx, oauthAuthorize, openapi.OauthAuthorization{
		ClientId:    app,
		RedirectUri: *redirect,
		Scopes:      splitList(*scopes),
		// S256 is the only method the contract accepts - `plain` would put the verifier on the
		// wire twice - so there is nothing to choose here.
		CodeChallenge:       challengeFor(secretVerifier),
		CodeChallengeMethod: "S256",
		State:               optional(*state),
	}, &code); err != nil {
		return err
	}

	consent := OauthConsent{OauthCode: code, CodeVerifier: secretVerifier}
	if err := cli.Emit(consent, Table{
		Columns: []string{"code", "verifier", "expires", "state"},
		Rows: [][]string{{
			code.Code, secretVerifier, shortTime(&code.ExpiresAt), text(code.State),
		}},
	}); err != nil {
		return err
	}
	printf(cli.Err,
		"the code is single-use and lives minutes, and the verifier is its other half: "+
			"both are shown once, and `hubctl oauth token` needs them together\n")
	return nil
}

// oauthExchange is the app's own call, made from here so a scripted session can prove the whole
// dance rather than the two thirds of it a person does.
//
// Anonymous, because the route is public and what authenticates is in the body: the single-use
// code, the verifier whose hash has to equal the challenge, and - for a confidential client - the
// secret. A bearer credential beside them would be a second identity the route does not accept.
func oauthExchange(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "oauth", "token",
		"--client <id> --redirect <uri> --code <c> --verifier <v> [--confidential]")
	clientID := flags.String("client", "", "the app the code was minted for")
	redirect := flags.String("redirect", "", "the authorization's redirect URI, repeated exactly")
	code := flags.String("code", "", "the single-use code `hubctl oauth authorize` answered")
	verifier := flags.String("verifier", "", "the PKCE verifier that code was minted under")
	confidential := flags.Bool("confidential", false,
		"the app keeps a secret; it is read from "+envClientSecret+" or from standard input")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *clientID == "" || *redirect == "" || *code == "" {
		return usagef("the exchange needs --client, --redirect and --code")
	}
	app, err := cli.parseID("--client", *clientID)
	if err != nil {
		return err
	}

	request := openapi.OauthTokenRequest{
		GrantType:    "authorization_code",
		Code:         *code,
		RedirectUri:  *redirect,
		ClientId:     app,
		CodeVerifier: optional(*verifier),
	}
	if *confidential {
		clientSecret, err := cli.readCredential(envClientSecret, "The app's client secret: ")
		if err != nil {
			return err
		}
		request.ClientSecret = &clientSecret
	}

	client, err := cli.anonymousClient(cli.Profile)
	if err != nil {
		return err
	}
	var tokens openapi.SessionTokens
	if err := client.Post(ctx, oauthTokenPath, request, &tokens); err != nil {
		return err
	}

	// Not held in the profile: this pair belongs to the app, not to this shell. It is printed
	// once, where the app - or the script standing in for one - can read it.
	if cli.JSON {
		return cli.Emit(tokens, Table{})
	}
	cli.emitTable(Table{
		Columns: []string{"token type", "session", "access expires", "refresh expires"},
		Rows: [][]string{{
			string(tokens.TokenType), tokens.Session.Id.String(),
			shortTime(&tokens.AccessTokenExpiresAt), shortTime(&tokens.RefreshTokenExpiresAt),
		}},
	})
	printf(cli.Out, "%s\n", tokens.AccessToken)
	printf(cli.Err,
		"that pair is the app's credential and is shown once; revoking the grant refuses it "+
			"on its next request\n")
	return nil
}

func oauthGrant(ctx context.Context, cli *CLI, args []string) error {
	if len(args) == 0 {
		return usagef("oauth grant needs a command: ls, revoke")
	}
	switch args[0] {
	case "ls":
		return oauthGrantList(ctx, cli, args[1:])
	case "revoke":
		return oauthGrantRevoke(ctx, cli, args[1:])
	default:
		return usagef("oauth grant has no command %q: ls, revoke", args[0])
	}
}

func oauthGrantList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "oauth grant", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var grants []openapi.OauthGrant
	if err := client.Get(ctx, oauthGrantsPath, nil, &grants); err != nil {
		return err
	}
	rows := make([][]string, 0, len(grants))
	for _, grant := range grants {
		rows = append(rows, []string{
			grant.Id.String(), grant.ClientName, strings.Join(grant.Scopes, " "),
			shortTime(&grant.CreatedAt), shortTime(grant.LastUsedAt),
		})
	}
	return cli.Emit(grants, Table{
		Columns: []string{"id", "app", "allowed", "since", "last used"},
		Rows:    rows,
	})
}

func oauthGrantRevoke(ctx context.Context, cli *CLI, args []string) error {
	const usage = "oauth grant revoke <id>"
	grantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "oauth grant", "revoke", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, oauthGrantsPath+"/"+grantID.String(), ""); err != nil {
		return err
	}
	printf(cli.Err, "withdrawn; every session the grant leashed refuses on its next request\n")
	return nil
}

// drawVerifier draws RFC 7636's code verifier. base64url without padding, which is exactly the
// unreserved alphabet the specification allows.
func drawVerifier() (string, error) {
	material := make([]byte, verifierBytes)
	if _, err := rand.Read(material); err != nil {
		return "", fmt.Errorf("drawing the PKCE verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(material), nil
}

// challengeFor is BASE64URL(SHA256(verifier)), which is the whole of S256.
func challengeFor(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// splitList reads a comma-separated flag, dropping the empty entries a trailing comma leaves.
func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
