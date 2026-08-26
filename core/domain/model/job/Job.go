// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package job is background work as the caller who asked for it sees it.
//
// It is deliberately not the queue. `core/port/queue` is how work is claimed, leased, retried and
// dead-lettered - the runner's vocabulary, and none of a caller's business. This package is the
// other side of the same row: the four or five facts a client polls after a `202 Accepted`, and
// the one translation between the two vocabularies (ADR-0008, api-guidelines.md §5).
package job

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// State is what a caller sees of a job's life.
//
// Five values where the queue keeps six. `DEAD_LETTER` is the queue's word for "the attempt
// budget ran out and the row is waiting for an operator"; to the caller that is `FAILED`, because
// "it did not work and will not be tried again" is the whole of what they can act on, and a sixth
// state would ask every client to learn a retry policy that is not part of the contract.
type State string

const (
	// StateQueued is accepted and waiting. The queue spells it PENDING.
	StateQueued State = "QUEUED"
	// StateRunning is claimed by a worker and under way.
	StateRunning State = "RUNNING"
	// StateSucceeded is done.
	StateSucceeded State = "SUCCEEDED"
	// StateFailed is done and did not work - including the dead letter.
	StateFailed State = "FAILED"
	// StateCancelled is stopped on request.
	StateCancelled State = "CANCELLED"
)

func (s State) String() string { return string(s) }

// IsTerminal reports whether the job is over. A terminal job is not cancellable and its state
// never changes again, which is what makes both a `409` rather than a silent success.
func (s State) IsTerminal() bool {
	return s == StateSucceeded || s == StateFailed || s == StateCancelled
}

// The states as the `job` table spells them (db/migrations/0001_init.sql). They live here rather
// than in the adapter because the mapping is the domain's decision, not the driver's: which
// stored value a caller is shown is a contract question, and an adapter that decided it could
// change the contract by changing a switch.
const (
	StoredPending    = "PENDING"
	StoredRunning    = "RUNNING"
	StoredSucceeded  = "SUCCEEDED"
	StoredFailed     = "FAILED"
	StoredDeadLetter = "DEAD_LETTER"
	StoredCancelled  = "CANCELLED"
)

// StateOf translates what the queue stored into what the caller is shown.
//
// An unknown value is an error rather than a default. A row whose state this version does not
// know is a row written by a version that knew more, and answering `QUEUED` for it would tell a
// client to keep polling something that is already over.
func StateOf(stored string) (State, error) {
	switch stored {
	case StoredPending:
		return StateQueued, nil
	case StoredRunning:
		return StateRunning, nil
	case StoredSucceeded:
		return StateSucceeded, nil
	case StoredFailed, StoredDeadLetter:
		return StateFailed, nil
	case StoredCancelled:
		return StateCancelled, nil
	default:
		return "", shared.ErrInternal.WithDetail("jobs.unknown_state")
	}
}

// Job is one piece of background work, narrowed to what leaves the boundary.
//
// What is absent is the point: no payload, no attempt count, no lease, no deduplication key. A
// payload can name objects the caller may not resolve, and the other three are the runner's
// bookkeeping - see the `Job` schema in api/openapi.yaml, which says the same thing to a client.
type Job struct {
	ID shared.ID
	// TenantID is whose work this is, and the zero identifier for work that belongs to no tenant.
	// The `job` table is the one table without row level security (ADR-0010 names the exception),
	// so this field is what every read of it has to be narrowed by - there is no policy
	// underneath that would catch a forgotten condition.
	TenantID shared.ID
	State    State
	// Progress is how far along, between 0 and 1, or nil from a job that cannot say. Nil is the
	// honest answer rather than a number nobody computed. No job kind reports one yet; the first
	// that can is the backup, and it fills this in rather than the resource growing a field.
	Progress *float64
	// ResultURL is where what the job produced can be fetched, empty while there is nothing.
	ResultURL string
	// ErrorCode is the message code of the last failure, never a message (ADR-0011). The queue
	// stores exactly a code in `last_error`, for the reason rule 10 gives: a message can carry
	// what the job was working on.
	ErrorCode  string
	CreatedAt  time.Time
	FinishedAt time.Time
}

// IsTerminal reports whether the job is over.
func (j Job) IsTerminal() bool { return j.State.IsTerminal() }

// Cancellable reports whether the job can still be stopped, and says why not when it cannot.
//
// A conflict rather than a success, because the two answers mean different things to whoever
// asked: a client that reads "cancelled" believes it prevented something, and on a job that
// succeeded three seconds earlier it prevented nothing.
func (j Job) Cancellable() error {
	if !j.IsTerminal() {
		return nil
	}
	return shared.ErrConflict.
		WithDetail("jobs.already_finished").
		WithParams(map[string]string{"job_id": j.ID.String(), "status": j.State.String()})
}
