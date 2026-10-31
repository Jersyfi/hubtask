// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

const workspaceArchivePath = "/backups:download-workspace"

// backupDownload writes the owner's archive of the workspace (UC-BAK-07) to a file. The bytes go
// to `<file>.part` and are renamed only once the ZIP's directory reads, so a transfer cut short
// leaves nothing at --out that looks like an archive.
func backupDownload(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "backup", "download", "--out <file> [--wait <d>]")
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

	partial := *out + ".part"
	var size int64
	err = cli.proveAgain(bounded, client, func(stepUp string) error {
		response, err := client.OpenDownload(bounded, workspaceArchivePath, stepUpProof(stepUp))
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
	file, err := os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", name, err)
	}
	size, copyErr := io.Copy(file, body)
	if err := errors.Join(copyErr, file.Sync(), file.Close()); err != nil {
		return 0, fmt.Errorf("%w (%v)", errArchiveIncomplete, err)
	}
	reader, err := zip.OpenReader(name)
	if err != nil {
		return 0, errArchiveIncomplete
	}
	_ = reader.Close()
	return size, nil
}
