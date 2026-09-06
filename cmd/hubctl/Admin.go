// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strconv"
	"time"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The control plane (H-06): provisioning, the lifecycle, and the archive a workspace leaves with.
//
// It is the one legitimate tenant enumerator, and it is reached with a credential no session
// carries: a personal access token minted for `admin:tenants`, behind a step-up (0.6.0 decision
// 6). So this group is the one place in the client where `hubctl auth login` is the right sign-in
// and `hubctl login` is not - which is worth knowing before the first refusal says so.

const (
	adminTenantsPath    = "/admin/tenants"
	adminEncryptionPath = "/admin/encryption"
)

func adminGroup() group {
	return group{
		name:    "admin",
		summary: "the control plane of an installation that runs more than one workspace",
		commands: []command{
			{
				name:    "tenant",
				usage:   "ls|create|suspend|resume|delete|export …",
				summary: "the workspaces, and their lifecycle",
				run:     adminTenant,
				// The export waits on a job, and `--timeout` bounds one call rather than one
				// piece of work.
				waits: true,
			},
			{
				name:    "encryption",
				usage:   "show|reseal",
				summary: "the master keyring's census, and the re-seal that lets a key retire",
				run:     adminEncryption,
			},
		},
	}
}

// adminTenant is a noun under a noun, and dispatches its own verb - `backup target`'s reasoning.
func adminTenant(ctx context.Context, cli *CLI, args []string) error {
	const verbs = "ls, create, suspend, resume, delete, export"
	if len(args) == 0 {
		return usagef("admin tenant needs a command: %s", verbs)
	}
	switch args[0] {
	case "ls":
		return adminTenantList(ctx, cli, args[1:])
	case "create":
		return adminTenantCreate(ctx, cli, args[1:])
	case "suspend":
		return adminTenantFlip(ctx, cli, args[1:], "suspend")
	case "resume":
		return adminTenantFlip(ctx, cli, args[1:], "resume")
	case "delete":
		return adminTenantDelete(ctx, cli, args[1:])
	case "export":
		return adminTenantExport(ctx, cli, args[1:])
	default:
		return usagef("admin tenant has no command %q: %s", args[0], verbs)
	}
}

func adminTenantList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin tenant", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var tenants []openapi.AdminTenant
	if err := client.Get(ctx, adminTenantsPath, nil, &tenants); err != nil {
		return err
	}
	return cli.Emit(tenants, tenantTable(tenants))
}

// adminTenantCreate provisions a workspace and hands over the owner's way in.
//
// The redemption token is the whole of that way in and is answered once, so it is printed like
// every other credential this client meets: on standard output, with the warning beside it on
// standard error.
func adminTenantCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin tenant", "create",
		"--slug <name> --name <display name> --owner-email <address> [--owner-name <name>] "+
			"[--locale <tag>] [--zone <zone>]")
	slug := flags.String("slug", "", "the subdomain the workspace answers on")
	name := flags.String("name", "", "what people read")
	ownerEmail := flags.String("owner-email", "", "who the workspace is for")
	ownerName := flags.String("owner-name", "", "what to call them")
	locale := flags.String("locale", "", "the workspace's default language tag")
	zone := flags.String("zone", "", "the workspace's default time zone")
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
		Slug:             *slug,
		DisplayName:      *name,
		OwnerEmail:       openapitypes.Email(*ownerEmail),
		OwnerDisplayName: optional(*ownerName),
		DefaultLocale:    optional(*locale),
		DefaultTimeZone:  optional(*zone),
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

// adminTenantFlip is suspend and resume, which are one write each and differ only in which.
func adminTenantFlip(ctx context.Context, cli *CLI, args []string, verb string) error {
	usage := "admin tenant " + verb + " <id>"
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", verb, "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Post(ctx, adminTenantsPath+"/"+tenantID.String()+":"+verb, nil, nil); err != nil {
		return err
	}
	printf(cli.Err, "workspace %s is %s\n", tenantID,
		map[string]string{"suspend": "suspended", "resume": "active again"}[verb])
	return nil
}

// adminTenantDelete asks for the most irreversible thing this API does, and behaves like it: the
// workspace's display name typed exactly, and a fresh step-up on top of it.
//
// The name is not read off the workspace and offered back - the point of typing it is that
// somebody typed it. The proof is the shared mechanism, in the header this act takes it in.
func adminTenantDelete(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin tenant delete <id> --confirm <display name>"
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "delete", "<id> --confirm <display name>")
	confirm := flags.String("confirm", "", "the workspace's display name, typed exactly")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *confirm == "" {
		return usagef("ending a workspace needs --confirm with its display name, typed exactly")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var scheduled openapi.TenantDeletionScheduled
	err = cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.PostWithHeader(ctx, adminTenantsPath+"/"+tenantID.String()+":delete",
			openapi.TenantDeletionRequest{Confirmation: *confirm}, stepUpProof(stepUp), &scheduled)
	})
	if err != nil {
		return err
	}

	// The grace is the whole answer: the data is still there, the export still works, and the
	// moment it stops being true is the one number worth printing.
	return cli.Emit(scheduled, Table{
		Columns: []string{"workspace", "purged after"},
		Rows: [][]string{{
			scheduled.TenantId.String(),
			scheduled.PurgeAfter.Local().Format("2006-01-02 15:04"),
		}},
	})
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
		})
	}
	return Table{
		Columns: []string{"id", "slug", "name", "status", "locale", "created", "purged after"},
		Rows:    rows,
	}
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

// adminEncryption is the rotation's two verbs (ADR-0045, security.md §8.1): the census that says
// whether a key may leave the ring, and the request that moves what an older key still holds.
func adminEncryption(ctx context.Context, cli *CLI, args []string) error {
	const verbs = "show, reseal"
	if len(args) == 0 {
		return usagef("admin encryption needs a command: %s", verbs)
	}
	switch args[0] {
	case "show":
		return adminEncryptionShow(ctx, cli, args[1:])
	case "reseal":
		return adminEncryptionReseal(ctx, cli, args[1:])
	default:
		return usagef("admin encryption has no command %q: %s", args[0], verbs)
	}
}

func adminEncryptionShow(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin encryption", "show", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var status openapi.EncryptionStatus
	if err := client.Get(ctx, adminEncryptionPath, nil, &status); err != nil {
		return err
	}
	return cli.Emit(status, encryptionTable(status))
}

// adminEncryptionReseal asks for the rounds and says where to watch them: the census, not a job -
// there is one job per workspace, and what the operator is waiting for is a count, not a run.
func adminEncryptionReseal(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin encryption", "reseal", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var accepted openapi.ResealAccepted
	if err := client.Post(ctx, adminEncryptionPath+":reseal", nil, &accepted); err != nil {
		return err
	}
	printf(cli.Err, "re-sealing queued for %d workspace(s) under key %s; "+
		"watch `hubctl admin encryption show` until every other key counts zero\n",
		accepted.QueuedTenants, accepted.ActiveKeyId)
	return cli.Emit(accepted, Table{
		Columns: []string{"active key", "queued workspaces"},
		Rows:    [][]string{{accepted.ActiveKeyId, itoa(accepted.QueuedTenants)}},
	})
}

func encryptionTable(status openapi.EncryptionStatus) Table {
	rows := make([][]string, 0, len(status.Keys))
	for _, key := range status.Keys {
		state := "predecessor"
		switch {
		case key.Active:
			state = "active"
		case !key.InRing:
			state = "NOT IN RING"
		}
		rows = append(rows, []string{key.KeyId, state, itoa64(key.SealedValues)})
	}
	return Table{Columns: []string{"key", "state", "sealed values"}, Rows: rows}
}

func itoa(value int) string     { return itoa64(int64(value)) }
func itoa64(value int64) string { return strconv.FormatInt(value, 10) }
