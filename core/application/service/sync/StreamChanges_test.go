// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant   = shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	account  = shared.ID("01936f2a-7c1e-7000-8000-0000000000a2")
	readable = shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")
	hidden   = shared.ID("01936f2a-7c1e-7000-8000-0000000000b2")
	now      = time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	window   = 90 * 24 * time.Hour
)

// changeStore is the log, in memory, keyed the way the walk pages it.
type changeStore struct {
	entries []repository.Recorded
	err     error
	// reads counts the walks, so a test can say how often the log was touched.
	reads int
}

func (s *changeStore) After(_ context.Context, after int64, batch int) ([]repository.Recorded, error) {
	s.reads++
	if s.err != nil {
		return nil, s.err
	}
	var out []repository.Recorded
	for _, entry := range s.entries {
		if entry.Seq > after {
			out = append(out, entry)
		}
		if len(out) == batch {
			break
		}
	}
	return out, nil
}

func (s *changeStore) Latest(context.Context) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	if len(s.entries) == 0 {
		return 0, nil
	}
	return s.entries[len(s.entries)-1].Seq, nil
}

// containerStore answers what a container is, and counts how often it was asked.
type containerStore struct {
	lookups int
	missing map[shared.ID]bool
}

func (s *containerStore) Find(_ context.Context, id shared.ID) (work.Container, error) {
	s.lookups++
	if s.missing[id] {
		return work.Container{}, shared.ErrNotFound
	}
	return work.Container{ID: id, TenantID: tenant, Type: work.ContainerCollection}, nil
}

// authorizer permits the containers it was told to permit, and counts the questions.
type authorizer struct {
	allowed   map[shared.ID]bool
	questions int
	err       error
}

func (a *authorizer) Authorize(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) error {
	a.questions++
	if a.err != nil {
		return a.err
	}
	// The container is the last scope of the path, which is what ContainerScopes builds.
	last := request.Path[len(request.Path)-1]
	if a.allowed[last.ID] {
		return nil
	}
	return shared.ErrForbidden.WithDetail("access.not_permitted")
}

// cursors is the codec, without the cryptography: the application never looks inside the string,
// so a test does not have to either.
type cursors struct{ err error }

func (c cursors) Encode(position Position) string {
	return strconv.FormatInt(position.Seq, 10) + "@" +
		strconv.FormatInt(position.IssuedAt.Unix(), 10)
}

func (c cursors) Decode(cursor string) (Position, error) {
	if c.err != nil {
		return Position{}, c.err
	}
	seqText, issuedText, found := strings.Cut(cursor, "@")
	if !found {
		return Position{}, shared.ErrValidation.WithDetail("sync.cursor_invalid")
	}
	seq, err := strconv.ParseInt(seqText, 10, 64)
	if err != nil {
		return Position{}, shared.ErrValidation.WithDetail("sync.cursor_invalid")
	}
	issued, err := strconv.ParseInt(issuedText, 10, 64)
	if err != nil {
		return Position{}, shared.ErrValidation.WithDetail("sync.cursor_invalid")
	}
	return Position{Seq: seq, IssuedAt: time.Unix(issued, 0).UTC()}, nil
}

type unitOfWork struct{ scopes []persistence.Scope }

func (u *unitOfWork) Within(
	ctx context.Context, scope persistence.Scope, fn func(context.Context) error,
) error {
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(
	ctx context.Context, scope persistence.Scope, fn func(context.Context) error,
) error {
	return u.Within(ctx, scope, fn)
}

func actor() appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: account, AccountName: "Anna", Kind: shared.ActorUser,
	}
}

func entry(seq int64, container shared.ID) repository.Recorded {
	return repository.Recorded{
		Change: repository.Change{
			TenantID: tenant, Entity: "work_item",
			EntityID:    shared.MustParseID("01936f2a-7c1e-7000-8e00-" + pad(seq)),
			Op:          repository.Upsert,
			ContainerID: container,
		},
		Seq: seq, OccurredAt: now,
	}
}

func pad(seq int64) string {
	text := strconv.FormatInt(seq, 16)
	return strings.Repeat("0", 12-len(text)) + text
}

type fixture struct {
	stream     StreamChanges
	changes    *changeStore
	containers *containerStore
	auth       *authorizer
	work       *unitOfWork
}

func streaming(t *testing.T, entries ...repository.Recorded) fixture {
	t.Helper()

	changes := &changeStore{entries: entries}
	containers := &containerStore{missing: map[shared.ID]bool{}}
	auth := &authorizer{allowed: map[shared.ID]bool{readable: true}}
	work := &unitOfWork{}

	return fixture{
		stream: StreamChanges{
			Changes: changes, Containers: containers, Authorizer: auth, UnitOfWork: work,
			Cursors: cursors{}, Clock: clock.Fixed(now), Window: window, Batch: 3,
		},
		changes: changes, containers: containers, auth: auth, work: work,
	}
}

// A client with no cursor starts from now. The stream is an accelerator, not a history: what
// happened before the connection is a pull's business (ADR-0021).
func TestAClientWithNoCursorStartsFromNow(t *testing.T) {
	f := streaming(t, entry(1, readable), entry(2, readable))

	from, err := f.stream.Resume(t.Context(), actor(), "")
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if from.Seq != 2 {
		t.Errorf("started at %d, want the head of the log", from.Seq)
	}

	batch, err := f.stream.Next(t.Context(), actor(), from)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(batch.Records) != 0 {
		t.Errorf("a fresh client was sent %d records of history", len(batch.Records))
	}
}

func TestACursorResumesWhereTheStreamStopped(t *testing.T) {
	f := streaming(t, entry(1, readable), entry(2, readable), entry(3, readable))

	first, err := f.stream.Next(t.Context(), actor(), Position{Seq: 0, IssuedAt: now})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(first.Records) != 3 {
		t.Fatalf("%d records, want three", len(first.Records))
	}

	// Resuming from the cursor of the second record replays the third and nothing else: no gap,
	// no duplicate.
	resumed, err := f.stream.Resume(t.Context(), actor(),
		f.stream.Encode(first.Records[1].Cursor))
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}
	rest, err := f.stream.Next(t.Context(), actor(), resumed)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(rest.Records) != 1 || rest.Records[0].Seq != 3 {
		t.Errorf("resumed with %d records, want only the third", len(rest.Records))
	}
}

// The acceptance criterion: never a record for a container the caller may not read, and the
// judgement is made here rather than by trusting the subscription.
func TestARecordForAContainerTheCallerMayNotReadIsWithheld(t *testing.T) {
	f := streaming(t, entry(1, readable), entry(2, hidden), entry(3, readable))

	batch, err := f.stream.Next(t.Context(), actor(), Position{IssuedAt: now})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	if len(batch.Records) != 2 {
		t.Fatalf("%d records, want the two in the readable container", len(batch.Records))
	}
	for _, record := range batch.Records {
		if record.ContainerID == hidden {
			t.Errorf("a record for a container the caller may not read was sent: %+v", record)
		}
	}

	// The cursor still advances past the withheld record. One that stalled on a container
	// somebody lost access to would re-read it on every round forever.
	if batch.Cursor.Seq != 3 {
		t.Errorf("the cursor stands at %d, want past everything that was read", batch.Cursor.Seq)
	}
}

// A change naming no container is one whose visibility nothing here can decide, and the safe
// answer is the one that does not leak it.
func TestARecordWithNoContainerIsWithheld(t *testing.T) {
	f := streaming(t, entry(1, ""))

	batch, err := f.stream.Next(t.Context(), actor(), Position{IssuedAt: now})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(batch.Records) != 0 {
		t.Errorf("a record with no container was sent: %+v", batch.Records)
	}
	if f.auth.questions != 0 {
		t.Error("a container that does not exist was put to the authorizer")
	}
}

// A container that has been purged takes its records with it: a client cannot be told about a
// change in something it can no longer be shown.
func TestRecordsOfAContainerThatIsGoneAreWithheld(t *testing.T) {
	f := streaming(t, entry(1, readable))
	f.containers.missing[readable] = true

	batch, err := f.stream.Next(t.Context(), actor(), Position{IssuedAt: now})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(batch.Records) != 0 {
		t.Errorf("a record of a container that is gone was sent: %+v", batch.Records)
	}
}

// One decision per container rather than one per record. Every record is still judged; what is
// remembered is the answer to the question it asks.
func TestTheSameContainerIsJudgedOncePerBatch(t *testing.T) {
	f := streaming(t, entry(1, readable), entry(2, readable), entry(3, readable))

	if _, err := f.stream.Next(t.Context(), actor(), Position{IssuedAt: now}); err != nil {
		t.Fatalf("reading: %v", err)
	}

	if f.auth.questions != 1 {
		t.Errorf("%d authorisation questions for one container, want 1", f.auth.questions)
	}
	if f.containers.lookups != 1 {
		t.Errorf("%d container lookups for one container, want 1", f.containers.lookups)
	}
}

// A blip in the authorisation path is not a refusal. Reporting it as "may not see" would silently
// shorten the stream on a database hiccup, which is a loss nothing would report.
func TestAnUnansweredAuthorisationQuestionIsNotARefusal(t *testing.T) {
	f := streaming(t, entry(1, readable))
	f.auth.err = shared.ErrUnavailable.WithDetail("postgres.query_failed")

	if _, err := f.stream.Next(t.Context(), actor(), Position{IssuedAt: now}); err == nil {
		t.Fatal("a failure to answer was treated as a refusal")
	}
}

// The only safe answer to a cursor older than the offline window: the log no longer holds
// everything that happened, and a delta across the gap would silently omit what was pruned
// (offline-sync.md §7).
func TestACursorOlderThanTheOfflineWindowIsRefused(t *testing.T) {
	f := streaming(t)

	stale := f.stream.Encode(Position{Seq: 7, IssuedAt: now.Add(-window - time.Hour)})
	_, err := f.stream.Resume(t.Context(), actor(), stale)
	if err == nil {
		t.Fatal("a cursor older than the window was accepted")
	}
	if !errors.Is(err, shared.ErrGone) {
		t.Errorf("a stale cursor reported %v, want gone", err)
	}
	if got := shared.AsError(err).DetailCode; got != "sync.cursor_too_old" {
		t.Errorf("detail %q", got)
	}
	if params := shared.AsError(err).Params; params["window_days"] != "90" {
		t.Errorf("the refusal does not say how far back the log reaches: %v", params)
	}

	// And one inside the window is fine, so the boundary is the subject rather than the refusal.
	fresh := f.stream.Encode(Position{Seq: 7, IssuedAt: now.Add(-window + time.Hour)})
	if _, err := f.stream.Resume(t.Context(), actor(), fresh); err != nil {
		t.Errorf("a cursor inside the window was refused: %v", err)
	}
}

func TestACursorThisInstallationDidNotMintIsRefused(t *testing.T) {
	f := streaming(t)
	f.stream.Cursors = cursors{err: shared.ErrValidation.WithDetail("sync.cursor_invalid")}

	if _, err := f.stream.Resume(t.Context(), actor(), "forged"); err == nil {
		t.Fatal("a forged cursor was accepted")
	} else if got := shared.AsError(err).DetailCode; got != "sync.cursor_invalid" {
		t.Errorf("detail %q", got)
	}
}

// A full batch means the log has more: the caller comes straight back rather than waiting to be
// woken.
func TestAFullBatchSaysThereIsMore(t *testing.T) {
	f := streaming(t, entry(1, readable), entry(2, readable), entry(3, readable),
		entry(4, readable))

	batch, err := f.stream.Next(t.Context(), actor(), Position{IssuedAt: now})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if !batch.More {
		t.Error("a full batch did not say there was more")
	}

	rest, err := f.stream.Next(t.Context(), actor(), batch.Cursor)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if rest.More {
		t.Error("a short batch said there was more")
	}
}

// Every read is bound to the tenant the actor belongs to (ADR-0010, rule 3).
func TestEveryReadIsBoundToTheActorsTenant(t *testing.T) {
	f := streaming(t, entry(1, readable))

	if _, err := f.stream.Next(t.Context(), actor(), Position{IssuedAt: now}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(f.work.scopes) == 0 {
		t.Fatal("the read ran outside a unit of work")
	}
	for _, scope := range f.work.scopes {
		if scope.TenantID != tenant {
			t.Errorf("a transaction was opened for %q", scope.TenantID)
		}
	}
}
