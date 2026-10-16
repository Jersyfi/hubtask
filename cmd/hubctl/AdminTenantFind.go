// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strconv"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// One workspace, the platform's reference on it, and finding workspaces (multi-tenancy.md §4.3):
// the reads a platform reconciles its records with, at the terminal.

func adminTenantShow(ctx context.Context, cli *CLI, args []string) error {
	usage := "admin tenant show <id>"
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "show", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var tenant openapi.AdminTenant
	if err := client.Get(ctx, adminTenantsPath+"/"+tenantID.String(), nil, &tenant); err != nil {
		return err
	}
	return cli.Emit(tenant, tenantTable([]openapi.AdminTenant{tenant}))
}

// adminTenantFind sends the filters in a body, not a query string: a reference may name a
// customer, and a query string travels through access logs.
func adminTenantFind(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin tenant", "find",
		"[--reference <ref>] [--slug <name>] [--plan <short name>] [--without-plan] [--status <state>] [--size <n>] [--cursor <c>]")
	reference := flags.String("reference", "", "the platform's own reference")
	slug := flags.String("slug", "", "the workspace's short name")
	plan := flags.String("plan", "", "the short name of a plan")
	withoutPlan := flags.Bool("without-plan", false, "only workspaces on no plan")
	status := flags.String("status", "", "ACTIVE, SUSPENDED or PENDING_DELETION")
	size := flags.Int("size", 0, "workspaces per page, 50 by default and 200 at most")
	cursor := flags.String("cursor", "", "the cursor the page before printed")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	query := openapi.TenantQuery{
		ExternalReference: optional(*reference), Slug: optional(*slug), Plan: optional(*plan), Cursor: optional(*cursor),
	}
	if *withoutPlan {
		query.WithoutPlan = withoutPlan
	}
	if *status != "" {
		state := openapi.TenantQueryStatus(*status)
		query.Status = &state
	}
	if *size > 0 {
		query.Size = size
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var page openapi.AdminTenantPage
	if err := client.Post(ctx, adminTenantsPath+":query", query, &page); err != nil {
		return err
	}
	if err := cli.Emit(page, tenantTable(page.Data)); err != nil {
		return err
	}
	if page.Page.HasMore && page.Page.NextCursor != nil && !cli.JSON {
		printf(cli.Err, "more: --cursor %s\n", *page.Page.NextCursor)
	}
	return nil
}

// adminTenantSetReference sets the platform's reference on a workspace, or clears it with
// --clear. With --expected-version, a workspace that changed since it was read refuses.
func adminTenantSetReference(ctx context.Context, cli *CLI, args []string) error {
	usage := "admin tenant set-reference <id> (--reference <ref> | --clear) [--expected-version <n>]"
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "set-reference", "<id> --reference <ref>")
	reference := flags.String("reference", "", "the platform's own reference")
	clearing := flags.Bool("clear", false, "remove the reference")
	expected := flags.Int("expected-version", 0, "the workspace's version as last read")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if (*reference == "") == !*clearing {
		return usagef("admin tenant set-reference needs either --reference or --clear")
	}
	update := openapi.TenantUpdate{}
	if !*clearing {
		update.ExternalReference = reference
	}
	version := ""
	if *expected > 0 {
		version = `"` + strconv.Itoa(*expected) + `"`
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var tenant openapi.AdminTenant
	if err := client.PatchVersioned(ctx, adminTenantsPath+"/"+tenantID.String(), update, version, &tenant); err != nil {
		return err
	}
	return cli.Emit(tenant, tenantTable([]openapi.AdminTenant{tenant}))
}
