// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"time"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const jobsPath = "/jobs"

// pollInterval is how often a following command asks again. Two seconds: a backup of a small
// workspace finishes in less than that, and a person watching a long one does not need a finer
// answer than "still going".
const pollInterval = 2 * time.Second

// defaultWait is how long a following command watches before it gives up watching. Half an hour,
// because that is the order of a backup or a restore of a real workspace, and `--timeout` - which
// bounds one call - would be the wrong dial for it.
const defaultWait = 30 * time.Minute

// waitFlag is the bound on watching, declared the same way by every command that can watch.
func waitFlag(flags *flag.FlagSet) *time.Duration {
	return flags.Duration("wait", defaultWait, "how long to wait for the work before giving up watching")
}

func jobGroup() group {
	return group{
		name:    "job",
		summary: "the background work an accepted request handed back",
		commands: []command{
			{
				name:    "show",
				usage:   "<id> [--follow] [--wait <d>]",
				summary: "how a piece of background work is getting on",
				run:     jobShow,
				waits:   true,
			},
			{
				name:    "cancel",
				usage:   "<id>",
				summary: "stop a piece of background work",
				run:     jobCancel,
			},
		},
	}
}

func jobShow(ctx context.Context, cli *CLI, args []string) error {
	const usage = "job show <id> [--follow]"
	jobID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "job", "show", "<id> [--follow]")
	follow := flags.Bool("follow", false, "keep asking until the job is finished")
	wait := waitFlag(flags)
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}

	if *follow {
		job, err := cli.followJob(ctx, client, jobID, *wait)
		if err != nil {
			return err
		}
		return cli.Emit(job, jobTable(job))
	}

	var job openapi.Job
	if err := client.Get(ctx, jobsPath+"/"+jobID.String(), nil, &job); err != nil {
		return err
	}
	return cli.Emit(job, jobTable(job))
}

func jobCancel(ctx context.Context, cli *CLI, args []string) error {
	const usage = "job cancel <id>"
	jobID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "job", "cancel", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var job openapi.Job
	if err := client.Post(ctx, jobsPath+"/"+jobID.String()+":cancel", nil, &job); err != nil {
		return err
	}
	return cli.Emit(job, jobTable(job))
}

// followJob asks until the job is finished, and says so while it waits.
//
// The progress goes to standard error, like every other diagnostic: what a pipe reads is the one
// document the command finally emits, and a script that follows a backup should not have to strip
// percentages out of it.
//
// The wait is bounded by `--wait` rather than by a loop with no end (CLAUDE.md rule 7), and the
// message on running out says how to ask again - a command that hung until somebody pressed Ctrl-C
// would leave a job nobody knows the identifier of.
func (cli *CLI) followJob(
	ctx context.Context, client *Client, jobID openapitypes.UUID, wait time.Duration,
) (openapi.Job, error) {
	// The wait is bounded here rather than left to run for ever (rule 7), and separately from the
	// per-call `--timeout`: one call and one piece of work are two different lengths of time.
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	path := jobsPath + "/" + jobID.String()
	reported := ""

	for {
		var job openapi.Job
		if err := client.Get(ctx, path, nil, &job); err != nil {
			return openapi.Job{}, err
		}

		if line := progressLine(job); line != reported {
			printf(cli.Err, "hubctl: %s\n", line)
			reported = line
		}
		if finished(job.Status) {
			return job, nil
		}

		select {
		case <-ctx.Done():
			return openapi.Job{}, waitedLongEnough(jobID, job, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// finished reports whether a job has reached a state nothing will move it out of.
func finished(status openapi.JobStatus) bool {
	switch status {
	case openapi.JobStatusSUCCEEDED, openapi.JobStatusFAILED, openapi.JobStatusCANCELLED:
		return true
	default:
		return false
	}
}

// progressLine is what a watching command says while it waits.
//
// The error code belongs in it. A job that is being retried carries the code of its last failure
// and goes back to `QUEUED` between attempts (`Job.error_code` in the contract), so a line that
// only printed the status would repeat "QUEUED" for the whole wait while the work failed six times
// - which is exactly what it did against a stack with no encryption keyring.
func progressLine(job openapi.Job) string {
	line := string(job.Status)
	if job.Progress != nil {
		line = fmt.Sprintf("%s %d%%", line, int(*job.Progress*100))
	}
	if job.ErrorCode != nil && *job.ErrorCode != "" {
		line += " (retrying after " + *job.ErrorCode + ")"
	}
	return line
}

// waitedLongEnough turns the deadline into the sentence a person can act on. The job is still
// running - nothing was lost by giving up on watching it - and both halves of what to do next are
// in the one line.
func waitedLongEnough(jobID openapitypes.UUID, job openapi.Job, cause error) error {
	if !errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	// A job that kept failing and kept being retried is not "still running" in any sense a person
	// means, so the last failure is in the sentence: without it, a wait that ran out looks like
	// slowness rather than like something that will never work.
	if job.ErrorCode != nil && *job.ErrorCode != "" {
		return fmt.Errorf(
			"the job is still being retried after --wait, last failure %s; "+
				"ask again with `hubctl job show %s`", *job.ErrorCode, jobID)
	}
	return fmt.Errorf(
		"the job is still running after --wait; ask again with `hubctl job show %s`, "+
			"or watch it for longer with --wait", jobID)
}

// jobFailed is what a command returns when the work it started did not work. The code is the
// catalogue's, so the sentence a person reads is the one the server would have shown them.
func (cli *CLI) jobFailed(job openapi.Job) error {
	if job.Status != openapi.JobStatusFAILED {
		return nil
	}
	code := "job.failed"
	if job.ErrorCode != nil && *job.ErrorCode != "" {
		code = *job.ErrorCode
	}
	if message, ok := cli.Catalogue.Message(code, nil); ok {
		return errorString(message)
	}
	return fmt.Errorf("the job failed: %s", code)
}

func jobTable(job openapi.Job) Table {
	return Table{
		Columns: []string{"job", "status", "progress", "result", "error", "finished"},
		Rows: [][]string{{
			job.JobId.String(),
			string(job.Status),
			percent(job.Progress),
			text(job.ResultUrl),
			text(job.ErrorCode),
			shortTime(job.FinishedAt),
		}},
	}
}

// percent renders progress for a person. A job that cannot say how far along it is says so with a
// dash rather than with a nought, which would read as "no progress at all".
func percent(progress *float32) string {
	if progress == nil {
		return "-"
	}
	return strconv.Itoa(int(*progress*100)) + "%"
}
