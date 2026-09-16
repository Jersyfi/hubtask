// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	backupdomain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// External anchoring (A-2, P-13): the configuration, the daily job, and the read-back.

// memoryStore is a backup target in memory: what the job wrote, readable back and rewritable by
// a test that plays the attacker.
type memoryStore struct {
	objects map[string][]byte
	puts    int
	refuse  error
}

func newMemoryStore() *memoryStore { return &memoryStore{objects: map[string][]byte{}} }

func (s *memoryStore) Put(_ context.Context, key string, content io.Reader) (int64, error) {
	if s.refuse != nil {
		return 0, s.refuse
	}
	raw, err := io.ReadAll(content)
	if err != nil {
		return 0, err
	}
	s.objects[key] = raw
	s.puts++
	return int64(len(raw)), nil
}

func (s *memoryStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if s.refuse != nil {
		return nil, s.refuse
	}
	raw, found := s.objects[key]
	if !found {
		return nil, shared.ErrNotFound.WithDetail(backupstorage.CodeObjectNotFound)
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

func (s *memoryStore) List(context.Context, string) ([]backupstorage.Entry, error) { return nil, nil }
func (s *memoryStore) Stat(context.Context, string) (backupstorage.Entry, error) {
	return backupstorage.Entry{}, nil
}
func (s *memoryStore) Delete(context.Context, string) error { return nil }

type storeOpener struct {
	store  *memoryStore
	opened []shared.ID
	refuse error
}

func (o *storeOpener) OpenTarget(_ context.Context, _, targetID shared.ID) (backupstorage.Store, error) {
	o.opened = append(o.opened, targetID)
	if o.refuse != nil {
		return nil, o.refuse
	}
	return o.store, nil
}

type workspaceStore struct {
	row     identity.Workspace
	updates int
}

func (w *workspaceStore) Find(context.Context) (identity.Workspace, error) { return w.row, nil }
func (w *workspaceStore) Update(_ context.Context, changed identity.Workspace, expected int, now time.Time) (bool, error) {
	if expected != w.row.Version {
		return false, nil
	}
	changed.Version = expected + 1
	changed.UpdatedAt = now
	w.row = changed
	w.updates++
	return true, nil
}

type targetFinder struct {
	targets map[shared.ID]backupdomain.Target
}

func (t targetFinder) Find(_ context.Context, id shared.ID) (backupdomain.Target, error) {
	target, found := t.targets[id]
	if !found {
		return backupdomain.Target{}, shared.ErrNotFound.WithDetail("backup.target_not_found")
	}
	return target, nil
}

type jobQueue struct{ queued []queue.Request }

func (q *jobQueue) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	q.queued = append(q.queued, request)
	return shared.MustParseID("0192f000-0000-7000-8000-0000000000e1"), nil
}

type anchoringHarness struct {
	anchoring  Anchoring
	trail      *trailStore
	store      *memoryStore
	opener     *storeOpener
	workspaces *workspaceStore
	jobs       *jobQueue
	sink       *auditSink
	authorizer *authorizerDouble
	clock      *movingClock
}

// movingClock is a clock a test moves by hand.
type movingClock struct{ at time.Time }

func (c *movingClock) Now() time.Time { return c.at }

func newAnchoringHarness(records []repository.Record) *anchoringHarness {
	h := &anchoringHarness{
		trail: &trailStore{records: records}, store: newMemoryStore(),
		workspaces: &workspaceStore{row: identity.Workspace{Tenant: identity.Tenant{ID: tenantID}, Version: 3}},
		jobs:       &jobQueue{}, sink: &auditSink{}, authorizer: &authorizerDouble{permits: true},
		clock: &movingClock{at: now},
	}
	h.opener = &storeOpener{store: h.store}
	h.anchoring = Anchoring{
		Workspaces: h.workspaces,
		Targets:    targetFinder{targets: map[shared.ID]backupdomain.Target{targetID: {ID: targetID, TenantID: tenantID, Enabled: true}}},
		Trail:      h.trail, Anchors: h.trail, Stores: h.opener, Jobs: h.jobs,
		Authorizer: h.authorizer, Audit: h.sink, UnitOfWork: &unitOfWork{}, Clock: h.clock,
		ProductVersion: "0.9.0-test",
	}
	return h
}

func (h *anchoringHarness) configure(t *testing.T, target shared.ID) Configuration {
	t.Helper()
	configured, err := (ConfigureAuditAnchoring{Anchoring: h.anchoring}).Execute(context.Background(), actor(), target)
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	return configured
}

func (h *anchoringHarness) verify(t *testing.T, anchors bool) Verification {
	t.Helper()
	verify := VerifyAuditChain{
		Trail: h.trail, Chain: chainDouble{}, Authorizer: h.authorizer, Audit: h.sink,
		UnitOfWork: &unitOfWork{}, Clock: h.clock, Anchoring: &h.anchoring,
	}
	found, err := verify.Check(context.Background(), actor(), VerifyRequest{
		Period: repository.Period{From: now.Add(-time.Hour), To: now.Add(48 * time.Hour)}, Anchors: anchors,
	})
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	return found
}

// Configuring names the target, is audited with before and after, and seeds the job; naming none
// switches anchoring off.
func TestConfiguringAnchoringIsAuditedAndSeedsTheJob(t *testing.T) {
	h := newAnchoringHarness(chainOf(3))

	configured := h.configure(t, targetID)
	if configured.TargetID != targetID || !configured.ConfiguredAt.Equal(now) || configured.ConfiguredBy != accountID {
		t.Errorf("configured %+v", configured)
	}
	if h.workspaces.row.Settings.AuditAnchorTargetID != targetID || h.workspaces.updates != 1 {
		t.Errorf("the setting is %v after %d updates", h.workspaces.row.Settings.AuditAnchorTargetID, h.workspaces.updates)
	}
	if len(h.jobs.queued) != 1 || h.jobs.queued[0].Kind != queue.KindAuditAnchor || h.jobs.queued[0].DedupeKey != tenantID.String() {
		t.Errorf("queued %+v", h.jobs.queued)
	}
	if len(h.sink.entries) != 1 || h.sink.entries[0].Action != AnchoringConfiguredAction {
		t.Fatalf("audited %+v", h.sink.entries)
	}
	change, _ := h.sink.entries[0].Changes["target_id"].(map[string]any)
	if change["from"] != "" || change["to"] != targetID.String() {
		t.Errorf("the entry says %v", change)
	}
	if request := h.authorizer.requests[0]; request.Permission != "STRUCTURE" && string(request.Permission) != "STRUCTURE" {
		t.Errorf("asked for %v, want STRUCTURE", request.Permission)
	}

	// Off again: audited from the target to none, and no job seeded.
	h.configure(t, shared.ID(""))
	if !h.workspaces.row.Settings.AuditAnchorTargetID.IsZero() || len(h.jobs.queued) != 1 {
		t.Errorf("switching off left %v and queued %d", h.workspaces.row.Settings.AuditAnchorTargetID, len(h.jobs.queued))
	}
	change, _ = h.sink.entries[1].Changes["target_id"].(map[string]any)
	if change["from"] != targetID.String() || change["to"] != "" {
		t.Errorf("the second entry says %v", change)
	}
	// The same setting again writes nothing.
	h.configure(t, shared.ID(""))
	if len(h.sink.entries) != 2 || h.workspaces.updates != 2 {
		t.Error("an unchanged configuration was written and audited")
	}
}

func TestConfiguringAnchoringRefusesATargetTheWorkspaceDoesNotHave(t *testing.T) {
	h := newAnchoringHarness(chainOf(1))
	_, err := (ConfigureAuditAnchoring{Anchoring: h.anchoring}).Execute(context.Background(), actor(), colleagueID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the answer was %v", err)
	}
	if h.workspaces.updates != 0 || len(h.jobs.queued) != 0 {
		t.Error("a refused configuration wrote something")
	}
}

func TestTheDescriptorTakesATargetOrNone(t *testing.T) {
	h := newAnchoringHarness(chainOf(1))
	out, err := (ConfigureAuditAnchoring{Anchoring: h.anchoring}).invoke(context.Background(), actor(),
		usecase.Input{"target_id": targetID.String()})
	if err != nil || out["target_id"] != targetID.String() {
		t.Fatalf("invoking: %v %v", out, err)
	}
	out, err = (ConfigureAuditAnchoring{Anchoring: h.anchoring}).invoke(context.Background(), actor(), usecase.Input{})
	if err != nil || out["target_id"] != nil {
		t.Fatalf("switching off: %v %v", out, err)
	}
}

// With the clock controlled: one file a day and one row, the next round the same day writes
// nothing, the next day writes the next anchor only where the chain moved.
func TestTheJobWritesOneFileADayAndOneRow(t *testing.T) {
	h := newAnchoringHarness(chainOf(5))
	h.configure(t, targetID)

	outcome, err := h.anchoring.Run(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("the first round: %v", err)
	}
	if !outcome.Anchored || outcome.LastSeq != 5 || outcome.NextDue.IsZero() {
		t.Errorf("outcome = %+v", outcome)
	}
	key := AnchorKey(tenantID, now)
	if !strings.HasPrefix(key, "hubtask-anchor-"+tenantID.String()+"-20260826") {
		t.Errorf("the key is %s", key)
	}
	written, held := h.store.objects[key]
	if !held || !strings.Contains(string(written), `"last_seq":5`) || !strings.Contains(string(written), `"product_version":"0.9.0-test"`) {
		t.Errorf("the file holds %s", written)
	}
	if len(h.trail.anchors) != 1 || h.trail.anchors[0].LastSeq != 5 || h.trail.anchors[0].Destination != targetID.String() ||
		h.trail.anchors[0].Receipt != digestOf(written) || !h.trail.anchors[0].AnchoredAt.Equal(now) {
		t.Errorf("the row is %+v", h.trail.anchors)
	}

	// The same day again: nothing new, whatever the chain did.
	h.trail.records = chainOf(7)
	h.clock.at = now.Add(3 * time.Hour)
	if outcome, err = h.anchoring.Run(context.Background(), tenantID); err != nil || outcome.Anchored || h.store.puts != 1 {
		t.Errorf("the second round the same day: %+v %v, %d files", outcome, err, h.store.puts)
	}

	// The next day: the chain moved, so the next anchor.
	h.clock.at = now.Add(24 * time.Hour)
	if outcome, err = h.anchoring.Run(context.Background(), tenantID); err != nil || !outcome.Anchored || outcome.LastSeq != 7 || h.store.puts != 2 {
		t.Errorf("the next day: %+v %v, %d files", outcome, err, h.store.puts)
	}
	if len(h.trail.anchors) != 2 {
		t.Errorf("%d rows", len(h.trail.anchors))
	}
	// And the day after, with nothing having happened: no file, no row.
	h.clock.at = now.Add(48 * time.Hour)
	if outcome, err = h.anchoring.Run(context.Background(), tenantID); err != nil || outcome.Anchored || h.store.puts != 2 {
		t.Errorf("a quiet day: %+v %v, %d files", outcome, err, h.store.puts)
	}
	if outcome.NextDue.Before(h.clock.at) || outcome.NextDue.Sub(h.clock.at) > 25*time.Hour {
		t.Errorf("the next round is due %s", outcome.NextDue)
	}
}

// A workspace without a target anchors nothing, and the job finishes.
func TestAWorkspaceWithoutATargetAnchorsNothing(t *testing.T) {
	h := newAnchoringHarness(chainOf(3))
	outcome, err := h.anchoring.Run(context.Background(), tenantID)
	if err != nil || outcome.Anchored || !outcome.NextDue.IsZero() || h.store.puts != 0 {
		t.Errorf("outcome = %+v %v, %d files", outcome, err, h.store.puts)
	}
	found := h.verify(t, true)
	if found.Anchor.Configured || found.Anchor.Agrees != nil || !found.SealedUntil.IsZero() {
		t.Errorf("the check says %+v", found.Anchor)
	}
	out := VerificationOutput(found)
	if out["anchoring_configured"] != false || out["anchored_until"] != nil || out["anchor_agrees"] != nil {
		t.Errorf("the answer says %v", out)
	}
}

// A target that refuses the write fails the round, and no row is recorded: the row says a copy
// exists, and there is none.
func TestATargetThatRefusesTheWriteRecordsNoAnchor(t *testing.T) {
	h := newAnchoringHarness(chainOf(3))
	h.configure(t, targetID)
	h.store.refuse = shared.ErrUnavailable.WithDetail(backupstorage.CodeTargetUnreachable)
	if _, err := h.anchoring.Run(context.Background(), tenantID); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("the answer was %v", err)
	}
	if len(h.trail.anchors) != 0 {
		t.Error("a row was recorded for a copy that was never written")
	}
}

// The verification with anchors: agreement, disagreement when the file is rewritten, a chain
// rewritten below the anchor reported at the anchor as well as at the break.
func TestVerifyingWithAnchorsReadsTheCopyBack(t *testing.T) {
	h := newAnchoringHarness(chainOf(5))
	h.configure(t, targetID)
	if _, err := h.anchoring.Run(context.Background(), tenantID); err != nil {
		t.Fatal(err)
	}

	found := h.verify(t, true)
	if !found.Valid || found.Anchor.Agrees == nil || !*found.Anchor.Agrees || found.Anchor.LastSeq != 5 ||
		!found.Anchor.AnchoredAt.Equal(now) || !found.Anchor.Configured {
		t.Errorf("an intact chain and its copy: %+v", found.Anchor)
	}
	out := VerificationOutput(found)
	if out["anchor_agrees"] != true || out["anchor_seq"] != int64(5) || out["anchoring_configured"] != true {
		t.Errorf("the answer says %v", out)
	}
	// Without asking, nothing is read.
	opened := len(h.opener.opened)
	if found := h.verify(t, false); found.Anchor.Agrees != nil || len(h.opener.opened) != opened {
		t.Errorf("a check that did not ask read the copy: %+v %v", found.Anchor, h.opener.opened)
	}

	// The attacker rewrites the file: the receipt no longer matches.
	key := AnchorKey(tenantID, now)
	original := h.store.objects[key]
	h.store.objects[key] = []byte(strings.Replace(string(original), `"last_seq":5`, `"last_seq":4`, 1))
	if found := h.verify(t, true); found.Anchor.Agrees != nil || found.Anchor.ErrorCode != CodeAnchorReceiptMismatch {
		t.Errorf("a rewritten copy: %+v", found.Anchor)
	}
	h.store.objects[key] = original

	// The attacker rewrites the chain below the anchor, recomputing every hash after it so that
	// the chain verifies inside the database: the copy disagrees.
	rewritten := chainOf(5)
	rewritten[1].Entry.Action = "container.deleted"
	var previous []byte
	for i := range rewritten {
		rewritten[i].PrevHash = previous
		rewritten[i].Hash, _ = chainDouble{}.Link(previous, rewritten[i].ID, rewritten[i].Seq, rewritten[i].Entry)
		previous = rewritten[i].Hash
	}
	h.trail.records = rewritten
	found = h.verify(t, true)
	if !found.Valid {
		t.Fatalf("the recomputed chain does not verify inside the database: %+v", found)
	}
	if found.Anchor.Agrees == nil || *found.Anchor.Agrees {
		t.Errorf("a chain rewritten and recomputed below the anchor was not reported at the anchor: %+v", found.Anchor)
	}

	// The cruder attacker rewrites one entry below the anchor and nothing else: the break is at
	// the entry, and the anchor - held against the chain as the walk derives it - disagrees too.
	crude := chainOf(5)
	crude[1].Entry.Action = "container.deleted"
	h.trail.records = crude
	found = h.verify(t, true)
	if found.Valid || found.FirstBrokenSeq != 2 {
		t.Errorf("the break was not found: %+v", found)
	}
	if found.Anchor.Agrees == nil || *found.Anchor.Agrees {
		t.Errorf("the rewrite below the anchor was not reported at the anchor: %+v", found.Anchor)
	}

	// A target that cannot be reached answers a code, not an error: the chain check stands.
	h.trail.records = chainOf(5)
	h.opener.refuse = shared.ErrUnavailable.WithDetail(backupstorage.CodeTargetUnreachable)
	found = h.verify(t, true)
	if !found.Valid || found.Anchor.ErrorCode != CodeAnchorUnreadable || found.Anchor.Agrees != nil {
		t.Errorf("an unreachable target: %+v", found.Anchor)
	}
}

// An anchor outside the walked period is held against the stored hash at its sequence number.
func TestAnAnchorOutsideThePeriodIsHeldAgainstTheStoredHash(t *testing.T) {
	h := newAnchoringHarness(chainOf(5))
	h.configure(t, targetID)
	if _, err := h.anchoring.Run(context.Background(), tenantID); err != nil {
		t.Fatal(err)
	}
	verify := VerifyAuditChain{
		Trail: h.trail, Chain: chainDouble{}, Authorizer: h.authorizer, Audit: h.sink,
		UnitOfWork: &unitOfWork{}, Clock: h.clock, Anchoring: &h.anchoring,
	}
	// The trail double walks everything whatever the period; an empty record list is the walk
	// that never reaches the anchor.
	h.trail.records = nil
	h.trail.records = append(h.trail.records, chainOf(5)...)
	walked := &trailStore{records: nil, anchor: h.trail.anchor}
	walked.records = nil
	verify.Trail = &periodlessTrail{trailStore: h.trail}
	found, err := verify.Check(context.Background(), actor(), VerifyRequest{
		Period: repository.Period{From: now.Add(-2 * time.Hour), To: now.Add(-time.Hour)}, Anchors: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if found.Anchor.Agrees == nil || !*found.Anchor.Agrees {
		t.Errorf("the anchor outside the period: %+v", found.Anchor)
	}
}

// periodlessTrail walks nothing, so that a check reaches for the stored hash.
type periodlessTrail struct{ *trailStore }

func (p *periodlessTrail) Walk(context.Context, repository.Period, func(repository.Record) error) error {
	return nil
}

var _ clock.Clock = (*movingClock)(nil)
