// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	backupTargetsPath = "/backup-targets"
	backupsPath       = "/backups"
)

// envBackupPassphrase is where the passphrase comes from.
//
// Not a flag, deliberately. A passphrase typed as `--passphrase secret` is in the shell's history,
// in `ps`, and in the CI log of whoever scripted it, and the one value in this whole CLI that
// cannot be reissued is this one: without it the archive is unreadable for ever
// (backup-restore.md §4). An environment variable and standard input are the two ways to hand a
// secret over that do not write it down.
const envBackupPassphrase = "HUBTASK_BACKUP_PASSPHRASE" //nolint:gosec // G101: the name of an environment variable, not a secret.

func backupGroup() group {
	return group{
		name:    "backup",
		summary: "where copies go, and the copies themselves",
		commands: []command{
			{
				name:    "target",
				usage:   "add|ls|test …",
				summary: "the places an archive can be written to",
				run:     backupTarget,
			},
			{
				name:    "run",
				usage:   "--target <id> [--mode FULL|INCREMENTAL] [--no-media] [--no-audit] [--follow]",
				summary: "start a backup now",
				run:     backupRun,
			},
			{
				name:    "ls",
				usage:   "--target <id>",
				summary: "the archives that are actually lying at the target",
				run:     backupList,
			},
			{
				name:    "show",
				usage:   "<id>",
				summary: "what one run did",
				run:     backupShow,
			},
			{
				name:    "verify",
				usage:   "<id> [--follow]",
				summary: "check an archive's checksums and that it decrypts",
				run:     backupVerify,
			},
		},
	}
}

// backupTarget is a noun under a noun, so it dispatches its own verb. Three levels rather than
// two: `hubctl backup target ls` reads as what it is, and flattening it to `hubctl backup-target`
// would give the CLI a second spelling of the same word.
func backupTarget(ctx context.Context, cli *CLI, args []string) error {
	if len(args) == 0 {
		return usagef("backup target needs a command: add, ls, test")
	}
	switch args[0] {
	case "add":
		return backupTargetAdd(ctx, cli, args[1:])
	case "ls":
		return backupTargetList(ctx, cli, args[1:])
	case "test":
		return backupTargetTest(ctx, cli, args[1:])
	default:
		return usagef("backup target has no command %q: add, ls, test", args[0])
	}
}

func backupTargetAdd(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "backup target", "add",
		"--name <name> --kind LOCAL|S3|SFTP|… --config <k=v,…> [--unencrypted] [--insecure-acknowledged]")
	name := flags.String("name", "", "what to call it")
	kind := flags.String("kind", "", "LOCAL, S3, SFTP, FTPS, FTP, WEBDAV, SMB, AZURE_BLOB, GCS, RCLONE or HTTP_PUT")
	config := flags.String("config", "", "where it points, as k=v pairs: path=daily or bucket=hubtask,region=eu-central-1")
	credentials := flags.String("credentials", "",
		"as k=v pairs, stored encrypted and never read back; prefer the environment for a secret")
	unencrypted := flags.Bool("unencrypted", false, "write the archives in clear (refused without --insecure-acknowledged)")
	acknowledged := flags.Bool("insecure-acknowledged", false, "yes, this target is unencrypted or a plaintext protocol")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *name == "" || *kind == "" {
		return usagef("backup target add needs --name and --kind")
	}

	settings, err := pairs("--config", *config)
	if err != nil {
		return err
	}
	secrets, err := pairs("--credentials", *credentials)
	if err != nil {
		return err
	}

	create := openapi.BackupTargetCreate{
		Name:                 *name,
		Kind:                 openapi.BackupTargetKind(*kind),
		Config:               anyMap(settings),
		InsecureAcknowledged: acknowledged,
	}
	if len(secrets) > 0 {
		values := anyMap(secrets)
		create.Credentials = &values
	}

	mode := openapi.BackupTargetCreateEncryptionModeAES256GCM
	if *unencrypted {
		mode = openapi.BackupTargetCreateEncryptionModeNONE
	}
	create.EncryptionMode = &mode

	if !*unencrypted {
		passphrase, err := cli.backupPassphrase()
		if err != nil {
			return err
		}
		create.EncryptionPassphrase = &passphrase
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var target openapi.BackupTarget
	if err := client.Post(ctx, backupTargetsPath, create, &target); err != nil {
		return err
	}
	// The one thing a person has to be told rather than shown: the passphrase is not stored, and
	// nothing in this installation can hand it back.
	if !*unencrypted {
		printf(cli.Err, "hubctl: keep the passphrase safe - without it the archives at %q "+
			"cannot be read, and this installation does not have a copy\n", target.Name)
	}
	return cli.Emit(target, targetTable([]openapi.BackupTarget{target}))
}

func backupTargetList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "backup target", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var targets []openapi.BackupTarget
	if err := client.Get(ctx, backupTargetsPath, nil, &targets); err != nil {
		return err
	}
	return cli.Emit(targets, targetTable(targets))
}

func backupTargetTest(ctx context.Context, cli *CLI, args []string) error {
	const usage = "backup target test <id>"
	targetID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "backup target", "test", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var target openapi.BackupTarget
	if err := client.Post(ctx, backupTargetsPath+"/"+targetID.String()+":test", nil, &target); err != nil {
		return err
	}
	return cli.Emit(target, targetTable([]openapi.BackupTarget{target}))
}

func backupRun(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "backup", "run",
		"--target <id> [--mode FULL|INCREMENTAL] [--no-media] [--no-audit] [--follow]")
	target := flags.String("target", "", "the target to write to")
	mode := flags.String("mode", "FULL", "FULL or INCREMENTAL")
	noMedia := flags.Bool("no-media", false, "leave the attachments out")
	noAudit := flags.Bool("no-audit", false, "leave the audit trail out")
	follow := flags.Bool("follow", false, "wait for the run to finish")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *target == "" {
		return usagef("backup run needs --target; `hubctl backup target ls` prints the identifiers")
	}
	targetID, err := cli.parseID("--target", *target)
	if err != nil {
		return err
	}

	start := openapi.BackupStart{TargetId: targetID}
	runMode := openapi.BackupStartMode(*mode)
	start.Mode = &runMode
	if *noMedia {
		start.IncludeMedia = boolean(false)
	}
	if *noAudit {
		start.IncludeAudit = boolean(false)
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var accepted openapi.JobRef
	if err := client.Post(ctx, backupsPath, start, &accepted); err != nil {
		return err
	}
	if !*follow {
		return cli.Emit(accepted, jobRefTable(accepted))
	}

	// The run's identifier *is* the archive's identifier in the manifest at the target, and it is
	// the job's identifier too: one value for all three, so following the job and then reading the
	// run needs no mapping in between (`/backups` in openapi.yaml).
	job, err := cli.followJob(ctx, client, accepted.JobId)
	if err != nil {
		return err
	}
	if err := cli.jobFailed(job); err != nil {
		return err
	}

	var run openapi.BackupRun
	if err := client.Get(ctx, backupsPath+"/"+job.JobId.String(), nil, &run); err != nil {
		return err
	}
	return cli.Emit(run, runTable([]openapi.BackupRun{run}))
}

// backupList reads the target rather than the database, which is the whole point of it: it is the
// listing that still answers after the installation that wrote the archives is gone
// (backup-restore.md §6), and it is where `hubctl restore` gets an archive to name.
func backupList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "backup", "ls", "--target <id>")
	target := flags.String("target", "", "the target to look at")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *target == "" {
		return usagef("backup ls needs --target; `hubctl backup target ls` prints the identifiers")
	}
	targetID, err := cli.parseID("--target", *target)
	if err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var archives []openapi.BackupRun
	if err := client.Get(ctx, backupTargetsPath+"/"+targetID.String()+"/backups", nil, &archives); err != nil {
		return err
	}
	return cli.Emit(archives, runTable(archives))
}

func backupShow(ctx context.Context, cli *CLI, args []string) error {
	const usage = "backup show <id>"
	runID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "backup", "show", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var run openapi.BackupRun
	if err := client.Get(ctx, backupsPath+"/"+runID.String(), nil, &run); err != nil {
		return err
	}
	return cli.Emit(run, runTable([]openapi.BackupRun{run}))
}

func backupVerify(ctx context.Context, cli *CLI, args []string) error {
	const usage = "backup verify <id> [--follow]"
	runID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "backup", "verify", "<id> [--follow]")
	follow := flags.Bool("follow", false, "wait for the check to finish")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var accepted openapi.JobRef
	if err := client.Post(ctx, backupsPath+"/"+runID.String()+":verify", nil, &accepted); err != nil {
		return err
	}
	if !*follow {
		return cli.Emit(accepted, jobRefTable(accepted))
	}

	job, err := cli.followJob(ctx, client, accepted.JobId)
	if err != nil {
		return err
	}
	if err := cli.jobFailed(job); err != nil {
		return err
	}

	// The verdict is on the run rather than on the job: `verified_at` and `verify_ok` are what a
	// person asked for, and a job that succeeded only says the check ran.
	var run openapi.BackupRun
	if err := client.Get(ctx, backupsPath+"/"+runID.String(), nil, &run); err != nil {
		return err
	}
	if run.VerifyOk != nil && !*run.VerifyOk {
		return errorString(fmt.Sprintf("the archive of run %s did not verify", runID))
	}
	return cli.Emit(run, runTable([]openapi.BackupRun{run}))
}

// backupPassphrase reads the passphrase from the environment, or asks for it on standard input.
func (cli *CLI) backupPassphrase() (string, error) {
	if fromEnv := strings.TrimSpace(cli.Env(envBackupPassphrase)); fromEnv != "" {
		return fromEnv, nil
	}
	// A prompt only where somebody is there to read it, like `auth login`: piped input gets none,
	// so that `echo "$PHRASE" | hubctl backup target add …` writes nothing it did not ask for.
	if file, ok := cli.In.(*os.File); ok {
		if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			printf(cli.Err, "Passphrase for the archives (it will be visible, "+
				"it is not stored, and it cannot be recovered), then press Ctrl-D:\n")
		}
	}

	raw, err := io.ReadAll(cli.In)
	if err != nil {
		return "", err
	}
	typed := strings.TrimSpace(string(raw))
	if typed == "" {
		return "", usagef("a passphrase is needed, or --unencrypted with --insecure-acknowledged; "+
			"%s is read when it is set", envBackupPassphrase)
	}
	return typed, nil
}

func targetTable(targets []openapi.BackupTarget) Table {
	rows := make([][]string, 0, len(targets))
	for _, target := range targets {
		rows = append(rows, []string{
			target.Id.String(),
			target.Name,
			string(target.Kind),
			string(target.EncryptionMode),
			yesNo(target.Enabled),
			lastTest(target),
			joined(target.Warnings),
		})
	}
	return Table{
		Columns: []string{"id", "name", "kind", "encryption", "enabled", "last test", "warnings"},
		Rows:    rows,
	}
}

// lastTest folds the two columns a person reads together into one: when it was tried, and whether
// it worked. A target nobody has tested says so rather than showing an empty pair.
func lastTest(target openapi.BackupTarget) string {
	if target.LastTestAt == nil {
		return "never"
	}
	verdict := "failed"
	if target.LastTestOk != nil && *target.LastTestOk {
		verdict = "ok"
	}
	return shortTime(target.LastTestAt) + " " + verdict
}

func runTable(runs []openapi.BackupRun) Table {
	rows := make([][]string, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, []string{
			run.Id.String(),
			string(run.Status),
			string(run.Mode),
			text(run.ArchivePath),
			count(run.ItemCount),
			byteSize(run.SizeBytes),
			shortTime(run.FinishedAt),
			verified(run),
		})
	}
	return Table{
		Columns: []string{"id", "status", "mode", "archive", "items", "size", "finished", "verified"},
		Rows:    rows,
	}
}

func verified(run openapi.BackupRun) string {
	if run.VerifiedAt == nil {
		return "never"
	}
	if run.VerifyOk != nil && *run.VerifyOk {
		return shortTime(run.VerifiedAt) + " ok"
	}
	return shortTime(run.VerifiedAt) + " failed"
}

// joined renders a list of message codes for a table, and an absent one as a dash.
func joined(values *[]string) string {
	if values == nil || len(*values) == 0 {
		return "-"
	}
	return strings.Join(*values, ",")
}

func jobRefTable(accepted openapi.JobRef) Table {
	return Table{
		Columns: []string{"job", "status", "result"},
		Rows: [][]string{{
			accepted.JobId.String(),
			string(accepted.Status),
			text(accepted.ResultUrl),
		}},
	}
}

// pairs reads `k=v,k=v` into a map, refusing the whole list if any member is not a pair. A flag
// that silently dropped `bucket` because somebody typed a space would configure a target that
// points somewhere else.
func pairs(flag, raw string) (map[string]string, error) {
	values := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return values, nil
	}
	for _, part := range strings.Split(raw, ",") {
		key, value, found := strings.Cut(part, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !found || key == "" {
			return nil, usagef("%s takes k=v pairs separated by commas; %q is not one", flag, part)
		}
		values[key] = value
	}
	return values, nil
}

func anyMap(values map[string]string) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func boolean(value bool) *bool { return &value }

func count(value *int) string {
	if value == nil {
		return "-"
	}
	return strconv.Itoa(*value)
}

// byteSize renders a size the way a person reads one. Powers of two, because that is what a disk and
// a bucket report, and one decimal because the difference between 1.2 and 1.9 GiB matters.
func byteSize(size *int) string {
	if size == nil {
		return "-"
	}
	value := float64(*size)
	for _, unit := range []string{"B", "KiB", "MiB", "GiB", "TiB"} {
		if value < 1024 || unit == "TiB" {
			if unit == "B" {
				return strconv.FormatInt(int64(value), 10) + " B"
			}
			return strconv.FormatFloat(value, 'f', 1, 64) + " " + unit
		}
		value /= 1024
	}
	return strconv.Itoa(*size) + " B"
}
