// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// JobLease is how long a job counts as running against its agent's concurrent runs: one whose
// worker died is not counted for ever.
const JobLease = 15 * time.Minute

// MaxProposalsPerJob bounds what one job proposes, so one ticket or one inventory does not fill
// the requests with hundreds of acts.
const MaxProposalsPerJob = 20

// TargetType is what a shipped agent's job is about: an entry - a ticket, an incident - or a hub
// whose inventory it reviews.
type TargetType string

const (
	TargetEntry TargetType = "ENTRY"
	TargetHub   TargetType = "HUB"
)

// Target is what a kind of shipped agent works on; an agent built elsewhere runs no job here.
func (k Kind) Target() (TargetType, bool) {
	switch k {
	case KindTicketTriage, KindCISuggestion, KindKnowledgeDraft:
		return TargetEntry, true
	case KindDuplicateReview:
		return TargetHub, true
	}
	return "", false
}

// JobStatus is where a job stands.
type JobStatus string

const (
	JobRunning JobStatus = "RUNNING"
	JobDone    JobStatus = "DONE"
	JobFailed  JobStatus = "FAILED"
)

// Job is one run of a shipped agent on one target (G11): what it was about, what it proposed and
// what it spent - never what it read or what the model answered (rule 10).
type Job struct {
	ID         shared.ID
	AgentID    shared.ID
	TargetType TargetType
	TargetID   shared.ID
	Status     JobStatus
	// Code is how it ended, a message code: what it proposed, why it proposed nothing, or why it
	// failed.
	Code string
	// Proposals counts the acts it left for a person and the suggestions it recorded.
	Proposals int
	Tokens    int64
	StartedBy shared.ID
	StartedAt time.Time
	EndedAt   *time.Time
}

// Running reports a job that counts against its agent's concurrent runs at a moment.
func (j Job) Running(now time.Time) bool {
	return j.Status == JobRunning && now.Sub(j.StartedAt) < JobLease
}

// Ended is the job as it ended.
func (j Job) Ended(status JobStatus, code string, proposals int, tokens int64, at time.Time) Job {
	j.Status, j.Code, j.Proposals, j.Tokens, j.EndedAt = status, code, proposals, tokens, &at
	return j
}

// MayStart judges whether a shipped agent may start a job now: its switches, its approval and its
// model as an act would be judged (Admit), then its tokens for the day and its runs at once (G6).
func (a Agent) MayStart(target TargetType, s Situation) error {
	want, ok := a.Kind.Target()
	if !ok {
		return shared.ErrConflict.WithDetail("agents.runs_elsewhere")
	}
	if target != want {
		return shared.ErrValidation.WithDetail("agents.target_invalid").
			WithParams(map[string]string{"target_type": string(want)})
	}
	if _, err := a.Admit(ClassRead, s); err != nil {
		return err
	}
	return a.MaySpend(s.Usage)
}
