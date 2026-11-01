// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const workspaceArchivePath = "/backups:download-workspace"

// envArchivePassphrase is the passphrase a download is encrypted to and a restore opens it with.
const envArchivePassphrase = "HUBTASK_ARCHIVE_PASSPHRASE" //nolint:gosec // G101: a variable's name

// backupDownload writes the owner's archive of the workspace (UC-BAK-07) to a file. The bytes go
// to `<file>.part` and are renamed only once the ZIP's directory reads, so a transfer cut short
// leaves nothing at --out that looks like an archive.
func backupDownload(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "backup", "download", "--out <file> [--wait <d>]  (encrypted to $HUBTASK_ARCHIVE_PASSPHRASE when set)")
	out := flags.String("out", "", "write the archive here (required: it is binary)")
	wait := waitFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return usagef("unexpected argument %q: hubctl backup download takes only flags", flags.Arg(0))
	}
	if *out == "" {
		return usagef("hubctl backup download needs --out <file>")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, *wait)
	defer cancel()

	// The passphrase from the environment, never a flag: a flag stays in the shell's history.
	var body []byte
	if passphrase := cli.Env(envArchivePassphrase); passphrase != "" {
		if body, err = json.Marshal(openapi.WorkspaceArchiveDownload{Passphrase: &passphrase}); err != nil {
			return err
		}
	}

	partial := *out + ".part"
	var size int64
	err = cli.proveAgain(bounded, client, func(stepUp string) error {
		response, err := client.OpenDownload(bounded, workspaceArchivePath, stepUpProof(stepUp), body)
		if err != nil {
			return err
		}
		defer func() { _ = response.Body.Close() }()
		size, err = writeWhole(partial, response.Body)
		return err
	})
	if err != nil {
		_ = os.Remove(partial)
		return err
	}
	if err := os.Rename(partial, *out); err != nil {
		_ = os.Remove(partial)
		return fmt.Errorf("keeping %s: %w", *out, err)
	}
	printf(cli.Err, "wrote the workspace's archive, %d bytes, to %s\n", size, *out)
	return nil
}

var errArchiveIncomplete = errors.New("the archive arrived incomplete; nothing was kept - try again after the window")

// writeWhole copies the body to the file and answers its size once the file reads as a ZIP.
func writeWhole(name string, body io.Reader) (int64, error) {
	file, err := os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // G304: the file the person named
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", name, err)
	}
	size, copyErr := io.Copy(file, body)
	if err := errors.Join(copyErr, file.Sync(), file.Close()); err != nil {
		return 0, fmt.Errorf("%w: %w", errArchiveIncomplete, err)
	}
	reader, err := zip.OpenReader(name)
	if err != nil {
		return 0, errArchiveIncomplete
	}
	_ = reader.Close()
	return size, nil
}
