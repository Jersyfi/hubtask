// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

var (
	tenantID  = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	accountID = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	strangerA = shared.MustParseID("0192f000-0000-7000-8000-00000000000f")
	mintedID  = shared.MustParseID("0192f000-0000-7000-8000-000000000001")
	now       = time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)

	// A PNG signature, so the sniff agrees with the claim rather than the test's imagination.
	pngBytes = append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0}, 24)...)
)

func actor(scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
		AccountName: "Anna Beispiel", Scopes: scopes,
	}
}

func config(limit int64) env.Config {
	return env.Config{Request: env.RequestConfig{MaxUploadBytes: limit}}
}

// --- the ports, as fakes ------------------------------------------------------------------

type objects struct {
	stored  map[shared.ID]domain.Object
	sealed  []domain.Object
	sealErr error
	findErr error
	// refs is what each object serves, for the authorisation question GetMedia asks.
	refs map[shared.ID][]repository.ItemRef
	// referenced are the objects MarkDeleted refuses to mark, which is what the repository does
	// for an object something still points at.
	referenced map[shared.ID]bool
	marked     []shared.ID
	// pages is what ListForItem answers, and asked records the page it was asked for - which is
	// how a test says that the size was clamped rather than passed through.
	pages    map[shared.ID]repository.ObjectPage
	asked    map[shared.ID]workrepo.Page
	inserted int
}

func newObjects() *objects {
	return &objects{
		stored:     map[shared.ID]domain.Object{},
		refs:       map[shared.ID][]repository.ItemRef{},
		referenced: map[shared.ID]bool{},
		pages:      map[shared.ID]repository.ObjectPage{},
		asked:      map[shared.ID]workrepo.Page{},
	}
}

func (o *objects) Insert(_ context.Context, object domain.Object) error {
	o.inserted++
	o.stored[object.ID] = object
	return nil
}

func (o *objects) Find(_ context.Context, id shared.ID) (domain.Object, error) {
	if o.findErr != nil {
		return domain.Object{}, o.findErr
	}
	object, ok := o.stored[id]
	if !ok {
		return domain.Object{}, shared.ErrNotFound.WithDetail("media.not_found")
	}
	return object, nil
}

func (o *objects) Seal(_ context.Context, object domain.Object) error {
	if o.sealErr != nil {
		return o.sealErr
	}
	o.sealed = append(o.sealed, object)
	o.stored[object.ID] = object
	return nil
}

func (o *objects) AdjustRefCount(context.Context, shared.ID, int) error { return nil }
func (o *objects) MarkDeleted(_ context.Context, id shared.ID, at time.Time) (bool, error) {
	if o.referenced[id] {
		return false, nil
	}
	object, ok := o.stored[id]
	if !ok || object.DeletedAt != nil {
		return false, nil
	}
	object.DeletedAt = &at
	o.stored[id] = object
	o.marked = append(o.marked, id)
	return true, nil
}
func (o *objects) Recount(context.Context, time.Time) error { return nil }
func (o *objects) MarkOrphans(
	context.Context, time.Time, repository.Thresholds,
) (int, error) {
	return 0, nil
}
func (o *objects) TakeOrphans(context.Context, time.Time, int) ([]repository.Orphan, error) {
	return nil, nil
}

func (o *objects) ReferencingItems(_ context.Context, id shared.ID) ([]repository.ItemRef, error) {
	return o.refs[id], nil
}

func (o *objects) ListForItem(
	_ context.Context, itemID shared.ID, page workrepo.Page,
) (repository.ObjectPage, error) {
	o.asked[itemID] = page
	return o.pages[itemID], nil
}
func (o *objects) RemoveRows(context.Context, []shared.ID) (int, error) { return 0, nil }

type store struct {
	content map[string][]byte
	types   map[string]string
	getErr  error
	deleted []string
	puts    []storage.Upload
}

func newStore() *store {
	return &store{content: map[string][]byte{}, types: map[string]string{}}
}

func (s *store) Put(_ context.Context, upload storage.Upload) error {
	body, err := io.ReadAll(upload.Content)
	if err != nil {
		return err
	}
	s.puts = append(s.puts, upload)
	s.content[upload.Key] = body
	s.types[upload.Key] = upload.ContentType
	return nil
}

func (s *store) Get(_ context.Context, key string) (storage.Object, error) {
	if s.getErr != nil {
		return storage.Object{}, s.getErr
	}
	body, ok := s.content[key]
	if !ok {
		return storage.Object{}, shared.ErrNotFound.WithDetail("storage.object_not_found")
	}
	return storage.Object{
		Content:     io.NopCloser(bytes.NewReader(body)),
		Size:        int64(len(body)),
		ContentType: s.types[key],
	}, nil
}

func (s *store) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	delete(s.content, key)
	return nil
}

// guard is the judgement, as the port declares it. It sniffs nothing: what the real one decides is
// tested where it lives (infrastructure/storage, SG-12), and what these tests need is that the
// answer is used and that a refusal is passed on.
type guard struct {
	judged string
	err    error
	asked  []string
}

func (g *guard) Inspect(content io.Reader, claimedType string, _ int64) (storage.Inspection, error) {
	g.asked = append(g.asked, claimedType)
	if g.err != nil {
		return storage.Inspection{}, g.err
	}
	return storage.Inspection{ContentType: g.judged, Content: content}, nil
}

type transfers struct {
	uploads   []domain.Object
	downloads []domain.Object
	err       error
}

func (t *transfers) IssueUpload(object domain.Object, expiresAt time.Time) (storage.Transfer, error) {
	t.uploads = append(t.uploads, object)
	if t.err != nil {
		return storage.Transfer{}, t.err
	}
	return storage.Transfer{
		URL: "https://storage.example/" + object.StorageKey, Method: "PUT", ExpiresAt: expiresAt,
	}, nil
}

func (t *transfers) IssueDownload(object domain.Object, expiresAt time.Time) (storage.Transfer, error) {
	t.downloads = append(t.downloads, object)
	return storage.Transfer{
		URL: "https://storage.example/" + object.StorageKey, Method: "GET", ExpiresAt: expiresAt,
	}, nil
}

type sink struct{ entries []audit.Entry }

func (s *sink) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

type unitOfWork struct {
	writes int
	reads  int
	scopes []persistence.Scope
}

func (u *unitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.writes++
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.reads++
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

type ids struct{ next shared.ID }

func (i *ids) NewID() shared.ID { return i.next }

// --- staging ------------------------------------------------------------------------------

func stagingHarness() (RequestMediaUpload, *objects, *transfers, *sink) {
	records, targets, trail := newObjects(), &transfers{}, &sink{}
	return RequestMediaUpload{
		Objects:    records,
		Transfers:  targets,
		Audit:      trail,
		UnitOfWork: &unitOfWork{},
		Clock:      clock.Fixed(now),
		IDs:        &ids{next: mintedID},
		Config:     config(1 << 20),
	}, records, targets, trail
}

func TestStagingWritesTheRecordAndAnswersWithTheTarget(t *testing.T) {
	handler, records, targets, trail := stagingHarness()

	staged, err := handler.Execute(t.Context(), actor(mediaWrite), UploadCommand{
		FileName: "plan.png", ClaimedType: "image/png", Size: 32, Usage: domain.UsageCover,
	})
	if err != nil {
		t.Fatalf("staging failed: %v", err)
	}

	if staged.Object.Status != domain.StatusPending {
		t.Errorf("the staged object is %s, want PENDING", staged.Object.Status)
	}
	// The key is minted, tenant-prefixed and carries nothing the client typed (T-11).
	if !strings.HasPrefix(staged.Object.StorageKey, "media/"+tenantID.String()+"/") {
		t.Errorf("the storage key is %q", staged.Object.StorageKey)
	}
	if strings.Contains(staged.Object.StorageKey, "plan.png") {
		t.Error("the file name became part of the storage key")
	}
	if records.inserted != 1 {
		t.Errorf("%d records written, want 1", records.inserted)
	}
	if len(targets.uploads) != 1 || staged.Transfer.Method != "PUT" {
		t.Errorf("the upload target is %+v", staged.Transfer)
	}
	if want := now.Add(UploadWindow); staged.Transfer.ExpiresAt != want {
		t.Errorf("the target expires at %v, want %v", staged.Transfer.ExpiresAt, want)
	}

	if len(trail.entries) != 1 || trail.entries[0].Action != MediaStagedAction {
		t.Fatalf("the staging was not recorded: %+v", trail.entries)
	}
	// User content stays out of the trail (rule 10): who staged how many bytes of what kind, and
	// not what the file is called.
	if _, named := trail.entries[0].Changes["file_name"]; named {
		t.Error("the file name reached the audit trail")
	}
}

func TestStagingNeedsTheWriteScope(t *testing.T) {
	handler, records, _, _ := stagingHarness()

	_, err := handler.Execute(t.Context(), actor(mediaRead), UploadCommand{
		Size: 32, Usage: domain.UsageAttachment,
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if records.inserted != 0 {
		t.Error("a record was written despite the refusal")
	}
}

func TestStagingRefusesWhatTheInstallationWillNotAccept(t *testing.T) {
	handler, _, targets, _ := stagingHarness()

	_, err := handler.Execute(t.Context(), actor(mediaWrite), UploadCommand{
		Size: 1<<20 + 1, Usage: domain.UsageAttachment,
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation refusal", err)
	}
	// Refused before anything was minted: a capability for an object that will never exist is one
	// nobody should be able to hold.
	if len(targets.uploads) != 0 {
		t.Error("an upload target was minted for a refused staging")
	}
}

// --- confirmation -------------------------------------------------------------------------

func confirmHarness(limit int64) (ConfirmMediaUpload, *objects, *store, *guard, *sink) {
	records, bytesOf, judge, trail := newObjects(), newStore(), &guard{judged: "image/png"}, &sink{}
	return ConfirmMediaUpload{
		Objects:    records,
		Store:      bytesOf,
		Guard:      judge,
		Audit:      trail,
		UnitOfWork: &unitOfWork{},
		Clock:      clock.Fixed(now),
		Config:     config(limit),
	}, records, bytesOf, judge, trail
}

// staged puts a PENDING record and its bytes where a confirmation will find them.
func staged(records *objects, bytesOf *store, claimed string, declared int64, body []byte) domain.Object {
	object := domain.Object{
		ID: mintedID, TenantID: tenantID,
		StorageKey: "media/" + tenantID.String() + "/" + mintedID.String(),
		FileName:   "plan.png", ContentType: claimed, ByteSize: declared,
		Usage: domain.UsageCover, Status: domain.StatusPending,
		CreatedBy: accountID, CreatedAt: now,
	}
	records.stored[object.ID] = object
	if body != nil {
		bytesOf.content[object.StorageKey] = body
		bytesOf.types[object.StorageKey] = claimed
	}
	return object
}

func TestConfirmingSealsTheObjectWithWhatTheBytesTurnedOutToBe(t *testing.T) {
	handler, records, bytesOf, judge, trail := confirmHarness(1 << 20)
	staged(records, bytesOf, "image/jpeg", int64(len(pngBytes)), pngBytes)

	sealed, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if err != nil {
		t.Fatalf("confirming failed: %v", err)
	}

	if sealed.Status != domain.StatusReady {
		t.Errorf("the object is %s, want READY", sealed.Status)
	}
	// The judged type, never the claim (T-11) - and the claim is what the guard was handed to
	// judge it against.
	if sealed.ContentType != "image/png" {
		t.Errorf("the object is stored as %q, want the judged type", sealed.ContentType)
	}
	if len(judge.asked) != 1 || judge.asked[0] != "image/jpeg" {
		t.Errorf("the guard was asked about %v, want the claim made at staging", judge.asked)
	}
	if sealed.ByteSize != int64(len(pngBytes)) {
		t.Errorf("the object measures %d bytes", sealed.ByteSize)
	}
	if len(records.sealed) != 1 {
		t.Errorf("%d seals written, want 1", len(records.sealed))
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != MediaConfirmedAction {
		t.Errorf("the confirmation was not recorded: %+v", trail.entries)
	}
}

// The acceptance criterion: an upload confirmed twice yields one media object - by state rather
// than by an idempotency key, so a client that retried without one gets the same answer.
func TestConfirmingTwiceYieldsOneObject(t *testing.T) {
	handler, records, bytesOf, _, trail := confirmHarness(1 << 20)
	staged(records, bytesOf, "image/png", int64(len(pngBytes)), pngBytes)

	first, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if err != nil {
		t.Fatalf("the first confirmation failed: %v", err)
	}
	second, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if err != nil {
		t.Fatalf("the second confirmation failed: %v", err)
	}

	if first.ID != second.ID || second.Status != domain.StatusReady {
		t.Errorf("the second confirmation answered %+v", second)
	}
	if len(records.sealed) != 1 {
		t.Errorf("%d seals written, want 1 - the second changed nothing", len(records.sealed))
	}
	if len(trail.entries) != 1 {
		t.Errorf("%d audit entries, want 1 - nothing happened the second time", len(trail.entries))
	}
}

func TestAConfirmationHoldsTheSizeAgainstWhatWasDeclared(t *testing.T) {
	cases := []struct {
		name     string
		declared int64
		limit    int64
		want     string
	}{
		{"a declaration that turned out wrong", 999, 1 << 20, "media.size_mismatch"},
		// The bucket checked nothing on the way in: a presigned PUT never passed this server, so
		// the limit is enforced here or nowhere (T-17).
		{"past the installation's limit", int64(len(pngBytes)), 8, "media.too_large"},
	}

	for _, c := range cases {
		handler, records, bytesOf, _, _ := confirmHarness(c.limit)
		object := staged(records, bytesOf, "image/png", c.declared, pngBytes)

		_, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
		if got := shared.AsError(err).DetailCode; got != c.want {
			t.Errorf("%s: %q, want %s", c.name, got, c.want)
		}
		// A refusal takes the staged bytes with it: leaving them would leave a file this
		// installation has judged unacceptable sitting in its store.
		if len(bytesOf.deleted) != 1 || bytesOf.deleted[0] != object.StorageKey {
			t.Errorf("%s: the refused bytes were not removed: %v", c.name, bytesOf.deleted)
		}
		if len(records.sealed) != 0 {
			t.Errorf("%s: the object was sealed despite the refusal", c.name)
		}
	}
}

func TestConfirmingWithoutBytesSaysSo(t *testing.T) {
	handler, records, bytesOf, _, _ := confirmHarness(1 << 20)
	staged(records, bytesOf, "image/png", 32, nil)

	_, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if got := shared.AsError(err).DetailCode; got != "media.content_missing" {
		t.Errorf("detail code %q, want media.content_missing", got)
	}
}

// A refused guard is a refused confirmation, and the object stays PENDING - which is what keeps
// "nothing unjudged is ever attached" true (T-11).
func TestAGuardRefusalStopsTheConfirmation(t *testing.T) {
	handler, records, bytesOf, judge, _ := confirmHarness(1 << 20)
	staged(records, bytesOf, "text/html", int64(len(pngBytes)), pngBytes)
	judge.err = shared.ErrValidation.WithDetail("media.type_mismatch")

	_, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if got := shared.AsError(err).DetailCode; got != "media.type_mismatch" {
		t.Errorf("detail code %q, want the guard's refusal", got)
	}
	if records.stored[mintedID].Status != domain.StatusPending {
		t.Error("a refused object was sealed anyway")
	}
}

// Somebody else's staging is not theirs to finish, and they are not told it exists (T-04).
func TestOnlyTheAccountThatStagedTheUploadConfirmsIt(t *testing.T) {
	handler, records, bytesOf, _, _ := confirmHarness(1 << 20)
	object := staged(records, bytesOf, "image/png", int64(len(pngBytes)), pngBytes)
	object.CreatedBy = strangerA
	records.stored[object.ID] = object

	_, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
	if got := shared.AsError(err).DetailCode; got != "media.not_found" {
		t.Errorf("detail code %q - it must read like an object that is not there", got)
	}
}

// --- the content routes ---------------------------------------------------------------------

func contentHarness() (MediaContent, *objects, *store, *guard) {
	records, bytesOf, judge := newObjects(), newStore(), &guard{judged: "image/png"}
	return MediaContent{
		Objects:    records,
		Store:      bytesOf,
		Guard:      judge,
		UnitOfWork: &unitOfWork{},
		Config:     config(1 << 20),
	}, records, bytesOf, judge
}

func TestTheContentRouteStoresTheJudgedBytesAndSealsNothing(t *testing.T) {
	handler, records, bytesOf, _ := contentHarness()
	object := staged(records, bytesOf, "image/png", int64(len(pngBytes)), nil)

	grant := Grant{TenantID: tenantID, MediaID: mintedID}
	if err := handler.Receive(t.Context(), grant, bytes.NewReader(pngBytes)); err != nil {
		t.Fatalf("receiving failed: %v", err)
	}

	if len(bytesOf.puts) != 1 {
		t.Fatalf("%d objects stored, want 1", len(bytesOf.puts))
	}
	put := bytesOf.puts[0]
	if put.Key != object.StorageKey || put.ContentType != "image/png" {
		t.Errorf("stored under %q as %q", put.Key, put.ContentType)
	}
	if put.Size != object.ByteSize {
		t.Errorf("stored with size %d, want the declared one", put.Size)
	}
	// The route moves bytes; the confirmation is what decides READY. A route that sealed would
	// make the confirmation optional, and the judgement with it.
	if records.stored[mintedID].Status != domain.StatusPending {
		t.Error("the content route sealed the object")
	}
}

func TestTheContentRouteRunsInTheTenantTheTokenNamed(t *testing.T) {
	handler, records, bytesOf, _ := contentHarness()
	staged(records, bytesOf, "image/png", int64(len(pngBytes)), nil)
	uow := handler.UnitOfWork.(*unitOfWork)

	grant := Grant{TenantID: tenantID, MediaID: mintedID}
	if err := handler.Receive(t.Context(), grant, bytes.NewReader(pngBytes)); err != nil {
		t.Fatalf("receiving failed: %v", err)
	}

	if len(uow.scopes) == 0 || uow.scopes[0].TenantID != tenantID {
		t.Errorf("the transaction ran as %+v, want the token's tenant", uow.scopes)
	}
	// There is no account: the holder of the URL is whoever the staging handed it to, and the byte
	// movement records nothing of its own.
	if !uow.scopes[0].ActorID.IsZero() {
		t.Errorf("the byte route claimed an account: %v", uow.scopes[0].ActorID)
	}
}

// A capability minted before the object was sealed is not a way to rewrite it afterwards.
func TestTheContentRouteRefusesToOverwriteASealedObject(t *testing.T) {
	handler, records, bytesOf, _ := contentHarness()
	object := staged(records, bytesOf, "image/png", int64(len(pngBytes)), pngBytes)
	object.Status = domain.StatusReady
	records.stored[object.ID] = object

	err := handler.Receive(t.Context(), Grant{TenantID: tenantID, MediaID: mintedID},
		bytes.NewReader(pngBytes))
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error %v, want a conflict", err)
	}
}

func TestNothingUnjudgedIsEverServed(t *testing.T) {
	handler, records, bytesOf, _ := contentHarness()
	staged(records, bytesOf, "image/png", int64(len(pngBytes)), pngBytes)

	// PENDING: the bytes are there and nothing has decided what they are.
	_, err := handler.Send(t.Context(), Grant{TenantID: tenantID, MediaID: mintedID})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("an unjudged object was served: %v", err)
	}

	object := records.stored[mintedID]
	object.Status = domain.StatusReady
	records.stored[mintedID] = object

	served, err := handler.Send(t.Context(), Grant{TenantID: tenantID, MediaID: mintedID})
	if err != nil {
		t.Fatalf("a sealed object was refused: %v", err)
	}
	defer func() { _ = served.Content.Content.Close() }()

	// The file name comes off the record, so there is nothing in the URL a holder could tamper
	// with to change what the download is called (T-11).
	if served.FileName != "plan.png" {
		t.Errorf("the download is named %q", served.FileName)
	}
	body, err := io.ReadAll(served.Content.Content)
	if err != nil || !bytes.Equal(body, pngBytes) {
		t.Errorf("the bytes came back as %d bytes (%v)", len(body), err)
	}
}

// A marked object is on its way out; a capability minted before that is not a way back in.
func TestAMarkedObjectIsGoneForTheContentRoutes(t *testing.T) {
	handler, records, bytesOf, _ := contentHarness()
	object := staged(records, bytesOf, "image/png", int64(len(pngBytes)), pngBytes)
	object.Status = domain.StatusReady
	marked := now
	object.DeletedAt = &marked
	records.stored[object.ID] = object

	if _, err := handler.Send(t.Context(), Grant{TenantID: tenantID, MediaID: mintedID}); !errors.Is(
		err, shared.ErrNotFound,
	) {
		t.Errorf("a marked object was served: %v", err)
	}
}

// --- the catalogue ---------------------------------------------------------------------------

// Both operations reach all three channels through the catalogue, and the projection they answer
// with is what a client reads. The typed Execute above is the same code; this is the path REST,
// MCP and an automation rule actually take (arc42 §4).
func TestBothOperationsAnswerThroughTheCatalogue(t *testing.T) {
	staging, records, _, _ := stagingHarness()
	stage := staging.Descriptor()

	if stage.Name != RequestMediaUploadName || stage.TokenScope != mediaWrite {
		t.Errorf("the staging descriptor is %+v", stage)
	}
	if stage.Audit.Action != MediaStagedAction || !stage.Audit.Required {
		t.Errorf("the staging declares audit %+v", stage.Audit)
	}
	// A media object is not a work item and writes no history; the reason travels with it rather
	// than the gate having to guess.
	if stage.Activity.Verb != "" || stage.Activity.Exempt == "" {
		t.Errorf("the staging declares activity %+v", stage.Activity)
	}

	out, err := stage.Handler.Invoke(t.Context(), actor(mediaWrite), map[string]any{
		"usage": "ATTACHMENT", "size": 32, "file_name": "notes.txt", "content_type": "text/plain",
	})
	if err != nil {
		t.Fatalf("staging through the catalogue failed: %v", err)
	}
	if out["status"] != string(domain.StatusPending) || out["usage"] != string(domain.UsageAttachment) {
		t.Errorf("the projection is %+v", out)
	}
	if out["file_name"] != "notes.txt" {
		t.Errorf("the projection names the file %v", out["file_name"])
	}
	// Absent rather than null: the object has no checksum, and nothing computes one yet.
	if out["checksum"] != nil {
		t.Errorf("the projection carries a checksum: %v", out["checksum"])
	}
	target, ok := out["upload"].(map[string]any)
	if !ok || target["method"] != "PUT" {
		t.Fatalf("the projection carries no upload target: %+v", out["upload"])
	}
	if _, sealed := out["download"]; sealed {
		t.Error("a PENDING object was given a download target")
	}

	// And the confirmation, over the record the staging just wrote.
	confirming, _, bytesOf, _, _ := confirmHarness(1 << 20)
	confirming.Objects = records
	stored := records.stored[mintedID]
	bytesOf.content[stored.StorageKey] = pngBytes
	bytesOf.types[stored.StorageKey] = stored.ContentType

	confirm := confirming.Descriptor()
	if confirm.Name != ConfirmMediaUploadName || confirm.Audit.Action != MediaConfirmedAction {
		t.Errorf("the confirmation descriptor is %+v", confirm)
	}

	sealedOut, err := confirm.Handler.Invoke(t.Context(), actor(mediaWrite),
		map[string]any{"media_id": mintedID.String()})
	if err != nil {
		t.Fatalf("confirming through the catalogue failed: %v", err)
	}
	if sealedOut["status"] != string(domain.StatusReady) {
		t.Errorf("the sealed projection is %+v", sealedOut)
	}
	// The staged answer carried a target; the sealed one does not - the upload is over.
	if _, staged := sealedOut["upload"]; staged {
		t.Error("a READY object was given an upload target")
	}
}

// A usage the model does not know is refused by name, before anything is written.
func TestTheCatalogueRefusesAUsageNobodyDeclared(t *testing.T) {
	staging, records, _, _ := stagingHarness()

	_, err := staging.Descriptor().Handler.Invoke(t.Context(), actor(mediaWrite), map[string]any{
		"usage": "AVATAR", "size": 32,
	})
	if got := shared.AsError(err).DetailCode; got != "media.usage_unknown" {
		t.Errorf("detail code %q, want media.usage_unknown", got)
	}
	if records.inserted != 0 {
		t.Error("a record was written for a usage nobody declared")
	}
}

func TestConfirmingNeedsAnIdentifier(t *testing.T) {
	handler, _, _, _, _ := confirmHarness(1 << 20)

	_, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{})
	if got := shared.AsError(err).DetailCode; got != "media.media_id_required" {
		t.Errorf("detail code %q, want media.media_id_required", got)
	}
}

func TestConfirmingSomethingThatIsNotThere(t *testing.T) {
	handler, _, _, _, _ := confirmHarness(1 << 20)

	_, err := handler.Execute(t.Context(), actor(mediaWrite), ConfirmCommand{MediaID: mintedID})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("error %v, want not found", err)
	}
}

// The content routes answer the same way for an object that is not there as for one of another
// tenant: a capability naming an identifier that means nothing here is not an oracle (T-04).
func TestTheContentRouteAnswersNotFoundForAnythingItCannotReach(t *testing.T) {
	handler, _, _, _ := contentHarness()

	err := handler.Receive(t.Context(), Grant{TenantID: tenantID, MediaID: mintedID},
		bytes.NewReader(pngBytes))
	if got := shared.AsError(err).DetailCode; got != "media.not_found" {
		t.Errorf("receiving: detail code %q, want media.not_found", got)
	}

	if _, err := handler.Send(t.Context(), Grant{TenantID: tenantID, MediaID: mintedID}); shared.
		AsError(err).DetailCode != "media.not_found" {
		t.Errorf("sending: %v", err)
	}
}

// A store that has lost the bytes of a sealed object is not a different answer to the client than
// an object that is not there: both are gone, and one of them is an operational problem.
func TestServingAnObjectWhoseBytesAreGone(t *testing.T) {
	handler, records, bytesOf, _ := contentHarness()
	object := staged(records, bytesOf, "image/png", int64(len(pngBytes)), nil)
	object.Status = domain.StatusReady
	records.stored[object.ID] = object

	if _, err := handler.Send(t.Context(), Grant{TenantID: tenantID, MediaID: mintedID}); !errors.Is(
		err, shared.ErrNotFound,
	) {
		t.Errorf("error %v, want not found", err)
	}
}

// A guard refusal on the way in stops the bytes before the store sees them.
func TestTheContentRoutePassesTheGuardsRefusalOn(t *testing.T) {
	handler, records, bytesOf, judge := contentHarness()
	staged(records, bytesOf, "text/html", int64(len(pngBytes)), nil)
	judge.err = shared.ErrValidation.WithDetail("media.type_mismatch")

	err := handler.Receive(t.Context(), Grant{TenantID: tenantID, MediaID: mintedID},
		bytes.NewReader(pngBytes))
	if got := shared.AsError(err).DetailCode; got != "media.type_mismatch" {
		t.Errorf("detail code %q, want the guard's refusal", got)
	}
	if len(bytesOf.puts) != 0 {
		t.Error("refused bytes reached the store")
	}
}

// storageQuotaFake refuses when told to - the resolution is the quota engine's; this package
// owes that the wall holds the staging door (H-08).
type storageQuotaFake struct{ refused error }

func (q storageQuotaFake) MediaBytes(context.Context, string, int64) error { return q.refused }

func TestTheStorageCeilingHoldsTheStagingDoor(t *testing.T) {
	handler, records, _, _ := stagingHarness()
	handler.Quota = storageQuotaFake{refused: shared.ErrValidation.WithDetail("capacity.media_bytes")}

	_, err := handler.Execute(t.Context(), actor(mediaWrite), UploadCommand{
		FileName: "plan.png", ClaimedType: "image/png", Size: 32, Usage: domain.UsageCover,
	})

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "capacity.media_bytes" {
		t.Errorf("answer %v, want capacity.media_bytes", err)
	}
	if len(records.stored) != 0 {
		t.Error("an object was staged past the wall")
	}
}
