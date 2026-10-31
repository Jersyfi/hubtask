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
