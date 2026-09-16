// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	backuprepo "github.com/Jersyfi/hubtask/core/application/repository/backup"
	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/backup"
	"github.com/Jersyfi/hubtask/core/application/service/importer"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	backupdomain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

var (
	tenantID  = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	accountID = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	hubID     = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	mediaID   = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	runID     = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	now       = time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
)

/* ── fakes ─────────────────────────────────────────────────────────────────────────────── */

type unitOfWork struct{}

func (unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}
func (unitOfWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return now }

type ids struct{}

func (ids) NewID() shared.ID { return runID }

type authorizer struct {
	err      error
	requests []access.Request
}

func (a *authorizer) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	a.requests = append(a.requests, request)
	return a.err
}

type runs struct {
	rows     map[shared.ID]domain.Run
	progress int
}

func newRuns() *runs { return &runs{rows: map[shared.ID]domain.Run{}} }

func (r *runs) Insert(_ context.Context, run domain.Run) error { r.rows[run.ID] = run; return nil }
func (r *runs) Find(_ context.Context, id shared.ID) (domain.Run, error) {
	run, ok := r.rows[id]
	if !ok {
		return domain.Run{}, shared.ErrNotFound.WithDetail(domain.CodeNotFound)
	}
	return run, nil
}
func (r *runs) Claim(_ context.Context, id shared.ID, at time.Time) (bool, error) {
	run, ok := r.rows[id]
	if !ok || (run.Status != domain.StatusPending && run.Status != domain.StatusRunning) {
		return false, nil
	}
	run.Status, run.StartedAt = domain.StatusRunning, &at
	r.rows[id] = run
	return true, nil
}
func (r *runs) RecordProgress(_ context.Context, id shared.ID, report backupdomain.Report, progress map[string]int) error {
	run := r.rows[id]
	run.Report, run.Progress = report, progress
	r.rows[id] = run
	r.progress++
	return nil
}
func (r *runs) Finish(_ context.Context, outcome domain.Outcome) error {
	run := r.rows[outcome.ID]
	run.Status, run.Report, run.Refused, run.ErrorCode, run.FinishedAt = outcome.Status, outcome.Report, outcome.Refused, outcome.ErrorCode, &outcome.FinishedAt
	r.rows[outcome.ID] = run
	return nil
}

type objects struct {
	rows    map[shared.ID]media.Object
	deleted []shared.ID
}

func (o *objects) Find(_ context.Context, id shared.ID) (media.Object, error) {
	object, ok := o.rows[id]
	if !ok {
		return media.Object{}, shared.ErrNotFound.WithDetail("media.not_found")
	}
	return object, nil
}
func (o *objects) MarkDeleted(_ context.Context, id shared.ID, _ time.Time) (bool, error) {
	o.deleted = append(o.deleted, id)
	return true, nil
}

type containers struct{ rows map[shared.ID]work.Container }

func (c containers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	container, ok := c.rows[id]
	if !ok {
		return work.Container{}, shared.ErrNotFound
	}
	return container, nil
}

type jobs struct{ requests []queue.Request }

func (j *jobs) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	j.requests = append(j.requests, request)
	return shared.MustParseID("0192f000-0000-7000-8000-0000000000a1"), nil
}

type objectStore struct {
	files map[string][]byte
	away  bool
}

func (s objectStore) Put(context.Context, storage.Upload) error { return nil }
func (s objectStore) Get(_ context.Context, key string) (storage.Object, error) {
	if s.away {
		return storage.Object{}, shared.ErrUnavailable.WithDetail("storage.unreachable")
	}
	content, ok := s.files[key]
	if !ok {
		return storage.Object{}, shared.ErrNotFound
	}
	return storage.Object{Content: io.NopCloser(bytes.NewReader(content)), Size: int64(len(content)), ContentType: "text/csv"}, nil
}
func (s objectStore) Delete(context.Context, string) error { return nil }

// converter answers a collection and one entry per line of the file, or refuses it.
type converter struct{ refuse error }

func (converter) Kind() domain.Kind { return domain.KindCSV }
func (c converter) Convert(_ context.Context, source repository.Source) (repository.Result, error) {
	if c.refuse != nil {
		return repository.Result{}, c.refuse
	}
	raw, _ := io.ReadAll(source.Content)
	collection := backupdomain.DuplicateID(source.Hub, "import", source.Digest)
	result := repository.Result{Records: map[string][]archive.Record{
		"containers": {{ID: collection.String(), Op: archive.OpUpsert, UpdatedAt: source.Now, Data: map[string]any{"id": collection.String(), "type": "COLLECTION", "parent_id": source.Hub.String(), "name": "Imported", "order_key": "a", "created_by": source.Actor.String(), "version": 1}}},
	}}
	for i, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "bad") {
			result.Refused = append(result.Refused, domain.Refusal{Row: i + 1, Code: domain.CodeRowTitleMissing})
			continue
		}
		id := backupdomain.DuplicateID(collection, "work_items", line)
		result.Records["work_items"] = append(result.Records["work_items"], archive.Record{
			ID: id.String(), Op: archive.OpUpsert, UpdatedAt: source.Now,
			Data: map[string]any{"id": id.String(), "collection_id": collection.String(), "type": "TASK", "title": line, "path": "/" + id.String() + "/", "depth": 0, "order_key": "a", "created_by": source.Actor.String(), "version": 1},
		})
	}
	return result, nil
}

// importRepo is the restore's write port over a map: what the applier lands, by table.
type importRepo struct {
	rows map[string]map[string]map[string]any
}

// newImportRepo holds the hub the import lands under: the collection names it as its parent, and
// a parent nowhere to be found is withheld rather than written (#693).
func newImportRepo() *importRepo {
	return &importRepo{rows: map[string]map[string]map[string]any{
		"container": {hubID.String(): {"id": hubID.String(), "type": "HUB"}},
	}}
}
func (r *importRepo) Holds(_ context.Context, table string, data map[string]any) (bool, error) {
	_, ok := r.rows[table][data["id"].(string)]
	return ok, nil
}
func (r *importRepo) Write(_ context.Context, table string, data map[string]any, overwrite bool) (bool, error) {
	if r.rows[table] == nil {
		r.rows[table] = map[string]map[string]any{}
	}
	id := data["id"].(string)
	if _, exists := r.rows[table][id]; exists && !overwrite {
		return false, nil
	}
	r.rows[table][id] = data
	return true, nil
}
func (r *importRepo) Clear(_ context.Context, table string) (int, error) {
	n := len(r.rows[table])
	delete(r.rows, table)
	return n, nil
}

type journal struct{}

func (journal) DeletedSince(context.Context, time.Time, func(backuprepo.Deletion) error) error {
	return nil
}

type epochs struct{ advanced int }

func (e *epochs) Advance(context.Context) (int64, error) { e.advanced++; return int64(e.advanced), nil }

func actor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID, AccountName: "Anna",
		TimeZone: "Europe/Berlin", Locale: "de", Scopes: []string{"containers:write", "containers:read"},
	}
}

func readyObject() media.Object {
	return media.Object{ID: mediaID, TenantID: tenantID, StorageKey: "t/" + mediaID.String(), Usage: media.UsageImport, Status: media.StatusReady, CreatedBy: accountID, ByteSize: 12}
}

func acceptor(auth *authorizer, run *runs, object media.Object, hub work.Container, queued *jobs) importer.ImportEntries {
	return importer.ImportEntries{
		Runs: run, Objects: &objects{rows: map[shared.ID]media.Object{object.ID: object}},
		Containers: containers{rows: map[shared.ID]work.Container{hub.ID: hub}},
		Authorizer: auth, Jobs: queued, Kinds: []domain.Kind{domain.KindCSV},
		UnitOfWork: unitOfWork{}, Clock: fixedClock{}, IDs: ids{},
	}
}

/* ── ImportEntries ─────────────────────────────────────────────────────────────────────── */

func TestAnImportIsAcceptedWithItsRunAndItsJob(t *testing.T) {
	auth, run, queued := &authorizer{}, newRuns(), &jobs{}
	h := acceptor(auth, run, readyObject(), work.Container{ID: hubID, Type: work.ContainerHub}, queued)
	accepted, err := h.Execute(context.Background(), actor(), importer.ImportCommand{MediaID: mediaID, Kind: domain.KindCSV, HubID: hubID, Mapping: map[string]string{"title": "Aufgabe"}})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.ImportID != runID || accepted.JobID.IsZero() {
		t.Errorf("accepted = %+v", accepted)
	}
	stored := run.rows[runID]
	if stored.Kind != domain.KindCSV || stored.HubID != hubID || stored.MediaID != mediaID || stored.Zone != "Europe/Berlin" || stored.Language != "de" || stored.Mapping["title"] != "Aufgabe" || stored.Status != domain.StatusPending {
		t.Errorf("run = %+v", stored)
	}
	if len(queued.requests) != 1 || queued.requests[0].Kind != queue.KindImport || queued.requests[0].Payload["import_id"] != runID.String() || queued.requests[0].DedupeKey == "" {
		t.Errorf("job = %+v", queued.requests)
	}
	// STRUCTURE on the hub, asked before anything is written.
	if len(auth.requests) != 1 || string(auth.requests[0].Permission) != "STRUCTURE" || auth.requests[0].TargetID != hubID {
		t.Errorf("authorisation = %+v", auth.requests)
	}
}

func TestAnImportIsRefusedForTheRightReasons(t *testing.T) {
	hub := work.Container{ID: hubID, Type: work.ContainerHub}
	cases := map[string]struct {
		object media.Object
		hub    work.Container
		cmd    importer.ImportCommand
		auth   error
		code   string
	}{
		"no hub":            {readyObject(), hub, importer.ImportCommand{MediaID: mediaID, Kind: domain.KindCSV}, nil, domain.CodeHubRequired},
		"a kind not served": {readyObject(), hub, importer.ImportCommand{MediaID: mediaID, Kind: domain.KindTrello, HubID: hubID}, nil, domain.CodeKindUnsupported},
		"not a hub":         {readyObject(), work.Container{ID: hubID, Type: work.ContainerCollection}, importer.ImportCommand{MediaID: mediaID, Kind: domain.KindCSV, HubID: hubID}, nil, domain.CodeNotAHub},
		"not an import":     {func() media.Object { o := readyObject(); o.Usage = media.UsageAttachment; return o }(), hub, importer.ImportCommand{MediaID: mediaID, Kind: domain.KindCSV, HubID: hubID}, nil, domain.CodeMediaNotImport},
		"not confirmed":     {func() media.Object { o := readyObject(); o.Status = media.StatusPending; return o }(), hub, importer.ImportCommand{MediaID: mediaID, Kind: domain.KindCSV, HubID: hubID}, nil, domain.CodeMediaNotReady},
		"somebody else's":   {func() media.Object { o := readyObject(); o.CreatedBy = hubID; return o }(), hub, importer.ImportCommand{MediaID: mediaID, Kind: domain.KindCSV, HubID: hubID}, nil, domain.CodeMediaNotOwned},
		"forbidden":         {readyObject(), hub, importer.ImportCommand{MediaID: mediaID, Kind: domain.KindCSV, HubID: hubID}, shared.ErrForbidden.WithDetail("access.forbidden"), "access.forbidden"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			run, queued := newRuns(), &jobs{}
			h := acceptor(&authorizer{err: c.auth}, run, c.object, c.hub, queued)
			_, err := h.Execute(context.Background(), actor(), c.cmd)
			var typed *shared.Error
			if !errors.As(err, &typed) || typed.DetailCode != c.code {
				t.Fatalf("got %v, want %s", err, c.code)
			}
			if len(run.rows) != 0 || len(queued.requests) != 0 {
				t.Error("a refusal writes nothing")
			}
		})
	}
	// An unknown hub answers the container's own code, and a missing file the media one.
	run, queued := newRuns(), &jobs{}
	h := acceptor(&authorizer{}, run, readyObject(), hub, queued)
	if _, err := h.Execute(context.Background(), actor(), importer.ImportCommand{MediaID: mediaID, Kind: domain.KindCSV, HubID: mediaID}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("unknown hub: %v", err)
	}
	if _, err := h.Execute(context.Background(), actor(), importer.ImportCommand{MediaID: hubID, Kind: domain.KindCSV, HubID: hubID}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("unknown file: %v", err)
	}
}

func TestTheDescriptorInvokesWithTheContractsFields(t *testing.T) {
	run, queued := newRuns(), &jobs{}
	h := acceptor(&authorizer{}, run, readyObject(), work.Container{ID: hubID, Type: work.ContainerHub}, queued)
	out, err := h.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{
		"media_id": mediaID.String(), "kind": "CSV", "hub_id": hubID.String(), "mapping": map[string]any{"title": "Aufgabe", "ignored": 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.String("import_id") != runID.String() || out.String("result_url") != "/imports/"+runID.String() || out.String("job_id") == "" {
		t.Errorf("out = %v", out)
	}
	if run.rows[runID].Mapping["title"] != "Aufgabe" || len(run.rows[runID].Mapping) != 1 {
		t.Errorf("mapping = %v", run.rows[runID].Mapping)
	}
	if _, err := h.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{"media_id": mediaID.String(), "kind": "JIRA", "hub_id": hubID.String()}); err == nil {
		t.Error("an unknown kind is refused by the descriptor's own parse")
	}
	d := h.Descriptor()
	if d.Name != "ImportEntries" || !d.Audit.Required || d.TokenScope != "containers:write" {
		t.Errorf("descriptor = %+v", d)
	}
}

/* ── GetImport ─────────────────────────────────────────────────────────────────────────── */

func TestAnImportIsReadByWhoeverMayReadTheHubAndAbsentOtherwise(t *testing.T) {
	run := newRuns()
	finished := now.Add(time.Minute)
	run.rows[runID] = domain.Run{
		ID: runID, TenantID: tenantID, Kind: domain.KindCSV, HubID: hubID, MediaID: mediaID, Status: domain.StatusSucceeded,
		Report:  backupdomain.Report{New: 3, Entities: map[string]int{"work_items": 2, "containers": 1}},
		Refused: []domain.Refusal{{Row: 4, Code: domain.CodeRowDateInvalid}}, CreatedAt: now, FinishedAt: &finished,
	}
	auth := &authorizer{}
	h := importer.GetImport{Runs: run, Authorizer: auth, UnitOfWork: unitOfWork{}}
	out, err := h.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{"import_id": runID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if out.String("status") != "SUCCEEDED" || out.String("hub_id") != hubID.String() {
		t.Errorf("out = %v", out)
	}
	report := out["report"].(usecase.Output)
	if report["new"] != 3 || report["entities"].(map[string]int)["work_items"] != 2 {
		t.Errorf("report = %v", report)
	}
	if refused := out["refused"].([]usecase.Output); len(refused) != 1 || refused[0]["row"] != 4 {
		t.Errorf("refused = %v", out["refused"])
	}
	if len(auth.requests) != 1 || string(auth.requests[0].Permission) != "READ" || auth.requests[0].TargetID != runID {
		t.Errorf("authorisation = %+v", auth.requests)
	}
	// Forbidden reads as absent.
	denied := importer.GetImport{Runs: run, Authorizer: &authorizer{err: shared.ErrForbidden}, UnitOfWork: unitOfWork{}}
	if _, err := denied.Execute(context.Background(), actor(), runID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("forbidden should read as absent: %v", err)
	}
	if _, err := h.Execute(context.Background(), actor(), hubID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("unknown should be absent: %v", err)
	}
	if !h.Descriptor().ReadOnly {
		t.Error("the read declares itself read-only")
	}
	pending := importer.RunOutput(domain.Run{ID: runID, Status: domain.StatusPending, Kind: domain.KindCSV, HubID: hubID, MediaID: mediaID, CreatedAt: now})
	if _, has := pending["report"]; has || pending["finished_at"] != nil {
		t.Errorf("a pending run carries no report: %v", pending)
	}
}

/* ── Runner ────────────────────────────────────────────────────────────────────────────── */

func runner(run *runs, object media.Object, files map[string][]byte, conv converter) (importer.Runner, *importRepo, *objects, *epochs) {
	landed := newImportRepo()
	stored := &objects{rows: map[shared.ID]media.Object{object.ID: object}}
	epoch := &epochs{}
	return importer.Runner{
		Runs: run, Objects: stored, Store: objectStore{files: files},
		Converters: map[domain.Kind]repository.Converter{domain.KindCSV: conv},
		Applier: backup.Applier{
			Import: landed, Journal: journal{}, Epochs: epoch, UnitOfWork: unitOfWork{}, Clock: fixedClock{},
			Batch: backup.DefaultRestoreBatch,
		},
		UnitOfWork: unitOfWork{}, Clock: fixedClock{}, MaxBytes: 1 << 20, SchemaVersion: "test", ProductVersion: "test",
	}, landed, stored, epoch
}

func pendingRun() domain.Run {
	return domain.Run{ID: runID, TenantID: tenantID, RequestedBy: accountID, Kind: domain.KindCSV, MediaID: mediaID, HubID: hubID, Status: domain.StatusPending, CreatedAt: now, Zone: "Europe/Berlin"}
}

func TestTheRunnerLandsTheFileAndFinishesTheRun(t *testing.T) {
	run := newRuns()
	run.rows[runID] = pendingRun()
	object := readyObject()
	r, landed, stored, epoch := runner(run, object, map[string][]byte{object.StorageKey: []byte("one\ntwo\nbad three\n")}, converter{})
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err != nil {
		t.Fatal(err)
	}
	finished := run.rows[runID]
	if finished.Status != domain.StatusSucceeded || finished.ErrorCode != "" {
		t.Fatalf("run = %+v", finished)
	}
	if finished.Report.New != 3 || finished.Report.Entities["work_items"] != 2 || finished.Report.Entities["containers"] != 1 {
		t.Errorf("report = %+v", finished.Report)
	}
	if len(finished.Refused) != 1 || finished.Refused[0].Row != 3 {
		t.Errorf("refused = %+v", finished.Refused)
	}
	if len(landed.rows["work_item"]) != 2 || len(landed.rows["container"]) != 2 {
		t.Errorf("landed = %v", landed.rows)
	}
	if len(stored.deleted) != 1 || stored.deleted[0] != mediaID {
		t.Error("the file is marked for deletion when the job ends")
	}
	if epoch.advanced != 1 {
		t.Error("the epoch advances on success, as after a MERGE restore")
	}
	if run.progress == 0 {
		t.Error("the applier records its progress on the run")
	}

	// The same file again, as a second run: the same identities, nothing new.
	second := shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	again := pendingRun()
	again.ID = second
	run.rows[second] = again
	r.Runs = run
	if err := r.Run(context.Background(), importer.RunInput{ImportID: second, TenantID: tenantID}); err != nil {
		t.Fatal(err)
	}
	if got := run.rows[second]; got.Status != domain.StatusSucceeded || got.Report.New != 0 || got.Report.Skipped != 3 {
		t.Errorf("the second run = %+v", got.Report)
	}
	if len(landed.rows["work_item"]) != 2 {
		t.Error("the second run created nothing")
	}
}

func TestTheRunnerRecordsAFileThatIsNotItsKindAndRetriesTheStoreBeingAway(t *testing.T) {
	run := newRuns()
	run.rows[runID] = pendingRun()
	object := readyObject()
	r, landed, stored, epoch := runner(run, object, map[string][]byte{object.StorageKey: []byte("x")}, converter{refuse: shared.ErrValidation.WithDetail(domain.CodeFileNotKind)})
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err != nil {
		t.Fatalf("a file that is not its kind is the run's outcome, not the job's: %v", err)
	}
	if got := run.rows[runID]; got.Status != domain.StatusFailed || got.ErrorCode != domain.CodeFileNotKind {
		t.Errorf("run = %+v", got)
	}
	if len(landed.rows["work_item"]) != 0 || len(landed.rows["container"]) != 1 || epoch.advanced != 0 || len(stored.deleted) != 1 {
		t.Error("nothing landed, the epoch stood, the file went")
	}

	// The bytes being gone is the file's fault and the run's outcome; the store being away is
	// transient - the run stays RUNNING and the job retries.
	run.rows[runID] = pendingRun()
	r.Store = objectStore{files: map[string][]byte{}}
	r.Converters = map[domain.Kind]repository.Converter{domain.KindCSV: converter{}}
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err != nil || run.rows[runID].Status != domain.StatusFailed {
		t.Errorf("a missing object is the run's failure: %v %+v", err, run.rows[runID].Status)
	}
	run.rows[runID] = pendingRun()
	r.Store = objectStore{files: map[string][]byte{object.StorageKey: []byte("x")}, away: true}
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err == nil || run.rows[runID].Status != domain.StatusRunning {
		t.Errorf("a store that is away is returned for the retry and the run stays running: %v %v", err, run.rows[runID].Status)
	}

	// A run already finished is left alone; a kind without a converter is recorded.
	done := pendingRun()
	done.Status = domain.StatusSucceeded
	run.rows[runID] = done
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err != nil {
		t.Errorf("a finished run is a no-op: %v", err)
	}
	trello := pendingRun()
	trello.Kind = domain.KindTrello
	run.rows[runID] = trello
	r.Store = objectStore{files: map[string][]byte{object.StorageKey: []byte("x")}}
	if err := r.Run(context.Background(), importer.RunInput{ImportID: runID, TenantID: tenantID}); err != nil || run.rows[runID].ErrorCode != domain.CodeKindUnsupported {
		t.Errorf("a kind without a converter: %v %+v", err, run.rows[runID])
	}
}
