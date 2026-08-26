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
	"time"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The templates on the command line (D-06, D-09).

const templatesPath = "/templates"

func templateGroup() group {
	return group{
		name:    "template",
		summary: "shapes of work, stamped out when they are needed",
		commands: []command{
			{
				name:    "ls",
				usage:   "[--container <id>]",
				summary: "list the templates usable in a container",
				run:     templateList,
			},
			{
				name: "create",
				usage: "--name <name> --scope TENANT|HUB|COLLECTION [--container <id>]" +
					" --root-type <type> --nodes <file|->",
				summary: "define a template from a node tree",
				run:     templateCreate,
			},
			{
				name:    "instantiate",
				usage:   "<template-id> --collection <id> [--parent <id>] [--anchor <date>] [--title <t>]",
				summary: "stamp a template out into a collection",
				run:     templateInstantiate,
			},
			{
				name:    "rm",
				usage:   "<template-id> [--expect-version <n>]",
				summary: "delete a template; what it stamped out stays",
				run:     templateRemove,
			},
		},
	}
}

func templateList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "template", "ls", "[--container <id>]")
	container := flags.String("container", "",
		"the container whose templates are wanted, together with the ones above it")
	size := flags.Int("size", 0, "how many per page (the server decides when unset)")
	cursor := flags.String("cursor", "", "continue the previous page")
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
	if *size > 0 {
		query.Set("size", strconv.Itoa(*size))
	}
	if *cursor != "" {
		query.Set("cursor", *cursor)
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var page openapi.TemplatePage
	if err := client.Get(ctx, templatesPath, query, &page); err != nil {
		return err
	}
	cli.reportMore(page.Page)
	return cli.Emit(page, templateTable(page.Data))
}

func templateCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "template", "create",
		"--name <name> --scope TENANT|HUB|COLLECTION [--container <id>] --root-type <type> --nodes <file|->")
	name := flags.String("name", "", "what the template is called")
	scope := flags.String("scope", "", "TENANT, HUB or COLLECTION")
	container := flags.String("container", "", "the container a HUB or COLLECTION template belongs to")
	rootType := flags.String("root-type", "", "TASK, WORK_PACKAGE or ACTIVITY")
	nodes := flags.String("nodes", "", "the node tree as JSON: a file, or - for standard input")
	description := flags.String("description", "", "what the template is for")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *name == "" || *scope == "" || *rootType == "" || *nodes == "" {
		return usagef("a template needs --name, --scope, --root-type and --nodes")
	}

	// The tree comes from a file or from standard input rather than from flags, and that is the
	// one shape decision this group makes. A tree is a document: expressing one as repeated flags
	// would need a syntax for depth that nobody could read back, and `--nodes -` is what makes
	// `hubctl template create ... --nodes - < move-house.json` the obvious thing to type.
	tree, err := readTemplateNodes(cli, *nodes)
	if err != nil {
		return err
	}

	itemType, err := cli.itemType(*rootType)
	if err != nil {
		return err
	}
	body := openapi.TemplateInput{
		Name: *name, ScopeType: openapi.TemplateScope(*scope), RootType: itemType,
		Nodes: tree, Description: optional(*description),
	}
	if *container != "" {
		parsed, err := cli.parseID("--container", *container)
		if err != nil {
			return err
		}
		body.ScopeId = &parsed
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.Template
	if err := client.Post(ctx, templatesPath, body, &created); err != nil {
		return err
	}
	return cli.Emit(created, templateTable([]openapi.Template{created}))
}

// readTemplateNodes reads the tree. The document is the contract's `nodes` array, so that what a
// person writes by hand is what `hubctl --json template ls` prints back.
func readTemplateNodes(cli *CLI, source string) ([]openapi.TemplateNode, error) {
	var raw []byte
	var err error
	if source == "-" {
		raw, err = io.ReadAll(cli.In)
	} else {
		//nolint:gosec // G304: the file the user named; reading the tree is the point.
		raw, err = os.ReadFile(source)
	}
	if err != nil {
		return nil, usageError{error: errorString("--nodes: " + err.Error())}
	}

	var nodes []openapi.TemplateNode
	if err := json.Unmarshal(raw, &nodes); err != nil {
		message, _ := cli.Catalogue.Message("templates.nodes_unreadable", nil)
		return nil, usageError{error: errorString("--nodes: " + message)}
	}
	if len(nodes) == 0 {
		return nil, usagef("--nodes carried no node; a template is a tree of at least one")
	}
	return nodes, nil
}

func templateInstantiate(ctx context.Context, cli *CLI, args []string) error {
	const usage = "template instantiate <template-id> --collection <id> [--parent <id>] [--anchor <date>] [--title <t>]"
	template, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "template", "instantiate", usage)
	collection := flags.String("collection", "", "the collection the tree lands in")
	parent := flags.String("parent", "", "the entry the tree hangs from")
	anchor := flags.String("anchor", "",
		"the day the relative dates count from, as 2026-09-10; unset counts from today")
	title := flags.String("title", "", "the root entry's title, overriding the template's own")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *collection == "" {
		return usagef("an instantiation needs --collection")
	}

	target, err := cli.parseID("--collection", *collection)
	if err != nil {
		return err
	}
	body := openapi.TemplateInstantiation{CollectionId: target, Title: optional(*title)}
	if *parent != "" {
		parsed, err := cli.parseID("--parent", *parent)
		if err != nil {
			return err
		}
		body.ParentId = &parsed
	}
	if *anchor != "" {
		day, err := time.Parse(time.DateOnly, *anchor)
		if err != nil {
			message, _ := cli.Catalogue.Message(
				"templates.anchor_invalid", map[string]string{"value": *anchor})
			return usageError{error: errorString("--anchor: " + message)}
		}
		body.AnchorDate = &openapitypes.Date{Time: day}
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var instance openapi.TemplateInstance
	if err := client.Post(
		ctx, templatesPath+"/"+template.String()+":instantiate", body, &instance,
	); err != nil {
		return err
	}
	if err := cli.Emit(instance, instanceTable(instance)); err != nil {
		return err
	}
	// I-W6 again: what the destination could not carry is said out loud. Under --json it is in
	// the payload; in a table it would otherwise be a number nobody can act on.
	if !cli.JSON {
		for _, dropped := range instance.DroppedReferences {
			reason, known := cli.Catalogue.Message(dropped.Code, nil)
			if !known {
				reason = dropped.Code
			}
			printf(cli.Err, "dropped in the destination: %s %s - %s\n",
				dropped.Kind, dropped.Id, reason)
		}
	}
	return nil
}

func templateRemove(ctx context.Context, cli *CLI, args []string) error {
	const usage = "template rm <template-id> [--expect-version <n>]"
	template, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "template", "rm", "<template-id> [--expect-version <n>]")
	expected := flags.Int("expect-version", 0, "refuse unless the template is still at this version")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(
		ctx, templatesPath+"/"+template.String(), versionPrecondition(*expected),
	); err != nil {
		return err
	}
	printf(cli.Err, "template deleted: %s (what it stamped out stays)\n", template)
	return nil
}

func templateTable(templates []openapi.Template) Table {
	rows := make([][]string, 0, len(templates))
	for _, template := range templates {
		rows = append(rows, []string{
			template.Id.String(),
			template.Name,
			string(template.ScopeType),
			id(template.ScopeId),
			string(template.RootType),
			strconv.Itoa(countNodes(template.Nodes)),
			strconv.Itoa(template.Version),
		})
	}
	return Table{
		Columns: []string{"id", "name", "scope", "container", "root", "nodes", "version"},
		Rows:    rows,
	}
}

// countNodes walks the tree, because "how big is this template" is the question a list answers and
// the contract carries the shape rather than the count.
func countNodes(nodes []openapi.TemplateNode) int {
	total := 0
	for _, node := range nodes {
		total++
		if node.Children != nil {
			total += countNodes(*node.Children)
		}
	}
	return total
}

func instanceTable(instance openapi.TemplateInstance) Table {
	return Table{
		Columns: []string{"root", "created", "dropped"},
		Rows: [][]string{{
			instance.RootItemId.String(),
			strconv.Itoa(instance.Created),
			strconv.Itoa(len(instance.DroppedReferences)),
		}},
	}
}
