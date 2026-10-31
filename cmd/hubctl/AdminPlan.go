// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Plans (multi-tenancy.md §4.2): the same verbs the API has, so that an operator who drives an
// installation from a terminal can do everything a platform or the dashboard does.
//
// A plan's values are keyed `area.name` exactly as `admin settings` keys the installation's -
// `quotas.items`, `sign_in.min_length` - so a key copied out of one table goes into the other.

const adminPlansPath = "/admin/plans"

// adminPlanSet changes one key of a plan, or clears it, and with --display-name or --leave-out
// changes those. It reads the plan and sends it back whole, with the version it read: a `PUT`
// replaces, and a save made on a plan somebody else changed meanwhile is refused rather than
// silently undoing theirs.
func adminPlanSet(ctx context.Context, cli *CLI, args []string, setting bool) error {
	verb := map[bool]string{true: "set", false: "clear"}[setting]
	flags := commandFlags(cli, "admin plan", verb,
		"<name> [<area>.<key> [value]] [--locked] [--display-name <name>] [--leave-out <features>]")
	locked := flags.Bool("locked", false, "every workspace on the plan is held to the value")
	display := flags.String("display-name", "", "a new name for the plan")
	leaveOut := flags.String("leave-out", "-", "the features the plan leaves out, comma-separated; empty for none")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	rest := flags.Args()
	if len(rest) == 0 {
		return usagef("admin plan %s needs the plan's short name", verb)
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var held openapi.Plan
	if err := client.Get(ctx, planPath(rest[0]), nil, &held); err != nil {
		return err
	}
	change := openapi.PlanChange{
		DisplayName: held.DisplayName, SignIn: held.SignIn, Legal: held.Legal,
		Quotas: held.Quotas, Ai: held.Ai, Module: held.Module, LeftOut: held.LeftOut,
	}
	if *display != "" {
		change.DisplayName = *display
	}
	if *leaveOut != "-" {
		change.LeftOut = featuresFrom(*leaveOut)
	}
	if len(rest) > 1 {
		if err := changePlanKey(&change, rest[1:], setting, *locked); err != nil {
			return err
		}
	}
	var written openapi.Plan
	if err := client.PutVersioned(ctx, planPath(held.Name), change, `"`+strconv.Itoa(held.Version)+`"`, &written); err != nil {
		return err
	}
	return cli.Emit(written, planTable(written))
}

func changePlanKey(change *openapi.PlanChange, rest []string, setting, locked bool) error {
	if setting && len(rest) < 2 {
		return usagef("admin plan set needs <area>.<key> and a value")
	}
	area, name, split := strings.Cut(rest[0], ".")
	if !split || area == "" || name == "" {
		return usagef("%q is not an <area>.<key> key - try `admin plan show`", rest[0])
	}
	target := planAreaOf(change, area)
	if target == nil {
		return usagef("%q is not an area of a plan: sign_in, legal, quotas, ai, module", area)
	}
	if *target == nil {
		*target = &map[string]openapi.InstanceSetting{}
	}
	held, known := (**target)[name]
	if !known {
		// The plan answers every key of every area, decided or not; a key it did not answer is a
		// typo, which the server would skip as a newer client's key and save nothing.
		return usagef("%q is not a key of a plan - try `admin plan show`", rest[0])
	}
	if setting {
		(**target)[name] = openapi.InstanceSetting{
			Set: true, Value: settingValue(held.Kind, rest[1]), Locked: locked,
		}
	} else {
		delete(**target, name)
	}
	return nil
}

func planAreaOf(change *openapi.PlanChange, area string) **map[string]openapi.InstanceSetting {
	switch area {
	case "sign_in":
		return &change.SignIn
	case "legal":
		return &change.Legal
	case "quotas":
		return &change.Quotas
	case "ai":
		return &change.Ai
	case "module":
		return &change.Module
	}
	return nil
}

func planPath(name string) string { return adminPlansPath + "/" + url.PathEscape(name) }

func featuresFrom(raw string) *[]openapi.PlanFeature {
	features := []openapi.PlanFeature{}
	for _, name := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			features = append(features, openapi.PlanFeature(strings.ToUpper(trimmed)))
		}
	}
	return &features
}

func leftOutOf(features *[]openapi.PlanFeature) string {
	if features == nil || len(*features) == 0 {
		return "-"
	}
	names := make([]string, 0, len(*features))
	for _, feature := range *features {
		names = append(names, string(feature))
	}
	return strings.Join(names, ",")
}

func planTable(held openapi.Plan) Table {
	table := settingsTable(openapi.InstanceSettings{
		SignIn: held.SignIn, Legal: held.Legal, Quotas: held.Quotas, Ai: held.Ai, Module: held.Module,
	})
	table.Rows = append(table.Rows, []string{"left_out", leftOutOf(held.LeftOut), "-"})
	return table
}
