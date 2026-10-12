// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The AI models an installation offers (ADR-0072 §1), and a workspace's choice among the sources
// it may use. The key of an offered model comes from HUBTASK_AI_API_KEY, as a workspace's own does,
// and is never printed: the API answers none.

const (
	adminAiProvidersPath = "/admin/ai-providers"
	aiSourcePath         = "/ai-source"
)

// adminAi dispatches `hubctl admin ai ls|add|set|rm`.
func adminAi(ctx context.Context, cli *CLI, args []string) error {
	const verbs = "ls, add, set, rm"
	if len(args) == 0 {
		return usagef("admin ai needs a command: %s", verbs)
	}
	switch args[0] {
	case "ls":
		return adminAiList(ctx, cli, args[1:])
	case "add":
		return adminAiWrite(ctx, cli, args[1:], false)
	case "set":
		return adminAiWrite(ctx, cli, args[1:], true)
	case "rm":
		return adminAiRemove(ctx, cli, args[1:])
	default:
		return usagef("admin ai has no command %q: %s", args[0], verbs)
	}
}

func adminAiList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin ai", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var offered []openapi.OfferedAiModel
	if err := client.Get(ctx, adminAiProvidersPath, nil, &offered); err != nil {
		return err
	}
	rows := make([][]string, 0, len(offered))
	for _, model := range offered {
		rows = append(rows, []string{
			model.Id.String(), model.DisplayName, model.Processor, string(model.Jurisdiction),
			string(model.Kind), text(model.CompletionModel), text(model.EmbeddingModel), yesNo(&model.HasApiKey),
		})
	}
	return cli.Emit(offered, Table{
		Columns: []string{"id", "name", "processor", "jurisdiction", "kind", "completion", "embedding", "key"},
		Rows:    rows,
	})
}

// adminAiWrite offers a model (`add`) or changes one (`set <id>`), set whole. Without
// HUBTASK_AI_API_KEY a change keeps the sealed key.
func adminAiWrite(ctx context.Context, cli *CLI, args []string, changing bool) error {
	verb, usage := "add", "--name <n> --processor <p> --kind <k> --base-url <u> --jurisdiction <j>"
	if changing {
		verb, usage = "set", "<id> "+usage
	}
	flags := commandFlags(cli, "admin ai", verb, usage+" [--completion-model <m>] [--embedding-model <m>]")
	name := flags.String("name", "", "the name every workspace chooses it by")
	processor := flags.String("processor", "", "who processes what is sent to it")
	kind := flags.String("kind", "", "OPENAI_COMPATIBLE or OLLAMA")
	baseURL := flags.String("base-url", "", "the endpoint")
	completion := flags.String("completion-model", "", "the model that answers in words")
	embedding := flags.String("embedding-model", "", "the model that answers in vectors")
	jurisdiction := flags.String("jurisdiction", "", "SELF_HOSTED, EEA, ADEQUACY or THIRD_COUNTRY")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	rest := flags.Args()
	if changing && len(rest) == 0 {
		return usagef("admin ai set needs a model - `admin ai ls` prints them")
	}
	if *name == "" || *processor == "" || *kind == "" || *baseURL == "" || *jurisdiction == "" {
		return usagef("admin ai %s needs --name, --processor, --kind, --base-url and --jurisdiction", verb)
	}

	body := openapi.OfferedAiModelConfiguration{
		DisplayName: *name, Processor: *processor, Kind: openapi.AiProviderKind(*kind),
		BaseUrl: *baseURL, Jurisdiction: openapi.AiJurisdiction(*jurisdiction),
		CompletionModel: optional(*completion), EmbeddingModel: optional(*embedding),
	}
	if key := cli.Env(aiKeyVariable); key != "" {
		body.ApiKey = &key
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var written openapi.OfferedAiModel
	if changing {
		err = client.Put(ctx, adminAiProvidersPath+"/"+rest[0], body, &written)
	} else {
		err = client.Post(ctx, adminAiProvidersPath, body, &written)
	}
	if err != nil {
		return err
	}
	return adminAiList(ctx, cli, nil)
}

func adminAiRemove(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin ai", "rm", "<id>")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	rest := flags.Args()
	if len(rest) == 0 {
		return usagef("admin ai rm needs a model - `admin ai ls` prints them")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, adminAiProvidersPath+"/"+rest[0], ""); err != nil {
		return err
	}
	return adminAiList(ctx, cli, nil)
}

// aiSource is `hubctl ai source [show]` and `hubctl ai source set`.
func aiSource(ctx context.Context, cli *CLI, args []string) error {
	if len(args) > 0 && args[0] == "set" {
		return aiSourceSet(ctx, cli, args[1:])
	}
	flags := commandFlags(cli, "ai", "source", "[show]")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() > 1 || (flags.NArg() == 1 && flags.Arg(0) != "show") {
		return usagef("hubctl ai source takes `show`, `set` or nothing")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var source openapi.AiSource
	if err := client.Get(ctx, aiSourcePath, nil, &source); err != nil {
		return err
	}
	return cli.Emit(source, aiSourceTable(source))
}

// aiSourceSet chooses the source. `--model` names an offered model by its name or its identifier;
// choosing one is this workspace's consent to send its content there.
func aiSourceSet(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "ai source", "set", "--source OWN|OFFERED|NONE [--model <name or id>] [--sources NONE|OFFERED|OWN|EITHER|INHERIT]")
	source := flags.String("source", "", "OWN, OFFERED or NONE")
	model := flags.String("model", "", "the offered model, by name or id - with OFFERED only")
	sources := flags.String("sources", "", "narrow what this workspace may use; INHERIT takes what the installation allows")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *source == "" {
		return usagef("hubctl ai source set needs --source")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	body := openapi.AiSourceChoice{Source: openapi.AiSourceKind(*source)}
	if *sources != "" {
		narrowing := openapi.AiSourceChoiceSources(*sources)
		body.Sources = &narrowing
	}
	if *model != "" {
		var current openapi.AiSource
		if err := client.Get(ctx, aiSourcePath, nil, &current); err != nil {
			return err
		}
		found := false
		for _, offered := range current.Offered {
			if offered.Id.String() == *model || strings.EqualFold(offered.DisplayName, *model) {
				id := offered.Id
				body.OfferedModelId = &id
				found = true
			}
		}
		if !found {
			return usagef("%q is not a model this installation offers here - `hubctl ai source` lists them", *model)
		}
	}
	var chosen openapi.AiSource
	if err := client.Put(ctx, aiSourcePath, body, &chosen); err != nil {
		return err
	}
	return cli.Emit(chosen, aiSourceTable(chosen))
}

// aiSourceTable prints what the workspace may use, what it chose, and every offered model by
// name, processor and jurisdiction.
func aiSourceTable(source openapi.AiSource) Table {
	chosen := string(source.Source)
	for _, offered := range source.Offered {
		if source.OfferedModelId != nil && offered.Id == *source.OfferedModelId {
			chosen += ": " + offered.DisplayName
		}
	}
	lock := "open"
	if source.SourcesLock != nil {
		lock = "LOCKED (" + string(*source.SourcesLock) + ")"
	}
	rows := [][]string{
		{"source", chosen},
		{"may use", string(source.Sources) + ", " + lock},
		{"own model at least", string(source.MinJurisdiction)},
	}
	for _, offered := range source.Offered {
		rows = append(rows, []string{"offered", offered.DisplayName + " - " + offered.Processor + ", " + string(offered.Jurisdiction)})
	}
	return Table{Columns: []string{"", ""}, Rows: rows}
}
