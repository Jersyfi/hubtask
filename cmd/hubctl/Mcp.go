// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// The agent interface, from outside the process (J-11, J-12, J-13, J-16).
//
// **The smallest honest proof that the inbound half works**: a handshake, then the three lists and
// a read of one of each. It is a client rather than an agent - it asks a model nothing and decides
// nothing - and what it is for is that somebody can type these verbs against a running installation
// instead of reading that they exist.
//
// Every command performs the handshake first, because the protocol says so and because the session
// it hands back is what the next request carries. That costs one extra round trip per command and
// buys a CLI with no state on disk, which is the right trade for a tool somebody runs once.

const (
	mcpPath          = "/mcp"
	mcpSessionHeader = "Mcp-Session-Id"
	mcpProtocol      = "2025-06-18"
)

func mcpGroup() group {
	return group{
		name:    "mcp",
		summary: "the agent interface: what an MCP client sees when it connects",
		commands: []command{
			{
				name:    "tools",
				usage:   "",
				summary: "the use cases an agent may call, generated from the catalogue",
				run:     mcpTools,
			},
			{
				name:    "resources",
				usage:   "[--templates]",
				summary: "what an agent may read by URI; --templates for the URI shapes",
				run:     mcpResources,
			},
			{
				name:    "prompts",
				usage:   "",
				summary: "the prepared prompts, with their versions and their arguments",
				run:     mcpPrompts,
			},
			{
				name:    "read",
				usage:   "<uri>",
				summary: "read one resource, e.g. hubtask://containers/<id>",
				run:     mcpRead,
			},
			{
				name:    "prompt",
				usage:   "<name> [--argument name=value]…",
				summary: "render one prompt into the messages a client would send",
				run:     mcpPrompt,
			},
		},
	}
}

// rpcAnswer is a JSON-RPC response, in the two shapes it takes.
type rpcAnswer struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"error"`
}

// mcpSession performs the handshake and answers the session identifier the rest of the call carries.
func mcpSession(ctx context.Context, cli *CLI) (string, error) {
	client, err := cli.client()
	if err != nil {
		return "", err
	}
	status, header, body, err := client.RPC(ctx, "", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": mcpProtocol,
			"clientInfo":      map[string]any{"name": "hubctl", "version": version},
			"capabilities":    map[string]any{},
		},
	})
	if err != nil {
		return "", err
	}
	if _, err := rpcResult(status, body); err != nil {
		return "", err
	}
	return firstHeader(header, mcpSessionHeader), nil
}

// mcpCall does the handshake, then one method, and answers its result.
func mcpCall(ctx context.Context, cli *CLI, method string, params map[string]any) (json.RawMessage, error) {
	session, err := mcpSession(ctx, cli)
	if err != nil {
		return nil, err
	}
	client, err := cli.client()
	if err != nil {
		return nil, err
	}

	request := map[string]any{"jsonrpc": "2.0", "id": 2, "method": method}
	if params != nil {
		request["params"] = params
	}
	status, _, body, err := client.RPC(ctx, session, request)
	if err != nil {
		return nil, err
	}
	return rpcResult(status, body)
}

// rpcResult turns one answer into a result or an error.
//
// A JSON-RPC refusal arrives with a `200` and an error object, so a transport that read the status
// alone would call every refusal a success. The code is kept in the message because it is what an
// agent's own client would act on, and the `data` is the same problem document the REST layer
// would have answered - which is what makes a refusal here as readable as one anywhere else.
func rpcResult(status int, body []byte) (json.RawMessage, error) {
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("the installation serves no MCP endpoint at %s", mcpPath)
	}
	if status >= http.StatusBadRequest {
		return nil, fmt.Errorf("the MCP endpoint answered %d", status)
	}

	var answer rpcAnswer
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, fmt.Errorf("the MCP endpoint answered with something that is not JSON-RPC: %w", err)
	}
	if answer.Error != nil {
		if len(answer.Error.Data) > 0 {
			return nil, fmt.Errorf("%s (%d): %s",
				answer.Error.Message, answer.Error.Code, string(answer.Error.Data))
		}
		return nil, fmt.Errorf("%s (%d)", answer.Error.Message, answer.Error.Code)
	}
	return answer.Result, nil
}

func firstHeader(header map[string][]string, name string) string {
	for key, values := range header {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func mcpTools(ctx context.Context, cli *CLI, args []string) error {
	if err := parseCommand(commandFlags(cli, "mcp", "tools", ""), args); err != nil {
		return err
	}
	result, err := mcpCall(ctx, cli, "tools/list", nil)
	if err != nil {
		return err
	}

	var listed struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Annotations struct {
				ReadOnlyHint    bool `json:"readOnlyHint"`
				DestructiveHint bool `json:"destructiveHint"`
			} `json:"annotations"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &listed); err != nil {
		return err
	}

	rows := make([][]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		rows = append(rows, []string{
			tool.Name,
			yesNo(&tool.Annotations.ReadOnlyHint),
			yesNo(&tool.Annotations.DestructiveHint),
			firstSentence(tool.Description),
		})
	}
	return cli.Emit(json.RawMessage(result), Table{
		Columns: []string{"tool", "read only", "destructive", "what it does"},
		Rows:    rows,
	})
}

func mcpResources(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "mcp", "resources", "[--templates]")
	templates := flags.Bool("templates", false, "the URI shapes rather than what exists")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	if *templates {
		result, err := mcpCall(ctx, cli, "resources/templates/list", nil)
		if err != nil {
			return err
		}
		var listed struct {
			ResourceTemplates []struct {
				URITemplate string `json:"uriTemplate"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"resourceTemplates"`
		}
		if err := json.Unmarshal(result, &listed); err != nil {
			return err
		}
		rows := make([][]string, 0, len(listed.ResourceTemplates))
		for _, template := range listed.ResourceTemplates {
			rows = append(rows, []string{
				template.URITemplate, template.Title, firstSentence(template.Description),
			})
		}
		return cli.Emit(json.RawMessage(result), Table{
			Columns: []string{"uri template", "kind", "what it is"},
			Rows:    rows,
		})
	}

	result, err := mcpCall(ctx, cli, "resources/list", nil)
	if err != nil {
		return err
	}
	var listed struct {
		Resources []struct {
			URI   string `json:"uri"`
			Name  string `json:"name"`
			Title string `json:"title"`
		} `json:"resources"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(result, &listed); err != nil {
		return err
	}
	rows := make([][]string, 0, len(listed.Resources))
	for _, resource := range listed.Resources {
		rows = append(rows, []string{resource.URI, resource.Title, resource.Name})
	}
	if err := cli.Emit(json.RawMessage(result), Table{
		Columns: []string{"uri", "kind", "name"},
		Rows:    rows,
	}); err != nil {
		return err
	}
	if !cli.JSON && listed.NextCursor != "" {
		printf(cli.Err, "more resources follow; this listing shows the first page\n")
	}
	return nil
}

func mcpPrompts(ctx context.Context, cli *CLI, args []string) error {
	if err := parseCommand(commandFlags(cli, "mcp", "prompts", ""), args); err != nil {
		return err
	}
	result, err := mcpCall(ctx, cli, "prompts/list", nil)
	if err != nil {
		return err
	}

	var listed struct {
		Prompts []struct {
			Name      string `json:"name"`
			Title     string `json:"title"`
			Arguments []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"arguments"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(result, &listed); err != nil {
		return err
	}

	rows := make([][]string, 0, len(listed.Prompts))
	for _, prompt := range listed.Prompts {
		arguments := make([]string, 0, len(prompt.Arguments))
		for _, argument := range prompt.Arguments {
			name := argument.Name
			if !argument.Required {
				name = "[" + name + "]"
			}
			arguments = append(arguments, name)
		}
		rows = append(rows, []string{prompt.Name, prompt.Title, strings.Join(arguments, " ")})
	}
	return cli.Emit(json.RawMessage(result), Table{
		Columns: []string{"prompt", "title", "arguments"},
		Rows:    rows,
	})
}

func mcpRead(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "mcp", "read", "<uri>")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return usagef("hubctl mcp read <uri>")
	}

	result, err := mcpCall(ctx, cli, "resources/read",
		map[string]any{"uri": flags.Arg(0)})
	if err != nil {
		return err
	}

	var read struct {
		Contents []struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(result, &read); err != nil {
		return err
	}
	if cli.JSON {
		return cli.Emit(json.RawMessage(result), Table{})
	}
	// The content of a resource is a use case's own answer, which is JSON: printed whole rather
	// than reshaped, because what this command is for is seeing what an agent would see.
	for _, content := range read.Contents {
		printf(cli.Out, "%s\n", content.Text)
	}
	return nil
}

func mcpPrompt(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "mcp", "prompt", "<name> [--argument name=value]…")
	var arguments stringList
	flags.Var(&arguments, "argument", "an argument the prompt declares, as name=value; repeat for several")
	// The name comes before the flags, as an identifier does everywhere else in this CLI: the flag
	// package stops at the first argument that is not a flag.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return usagef("hubctl mcp prompt <name> [--argument name=value]…")
	}
	name := args[0]
	if err := parseCommand(flags, args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return usagef("the name comes before the flags: hubctl mcp prompt %s", name)
	}

	params := map[string]any{"name": name}
	if len(arguments) > 0 {
		given := make(map[string]string, len(arguments))
		for _, argument := range arguments {
			name, value, found := strings.Cut(argument, "=")
			if !found || name == "" {
				return usagef("--argument takes name=value, not %q", argument)
			}
			given[name] = value
		}
		params["arguments"] = given
	}

	result, err := mcpCall(ctx, cli, "prompts/get", params)
	if err != nil {
		return err
	}

	var rendered struct {
		Description string `json:"description"`
		Messages    []struct {
			Role    string `json:"role"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
				URI  string `json:"uri"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(result, &rendered); err != nil {
		return err
	}

	rows := make([][]string, 0, len(rendered.Messages))
	for _, message := range rendered.Messages {
		what := message.Content.Text
		if message.Content.Type != "text" {
			what = message.Content.URI
		}
		rows = append(rows, []string{message.Role, message.Content.Type, firstSentence(what)})
	}
	return cli.Emit(json.RawMessage(result), Table{
		Columns: []string{"role", "type", "content"},
		Rows:    rows,
	})
}

// firstSentence keeps a table a table. A tool's description carries its preconditions and side
// effects, which is what an agent needs and not what somebody scanning a list does - `--json` is
// where the whole of it is.
func firstSentence(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\n", " "), "\r", " ")
	if cut := strings.Index(text, ". "); cut > 0 {
		return text[:cut+1]
	}
	if len(text) > 90 {
		return text[:87] + "…"
	}
	return text
}
