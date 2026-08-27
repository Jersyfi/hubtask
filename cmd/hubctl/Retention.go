// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const retentionPath = "/retention-policies"

func retentionGroup() group {
	return group{
		name:    "retention",
		summary: "how long things are kept, and what happens when the period runs out",
		commands: []command{
			{
				name:    "ls",
				usage:   "[--container <id>] [--effective]",
				summary: "the rules, or the ones actually in force somewhere",
				run:     retentionList,
			},
			{
				name:    "add",
				usage:   "--kind <data kind> --days <n> --action ARCHIVE|TRASH|ANONYMIZE|HARD_DELETE|EXPORT_THEN_DELETE|NOTIFY_ONLY",
				summary: "write a rule",
				run:     retentionAdd,
			},
			{
				name:    "preview",
				usage:   "<policy id>",
				summary: "what the rule would do, without doing it",
				run:     retentionPreview,
			},
			{
				name:    "retain",
				usage:   "<item id>",
				summary: "take one entry out of the running period",
				run:     retentionRetain,
			},
		},
	}
}

func retentionList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "retention", "ls", "[--container <id>] [--effective]")
	container := flags.String("container", "", "the hub or collection to ask about")
	effective := flags.Bool("effective", false, "only the rules actually in force, inheritance included")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	query := url.Values{}
	if *container != "" {
		containerID, err := cli.parseID("--container", *container)
		if err != nil {
			return err
		}
		query.Set("container_id", containerID.String())
	}
	if *effective {
		query.Set("effective", "true")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var policies []openapi.RetentionPolicy
	if err := client.Get(ctx, retentionPath, query, &policies); err != nil {
		return err
	}
	return cli.Emit(policies, policyTable(policies))
}

func retentionAdd(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "retention", "add",
		"--kind <data kind> --days <n> --action <action> [--scope TENANT|HUB|COLLECTION --scope-id <id>]")
	kind := flags.String("kind", "", "the data kind, as data-retention.md lists them, e.g. COMPLETED_ITEM")
	days := flags.Int("days", -1, "how long it is kept before the action")
	action := flags.String("action", "", "ARCHIVE, TRASH, ANONYMIZE, HARD_DELETE, EXPORT_THEN_DELETE or NOTIFY_ONLY")
	scope := flags.String("scope", "TENANT", "TENANT, HUB or COLLECTION")
	scopeID := flags.String("scope-id", "", "the hub or collection, where the scope is not the workspace")
	condition := flags.String("condition", "", "a CEL expression narrowing what the rule applies to")
	thenAfter := flags.Int("then-after", 0, "days after the first action before the second")
	thenAction := flags.String("then-action", "", "what happens after that")
	justification := flags.String("justification", "",
		"why a period past the data kind's upper bound is right; the server requires it there")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *kind == "" || *action == "" || *days < 0 {
		return usagef("retention add needs --kind, --days and --action")
	}

	policy := openapi.RetentionPolicy{
		DataKind:   *kind,
		RetainDays: *days,
		Action:     openapi.RetentionPolicyAction(*action),
	}
	scopeKind := openapi.RetentionPolicyScopeKind(*scope)
	policy.Scope.Kind = &scopeKind
	if *scopeID != "" {
		parsed, err := cli.parseID("--scope-id", *scopeID)
		if err != nil {
			return err
		}
		policy.Scope.Id = &parsed
	}
	policy.Condition = optional(*condition)
	policy.Justification = optional(*justification)
	if *thenAfter > 0 {
		policy.ThenAfterDays = thenAfter
	}
	if *thenAction != "" {
		next := openapi.RetentionPolicyThenAction(*thenAction)
		policy.ThenAction = &next
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.RetentionPolicy
	if err := client.Post(ctx, retentionPath, policy, &created); err != nil {
		return err
	}
	return cli.Emit(created, policyTable([]openapi.RetentionPolicy{created}))
}

// preview is the answer to `:preview`. Written out here for the same reason the REST layer writes
// its own: the response is an inline schema in `openapi.yaml` and the generator makes no type for
// one, and a preview read through a `map[string]any` would print whatever arrived rather than what
// the contract says arrives.
type preview struct {
	Matched      int            `json:"matched"`
	Blocked      map[string]int `json:"blocked"`
	ShareOfScope float64        `json:"share_of_scope"`
	Samples      []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		EffectiveAt string `json:"effective_at"`
	} `json:"samples"`
}

// retentionPreview is what makes a rule safe to write: how much it would take, what is stopping
// it, and a handful of examples. `data-retention.md` §5 asks for it before a rule is switched on,
// and a CLI that could create rules but not preview them would be the half that does damage.
func retentionPreview(ctx context.Context, cli *CLI, args []string) error {
	const usage = "retention preview <policy id>"
	policyID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "retention", "preview", "<policy id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var answer preview
	if err := client.Post(ctx, retentionPath+"/"+policyID.String()+":preview", nil, &answer); err != nil {
		return err
	}
	if cli.JSON {
		return cli.Emit(answer, Table{})
	}

	cli.emitTable(Table{
		Columns: []string{"matched", "share of scope", "blocked"},
		Rows: [][]string{{
			strconv.Itoa(answer.Matched),
			strconv.FormatFloat(answer.ShareOfScope*100, 'f', 1, 64) + "%",
			blockedBy(answer.Blocked),
		}},
	})
	if len(answer.Samples) == 0 {
		return nil
	}

	printf(cli.Out, "\n")
	rows := make([][]string, 0, len(answer.Samples))
	for _, sample := range answer.Samples {
		rows = append(rows, []string{sample.ID, sample.Title, sample.EffectiveAt})
	}
	cli.emitTable(Table{Columns: []string{"id", "title", "would happen"}, Rows: rows})
	return nil
}

// blockedBy renders the reasons in a stable order and says "nothing" rather than printing an empty
// column: "nothing is stopping this rule" is the sentence somebody about to switch it on needs.
func blockedBy(blocked map[string]int) string {
	if len(blocked) == 0 {
		return "nothing"
	}
	reasons := make([]string, 0, len(blocked))
	for reason := range blocked {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)

	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, reason+"="+strconv.Itoa(blocked[reason]))
	}
	return strings.Join(parts, " ")
}

func retentionRetain(ctx context.Context, cli *CLI, args []string) error {
	const usage = "retention retain <item id>"
	itemID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "retention", "retain", "<item id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var item openapi.WorkItem
	if err := client.Post(ctx, itemsPath+"/"+itemID.String()+":retain", nil, &item); err != nil {
		return err
	}
	return cli.Emit(item, itemTable([]openapi.WorkItem{item}))
}

func policyTable(policies []openapi.RetentionPolicy) Table {
	rows := make([][]string, 0, len(policies))
	for _, policy := range policies {
		rows = append(rows, []string{
			id(policy.Id),
			policy.DataKind,
			scopeOf(policy),
			strconv.Itoa(policy.RetainDays),
			string(policy.Action),
			then(policy),
			yesNo(policy.Enabled),
			yesNo(policy.InForce),
		})
	}
	return Table{
		Columns: []string{"id", "data kind", "scope", "days", "action", "then", "enabled", "in force"},
		Rows:    rows,
	}
}

func scopeOf(policy openapi.RetentionPolicy) string {
	kind := "TENANT"
	if policy.Scope.Kind != nil {
		kind = string(*policy.Scope.Kind)
	}
	if policy.Scope.Id == nil {
		return kind
	}
	return kind + " " + policy.Scope.Id.String()
}

// then renders the second step of a chain, which is what makes a rule more than a deletion: "trash
// after 30 days, then hard delete 30 days later" is one rule, and reading it as two columns would
// lose that.
func then(policy openapi.RetentionPolicy) string {
	if policy.ThenAction == nil {
		return "-"
	}
	after := 0
	if policy.ThenAfterDays != nil {
		after = *policy.ThenAfterDays
	}
	return string(*policy.ThenAction) + " +" + strconv.Itoa(after) + "d"
}
