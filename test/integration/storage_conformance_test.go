// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	storageport "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	storage "github.com/Jersyfi/hubtask/infrastructure/storage"
	"github.com/Jersyfi/hubtask/test/s3test"
)

// The conformance suite of C-05: both adapters answer the port identically, the S3 one proved
// against a real S3-compatible server - whose strict SigV4 validation is also what proves the
// hand-written signer. One suite run twice, so the two stores cannot drift apart in behaviour.

// startS3 runs one S3-compatible server for this test and returns the adapter pointed at it.
//
// The server itself, and why it is no longer MinIO, is in test/s3test - one place now, where it
// used to be three copies of the same pin plus a fourth in scripts/pitr-drill.sh (#1029).
//
// The bucket is made by the adapter under test here, unlike in the backup suite: CreateBucket is
// part of the port this suite proves, and an operator's first run against a fresh bucket is the
// case it stands for.
func startS3(t *testing.T) *storage.S3Storage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container := s3test.Start(t, "")

	store, err := storage.NewS3Storage(env.StorageConfig{
		Kind:         env.StorageS3,
		Endpoint:     s3test.Endpoint(ctx, t, container),
		Region:       s3test.Region,
		Bucket:       "hubtask-media",
		AccessKey:    secret.New(s3test.AccessKey),
		SecretKey:    secret.New(s3test.SecretKey),
		UsePathStyle: true,
	}, 10*time.Second)
	if err != nil {
		t.Fatalf("building the adapter: %v", err)
	}
	if err := store.CreateBucket(ctx); err != nil {
		t.Fatalf("creating the bucket: %v", err)
	}
	return store
}

func TestObjectStoreConformance(t *testing.T) {
	t.Run("local", func(t *testing.T) {
		conformance(t, storage.NewLocalStorage(t.TempDir()))
	})
	t.Run("s3 against SeaweedFS", func(t *testing.T) {
		conformance(t, startS3(t))
	})
}

// conformance is the one behaviour both adapters owe the port.
func conformance(t *testing.T, store storageport.ObjectStore) {
	t.Helper()
	ctx := context.Background()

	t.Run("an object round trips with its type and size", func(t *testing.T) {
		content := []byte("a modest object")
		if err := store.Put(ctx, storageport.Upload{
			Key: "media/roundtrip", Content: bytes.NewReader(content),
			Size: int64(len(content)), ContentType: "application/pdf",
		}); err != nil {
			t.Fatalf("putting: %v", err)
		}

		object, err := store.Get(ctx, "media/roundtrip")
		if err != nil {
			t.Fatalf("getting: %v", err)
		}
		defer object.Content.Close()

		read, err := io.ReadAll(object.Content)
		if err != nil || !bytes.Equal(read, content) {
			t.Fatalf("read %q (%v)", read, err)
		}
		if object.ContentType != "application/pdf" {
			t.Errorf("type %q, want the stored one", object.ContentType)
		}
		if object.Size != int64(len(content)) {
			t.Errorf("size %d, want %d", object.Size, len(content))
		}
	})

	t.Run("a large object streams both ways intact", func(t *testing.T) {
		const size = 8 << 20
		seed := bytes.Repeat([]byte("hubtask-conformance-"), size/20+1)[:size]
		wantSum := sha256.Sum256(seed)

		if err := store.Put(ctx, storageport.Upload{
			Key: "media/large", Content: bytes.NewReader(seed), Size: size,
			ContentType: "application/octet-stream",
		}); err != nil {
			t.Fatalf("putting 8 MiB: %v", err)
		}

		object, err := store.Get(ctx, "media/large")
		if err != nil {
			t.Fatalf("getting 8 MiB: %v", err)
		}
		defer object.Content.Close()

		digest := sha256.New()
		copied, err := io.Copy(digest, object.Content)
		if err != nil || copied != size {
			t.Fatalf("streamed %d of %d bytes (%v)", copied, size, err)
		}
		if !bytes.Equal(digest.Sum(nil), wantSum[:]) {
			t.Fatal("the object came back different")
		}
	})

	t.Run("a second put replaces the object", func(t *testing.T) {
		for _, body := range []string{"first", "second"} {
			if err := store.Put(ctx, storageport.Upload{
				Key: "media/replaced", Content: strings.NewReader(body),
				Size: int64(len(body)), ContentType: "text/plain",
			}); err != nil {
				t.Fatal(err)
			}
		}
		object, err := store.Get(ctx, "media/replaced")
		if err != nil {
			t.Fatal(err)
		}
		defer object.Content.Close()
		if read, _ := io.ReadAll(object.Content); string(read) != "second" {
			t.Fatalf("read %q, want the replacement", read)
		}
	})

	t.Run("a missing object is not found", func(t *testing.T) {
		if _, err := store.Get(ctx, "media/never-there"); !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("answered %v", err)
		}
	})

	t.Run("deletion is complete and idempotent", func(t *testing.T) {
		if err := store.Put(ctx, storageport.Upload{
			Key: "media/doomed", Content: strings.NewReader("bytes"), Size: 5,
			ContentType: "text/plain",
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.Delete(ctx, "media/doomed"); err != nil {
			t.Fatalf("deleting: %v", err)
		}
		if _, err := store.Get(ctx, "media/doomed"); !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("the object survived: %v", err)
		}
		if err := store.Delete(ctx, "media/doomed"); err != nil {
			t.Fatalf("the repeat was refused: %v", err)
		}
	})

	t.Run("a walking key is refused", func(t *testing.T) {
		for _, key := range []string{"", "/lead", "a/../b", "a//b"} {
			if _, err := store.Get(ctx, key); shared.AsError(err).DetailCode != "storage.key_invalid" {
				t.Errorf("key %q answered %v", key, err)
			}
		}
	})

	t.Run("the judged type survives the guard and the store", func(t *testing.T) {
		svg := `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`
		inspection, err := storage.Inspect(strings.NewReader(svg), "image/svg+xml", 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Put(ctx, storageport.Upload{
			Key: "media/judged", Content: inspection.Content, Size: int64(len(svg)),
			ContentType: inspection.ContentType,
		}); err != nil {
			t.Fatal(err)
		}

		object, err := store.Get(ctx, "media/judged")
		if err != nil {
			t.Fatal(err)
		}
		defer object.Content.Close()
		if media.DeliveryFor(object.ContentType) != media.DispositionAttachment {
			t.Fatalf("the stored SVG serves as %q with a rendering path (SG-12)", object.ContentType)
		}
	})
}
