// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

// The erasure (E-10, Art. 17, QS-19). What is under test is that every storage location is served,
// that each removal leaves the two records that stop it coming back, and that the two modes differ
// in exactly one thing: whether the person's own contributions go with them.

// erasureStore is every storage location, in memory, counting what it was asked to do.
type erasureStore struct {
	anonymised   bool
	deleted      bool
	credentials  int
	notified     int
	assignments  int
	comments     []repository.Authored
	commentsGone int
	media        []repository.Medium
	discarded    []shared.ID
	failOn       string
	order        []string
	// runningRules is what the workspace would be left with: rules that act as the person.
	runningRules int
	intakeGone   int
	intakeFreed  int
}

func (e *erasureStore) DiscardIntake(context.Context, shared.ID) (int, error) {
	if err := e.step("discard intake"); err != nil {
		return 0, err
	}
	e.intakeGone = 2
	return e.intakeGone, nil
}

func (e *erasureStore) ReleaseIntake(context.Context, shared.ID) (int, error) {
	if err := e.step("release intake"); err != nil {
		return 0, err
	}
	e.intakeFreed = 2
	return e.intakeFreed, nil
}

func (e *erasureStore) AutomationsRunningAs(context.Context, shared.ID) (int, error) {
	if err := e.step("automations"); err != nil {
		return 0, err
	}
	return e.runningRules, nil
}

func (e *erasureStore) step(name string) error {
	e.order = append(e.order, name)
	if e.failOn == name {
		return errors.New("the storage location refused")
	}
	return nil
}

func (e *erasureStore) Anonymise(_ context.Context, _ shared.ID, marker string, _ time.Time) (bool, error) {
	if err := e.step("anonymise"); err != nil {
		return false, err
	}
	e.anonymised = marker == FormerUser
	return true, nil
}

func (e *erasureStore) Delete(context.Context, shared.ID) (bool, error) {
	if err := e.step("delete"); err != nil {
		return false, err
	}
	e.deleted = true
	return true, nil
}

func (e *erasureStore) RevokeCredentials(context.Context, shared.ID) (int, error) {
	if err := e.step("credentials"); err != nil {
		return 0, err
	}
	return e.credentials, nil
}

func (e *erasureStore) DiscardNotifications(context.Context, shared.ID) (int, error) {
	if err := e.step("notifications"); err != nil {
		return 0, err
	}
	return e.notified, nil
}

func (e *erasureStore) ReleaseAssignments(context.Context, shared.ID, time.Time) (int, error) {
	if err := e.step("assignments"); err != nil {
		return 0, err
	}
	return e.assignments, nil
}

func (e *erasureStore) AuthoredComments(context.Context, shared.ID) ([]repository.Authored, error) {
	if err := e.step("read comments"); err != nil {
		return nil, err
	}
	return e.comments, nil
}

func (e *erasureStore) DeleteAuthoredComments(context.Context, shared.ID) (int, error) {
	if err := e.step("delete comments"); err != nil {
		return 0, err
	}
	e.commentsGone = len(e.comments)
	return len(e.comments), nil
}

func (e *erasureStore) OrphanedMedia(context.Context, shared.ID) ([]repository.Medium, error) {
	if err := e.step("media"); err != nil {
		return nil, err
	}
	return e.media, nil
}

func (e *erasureStore) DiscardMedium(_ context.Context, mediaID shared.ID) error {
	if err := e.step("discard medium"); err != nil {
		return err
	}
	e.discarded = append(e.discarded, mediaID)
	return nil
}

// removalStore is the one engine every removal goes through.
type removalStore struct {
	recorded []lifecycle.Removal
	purge    []time.Time
}

func (r *removalStore) Record(
	_ context.Context, removals []lifecycle.Removal, _ time.Time, purgeAfter time.Time,
) error {
	r.recorded = append(r.recorded, removals...)
	r.purge = append(r.purge, purgeAfter)
	return nil
}

// objectStore is the bucket. It answers what it was asked to remove, and can refuse.
type objectStore struct {
	deleted []string
	refuse  bool
}

func (o *objectStore) Put(context.Context, storage.Upload) error { return nil }

func (o *objectStore) Get(context.Context, string) (storage.Object, error) {
	return storage.Object{}, shared.ErrNotFound
}

func (o *objectStore) Delete(_ context.Context, key string) error {
	if o.refuse {
		return shared.ErrUnavailable.WithDetail("storage.unavailable")
	}
	o.deleted = append(o.deleted, key)
	return nil
}

type pseudonymStore struct{ assigned map[shared.ID]string }

func (p *pseudonymStore) Assign(
	_ context.Context, actorID shared.ID, pseudonym, _ string, _ time.Time,
) error {
	if p.assigned == nil {
		p.assigned = map[shared.ID]string{}
	}
	if _, already := p.assigned[actorID]; !already {
		p.assigned[actorID] = pseudonym
	}
	return nil
}

func (p *pseudonymStore) For(
	_ context.Context, actorIDs []shared.ID,
) (map[shared.ID]string, error) {
	out := map[shared.ID]string{}
	for _, actorID := range actorIDs {
		if name, found := p.assigned[actorID]; found {
			out[actorID] = name
		}
	}
	return out, nil
}

type erasureHarness struct {
	storage    *erasureStore
	removals   *removalStore
	objects    *objectStore
	pseudonyms *pseudonymStore
	audit      *auditSink
}

func newErasureHarness() *erasureHarness {
	return &erasureHarness{
		storage: &erasureStore{
			credentials: 2, notified: 5, assignments: 3,
			comments: []repository.Authored{
				{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e1"), ItemID: subjectID},
				{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e2"), ItemID: subjectID},
			},
			media: []repository.Medium{
				{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e3"), StorageKey: "media/ab/cd"},
			},
		},
		removals: &removalStore{}, objects: &objectStore{},
		pseudonyms: &pseudonymStore{}, audit: &auditSink{},
	}
}

func (h *erasureHarness) eraser() Eraser {
	return Eraser{
		Requests: newRequestStore(), Erasure: h.storage, Pseudonyms: h.pseudonyms,
		Removals: h.removals, Objects: h.objects, Audit: h.audit,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
		TombstoneWindow: 90 * 24 * time.Hour,
	}
}

func erasureCase(mode domain.ErasureMode) domain.Request {
	return domain.Request{
		ID:   shared.MustParseID("0192f000-0000-7000-8000-0000000000d1"),
		Kind: domain.KindErasure, Status: domain.StatusInProgress,
		SubjectAccountID: subjectID, ErasureMode: mode,
	}
}

// A full deletion serves every location, and takes the person's own contributions with them.
func TestAFullDeletionServesEveryStorageLocation(t *testing.T) {
	h := newErasureHarness()

	erased, err := h.eraser().Erase(context.Background(), actor(), erasureCase(domain.ModeFullDelete))
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}

	if erased.Credentials != 2 || erased.Notifications != 5 || erased.Assignments != 3 {
		t.Errorf("the erasure did %+v", erased)
	}
	if erased.Comments != 2 || erased.Media != 1 || !erased.AccountRemoved {
		t.Errorf("the erasure did %+v", erased)
	}
	if h.storage.anonymised {
		t.Error("a full deletion anonymised the account instead of removing it")
	}

	// The credentials go first: nothing may act as the person half way through an erasure.
	if h.storage.order[0] != "credentials" {
		t.Errorf("the erasure began with %q", h.storage.order[0])
	}

	// Every removal leaves the two records that stop it coming back (ADR-0020 §6).
	entities := map[string]int{}
	for _, removal := range h.removals.recorded {
		entities[removal.Entity]++
		if removal.Reason != lifecycle.DeletedByErasure {
			t.Errorf("a removal was recorded as %s", removal.Reason)
		}
	}
	if entities["comment"] != 2 || entities["account"] != 1 || entities["media_object"] != 1 {
		t.Errorf("the journal holds %v", entities)
	}
	// And the marker outlives the removal by the offline window.
	if !h.removals.purge[0].Equal(now.Add(90 * 24 * time.Hour)) {
		t.Errorf("the tombstone is purged at %s", h.removals.purge[0])
	}

	// The bytes are gone as well as the row.
	if len(h.objects.deleted) != 1 || h.objects.deleted[0] != "media/ab/cd" {
		t.Errorf("the object store was asked for %v", h.objects.deleted)
	}
}

// Anonymisation keeps the workspace's content - which belongs to third parties as much as to the
// person - and everything of the person's in the account goes.
func TestAnAnonymisationKeepsTheContributionsAndTheAccountRow(t *testing.T) {
	h := newErasureHarness()

	erased, err := h.eraser().Erase(context.Background(), actor(), erasureCase(domain.ModeAnonymize))
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}

	if !erased.AccountAnonymised || erased.AccountRemoved {
		t.Errorf("the erasure did %+v", erased)
	}
	if erased.Comments != 0 || h.storage.commentsGone != 0 {
		t.Error("an anonymisation removed the person's contributions")
	}
	if !h.storage.anonymised {
		t.Errorf("the account was not anonymised with the marker %q", FormerUser)
	}
	// The credentials still go: an anonymised account keeps its row and must not keep a token that
	// still works.
	if erased.Credentials != 2 {
		t.Errorf("%d credentials were revoked", erased.Credentials)
	}
}

// The trail is exempt from erasure and pseudonymises instead (audit.md §6). The mapping is written
// in the same transaction, because one written afterwards is a window in which the trail still
// answers a name.
func TestAnErasureLeavesAPseudonymForTheTrail(t *testing.T) {
	h := newErasureHarness()

	if _, err := h.eraser().Erase(context.Background(), actor(), erasureCase(domain.ModeFullDelete)); err != nil {
		t.Fatalf("erasing: %v", err)
	}

	pseudonym := h.pseudonyms.assigned[subjectID]
	if pseudonym == "" {
		t.Fatal("the erasure left no pseudonym")
	}
	if pseudonym == "Anna Beispiel" || pseudonym == subjectID.String() {
		t.Errorf("the pseudonym is %q, which is not a pseudonym", pseudonym)
	}
	// Derived rather than random, so that a retried erasure produces the same label.
	if pseudonymFor(subjectID) != pseudonym {
		t.Error("the pseudonym is not derived from the account")
	}
}

// The erasure is recorded in the trail it does not touch, with counts rather than identifiers:
// what an auditor needs is that every location was served, which is checkable.
func TestTheErasureIsRecordedWithWhatItDid(t *testing.T) {
	h := newErasureHarness()

	if _, err := h.eraser().Erase(context.Background(), actor(), erasureCase(domain.ModeFullDelete)); err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if len(h.audit.entries) != 1 {
		t.Fatalf("%d entries were written", len(h.audit.entries))
	}

	entry := h.audit.entries[0]
	if entry.Action != ErasedAction || entry.Severity != audit.SeverityCritical {
		t.Errorf("the erasure was recorded as %s / %s", entry.Action, entry.Severity)
	}
	if entry.LegalBasis != "dsr.erasure" {
		t.Errorf("the entry names the occasion %q", entry.LegalBasis)
	}
	for _, field := range []string{"mode", "credentials", "notifications", "assignments", "comments", "media", "account"} {
		if _, present := entry.Changes[field]; !present {
			t.Errorf("the entry does not say what happened to %s: %v", field, entry.Changes)
		}
	}
	// No name anywhere in it.
	for field, value := range entry.Changes {
		if masked, ok := value.(map[string]any); ok && masked["to"] == "Anna Beispiel" {
			t.Errorf("the entry carries a name in %s", field)
		}
	}
}

// A bucket that will not release a file leaves the row alone for the reconciliation to find - the
// other order would be a row pointing at nothing, hunted for ever.
func TestAMediumTheStoreWillNotReleaseKeepsItsRow(t *testing.T) {
	h := newErasureHarness()
	h.objects.refuse = true

	erased, err := h.eraser().Erase(context.Background(), actor(), erasureCase(domain.ModeFullDelete))
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if erased.Media != 0 {
		t.Errorf("%d media were counted as removed", erased.Media)
	}
	if len(h.storage.discarded) != 0 {
		t.Error("a medium's row went while its bytes stayed")
	}
	// And the erasure itself still succeeded: failing it over one file would leave the case open
	// and the erasure half done.
	if !erased.AccountRemoved {
		t.Error("the erasure gave up over a file the bucket would not release")
	}
}

// A case about somebody this workspace does not hold is answered rather than failed: the person
// asked, and the answer is that there is nothing here of theirs.
func TestACaseWithNoAccountHereErasesNothingAndSaysSo(t *testing.T) {
	h := newErasureHarness()
	request := erasureCase(domain.ModeFullDelete)
	request.SubjectAccountID = ""

	erased, err := h.eraser().Erase(context.Background(), actor(), request)
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if erased.AccountRemoved || len(h.storage.order) != 0 {
		t.Errorf("something was erased for a case about nobody: %+v", h.storage.order)
	}
}

func TestAnErasureWithoutAModeIsRefused(t *testing.T) {
	h := newErasureHarness()
	request := erasureCase("")

	if _, err := h.eraser().Erase(context.Background(), actor(), request); err == nil {
		t.Fatal("an erasure ran without a mode")
	}
	if len(h.storage.order) != 0 {
		t.Error("an erasure with no mode touched a storage location")
	}
}

// A storage location that refuses fails the erasure rather than leaving it half done and silent -
// which is the failure risk R-09 is about.
func TestAStorageLocationThatRefusesFailsTheErasure(t *testing.T) {
	for _, step := range []string{"credentials", "notifications", "assignments", "read comments", "media"} {
		h := newErasureHarness()
		h.storage.failOn = step

		if _, err := h.eraser().Erase(context.Background(), actor(), erasureCase(domain.ModeFullDelete)); err == nil {
			t.Errorf("an erasure whose %s step refused reported success", step)
		}
		if len(h.audit.entries) != 0 {
			t.Errorf("a failed erasure was recorded as done (%s)", step)
		}
	}
}

// A full deletion is refused while a rule still acts as the person. The reference is `ON DELETE
// RESTRICT`, so the alternative to this refusal is a foreign key violation reaching the caller as
// a dependency error - which is what PG-2 found (E-11).
func TestAFullDeletionIsRefusedWhileARuleActsAsThePerson(t *testing.T) {
	harness := newErasureHarness()
	harness.storage.runningRules = 3

	_, err := harness.eraser().Erase(context.Background(), actor(), erasureCase(domain.ModeFullDelete))

	problem := shared.AsError(err)
	if problem == nil || problem.Code != shared.ErrConflict.Code {
		t.Fatalf("a deletion that would leave a rule running was not refused: %v", err)
	}
	if problem.DetailCode != domain.CodeErasureBlockedByRule {
		t.Errorf("the refusal is %q, not the code the operator can act on", problem.DetailCode)
	}
	if problem.Params["rules"] != "3" {
		t.Errorf("the refusal does not say how many rules stand in the way: %v", problem.Params)
	}
	if harness.storage.deleted {
		t.Error("the account was deleted anyway")
	}
}

// And an anonymisation is not: the row stays, so the rule keeps a reference that resolves - to an
// account which may no longer act, so the rule cannot run either way.
func TestAnAnonymisationIsNotRefusedWhileARuleActsAsThePerson(t *testing.T) {
	harness := newErasureHarness()
	harness.storage.runningRules = 3

	if _, err := harness.eraser().Erase(
		context.Background(), actor(), erasureCase(domain.ModeAnonymize),
	); err != nil {
		t.Fatalf("anonymising: %v", err)
	}
	if !harness.storage.anonymised {
		t.Error("the account was not anonymised")
	}
}

// The intake is the one location that knows the person by address rather than by account, and the
// two modes answer it differently: the message goes, or it stays and stops being anybody's.
func TestTheIntakeIsServedInBothModes(t *testing.T) {
	full := newErasureHarness()
	erased, err := full.eraser().Erase(context.Background(), actor(), erasureCase(domain.ModeFullDelete))
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if erased.Intake != 2 || full.storage.intakeGone != 2 {
		t.Errorf("a full deletion left the person's intake: %+v", erased)
	}

	kept := newErasureHarness()
	if _, err := kept.eraser().Erase(
		context.Background(), actor(), erasureCase(domain.ModeAnonymize),
	); err != nil {
		t.Fatalf("anonymising: %v", err)
	}
	if kept.storage.intakeGone != 0 || kept.storage.intakeFreed != 2 {
		t.Error("an anonymisation deleted the intake instead of taking the address off it")
	}
}
