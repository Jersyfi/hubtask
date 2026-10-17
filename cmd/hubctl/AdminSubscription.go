// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strconv"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The installation's subscriptions for a purchase platform (UC-INS-14): the addresses its events
// are pushed to. A new subscription and a new secret are answered once and demand a fresh step-up,
// as the API does.

const adminSubscriptionsPath = "/admin/subscriptions"

func adminSubscription(ctx context.Context, cli *CLI, args []string) error {
	const verbs = "ls, show, add, pause, resume, rm, rotate-secret, deliveries, redeliver"
	if len(args) == 0 {
		return usagef("admin subscription needs a command: %s", verbs)
	}
	switch args[0] {
	case "ls":
		return adminSubscriptionList(ctx, cli, args[1:])
	case "show":
		return adminSubscriptionShow(ctx, cli, args[1:])
	case "add":
		return adminSubscriptionAdd(ctx, cli, args[1:])
	case "pause":
		return adminSubscriptionState(ctx, cli, args[1:], "pause", openapi.PlatformSubscriptionUpdateStatePAUSED)
	case "resume":
		return adminSubscriptionState(ctx, cli, args[1:], "resume", openapi.PlatformSubscriptionUpdateStateACTIVE)
	case "rm":
		return adminSubscriptionRemove(ctx, cli, args[1:])
	case "rotate-secret":
		return adminSubscriptionRotate(ctx, cli, args[1:])
	case "deliveries":
		return adminSubscriptionDeliveries(ctx, cli, args[1:])
	case "redeliver":
		return adminSubscriptionRedeliver(ctx, cli, args[1:])
	default:
		return usagef("admin subscription has no command %q: %s", args[0], verbs)
	}
}

func subscriptionTable(subscriptions []openapi.PlatformSubscription) Table {
	rows := make([][]string, 0, len(subscriptions))
	for _, s := range subscriptions {
		last := "-"
		if s.LastDeliveryAt != nil {
			last = s.LastDeliveryAt.Format("2006-01-02 15:04")
		}
		rows = append(rows, []string{
			s.Id.String(), s.TargetUrl, string(s.State), strconv.Itoa(s.DeadLetters), last, text(s.LastError),
		})
	}
	return Table{Columns: []string{"id", "target", "state", "dead letters", "last delivery", "last error"}, Rows: rows}
}

func adminSubscriptionList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin subscription", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var subscriptions []openapi.PlatformSubscription
	if err := client.Get(ctx, adminSubscriptionsPath, nil, &subscriptions); err != nil {
		return err
	}
	return cli.Emit(subscriptions, subscriptionTable(subscriptions))
}

func adminSubscriptionShow(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin subscription show <id>"
	id, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	if err := parseOnlyFlags(commandFlags(cli, "admin subscription", "show", "<id>"), rest, usage); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var subscription openapi.PlatformSubscription
	if err := client.Get(ctx, adminSubscriptionsPath+"/"+id.String(), nil, &subscription); err != nil {
		return err
	}
	return cli.Emit(subscription, subscriptionTable([]openapi.PlatformSubscription{subscription}))
}

// emitSecret prints a subscription with its secret, which nothing answers again.
func emitSecret(cli *CLI, minted openapi.PlatformSubscriptionSecret) error {
	return cli.Emit(minted, Table{
		Columns: []string{"id", "target", "secret"},
		Rows:    [][]string{{minted.Id.String(), minted.TargetUrl, minted.Secret}},
	})
}

func adminSubscriptionAdd(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin subscription add --target <url>"
	flags := commandFlags(cli, "admin subscription", "add", "--target <url>")
	target := flags.String("target", "", "where the installation's events are POSTed")
	if err := parseOnlyFlags(flags, args, usage); err != nil {
		return err
	}
	if *target == "" {
		return usagef("admin subscription add needs --target")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var minted openapi.PlatformSubscriptionSecret
	if err := cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.PostWithHeader(ctx, adminSubscriptionsPath,
			openapi.PlatformSubscriptionCreate{TargetUrl: *target}, stepUpProof(stepUp), &minted)
	}); err != nil {
		return err
	}
	return emitSecret(cli, minted)
}

func adminSubscriptionState(
	ctx context.Context, cli *CLI, args []string, verb string, state openapi.PlatformSubscriptionUpdateState,
) error {
	usage := "admin subscription " + verb + " <id>"
	id, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	if err := parseOnlyFlags(commandFlags(cli, "admin subscription", verb, "<id>"), rest, usage); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var changed openapi.PlatformSubscription
	if err := client.Patch(ctx, adminSubscriptionsPath+"/"+id.String(),
		openapi.PlatformSubscriptionUpdate{State: &state}, &changed); err != nil {
		return err
	}
	return cli.Emit(changed, subscriptionTable([]openapi.PlatformSubscription{changed}))
}

func adminSubscriptionRemove(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin subscription rm <id>"
	id, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	if err := parseOnlyFlags(commandFlags(cli, "admin subscription", "rm", "<id>"), rest, usage); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, adminSubscriptionsPath+"/"+id.String(), ""); err != nil {
		return err
	}
	return adminSubscriptionList(ctx, cli, nil)
}

func adminSubscriptionRotate(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin subscription rotate-secret <id>"
	id, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	if err := parseOnlyFlags(commandFlags(cli, "admin subscription", "rotate-secret", "<id>"), rest, usage); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var minted openapi.PlatformSubscriptionSecret
	if err := cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.PostWithHeader(ctx, adminSubscriptionsPath+"/"+id.String()+":rotate-secret",
			nil, stepUpProof(stepUp), &minted)
	}); err != nil {
		return err
	}
	return emitSecret(cli, minted)
}

func adminSubscriptionDeliveries(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin subscription deliveries <id> [--status <status>] [--cursor <cursor>]"
	id, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin subscription", "deliveries", "<id> [--status <status>] [--cursor <cursor>]")
	status := flags.String("status", "", "PENDING, SUCCEEDED, FAILED or DEAD_LETTER")
	cursor := flags.String("cursor", "", "where the last page ended")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	query := url.Values{}
	if *status != "" {
		query.Set("status", *status)
	}
	if *cursor != "" {
		query.Set("cursor", *cursor)
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var page openapi.PlatformDeliveryPage
	if err := client.Get(ctx, adminSubscriptionsPath+"/"+id.String()+"/deliveries", query, &page); err != nil {
		return err
	}
	rows := make([][]string, 0, len(page.Data))
	for _, d := range page.Data {
		answer := "-"
		if d.ResponseStatus != nil {
			answer = strconv.Itoa(*d.ResponseStatus)
		}
		rows = append(rows, []string{
			d.EventId.String(), d.EventType, string(d.Status), strconv.Itoa(d.Attempt), answer, text(d.ErrorCode),
		})
	}
	if err := cli.Emit(page, Table{
		Columns: []string{"event", "type", "status", "attempts", "answer", "error"}, Rows: rows,
	}); err != nil {
		return err
	}
	if page.Page.NextCursor != nil && *page.Page.NextCursor != "" {
		printf(cli.Err, "continue with --cursor %s\n", *page.Page.NextCursor)
	}
	return nil
}

func adminSubscriptionRedeliver(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin subscription redeliver <id>"
	id, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	if err := parseOnlyFlags(commandFlags(cli, "admin subscription", "redeliver", "<id>"), rest, usage); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var queued struct {
		Queued int `json:"queued"`
	}
	if err := client.Post(ctx, adminSubscriptionsPath+"/"+id.String()+":redeliver", nil, &queued); err != nil {
		return err
	}
	return cli.Emit(queued, Table{Columns: []string{"queued"}, Rows: [][]string{{strconv.Itoa(queued.Queued)}}})
}
