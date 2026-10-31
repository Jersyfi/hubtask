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
// keep the new count at the number read before counting (migration 0292).
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

	kept.changes[collection.ID] = 8
	third := reader.Count(context.Background(), actor(), &unitOfWork{}, []domain.Container{collection}, structures)
	if len(store.asked) != 2 || third[collection.ID].Counts.Done != 2 {
		t.Fatalf("a changed collection answered %+v", third)
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
