// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	storageport "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	storage "github.com/Jersyfi/hubtask/infrastructure/storage"
)

// The conformance suite of C-05: both adapters answer the port identically, the S3 one proved
// against a real MinIO - whose strict SigV4 validation is also what proves the hand-written
// signer. One suite run twice, so the two stores cannot drift apart in behaviour.

// minioImage is overridable the way the PostgreSQL image is (test/dbtest), so the support matrix
// can vary it without a code change.
func minioImage() string {
	if image := os.Getenv("HUBTASK_TEST_MINIO_IMAGE"); image != "" {
		return image
	}
	return "minio/minio:latest"
}

// startMinIO runs one MinIO for this test and returns the adapter pointed at it.
func startMinIO(t *testing.T) *storage.S3Storage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: minioImage(),
			Env: map[string]string{
				"MINIO_ROOT_USER":     "conformance",
				"MINIO_ROOT_PASSWORD": "conformance-secret",
			},
			Cmd:          []string{"server", "/data"},
			ExposedPorts: []string{"9000/tcp"},
			WaitingFor: wait.ForHTTP("/minio/health/ready").
				WithPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("starting MinIO: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "9000/tcp")
	if err != nil {
		t.Fatal(err)
	}

	store, err := storage.NewS3Storage(env.StorageConfig{
		Kind:     env.StorageS3,
		Endpoint: fmt.Sprintf("http://%s:%s", host, port.Port()),
		// us-east-1, deliberately: CreateBucket sends no location constraint, and this is the
		// region for which none is needed.
		Region:       "us-east-1",
		Bucket:       "hubtask-media",
		AccessKey:    secret.New("conformance"),
		SecretKey:    secret.New("conformance-secret"),
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
	t.Run("s3 against MinIO", func(t *testing.T) {
		conformance(t, startMinIO(t))
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
