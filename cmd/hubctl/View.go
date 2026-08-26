// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The saved views on the command line (D-07, D-08, D-09).

const viewsPath = "/views"

func viewGroup() group {
	return group{
		name:    "view",
		summary: "saved queries - and the files they turn into",
		commands: []command{
			{
				name:    "ls",
				usage:   "[--container <id>]",
				summary: "list the views the caller may see",
				run:     viewList,
			},
			{
				name: "create",
				usage: "--name <name> --scope TENANT|HUB|COLLECTION|ACCOUNT [--container <id>]" +
					" --layout <layout> --query <file|-> [--share]",
				summary: "save a query",
				run:     viewCreate,
			},
			{
				name:    "export",
				usage:   "<view-id> --format CSV|JSON|ICS [--zone <iana>] [--out <file>]",
				summary: "render a view whole",
				run:     viewExport,
			},
			{
				name:    "rm",
				usage:   "<view-id> [--expect-version <n>]",
				summary: "delete a view",
				run:     viewRemove,
			},
		},
	}
}

func viewList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "view", "ls", "[--container <id>]")
	container := flags.String("container", "",
		"the container whose shared views are wanted, together with the caller's own")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	query := url.Values{}
	if *container != "" {
		parsed, err := cli.parseID("--container", *container)
		if err != nil {
			return err
		}
		query.Set("container_id", parsed.String())
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var views []openapi.SavedView
	if err := client.Get(ctx, viewsPath, query, &views); err != nil {
		return err
	}
	return cli.Emit(views, viewTable(views))
}

func viewCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "view", "create",
		"--name <name> --scope <scope> [--container <id>] --layout <layout> --query <file|-> [--share]")
	name := flags.String("name", "", "what the view is called")
	scope := flags.String("scope", "", "TENANT, HUB, COLLECTION or ACCOUNT")
	container := flags.String("container", "", "the container a HUB or COLLECTION view belongs to")
	layout := flags.String("layout", "",
		"how a client draws it: LIST_COLLAPSED, LIST_EXPANDED, KANBAN or TIMELINE")
	query := flags.String("query", "", "the query document as JSON: a file, or - for standard input")
	share := flags.Bool("share", false, "share it with everyone who may read its scope")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *name == "" || *scope == "" || *layout == "" || *query == "" {
		return usagef("a view needs --name, --scope, --layout and --query")
	}

	document, err := readQueryDocument(cli, *query)
	if err != nil {
		return err
	}

	body := openapi.SavedViewCreate{
		Name: *name, Layout: *layout, Query: document,
		ScopeType: openapi.SavedViewCreateScopeType(*scope),
	}
	if *container != "" {
		parsed, err := cli.parseID("--container", *container)
		if err != nil {
			return err
		}
		body.ScopeId = &parsed
	}
	if *share {
		shared := openapi.SavedViewCreateSharing("SCOPE")
		body.Sharing = &shared
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.SavedView
	if err := client.Post(ctx, viewsPath, body, &created); err != nil {
		return err
	}
	return cli.Emit(created, viewTable([]openapi.SavedView{created}))
}

// readQueryDocument reads the query the same way a template's tree is read, and for the same
// reason: a filter is a document, and `--query -` is what makes it pipeable.
func readQueryDocument(cli *CLI, source string) (map[string]any, error) {
	var raw []byte
	var err error
	if source == "-" {
		raw, err = io.ReadAll(cli.In)
	} else {
		//nolint:gosec // G304: the file the user named; reading the query is the point.
		raw, err = os.ReadFile(source)
	}
	if err != nil {
		return nil, usageError{error: errorString("--query: " + err.Error())}
	}

	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		message, _ := cli.Catalogue.Message("query.document_unreadable", nil)
		return nil, usageError{error: errorString("--query: " + message)}
	}
	return document, nil
}

func viewExport(ctx context.Context, cli *CLI, args []string) error {
	const usage = "view export <view-id> --format CSV|JSON|ICS [--zone <iana>] [--out <file>]"
	view, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "view", "export", usage)
	format := flags.String("format", "", "CSV, JSON or ICS")
	zone := flags.String("zone", "", "the IANA zone the dates are written in; unset is the caller's own")
	out := flags.String("out", "", "write the file here instead of to standard output")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *format == "" {
		return usagef("an export needs --format")
	}

	wanted := openapi.ViewExportFormat(strings.ToUpper(*format))
	if !wanted.Valid() {
		message, _ := cli.Catalogue.Message(
			"views.export_format_unknown", map[string]string{"value": *format})
		return usageError{error: errorString("--format: " + message)}
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	// The answer is a file rather than a payload, so it is taken as bytes and written where the
	// caller said. --json is not consulted: a CSV is not JSON, and an export that reshaped itself
	// because of a global flag would be a surprise in a pipeline.
	body := openapi.ViewExport{Format: wanted, TimeZone: optional(*zone)}
	document, truncated, err := client.Download(
		ctx, viewsPath+"/"+view.String()+":export", body)
	if err != nil {
		return err
	}
	if truncated {
		// On standard error, so that a redirected file is exactly the export and the warning is
		// still seen by whoever ran it.
		printf(cli.Err, "the export reached the row cap; what follows is the first page of it\n")
	}

	if *out == "" {
		_, _ = cli.Out.Write(document)
		return nil
	}
	if err := os.WriteFile(*out, document, 0o600); err != nil {
		return err
	}
	printf(cli.Err, "written: %s (%d bytes)\n", *out, len(document))
	return nil
}

func viewRemove(ctx context.Context, cli *CLI, args []string) error {
	const usage = "view rm <view-id> [--expect-version <n>]"
	view, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "view", "rm", "<view-id> [--expect-version <n>]")
	expected := flags.Int("expect-version", 0, "refuse unless the view is still at this version")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, viewsPath+"/"+view.String(), versionPrecondition(*expected)); err != nil {
		return err
	}
	// The feed that served it keeps its token and serves nothing, which is worth saying to
	// whoever is about to wonder why their calendar went quiet.
	printf(cli.Err, "view deleted: %s (a calendar feed over it now serves nothing)\n", view)
	return nil
}

func viewTable(views []openapi.SavedView) Table {
	rows := make([][]string, 0, len(views))
	for _, view := range views {
		rows = append(rows, []string{
			view.Id.String(),
			view.Name,
			string(view.ScopeType),
			view.Layout,
			string(view.Sharing),
			strconv.Itoa(view.Version),
		})
	}
	return Table{
		Columns: []string{"id", "name", "scope", "layout", "sharing", "version"},
		Rows:    rows,
	}
}
