// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The outbound half of the integration surface (G-13): who is being told what happens here, what
// reached them, and what to do when it did not.
//
// Two things this group is careful about. A signing secret is answered once - at creation and at
// each rotation - and there is no read that can produce it again, so both commands that see one
// say so on standard error (D-09's discipline). And a rotation's grace period is named in the
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
				usage:   "--url <target> --event <type> [--event <type>…]",
				summary: "subscribe an external system - the secret is shown once",
				run:     webhookAdd,
			},
			{name: "ls", usage: "", summary: "the workspace's subscriptions", run: webhookList},
			{name: "show", usage: "<id>", summary: "one subscription", run: webhookShow},
			{
				name:    "pause",
				usage:   "<id>",
				summary: "stop delivering deliberately, which is not what DISABLED means",
				run:     webhookPause,
			},
			{name: "resume", usage: "<id>", summary: "start delivering again", run: webhookResume},
			{name: "rm", usage: "<id>", summary: "unsubscribe", run: webhookRemove},
			{
				name:    "deliveries",
				usage:   "<id> [--status <s>] [--cursor <c>] [--size <n>]",
				summary: "what was sent to it, newest first",
				run:     webhookDeliveries,
			},
			{
				name:    "replay",
				usage:   "<id> <delivery id>",
				summary: "send one delivery again, carrying the event identifier it carried",
				run:     webhookReplay,
			},
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
	flags := commandFlags(cli, "webhook", "add", "--url <target> --event <type> [--event <type>…]")
	target := flags.String("url", "", "where the deliveries go")
	var events stringList
	flags.Var(&events, "event", "an event type to subscribe to; repeat for several")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *target == "" || len(events) == 0 {
		return usagef("webhook add needs --url and at least one --event")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.WebhookSubscriptionSecret
	if err := client.Post(ctx, webhooksPath, openapi.WebhookSubscriptionCreate{
		TargetUrl: *target, EventTypes: events,
	}, &created); err != nil {
		return err
	}
	if !cli.JSON {
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

func webhookList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "webhook", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var subscriptions []openapi.WebhookSubscription
	if err := client.Get(ctx, webhooksPath, nil, &subscriptions); err != nil {
		return err
	}
	return cli.Emit(subscriptions, webhookTable(subscriptions))
}

func webhookShow(ctx context.Context, cli *CLI, args []string) error {
	webhookID, err := cli.onlyID(args, "webhook show <id>")
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var subscription openapi.WebhookSubscription
	if err := client.Get(ctx, webhooksPath+"/"+webhookID.String(), nil, &subscription); err != nil {
		return err
	}
	return cli.Emit(subscription, webhookTable([]openapi.WebhookSubscription{subscription}))
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

func webhookRemove(ctx context.Context, cli *CLI, args []string) error {
	webhookID, err := cli.onlyID(args, "webhook rm <id>")
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, webhooksPath+"/"+webhookID.String(), ""); err != nil {
		return err
	}
	printf(cli.Err, "unsubscribed: %s (nothing more is delivered to it)\n", webhookID.String())
	return nil
}

func webhookDeliveries(ctx context.Context, cli *CLI, args []string) error {
	const usage = "webhook deliveries <id> [--status <s>] [--cursor <c>] [--size <n>]"
	webhookID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "webhook", "deliveries", "<id> [--status <s>]")
	status := flags.String("status", "", "PENDING, SUCCEEDED, FAILED or DEAD_LETTER")
	cursor := flags.String("cursor", "", "continue the previous page")
	size := flags.Int("size", 0, "how many at most")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	query := url.Values{}
	if *status != "" {
		query.Set("status", *status)
	}
	if *size > 0 {
		query.Set("size", strconv.Itoa(*size))
	}
	if *cursor != "" {
		query.Set("cursor", *cursor)
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var page openapi.WebhookDeliveryPage
	if err := client.Get(ctx,
		webhooksPath+"/"+webhookID.String()+"/deliveries", query, &page); err != nil {
		return err
	}
	if err := cli.Emit(page, deliveryTable(page.Data)); err != nil {
		return err
	}
	cli.reportMore(page.Page)
	return nil
}

// webhookReplay takes two identifiers, and the order is the path's: the subscription owns the
// delivery, and a delivery identifier on its own names nothing.
func webhookReplay(ctx context.Context, cli *CLI, args []string) error {
	const usage = "webhook replay <id> <delivery id>"
	webhookID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	deliveryID, rest, err := cli.takeID(rest, usage)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("unexpected argument %q: hubctl %s", rest[0], usage)
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var delivery openapi.WebhookDelivery
	if err := client.Post(ctx, webhooksPath+"/"+webhookID.String()+
		"/deliveries/"+deliveryID.String()+":replay", nil, &delivery); err != nil {
		return err
	}
	if !cli.JSON {
		// The same event identifier, deliberately: a subscriber that deduplicates on it recognises
		// the repeat rather than acting twice (automation.md §3.1).
		printf(cli.Err, "queued again, carrying the event it carried: %s\n", delivery.EventId.String())
	}
	return cli.Emit(delivery, deliveryTable([]openapi.WebhookDelivery{delivery}))
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
			string(subscription.State),
			strconv.Itoa(subscription.FailureCount),
		})
	}
	return Table{
		Columns: []string{"id", "target", "events", "state", "failures"},
		Rows:    rows,
	}
}

func deliveryTable(deliveries []openapi.WebhookDelivery) Table {
	rows := make([][]string, 0, len(deliveries))
	for _, delivery := range deliveries {
		rows = append(rows, []string{
			delivery.Id.String(),
			delivery.EventId.String(),
			string(delivery.Status),
			strconv.Itoa(delivery.Attempt),
			responseStatus(delivery),
			text(delivery.ErrorCode),
		})
	}
	return Table{
		Columns: []string{"id", "event", "status", "attempt", "answer", "error"},
		Rows:    rows,
	}
}

// responseStatus is what the target said, and a dash where it said nothing at all - a timeout and
// a 500 are different failures and the column has to be able to show which.
func responseStatus(delivery openapi.WebhookDelivery) string {
	if delivery.ResponseStatus == nil {
		return "-"
	}
	return strconv.Itoa(*delivery.ResponseStatus)
}
