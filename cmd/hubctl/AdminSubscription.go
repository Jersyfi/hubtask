// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strconv"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The installation's subscriptions for a purchase platform (UC-INS-14): the addresses its events
// are pushed to. A new subscription and a new secret are answered once and demand a fresh step-up,
// as the API does.

const adminSubscriptionsPath = "/admin/subscriptions"

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
