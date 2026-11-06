// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// keptStore is a KeptFigures in memory: a number of changes per collection, the kept rows.
type keptStore struct {
	changes   map[shared.ID]int64
	kept      map[repository.KeptFigureKey]repository.KeptFigure
	statesErr error
	verified  []repository.KeptFigure
}

func (k *keptStore) States(_ context.Context, wanted []repository.KeptFigureKey) (map[repository.KeptFigureKey]repository.KeptFigureState, error) {
	if k.statesErr != nil {
		return nil, k.statesErr
	}
	out := map[repository.KeptFigureKey]repository.KeptFigureState{}
	for _, key := range wanted {
		out[key] = repository.KeptFigureState{Changes: k.changes[key.CollectionID], Kept: k.kept[key]}
	}
	return out, nil
}

func (k *keptStore) Keep(_ context.Context, figures []repository.KeptFigure) error {
	for _, figure := range figures {
		k.kept[figure.Key] = figure
	}
	return nil
}

func (k *keptStore) Fold(context.Context) (int64, error)             { return 0, nil }
func (k *keptStore) Prune(context.Context, time.Time) (int64, error) { return 0, nil }

func (k *keptStore) ToVerify(context.Context, []domain.Figure, int) ([]repository.KeptFigure, error) {
	var out []repository.KeptFigure
	for _, figure := range k.kept {
		if figure.Changes == k.changes[figure.Key.CollectionID] && figure.Key.Kind.Recountable() {
			out = append(out, figure)
		}
	}
	return out, nil
}

func (k *keptStore) Verify(_ context.Context, figure repository.KeptFigure, _ time.Time) (bool, error) {
	k.verified = append(k.verified, figure)
	k.kept[figure.Key] = figure
	return true, nil
}

// jobsAsked remembers what was enqueued.
type jobsAsked struct{ requests []queue.Request }

func (j *jobsAsked) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	j.requests = append(j.requests, request)
	return shared.ID("job"), nil
}

func figureKeptReader(store *figureStore, kept *keptStore, jobs FigureJobs, at time.Time) FigureReader {
	return FigureReader{
		Figures: store, Clock: clock.Fixed(at), Profiles: &profiles{rows: seededProfiles()},
		Kept: kept, Jobs: jobs,
	}
}

// A kept count answers while its collection is unchanged; a change makes the read count again and
// keep the new count at the number read before counting (migration 0299).
func TestAKeptFigureAnswersOnlyWhileItsCollectionIsUnchanged(t *testing.T) {
	collection := countedCollection(collectionID)
	structures := map[shared.ID]domain.EffectiveStructure{collection.ID: structureCounting(domain.FigureProgress, domain.ItemTask, domain.ItemWorkPackage)}
	store := &figureStore{counts: map[shared.ID]domain.FigureCounts{collection.ID: {Done: 1, Total: 4}}}
	kept := &keptStore{changes: map[shared.ID]int64{collection.ID: 7}, kept: map[repository.KeptFigureKey]repository.KeptFigure{}}
	jobs := &jobsAsked{}
	reader := figureKeptReader(store, kept, jobs, now)

	first := reader.Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if len(store.asked) != 1 || first[collection.ID].Counts.Total != 4 {
		t.Fatalf("the first read asked %d, answered %+v", len(store.asked), first)
	}
	if len(kept.kept) != 1 {
		t.Fatalf("kept %+v", kept.kept)
	}
	for _, figure := range kept.kept {
		if figure.Changes != 7 || figure.Counts.Done != 1 {
			t.Fatalf("kept %+v", figure)
		}
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindFigureRecount || jobs.requests[0].TenantID != tenantID {
		t.Fatalf("enqueued %+v", jobs.requests)
	}

	store.counts = map[shared.ID]domain.FigureCounts{collection.ID: {Done: 2, Total: 4}}
	second := reader.Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if len(store.asked) != 1 || second[collection.ID].Counts.Done != 1 {
		t.Fatalf("an unchanged collection was counted again: asked %d, answered %+v", len(store.asked), second)
	}

	// A minute after the count, the changed collection is counted again (within the minute it is
	// answered as of its time: TestAChangedCollectionAnswersItsLastCountAsOfItsTime).
	kept.changes[collection.ID] = 8
	later := figureKeptReader(store, kept, jobs, now.Add(domain.FigureStaleFor))
	third := later.Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if len(store.asked) != 2 || third[collection.ID].Counts.Done != 2 || !third[collection.ID].CountedAt.IsZero() {
		t.Fatalf("a changed collection answered %+v", third)
	}
}

// UC-WRK-29 check 8 (the owner's fifth answers 3): under steady writes, a read within a minute of the last
// kept count answers it marked with its time, without counting in the read, and hands the count to
// a late one that keeps the next read's.
func TestAChangedCollectionAnswersItsLastCountAsOfItsTime(t *testing.T) {
	collection := countedCollection(collectionID)
	structures := map[shared.ID]domain.EffectiveStructure{collection.ID: structureCounting(domain.FigureProgress, domain.ItemTask, domain.ItemWorkPackage)}
	store := &figureStore{counts: map[shared.ID]domain.FigureCounts{collection.ID: {Done: 1, Total: 4}}}
	kept := &keptStore{changes: map[shared.ID]int64{collection.ID: 7}, kept: map[repository.KeptFigureKey]repository.KeptFigure{}}
	var held []func(context.Context)
	reader := func(at time.Time) FigureReader {
		r := figureKeptReader(store, kept, &jobsAsked{}, at)
		r.Spawn = func(_ context.Context, _ string, fn func(context.Context)) { held = append(held, fn) }
		return r
	}
	reader(now).Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)

	kept.changes[collection.ID] = 8
	store.counts = map[shared.ID]domain.FigureCounts{collection.ID: {Done: 2, Total: 4}}
	stale := reader(now.Add(30*time.Second)).Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	figure := stale[collection.ID]
	if figure.Unavailable || figure.Counts.Done != 1 || !figure.CountedAt.Equal(now) {
		t.Fatalf("within the minute answered %+v, want the kept count as of %v", figure, now)
	}
	if len(store.asked) != 1 {
		t.Errorf("the read counted itself (%d counts), want it handed to a late count", len(store.asked))
	}
	if out := figureOutput(figure); out["counted_at"] != now.UTC().Format(time.RFC3339) {
		t.Errorf("answered %v, want counted_at", out)
	}
	if len(held) != 1 {
		t.Fatalf("%d late counts started, want one", len(held))
	}
	held[0](context.Background())

	fresh := reader(now.Add(31*time.Second)).Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if got := fresh[collection.ID]; got.Counts.Done != 2 || !got.CountedAt.IsZero() {
		t.Fatalf("after the late count answered %+v, want the current count", got)
	}
	if out := figureOutput(fresh[collection.ID]); out["counted_at"] != nil {
		t.Errorf("a current count carries counted_at: %v", out)
	}
}

// A kind the clock moves answers from what is kept for a minute only.
func TestAFlowFigureIsKeptForAMinute(t *testing.T) {
	collection := countedCollection(collectionID)
	structures := map[shared.ID]domain.EffectiveStructure{collection.ID: structureCounting(domain.FigureFlow, domain.ItemTask)}
	store := &figureStore{counts: map[shared.ID]domain.FigureCounts{collection.ID: {Completed: 3}}}
	kept := &keptStore{changes: map[shared.ID]int64{}, kept: map[repository.KeptFigureKey]repository.KeptFigure{}}

	figureKeptReader(store, kept, nil, now).Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	figureKeptReader(store, kept, nil, now.Add(59*time.Second)).Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if len(store.asked) != 1 {
		t.Fatalf("within the minute counted %d times", len(store.asked))
	}
	figureKeptReader(store, kept, nil, now.Add(time.Minute)).Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if len(store.asked) != 2 {
		t.Fatalf("after the minute counted %d times", len(store.asked))
	}
}

// What is kept that cannot be read is counted, and nothing is kept on a number nobody read.
func TestKeptFiguresThatCannotBeReadAreCounted(t *testing.T) {
	collection := countedCollection(collectionID)
	structures := map[shared.ID]domain.EffectiveStructure{collection.ID: structureCounting(domain.FigureOpen, domain.ItemTask)}
	store := &figureStore{counts: map[shared.ID]domain.FigureCounts{collection.ID: {Open: 5}}}
	kept := &keptStore{statesErr: errors.New("statement timeout"), kept: map[repository.KeptFigureKey]repository.KeptFigure{}}

	figures := figureKeptReader(store, kept, nil, now).Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if figures[collection.ID].Unavailable || figures[collection.ID].Counts.Open != 5 || len(kept.kept) != 0 {
		t.Fatalf("answered %+v, kept %+v", figures, kept.kept)
	}
}

// The recount writes the fresh count over one that drifted and leaves one that did not, and
// recounts nothing the clock moves.
func TestTheRecountRepairsADriftedFigure(t *testing.T) {
	right := repository.KeptFigureKey{CollectionID: collectionID, Kind: domain.FigureOpen, BasisKey: "a"}
	other := shared.ID("0192f000-0000-7000-8000-0000000000e2")
	wrong := repository.KeptFigureKey{CollectionID: other, Kind: domain.FigureOpen, BasisKey: "a"}
	kept := &keptStore{
		changes: map[shared.ID]int64{collectionID: 3, other: 3},
		kept: map[repository.KeptFigureKey]repository.KeptFigure{
			right: {Key: right, Changes: 3, Counts: domain.FigureCounts{Open: 2}, Present: true},
			wrong: {Key: wrong, Changes: 3, Counts: domain.FigureCounts{Open: 9}, Present: true},
		},
	}
	store := &figureStore{counts: map[shared.ID]domain.FigureCounts{collectionID: {Open: 2}, other: {Open: 4}}}
	drift := &driftSignals{}
	recount := RecountFigures{Kept: kept, Figures: store, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), Signals: drift}

	outcome, err := recount.Execute(context.Background(), actor())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Verified != 2 || outcome.Repaired != 1 || drift.count != 1 {
		t.Fatalf("outcome %+v, drift %d", outcome, drift.count)
	}
	if kept.kept[wrong].Counts.Open != 4 || kept.kept[right].Counts.Open != 2 {
		t.Fatalf("kept %+v", kept.kept)
	}
}

type driftSignals struct{ count int64 }

func (d *driftSignals) FigureDrifted(_ context.Context, count int64) { d.count += count }

// slowFigures runs past the read's deadline on its first count and answers on the next, as a count
// over a collection of hundreds of thousands of entries does; it remembers each count's deadline.
type slowFigures struct {
	figureStore
	deadlines []time.Duration
}

func (s *slowFigures) Count(
	ctx context.Context, requests []domain.FigureRequest, window domain.FigureWindow,
) (map[shared.ID]domain.FigureCounts, error) {
	deadline, _ := ctx.Deadline()
	s.deadlines = append(s.deadlines, time.Until(deadline).Round(time.Second))
	if len(s.deadlines) == 1 {
		return nil, context.DeadlineExceeded
	}
	return s.figureStore.Count(ctx, requests, window)
}

// A count the read's deadline cut off is counted once more outside the read, under a deadline of its
// own, and kept: the next read answers from it instead of paying the deadline and the mark again,
// which at 10⁷ entries every read of the largest hub did (capacity-10000000-2026-11-01.md).
func TestACountTheReadCouldNotWaitForIsCountedAfterwardsAndKept(t *testing.T) {
	collection := countedCollection(collectionID)
	structures := map[shared.ID]domain.EffectiveStructure{collection.ID: structureCounting(domain.FigureProgress, domain.ItemTask, domain.ItemWorkPackage)}
	store := &slowFigures{figureStore: figureStore{counts: map[shared.ID]domain.FigureCounts{collection.ID: {Done: 3, Total: 9}}}}
	kept := &keptStore{changes: map[shared.ID]int64{collection.ID: 7}, kept: map[repository.KeptFigureKey]repository.KeptFigure{}}
	jobs := &jobsAsked{}
	var spawned []string
	reader := FigureReader{
		Figures: store, Clock: clock.Fixed(now), Profiles: &profiles{rows: seededProfiles()},
		Kept: kept, Jobs: jobs,
		Spawn: func(ctx context.Context, component string, fn func(context.Context)) {
			spawned = append(spawned, component)
			fn(ctx)
		},
	}

	first := reader.Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if out := figureOutput(first[collection.ID]); out["unavailable"] != true {
		t.Fatalf("the cut-off read answered %v", out)
	}
	if len(spawned) != 1 || len(store.deadlines) != 2 || store.deadlines[1] != figureCountDeadline {
		t.Fatalf("spawned %v, deadlines %v", spawned, store.deadlines)
	}
	if len(kept.kept) != 1 {
		t.Fatalf("the late count was not kept: %+v", kept.kept)
	}
	for _, figure := range kept.kept {
		if figure.Changes != 7 || figure.Counts.Done != 3 {
			t.Fatalf("kept %+v", figure)
		}
	}

	second := reader.Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if len(store.deadlines) != 2 || second[collection.ID].Counts.Done != 3 {
		t.Fatalf("the next read counted again (%d counts) or answered %+v", len(store.deadlines), second)
	}
}

// While one late count for a figure runs, another cut-off read starts none: eight callers on a slow
// hub would otherwise run eight counts of it at once.
func TestOnlyOneLateCountRunsPerFigure(t *testing.T) {
	t.Cleanup(func() { slowCounts.Clear() })
	collection := countedCollection(collectionID)
	structures := map[shared.ID]domain.EffectiveStructure{collection.ID: structureCounting(domain.FigureProgress, domain.ItemTask, domain.ItemWorkPackage)}
	kept := &keptStore{changes: map[shared.ID]int64{collection.ID: 7}, kept: map[repository.KeptFigureKey]repository.KeptFigure{}}
	var held []func(context.Context)
	reader := FigureReader{
		Figures:  &figureStore{err: context.DeadlineExceeded},
		Clock:    clock.Fixed(now),
		Profiles: &profiles{rows: seededProfiles()},
		Kept:     kept, Jobs: &jobsAsked{},
		Spawn: func(_ context.Context, _ string, fn func(context.Context)) { held = append(held, fn) },
	}
	for range 3 {
		reader.Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	}
	if len(held) != 1 {
		t.Fatalf("%d late counts started for one figure", len(held))
	}
	// The reads while it runs do not count at all: each would wait out the deadline for the mark.
	if asked := len(reader.Figures.(*figureStore).asked); asked != 1 {
		t.Fatalf("%d counts while the late one ran, want the first only", asked)
	}
	held[0](context.Background())
	reader.Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if len(held) != 2 {
		t.Fatalf("a finished late count left its figure blocked: %d started", len(held))
	}
	held[1](context.Background())
}

// steppedClock moves on by step each time it is read: a late count that reads it before and after
// counting takes step.
type steppedClock struct {
	at   time.Time
	step time.Duration
}

func (c *steppedClock) Now() time.Time {
	now := c.at
	c.at = c.at.Add(c.step)
	return now
}

// A figure whose count outlasted the read's deadline is not counted by the next read after a write
// - it would only wait out the deadline and leave a cancelled statement - but handed to a late count
// at once; once a late count answers within the deadline, a read counts it again itself
// (capacity-10000000-2026-11-01.md, finding 4).
func TestASlowFigureIsCountedLateWithoutWaitingOutTheRead(t *testing.T) {
	t.Cleanup(func() { slowCounts.Clear() })
	collection := countedCollection(collectionID)
	structures := map[shared.ID]domain.EffectiveStructure{collection.ID: structureCounting(domain.FigureProgress, domain.ItemTask, domain.ItemWorkPackage)}
	store := &slowFigures{figureStore: figureStore{counts: map[shared.ID]domain.FigureCounts{collection.ID: {Done: 3, Total: 9}}}}
	kept := &keptStore{changes: map[shared.ID]int64{collection.ID: 7}, kept: map[repository.KeptFigureKey]repository.KeptFigure{}}
	ticking := &steppedClock{at: now, step: time.Second}
	var held []func(context.Context)
	reader := FigureReader{
		Figures: store, Clock: ticking, Profiles: &profiles{rows: seededProfiles()},
		Kept: kept, Jobs: &jobsAsked{},
		Spawn: func(_ context.Context, _ string, fn func(context.Context)) { held = append(held, fn) },
	}
	read := func() domain.CollectionFigure {
		return reader.Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)[collection.ID]
	}

	read() // cut off: the count is handed to a late one, which takes a second
	held[0](context.Background())
	kept.changes[collection.ID] = 8 // a write leaves the kept count behind

	// Within the minute the kept count answers as of its time (fifth answers 3), and the count
	// goes to a late one rather than the read.
	if figure := read(); figure.Unavailable || figure.CountedAt.IsZero() || figure.Counts.Done != 3 {
		t.Fatalf("the read after the write answered %+v", figure)
	}
	if len(store.deadlines) != 2 || len(held) != 2 {
		t.Fatalf("the read counted %d times (want 2: the cut-off and the late one) and started %d late counts",
			len(store.deadlines), len(held))
	}

	ticking.step = 0 // the late count now answers at once
	held[1](context.Background())
	kept.changes[collection.ID] = 9
	ticking.at = ticking.at.Add(domain.FigureStaleFor) // past the minute a kept count answers
	if figure := read(); figure.Unavailable || figure.Counts.Done != 3 {
		t.Fatalf("a figure counted quickly again was not counted by the read: %+v", figure)
	}
	if len(held) != 2 {
		t.Fatalf("%d late counts for a figure that counts in time", len(held))
	}
}
