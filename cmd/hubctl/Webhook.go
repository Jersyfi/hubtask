// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The outbound half of the integration surface: who is being told what happens here, what
// reached them, and what to do when it did not.
//
// Two things this group is careful about. A signing secret is answered once - at creation and at
// each rotation - and there is no read that can produce it again, so both commands that see one
// say so on standard error (project-structure.md §3). And a rotation's grace period is named in the
// command rather than defaulted silently: how long the old secret keeps verifying is the
// difference between a deployment and an outage, and zero is what a leak calls for.

const webhooksPath = "/integrations/webhooks"

func webhookGroup() group {
	return group{
		name:    "webhook",
		summary: "outbound subscriptions, their deliveries, and their secrets",
		commands: []command{
			{
				name:    "add",
				usage:   "--url <target> --event <type> [--event <type>…] [--filter <condition>] [--scope HUB|COLLECTION --scope-id <id>] [--format MATRIX|SLACK|MATTERMOST|TEAMS [--room <id>] [--locale <tag>]]",
				summary: "subscribe an external system - the secret is shown once",
				run:     webhookAdd,
			},
			{
				name:    "pause",
				usage:   "<id>",
				summary: "stop delivering deliberately, which is not what DISABLED means",
				run:     webhookPause,
			},
			{name: "resume", usage: "<id>", summary: "start delivering again", run: webhookResume},
			{
				name:    "rotate-secret",
				usage:   "<id> [--grace <seconds>]",
				summary: "mint a new signing secret - shown once, with a grace period for the old one",
				run:     webhookRotateSecret,
			},
		},
	}
}

func webhookAdd(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "webhook", "add", "--url <target> --event <type> [--event <type>…] [--filter <condition>] [--scope HUB|COLLECTION --scope-id <id>] [--format MATRIX|SLACK|MATTERMOST|TEAMS [--room <id>] [--locale <tag>]]")
	target := flags.String("url", "", "where the deliveries go")
	var events stringList
	flags.Var(&events, "event", "an event type to subscribe to; repeat for several")
	filter := flags.String("filter", "", "a condition an event must meet to be sent, over event, data, actor and now")
	scope := flags.String("scope", "", "HUB or COLLECTION to listen there only; omitted is the whole workspace")
	scopeID := flags.String("scope-id", "", "the hub or the collection --scope names")
	format := flags.String("format", "", "MATRIX, SLACK, MATTERMOST or TEAMS to tell a chat room; omitted is the signed CloudEvent")
	room := flags.String("room", "", "the Matrix room identifier, !opaque:server")
	locale := flags.String("locale", "", "the language of a chat room's sentence, such as de")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *target == "" || len(events) == 0 {
		return usagef("webhook add needs --url and at least one --event")
	}
	create := openapi.WebhookSubscriptionCreate{TargetUrl: *target, EventTypes: events}
	if *filter != "" {
		create.Filter = filter
	}
	chat := false
	if *format != "" {
		chosen := openapi.WebhookFormat(strings.ToUpper(*format))
		create.Format = &chosen
		chat = chosen != openapi.WebhookFormatCLOUDEVENTS
		if chosen == openapi.WebhookFormatMATRIX {
			token, err := readChatToken(cli)
			if err != nil {
				return err
			}
			create.Credential = &token
		}
	}
	if *room != "" {
		create.Room = room
	}
	if *locale != "" {
		create.Locale = locale
	}
	if *scope != "" || *scopeID != "" {
		// Both or neither: a scope without its container, or an identifier without saying what it
		// names, is a mistake this client can name before the server does.
		if *scope == "" || *scopeID == "" {
			return usagef("webhook add needs --scope and --scope-id together")
		}
		parsed, err := cli.parseID("--scope-id", *scopeID)
		if err != nil {
			return err
		}
		create.Scope = &openapi.WebhookScope{Type: openapi.WebhookScopeType(strings.ToUpper(*scope)), Id: &parsed}
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.WebhookSubscriptionSecret
	if err := client.Post(ctx, webhooksPath, create, &created); err != nil {
		return err
	}
	if !cli.JSON && !chat {
		printf(cli.Err,
			"that signing secret is shown once: store it now, and rotate the subscription if it leaks\n")
	}
	return cli.Emit(created, Table{
		Columns: []string{"id", "target", "events", "state", "secret"},
		Rows: [][]string{{
			created.Id.String(), created.TargetUrl,
			strings.Join(created.EventTypes, ","), string(created.State), created.Secret,
		}},
	})
}

func webhookPause(ctx context.Context, cli *CLI, args []string) error {
	return webhookState(ctx, cli, args, "pause", openapi.WebhookSubscriptionUpdateStatePAUSED)
}

func webhookResume(ctx context.Context, cli *CLI, args []string) error {
	return webhookState(ctx, cli, args, "resume", openapi.WebhookSubscriptionUpdateStateACTIVE)
}

// webhookState is both halves of the switch. `DISABLED` is deliberately not among them: it is what
// the system concludes from a run of failures, and the two say different things to whoever reads
// the list.
func webhookState(
	ctx context.Context, cli *CLI, args []string, verb string,
	state openapi.WebhookSubscriptionUpdateState,
) error {
	webhookID, err := cli.onlyID(args, "webhook "+verb+" <id>")
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var updated openapi.WebhookSubscription
	if err := client.Patch(ctx, webhooksPath+"/"+webhookID.String(),
		openapi.WebhookSubscriptionUpdate{State: &state}, &updated); err != nil {
		return err
	}
	return cli.Emit(updated, webhookTable([]openapi.WebhookSubscription{updated}))
}

func webhookRotateSecret(ctx context.Context, cli *CLI, args []string) error {
	const usage = "webhook rotate-secret <id> [--grace <seconds>]"
	webhookID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "webhook", "rotate-secret", "<id> [--grace <seconds>]")
	grace := flags.Int("grace", -1,
		"how long the old secret keeps verifying; 0 retires it at once, which is what a leak calls for")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	rotation := openapi.WebhookSecretRotation{}
	if *grace >= 0 {
		rotation.GraceSeconds = grace
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var rotated openapi.WebhookSubscriptionSecret
	if err := client.Post(ctx,
		webhooksPath+"/"+webhookID.String()+":rotate-secret", rotation, &rotated); err != nil {
		return err
	}
	if !cli.JSON {
		printf(cli.Err,
			"that signing secret is shown once: deploy it before the grace period ends, or deliveries stop verifying\n")
	}
	return cli.Emit(rotated, Table{
		Columns: []string{"id", "target", "state", "secret"},
		Rows: [][]string{{
			rotated.Id.String(), rotated.TargetUrl, string(rotated.State), rotated.Secret,
		}},
	})
}

// envChatToken carries a Matrix account's access token to `webhook add --format MATRIX`.
const envChatToken = "HUBTASK_CHAT_TOKEN"

// readChatToken takes the room's token from the environment or standard input, never from an
// argument, for readToken's reason: an argument is visible in `ps` and the shell history.
func readChatToken(cli *CLI) (string, error) {
	if fromEnv := strings.TrimSpace(cli.Env(envChatToken)); fromEnv != "" {
		return fromEnv, nil
	}
	raw, err := io.ReadAll(cli.In)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", usagef("a Matrix room needs its access token: pipe it in, or set " + envChatToken)
	}
	return token, nil
}

// stringList is a flag that may be repeated. A list rather than a comma-separated string, because
// an event type is a value somebody pastes and a comma inside one would be a silent truncation.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return usagef("an empty value is not an event type")
	}
	*l = append(*l, trimmed)
	return nil
}

func webhookTable(subscriptions []openapi.WebhookSubscription) Table {
	rows := make([][]string, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		rows = append(rows, []string{
			subscription.Id.String(),
			subscription.TargetUrl,
			strings.Join(subscription.EventTypes, ","),
			webhookScope(subscription.Scope),
			strconv.FormatBool(subscription.Filter != nil),
			string(subscription.State),
			strconv.Itoa(subscription.FailureCount),
		})
	}
	return Table{
		Columns: []string{"id", "target", "events", "scope", "condition", "state", "failures"},
		Rows:    rows,
	}
}

// webhookScope is where a subscription listens, as one cell: TENANT, or the level and the container.
func webhookScope(scope *openapi.WebhookScope) string {
	if scope == nil || scope.Id == nil {
		return string(openapi.WebhookScopeTypeTENANT)
	}
	return string(scope.Type) + ":" + scope.Id.String()
}
