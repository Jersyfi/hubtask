// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
)

// BK-1. One suite, four adapters, and the same sentences from all of them - which is the only
// thing that makes the port a port rather than four unrelated pieces of code.

func TestBackupTargetConformance(t *testing.T) {
	t.Run("local", func(t *testing.T) {
		root := t.TempDir()
		conformance(t, open(t, registry(t, root), specFor(backup.KindLocal,
			backup.TargetConfig{"path": "targets/one"}, nil)))
	})

	t.Run("s3 against MinIO", func(t *testing.T) {
		endpoint := startMinIO(t)
		conformance(t, open(t, registry(t, ""), specFor(backup.KindS3,
			backup.TargetConfig{
				"bucket": "hubtask-backups", "endpoint": endpoint,
				"region": "us-east-1", "path": "instance",
			},
			map[string]string{
				"access_key": "conformance", "secret_key": "conformance-secret",
			})))
	})

	t.Run("webdav against Apache", func(t *testing.T) {
		base := startWebDAV(t)
		conformance(t, open(t, registry(t, ""), specFor(backup.KindWebDAV,
			backup.TargetConfig{"url": base}, nil)))
	})

	t.Run("sftp against OpenSSH", func(t *testing.T) {
		address, hostKey := startSFTP(t)
		host, sshPort, _ := strings.Cut(address, ":")
		conformance(t, open(t, registry(t, ""), specFor(backup.KindSFTP,
			backup.TargetConfig{
				"host": host, "port": sshPort, "path": "/backups",
				"username": "hubtask", "host_key": hostKey,
			},
			map[string]string{"password": "conformance-secret"})))
	})
}

func open(t *testing.T, adapters backupstorage.Registry, spec port.Spec) port.Store {
	t.Helper()
	store, err := adapters.Open(context.Background(), spec)
	if err != nil {
		t.Fatalf("opening the target: %v", err)
	}
	return store
}

// conformance is the behaviour every adapter owes the port, in the order a backup actually uses
// it: write, list, read, verify, delete - and then the same again, because a backup runs on a
// schedule and the second run must behave like the first.
func conformance(t *testing.T, store port.Store) {
	t.Helper()
	ctx := context.Background()

	const key = "2026/08/26/archive-001.hbk"
	content := []byte("a small archive, and the bytes have to come back exactly as they went")

	t.Run("a fresh target lists nothing", func(t *testing.T) {
		entries, err := store.List(ctx, "")
		if err != nil {
			t.Fatalf("listing a fresh target: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("a fresh target lists %v", entries)
		}
	})

	t.Run("an archive is written and comes back byte for byte", func(t *testing.T) {
		written, err := store.Put(ctx, key, bytes.NewReader(content))
		if err != nil {
			t.Fatalf("writing: %v", err)
		}
		if written != int64(len(content)) {
			t.Fatalf("wrote %d bytes, want %d", written, len(content))
		}

		stream, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		defer func() { _ = stream.Close() }()

		read, err := io.ReadAll(stream)
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		if !bytes.Equal(read, content) {
			t.Fatalf("read %q", read)
		}
	})

	t.Run("the listing carries the whole key and the size", func(t *testing.T) {
		entries, err := store.List(ctx, "")
		if err != nil {
			t.Fatalf("listing: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("%d entries: %v", len(entries), entries)
		}
		if entries[0].Key != key {
			t.Fatalf("the key came back as %q - it has to be usable with Get", entries[0].Key)
		}
		if entries[0].Size != int64(len(content)) {
			t.Fatalf("the listing says %d bytes", entries[0].Size)
		}
		// The date is what generational retention sorts on, so an adapter that cannot answer it
		// would make every archive look the same age.
		if entries[0].ModifiedAt.IsZero() {
			t.Fatal("the listing carries no date")
		}
	})

	t.Run("a prefix narrows the listing", func(t *testing.T) {
		if _, err := store.Put(ctx, "2026/09/01/archive-002.hbk", strings.NewReader("x")); err != nil {
			t.Fatalf("writing a second archive: %v", err)
		}

		august, err := store.List(ctx, "2026/08")
		if err != nil {
			t.Fatalf("listing a prefix: %v", err)
		}
		if len(august) != 1 || august[0].Key != key {
			t.Fatalf("the narrowed listing is %v", august)
		}

		everything, err := store.List(ctx, "")
		if err != nil {
			t.Fatalf("listing: %v", err)
		}
		if len(everything) != 2 {
			t.Fatalf("%d entries after two writes: %v", len(everything), everything)
		}
	})

	t.Run("verifying an archive needs no read", func(t *testing.T) {
		entry, err := store.Stat(ctx, key)
		if err != nil {
			t.Fatalf("measuring: %v", err)
		}
		if entry.Size != int64(len(content)) {
			t.Fatalf("it says %d bytes", entry.Size)
		}
		if entry.Key != key {
			t.Fatalf("it names %q", entry.Key)
		}
	})

	t.Run("an archive that is not there is not there", func(t *testing.T) {
		if _, err := store.Stat(ctx, "2026/08/26/nothing.hbk"); !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("measuring a missing archive: %v", err)
		}
		if _, err := store.Get(ctx, "2026/08/26/nothing.hbk"); !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("reading a missing archive: %v", err)
		}
		// Deleting what is not there succeeds: deletion is the state the caller asked for, and
		// the generational retention that calls it retries.
		if err := store.Delete(ctx, "2026/08/26/nothing.hbk"); err != nil {
			t.Fatalf("deleting a missing archive: %v", err)
		}
	})

	t.Run("an archive is replaced rather than appended to", func(t *testing.T) {
		replacement := []byte("the second archive, which is shorter")
		if _, err := store.Put(ctx, key, bytes.NewReader(replacement)); err != nil {
			t.Fatalf("rewriting: %v", err)
		}

		entry, err := store.Stat(ctx, key)
		if err != nil {
			t.Fatalf("measuring: %v", err)
		}
		if entry.Size != int64(len(replacement)) {
			t.Fatalf("after a rewrite it says %d bytes, want %d", entry.Size, len(replacement))
		}
	})

	t.Run("a key that walks is refused", func(t *testing.T) {
		for _, walking := range []string{"../outside.hbk", "/absolute.hbk", "a/../../outside.hbk"} {
			if _, err := store.Put(ctx, walking, strings.NewReader("x")); err == nil {
				t.Errorf("the key %q was written", walking)
			}
			if _, err := store.Get(ctx, walking); err == nil {
				t.Errorf("the key %q was read", walking)
			}
		}
	})

	t.Run("what was deleted is gone, and deleting again is fine", func(t *testing.T) {
		if err := store.Delete(ctx, key); err != nil {
			t.Fatalf("deleting: %v", err)
		}
		if _, err := store.Stat(ctx, key); !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("the deleted archive is still there: %v", err)
		}
		if err := store.Delete(ctx, key); err != nil {
			t.Fatalf("deleting again: %v", err)
		}

		left, err := store.List(ctx, "")
		if err != nil {
			t.Fatalf("listing: %v", err)
		}
		keys := make([]string, 0, len(left))
		for _, entry := range left {
			keys = append(keys, entry.Key)
		}
		if slices.Contains(keys, key) {
			t.Fatalf("the deleted archive is still listed: %v", keys)
		}
	})

	t.Run("an archive larger than one request", func(t *testing.T) {
		// Past the S3 adapter's part size, so the multipart path is the one under test - and
		// large enough on the other three that a chunked write loop is exercised rather than a
		// single buffer.
		size := int64(backupstorage.PartSize) + 4096
		if _, err := store.Put(ctx, "large.hbk", io.LimitReader(&pattern{}, size)); err != nil {
			t.Fatalf("writing a large archive: %v", err)
		}

		entry, err := store.Stat(ctx, "large.hbk")
		if err != nil {
			t.Fatalf("measuring the large archive: %v", err)
		}
		if entry.Size != size {
			t.Fatalf("the target says %d bytes, want %d", entry.Size, size)
		}

		stream, err := store.Get(ctx, "large.hbk")
		if err != nil {
			t.Fatalf("reading the large archive: %v", err)
		}
		defer func() { _ = stream.Close() }()

		// Compared against the same generator rather than against a copy in memory: holding the
		// archive twice would be the test doing what the adapter is written not to.
		if err := sameAs(stream, io.LimitReader(&pattern{}, size)); err != nil {
			t.Fatalf("the large archive came back changed: %v", err)
		}
		if err := store.Delete(ctx, "large.hbk"); err != nil {
			t.Fatalf("deleting the large archive: %v", err)
		}
	})
}

// sameAs compares two streams without holding either.
func sameAs(got, want io.Reader) error {
	gotBuffer := make([]byte, 64<<10)
	wantBuffer := make([]byte, 64<<10)

	for offset := int64(0); ; {
		gotRead, gotErr := io.ReadFull(got, gotBuffer)
		wantRead, _ := io.ReadFull(want, wantBuffer[:gotRead])
		if gotRead != wantRead {
			return errors.New("the streams are different lengths")
		}
		if !bytes.Equal(gotBuffer[:gotRead], wantBuffer[:wantRead]) {
			return errors.New("the streams differ")
		}
		offset += int64(gotRead)

		if errors.Is(gotErr, io.EOF) || errors.Is(gotErr, io.ErrUnexpectedEOF) {
			// The other side has to end here too.
			if n, _ := want.Read(make([]byte, 1)); n != 0 {
				return errors.New("what came back is shorter than what went")
			}
			return nil
		}
		if gotErr != nil {
			return gotErr
		}
	}
}

// pattern is an endless stream whose byte depends on its position, not on where a read happened
// to start. Zeroes would hide an adapter that dropped a chunk and refilled it with the previous
// one; a per-call pattern would depend on the buffer sizes and compare two different things.
type pattern struct{ at int64 }

func (p *pattern) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = byte((p.at + int64(i)) % 251)
	}
	p.at += int64(len(b))
	return len(b), nil
}
