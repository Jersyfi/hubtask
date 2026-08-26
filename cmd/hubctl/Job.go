// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"errors"
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

func jobGroup() group {
	return group{
		name:    "job",
		summary: "the background work an accepted request handed back",
		commands: []command{
			{
				name:    "show",
				usage:   "<id> [--follow]",
				summary: "how a piece of background work is getting on",
				run:     jobShow,
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
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}

	if *follow {
		job, err := cli.followJob(ctx, client, jobID)
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
// The wait is bounded by the command's own `--timeout`, deliberately rather than by a loop with no
// end (CLAUDE.md rule 7). A backup of a real workspace outlives the default thirty seconds, so the
// message on running out says what to pass and how to ask again - a command that hung until
// somebody pressed Ctrl-C would leave a job nobody knows the identifier of.
func (cli *CLI) followJob(
	ctx context.Context, client *Client, jobID openapitypes.UUID,
) (openapi.Job, error) {
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
			return openapi.Job{}, waitedLongEnough(jobID, ctx.Err())
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

func progressLine(job openapi.Job) string {
	if job.Progress == nil {
		return string(job.Status)
	}
	return fmt.Sprintf("%s %d%%", job.Status, int(*job.Progress*100))
}

// waitedLongEnough turns the deadline into the sentence a person can act on. The job is still
// running - nothing was lost by giving up on watching it - and both halves of what to do next are
// in the one line.
func waitedLongEnough(jobID openapitypes.UUID, cause error) error {
	if !errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	return fmt.Errorf(
		"the job is still running after --timeout; ask again with `hubctl job show %s`, "+
			"or wait for it with a longer --timeout", jobID)
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
