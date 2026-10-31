// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

// The commands generated from the contract (F-7, tools/doors): one per operation that names
// itself with `x-hubtask-cli`, read from Doors.gen.go. This file is the one interpreter of that
// table - a new command is a line in the contract, never a hand-written function, so the command
// and the operation cannot drift. A hand-written command of the same name wins: it is kept where
// it does more than the operation (a prompt, a table a person reads better), and says so in
// HandWritten_test.go.

// door is one generated command; doorParam one argument or flag of it.
type door struct {
	Group, Verb, OperationID, Method, Path, Summary string
	PathParams, Query, Body                         []doorParam
	HasBody, BodyRequired, IfMatch, StepUp, List    bool
	Columns                                         []string
	Rows                                            string // the page's member of rows, `data` when empty
}

type doorParam struct {
	Name, Kind, Help string
	Required         bool
}

// withDoors adds the generated commands to the hand-written tree.
func withDoors(tree []group) []group {
	index := map[string]int{}
	for i, g := range tree {
		index[g.name] = i
	}
	for _, d := range generatedCommands {
		at, ok := index[d.Group]
		if !ok {
			tree = append(tree, group{name: d.Group, summary: generatedGroups[d.Group]})
			at = len(tree) - 1
			index[d.Group] = at
		}
		if tree[at].run != nil || hasCommand(tree[at], d.Verb) {
			continue
		}
		tree[at].commands = append(tree[at].commands, command{name: d.Verb, summary: d.Summary, run: d.run})
	}
	return tree
}

func hasCommand(g group, verb string) bool {
	for _, c := range g.commands {
		if c.name == verb {
			return true
		}
	}
	return false
}

// flagName is a property's flag: `read_restricted` is `--read-restricted`.
func flagName(property string) string { return strings.ReplaceAll(property, "_", "-") }

func (d door) usage() string {
	parts := make([]string, 0, len(d.PathParams)+2)
	for _, p := range d.PathParams {
		parts = append(parts, "<"+flagName(p.Name)+">")
	}
	for _, p := range append(append([]doorParam{}, d.Query...), d.Body...) {
		if p.Required {
			parts = append(parts, "--"+flagName(p.Name)+" <"+p.Kind+">")
		}
	}
	parts = append(parts, "[flags]")
	return strings.Join(parts, " ")
}

func (d door) run(ctx context.Context, cli *CLI, args []string) error {
	positional := make([]string, 0, len(d.PathParams))
	for _, p := range d.PathParams {
		if len(args) == 0 || strings.HasPrefix(args[0], "-") || strings.TrimSpace(args[0]) == "" {
			if p.Kind == "uuid" {
				return usagef("hubctl %s %s needs <%s>, an identifier: hubctl %s %s %s",
					d.Group, d.Verb, flagName(p.Name), d.Group, d.Verb, d.usage())
			}
			return usagef("usage: hubctl %s %s %s", d.Group, d.Verb, d.usage())
		}
		positional = append(positional, args[0])
		args = args[1:]
	}

	flags := commandFlags(cli, d.Group, d.Verb, d.usage())
	queryValues := map[string]*string{}
	for _, p := range d.Query {
		queryValues[p.Name] = declare(flags, p, p.Help)
	}
	bodyValues := map[string]*string{}
	for _, p := range d.Body {
		if flags.Lookup(flagName(p.Name)) != nil {
			continue
		}
		bodyValues[p.Name] = declare(flags, p, kindHelp(p))
	}
	var bodyFile, version *string
	if d.HasBody && flags.Lookup("body") == nil {
		bodyFile = flags.String("body", "", "the request body as a JSON file, or - for standard input; flags override its members")
	}
	if d.IfMatch && flags.Lookup("expect-version") == nil {
		version = flags.String("expect-version", "", "the version read; refused if it has moved since")
	}
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		// A word the command does not take is refused, never dropped: `people grant <id>` without
		// --account-id would otherwise grant nobody the person meant.
		// `--confirm false` would otherwise read as --confirm and drop the false.
		if _, isBool := strconv.ParseBool(flags.Arg(0)); isBool == nil {
			return usagef("hubctl %s %s takes no argument %q: a switch takes its value as --name=%s",
				d.Group, d.Verb, flags.Arg(0), flags.Arg(0))
		}
		return usagef("hubctl %s %s takes no argument %q: %s", d.Group, d.Verb, flags.Arg(0), d.usage())
	}
	named := given(flags)

	path := d.Path
	for i, p := range d.PathParams {
		if p.Kind == "uuid" {
			if _, err := cli.parseID("<"+flagName(p.Name)+">", positional[i]); err != nil {
				return err
			}
		}
		path = strings.Replace(path, "{"+p.Name+"}", url.PathEscape(positional[i]), 1)
	}

	query := url.Values{}
	for _, p := range d.Query {
		if named[flagName(p.Name)] {
			if p.Kind == "uuid" {
				if _, err := cli.parseID("--"+flagName(p.Name), *queryValues[p.Name]); err != nil {
					return err
				}
			}
			query.Set(p.Name, *queryValues[p.Name])
		} else if p.Required {
			return usagef("hubctl %s %s needs --%s", d.Group, d.Verb, flagName(p.Name))
		}
	}

	var body map[string]any
	if d.HasBody {
		var err error
		source := ""
		if bodyFile != nil {
			source = *bodyFile
		}
		if body, err = readBody(cli.In, source); err != nil {
			return err
		}
		for _, p := range d.Body {
			if !named[flagName(p.Name)] || bodyValues[p.Name] == nil {
				if p.Required && source == "" && bodyValues[p.Name] != nil {
					return usagef("hubctl %s %s needs --%s", d.Group, d.Verb, flagName(p.Name))
				}
				continue
			}
			value, err := parseValue(p, *bodyValues[p.Name])
			if err != nil {
				return usagef("--%s: %v", flagName(p.Name), err)
			}
			body[p.Name] = value
		}
	}

	header := map[string][]string{}
	if version != nil && *version != "" {
		if _, err := strconv.Atoi(*version); err != nil {
			return usagef("--expect-version: %q is not a version", *version)
		}
		header["If-Match"] = []string{`"` + *version + `"`}
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var answer any
	send := func(stepUp string) error {
		for name, values := range stepUpProof(stepUp) {
			header[name] = values
		}
		var payload any
		if d.HasBody && (len(body) > 0 || d.BodyRequired) {
			payload = body
		}
		return client.call(ctx, d.Method, path, query, payload, header, &answer)
	}
	if d.StepUp {
		err = cli.proveAgain(ctx, client, send)
	} else {
		err = send("")
	}
	if err != nil {
		return err
	}
	if dryRun(query, body) {
		printf(cli.Err, "a dry run: nothing changed\n")
	}
	if answer == nil {
		// No answer to show: what was done is said on standard error, so a deletion is not
		// silence and a pipe stays empty.
		printf(cli.Err, "done: %s %s\n", strings.ToLower(d.Summary), strings.Join(positional, " "))
		return nil
	}
	if err := cli.Emit(answer, d.table(answer)); err != nil {
		return err
	}
	d.reportMore(cli, answer)
	return nil
}

// dryRun is whether the call only asked what it would do (P-08), which the person is told.
func dryRun(query url.Values, body map[string]any) bool {
	return query.Get("dry_run") == "true" || body["dry_run"] == true
}

// reportMore says how a paged list goes on, on standard error so a pipe holds the page alone.
func (d door) reportMore(cli *CLI, answer any) {
	object, _ := answer.(map[string]any)
	page, nested := object["page"].(map[string]any)
	if !nested {
		page = object // a flat page, as the suggestions' is, says it beside its rows
	}
	next, _ := page["next_cursor"].(string)
	if cli.JSON || next == "" {
		return
	}
	for _, p := range append(append([]doorParam{}, d.Query...), d.Body...) {
		if p.Name == "cursor" {
			if page["has_more"] == false {
				// A feed read forward ends with the cursor the next read starts from.
				printf(cli.Err, "nothing more for now; continue later with --cursor %s\n", next)
				return
			}
			printf(cli.Err, "more entries follow; continue with --cursor %s\n", next)
			return
		}
	}
}

// declare makes a parameter's flag. A boolean one stands alone as a switch (`--refresh`) and still
// takes `--refresh=false`.
func declare(flags *flag.FlagSet, p doorParam, help string) *string {
	value := new(string)
	if p.Kind == "boolean" {
		flags.Var((*switchFlag)(value), flagName(p.Name), help)
		return value
	}
	flags.StringVar(value, flagName(p.Name), "", help)
	return value
}

// switchFlag is a boolean flag kept as the text it was given, so it travels as the person wrote it.
type switchFlag string

func (s *switchFlag) String() string   { return string(*s) }
func (s *switchFlag) IsBoolFlag() bool { return true }
func (s *switchFlag) Set(value string) error {
	if _, err := strconv.ParseBool(value); err != nil {
		return fmt.Errorf("%q is not true or false", value)
	}
	*s = switchFlag(value)
	return nil
}

func kindHelp(p doorParam) string {
	hint := map[string]string{
		"strings": "comma-separated", "json": "as JSON", "boolean": "true or false",
		"integer": "a whole number", "number": "a number", "uuid": "an identifier",
	}[p.Kind]
	switch {
	case p.Help == "":
		return hint
	case hint == "":
		return p.Help
	default:
		return p.Help + " (" + hint + ")"
	}
}

func readBody(in io.Reader, source string) (map[string]any, error) {
	body := map[string]any{}
	if source == "" {
		return body, nil
	}
	var raw []byte
	var err error
	if source == "-" {
		raw, err = io.ReadAll(in)
	} else {
		raw, err = os.ReadFile(source) //nolint:gosec // G304: the file the person names
	}
	if err != nil {
		return nil, fmt.Errorf("reading --body: %w", err)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, usagef("--body is not a JSON object: %v", err)
	}
	return body, nil
}

func parseValue(p doorParam, raw string) (any, error) {
	switch p.Kind {
	case "integer":
		return strconv.ParseInt(raw, 10, 64)
	case "number":
		return strconv.ParseFloat(raw, 64)
	case "boolean":
		return strconv.ParseBool(raw)
	case "strings":
		values := []string{}
		for _, part := range strings.Split(raw, ",") {
			if part = strings.TrimSpace(part); part != "" {
				values = append(values, part)
			}
		}
		return values, nil
	case "json":
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, fmt.Errorf("not JSON: %w", err)
		}
		return value, nil
	}
	return raw, nil
}

// table lays an answer out for a person: a list by the declared columns, one object as a field
// and its value per line.
func (d door) table(answer any) Table {
	object, _ := answer.(map[string]any)
	member := d.Rows
	if member == "" {
		member = "data"
	}
	rows, ok := object[member].([]any)
	if plain, isArray := answer.([]any); isArray {
		rows, ok = plain, true
	}
	if ok && d.List {
		table := Table{Columns: columnHeads(d.Columns)}
		for _, row := range rows {
			fields, _ := row.(map[string]any)
			line := make([]string, len(d.Columns))
			for i, column := range d.Columns {
				line[i] = cell(fields[column])
			}
			table.Rows = append(table.Rows, line)
		}
		return table
	}
	table := Table{Columns: []string{"FIELD", "VALUE"}}
	names := make([]string, 0, len(object))
	for name := range object {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		table.Rows = append(table.Rows, []string{name, cell(object[name])})
	}
	return table
}

func columnHeads(columns []string) []string {
	out := make([]string, len(columns))
	for i, c := range columns {
		out[i] = strings.ToUpper(c)
	}
	return out
}

func cell(value any) string {
	switch v := value.(type) {
	case nil:
		return "-"
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// verbOf finds a group's command named by one or two words, and what follows it.
func verbOf(g group, args []string) (command, []string, bool) {
	var found command
	width := 0
	for _, c := range g.commands {
		words := strings.Fields(c.name)
		if len(words) > width && len(args) >= len(words) && strings.Join(args[:len(words)], " ") == c.name {
			found, width = c, len(words)
		}
	}
	if width == 0 {
		return command{}, nil, false
	}
	return found, args[width:], true
}
