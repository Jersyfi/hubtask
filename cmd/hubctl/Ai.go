// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The workspace's AI provider, as an operator configures it (J-02, J-16).
//
// **The key is never printed and never read back.** The API answers no key - it is sealed on the
// way in and opened only by the adapter that makes the call - so there is nothing here to echo even
// by accident. What `show` prints is the decision: which provider, under which models, in which
// jurisdiction, and whether this workspace has consented to sending its content there at all.
//
// The key is taken from the environment rather than from a flag, and that is the one thing about
// this file worth arguing over. A flag lands in the shell's history, in `ps`, and in whatever
// captures a terminal - which is exactly how a provider key leaks (security.md §9). The variable is
// read once and never printed.

const aiProviderPath = "/ai-provider"

// aiKeyVariable is where `ai config set` reads the provider key from.
const aiKeyVariable = "HUBTASK_AI_API_KEY"

func aiGroup() group {
	return group{
		name:    "ai",
		summary: "the workspace's AI provider: which one, which models, and whether it may be used",
		commands: []command{
			{
				name:    "config",
				usage:   "show",
				summary: "which provider this workspace would ask, and whether it may",
				run:     aiConfigShow,
			},
			{
				name: "config-set",
				usage: "--kind NOOP|OPENAI_COMPATIBLE|OLLAMA --jurisdiction SELF_HOSTED|EEA|ADEQUACY|THIRD_COUNTRY" +
					" [--base-url <u>] [--completion-model <m>] [--embedding-model <m>] [--allow-processing]",
				summary: "set it whole; the key comes from " + aiKeyVariable + ", never a flag",
				run:     aiConfigSet,
			},
		},
	}
}

func aiConfigShow(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "ai", "config", "show")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	// `show` is the only word this command takes, and it is optional: `hubctl ai config` reads.
	if flags.NArg() > 1 || (flags.NArg() == 1 && flags.Arg(0) != "show") {
		return usagef("hubctl ai config takes `show` or nothing")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var configured openapi.AiProvider
	if err := client.Get(ctx, aiProviderPath, nil, &configured); err != nil {
		return err
	}
	return cli.Emit(configured, aiProviderTable(configured))
}

func aiConfigSet(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "ai", "config-set",
		"--kind <k> --jurisdiction <j> [--base-url <u>] [--completion-model <m>] [--embedding-model <m>] [--allow-processing]")
	kind := flags.String("kind", "", "NOOP, OPENAI_COMPATIBLE or OLLAMA")
	jurisdiction := flags.String("jurisdiction", "",
		"SELF_HOSTED, EEA, ADEQUACY or THIRD_COUNTRY - where the provider processes what is sent to it")
	baseURL := flags.String("base-url", "", "the endpoint; required for every kind but NOOP")
	completion := flags.String("completion-model", "", "the model that answers in words")
	embedding := flags.String("embedding-model", "", "the model that answers in vectors")
	// Consent is its own flag because it is its own decision (J-02): configuring a provider and
	// agreeing to send this workspace's content to it are two acts, and a default that ran them
	// together would make the second one by accident.
	allow := flags.Bool("allow-processing", false,
		"consent to this workspace's content being sent to the provider")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *kind == "" || *jurisdiction == "" {
		return usagef("hubctl ai config-set needs --kind and --jurisdiction")
	}

	configuration := openapi.AiProviderConfiguration{
		Kind:              openapi.AiProviderKind(*kind),
		Jurisdiction:      openapi.AiJurisdiction(*jurisdiction),
		BaseUrl:           optional(*baseURL),
		CompletionModel:   optional(*completion),
		EmbeddingModel:    optional(*embedding),
		ProcessingAllowed: allow,
	}
	// Absent rather than empty when the variable is unset: the contract reads an omitted key as
	// "keep the one already sealed" and an empty string as "clear it", and a CLI that sent the
	// second for the first would silently unconfigure a working provider.
	if key := cli.Env(aiKeyVariable); key != "" {
		configuration.ApiKey = &key
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var configured openapi.AiProvider
	if err := client.Put(ctx, aiProviderPath, configuration, &configured); err != nil {
		return err
	}
	return cli.Emit(configured, aiProviderTable(configured))
}

// aiProviderTable prints the decision and never a credential. `has_api_key` is the whole of what
// this surface says about the key, and it is the API's own field rather than something inferred
// here.
func aiProviderTable(configured openapi.AiProvider) Table {
	return Table{
		Columns: []string{"kind", "base url", "completion", "embedding", "jurisdiction", "key", "processing"},
		Rows: [][]string{{
			string(configured.Kind),
			text(configured.BaseUrl),
			text(configured.CompletionModel),
			text(configured.EmbeddingModel),
			string(configured.Jurisdiction),
			yesNo(&configured.HasApiKey),
			yesNo(&configured.ProcessingAllowed),
		}},
	}
}
