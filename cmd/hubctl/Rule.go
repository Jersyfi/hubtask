// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The automation surface as a person meets it: write a rule, read it back, switch it on,
// see what it did.
//
// The one thing this group does not do is hide the split between writing a rule and letting it
// loose. A rule is created switched off and `:enable` is its own call with its own audit entry
// (automation.md §1) - so `rule add` prints a rule that is doing nothing, and says so, rather than
// enabling it as a convenience.

const (
	rulesPath = "/automation/rules"
)

func ruleGroup() group {
	return group{
		name:    "rule",
		summary: "automation rules, and what they have done",
		commands: []command{
			{
				name:    "add",
				usage:   "--name <name> --trigger <kind> --action <KIND[:json]> [--scope TENANT|HUB|COLLECTION] [--scope-id <id>] [--run-as <id>] [--event-type <t>] [--rrule <r>] [--timezone <z>]",
				summary: "write a rule - switched off, as every new rule is",
				run:     ruleAdd,
			},
			{
				name:    "test",
				usage:   "<id> --event <type> [--subject <id>] [--payload <json>]",
				summary: "the dry run: what it would do, without doing any of it",
				run:     ruleTest,
			},
			{
				name:    "rotate-token",
				usage:   "<id>",
				summary: "mint the address an INBOUND_WEBHOOK rule listens on - shown once",
				run:     ruleRotateToken,
			},
		},
	}
}

func ruleAdd(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "rule", "add",
		"--name <name> --trigger <kind> --action <KIND[:json]> [--scope <kind>] [--scope-id <id>]")
	name := flags.String("name", "", "what the rule is called")
	trigger := flags.String("trigger", "", "EVENT, SCHEDULE, RELATIVE_DATE, INBOUND_WEBHOOK, MANUAL or JUMBLE_ENTRY")
	eventType := flags.String("event-type", "", "EVENT only: the full type, e.g. de.hubtask.work.item.overdue.v1")
	rrule := flags.String("rrule", "", "SCHEDULE only: an RFC 5545 RRULE")
	timezone := flags.String("timezone", "", "SCHEDULE only: an IANA zone, because a schedule without one means something different in summer")
	scope := flags.String("scope", "TENANT", "TENANT, HUB or COLLECTION")
	scopeID := flags.String("scope-id", "", "the hub or the collection; TENANT names nothing")
	runAs := flags.String("run-as", "", "the account the rule acts as - it can never do more than that account may")
	onError := flags.String("on-error", "", "STOP, CONTINUE or RETRY")
	var actions actionList
	flags.Var(&actions, "action", "a step, as KIND or KIND:{\"json\":\"params\"}; repeat for several")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *name == "" || *trigger == "" || len(actions) == 0 {
		return usagef("rule add needs --name, --trigger and at least one --action")
	}
	// The account the rule acts as is named rather than guessed. This installation has no "who am
	// I" route to ask, and defaulting to the caller would be the client deciding whose rights a
	// standing instruction runs with - which is exactly the composition rule automation.md §2
	// makes the writer's decision.
	if *runAs == "" {
		return usagef("rule add needs --run-as: a rule acts as an account, and which one is not a guess")
	}

	create := openapi.AutomationRuleCreate{
		Name:    *name,
		Actions: actions,
		Trigger: openapi.RuleTrigger{Kind: openapi.RuleTriggerKind(*trigger)},
	}
	runAsID, err := cli.parseID("--run-as", *runAs)
	if err != nil {
		return err
	}
	create.RunAs = runAsID
	create.Scope.Type = openapi.RuleScopeType(*scope)
	if *scopeID != "" {
		parsed, err := cli.parseID("--scope-id", *scopeID)
		if err != nil {
			return err
		}
		create.Scope.Id = &parsed
	}
	if *eventType != "" {
		create.Trigger.EventType = eventType
	}
	if *rrule != "" {
		create.Trigger.Rrule = rrule
	}
	if *timezone != "" {
		create.Trigger.Timezone = timezone
	}
	if *onError != "" {
		kind := openapi.AutomationRuleCreateOnError(*onError)
		create.OnError = &kind
	}

	client, err := cli.client()
	if err != nil {
		return err
	}

	var written openapi.AutomationRule
	if err := client.Post(ctx, rulesPath, create, &written); err != nil {
		return err
	}
	if !cli.JSON {
		printf(cli.Err,
			"the rule is switched off, as every new rule is: read it back, then `hubctl rule enable %s`\n",
			written.Id.String())
	}
	return cli.Emit(written, ruleTable([]openapi.AutomationRule{written}))
}

// ruleTest is the dry run: what the rule would do, with nothing done.
//
// The sample event is named rather than inferred from the rule. A rule's conditions read the event
// (`event.type`, `event.subject`), so a dry run without one would be evaluating them against
// nothing and answering "would run" for a rule that never fires - which is worse than no dry run
// (automation.md §2).
func ruleTest(ctx context.Context, cli *CLI, args []string) error {
	const usage = "rule test <id> --event <type> [--subject <id>] [--payload <json>]"
	ruleID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "rule", "test", "<id> --event <type> [--subject <id>]")
	eventType := flags.String("event", "", "the sample event's type, e.g. de.hubtask.work.item.overdue.v1")
	subject := flags.String("subject", "", "what the sample event is about")
	payload := flags.String("payload", "", "the body an inbound delivery would have carried, as JSON")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *eventType == "" {
		return usagef("rule test needs --event: a rule's conditions read the event, so a dry run without one measures nothing")
	}

	sample := map[string]any{"type": *eventType}
	if *subject != "" {
		parsed, err := cli.parseID("--subject", *subject)
		if err != nil {
			return err
		}
		sample["subject"] = parsed.String()
	}
	body := map[string]any{"rule_id": ruleID.String(), "sample_event": sample}
	if *payload != "" {
		decoded := map[string]any{}
		if err := json.Unmarshal([]byte(*payload), &decoded); err != nil {
			return usagef("--payload is not a JSON object")
		}
		body["payload"] = decoded
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var result openapi.RuleTestResult
	if err := client.Post(ctx, rulesPath+":test", body, &result); err != nil {
		return err
	}
	if !cli.JSON {
		printf(cli.Err, "nothing was done: a dry run answers what would happen\n")
	}
	return cli.Emit(result, testTable(result))
}

// ruleRotateToken mints the address an inbound rule listens on, and says the one thing that has to
// be said about a credential printed to a terminal (project-structure.md §3).
func ruleRotateToken(ctx context.Context, cli *CLI, args []string) error {
	ruleID, err := cli.onlyID(args, "rule rotate-token <id>")
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var minted openapi.InboundTriggerToken
	if err := client.Post(ctx,
		rulesPath+"/"+ruleID.String()+":rotate-inbound-token", nil, &minted); err != nil {
		return err
	}
	if !cli.JSON {
		printf(cli.Err,
			"that token is the whole credential and is shown once: store it, and rotate again if it leaks\n")
	}
	return cli.Emit(minted, Table{
		Columns: []string{"token", "rotated"},
		Rows:    [][]string{{minted.Token, shortTime(&minted.RotatedAt)}},
	})
}

// actionList is `--action` repeated: a kind, optionally with the JSON parameters the kind's use
// case declares. A flag rather than a file, because the shape is small and a rule written on one
// line is a rule somebody can read back in a shell history.
type actionList []openapi.RuleAction

func (l *actionList) String() string { return "" }

func (l *actionList) Set(value string) error {
	kind, params, hasParams := strings.Cut(value, ":")
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return usagef("--action needs a kind, as ADD_LABEL or ADD_LABEL:{\"label_id\":\"…\"}")
	}

	action := openapi.RuleAction{Kind: kind}
	if hasParams && strings.TrimSpace(params) != "" {
		decoded := map[string]any{}
		if err := json.Unmarshal([]byte(params), &decoded); err != nil {
			return usagef("--action %s: the parameters are not a JSON object", kind)
		}
		action.Params = &decoded
	}
	*l = append(*l, action)
	return nil
}

func ruleTable(rules []openapi.AutomationRule) Table {
	rows := make([][]string, 0, len(rules))
	for _, rule := range rules {
		rows = append(rows, []string{
			rule.Id.String(),
			rule.Name,
			string(rule.Trigger.Kind),
			string(rule.Scope.Type),
			switchState(rule.Enabled),
			strconv.Itoa(len(rule.Actions)),
			shortTime(rule.NextRunAt),
		})
	}
	return Table{
		Columns: []string{"id", "name", "trigger", "scope", "state", "actions", "next"},
		Rows:    rows,
	}
}

// switchState says "off" rather than leaving the column blank: a rule that is doing nothing is the
// one thing somebody reading a list of rules must not have to guess at.
func switchState(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func testTable(result openapi.RuleTestResult) Table {
	rows := make([][]string, 0, len(result.Actions)+1)
	rows = append(rows, []string{"conditions", matchedState(result.Matched), ""})
	for _, action := range result.Actions {
		rows = append(rows, []string{action.Path, action.Kind, wouldRunState(action)})
	}
	return Table{Columns: []string{"where", "action", "outcome"}, Rows: rows}
}

// wouldRunState is what the dry run says about one step, including the one thing a dry run can
// fail at: a branch whose condition could not be evaluated for the sample.
func wouldRunState(action openapi.RuleTestAction) string {
	if action.ErrorCode != nil && *action.ErrorCode != "" {
		return *action.ErrorCode
	}
	if action.WouldRun {
		return "would run"
	}
	return "skipped"
}

func matchedState(matched bool) string {
	if matched {
		return "would run"
	}
	return "would not run"
}

// onlyID is takeID for a command that takes an identifier and nothing else. It exists so that a
// stray flag is a named mistake rather than a silently ignored one - the reason takeID reads the
// identifier before the flags in the first place.
func (cli *CLI) onlyID(args []string, usage string) (openapitypes.UUID, error) {
	parsed, rest, err := cli.takeID(args, usage)
	if err != nil {
		return openapitypes.UUID{}, err
	}
	if len(rest) > 0 {
		return openapitypes.UUID{}, usagef("unexpected argument %q: hubctl %s", rest[0], usage)
	}
	return parsed, nil
}
