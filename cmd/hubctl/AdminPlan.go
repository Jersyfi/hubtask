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

func adminPlan(ctx context.Context, cli *CLI, args []string) error {
	const verbs = "ls, show, create, set, clear, rm"
	if len(args) == 0 {
		return usagef("admin plan needs a command: %s", verbs)
	}
	switch args[0] {
	case "ls":
		return adminPlanList(ctx, cli, args[1:])
	case "show":
		return adminPlanShow(ctx, cli, args[1:])
	case "create":
		return adminPlanCreate(ctx, cli, args[1:])
	case "set":
		return adminPlanSet(ctx, cli, args[1:], true)
	case "clear":
		return adminPlanSet(ctx, cli, args[1:], false)
	case "rm":
		return adminPlanRemove(ctx, cli, args[1:])
	default:
		return usagef("admin plan has no command %q: %s", args[0], verbs)
	}
}

func adminPlanList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin plan", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var listed struct {
		Data []openapi.Plan `json:"data"`
	}
	if err := client.Get(ctx, adminPlansPath, nil, &listed); err != nil {
		return err
	}
	rows := make([][]string, 0, len(listed.Data))
	for _, held := range listed.Data {
		rows = append(rows, []string{
			held.Name, held.DisplayName, strconv.Itoa(held.Workspaces), strconv.Itoa(held.Version),
			leftOutOf(held.LeftOut),
		})
	}
	return cli.Emit(listed.Data, Table{
		Columns: []string{"name", "display name", "workspaces", "version", "left out"}, Rows: rows,
	})
}

func adminPlanShow(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin plan", "show", "<name>")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return usagef("admin plan show needs the plan's short name")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var held openapi.Plan
	if err := client.Get(ctx, planPath(flags.Arg(0)), nil, &held); err != nil {
		return err
	}
	return cli.Emit(held, planTable(held))
}

func adminPlanCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin plan", "create",
		"--name <short name> --display-name <name> [--leave-out AUTOMATION_RULES,WEBHOOKS,OWN_SIGN_IN_PROVIDERS]")
	name := flags.String("name", "", "the short name a platform maps its products to; it never changes")
	display := flags.String("display-name", "", "what the operator calls the plan")
	leaveOut := flags.String("leave-out", "", "the features the plan leaves out, comma-separated")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *name == "" || *display == "" {
		return usagef("a plan needs --name and --display-name")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.Plan
	if err := client.Post(ctx, adminPlansPath, openapi.PlanCreate{
		Name: *name, DisplayName: *display, LeftOut: featuresFrom(*leaveOut),
	}, &created); err != nil {
		return err
	}
	return cli.Emit(created, planTable(created))
}

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
		Quotas: held.Quotas, Ai: held.Ai, LeftOut: held.LeftOut,
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
		return usagef("%q is not an area of a plan: sign_in, legal, quotas, ai", area)
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
	}
	return nil
}

func adminPlanRemove(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin plan", "rm", "<name>")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return usagef("admin plan rm needs the plan's short name")
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, planPath(flags.Arg(0)), ""); err != nil {
		return err
	}
	printf(cli.Err, "plan %s removed; its name stays taken\n", flags.Arg(0))
	return nil
}

// adminTenantMovePlan puts a workspace on a plan, or on none with --plan "".
func adminTenantMovePlan(ctx context.Context, cli *CLI, args []string) error {
	usage := "admin tenant move-plan <id> --plan <short name> [--expected-version <n>]"
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "move-plan", "<id> --plan <short name>")
	target := flags.String("plan", "", "the plan's short name; empty for none")
	expected := flags.Int("expected-version", 0, "the workspace's version as last read")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	move := openapi.TenantPlanMove{}
	if *target != "" {
		move.Plan = target
	}
	if *expected > 0 {
		move.ExpectedVersion = expected
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var moved openapi.TenantPlanMoved
	if err := client.Post(ctx, adminTenantsPath+"/"+tenantID.String()+":move-plan", move, &moved); err != nil {
		return err
	}
	plan := "no plan"
	if moved.Tenant.Plan != nil {
		plan = *moved.Tenant.Plan
	}
	return cli.Emit(moved, Table{
		Columns: []string{"workspace", "plan", "features resting", "exceptions kept"},
		Rows: [][]string{{
			moved.Tenant.Slug, plan, leftOutOf(&moved.FeaturesResting), strings.Join(moved.ExceptionsKept, ","),
		}},
	})
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
		SignIn: held.SignIn, Legal: held.Legal, Quotas: held.Quotas, Ai: held.Ai,
	})
	table.Rows = append(table.Rows, []string{"left_out", leftOutOf(held.LeftOut), "-"})
	return table
}
