// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/recurrence"
)

var (
	runID      = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	jobID      = shared.MustParseID("0192f000-0000-7000-8000-00000000000e")
	scheduleID = shared.MustParseID("0192f000-0000-7000-8000-00000000000f")
)

// runStore is the runs, as a table a test writes by hand.
type runStore struct {
	stored   map[shared.ID]domain.Run
	started  []domain.Run
	claimed  bool
	outcomes []domain.Outcome
	latest   *domain.Run
}

func newRuns() *runStore {
	return &runStore{stored: map[shared.ID]domain.Run{}, claimed: true}
}

func (s *runStore) Start(_ context.Context, run domain.Run) (bool, error) {
	s.started = append(s.started, run)
	if !s.claimed {
		return false, nil
	}
	s.stored[run.ID] = run
	return true, nil
}

func (s *runStore) Find(_ context.Context, id shared.ID) (domain.Run, error) {
	run, found := s.stored[id]
	if !found {
		return domain.Run{}, shared.ErrNotFound.WithDetail(domain.CodeRunNotFound)
	}
	return run, nil
}

func (s *runStore) Finish(_ context.Context, outcome domain.Outcome) error {
	s.outcomes = append(s.outcomes, outcome)
	run := s.stored[outcome.ID]
	run.Status, run.ArchivePath, run.FinishedAt = outcome.Status, outcome.ArchivePath, outcome.FinishedAt
	run.SizeBytes, run.ItemCount, run.MediaCount = outcome.SizeBytes, outcome.ItemCount, outcome.MediaCount
	run.SnapshotAt, run.ErrorCode, run.Checksum = outcome.SnapshotAt, outcome.ErrorCode, outcome.Checksum
	s.stored[outcome.ID] = run
	return nil
}

func (s *runStore) LatestSuccessful(context.Context, shared.ID) (domain.Run, error) {
	if s.latest == nil {
		return domain.Run{}, shared.ErrNotFound.WithDetail(domain.CodeNoParentArchive)
	}
	return *s.latest, nil
}

func (s *runStore) RecordVerification(_ context.Context, id shared.ID, at time.Time, ok bool) error {
	run := s.stored[id]
	run.VerifiedAt, run.VerifyOK = at, &ok
	s.stored[id] = run
	return nil
}

func (s *runStore) SetExpiry(_ context.Context, id shared.ID, expiresAt time.Time) error {
	run := s.stored[id]
	run.ExpiresAt = expiresAt
	s.stored[id] = run
	return nil
}

func (s *runStore) MarkExpired(_ context.Context, id shared.ID) error {
	run := s.stored[id]
	run.Status = domain.RunExpired
	s.stored[id] = run
	return nil
}

func (s *runStore) LastSuccessPerTarget(context.Context) (map[shared.ID]time.Time, error) {
	answer := map[shared.ID]time.Time{}
	for _, run := range s.stored {
		if run.Succeeded() {
			answer[run.TargetID] = run.FinishedAt
		}
	}
	return answer, nil
}

var _ repository.Runs = (*runStore)(nil)

// jobs is the queue, remembering what it was asked for.
type jobs struct {
	requests []queue.Request
	failure  error
}

func (j *jobs) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	if j.failure != nil {
		return "", j.failure
	}
	j.requests = append(j.requests, request)
	return jobID, nil
}

func (j *jobs) Claim(context.Context, queue.Lease) ([]queue.Job, error) { return nil, nil }
func (j *jobs) Hold(context.Context, queue.Job) error                   { return nil }
func (j *jobs) Complete(context.Context, queue.Job) error               { return nil }
func (j *jobs) Repeat(context.Context, queue.Job, time.Time) error      { return nil }
func (j *jobs) Fail(context.Context, queue.Failure) error               { return nil }

func (j *jobs) Depth(context.Context) ([]queue.Depth, error) { return nil, nil }

var _ queue.Queue = (*jobs)(nil)

func (h *harness) runner(runs *runStore, queued *jobs) Runner {
	return Runner{
		Runs: runs, Targets: h.targets, Jobs: queued,
		Authorizer: h.authorizer, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), IDs: ids{next: runID},
	}
}

// enabledTarget puts one target in the shelf, the way creating one would have.
func enabledTarget(t *testing.T, h *harness) domain.Target {
	t.Helper()

	target, err := domain.NewTarget(domain.NewTargetInput{
		ID: targetID, TenantID: tenantID, Name: "Off-site bucket", Kind: domain.KindS3,
		Config:    domain.TargetConfig{"bucket": "hubtask-backups", "endpoint": "https://s3.example.org"},
		CreatedBy: actorID, Now: now,
	})
	if err != nil {
		t.Fatalf("building the target: %v", err)
	}
	h.targets.stored = append(h.targets.stored, target)
	return target
}

func TestStartingABackupEnqueuesAJobAndNamesTheRunItWillProduce(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	runs, queued := newRuns(), &jobs{}

	accepted, err := (StartBackup{Runner: h.runner(runs, queued)}).
		Execute(context.Background(), caller(), StartBackupCommand{TargetID: targetID})
	if err != nil {
		t.Fatalf("starting: %v", err)
	}

	switch {
	case accepted.JobID != jobID:
		t.Fatalf("job %s", accepted.JobID)
	case accepted.RunID != runID:
		t.Fatalf("run %s", accepted.RunID)
	case len(queued.requests) != 1:
		t.Fatalf("%d jobs enqueued", len(queued.requests))
	}

	request := queued.requests[0]
	if request.Kind != queue.KindBackupRun {
		t.Fatalf("kind %s", request.Kind)
	}
	// The deduplication key is the target: two requests to back up the same target collapse into
	// the one that is already happening, which is the lock §5 asks for expressed in the queue.
	if request.DedupeKey != string(queue.KindBackupRun)+":"+targetID.String() {
		t.Fatalf("dedupe key %q", request.DedupeKey)
	}
	if request.Payload["mode"] != string(domain.ModeFull) {
		t.Fatalf("a manual run defaulted to %v, want FULL", request.Payload["mode"])
	}
	if request.Payload["run_id"] != runID.String() {
		t.Fatalf("the job does not name its run: %v", request.Payload)
	}
}

// The administrator's line rather than the owner's: creating a target opens a channel the data may
// leave by, and using one that has already been approved is running the workspace.
func TestStartingABackupAsksForTheAdministratorsRight(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)

	if _, err := (StartBackup{Runner: h.runner(newRuns(), &jobs{})}).
		Execute(context.Background(), caller(), StartBackupCommand{TargetID: targetID}); err != nil {
		t.Fatalf("starting: %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d authorisations", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.Permission != domainservice.PermissionStructure {
		t.Fatalf("permission %s", request.Permission)
	}
	if request.TokenScope != backupManage {
		t.Fatalf("token scope %s", request.TokenScope)
	}
}

// A run is the moment the tenant's data actually leaves, which is the entry an auditor looks for.
func TestStartingABackupIsAudited(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)

	if _, err := (StartBackup{Runner: h.runner(newRuns(), &jobs{})}).
		Execute(context.Background(), caller(), StartBackupCommand{TargetID: targetID}); err != nil {
		t.Fatalf("starting: %v", err)
	}

	if len(h.audit.entries) != 1 {
		t.Fatalf("%d audit entries", len(h.audit.entries))
	}
	entry := h.audit.entries[0]
	if entry.Action != StartedAction || entry.TargetID != runID {
		t.Fatalf("entry: %+v", entry)
	}
}

// A refusal rather than a quiet promotion to a full run: a caller that asked for an incremental
// and silently got a full one has been told nothing about how long it will take.
func TestAnIncrementalWithNoParentIsRefused(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	runs, queued := newRuns(), &jobs{}

	_, err := (StartBackup{Runner: h.runner(runs, queued)}).Execute(context.Background(), caller(),
		StartBackupCommand{TargetID: targetID, Mode: domain.ModeIncremental})
	if err == nil {
		t.Fatal("an incremental with nothing to continue was accepted")
	}
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("category: %v", err)
	}
	if len(queued.requests) != 0 {
		t.Fatal("a job was enqueued for a run that cannot happen")
	}
}

func TestAnIncrementalNamesTheArchiveItContinues(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	runs, queued := newRuns(), &jobs{}
	parent := domain.Run{ID: scheduleID, TargetID: targetID, Status: domain.RunSucceeded}
	runs.latest = &parent

	if _, err := (StartBackup{Runner: h.runner(runs, queued)}).Execute(context.Background(), caller(),
		StartBackupCommand{TargetID: targetID, Mode: domain.ModeIncremental}); err != nil {
		t.Fatalf("starting: %v", err)
	}

	if queued.requests[0].Payload["parent_run_id"] != parent.ID.String() {
		t.Fatalf("the job does not name its parent: %v", queued.requests[0].Payload)
	}
}

func TestABackupToATargetThatIsSwitchedOffIsRefused(t *testing.T) {
	h := newHarness()
	target := enabledTarget(t, h)
	target.Enabled = false
	h.targets.stored[0] = target

	_, err := (StartBackup{Runner: h.runner(newRuns(), &jobs{})}).
		Execute(context.Background(), caller(), StartBackupCommand{TargetID: targetID})
	if err == nil {
		t.Fatal("a backup to a disabled target was accepted")
	}
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("category: %v", err)
	}
}

func TestAModeNobodyDefinedIsRefused(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)

	_, err := (StartBackup{Runner: h.runner(newRuns(), &jobs{})}).Execute(context.Background(), caller(),
		StartBackupCommand{TargetID: targetID, Mode: "PARTIAL"})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("mode PARTIAL: %v", err)
	}
}

func TestARunIsReadBack(t *testing.T) {
	h := newHarness()
	runs := newRuns()
	runs.stored[runID] = domain.Run{
		ID: runID, TargetID: targetID, TenantID: tenantID, Trigger: domain.TriggerManual,
		Mode: domain.ModeFull, Status: domain.RunSucceeded, ArchivePath: "hubtask-backup-x",
		StartedAt: now, FinishedAt: now.Add(time.Minute),
	}

	run, err := (GetBackupRun{Runner: h.runner(runs, &jobs{})}).
		Execute(context.Background(), caller(), runID)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if run.ID != runID || run.ArchivePath != "hubtask-backup-x" {
		t.Fatalf("run: %+v", run)
	}
	if h.uow.reads != 1 || h.uow.writes != 0 {
		t.Fatalf("a read opened %d write transactions", h.uow.writes)
	}
}

func TestVerifyingEnqueuesAJobForAnArchiveThatExists(t *testing.T) {
	h := newHarness()
	runs, queued := newRuns(), &jobs{}
	runs.stored[runID] = domain.Run{
		ID: runID, TargetID: targetID, Status: domain.RunSucceeded, ArchivePath: "hubtask-backup-x",
	}

	accepted, err := (VerifyBackup{Runner: h.runner(runs, queued)}).
		Execute(context.Background(), caller(), runID)
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if accepted.RunID != runID || accepted.JobID != jobID {
		t.Fatalf("accepted: %+v", accepted)
	}
	if queued.requests[0].Kind != queue.KindBackupVerify {
		t.Fatalf("kind %s", queued.requests[0].Kind)
	}
}

// There is nothing at the target to check for a run that never finished writing one, and an answer
// of "no" for that reason would read as a corrupt archive.
func TestVerifyingARunThatLeftNoArchiveIsRefused(t *testing.T) {
	h := newHarness()
	runs, queued := newRuns(), &jobs{}
	runs.stored[runID] = domain.Run{ID: runID, TargetID: targetID, Status: domain.RunFailed}

	_, err := (VerifyBackup{Runner: h.runner(runs, queued)}).
		Execute(context.Background(), caller(), runID)
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("verifying a failed run: %v", err)
	}
	if len(queued.requests) != 0 {
		t.Fatal("a job was enqueued for an archive that is not there")
	}
}

func TestVerifyingARunNobodyHasIsNotFound(t *testing.T) {
	h := newHarness()

	_, err := (VerifyBackup{Runner: h.runner(newRuns(), &jobs{})}).
		Execute(context.Background(), caller(), runID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("verifying a run nobody has: %v", err)
	}
}

// A refusal from the authoriser stops everything, which is the property rule 2 is about.
func TestAnActorWithoutTheRightStartsNothing(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	h.authorizer.err = shared.ErrForbidden
	queued := &jobs{}

	if _, err := (StartBackup{Runner: h.runner(newRuns(), queued)}).
		Execute(context.Background(), caller(), StartBackupCommand{TargetID: targetID}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("starting: %v", err)
	}
	if len(queued.requests) != 0 || len(h.audit.entries) != 0 {
		t.Fatal("a refused caller still enqueued a job or wrote an entry")
	}
}

// expander is the recurrence port, playing a rehearsed answer.
type expander struct {
	moments []time.Time
	failure error
	rules   []recurrence.Rule
}

func (e *expander) Occurrences(rule recurrence.Rule, after, before time.Time, limit int) ([]time.Time, error) {
	e.rules = append(e.rules, rule)
	if e.failure != nil {
		return nil, e.failure
	}
	var within []time.Time
	for _, moment := range e.moments {
		if moment.After(after) && !moment.After(before) && len(within) < limit {
			within = append(within, moment)
		}
	}
	return within, nil
}

var _ recurrence.Expander = (*expander)(nil)

// The other two channels go through the descriptor rather than through Execute, so the mapping in
// and out is worth exercising: MCP and automation call exactly this.
func TestTheDescriptorsCarryTheSameWorkAsExecute(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	runs, queued := newRuns(), &jobs{}
	runner := h.runner(runs, queued)

	started, err := (StartBackup{Runner: runner}).invoke(context.Background(), caller(),
		map[string]any{
			"target_id": targetID.String(), "mode": "FULL",
			"include_media": false, "include_audit": false,
		})
	if err != nil {
		t.Fatalf("invoking StartBackup: %v", err)
	}
	if started.String("job_id") != jobID.String() || started.String("run_id") != runID.String() {
		t.Fatalf("output: %+v", started)
	}
	if queued.requests[0].Payload["include_media"] != false {
		t.Fatalf("a flag that was sent as false arrived as %v", queued.requests[0].Payload["include_media"])
	}

	// And the two flags default to true when they are absent, because a backup that quietly left
	// the attachments behind is not the backup anybody meant.
	queued.requests = nil
	if _, err := (StartBackup{Runner: runner}).invoke(context.Background(), caller(),
		map[string]any{"target_id": targetID.String()}); err != nil {
		t.Fatalf("invoking StartBackup: %v", err)
	}
	if queued.requests[0].Payload["include_media"] != true {
		t.Fatalf("an absent flag arrived as %v", queued.requests[0].Payload["include_media"])
	}

	runs.stored[runID] = domain.Run{
		ID: runID, TargetID: targetID, Trigger: domain.TriggerManual, Mode: domain.ModeFull,
		Status: domain.RunSucceeded, ArchivePath: "hubtask-backup-x", StartedAt: now,
		FinishedAt: now.Add(time.Minute), SizeBytes: 4096, ItemCount: 12, MediaCount: 2,
		SnapshotAt: now, ParentRunID: scheduleID,
	}
	out, err := (GetBackupRun{Runner: runner}).invoke(context.Background(), caller(),
		map[string]any{"run_id": runID.String()})
	if err != nil {
		t.Fatalf("invoking GetBackupRun: %v", err)
	}
	switch {
	case out.String("id") != runID.String():
		t.Fatalf("id %q", out.String("id"))
	case out.String("archive_path") != "hubtask-backup-x":
		t.Fatalf("archive path %q", out.String("archive_path"))
	case out["size_bytes"] != int64(4096):
		t.Fatalf("size %v", out["size_bytes"])
	case out["parent_run_id"] != scheduleID.String():
		t.Fatalf("parent %v", out["parent_run_id"])
	}

	verified, err := (VerifyBackup{Runner: runner}).invoke(context.Background(), caller(),
		map[string]any{"run_id": runID.String()})
	if err != nil {
		t.Fatalf("invoking VerifyBackup: %v", err)
	}
	if verified.String("run_id") != runID.String() {
		t.Fatalf("output: %+v", verified)
	}
}

// A run that produced nothing answers nothing rather than zeroes: a size of 0 and a finished_at of
// 1970 are two ways of saying "we do not know" that read as facts.
func TestARunningRunAnswersNoNumbersItDoesNotHave(t *testing.T) {
	run := domain.Run{
		ID: runID, TargetID: targetID, Trigger: domain.TriggerSchedule,
		Mode: domain.ModeIncremental, Status: domain.RunRunning, StartedAt: now,
	}

	out := runOutput(run)
	for _, absent := range []string{
		"archive_path", "size_bytes", "item_count", "media_count",
		"finished_at", "expires_at", "verified_at", "verify_ok", "error_code",
		"schedule_id", "parent_run_id", "snapshot_at",
	} {
		if _, present := out[absent]; present {
			t.Errorf("a running run answered %s: %v", absent, out[absent])
		}
	}
	if out.String("status") != string(domain.RunRunning) {
		t.Fatalf("status %q", out.String("status"))
	}
}

// The three descriptors say what they do, which is what the other two channels render.
func TestTheThreeRunDescriptorsAreComplete(t *testing.T) {
	h := newHarness()
	runner := h.runner(newRuns(), &jobs{})
	for _, descriptor := range []struct {
		name    string
		summary string
		scope   string
		read    bool
	}{
		{StartBackupName, (StartBackup{Runner: runner}).Descriptor().Summary,
			(StartBackup{Runner: runner}).Descriptor().TokenScope, false},
		{GetBackupRunName, (GetBackupRun{Runner: runner}).Descriptor().Summary,
			(GetBackupRun{Runner: runner}).Descriptor().TokenScope, true},
		{VerifyBackupName, (VerifyBackup{Runner: runner}).Descriptor().Summary,
			(VerifyBackup{Runner: runner}).Descriptor().TokenScope, false},
	} {
		if len(descriptor.summary) < 80 {
			t.Errorf("%s has a summary of %d characters - the other two channels render it",
				descriptor.name, len(descriptor.summary))
		}
		wanted := backupManage
		if descriptor.read {
			wanted = backupRead
		}
		if descriptor.scope != wanted {
			t.Errorf("%s asks for %q, want %q", descriptor.name, descriptor.scope, wanted)
		}
	}
}
