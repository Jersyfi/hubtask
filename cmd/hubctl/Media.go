// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const mediaPath = "/media"

func mediaGroup() group {
	return group{
		name:    "media",
		summary: "files - uploaded once, attached where they belong",
		commands: []command{
			{
				name:    "upload",
				usage:   "<file> [--type <mime>] [--name <filename>] [--usage ATTACHMENT|COVER]",
				summary: "upload a file: stage it, put the bytes, confirm",
				run:     mediaUpload,
			},
			{
				name:    "attach",
				usage:   "<item-id> --media <id>",
				summary: "attach an uploaded file to an entry",
				run:     mediaAttach,
			},
		},
	}
}

// mediaUpload walks the three-step flow the contract declares - stage, put, confirm - as one
// command, because no step of it means anything to a person on its own: an upload that stops
// after staging is a PENDING object the reconciliation job will sweep away.
func mediaUpload(ctx context.Context, cli *CLI, args []string) error {
	const usage = "media upload <file> [--type <mime>] [--name <filename>] [--usage ATTACHMENT|COVER]"
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return usagef("the file comes first: hubctl %s", usage)
	}
	path := args[0]
	flags := commandFlags(cli, "media", "upload", "<file> [--type <mime>] [--name <filename>] [--usage ATTACHMENT|COVER]")
	claim := flags.String("type", "", "the content type to declare; the server judges the bytes either way")
	name := flags.String("name", "", "the file name to store; the file's own name when unset")
	purpose := flags.String("usage", string(openapi.MediaUploadRequestUsageATTACHMENT), "ATTACHMENT or COVER")
	if err := parseCommand(flags, args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return usagef("unexpected argument %q: hubctl %s takes one file", flags.Arg(0), usage)
	}

	mediaUsage := openapi.MediaUploadRequestUsage(*purpose)
	if !mediaUsage.Valid() {
		message, _ := cli.Catalogue.Message("media.usage_unknown", map[string]string{"value": *purpose})
		return usageError{error: errorString(message)}
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: the file the user named; reading it is the point.
	if err != nil {
		return err
	}
	fileName := *name
	if fileName == "" {
		fileName = filepath.Base(path)
	}

	client, err := cli.client()
	if err != nil {
		return err
	}

	body := openapi.MediaUploadRequest{
		Size:        int64(len(data)),
		Usage:       mediaUsage,
		FileName:    &fileName,
		ContentType: optional(*claim),
	}
	var staged openapi.MediaObject
	if err := client.Post(ctx, mediaPath, body, &staged); err != nil {
		return err
	}
	if staged.Upload == nil {
		return errorString("the installation staged the upload but named no target to put the bytes")
	}

	// A local-storage installation answers with a path relative to its own origin; a presigned
	// bucket answers with an absolute address. Either way the URL is called as given, against the
	// profile's installation where it is relative.
	target := staged.Upload.Url
	if strings.HasPrefix(target, "/") {
		target = cli.Profile.BaseURL + target
	}
	if err := client.Upload(ctx, string(staged.Upload.Method), target, data); err != nil {
		return err
	}

	var confirmed openapi.MediaObject
	if err := client.Post(ctx, mediaPath+"/"+staged.Id.String()+":confirm", nil, &confirmed); err != nil {
		return err
	}
	return cli.Emit(confirmed, mediaTable([]openapi.MediaObject{confirmed}))
}

func mediaAttach(ctx context.Context, cli *CLI, args []string) error {
	const usage = "media attach <item-id> --media <id>"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "media", "attach", "<item-id> --media <id>")
	media := flags.String("media", "", "the media object to attach; `hubctl media upload` prints its identifier")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *media == "" {
		return usagef("say what to attach: --media <id>")
	}
	object, err := cli.parseID("--media", *media)
	if err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var attachments openapi.ItemAttachments
	target := itemsPath + "/" + item.String() + "/attachments/" + object.String()
	if err := client.Put(ctx, target, nil, &attachments); err != nil {
		return err
	}

	rows := make([][]string, 0, len(attachments.MediaIds))
	for _, attached := range attachments.MediaIds {
		rows = append(rows, []string{attachments.ItemId.String(), attached.String()})
	}
	return cli.Emit(attachments, Table{Columns: []string{"item", "media"}, Rows: rows})
}

func mediaTable(objects []openapi.MediaObject) Table {
	rows := make([][]string, 0, len(objects))
	for _, object := range objects {
		rows = append(rows, []string{
			object.Id.String(),
			string(object.Status),
			object.ContentType,
			text(object.FileName),
			strconv.FormatInt(object.Size, 10),
			strconv.Itoa(object.RefCount),
		})
	}
	return Table{Columns: []string{"id", "status", "type", "name", "size", "refs"}, Rows: rows}
}
