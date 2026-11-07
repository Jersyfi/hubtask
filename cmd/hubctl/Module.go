// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The suite's modules at the workspace or one hub (UC-INS-21): which are on, which level decided
// it and which locked it, and the switch itself - the same use case the screen and MCP reach.

const modulesPath = "/tenant/modules"

func moduleGroup() group {
	return group{
		name:    "module",
		summary: "the modules switched on for the workspace or a hub",
		commands: []command{
			{name: "ls", summary: "every module, on or off, and who decided it", run: moduleList},
			{name: "on", summary: "switch a module on", run: moduleSwitch("on", openapi.ModuleSwitchSwitchON)},
			{name: "off", summary: "switch a module off; its data is kept and hidden", run: moduleSwitch("off", openapi.ModuleSwitchSwitchOFF)},
			{name: "inherit", summary: "leave a module to the level above", run: moduleSwitch("inherit", openapi.ModuleSwitchSwitchINHERIT)},
		},
	}
}

func moduleList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "module", "ls", "[--hub <id>]")
	hub := flags.String("hub", "", "the hub to answer for; omitted is the workspace")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	query := url.Values{}
	if *hub != "" {
		parsed, err := cli.parseID("--hub", *hub)
		if err != nil {
			return err
		}
		query.Set("hub_id", parsed.String())
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var list openapi.ModuleList
	if err := client.Get(ctx, modulesPath, query, &list); err != nil {
		return err
	}
	return cli.Emit(list, moduleTable(list))
}

// moduleSwitch is one of the three verbs: on, off and inherit take the module and an optional hub.
func moduleSwitch(verb string, to openapi.ModuleSwitchSwitch) func(context.Context, *CLI, []string) error {
	return func(ctx context.Context, cli *CLI, args []string) error {
		flags := commandFlags(cli, "module", verb, "[--hub <id>] <module>")
		hub := flags.String("hub", "", "the hub to switch it for; omitted is the workspace")
		if err := parseCommand(flags, args); err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return usagef("hubctl module %s takes one module: %s", verb, moduleNames())
		}
		name := openapi.Module(strings.ToUpper(flags.Arg(0)))
		if !name.Valid() {
			return usagef("%q is no module: %s", flags.Arg(0), moduleNames())
		}
		body := openapi.ModuleSwitch{Switch: to}
		if *hub != "" {
			parsed, err := cli.parseID("--hub", *hub)
			if err != nil {
				return err
			}
			body.HubId = &parsed
		}
		client, err := cli.client()
		if err != nil {
			return err
		}
		var list openapi.ModuleList
		if err := client.Put(ctx, modulesPath+"/"+url.PathEscape(string(name)), body, &list); err != nil {
			return err
		}
		return cli.Emit(list, moduleTable(list))
	}
}

func moduleNames() string {
	return "workflows, planning, knowledge, whiteboard, service_desk, assets, time, goals, agents"
}

func moduleTable(list openapi.ModuleList) Table {
	rows := make([][]string, 0, len(list.Modules))
	for _, state := range list.Modules {
		on := "off"
		if state.On {
			on = "on"
		}
		locked := ""
		if state.LockedBy != nil {
			locked = string(*state.LockedBy)
		}
		rows = append(rows, []string{strings.ToLower(string(state.Module)), on, string(state.Source), locked})
	}
	return Table{Columns: []string{"module", "state", "decided by", "locked by"}, Rows: rows}
}
