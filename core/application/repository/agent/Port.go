// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package agent is the storage port of the governed agents module (M9): models, agents, the
// workspace's kill switch, the ledger of acts, the acts held for a person, the evaluation sets
// and runs, and the provenance of accepted AI values. Every method runs in the caller's unit of
// work, scoped to its tenant (rule 3).
package agent

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Halt is the workspace's kill switch as it stands; a workspace that never set it is not halted.
type Halt struct {
	Halted bool
	SetBy  shared.ID
	SetAt  time.Time
}

// Metering is one agent's day in the ledger.
type Metering struct {
	AgentID shared.ID
	Day     time.Time
	Allowed int
	Held    int
	Refused int
	Tokens  int64
}

// Store is the port. A row that does not exist answers shared.ErrNotFound; a write held to a
// version that moved answers shared.ErrVersionConflict.
type Store interface {
	ListModels(ctx context.Context, max int) ([]domain.Model, error)
	FindModel(ctx context.Context, id shared.ID) (domain.Model, error)
	InsertModel(ctx context.Context, m domain.Model, at time.Time) error
	UpdateModel(ctx context.Context, m domain.Model, expected int, at time.Time) error

	ListAgents(ctx context.Context, owner, model shared.ID, max int) ([]domain.Agent, error)
	FindAgent(ctx context.Context, id shared.ID) (domain.Agent, error)
	// FindAgentByAccount answers the agent acting as an account, ErrNotFound for any other.
	FindAgentByAccount(ctx context.Context, account shared.ID) (domain.Agent, error)
	InsertAgent(ctx context.Context, a domain.Agent) error
	UpdateAgent(ctx context.Context, a domain.Agent, expected int, at time.Time) error

	Halt(ctx context.Context) (Halt, error)
	PutHalt(ctx context.Context, id shared.ID, h Halt) error

	RecordAction(ctx context.Context, a domain.Action) error
	// Usage counts an agent's acts that were neither reads nor refused since hourFrom, and the
	// tokens its runs spent since dayFrom.
	Usage(ctx context.Context, agent shared.ID, hourFrom, dayFrom time.Time) (domain.Usage, error)
	ListActions(ctx context.Context, agent shared.ID, before *time.Time, max int) ([]domain.Action, error)
	Metering(ctx context.Context, agent shared.ID, from time.Time, max int) ([]Metering, error)

	InsertRequest(ctx context.Context, r domain.Request) error
	FindRequest(ctx context.Context, id shared.ID) (domain.Request, error)
	ListRequests(ctx context.Context, status domain.RequestStatus, agent shared.ID, max int) ([]domain.Request, error)
	// DecideRequest writes a pending request's answer; false where it was no longer pending.
	DecideRequest(ctx context.Context, r domain.Request, expected int) (bool, error)
	// FailRequest records that an approved request could not be performed, with the refusal's code.
	FailRequest(ctx context.Context, id shared.ID, code string) error

	ListCases(ctx context.Context, agent shared.ID, max int) ([]domain.Case, error)
	InsertCase(ctx context.Context, c domain.Case) error
	DeleteCase(ctx context.Context, agent, id shared.ID) (bool, error)
	InsertRun(ctx context.Context, r domain.Run) error
	ListRuns(ctx context.Context, agent shared.ID, max int) ([]domain.Run, error)

	InsertJob(ctx context.Context, j domain.Job) error
	// EndJob writes a running job's end; false where it had ended already.
	EndJob(ctx context.Context, j domain.Job) (bool, error)
	FindJob(ctx context.Context, id shared.ID) (domain.Job, error)
	ListJobs(ctx context.Context, agent shared.ID, max int) ([]domain.Job, error)
	// RunningJobs counts an agent's jobs running that started since a moment.
	RunningJobs(ctx context.Context, agent shared.ID, since time.Time) (int, error)

	InsertMark(ctx context.Context, m domain.Mark) error
	ListMarks(ctx context.Context, targetType string, target shared.ID, max int) ([]domain.Mark, error)
}
