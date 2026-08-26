// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"sort"
	"strconv"
	"strings"

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
				usage:   "--target <id> --archive <path>",
				summary: "read an archive without changing anything",
				run:     restoreInspect,
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
	flags := commandFlags(cli, "restore", "inspect", "--target <id> --archive <path>")
	target := flags.String("target", "", "the target the archive lies at")
	archive := flags.String("archive", "", "the archive, as `hubctl backup ls` prints it")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	request, err := cli.restoreRequest(*target, *archive, string(openapi.RestoreRequestModeINSPECT))
	if err != nil {
		return err
	}
	return cli.startRestore(ctx, request)
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
	return cli.startRestore(ctx, request)
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
	// The passphrase is read from the environment only, never from a flag, and never asked for on
	// a prompt here: a restore is the command most likely to be run from a script, and a prompt in
	// the middle of one is a hang nobody can see.
	if passphrase := strings.TrimSpace(cli.Env(envBackupPassphrase)); passphrase != "" {
		request.DecryptionPassphrase = &passphrase
	}
	return request, nil
}

// startRestore sends the request and follows the job it becomes, because a restore that answered
// "accepted" and nothing else would leave the report - the whole point of a dry run - unread.
func (cli *CLI) startRestore(ctx context.Context, request openapi.RestoreRequest) error {
	client, err := cli.client()
	if err != nil {
		return err
	}
	var accepted openapi.JobRef
	if err := client.Post(ctx, restoresPath, request, &accepted); err != nil {
		return err
	}

	job, err := cli.followJob(ctx, client, accepted.JobId)
	if err != nil {
		return err
	}
	if err := cli.jobFailed(job); err != nil {
		return err
	}

	var run openapi.RestoreRun
	if err := client.Get(ctx, restoresPath+"/"+job.JobId.String(), nil, &run); err != nil {
		return err
	}
	return cli.emitRestore(run)
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
