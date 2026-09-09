// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const backupSchedulesPath = "/backup-schedules"

// backupSchedule is a noun under a noun, the way `backup target` is, and it dispatches its own
// verb for the same reason: `hubctl backup schedule ls` reads as what it is.
//
// The operator who found this gap is the one who could create a schedule and not list one
// (F4-02). The four verbs here are what that costs to fix.
func backupSchedule(ctx context.Context, cli *CLI, args []string) error {
	if len(args) == 0 {
		return usagef("backup schedule needs a command: ls, set, rm")
	}
	switch args[0] {
	case "ls":
		return backupScheduleList(ctx, cli, args[1:])
	case "set":
		return backupScheduleSet(ctx, cli, args[1:])
	case "rm":
		return backupScheduleRemove(ctx, cli, args[1:])
	default:
		return usagef("backup schedule has no command %q: ls, set, rm", args[0])
	}
}

func backupScheduleList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "backup schedule", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var schedules []openapi.BackupSchedule
	if err := client.Get(ctx, backupSchedulesPath, nil, &schedules); err != nil {
		return err
	}
	return cli.Emit(schedules, scheduleTable(schedules))
}

// backupScheduleSet changes what a schedule does, or switches it off. Only what is named moves:
// the command sends the flags that were actually given, so `--off` leaves the rule alone.
func backupScheduleSet(ctx context.Context, cli *CLI, args []string) error {
	const usage = "backup schedule set <id> [--rrule <rule>] [--timezone <zone>] " +
		"[--mode FULL|INCREMENTAL] [--on|--off]"
	scheduleID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "backup schedule", "set", "<id> [--rrule …] [--on|--off]")
	rrule := flags.String("rrule", "", "the recurrence, RFC 5545 without a DTSTART")
	zone := flags.String("timezone", "", "the IANA zone the rule is read in")
	mode := flags.String("mode", "", "FULL or INCREMENTAL")
	fullRule := flags.String("full-rrule", "", "which of the rule's occurrences are full ones")
	on := flags.Bool("on", false, "switch the schedule on")
	off := flags.Bool("off", false, "switch it off, keeping the rule")
	if err := parseCommand(flags, rest); err != nil {
		return err
	}
	if *on && *off {
		return usagef("backup schedule set takes --on or --off, not both")
	}

	change := openapi.BackupScheduleUpdate{}
	change.Rrule = optional(*rrule)
	change.Timezone = optional(*zone)
	change.FullRrule = optional(*fullRule)
	if *mode != "" {
		wanted := openapi.BackupScheduleUpdateMode(*mode)
		change.Mode = &wanted
	}
	if *on || *off {
		enabled := *on
		change.Enabled = &enabled
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var changed openapi.BackupSchedule
	if err := client.Patch(
		ctx, backupSchedulesPath+"/"+scheduleID.String(), change, &changed,
	); err != nil {
		return err
	}
	return cli.Emit(changed, scheduleTable([]openapi.BackupSchedule{changed}))
}

// backupScheduleRemove takes the recurrence away. What it produced stays: the archives at the
// target, and the runs in the log.
func backupScheduleRemove(ctx context.Context, cli *CLI, args []string) error {
	const usage = "backup schedule rm <id>"
	scheduleID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "backup schedule", "rm", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	return client.Delete(ctx, backupSchedulesPath+"/"+scheduleID.String(), "")
}

// backupTargetRemove takes a place archives go out of the configuration. The server refuses while
// a schedule still names it, and names the schedules - so the CLI passes the refusal on rather
// than checking first and racing with whoever writes a schedule in between.
func backupTargetRemove(ctx context.Context, cli *CLI, args []string) error {
	const usage = "backup target rm <id>"
	targetID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "backup target", "rm", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	return client.Delete(ctx, backupTargetsPath+"/"+targetID.String(), "")
}

// scheduleTable is what an operator reads to answer "what runs, when, and is it on".
func scheduleTable(schedules []openapi.BackupSchedule) Table {
	rows := make([][]string, 0, len(schedules))
	for _, schedule := range schedules {
		rows = append(rows, []string{
			identifier(schedule.Id),
			schedule.TargetId.String(),
			scheduleScope(schedule),
			schedule.Rrule,
			text(schedule.Timezone),
			scheduleMode(schedule),
			scheduleState(schedule),
			nextRun(schedule),
		})
	}
	return Table{
		Columns: []string{"id", "target", "scope", "rrule", "zone", "mode", "state", "next"},
		Rows:    rows,
	}
}

func scheduleScope(schedule openapi.BackupSchedule) string {
	if schedule.Scope.Kind == nil {
		return ""
	}
	if schedule.Scope.Id == nil {
		return string(*schedule.Scope.Kind)
	}
	return string(*schedule.Scope.Kind) + " " + schedule.Scope.Id.String()
}

func scheduleMode(schedule openapi.BackupSchedule) string {
	if schedule.Mode == nil {
		return ""
	}
	return string(*schedule.Mode)
}

// scheduleState is a word rather than a boolean column, because "off" is the state an operator is
// looking for and `enabled false` reads as a field rather than as an answer.
func scheduleState(schedule openapi.BackupSchedule) string {
	if schedule.Enabled != nil && !*schedule.Enabled {
		return "off"
	}
	return "on"
}

// nextRun says "never" for a schedule that owes no moment, which is a rule that is off or one
// whose recurrence is spent - and both are things somebody wants to see rather than a blank.
func nextRun(schedule openapi.BackupSchedule) string {
	if schedule.NextRunAt == nil {
		return "never"
	}
	return schedule.NextRunAt.UTC().Format("2006-01-02 15:04Z")
}

// identifier prints a readOnly identifier the contract leaves optional, and an empty string for
// an answer that carried none rather than the pointer's address.
func identifier(id *openapi_types.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
