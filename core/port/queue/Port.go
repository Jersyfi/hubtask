// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package queue is the port for work that happens outside a request: the job queue, the handlers
// that run on it, and the leader that exists exactly once (ADR-0008).
//
// The queue is a table in PostgreSQL rather than a broker, so that a self-hosted installation
// needs no second piece of infrastructure and a job can be created in the same transaction as the
// change that asks for it. The port is what keeps that a decision rather than a fact: a broker
// adapter later implements the same three interfaces.
//
// Two guarantees run through everything here, and the rest of the design follows from them:
//
//   - At-least-once. A process may die at any moment (observability-reliability.md §1), so a job
//     that was picked up but not finished is picked up again once its lease expires. A handler
//     that writes its effect in the transaction the runner opened therefore takes effect exactly
//     once, because the effect and the job's completion commit together (test RT-3).
//   - The queue is not a place to keep state. A job carries what it needs to run, and everything
//     else is read from the database when it runs - a payload that has been waiting an hour is a
//     snapshot of an hour ago.
package queue

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Kind names what a job does. It is the label of every job metric, so the set stays small and
// written by hand - a kind assembled from data would be an unbounded label
// (observability-reliability.md §3.2).
type Kind string

const (
	// KindOutboxDispatch delivers one tenant's pending outbox events (ADR-0007). One job per
	// tenant, which is what "system jobs loop per tenant rather than running globally"
	// (multi-tenancy.md §2.1) looks like once it is a queue.
	KindOutboxDispatch Kind = "outbox.dispatch"

	// KindInvitationEmail tells somebody they have been invited. Queued in the transaction that
	// created the account, so the message and the seat exist together or neither does - and
	// delivered by a worker, so an unreachable mail server never fails the request that invited
	// them (observability-reliability.md §7).
	KindInvitationEmail Kind = "notification.invitation"

	// KindNotificationDeliver sends one notification (C-09). One job per record rather than one
	// per tenant, because the retry belongs to the message: an address the server refuses must not
	// hold up everybody else's mail, and the queue's own attempt budget and dead letter are
	// exactly the retry observability-reliability.md §7 promises for "the reminder arrives late".
	//
	// Queued by the outbox consumer in the dispatcher's transaction, so the record and the job to
	// send it commit together - and detached when it runs, because an SMTP server inside a
	// transaction holds a database connection for as long as somebody else's machine takes (§8).
	KindNotificationDeliver Kind = "notification.deliver"

	// KindReminderFire is the first job in this system that exists because of a stored future
	// timestamp rather than because something just happened (D-03).
	//
	// One job per tenant, seeded by the write that made something due - a reminder written, a due
	// date moved - with RunAt at the moment that write brought forward, because nothing may
	// enumerate tenants (multi-tenancy.md §2.1) and a scheduler therefore cannot create one job
	// per tenant even if it wanted to. It reschedules itself to the next moment the tenant owes
	// and completes when it owes none, which is what keeps a quiet tenant costing nothing: the
	// next write re-seeds it.
	KindReminderFire Kind = "reminder.fire"

	// KindRecurrenceMaterialize turns one tenant's series into the entries their rolling windows
	// owe (D-05, ADR-0008).
	//
	// The same shape as the reminder's wake-up and for the same reason: nothing may enumerate
	// tenants, so the write that made something owed seeds it - a rule written, an occurrence
	// completed - and the pass reschedules itself to the moment the horizon reaches the next
	// occurrence. A series in ON_COMPLETION owes nothing until somebody completes something, so
	// its pass finishes and the completion seeds the next one.
	KindRecurrenceMaterialize Kind = "recurrence.materialize"

	// KindRetentionSweep removes what one tenant's retention periods say may go (ADR-0020).
	//
	// One job per tenant, which reschedules itself forever: a poller lives as one row rather than
	// as a new row per round (Result.Repeat). It is created by a deletion rather than by a
	// scheduler enumerating tenants, because nothing in this system may enumerate them - the
	// `tenant` table is behind row level security with no bypass for the application role, and
	// tenant administration runs through the control plane (db/migrations/0001_init.sql). A
	// deletion scheduling its own cleanup is also the more honest statement of what has to happen.
	KindRetentionSweep Kind = "retention.sweep"

	// KindMediaReconcile makes one tenant's media reference counts honest and reclaims what
	// nothing points at (C-06, data-protection.md §5).
	//
	// One job per tenant, rescheduling itself forever, seeded by a write in the tenant for the
	// reason the retention sweep is: nothing in this system may enumerate tenants, so a scheduler
	// cannot create one job per tenant even if it wanted to. A staging is what seeds it - an
	// upload is the first thing that can ever need reclaiming - and a deletion pulls it forward.
	KindMediaReconcile Kind = "media.reconcile"
)

func (k Kind) String() string { return string(k) }

// Request is a job somebody asks for.
type Request struct {
	Kind Kind
	// TenantID is whose work this is. Zero for work that belongs to no tenant: the job table is
	// deliberately the one table without row level security, because a queue that could only be
	// read under a tenant context could never be read by a worker that does not know the tenant
	// yet (db/migrations/0001_init.sql).
	TenantID shared.ID
	// Payload is what the handler needs in order to start. It is data, never a domain object:
	// the row outlives the process that wrote it, and a type that changed shape in between would
	// take the job with it.
	Payload map[string]any
	// DedupeKey collapses the same request made twice into one job. It is unique per kind while
	// a job is pending or running, so "make sure this tenant is being dispatched" costs one
	// insert that usually does nothing.
	DedupeKey string
	// RunAt is the earliest the job may start. Zero means now.
	RunAt time.Time
	// MaxAttempts is how often the job is tried before it goes to the dead letter. Zero leaves
	// the database default in place.
	MaxAttempts int
}

// Job is one claimed unit of work, as the runner hands it to a handler.
type Job struct {
	ID       shared.ID
	TenantID shared.ID
	Kind     Kind
	Payload  map[string]any
	// Attempts counts this attempt, so the first run of a job reports 1. A handler that behaves
	// differently on a retry - a gentler timeout, a smaller batch - reads it here.
	Attempts    int
	MaxAttempts int
	// Lease is when this claim expires, and it is also the token that proves the claim. Every
	// statement that ends the attempt names it, so a worker that fell so far behind that somebody
	// else took the job over changes nothing and rolls back instead of applying its work twice.
	//
	// It is on the job rather than kept by the runner because the fence is only worth anything if
	// it cannot be forgotten: there is no way to call Complete without it.
	Lease time.Time
}

// LastAttempt reports whether a failure now is final. The runner asks it to decide between a
// retry and the dead letter; a handler may ask it to decide how loudly to complain.
func (j Job) LastAttempt() bool { return j.Attempts >= j.MaxAttempts }

// Result is what a handler leaves behind when it succeeds.
type Result struct {
	// Repeat keeps the job alive instead of finishing it: the same row returns to the queue and
	// runs again after RepeatAfter. That is how a poller lives - one row per tenant that is
	// rescheduled forever, rather than a new row per round that the deduplication of the running
	// one would swallow anyway.
	Repeat      bool
	RepeatAfter time.Duration
}

// Handler runs one kind of job.
//
// It runs inside the transaction the runner opened, which is the whole reliability argument: what
// the handler writes and the job's own completion commit together, so a process that dies halfway
// leaves neither behind. A handler that needs to reach outside the database - a webhook, an email -
// does so through a job of its own rather than from in here, because an external call inside a
// transaction holds a database connection for as long as somebody else's server feels like taking
// (observability-reliability.md §8).
type Handler interface {
	Run(ctx context.Context, job Job) (Result, error)
}

// Detached is a handler the runner does not wrap in a transaction.
//
// The narrow exception to the paragraph above, for the one kind of job that cannot live inside it:
// a pass that has to reach a bucket between two writes. Holding the transaction open across that
// call is exactly what observability-reliability.md §8 forbids, and doing the call after the
// commit is not available to a handler - the runner owns the boundary.
//
// What an implementer gives up is the atomicity everybody else gets for free: its writes and the
// job's completion no longer commit together, so a process that dies between them leaves work done
// and a job that will be claimed again. That is only acceptable for a pass that is safe to run
// twice, and an implementer of this interface is asserting exactly that.
type Detached interface {
	Handler

	// OwnsItsTransactions is the assertion, and its value is never read: implementing the
	// interface is the declaration. A method rather than an empty interface, so that a handler
	// cannot satisfy it by accident.
	OwnsItsTransactions()
}

// Reporter is how a long job says how far along it is (E-05).
//
// A second interface rather than a method on Queue, for the reason persistence.Snapshot is one:
// almost no job needs it. Most finish in milliseconds and the honest answer to "how far along" is
// the null E-01 documented - a client renders an indeterminate bar for it rather than a number
// nobody measured. A method on Queue would put it on every double in the repository for the sake of
// the one handler that runs for minutes.
type Reporter interface {
	// Report writes the fraction, between 0 and 1, and is fenced on the job's lease like every
	// other statement a handler runs: a worker that fell so far behind that somebody else took the
	// job over writes nothing.
	//
	// A failure to report progress is not a failure of the job. The number is for whoever is
	// watching, and losing it is worth a log line rather than a backup.
	Report(ctx context.Context, job Job, fraction float64) error
}

// Lease is the terms on which a batch of jobs is claimed.
type Lease struct {
	// Now is the reading of the clock port, so that a test does not have to wait for time.
	Now time.Time
	// Until is when the claim expires. A job whose lease has run out is claimable again -
	// that is the only thing standing between a killed process and a job nobody ever finishes.
	// It has to outlast the job's own timeout, otherwise a second worker starts the same work
	// while the first is still doing it.
	Until time.Time
	// Batch is how many jobs are claimed at once.
	Batch int
}

// Failure is a job that did not work out.
type Failure struct {
	Job Job
	// Code is why, machine readable: a detail code, never a sentence (rule 8) and never anything
	// the failing operation was working on (rule 10). It is what an operator sees on a dead
	// letter, so it has to be enough to act on and nothing more.
	Code string
	// RetryAt is when the next attempt is due. Zero sends the job to the dead letter instead:
	// the attempts are used up, or the error is one that will not read differently next time.
	RetryAt time.Time
}

// Depth is the backlog of one kind, for the gauge an alert watches when processing stops keeping
// up (observability-reliability.md §10, alert A-06).
type Depth struct {
	Kind Kind
	// Pending counts what is waiting or overdue, not what is running: a job in flight is work
	// being done, and counting it would make a busy queue look like a stuck one.
	Pending int
}

// Queue is the job table. Like every repository it is called inside a unit of work and never
// opens a transaction of its own (project-structure.md §3).
type Queue interface {
	// Enqueue adds a job, or does nothing when one with the same dedupe key is already waiting
	// or running. It is deliberately not an error: the caller asked for work to happen, and work
	// that is already scheduled to happen satisfies that.
	Enqueue(ctx context.Context, request Request) error

	// Claim takes the next batch and marks it running until the lease expires. Implementations
	// use FOR UPDATE SKIP LOCKED, so several workers claim disjoint batches without waiting for
	// each other (ADR-0008).
	Claim(ctx context.Context, lease Lease) ([]Job, error)

	// Hold takes the row lock on one claimed job for the rest of the caller's transaction.
	//
	// It exists for the one duty whose correctness depends on it (D-03). A job that decides when
	// it next runs reads that moment from the data, and a write committing between that read and
	// the reschedule would find the row RUNNING - where Enqueue's conflict clause cannot pull a
	// wake-up forward - and its reminder would wait for a wake-up nobody scheduled. Held from the
	// start of the pass, such a write instead waits for the pass to end and then meets either a
	// pending row it can pull forward or a finished one it may replace.
	//
	// Every other handler is welcome to ignore it: a poller that always comes back has nothing to
	// lose by missing a pull-forward.
	Hold(ctx context.Context, job Job) error

	// Complete finishes a job for good. Called in the same transaction as the handler's effect,
	// and refused when the job's lease no longer holds.
	Complete(ctx context.Context, job Job) error

	// Repeat returns a job to the queue for another round at runAt, with its attempt count
	// cleared: a poller's next round is not a retry of the last one.
	Repeat(ctx context.Context, job Job, runAt time.Time) error

	// Fail records an attempt that did not work: back to the queue when there is a retry left,
	// to the dead letter when there is not. It runs in a transaction of its own, because the
	// handler's has just been rolled back.
	Fail(ctx context.Context, failure Failure) error

	// Depth reports the backlog per kind, across every tenant. The job table has no tenant
	// boundary to respect here, which is what makes one global gauge possible at all.
	Depth(ctx context.Context) ([]Depth, error)
}

// Leadership is the "exactly one" of ADR-0008: the scheduler role may run in several replicas so
// that one can fail, but only one of them may act, or every reminder fires twice.
//
// It is not a lease in a table but a PostgreSQL advisory lock held on one connection. The
// difference matters when a process is killed rather than shut down: a table lease has to expire
// before anybody else may act, while a lock dies with the connection that held it - the operating
// system does the releasing, and the successor takes over in seconds
// (observability-reliability.md §9).
type Leadership interface {
	// Acquire tries to become the leader and reports whether it worked. A follower calls it again
	// on its next tick; being turned down is the normal state of a standby, not an error.
	Acquire(ctx context.Context) (bool, error)

	// Confirm reports whether leadership is still held. A leader asks before every tick: the
	// connection carrying the lock can be cut without anybody noticing, and a former leader that
	// keeps working is exactly the double execution the lock exists to prevent.
	Confirm(ctx context.Context) (bool, error)

	// Release gives leadership up. A graceful shutdown calls it so that the standby can take over
	// immediately rather than after the connection times out.
	Release(ctx context.Context) error
}
