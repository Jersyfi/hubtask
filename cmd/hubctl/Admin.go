// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"flag"
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
				usage:   "ls|create|suspend|resume|delete|export|open-password|close-password|replace-holds …",
				summary: "the workspaces, and their lifecycle",
				run:     adminTenant,
				// The export waits on a job, and `--timeout` bounds one call rather than one
				// piece of work.
				waits: true,
			},
			{
				name:    "settings",
				usage:   "show|set|clear …",
				summary: "the values this installation decided for every workspace on it",
				run:     adminSettings,
			},
			{
				name:    "operator",
				usage:   "ls|add|rm …",
				summary: "who may run this installation",
				run:     adminOperator,
			},
			{
				name:    "provider",
				usage:   "ls|add|rm …",
				summary: "the ways in this installation offers every workspace",
				run:     adminProvider,
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
	const verbs = "ls, create, suspend, resume, delete, export, open-password, close-password, replace-holds"
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
	case "open-password":
		return adminTenantOpenPassword(ctx, cli, args[1:])
	case "close-password":
		return adminTenantClosePassword(ctx, cli, args[1:])
	case "replace-holds":
		return adminTenantReplaceHolds(ctx, cli, args[1:])
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

// adminTenantOpenPassword opens the password for one workspace for a while (ADR-0078 §3): the lever
// for a provider that is switched on but broken. Who asked and why are not optional - the workspace's
// administrators read both - and the proof is the shared mechanism, in the header the act takes it in.
func adminTenantOpenPassword(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin tenant open-password <id> --requester <who asked> --reason <why> [--hours <n>]"
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "open-password",
		"<id> --requester <who asked> --reason <why> [--hours <n>]")
	requester := flags.String("requester", "", "who asked for it - a ticket reference rather than a name")
	reason := flags.String("reason", "", "why: what is wrong with the provider")
	hours := flags.Int("hours", 0, "how long it stands, from now: 1 to 168 (a day when not given)")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *requester == "" || *reason == "" {
		return usagef("opening the password needs --requester and --reason: the workspace's administrators read both")
	}
	request := openapi.PasswordOpeningRequest{Requester: *requester, Reason: *reason}
	// The default day is the server's, and only where no --hours was given: a --hours 0 is sent as
	// typed, so the server refuses it rather than this client turning it into a day.
	flags.Visit(func(given *flag.Flag) {
		if given.Name == "hours" {
			request.Hours = hours
		}
	})

	client, err := cli.client()
	if err != nil {
		return err
	}
	var tenant openapi.AdminTenant
	err = cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.PostWithHeader(ctx, adminTenantsPath+"/"+tenantID.String()+":open-password",
			request, stepUpProof(stepUp), &tenant)
	})
	if err != nil {
		return err
	}
	if cli.JSON {
		return cli.Emit(tenant, Table{})
	}
	cli.emitTable(openingTable(tenant))
	printf(cli.Err, "the workspace's administrators are told; it closes on its own at the time above, "+
		"or with admin tenant close-password\n")
	return nil
}

// adminTenantClosePassword ends an opening before its time. No step-up: it narrows the way in.
func adminTenantClosePassword(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin tenant close-password <id>"
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "close-password", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var tenant openapi.AdminTenant
	if err := client.Post(ctx, adminTenantsPath+"/"+tenantID.String()+":close-password", nil, &tenant); err != nil {
		return err
	}
	if cli.JSON {
		return cli.Emit(tenant, Table{})
	}
	printf(cli.Err, "the password of workspace %s follows its own settings again\n", tenantID)
	return nil
}

// openingTable is the opening as it stands: until when, who asked, why.
func openingTable(tenant openapi.AdminTenant) Table {
	row := []string{tenant.Id.String(), "-", "-", "-"}
	if opening := tenant.PasswordOpening; opening != nil {
		row = []string{tenant.Id.String(), shortTime(&opening.Until), opening.Requester, opening.Reason}
	}
	return Table{Columns: []string{"workspace", "open until", "requester", "reason"}, Rows: [][]string{row}}
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
			passwordOpenUntil(tenant.PasswordOpening),
			legalHold(tenant),
		})
	}
	return Table{
		Columns: []string{"id", "slug", "name", "status", "locale", "created", "purged after", "password open until", "legal hold"},
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
