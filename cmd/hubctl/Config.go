// SPDX-License-Identifier: Apache-2.0
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
)

// The configuration document (F-6): the same verbs as REST and MCP - schema, export, explain,
// validate, plan, apply, history, rollback - with the proposals a person decides and templates.
// The documents are JSON files; `-f -` reads standard input, so an export pipes into a plan.

const configPath = "/config"

func configGroup() group {
	return group{
		name:    "config",
		summary: "the workspace's configuration as one document: plan, apply, history, rollback",
		commands: []command{
			{name: "schema", summary: "the document's JSON Schema", run: configGet("schema", "/meta/config-schema")},
			{name: "export", summary: "the document in force, with its hash; --hub a hub's own", run: configExport},
			{name: "explain", summary: "one key: its value, source, lock and who may change it", run: configExplain},
			{name: "validate", summary: "what a document refuses", run: configValidate},
			{name: "plan", summary: "what a document would change, writing nothing", run: configPlan},
			{name: "apply", summary: "apply a planned document by its plan hash", run: configApply},
			{name: "history", summary: "the configuration's versions", run: configHistory},
			{name: "rollback", summary: "plan going back to a version; --yes applies it", run: configRollback},
			{name: "proposals", summary: "the changes proposed for a person to decide", run: configProposals},
			{name: "approve", summary: "apply a proposed change", run: configDecide("approve", "APPLY")},
			{name: "decline", summary: "decline a proposed change", run: configDecide("decline", "DECLINE")},
			{name: "policy", summary: "what an agent's apply does: ALLOWED, APPROVAL or NEVER", run: configPolicy},
			{name: "templates", summary: "the workspace's templates and the updates available", run: configTemplates},
			{name: "publish", summary: "publish a template's next version", run: configPublish},
			{name: "take", summary: "plan taking or upgrading a template", run: configTake},
		},
	}
}

func configGet(verb, path string) func(context.Context, *CLI, []string) error {
	return func(ctx context.Context, cli *CLI, args []string) error {
		flags := commandFlags(cli, "config", verb, "")
		if err := parseCommand(flags, args); err != nil {
			return err
		}
		client, err := cli.client()
		if err != nil {
			return err
		}
		var out map[string]any
		if err := client.Get(ctx, path, nil, &out); err != nil {
			return err
		}
		return cli.emitJSON(out)
	}
}

// hubFlag names the hub whose own document a verb is for: its modules, structure and workflow
// scheme. Omitted is the workspace's document.
func hubFlag(flags interface {
	String(name, value, usage string) *string
}) *string {
	return flags.String("hub", "", "a hub's identifier, for the hub's own document")
}

// withHubQuery and withHubBody carry the hub a verb names.
func withHubQuery(query url.Values, hub string) url.Values {
	if hub != "" {
		query.Set("hub_id", hub)
	}
	return query
}

func withHubBody(body map[string]any, hub string) map[string]any {
	if hub != "" {
		body["hub_id"] = hub
	}
	return body
}

func configExport(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "export", "[--hub <id>]")
	hub := hubFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Get(ctx, configPath, withHubQuery(url.Values{}, *hub), &out); err != nil {
		return err
	}
	return cli.emitJSON(out)
}

func configExplain(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "explain", "<key> [--hub <id>]")
	hub := hubFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return usagef("hubctl config explain takes one key path, such as types.incident")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Get(ctx, configPath+"/explain", withHubQuery(url.Values{"key": {flags.Arg(0)}}, *hub), &out); err != nil {
		return err
	}
	rows := [][]string{}
	for _, field := range []string{"key", "source", "locked_by", "permission", "operation", "applies_to"} {
		rows = append(rows, []string{field, cellOf(out[field])})
	}
	return cli.Emit(out, Table{Columns: []string{"", ""}, Rows: rows})
}

// readDocument reads a configuration document from a file, or standard input for "-".
func readDocument(cli *CLI, source string) (map[string]any, error) {
	if source == "" {
		return nil, usagef("a document is named with -f <file>, or -f - for standard input")
	}
	var raw []byte
	var err error
	if source == "-" {
		raw, err = io.ReadAll(cli.In)
	} else {
		raw, err = os.ReadFile(source) //nolint:gosec // G304: the file the user named; reading it is the point.
	}
	if err != nil {
		return nil, usageError{error: errorString("-f: " + err.Error())}
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, usageError{error: errorString("-f: not a JSON document")}
	}
	// An export is accepted as it is: its document is what is planned.
	if inner, ok := document["document"].(map[string]any); ok {
		document = inner
	}
	return document, nil
}

func configMode(replace bool) string {
	if replace {
		return "REPLACE"
	}
	return "MERGE"
}

func configValidate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "validate", "-f <file> [--replace] [--hub <id>]")
	file := flags.String("f", "", "the document; - reads standard input")
	replace := flags.Bool("replace", false, "state each section the document names whole")
	hub := hubFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	document, err := readDocument(cli, *file)
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Post(ctx, configPath+":validate", withHubBody(map[string]any{"document": document, "mode": configMode(*replace)}, *hub), &out); err != nil {
		return err
	}
	return cli.Emit(out, configRefusalTable(out))
}

func configPlan(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "plan", "-f <file> [--replace] [--hub <id>]")
	file := flags.String("f", "", "the document; - reads standard input")
	replace := flags.Bool("replace", false, "state each section the document names whole")
	hub := hubFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	document, err := readDocument(cli, *file)
	if err != nil {
		return err
	}
	return planAndShow(ctx, cli, withHubBody(map[string]any{"document": document, "mode": configMode(*replace)}, *hub))
}

func planAndShow(ctx context.Context, cli *CLI, body map[string]any) error {
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Post(ctx, configPath+":plan", body, &out); err != nil {
		return err
	}
	return cli.Emit(out, configPlanTable(out))
}

func configPlanTable(plan map[string]any) Table {
	rows := [][]string{}
	changes, _ := plan["changes"].([]any)
	for _, raw := range changes {
		change, _ := raw.(map[string]any)
		fields, _ := change["fields"].([]any)
		names := make([]string, 0, len(fields))
		for _, f := range fields {
			names = append(names, cellOf(f))
		}
		rows = append(rows, []string{cellOf(change["op"]), cellOf(change["path"]), strings.Join(names, ",")})
	}
	refusals, _ := plan["refusals"].([]any)
	for _, raw := range refusals {
		refusal, _ := raw.(map[string]any)
		rows = append(rows, []string{"REFUSED", cellOf(refusal["path"]), cellOf(refusal["code"])})
	}
	reach, _ := plan["reach"].([]any)
	for _, raw := range reach {
		r, _ := raw.(map[string]any)
		var what []string
		if entries, held := r["entries"]; held {
			what = append(what, cellOf(entries)+" entries move")
		}
		if accounts, held := r["accounts"]; held {
			what = append(what, cellOf(accounts)+" people, "+cellOf(r["groups"])+" groups")
		}
		rows = append(rows, []string{"REACHES", cellOf(r["path"]), strings.Join(what, "; ")})
	}
	rows = append(rows, []string{"PLAN", cellOf(plan["plan_hash"]), ""})
	return Table{Columns: []string{"op", "key", "fields"}, Rows: rows}
}

func configRefusalTable(out map[string]any) Table {
	rows := [][]string{}
	refusals, _ := out["refusals"].([]any)
	for _, raw := range refusals {
		refusal, _ := raw.(map[string]any)
		rows = append(rows, []string{cellOf(refusal["path"]), cellOf(refusal["code"])})
	}
	return Table{Columns: []string{"key", "refused"}, Rows: rows}
}

func configApply(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "apply", "-f <file> --plan <hash> [--replace] [--acknowledge] [--template key@version] [--hub <id>]")
	hub := hubFlag(flags)
	file := flags.String("f", "", "the document that was planned; - reads standard input")
	planHash := flags.String("plan", "", "the plan hash `hubctl config plan` answered")
	replace := flags.Bool("replace", false, "state each section the document names whole")
	acknowledge := flags.Bool("acknowledge", false, "the plan hides data or changes permissions, and that is seen")
	template := flags.String("template", "", "the template@version the document came from")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *planHash == "" {
		return usagef("hubctl config apply needs --plan <hash> from hubctl config plan")
	}
	document, err := readDocument(cli, *file)
	if err != nil {
		return err
	}
	body := map[string]any{"document": document, "mode": configMode(*replace), "plan_hash": *planHash, "acknowledge": *acknowledge}
	if *template != "" {
		body["template"] = *template
	}
	return applyAndShow(ctx, cli, withHubBody(body, *hub))
}

func applyAndShow(ctx context.Context, cli *CLI, body map[string]any) error {
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Post(ctx, configPath+":apply", body, &out); err != nil {
		return err
	}
	return cli.Emit(out, configResultTable(out))
}

func configResultTable(out map[string]any) Table {
	row := []string{cellOf(out["state"]), "", ""}
	if version, ok := out["version"].(map[string]any); ok {
		row[1] = cellOf(version["version"])
	}
	if proposal, ok := out["proposal"].(map[string]any); ok {
		row[2] = cellOf(proposal["id"])
	}
	return Table{Columns: []string{"state", "version", "proposal"}, Rows: [][]string{row}}
}

func configHistory(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "history", "[--before <version>] [--hub <id>]")
	before := flags.Int("before", 0, "list the versions before this one")
	hub := hubFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	query := url.Values{}
	if *before > 0 {
		query.Set("before", strconv.Itoa(*before))
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Get(ctx, configPath+"/versions", withHubQuery(query, *hub), &out); err != nil {
		return err
	}
	rows := [][]string{}
	data, _ := out["data"].([]any)
	for _, raw := range data {
		v, _ := raw.(map[string]any)
		rows = append(rows, []string{
			cellOf(v["version"]), cellOf(v["created_at"]), cellOf(v["actor_kind"]), cellOf(v["proposed_by"]),
			cellOf(v["added"]) + "/" + cellOf(v["changed"]) + "/" + cellOf(v["removed"]),
			cellOf(v["template"]), cellOf(v["rolled_back_to"]),
		})
	}
	return cli.Emit(out, Table{Columns: []string{"version", "at", "by", "proposed by", "added/changed/removed", "template", "rollback to"}, Rows: rows})
}

func configRollback(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "rollback", "<version> [--yes] [--acknowledge] [--hub <id>]")
	hub := hubFlag(flags)
	yes := flags.Bool("yes", false, "apply the plan rather than only show it")
	acknowledge := flags.Bool("acknowledge", false, "the plan hides data or changes permissions, and that is seen")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	version, err := strconv.Atoi(flags.Arg(0))
	if flags.NArg() != 1 || err != nil || version < 1 {
		return usagef("hubctl config rollback takes one version number")
	}
	if !*yes {
		return planAndShow(ctx, cli, withHubBody(map[string]any{"to_version": version}, *hub))
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var plan map[string]any
	if err := client.Post(ctx, configPath+":plan", withHubBody(map[string]any{"to_version": version}, *hub), &plan); err != nil {
		return err
	}
	if plan["plan_hash"] == nil {
		return cli.Emit(plan, configPlanTable(plan))
	}
	return applyAndShow(ctx, cli, withHubBody(map[string]any{"to_version": version, "plan_hash": plan["plan_hash"], "acknowledge": *acknowledge}, *hub))
}

func configProposals(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "proposals", "[--all]")
	all := flags.Bool("all", false, "list decided proposals too")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	query := url.Values{}
	if !*all {
		query.Set("state", "WAITING")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Get(ctx, configPath+"/proposals", query, &out); err != nil {
		return err
	}
	rows := [][]string{}
	data, _ := out["data"].([]any)
	for _, raw := range data {
		p, _ := raw.(map[string]any)
		flagged := ""
		if p["hides_data"] == true || p["changes_permissions"] == true {
			flagged = "acknowledge"
		}
		rows = append(rows, []string{
			cellOf(p["id"]), cellOf(p["state"]), cellOf(p["created_at"]),
			cellOf(p["added"]) + "/" + cellOf(p["changed"]) + "/" + cellOf(p["removed"]), flagged,
		})
	}
	return cli.Emit(out, Table{Columns: []string{"proposal", "state", "at", "added/changed/removed", "needs"}, Rows: rows})
}

func configDecide(verb, decision string) func(context.Context, *CLI, []string) error {
	return func(ctx context.Context, cli *CLI, args []string) error {
		flags := commandFlags(cli, "config", verb, "<proposal-id> [--acknowledge]")
		acknowledge := flags.Bool("acknowledge", false, "the plan hides data or changes permissions, and that is seen")
		if err := parseCommand(flags, args); err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return usagef("hubctl config %s takes one proposal", verb)
		}
		id, err := cli.parseID("proposal", flags.Arg(0))
		if err != nil {
			return err
		}
		client, err := cli.client()
		if err != nil {
			return err
		}
		var out map[string]any
		body := map[string]any{"decision": decision, "acknowledge": *acknowledge}
		if err := client.Post(ctx, configPath+"/proposals/"+url.PathEscape(id.String())+":decide", body, &out); err != nil {
			return err
		}
		return cli.Emit(out, configResultTable(out))
	}
}

func configPolicy(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "policy", "ALLOWED|APPROVAL|NEVER")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	value := strings.ToUpper(flags.Arg(0))
	if flags.NArg() != 1 || (value != "ALLOWED" && value != "APPROVAL" && value != "NEVER") {
		return usagef("hubctl config policy takes ALLOWED, APPROVAL or NEVER")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Put(ctx, configPath+"/policy", map[string]any{"agent_apply": value}, &out); err != nil {
		return err
	}
	return cli.Emit(out, Table{Columns: []string{"agent apply"}, Rows: [][]string{{cellOf(out["agent_apply"])}}})
}

func configTemplates(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "templates", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Get(ctx, configPath+"/templates", nil, &out); err != nil {
		return err
	}
	rows := [][]string{}
	data, _ := out["data"].([]any)
	for _, raw := range data {
		t, _ := raw.(map[string]any)
		update := ""
		if t["update_available"] == true {
			update = "update available"
		}
		rows = append(rows, []string{cellOf(t["provenance"]), cellOf(t["name"]), cellOf(t["taken_version"]), update})
	}
	return cli.Emit(out, Table{Columns: []string{"template", "name", "taken", ""}, Rows: rows})
}

func configPublish(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "publish", "<key> --name <name> [-f <file>]")
	name := flags.String("name", "", "the template's name")
	file := flags.String("f", "", "the blocks it sets up; omitted takes the configuration in force")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 || *name == "" {
		return usagef("hubctl config publish takes one key and --name")
	}
	body := map[string]any{"key": flags.Arg(0), "name": *name}
	if *file != "" {
		document, err := readDocument(cli, *file)
		if err != nil {
			return err
		}
		body["document"] = document
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Post(ctx, configPath+"/templates", body, &out); err != nil {
		return err
	}
	return cli.Emit(out, Table{Columns: []string{"template"}, Rows: [][]string{{cellOf(out["provenance"])}}})
}

func configTake(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "config", "take", "<key> [--version <n>] [--take <path>,...]")
	version := flags.Int("version", 0, "the version; omitted is the newest")
	take := flags.String("take", "", "the upgrade's changes to take, by key path; omitted takes every TAKE")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return usagef("hubctl config take takes one template key")
	}
	body := map[string]any{}
	if *version > 0 {
		body["version"] = *version
	}
	if *take != "" {
		body["take"] = strings.Split(*take, ",")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var out map[string]any
	if err := client.Post(ctx, configPath+"/templates/"+url.PathEscape(flags.Arg(0))+":plan", body, &out); err != nil {
		return err
	}
	table := configPlanTable(out)
	upgrades, _ := out["upgrade"].([]any)
	for _, raw := range upgrades {
		u, _ := raw.(map[string]any)
		table.Rows = append(table.Rows, []string{cellOf(u["state"]), cellOf(u["path"]), ""})
	}
	return cli.Emit(out, table)
}

// textOf prints a decoded JSON value in a table cell.
func cellOf(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(value)
	default:
		raw, _ := json.Marshal(value)
		return string(raw)
	}
}
