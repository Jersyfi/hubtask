// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	storageport "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	storage "github.com/Jersyfi/hubtask/infrastructure/storage"
)

// The three runs C-06 names as its acceptance, against the real database and - for the one that is
// about somebody else's server - a real MinIO. Each of them is a sentence in the task that would
// otherwise be a claim.

// pngBytes is a real PNG signature and enough after it to be worth storing.
var pngBytes = append(
	[]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0}, 56)...,
)

// stagedForUpload stages an object that declares its size and claims nothing about its type, so
// that the confirmation's judgement is the sniff alone - which is what T-11 is about.
func stagedForUpload(ctx context.Context, t *testing.T, size int64) media.Object {
	t.Helper()

	object, err := media.NewPendingObject(media.NewObjectInput{
		ID: freshID(t), TenantID: tenantA, FileName: "beach.png",
		DeclaredSize: size, SizeLimit: 1 << 20,
		Usage: media.UsageAttachment, CreatedBy: authorA, Now: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return mediaRepo().Insert(ctx, object)
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	return object
}

// countingSink is the audit trail as these tests need it: how many entries, and which actions. The
// real one is proved in audit_trail_test.go, and writing through it here would make a test about
// media a test about the hash chain.
type countingSink struct{ actions []audit.Action }

func (s *countingSink) Append(_ context.Context, entry audit.Entry) error {
	s.actions = append(s.actions, entry.Action)
	return nil
}

func mediaActor(tenant, account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account,
		AccountName: "Anna", Scopes: []string{"media:write", "media:read"},
	}
}

func mediaConfig() env.Config {
	return env.Config{Request: env.RequestConfig{MaxUploadBytes: 1 << 20}}
}

// TestAnUploadConfirmedTwiceIsOneMediaObject is the acceptance sentence about idempotence.
//
// By state rather than by an idempotency key, which is the stronger promise: a client that retried
// without a key gets the same answer, and the second confirmation reads the bytes back no second
// time and writes no second audit entry.
func TestAnUploadConfirmedTwiceIsOneMediaObject(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	staged := stagedForUpload(ctx, t, int64(len(pngBytes)))

	// The bytes, put where the presigned upload would have put them.
	store := storage.NewLocalStorage(t.TempDir())
	if err := store.Put(ctx, storageport.Upload{
		Key: staged.StorageKey, Content: bytes.NewReader(pngBytes),
		Size: int64(len(pngBytes)), ContentType: "image/png",
	}); err != nil {
		t.Fatalf("staging the bytes: %v", err)
	}

	trail := &countingSink{}
	confirm := mediaservice.ConfirmMediaUpload{
		Objects: mediaRepo(), Store: store, Guard: storage.NewUploadGuard(), Audit: trail,
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock:      clock.Fixed(created), Config: mediaConfig(),
	}
	actor := mediaActor(tenantA, authorA)

	first, err := confirm.Execute(ctx, actor, mediaservice.ConfirmCommand{MediaID: staged.ID})
	if err != nil {
		t.Fatalf("the first confirmation failed: %v", err)
	}
	second, err := confirm.Execute(ctx, actor, mediaservice.ConfirmCommand{MediaID: staged.ID})
	if err != nil {
		t.Fatalf("the second confirmation failed: %v", err)
	}

	if first.ID != second.ID || first.Status != media.StatusReady || second.Status != media.StatusReady {
		t.Fatalf("two confirmations produced %+v and %+v", first, second)
	}
	// Nothing was claimed, so what is stored is what the bytes turned out to be - which is the
	// judgement T-11 asks for, and the second confirmation does not make it again.
	if second.ContentType != "image/png" {
		t.Errorf("the sealed type is %q, want the sniffed one", second.ContentType)
	}
	if stored := findMedia(ctx, t, tenantA, staged.ID); stored.ByteSize != int64(len(pngBytes)) {
		t.Errorf("the sealed size is %d, want %d", stored.ByteSize, len(pngBytes))
	}
	if len(trail.actions) != 1 {
		t.Errorf("the trail records %v, want one confirmation", trail.actions)
	}
}

// TestAPurgedItemLosesItsReferencesAndTheJobReclaimsTheObject is the acceptance sentence about the
// deletion path: purging an entry drops its references, and the reconciliation removes what nothing
// points at and writes the journal entry the retention model requires (data-protection.md §5).
func TestAPurgedItemLosesItsReferencesAndTheJobReclaimsTheObject(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	object := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment))

	store := storage.NewLocalStorage(t.TempDir())
	if err := store.Put(ctx, storageport.Upload{
		Key: object.StorageKey, Content: bytes.NewReader(pngBytes),
		Size: int64(len(pngBytes)), ContentType: "image/png",
	}); err != nil {
		t.Fatalf("staging the bytes: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := mediaRepo().Add(ctx, task, object.ID, shared.HLC{}); err != nil {
			return err
		}
		return mediaRepo().AdjustRefCount(ctx, object.ID, 1)
	}); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	// The purge. `item_attachment.item_id` is ON DELETE CASCADE, so the link goes with the entry -
	// and the counter is left saying one, which is exactly the drift the recount exists for.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := postgres.NewTrashRepository(pageCursors()).PurgeItems(ctx, []shared.ID{task})
		return err
	}); err != nil {
		t.Fatalf("purging the entry: %v", err)
	}
	if stored := findMedia(ctx, t, tenantA, object.ID); stored.RefCount != 1 {
		t.Fatalf("ref_count %d after the purge, want the stale 1 the recount corrects", stored.RefCount)
	}

	// The first pass recounts to zero and stamps, and marks nothing at all. This is the pass that
	// used to take the object away: at the moment it runs, the row has just lost its last
	// reference, and that is indistinguishable from an upload waiting for its first one.
	reconcile := mediaservice.ReconcileMedia{
		Objects: mediaRepo(), Store: store,
		Removals:   postgres.NewLifecycleRepository(),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock:      clock.Fixed(created.Add(time.Hour)),
		Config: env.MediaConfig{
			StagingGrace: 24 * time.Hour, UnreferencedGrace: time.Hour,
			OrphanGrace: time.Hour, BatchSize: 10,
		},
		Retention: env.RetentionConfig{TombstoneWindow: 90 * 24 * time.Hour},
	}
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantA}

	if _, err := reconcile.Execute(ctx, actor); err != nil {
		t.Fatalf("the first pass failed: %v", err)
	}
	counted := findMedia(ctx, t, tenantA, object.ID)
	if counted.RefCount != 0 || counted.DeletedAt != nil {
		t.Fatalf("after the recount the object is %+v", counted)
	}

	// The second pass, once nothing has pointed at it for the grace: now it is marked. Still
	// nothing is reclaimed - the bytes wait out their own window after the marking.
	reconcile.Clock = clock.Fixed(created.Add(4 * time.Hour))
	if _, err := reconcile.Execute(ctx, actor); err != nil {
		t.Fatalf("the second pass failed: %v", err)
	}
	marked := findMedia(ctx, t, tenantA, object.ID)
	if marked.DeletedAt == nil {
		t.Fatalf("after the grace the object is %+v", marked)
	}

	// The third pass: the bytes go, then the row, and the journal entry is written in the same
	// transaction as the removal.
	reconcile.Clock = clock.Fixed(created.Add(7 * time.Hour))
	outcome, err := reconcile.Execute(ctx, actor)
	if err != nil {
		t.Fatalf("the third pass failed: %v", err)
	}
	// At least this one. The pass is tenant-wide and the package shares one database, so an
	// unreferenced object another test finished with is reclaimed by the same pass - which is the
	// job doing its work rather than a surprise. What this test is about is this object.
	if outcome.Reclaimed < 1 {
		t.Fatalf("the pass reclaimed %d", outcome.Reclaimed)
	}

	if _, err := store.Get(ctx, object.StorageKey); err == nil {
		t.Error("the bytes are still in storage")
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := mediaRepo().Find(ctx, object.ID)
		return err
	}); err == nil {
		t.Error("the record is still there")
	}
	assertJournalled(ctx, t, object.ID)
}

// assertJournalled reads the deletion journal directly. Through the pool rather than a repository,
// because nothing reads this table in production: it exists so that a restore from backup cannot
// bring back what was deleted (ADR-0020 §6, backup-restore.md §6).
func assertJournalled(ctx context.Context, t *testing.T, mediaID shared.ID) {
	t.Helper()

	var reason string
	err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT reason FROM deletion_journal
		WHERE tenant_id = $1 AND entity = 'media_object' AND entity_id = $2`,
		tenantA.String(), mediaID.String()).Scan(&reason)
	if err != nil {
		t.Fatalf("reading the deletion journal: %v", err)
	}
	if reason != "RETENTION" {
		t.Errorf("the journal records %q, want the machinery rather than a person", reason)
	}

	var tombstones int
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT count(*) FROM tombstone
		WHERE tenant_id = $1 AND entity = 'media_object' AND entity_id = $2`,
		tenantA.String(), mediaID.String()).Scan(&tombstones); err != nil {
		t.Fatalf("reading the tombstones: %v", err)
	}
	if tombstones != 1 {
		t.Errorf("%d tombstones, want the one that stops a device recreating the row", tombstones)
	}
}

// TestAPresignedURLWorksUntilItExpires is the acceptance sentence about the presigned upload, and
// the expiry is a test rather than a claim: one URL, used twice, against a server that is not this
// one - once inside its window and once after it has closed.
func TestAPresignedURLWorksUntilItExpires(t *testing.T) {
	ctx := context.Background()
	store := startMinIO(t)

	object := media.Object{
		ID:         shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e1"),
		TenantID:   tenantA,
		StorageKey: "media/presigned/object",
		FileName:   "beach.png",
	}

	// Two seconds, which is what makes the second half of this test cheap. The window a real
	// staging mints is fifteen minutes (media.UploadWindow); what is under test is that the window
	// is enforced at all, and by whom.
	const window = 2 * time.Second
	target, err := store.IssueUpload(object, time.Now().Add(window))
	if err != nil {
		t.Fatalf("minting the target: %v", err)
	}

	if status := put(ctx, t, target.URL, pngBytes); status != http.StatusOK {
		t.Fatalf("the presigned upload answered %d", status)
	}

	// Refused by storage rather than by this server, which is the whole point of the presigned
	// flow: the bytes never pass through here, so the expiry is the only thing standing between
	// the URL and whoever holds it.
	time.Sleep(window + time.Second)
	if status := put(ctx, t, target.URL, pngBytes); status != http.StatusForbidden {
		t.Errorf("an expired presigned upload answered %d, want 403", status)
	}
}

// put sends the bytes to a presigned URL and reports the status.
//
// A plain client on purpose: the point of the test is that somebody who is not this server can
// upload with nothing but the URL, and routing it through the guarded client would be proving the
// opposite.
func put(ctx context.Context, t *testing.T, url string, content []byte) int {
	t.Helper()

	request, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("the presigned upload could not be made: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}
