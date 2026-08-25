// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const customFieldsPath = "/custom-fields"

func fieldGroup() group {
	return group{
		name:    "field",
		summary: "custom fields - defined once, written per entry",
		commands: []command{
			{
				name:    "ls",
				usage:   "[--collection <id>]",
				summary: "list the definitions in force",
				run:     fieldList,
			},
			{
				name: "define",
				usage: "--key <key> --kind TEXT|NUMBER|DATE|SELECT|MULTI_SELECT|BOOL|USER|URL" +
					" [--collection <id>] [--applies-to <types>] [--options <values>] [--required]",
				summary: "define a custom field",
				run:     fieldDefine,
			},
			{
				name:    "set",
				usage:   "<item-id> <key> --value <value> | --clear",
				summary: "write one custom field on an entry",
				run:     fieldSet,
			},
		},
	}
}

func fieldList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "field", "ls", "[--collection <id>]")
	collection := flags.String("collection", "",
		"the collection whose definitions are wanted; unset answers the workspace-wide ones alone")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	query := url.Values{}
	if *collection != "" {
		parsed, err := cli.parseID("--collection", *collection)
		if err != nil {
			return err
		}
		query.Set("collection_id", parsed.String())
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var definitions []openapi.CustomFieldDefinition
	if err := client.Get(ctx, customFieldsPath, query, &definitions); err != nil {
		return err
	}
	return cli.Emit(definitions, fieldTable(definitions))
}

func fieldDefine(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "field", "define",
		"--key <key> --kind <kind> [--collection <id>] [--applies-to <types>] [--options <values>] [--required]")
	key := flags.String("key", "", "the identifier the value is stored under; fixed once defined")
	kind := flags.String("kind", "", "what a value looks like: TEXT, NUMBER, DATE, SELECT, MULTI_SELECT, BOOL, USER or URL")
	collection := flags.String("collection", "", "the collection the definition belongs to; unset defines it workspace-wide")
	appliesTo := flags.String("applies-to", "", "comma-separated item types that carry the field; unset means TASK alone")
	options := flags.String("options", "", "comma-separated values a SELECT or MULTI_SELECT permits")
	required := flags.Bool("required", false, "whether the field has to hold a value")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *key == "" || *kind == "" {
		return usagef("a definition needs --key and --kind")
	}

	fieldKind := openapi.CustomFieldKind(*kind)
	if !fieldKind.Valid() {
		message, _ := cli.Catalogue.Message("fields.kind_unknown", map[string]string{"value": *kind})
		return usageError{error: errorString(message)}
	}

	body := openapi.CustomFieldDefinitionCreate{Key: *key, Kind: fieldKind}
	if *collection != "" {
		parsed, err := cli.parseID("--collection", *collection)
		if err != nil {
			return err
		}
		body.CollectionId = &parsed
	}
	if *appliesTo != "" {
		types := make([]openapi.ItemType, 0, 3)
		for _, raw := range strings.Split(*appliesTo, ",") {
			itemType, err := cli.itemType(strings.TrimSpace(raw))
			if err != nil {
				return err
			}
			types = append(types, itemType)
		}
		body.AppliesTo = &types
	}
	if *options != "" {
		values := strings.Split(*options, ",")
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
		}
		body.Options = &values
	}
	if *required {
		body.IsRequired = required
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.CustomFieldDefinition
	if err := client.Post(ctx, customFieldsPath, body, &created); err != nil {
		return err
	}
	return cli.Emit(created, fieldTable([]openapi.CustomFieldDefinition{created}))
}

func fieldSet(ctx context.Context, cli *CLI, args []string) error {
	const usage = "field set <item-id> <key> --value <value> | --clear"
	item, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	// The key follows the identifier, before the flags and for the same reason the identifier
	// comes first: the flag package stops parsing at it either way.
	if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
		return usagef("the key comes second: hubctl %s", usage)
	}
	key := rest[0]

	flags := commandFlags(cli, "field", "set", "<item-id> <key> --value <value> | --clear")
	value := flags.String("value", "",
		"the value; numbers, booleans and arrays are read as JSON, anything else as text")
	remove := flags.Bool("clear", false, "remove the value stored under the key")
	if err := parseOnlyFlags(flags, rest[1:], usage); err != nil {
		return err
	}
	if *remove == (*value != "") {
		return usagef("say what to write: --value <value>, or --clear to remove it")
	}

	body := openapi.CustomFieldValue{}
	if !*remove {
		body.Value = fieldValue(*value)
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var updated openapi.WorkItem
	target := itemsPath + "/" + item.String() + "/custom-fields/" + url.PathEscape(key)
	if err := client.Put(ctx, target, body, &updated); err != nil {
		return err
	}
	return cli.Emit(updated, itemTable([]openapi.WorkItem{updated}))
}

// fieldValue decides what travels for --value. Anything that reads as JSON goes as JSON - `42`,
// `true`, `["a","b"]` - and anything else goes as text, which is the common case. A TEXT field
// whose value happens to read as a number wants quotes: --value '"42"'. The server validates
// against the definition either way; this only decides the shape of the claim.
func fieldValue(raw string) any {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return raw
	}
	return parsed
}

func fieldTable(definitions []openapi.CustomFieldDefinition) Table {
	rows := make([][]string, 0, len(definitions))
	for _, definition := range definitions {
		types := make([]string, 0, len(definition.AppliesTo))
		for _, itemType := range definition.AppliesTo {
			types = append(types, string(itemType))
		}
		rows = append(rows, []string{
			definition.Id.String(),
			definition.Key,
			string(definition.Kind),
			id(definition.CollectionId),
			strings.Join(types, ","),
			yesNo(&definition.IsRequired),
			strings.Join(definition.Options, ","),
			strconv.Itoa(definition.Version),
		})
	}
	return Table{
		Columns: []string{"id", "key", "kind", "collection", "applies-to", "required", "options", "version"},
		Rows:    rows,
	}
}
