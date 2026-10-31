// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strconv"
	"time"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The control plane: provisioning, the lifecycle, the archive a workspace leaves
// with — and since ADR-0070 §5 the instance level too, which is the second of its three doors.
//
// "Ein Betreiber wählt seine Tür, nicht seinen Funktionsumfang": every verb the dashboard has is
// here. `settings` for the values and their locks, `operator` for the register, `provider` for the
// ways in this installation offers, and `legal` is `settings` under its own area rather than a
// group of its own — four links are not a noun.
//
// It is the one legitimate tenant enumerator, and it is reached with a credential no session
// carries: a personal access token minted for `admin:tenants`, behind a step-up (0.6.0 decision
// 6). So this group is the one place in the client where `hubctl auth login` is the right sign-in
// and `hubctl login` is not - which is worth knowing before the first refusal says so.

const (
	adminTenantsPath = "/admin/tenants"
)

func adminGroup() group {
	return group{
		name:    "admin",
		summary: "the control plane of an installation that runs more than one workspace",
		// The rest of each noun's verbs - `tenant ls`, `plan show`, `encryption reseal` - are
		// generated from the contract (Door.go); these do more than their operation.
		commands: []command{
			{
				name:    "tenant create",
				usage:   "--slug <name> --name <display name> --owner-email <address> [flags]",
				summary: "provision a workspace and hand over the owner's way in, shown once",
				run:     adminTenantCreate,
			},
			{
				name:    "tenant set-reference",
				usage:   "<id> (--reference <ref> | --clear) [--expected-version <n>]",
				summary: "set or clear the platform's reference on a workspace",
				run:     adminTenantSetReference,
			},
			{
				name:    "tenant export",
				usage:   "<id> --target <id> [--follow] [--wait <d>]",
				summary: "write the workspace whole to a configured target",
				run:     adminTenantExport,
				// The export waits on a job, and `--timeout` bounds one call rather than one
				// piece of work.
				waits: true,
			},
			{
				name:    "tenant replace-holds",
				usage:   "<id> --recovery-point <time> (--from-cluster <dsn> | --from-archive <path> --target <id> | --from <file>)",
				summary: "put back the legal holds a recovery to an earlier point lost",
				run:     adminTenantReplaceHolds,
				waits:   true,
			},
			{
				name:    "tenant reapply-erasures",
				usage:   "<id> --recovery-point <time> (--from-cluster <dsn> | --from <file>)",
				summary: "apply again the erasures a recovery to an earlier point undid",
				run:     adminTenantReapplyErasures,
				waits:   true,
			},
			{
				name:    "plan set",
				usage:   "<name> [<area>.<key> [value]] [--locked] [--display-name <name>] [--leave-out <features>]",
				summary: "set one key of a plan, at the version read",
				run:     func(ctx context.Context, cli *CLI, args []string) error { return adminPlanSet(ctx, cli, args, true) },
			},
			{
				name:    "plan clear",
				usage:   "<name> <area>.<key>",
				summary: "clear one key of a plan, at the version read",
				run:     func(ctx context.Context, cli *CLI, args []string) error { return adminPlanSet(ctx, cli, args, false) },
			},
			{
				name:    "subscription add",
				usage:   "--target <url>",
				summary: "push the installation's events to an address; its secret is shown once",
				run:     adminSubscriptionAdd,
			},
			{
				name:    "subscription rotate-secret",
				usage:   "<id>",
				summary: "mint the subscription's next secret, shown once",
				run:     adminSubscriptionRotate,
			},
			{
				name:    "subscription pause",
				usage:   "<id>",
				summary: "stop pushing to the subscription, keeping what waits",
				run: func(ctx context.Context, cli *CLI, args []string) error {
					return adminSubscriptionState(ctx, cli, args, "pause", openapi.PlatformSubscriptionUpdateStatePAUSED)
				},
			},
			{
				name:    "subscription resume",
				usage:   "<id>",
				summary: "push to the subscription again",
				run: func(ctx context.Context, cli *CLI, args []string) error {
					return adminSubscriptionState(ctx, cli, args, "resume", openapi.PlatformSubscriptionUpdateStateACTIVE)
				},
			},
			{
				name:    "usage",
				usage:   "--from <day> --to <day> [--metric <name>] [--workspace <id>]",
				summary: "the use per workspace, metric and day, while metering is on",
				run:     adminUsage,
			},
			{
				name:    "settings",
				usage:   "show|set|clear …",
				summary: "the values this installation decided for every workspace on it",
				run:     adminSettings,
			},
			{
				name:    "operator add",
				usage:   "(--workspace <slug> --email <address> | --service-account <name> | --account <id>)",
				summary: "let an account run this installation",
				run:     adminOperatorAdd,
			},
			{
				name:    "elevate",
				usage:   "",
				summary: "raise this signed-in session to the installation for an hour, after proving it is you",
				run:     adminElevate,
			},
			{
				name:    "ai",
				usage:   "ls|add|set|rm …",
				summary: "the AI models this installation offers every workspace",
				run:     adminAi,
			},
			{
				name:    "target",
				usage:   "ls|add|test|rm …",
				summary: "the installation's own backup targets, where a workspace without one is exported to",
				run:     adminTarget,
			},
		},
	}
}

// adminTenantCreate provisions a workspace and hands over the owner's way in.
//
// The redemption token is the whole of that way in and is answered once, so it is printed like
// every other credential this client meets: on standard output, with the warning beside it on
// standard error.
func adminTenantCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin tenant", "create",
		"--slug <name> --name <display name> --owner-email <address> [--owner-name <name>] "+
			"[--locale <tag>] [--zone <zone>] [--plan <short name>] [--reference <ref>]")
	slug := flags.String("slug", "", "the subdomain the workspace answers on")
	name := flags.String("name", "", "what people read")
	ownerEmail := flags.String("owner-email", "", "who the workspace is for")
	ownerName := flags.String("owner-name", "", "what to call them")
	locale := flags.String("locale", "", "the workspace's default language tag")
	zone := flags.String("zone", "", "the workspace's default time zone")
	plan := flags.String("plan", "", "the plan the workspace starts on")
	reference := flags.String("reference", "", "the platform's own reference for the workspace")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *slug == "" || *name == "" || *ownerEmail == "" {
		return usagef("provisioning needs --slug, --name and --owner-email")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var provisioned openapi.ProvisionedTenant
	if err := client.Post(ctx, adminTenantsPath, openapi.TenantProvision{
		Slug:              *slug,
		DisplayName:       *name,
		OwnerEmail:        openapitypes.Email(*ownerEmail),
		OwnerDisplayName:  optional(*ownerName),
		DefaultLocale:     optional(*locale),
		DefaultTimeZone:   optional(*zone),
		Plan:              optional(*plan),
		ExternalReference: optional(*reference),
	}, &provisioned); err != nil {
		return err
	}

	if cli.JSON {
		return cli.Emit(provisioned, Table{})
	}
	cli.emitTable(Table{
		Columns: []string{"id", "slug", "name", "status", "owner", "default hub", "example collection"},
		Rows: [][]string{{
			provisioned.Id.String(), provisioned.Slug, provisioned.DisplayName,
			string(provisioned.Status), provisioned.OwnerAccountId.String(),
			provisioned.DefaultHubId.String(), provisioned.ExampleCollectionId.String(),
		}},
	})
	printf(cli.Out, "%s\n", provisioned.OwnerRedemptionToken)
	printf(cli.Err,
		"that redemption token is the owner's whole way in and is shown once: hand it to them, "+
			"and provision again if it is lost\n")
	return nil
}

// adminTenantExport writes the workspace whole to a configured target, and follows the job.
//
// The archive is at the target rather than at a URL, so the job carries no result to read back:
// what this command can say is that the export finished, and where it was written is the target
// that was named.
func adminTenantExport(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin tenant export <id> --target <id>"
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "export", "<id> --target <id> [--follow] [--wait <d>]")
	target := flags.String("target", "", "the configured backup target the archive is written to")
	follow := flags.Bool("follow", false, "keep asking until the export is finished")
	wait := waitFlag(flags)
	if err := parseCommand(flags, rest); err != nil {
		return err
	}
	if *target == "" {
		return usagef("an export needs --target: the archive is written to a configured target")
	}
	targetID, err := cli.parseID("--target", *target)
	if err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var accepted openapi.JobRef
	if err := client.Post(ctx, adminTenantsPath+"/"+tenantID.String()+":export",
		openapi.TenantExportRequest{TargetId: targetID}, &accepted); err != nil {
		return err
	}
	if !*follow {
		return cli.Emit(accepted, acceptedTable(accepted))
	}

	job, err := cli.followJob(ctx, client, accepted.JobId, *wait)
	if err != nil {
		return err
	}
	if err := cli.jobFailed(job); err != nil {
		return err
	}
	return cli.Emit(job, jobTable(job))
}

func tenantTable(tenants []openapi.AdminTenant) Table {
	rows := make([][]string, 0, len(tenants))
	for _, tenant := range tenants {
		rows = append(rows, []string{
			tenant.Id.String(),
			tenant.Slug,
			tenant.DisplayName,
			string(tenant.Status),
			text(tenant.DefaultLocale),
			shortTime(&tenant.CreatedAt),
			purgeAfter(tenant.PurgeAfter),
			purgeAfter(tenant.PurgeStartedAt),
			passwordOpenUntil(tenant.PasswordOpening),
			legalHold(tenant),
			text(tenant.Plan),
			text(tenant.ExternalReference),
		})
	}
	return Table{
		// "removal began" is the moment from which the deletion can no longer be called off.
		Columns: []string{"id", "slug", "name", "status", "locale", "created", "purged after", "removal began", "password open until", "legal hold", "plan", "reference"},
		Rows:    rows,
	}
}

// legalHold says a hold is in force, and for a workspace pending deletion that its deletion waits
// for the last one to be lifted (data-protection.md §5). Only the state: the workspace keeps which.
func legalHold(tenant openapi.AdminTenant) string {
	if tenant.LegalHold == nil || !*tenant.LegalHold {
		return "-"
	}
	if tenant.Status == openapi.AdminTenantStatusPENDINGDELETION {
		return "in force, deletion waits"
	}
	return "in force"
}

// passwordOpenUntil is a dash for every workspace whose password no operator has opened.
func passwordOpenUntil(opening *openapi.PasswordOpening) string {
	if opening == nil {
		return "-"
	}
	return shortTime(&opening.Until)
}

// purgeAfter is empty for every workspace nobody has asked to end, which is nearly all of them.
func purgeAfter(at *time.Time) string {
	if at == nil {
		return "-"
	}
	return shortTime(at)
}

func acceptedTable(accepted openapi.JobRef) Table {
	return Table{
		Columns: []string{"job", "status"},
		Rows:    [][]string{{accepted.JobId.String(), string(accepted.Status)}},
	}
}

func itoa64(value int64) string { return strconv.FormatInt(value, 10) }
