// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const restoresPath = "/restores"

func restoreGroup() group {
	return group{
		name:    "restore",
		summary: "bringing an archive back, and finding out first what that would do",
		commands: []command{
			{
				name:    "inspect",
				usage:   "--target <id> --archive <path> --tenant <id> [--wait <d>]",
				summary: "read an archive without changing anything",
				run:     restoreInspect,
				waits:   true,
			},
			{
				name:    "run",
				usage:   "--target <id> --archive <path> --mode NEW_TENANT|MERGE|SELECTIVE|REPLACE_TENANT|INSTANCE [--apply]",
				summary: "restore, as a dry run unless --apply is given",
				run:     restoreRun,
			},
			{
				name:    "show",
				usage:   "<id>",
				summary: "what one restore did, or was about to do",
				run:     restoreShow,
			},
		},
	}
}

// restoreInspect is `restore run --mode INSPECT` with nothing else to decide. It is a command of
// its own because it is the one a person reaches for first, and because reading an archive with a
// verb called "run" reads like a thing that changes something.
func restoreInspect(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "restore", "inspect", "--target <id> --archive <path> --tenant <id>")
	target := flags.String("target", "", "the target the archive lies at")
	archive := flags.String("archive", "", "the archive, as `hubctl backup ls` prints it")
	// An inspection reads an archive and writes nothing, and it still names a workspace: reading
	// one is scoped like every other read, and the modes that write into a workspace that already
	// exists are the same set (`needsTenant` in the domain).
	tenant := flags.String("tenant", "", "the workspace the reading is scoped to")
	wait := waitFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	request, err := cli.restoreRequest(*target, *archive, string(openapi.RestoreRequestModeINSPECT))
	if err != nil {
		return err
	}
	if *tenant != "" {
		tenantID, err := cli.parseID("--tenant", *tenant)
		if err != nil {
			return err
		}
		request.TargetTenantId = &tenantID
	}
	return cli.startRestore(ctx, request, *wait)
}

func restoreRun(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "restore", "run",
		"--target <id> --archive <path> --mode <mode> [--tenant <id>] [--conflict SKIP|OVERWRITE|DUPLICATE] [--apply]")
	target := flags.String("target", "", "the target the archive lies at")
	archive := flags.String("archive", "", "the archive, as `hubctl backup ls` prints it")
	mode := flags.String("mode", "", "NEW_TENANT, MERGE, SELECTIVE, REPLACE_TENANT or INSTANCE")
	tenant := flags.String("tenant", "", "the workspace to restore into, where the mode needs one")
	conflict := flags.String("conflict", "", "SKIP, OVERWRITE or DUPLICATE (the server's default is SKIP)")
	apply := flags.Bool("apply", false, "actually do it; without this the run is a dry run with a report")
	noSafety := flags.Bool("no-safety-backup", false, "skip the copy taken before a destructive mode")
	confirm := flags.String("confirm", "", "the exact workspace name, which a destructive mode requires")
	wait := waitFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *mode == "" {
		return usagef("restore run needs --mode; use `hubctl restore inspect` to read an archive first")
	}

	request, err := cli.restoreRequest(*target, *archive, *mode)
	if err != nil {
		return err
	}
	if *apply {
		request.DryRun = boolean(false)
	}
	if *noSafety {
		request.CreateSafetyBackup = boolean(false)
	}
	if *confirm != "" {
		request.Confirmation = confirm
	}
	if *conflict != "" {
		rule := openapi.RestoreRequestConflictRule(*conflict)
		request.ConflictRule = &rule
	}
	if *tenant != "" {
		tenantID, err := cli.parseID("--tenant", *tenant)
		if err != nil {
			return err
		}
		request.TargetTenantId = &tenantID
	}
	return cli.startRestore(ctx, request, *wait)
}

func restoreShow(ctx context.Context, cli *CLI, args []string) error {
	const usage = "restore show <id>"
	restoreID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "restore", "show", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var run openapi.RestoreRun
	if err := client.Get(ctx, restoresPath+"/"+restoreID.String(), nil, &run); err != nil {
		return err
	}
	return cli.emitRestore(run)
}

// restoreRequest is what every one of the three commands builds, so that the passphrase, the
// archive and the target are read the same way whichever verb is used.
func (cli *CLI) restoreRequest(target, archive, mode string) (openapi.RestoreRequest, error) {
	if target == "" || archive == "" {
		return openapi.RestoreRequest{}, usagef(
			"a restore needs --target and --archive; `hubctl backup ls --target <id>` prints both")
	}
	targetID, err := cli.parseID("--target", target)
	if err != nil {
		return openapi.RestoreRequest{}, err
	}

	request := openapi.RestoreRequest{
		TargetId:  targetID,
		ArchiveId: archive,
		Mode:      openapi.RestoreRequestMode(mode),
	}
	// No `decryption_passphrase`, for the reason `backup target add` sends no passphrase: the key
	// an archive is written under is derived from the installation's master key (E-02), and this
	// version refuses the field rather than ignoring it.
	return request, nil
}

// startRestore sends the request and follows the job it becomes, because a restore that answered
// "accepted" and nothing else would leave the report - the whole point of a dry run - unread.
func (cli *CLI) startRestore(
	ctx context.Context, request openapi.RestoreRequest, wait time.Duration,
) error {
	client, err := cli.client()
	if err != nil {
		return err
	}
	var accepted openapi.JobRef
	if err := client.Post(ctx, restoresPath, request, &accepted); err != nil {
		return err
	}

	job, err := cli.followJob(ctx, client, accepted.JobId, wait)
	if err != nil {
		return err
	}
	if err := cli.jobFailed(job); err != nil {
		return err
	}

	// The restore is at the path the 202 named: a job identifier is not a restore identifier, and
	// the job resource carries no result of its own.
	var run openapi.RestoreRun
	if err := cli.readResult(ctx, client, accepted, &run); err != nil {
		return err
	}
	if err := cli.emitRestore(run); err != nil {
		return err
	}

	// A job that finished and a restore that worked are two different statements, and the second
	// is the one somebody asked about: the worker can complete its job having refused the restore
	// inside it. Printing the report and exiting 0 would tell a script that a failed restore was
	// fine - the same mistake as a verification that prints `valid false` and succeeds.
	if run.Status == openapi.RestoreRunStatusFAILED || run.Status == openapi.RestoreRunStatusCANCELLED {
		return cli.restoreFailed(run)
	}
	return nil
}

// restoreFailed turns the run's own failure into the sentence the catalogue has for it, so that a
// restore refused by the server reads the same whether the refusal came back on the request or
// minutes later on the run.
func (cli *CLI) restoreFailed(run openapi.RestoreRun) error {
	code := "backup.restore_failed"
	if run.ErrorCode != nil && *run.ErrorCode != "" {
		code = *run.ErrorCode
	}
	if message, ok := cli.Catalogue.Message(code, nil); ok {
		return errorString(message)
	}
	return errorString("the restore did not work: " + code)
}

// emitRestore prints the run and, underneath it, what the run did to each kind of object. Two
// tables for a person and one document for a pipe: the report is the answer, and squeezing it into
// the run's row would put nine numbers in one column.
func (cli *CLI) emitRestore(run openapi.RestoreRun) error {
	if cli.JSON {
		return cli.Emit(run, Table{})
	}

	cli.emitTable(Table{
		Columns: []string{"id", "status", "mode", "dry run", "archive", "tenant", "safety copy"},
		Rows: [][]string{{
			run.Id.String(),
			string(run.Status),
			string(run.Mode),
			boolText(run.DryRun),
			run.SourceArchive,
			id(run.TenantId),
			id(run.SafetyBackupRunId),
		}},
	})
	if run.Report == nil {
		return nil
	}

	printf(cli.Out, "\n")
	report := *run.Report
	cli.emitTable(Table{
		Columns: []string{"new", "overwritten", "skipped", "duplicated", "conflicts", "withheld", "media"},
		Rows: [][]string{{
			count(report.New),
			count(report.Overwritten),
			count(report.Skipped),
			count(report.Duplicated),
			count(report.Conflicts),
			count(report.Deleted),
			count(report.Media),
		}},
	})
	if report.Entities != nil && len(*report.Entities) > 0 {
		printf(cli.Out, "\n")
		cli.emitTable(entityTable(*report.Entities))
	}
	return nil
}

// entityTable is what each kind of object contributed, in a stable order: a map printed in Go's
// own iteration order would shuffle between two runs of the same command.
func entityTable(entities map[string]int) Table {
	names := make([]string, 0, len(entities))
	for name := range entities {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([][]string, 0, len(names))
	for _, name := range names {
		rows = append(rows, []string{name, strconv.Itoa(entities[name])})
	}
	return Table{Columns: []string{"entity", "records"}, Rows: rows}
}

func boolText(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
