// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	"github.com/Jersyfi/hubtask/core/application/service/importer"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// An import into a collection of the hub rather than as new collections under it.

var collectionID = shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")

func intoAcceptor(auth *authorizer, run *runs, queued *jobs, rows ...work.Container) importer.ImportEntries {
	h := acceptor(auth, run, readyObject(), work.Container{ID: hubID, Type: work.ContainerHub}, queued)
	held := containers{rows: map[shared.ID]work.Container{hubID: {ID: hubID, Type: work.ContainerHub}}}
	for _, row := range rows {
		held.rows[row.ID] = row
	}
	h.Containers = held
	return h
}

func TestAnImportIntoACollectionAsksTheCollectionsStructureAndQueuesItsOwnKind(t *testing.T) {
	auth, run, queued := &authorizer{}, newRuns(), &jobs{}
	h := intoAcceptor(auth, run, queued, work.Container{ID: collectionID, Type: work.ContainerCollection, ParentID: hubID})
	out, err := h.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{
		"media_id": mediaID.String(), "kind": "CSV", "hub_id": hubID.String(), "collection_id": collectionID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.String("import_id") != runID.String() {
		t.Errorf("out = %v", out)
	}
	if stored := run.rows[runID]; stored.CollectionID != collectionID || stored.HubID != hubID {
		t.Errorf("run = %+v", stored)
	}
	// A kind of its own, so that a process that would create collections from it never claims it.
	if len(queued.requests) != 1 || queued.requests[0].Kind != queue.KindImportIntoCollection {
		t.Errorf("job = %+v", queued.requests)
	}
	// STRUCTURE on the collection, by its own path: a label or a column may be added there.
	if len(auth.requests) != 1 {
		t.Fatalf("authorisation = %+v", auth.requests)
	}
	asked := auth.requests[0]
	if string(asked.Permission) != "STRUCTURE" || asked.TargetID != collectionID || len(asked.Path) != 3 || asked.Path[2].ID != collectionID || asked.Path[1].ID != hubID {
		t.Errorf("authorisation = %+v", asked)
	}
}

func TestAnImportIntoACollectionIsRefusedWhereItIsNotOneOfTheHubs(t *testing.T) {
	otherHub := shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
	gone := now
	cases := map[string][]work.Container{
		"unknown":        nil,
		"of another hub": {{ID: collectionID, Type: work.ContainerCollection, ParentID: otherHub}},
		"a hub, not one": {{ID: collectionID, Type: work.ContainerHub}},
		"in the trash":   {{ID: collectionID, Type: work.ContainerCollection, ParentID: hubID, DeletedAt: &gone}},
	}
	for name, rows := range cases {
		t.Run(name, func(t *testing.T) {
			auth, run, queued := &authorizer{}, newRuns(), &jobs{}
			h := intoAcceptor(auth, run, queued, rows...)
			_, err := h.Execute(context.Background(), actor(), importer.ImportCommand{MediaID: mediaID, Kind: domain.KindCSV, HubID: hubID, CollectionID: collectionID})
			var typed *shared.Error
			if !errors.As(err, &typed) || typed.DetailCode != domain.CodeNotInHub {
				t.Fatalf("got %v, want %s", err, domain.CodeNotInHub)
			}
			if len(run.rows) != 0 || len(queued.requests) != 0 || len(auth.requests) != 0 {
				t.Error("a refusal writes nothing and asks about no path the caller named")
			}
		})
	}
}

func TestAnImportIntoACollectionIsReadWhereItLanded(t *testing.T) {
	run := newRuns()
	run.rows[runID] = domain.Run{ID: runID, TenantID: tenantID, Kind: domain.KindCSV, HubID: hubID, CollectionID: collectionID, MediaID: mediaID, Status: domain.StatusPending, CreatedAt: now}
	auth := &authorizer{}
	out, err := importer.GetImport{Runs: run, Authorizer: auth, UnitOfWork: unitOfWork{}}.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{"import_id": runID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if out.String("collection_id") != collectionID.String() {
		t.Errorf("out = %v", out)
	}
	if len(auth.requests) != 1 || len(auth.requests[0].Path) != 3 || auth.requests[0].Path[2].ID != collectionID {
		t.Errorf("a person shown only the collection reads the import into it: %+v", auth.requests)
	}
	if hubOnly := importer.RunOutput(pendingRun()); hubOnly["collection_id"] != nil {
		t.Errorf("an import that created collections names none: %v", hubOnly)
	}
}

// recording answers nothing and keeps what it was told.
type recording struct{ source *repository.Source }

func (recording) Kind() domain.Kind { return domain.KindCSV }
func (r recording) Convert(_ context.Context, source repository.Source) (repository.Result, error) {
	*r.source = source
	return repository.Result{Records: map[string][]archive.Record{}}, nil
}

type buckets struct{ rows []work.Bucket }

func (b buckets) List(context.Context, shared.ID) ([]work.Bucket, error)  { return b.rows, nil }
func (b buckets) LastOrderKey(context.Context, shared.ID) (string, error) { return "b5", nil }

type labels struct{ rows []work.Label }

func (l labels) List(context.Context, shared.ID) ([]work.Label, error) { return l.rows, nil }

type itemOrders struct{ asked []shared.ID }

func (i *itemOrders) LastOrderKey(_ context.Context, collection, parent shared.ID) (string, error) {
	i.asked = append(i.asked, collection, parent)
	return "i9", nil
}

func TestTheRunnerHandsTheConverterTheCollectionAsItStands(t *testing.T) {
	run := newRuns()
	pending := pendingRun()
	pending.CollectionID = collectionID
	run.rows[runID] = pending
	object := readyObject()
	var told repository.Source
	r, _, _, _ := runner(run, object, map[string][]byte{object.StorageKey: []byte("one\n")}, converter{})
	r.Converters = map[domain.Kind]repository.Converter{domain.KindCSV: recording{source: &told}}
	bucketID := shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	labelID := shared.MustParseID("0192f000-0000-7000-8000-0000000000d2")
	orders := &itemOrders{}
	r.Containers = containers{rows: map[shared.ID]work.Container{collectionID: {ID: collectionID, Type: work.ContainerCollection, ParentID: hubID}}}
	r.Buckets = buckets{rows: []work.Bucket{{ID: bucketID, Name: "Doing"}}}
	r.Labels = labels{rows: []work.Label{{ID: labelID, Name: "Home"}}}
	r.Items = orders
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err != nil {
		t.Fatal(err)
	}
	if run.rows[runID].Status != domain.StatusSucceeded {
		t.Fatalf("run = %+v", run.rows[runID])
	}
	into := told.Into
	if into == nil || into.Collection != collectionID || told.Hub != hubID {
		t.Fatalf("source = %+v", told)
	}
	if len(into.Buckets) != 1 || into.Buckets[0] != (repository.Named{ID: bucketID, Name: "Doing"}) || len(into.Labels) != 1 || into.Labels[0].ID != labelID {
		t.Errorf("the collection's columns and labels = %+v", into)
	}
	// The top level of the collection: the empty parent.
	if into.LastBucketOrder != "b5" || into.LastEntryOrder != "i9" || len(orders.asked) != 2 || orders.asked[0] != collectionID || !orders.asked[1].IsZero() {
		t.Errorf("the order keys = %+v, asked %v", into, orders.asked)
	}

	// An import that creates collections reads no target.
	var hubTold repository.Source
	run.rows[runID] = pendingRun()
	r.Converters = map[domain.Kind]repository.Converter{domain.KindCSV: recording{source: &hubTold}}
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err != nil {
		t.Fatal(err)
	}
	if hubTold.Into != nil {
		t.Errorf("an import under the hub has no target: %+v", hubTold.Into)
	}
}

func TestTheRunnerFailsAnImportWhoseCollectionLeftTheHub(t *testing.T) {
	run := newRuns()
	pending := pendingRun()
	pending.CollectionID = collectionID
	run.rows[runID] = pending
	object := readyObject()
	r, landed, stored, _ := runner(run, object, map[string][]byte{object.StorageKey: []byte("one\n")}, converter{})
	gone := now
	r.Containers = containers{rows: map[shared.ID]work.Container{collectionID: {ID: collectionID, Type: work.ContainerCollection, ParentID: hubID, DeletedAt: &gone}}}
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err != nil {
		t.Fatalf("a collection gone is the run's outcome, not a retry: %v", err)
	}
	if got := run.rows[runID]; got.Status != domain.StatusFailed || got.ErrorCode != domain.CodeNotInHub {
		t.Errorf("run = %+v", got)
	}
	if len(landed.rows["work_item"]) != 0 || len(stored.deleted) != 1 {
		t.Error("nothing lands, and the file is still marked for deletion")
	}
}
