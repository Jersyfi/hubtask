// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"reflect"
	"slices"
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

// The erasure (Art. 17, QS-19). What is under test is that every storage location is served,
// that each removal leaves the two records that stop it coming back, and that the two modes differ
// in exactly one thing: whether the person's own contributions go with them.

// erasureStore is every storage location, in memory, counting what it was asked to do.
type erasureStore struct {
	anonymised   bool
	deleted      bool
	credentials  int
	notified     int
	commentsGone int
	media        []repository.Medium
	discarded    []shared.ID
	failOn       string
	order        []string
	// runningRules is what the workspace would be left with: rules that act as the person.
	runningRules int
	intakeGone   int
	intakeFreed  int
	// contributions are the person's rows on entries, and where each entry is.
	contributions  []repository.Contribution
	commentIDsGone []shared.ID
	releasedOn     []shared.ID
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

func (e *erasureStore) CountIntake(context.Context, shared.ID) (int, error) {
	if err := e.step("count intake"); err != nil {
		return 0, err
	}
	return 2, nil
}

// holdSource is the holds in force, in placement order, as the lifecycle repository answers them.
type holdSource struct{ holds lifecycle.Holds }

func (h *holdSource) Active(context.Context) (lifecycle.Holds, error) { return h.holds, nil }

func (h *holdSource) Contributors(context.Context, []shared.ID) (map[shared.ID][]shared.ID, error) {
	return nil, nil
}

// keptStore is what each hold kept, as the last record left it.
type keptStore struct {
	recorded [][]domain.Kept
	parts    map[shared.ID][]domain.Kept
	blocked  map[shared.ID]string
	keeps    bool
}

func (k *keptStore) RecordKept(_ context.Context, requestID shared.ID, kept []domain.Kept, at time.Time) error {
	k.recorded = append(k.recorded, kept)
	if k.parts == nil {
		k.parts = map[shared.ID][]domain.Kept{}
	}
	still := map[shared.ID]bool{}
	for _, part := range kept {
		still[part.HoldID] = true
	}
	next := make([]domain.Kept, 0, len(k.parts[requestID])+len(kept))
	for _, part := range k.parts[requestID] {
		if !still[part.HoldID] && part.Pending() {
			part.ErasedAt = at
		}
		if !still[part.HoldID] {
			next = append(next, part)
		}
	}
	for _, part := range kept {
		part.RecordedAt = at
		next = append(next, part)
	}
	k.parts[requestID] = next
	return nil
}

func (k *keptStore) KeptOf(_ context.Context, ids []shared.ID) (map[shared.ID][]domain.Kept, error) {
	out := map[shared.ID][]domain.Kept{}
	for _, id := range ids {
		if parts, ok := k.parts[id]; ok {
			out[id] = parts
		}
	}
	return out, nil
}

func (k *keptStore) PendingKept(_ context.Context, hold shared.ID) ([]repository.PendingKept, error) {
	var out []repository.PendingKept
	for request, parts := range k.parts {
		for _, part := range parts {
			if part.Pending() && (hold.IsZero() || part.HoldID == hold) {
				out = append(out, repository.PendingKept{RequestID: request, HoldID: part.HoldID})
			}
		}
	}
	return out, nil
}

func (k *keptStore) BlockKept(_ context.Context, requestID shared.ID, code string, _ map[string]string) error {
	if k.blocked == nil {
		k.blocked = map[shared.ID]string{}
	}
	k.blocked[requestID] = code
	return nil
}

func (k *keptStore) KeepsAccount(context.Context, shared.ID) (bool, error) { return k.keeps, nil }
func (e *erasureStore) Contributions(context.Context, shared.ID) ([]repository.Contribution, error) {
	if err := e.step("contributions"); err != nil {
		return nil, err
	}
	return e.contributions, nil
}

func (e *erasureStore) DeleteComments(_ context.Context, _ shared.ID, ids []shared.ID) (int, error) {
	if err := e.step("delete comments"); err != nil {
		return 0, err
	}
	e.commentsGone = len(ids)
	e.commentIDsGone = append(e.commentIDsGone, ids...)
	return len(ids), nil
}

func (e *erasureStore) ReleaseAssignmentsOn(
	_ context.Context, _ shared.ID, ids []shared.ID, _ time.Time,
) (int, error) {
	if err := e.step("assignments"); err != nil {
		return 0, err
	}
	e.releasedOn = append(e.releasedOn, ids...)
	return len(ids), nil
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
	holds      *holdSource
	kept       *keptStore
	subjects   *subjectStore
}

// Where the person's rows are: two comments on one task, and three tasks assigned to them, all in
// one hub's collection.
var (
	erasureHub        = shared.MustParseID("0192f000-0000-7000-8000-0000000000c7")
	erasureCollection = shared.MustParseID("0192f000-0000-7000-8000-0000000000c8")
	erasureTasks      = []shared.ID{
		shared.MustParseID("0192f000-0000-7000-8000-0000000000f7"),
		shared.MustParseID("0192f000-0000-7000-8000-0000000000f8"),
		shared.MustParseID("0192f000-0000-7000-8000-0000000000f9"),
	}
	erasureComments = []shared.ID{
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e1"),
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e2"),
	}
)

func erasureRows() []repository.Contribution {
	at := func(kind repository.ContributionKind, id, item shared.ID) repository.Contribution {
		return repository.Contribution{
			Kind: kind, ID: id, ItemID: item, Path: "/" + item.String() + "/",
			CollectionID: erasureCollection, HubID: erasureHub, ItemCreatedBy: accountID,
		}
	}
	rows := []repository.Contribution{
		at(repository.ContributedComment, erasureComments[0], erasureTasks[0]),
		at(repository.ContributedComment, erasureComments[1], erasureTasks[0]),
	}
	for _, task := range erasureTasks {
		rows = append(rows, at(repository.ContributedAssignment, task, task))
	}
	return rows
}

func newErasureHarness() *erasureHarness {
	return &erasureHarness{
		storage: &erasureStore{
			credentials: 2, notified: 5, contributions: erasureRows(),
			media: []repository.Medium{
				{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e3"), StorageKey: "media/ab/cd"},
			},
		},
		removals: &removalStore{}, objects: &objectStore{},
		pseudonyms: &pseudonymStore{}, audit: &auditSink{},
		holds: &holdSource{}, kept: &keptStore{}, subjects: newSubjectStore(),
	}
}

func (h *erasureHarness) eraser() Eraser { return h.eraserFor(newRequestStore()) }

func (h *erasureHarness) eraserFor(requests *requestStore) Eraser {
	return Eraser{
		Requests: requests, Erasure: h.storage, Pseudonyms: h.pseudonyms,
		Holds: h.holds, Kept: h.kept, Subjects: h.subjects,
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

	// The credentials are the first thing that goes - after the reading that decides what is kept -
	// so nothing may act as the person half way through an erasure.
	writes := slices.DeleteFunc(slices.Clone(h.storage.order), func(step string) bool {
		return step == "contributions" || step == "automations" || step == "count intake"
	})
	if writes[0] != "credentials" {
		t.Errorf("the erasure began removing with %q", writes[0])
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

type hubsLeft struct {
	held  []shared.ID
	steps *[]string
	named [][]shared.ID
}

func (l *hubsLeft) HeldBy(context.Context, shared.ID) ([]shared.ID, error) {
	*l.steps = append(*l.steps, "held by")
	return l.held, nil
}

func (l *hubsLeft) AfterMemberLeft(_ context.Context, _ shared.ID, named []shared.ID) (int, error) {
	*l.steps = append(*l.steps, "after member left")
	l.named = append(l.named, named)
	return len(named), nil
}

// UC-ID-16 check 6: the private hubs the person was a member of are read before the account goes -
// a deletion takes the memberships with it - and asked about after it, in both modes.
func TestAnErasureAsksAfterThePrivateHubsThePersonLeft(t *testing.T) {
	hub := shared.MustParseID("0192f000-0000-7000-8000-0000000000a9")
	for _, mode := range []domain.ErasureMode{domain.ModeFullDelete, domain.ModeAnonymize} {
		t.Run(string(mode), func(t *testing.T) {
			h := newErasureHarness()
			last := &hubsLeft{held: []shared.ID{hub}, steps: &h.storage.order}
			eraser := h.eraser()
			eraser.LastMember = last

			if _, err := eraser.Erase(context.Background(), actor(), erasureCase(mode)); err != nil {
				t.Fatalf("erasing: %v", err)
			}
			if len(last.named) != 1 || !slices.Equal(last.named[0], []shared.ID{hub}) {
				t.Fatalf("asked about %v, want the hub the person held", last.named)
			}
			held := slices.Index(h.storage.order, "held by")
			after := slices.Index(h.storage.order, "after member left")
			account := slices.IndexFunc(h.storage.order, func(step string) bool {
				return step == "delete" || step == "anonymise"
			})
			if held < 0 || account < 0 || held > account || after < account {
				t.Errorf("the steps ran %v, want the hubs read before the account and asked after", h.storage.order)
			}
		})
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
	for _, step := range []string{
		"contributions", "credentials", "notifications", "assignments", "delete comments", "media",
	} {
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
// a dependency error - which is what PG-2 found.
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

var erasureHoldID = shared.MustParseID("0192f000-0000-7000-8000-0000000003a1")

// A hold on the hub keeps the person's comments and assignments there; the account is anonymised
// as always, but not deleted, because the kept comments still name it (UC-PRV-03 checks 3, 6, 10;
// data-protection.md §4.1). The case is locked first, and what was kept is recorded and audited.
func TestAHoldOnAHubKeepsWhatIsInItAndTheAccountIsAnonymised(t *testing.T) {
	h := newErasureHarness()
	h.holds.holds = lifecycle.Holds{{ID: erasureHoldID, Scope: lifecycle.HoldContainer, ScopeID: erasureHub}}
	requests := newRequestStore()

	erased, err := h.eraserFor(requests).Erase(context.Background(), actor(), erasureCase(domain.ModeFullDelete))
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}

	if requests.locked != 1 {
		t.Errorf("the case was locked %d times, want once", requests.locked)
	}
	if erased.Comments != 0 || h.storage.commentsGone != 0 || erased.Assignments != 0 {
		t.Errorf("the erasure removed what the hold keeps: %+v", erased)
	}
	if !erased.AccountAnonymised || erased.AccountRemoved || h.storage.deleted {
		t.Errorf("the account was not anonymised in place: %+v", erased)
	}
	if h.pseudonyms.assigned[subjectID] == "" {
		t.Error("the anonymised account left no pseudonym")
	}
	if erased.Credentials != 2 || erased.Notifications != 5 {
		t.Errorf("credentials and notifications are no evidence and go: %+v", erased)
	}
	want := []domain.Kept{{HoldID: erasureHoldID, HoldScope: lifecycle.HoldContainer, HoldScopeID: erasureHub,
		Comments: 2, Assignments: 3}}
	if len(h.kept.recorded) != 1 || !reflect.DeepEqual(h.kept.recorded[0], want) {
		t.Errorf("recorded %+v, want %+v", h.kept.recorded, want)
	}

	entry := h.audit.entries[0]
	for field, value := range map[string]any{
		"kept_holds": erasureHoldID.String(), "kept_comments": 2, "kept_assignments": 3,
		"kept_legal_basis": domain.KeptLegalBasis, "account": "anonymised",
	} {
		if got := entry.Changes[field].(map[string]any)["to"]; got != value {
			t.Errorf("the entry records %s = %v, want %v", field, got, value)
		}
	}
}

// A hold on the person keeps the account itself: not anonymised, not deleted, no pseudonym, and
// restricted (UC-PRV-03 check 13) - its credentials still go, its sign-in stays.
func TestAHoldOnThePersonKeepsTheAccountAndRestrictsIt(t *testing.T) {
	h := newErasureHarness()
	h.holds.holds = lifecycle.Holds{{ID: erasureHoldID, Scope: lifecycle.HoldAccount, ScopeID: subjectID}}

	erased, err := h.eraser().Erase(context.Background(), actor(), erasureCase(domain.ModeFullDelete))
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if !erased.AccountKept || erased.AccountAnonymised || erased.AccountRemoved {
		t.Errorf("the account's end is %+v, want kept", erased)
	}
	if h.storage.anonymised || h.storage.deleted {
		t.Error("the held account was anonymised or deleted")
	}
	if h.subjects.statuses[subjectID] != "RESTRICTED" {
		t.Errorf("the held account is %q, want RESTRICTED", h.subjects.statuses[subjectID])
	}
	if _, assigned := h.pseudonyms.assigned[subjectID]; assigned {
		t.Error("a pseudonym replaced the name of an account a hold keeps")
	}
	if erased.Credentials != 2 {
		t.Errorf("%d credentials were revoked, want every one", erased.Credentials)
	}
	if !h.kept.recorded[0][0].Account {
		t.Error("the record does not say the account was kept")
	}
}

// A case of an account a hold keeps cannot be lifted out of its restriction: the held data would
// become processable again.
func TestTheRestrictionOfAnAccountAHoldKeepsCannotBeLifted(t *testing.T) {
	subjects := newSubjectStore()
	restriction := newRestriction(subjects, &authorizerDouble{}, &auditSink{})
	restriction.Kept = &keptStore{keeps: true}

	err := restriction.Execute(context.Background(), actor(), RestrictCommand{
		AccountID: subjectID, Restricted: false, Reason: "Settled",
	})
	if shared.AsError(err).DetailCode != domain.CodeRestrictionKeptByErasure {
		t.Fatalf("lifting reported %v, want %s", err, domain.CodeRestrictionKeptByErasure)
	}
	if _, written := subjects.statuses[subjectID]; written {
		t.Error("the restriction was lifted anyway")
	}

	// Placing one is never refused for it.
	if err := restriction.Execute(context.Background(), actor(), RestrictCommand{
		AccountID: subjectID, Restricted: true, Reason: "Art. 18",
	}); err != nil {
		t.Fatalf("restricting: %v", err)
	}
}
