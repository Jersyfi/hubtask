// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package job

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/job"
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
	// asker is who the payload names; the zero value is a job the system started.
	asker repository.Asker
}

func (s *jobStore) AskerOf(context.Context, shared.ID) (repository.Asker, error) {
	if !s.present {
		return repository.Asker{}, shared.ErrNotFound.WithDetail("jobs.not_found")
	}
	return s.asker, nil
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
	// permits is what the recording-free question answers, and permitted what it was asked.
	permits   bool
	permitted []access.Request
}

func (a *authorizerDouble) Permits(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) (bool, error) {
	a.permitted = append(a.permitted, request)
	return a.permits, nil
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

// A job that can say how far along it is, and where its result will be, carries both.
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

// asking is the harness of a question to AI the caller asked about one entry.
func asking(state domain.State, askedBy shared.ID, kind string) *harness {
	h := newHarness(state)
	h.jobs.asker = repository.Asker{
		Kind: kind, AskedBy: askedBy, TargetType: "WORK_ITEM",
		TargetID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e1"),
	}
	h.authorizer.permits = true
	return h
}

func askerWithToken() appshared.ActorContext {
	actor := caller()
	actor.Scopes = []string{jobsRead, jobsCancel}
	return actor
}

// UC-AI-04 check 9: the person who asked a question to AI watches and stops it wherever their role
// sits - asked along the entry's path with the question that records nothing, and never through
// the workspace-scope permission, so no refusal is written for somebody let through.
func TestThePersonWhoAskedMayWatchAndStopTheirQuestion(t *testing.T) {
	h := asking(domain.StateRunning, actorID, "ai.suggest")

	if _, err := h.get().Execute(context.Background(), askerWithToken(), Query{JobID: jobID}); err != nil {
		t.Fatalf("reading their own question: %v", err)
	}
	stopped, err := h.cancel().Execute(context.Background(), askerWithToken(), Query{JobID: jobID})
	if err != nil {
		t.Fatalf("stopping their own question: %v", err)
	}
	if stopped.State != domain.StateCancelled || len(h.audit.entries) != 1 {
		t.Errorf("stopped %+v with %d audit entries", stopped, len(h.audit.entries))
	}
	if len(h.authorizer.requests) != 0 {
		t.Errorf("the workspace-scope permission was asked: %+v", h.authorizer.requests)
	}
	if len(h.authorizer.permitted) != 2 {
		t.Fatalf("%d recording-free questions", len(h.authorizer.permitted))
	}
	path := h.authorizer.permitted[0].Path
	if len(path) != 2 || path[1].ID != h.jobs.asker.TargetID {
		t.Errorf("asked along %v, want the workspace and the entry", path)
	}
}

// Everything that is not the asker's own question falls through to the ordinary permission, which
// decides and records as it always did.
func TestAnyoneElseNeedsTheOrdinaryPermission(t *testing.T) {
	other := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f9")
	for name, h := range map[string]*harness{
		"another person's question": asking(domain.StateRunning, other, "ai.suggest"),
		"work the system started":   asking(domain.StateRunning, shared.ID(""), "ai.suggest"),
		"a backup":                  asking(domain.StateRunning, actorID, "backup.run"),
		"an asker who lost the read": func() *harness {
			h := asking(domain.StateRunning, actorID, "ai.suggest")
			h.authorizer.permits = false
			return h
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")
			_, err := h.cancel().Execute(context.Background(), askerWithToken(), Query{JobID: jobID})
			if !errors.Is(err, shared.ErrForbidden) {
				t.Fatalf("answered %v, want the ordinary refusal", err)
			}
			if len(h.authorizer.requests) != 1 || h.authorizer.requests[0].Permission != domainservice.PermissionStructure {
				t.Errorf("the ordinary permission was not what decided: %+v", h.authorizer.requests)
			}
			if h.jobs.cancelled {
				t.Error("the job was stopped")
			}
		})
	}
}

// A token without the scope is no asker either: the scope is the second bound, and it holds for
// the asker as for anybody.
func TestTheAskersTokenStillNeedsTheScope(t *testing.T) {
	h := asking(domain.StateRunning, actorID, "ai.suggest")
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.insufficient_scope")
	actor := caller()
	actor.Scopes = []string{jobsRead}

	if _, err := h.cancel().Execute(context.Background(), actor, Query{JobID: jobID}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("answered %v", err)
	}
	if len(h.authorizer.permitted) != 0 {
		t.Error("the asker's read was asked for a token that may not stop anything")
	}
}

// A rule drafted for its asker is theirs to stop: the draft is about nobody's content but theirs, so
// being the asker is the whole question.
func TestTheAskerStopsTheDraftingOfTheirRule(t *testing.T) {
	h := newHarness(domain.StateRunning)
	h.jobs.asker = repository.Asker{Kind: "ai.draft_rule", AskedBy: actorID, TargetType: "ACCOUNT", TargetID: actorID}

	if _, err := h.cancel().Execute(context.Background(), askerWithToken(), Query{JobID: jobID}); err != nil {
		t.Fatalf("stopping the draft: %v", err)
	}
	if len(h.authorizer.requests) != 0 || len(h.authorizer.permitted) != 0 {
		t.Errorf("a permission was asked of the asker: %+v %+v", h.authorizer.requests, h.authorizer.permitted)
	}
}
