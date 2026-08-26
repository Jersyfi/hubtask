// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/job"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenantID = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000001")
	actorID  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000002")
	jobID    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000003")

	now = time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
)

func caller() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: actorID,
		AccountName: "Ada", Locale: "en", TimeZone: "UTC",
	}
}

// jobStore is the repository as these use cases see it: a row that is there or is not, and a
// cancellation that refuses a terminal one exactly as the statement does.
type jobStore struct {
	stored    domain.Job
	present   bool
	cancelled bool
	at        time.Time
}

func (s *jobStore) Find(context.Context, shared.ID) (domain.Job, error) {
	if !s.present {
		return domain.Job{}, shared.ErrNotFound.WithDetail("jobs.not_found")
	}
	return s.stored, nil
}

func (s *jobStore) Cancel(_ context.Context, _ shared.ID, at time.Time) (domain.Job, error) {
	if !s.present {
		return domain.Job{}, shared.ErrNotFound.WithDetail("jobs.not_found")
	}
	if s.stored.IsTerminal() {
		return domain.Job{}, s.stored.Cancellable()
	}
	s.cancelled, s.at = true, at
	s.stored.State, s.stored.FinishedAt = domain.StateCancelled, at
	return s.stored, nil
}

type authorizerDouble struct {
	err      error
	requests []access.Request
}

func (a *authorizerDouble) Authorize(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) error {
	a.requests = append(a.requests, request)
	return a.err
}

type unitOfWork struct{ writes, reads int }

func (u *unitOfWork) Within(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	u.writes++
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	u.reads++
	return fn(ctx)
}

// sink judges what it is given the way the adapter does, so an entry the database would refuse
// fails here rather than in an integration test.
type sink struct{ entries []audit.Entry }

func (s *sink) Append(_ context.Context, entry audit.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	s.entries = append(s.entries, entry)
	return nil
}

type harness struct {
	jobs       *jobStore
	authorizer *authorizerDouble
	audit      *sink
	uow        *unitOfWork
}

func newHarness(state domain.State) *harness {
	return &harness{
		jobs: &jobStore{
			present: true,
			stored: domain.Job{
				ID: jobID, TenantID: tenantID, State: state, CreatedAt: now.Add(-time.Hour),
			},
		},
		authorizer: &authorizerDouble{},
		audit:      &sink{},
		uow:        &unitOfWork{},
	}
}

func (h *harness) get() GetJob {
	return GetJob{Jobs: h.jobs, Authorizer: h.authorizer, UnitOfWork: h.uow}
}

func (h *harness) cancel() CancelJob {
	return CancelJob{
		Jobs: h.jobs, Authorizer: h.authorizer, Audit: h.audit,
		Clock: clock.Fixed(now), UnitOfWork: h.uow,
	}
}

func TestReadingAJobAsksForReadAtTheTenantAndOpensAReadOnlyTransaction(t *testing.T) {
	h := newHarness(domain.StateRunning)

	job, err := h.get().Execute(context.Background(), caller(), Query{JobID: jobID})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if job.State != domain.StateRunning {
		t.Fatalf("state %q", job.State)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d authorisation questions asked", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	switch {
	case request.Permission != domainservice.PermissionRead:
		t.Fatalf("asked for %q", request.Permission)
	case request.TokenScope != jobsRead:
		t.Fatalf("asked for scope %q", request.TokenScope)
	case len(request.Path) != 1:
		t.Fatalf("the path has %d scopes; a job is anchored to nothing", len(request.Path))
	}

	// A read replica may serve it, and a write transaction would pin the poll to the primary.
	if h.uow.writes != 0 || h.uow.reads != 1 {
		t.Fatalf("%d writes and %d reads", h.uow.writes, h.uow.reads)
	}
}

// The refusal is written before the transaction opens, which is what the ordering of these two
// calls is about: an entry written inside it would be rolled back with the refusal.
func TestARefusedReadNeverReachesTheRepository(t *testing.T) {
	h := newHarness(domain.StateQueued)
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	if _, err := h.get().Execute(context.Background(), caller(), Query{JobID: jobID}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("read: %v, want forbidden", err)
	}
	if h.uow.reads != 0 || h.uow.writes != 0 {
		t.Fatal("a refused read opened a transaction")
	}
}

func TestCancellingAsksForAHigherPermissionAndItsOwnScope(t *testing.T) {
	h := newHarness(domain.StateRunning)

	job, err := h.cancel().Execute(context.Background(), caller(), Query{JobID: jobID})
	if err != nil {
		t.Fatalf("cancelling: %v", err)
	}
	if job.State != domain.StateCancelled {
		t.Fatalf("state %q after cancelling", job.State)
	}
	if !h.jobs.cancelled || !h.jobs.at.Equal(now) {
		t.Fatalf("the repository was asked to cancel at %s", h.jobs.at)
	}

	request := h.authorizer.requests[0]
	switch {
	case request.Permission != domainservice.PermissionStructure:
		t.Fatalf("asked for %q; stopping the workspace's work is an administrator's act", request.Permission)
	case request.TokenScope != jobsCancel:
		t.Fatalf("asked for scope %q", request.TokenScope)
	case request.TokenScope == jobsRead:
		t.Fatal("show me and stop it are the same scope")
	}
}

// The entry an auditor is looking for after "why did the backup not run on Tuesday": who stopped
// it, when, and what it was doing at the time.
func TestCancellingWritesTheAuditEntryInTheSameTransaction(t *testing.T) {
	h := newHarness(domain.StateRunning)

	if _, err := h.cancel().Execute(context.Background(), caller(), Query{JobID: jobID}); err != nil {
		t.Fatalf("cancelling: %v", err)
	}
	if len(h.audit.entries) != 1 {
		t.Fatalf("%d audit entries", len(h.audit.entries))
	}

	entry := h.audit.entries[0]
	switch {
	case entry.Action != JobCancelledAction:
		t.Fatalf("action %q", entry.Action)
	case entry.Severity != audit.SeverityNotice:
		t.Fatalf("severity %q", entry.Severity)
	case entry.TargetID != jobID || entry.TargetType != jobTarget:
		t.Fatalf("target %s/%s", entry.TargetType, entry.TargetID)
	case !entry.OccurredAt.Equal(now):
		t.Fatalf("occurred at %s", entry.OccurredAt)
	}
	if h.uow.writes != 1 {
		t.Fatalf("%d write transactions", h.uow.writes)
	}
}

func TestCancellingATerminalJobIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	for _, state := range []domain.State{
		domain.StateSucceeded, domain.StateFailed, domain.StateCancelled,
	} {
		h := newHarness(state)

		_, err := h.cancel().Execute(context.Background(), caller(), Query{JobID: jobID})
		if !errors.Is(err, shared.ErrConflict) {
			t.Fatalf("%q: %v, want a conflict", state, err)
		}
		if h.jobs.cancelled {
			t.Fatalf("%q: the job was cancelled anyway", state)
		}
		if len(h.audit.entries) != 0 {
			t.Fatalf("%q: an audit entry was written for a cancellation that did not happen", state)
		}
	}
}

func TestAMissingJobIdentifierIsAValidationError(t *testing.T) {
	h := newHarness(domain.StateQueued)

	if _, err := h.get().Execute(context.Background(), caller(), Query{}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("read without an identifier: %v", err)
	}
	if _, err := h.cancel().Execute(context.Background(), caller(), Query{}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("cancel without an identifier: %v", err)
	}
	if len(h.authorizer.requests) != 0 {
		t.Fatal("a request with no identifier reached the authorisation service")
	}
}

// What leaves the boundary, checked where it is decided rather than at the edge that maps it: the
// catalogue's answer carries no payload, no attempt count, no lease and no deduplication key.
func TestTheAnswerCarriesNothingOfTheQueuesOwnBookkeeping(t *testing.T) {
	h := newHarness(domain.StateRunning)

	out, err := h.get().invoke(context.Background(), caller(), map[string]any{"job_id": jobID.String()})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}

	allowed := map[string]bool{
		"job_id": true, "status": true, "progress": true, "result_url": true,
		"error_code": true, "created_at": true, "finished_at": true,
	}
	for field := range out {
		if !allowed[field] {
			t.Fatalf("the answer carries %q, which is the queue's business", field)
		}
	}
	// Absent rather than zero: a progress of 0 means "nothing done yet", and a client that read
	// one for the other would draw a bar that never moves.
	if _, present := out["progress"]; present {
		t.Fatal("a job that cannot compute progress reported one")
	}
	if _, present := out["finished_at"]; present {
		t.Fatal("a running job reported a finishing time")
	}
}

// The catalogue entries. The parity gate checks that they are registered; what belongs here is
// what they say about themselves, because those declarations are what three channels are built
// from and what gate SG-13 reads.
func TestTheCatalogueEntriesDeclareWhatTheThreeChannelsNeed(t *testing.T) {
	h := newHarness(domain.StateQueued)

	read := h.get().Descriptor()
	switch {
	case read.Name != GetJobName:
		t.Errorf("name %q", read.Name)
	case !read.ReadOnly:
		t.Error("the read is not marked read-only, so an agent client cannot tell it is safe")
	case read.TokenScope != jobsRead:
		t.Errorf("scope %q", read.TokenScope)
	case read.Audit.Required:
		t.Error("an ordinary poll is declared as an entry the trail owes")
	case read.RESTOperation() != "getJob" || read.MCPTool() != "get_job":
		t.Errorf("channel identities %q and %q", read.RESTOperation(), read.MCPTool())
	}

	stop := h.cancel().Descriptor()
	switch {
	case stop.Name != CancelJobName:
		t.Errorf("name %q", stop.Name)
	case stop.ReadOnly:
		t.Error("stopping work is marked read-only")
	case stop.TokenScope != jobsCancel:
		t.Errorf("scope %q", stop.TokenScope)
	case !stop.Audit.Required:
		t.Error("stopping work is declared as something the trail need not record")
	case stop.Audit.Action != JobCancelledAction:
		t.Errorf("audit action %q", stop.Audit.Action)
	}

	// Both declare the one field they take, so every channel refuses a malformed identifier the
	// same way rather than three times differently.
	for _, descriptor := range []usecase.Descriptor{read, stop} {
		if len(descriptor.Input) != 1 || descriptor.Input[0].Name != "job_id" {
			t.Fatalf("%s declares %v", descriptor.Name, descriptor.Input)
		}
		if !descriptor.Input[0].Required {
			t.Errorf("%s takes job_id as optional", descriptor.Name)
		}
		if err := descriptor.ValidateInput(usecase.Input{}); err == nil {
			t.Errorf("%s accepted a call with no identifier", descriptor.Name)
		}
		// The shape is the catalogue's to check; that the string is an identifier is the
		// accessor's, and both refusals have to happen before a repository is reached.
		if _, err := descriptor.Handler.Invoke(
			context.Background(), caller(), usecase.Input{"job_id": "not a uuid"},
		); err == nil {
			t.Errorf("%s accepted an identifier that is not one", descriptor.Name)
		}
	}
}

func TestCancellingThroughTheCatalogueAnswersTheJobItNowIs(t *testing.T) {
	h := newHarness(domain.StateRunning)

	out, err := h.cancel().invoke(
		context.Background(), caller(), usecase.Input{"job_id": jobID.String()})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if out["status"] != domain.StateCancelled.String() {
		t.Fatalf("the answer says %v", out["status"])
	}
	if out["finished_at"] != now {
		t.Fatalf("finished at %v", out["finished_at"])
	}

	// The identifier is validated by the catalogue before a handler sees it; the handler still
	// refuses one that reached it another way rather than passing an empty one downwards.
	if _, err := h.cancel().invoke(context.Background(), caller(), usecase.Input{}); err == nil {
		t.Fatal("a call with no identifier was accepted")
	}
	if _, err := h.get().invoke(context.Background(), caller(), usecase.Input{}); err == nil {
		t.Fatal("a read with no identifier was accepted")
	}
}

// A job that can say how far along it is, and where its result will be, carries both. Nothing
// reports either yet - the first that will is the backup - so this is the shape that has to be
// there when it does rather than a field added later.
func TestAJobThatCanSayHowFarAlongItIsSaysSo(t *testing.T) {
	half := 0.5
	h := newHarness(domain.StateRunning)
	h.jobs.stored.Progress = &half
	h.jobs.stored.ResultURL = "https://example.invalid/backups/2026-08-26"
	h.jobs.stored.ErrorCode = "backup.target_slow"

	out, err := h.get().invoke(
		context.Background(), caller(), usecase.Input{"job_id": jobID.String()})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	switch {
	case out["progress"] != half:
		t.Errorf("progress %v", out["progress"])
	case out["result_url"] != h.jobs.stored.ResultURL:
		t.Errorf("result reference %v", out["result_url"])
	case out["error_code"] != "backup.target_slow":
		t.Errorf("error code %v", out["error_code"])
	}
}
