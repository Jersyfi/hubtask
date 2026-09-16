// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const importsPath = "/imports"

// importGroup is `hubctl import <kind> <file> --hub <id>` (P-08): the upload, staged as an
// import, and the request, and the job followed to its end - because an import that stops after
// the upload is a file nobody asked anything of.
func importGroup() group {
	return group{
		name:    "import",
		summary: "bring another system's entries in: a CSV, a Trello board, a Google Tasks export, a Microsoft To Do dump",
		usage:   "<csv|trello|google-tasks|microsoft-todo> <file> --hub <id> [--map <field>=<column>]... [--wait <d>]",
		run:     importRun,
		// The job is followed to its end: bounded by --wait rather than by the per-command
		// deadline, which a file of a few thousand rows would outrun.
		unbounded: true,
	}
}

var importKinds = map[string]openapi.ImportKind{
	"csv":            openapi.ImportKindCSV,
	"trello":         openapi.ImportKindTRELLO,
	"google-tasks":   openapi.ImportKindGOOGLETASKS,
	"microsoft-todo": openapi.ImportKindMICROSOFTTODO,
	"todo":           openapi.ImportKindMICROSOFTTODO,
}

// graphRequests is what `hubctl import microsoft-todo` without a file prints: the two requests
// the file comes from, made by the person with their own sign-in, and the shape to hand back.
const graphRequests = `Microsoft To Do has no export. The file is what the Graph API answers, fetched with your own
sign-in (https://developer.microsoft.com/graph/graph-explorer, or a token you hold):

  1. GET https://graph.microsoft.com/v1.0/me/todo/lists
  2. for each list:  GET https://graph.microsoft.com/v1.0/me/todo/lists/{id}/tasks

Paste each list's tasks answer under that list as "tasks" in the first document, so that the file
reads {"value": [{"id": ..., "displayName": ..., "tasks": {"value": [...]}}, ...]}, and then:

  hubctl import microsoft-todo <file> --hub <id>`

func importRun(ctx context.Context, cli *CLI, args []string) error {
	const usage = "import <csv|trello|google-tasks|microsoft-todo> <file> --hub <id> [--map <field>=<column>]..."
	// Microsoft To Do has no export (backlog 0.9.0, decision 8): the file is what the Graph API
	// answers, fetched by the person. `hubctl import microsoft-todo` alone says how.
	if len(args) == 1 && (strings.EqualFold(args[0], "microsoft-todo") || strings.EqualFold(args[0], "todo")) {
		printf(cli.Out, "%s\n", graphRequests)
		return nil
	}
	if len(args) < 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return usagef("the kind and the file come first: hubctl %s", usage)
	}
	kind, known := importKinds[strings.ToLower(args[0])]
	if !known {
		return usagef("%q is not a kind hubctl imports: hubctl %s", args[0], usage)
	}
	path := args[1]
	flags := commandFlags(cli, "import", "", "<kind> <file> --hub <id> [--map <field>=<column>]...")
	hub := flags.String("hub", "", "the hub the collections land under")
	var mappings multiFlag
	flags.Var(&mappings, "map", "a CSV column by field, field=column; repeatable")
	wait := waitFlag(flags)
	if err := parseCommand(flags, args[2:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return usagef("unexpected argument %q: hubctl %s", flags.Arg(0), usage)
	}
	if *hub == "" {
		return usagef("say where the collections land: --hub <id>")
	}
	hubID, err := cli.parseID("--hub", *hub)
	if err != nil {
		return err
	}
	mapping := map[string]string{}
	for _, each := range mappings {
		field, column, found := strings.Cut(each, "=")
		if !found || field == "" || column == "" {
			return usagef("--map takes field=column, got %q", each)
		}
		mapping[field] = column
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: the file the user named; reading it is the point.
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}

	// The upload, staged as an import: the same three steps as `media upload`, under the usage
	// that says the file attaches to nothing.
	fileName := filepath.Base(path)
	var staged openapi.MediaObject
	if err := client.Post(ctx, mediaPath, openapi.MediaUploadRequest{
		Size: int64(len(data)), Usage: openapi.MediaUploadRequestUsageIMPORT, FileName: &fileName,
		ContentType: optional(importContentType(kind)),
	}, &staged); err != nil {
		return err
	}
	if staged.Upload == nil {
		return errorString("the installation staged the upload but named no target to put the bytes")
	}
	target := staged.Upload.Url
	if strings.HasPrefix(target, "/") {
		target = cli.Profile.BaseURL + target
	}
	if err := client.Upload(ctx, string(staged.Upload.Method), target, data); err != nil {
		return err
	}
	if err := client.Post(ctx, mediaPath+"/"+staged.Id.String()+":confirm", nil, nil); err != nil {
		return err
	}

	request := openapi.ImportRequest{MediaId: staged.Id, Kind: kind, HubId: hubID}
	if len(mapping) > 0 {
		request.Mapping = &mapping
	}
	var accepted openapi.JobRef
	if err := client.Post(ctx, importsPath, request, &accepted); err != nil {
		return err
	}
	if _, err := cli.followJob(ctx, client, accepted.JobId, *wait); err != nil {
		return err
	}
	if accepted.ResultUrl == nil {
		return errorString("the import was accepted but named no result")
	}
	var run openapi.ImportRun
	if err := client.Get(ctx, *accepted.ResultUrl, nil, &run); err != nil {
		return err
	}
	return cli.Emit(run, importTable(run))
}

func importContentType(kind openapi.ImportKind) string {
	if kind == openapi.ImportKindCSV {
		return "text/csv"
	}
	return "application/json"
}

func importTable(run openapi.ImportRun) Table {
	row := []string{run.Id.String(), string(run.Kind), string(run.Status), "", "", "", ""}
	if run.Report != nil {
		row[3], row[4] = count(run.Report.New), count(run.Report.Skipped)
	}
	if run.Refused != nil {
		row[5] = count(intPtr(len(*run.Refused)))
	}
	if run.ErrorCode != nil {
		row[6] = *run.ErrorCode
	}
	return Table{Columns: []string{"id", "kind", "status", "new", "skipped", "refused", "error"}, Rows: [][]string{row}}
}

func intPtr(n int) *int { return &n }

// multiFlag collects a repeatable flag.
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }
