// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// memoryStore is a backup target that keeps its objects in a map.
//
// It is guarded by a mutex because the writer's producer runs in a goroutine of its own, and a
// test whose failure mode is the race detector rather than an assertion is a test nobody trusts.
type memoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	// order is the sequence the members were written in, which is half of what this package
	// promises: the manifest after the data, checksums.txt after everything.
	order []string
	// failAt makes a Put fail, so that a half-written archive can be looked at.
	failAt string
}

func newStore() *memoryStore { return &memoryStore{objects: map[string][]byte{}} }

func (s *memoryStore) Put(_ context.Context, key string, content io.Reader) (int64, error) {
	if s.failAt != "" && strings.HasSuffix(key, s.failAt) {
		return 0, shared.ErrUnavailable.WithDetail(backupstorage.CodeTargetUnreachable)
	}
	written, err := io.ReadAll(content)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, seen := s.objects[key]; !seen {
		s.order = append(s.order, key)
	}
	s.objects[key] = written
	return int64(len(written)), nil
}

func (s *memoryStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	object, found := s.objects[key]
	if !found {
		return nil, shared.ErrNotFound.WithDetail(backupstorage.CodeObjectNotFound)
	}
	return io.NopCloser(bytes.NewReader(object)), nil
}

func (s *memoryStore) List(_ context.Context, prefix string) ([]backupstorage.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var entries []backupstorage.Entry
	for key, object := range s.objects {
		if strings.HasPrefix(key, prefix) {
			entries = append(entries, backupstorage.Entry{Key: key, Size: int64(len(object))})
		}
	}
	slices.SortFunc(entries, func(a, b backupstorage.Entry) int { return strings.Compare(a.Key, b.Key) })
	return entries, nil
}

func (s *memoryStore) Stat(_ context.Context, key string) (backupstorage.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	object, found := s.objects[key]
	if !found {
		return backupstorage.Entry{}, shared.ErrNotFound.WithDetail(backupstorage.CodeObjectNotFound)
	}
	return backupstorage.Entry{Key: key, Size: int64(len(object))}, nil
}

func (s *memoryStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *memoryStore) object(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	object, found := s.objects[key]
	return object, found
}

func (s *memoryStore) keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.order)
}

var _ backupstorage.Store = (*memoryStore)(nil)

// reversible is a stand-in for the archive cipher.
//
// It is deliberately not AES. What this package's tests are about is whether the writer encrypts
// at all, whether it binds each member to its place, and whether the reader reverses exactly what
// the writer did - and a fake that is obviously not encryption makes a plaintext leak visible in
// the assertion rather than in a hexdump. The real cipher is exercised end to end where it belongs,
// against real archives, in test/backup.
type reversible struct {
	sealed []crypto.Purpose
}

const reversibleMask = 0x5A

func (r *reversible) KeyBytes() int { return 32 }

func (r *reversible) Seal(w io.Writer, key secret.Bytes, purpose crypto.Purpose) (io.WriteCloser, error) {
	if key.Len() != r.KeyBytes() {
		return nil, shared.ErrValidation.WithDetail("crypto.key_unusable")
	}
	r.sealed = append(r.sealed, purpose)
	if _, err := io.WriteString(w, "sealed:"+string(purpose)+"\n"); err != nil {
		return nil, err
	}
	return &masking{to: w}, nil
}

func (r *reversible) Open(from io.Reader, key secret.Bytes, purpose crypto.Purpose) (io.Reader, error) {
	if key.Len() != r.KeyBytes() {
		return nil, shared.ErrValidation.WithDetail("crypto.key_unusable")
	}
	buffered := bufio.NewReader(from)
	header, err := buffered.ReadString('\n')
	if err != nil {
		return nil, crypto.NotAuthentic()
	}
	if strings.TrimSuffix(header, "\n") != "sealed:"+string(purpose) {
		return nil, crypto.NotAuthentic()
	}
	return &unmasking{from: buffered}, nil
}

type masking struct{ to io.Writer }

func (m *masking) Write(p []byte) (int, error) {
	masked := make([]byte, len(p))
	for i := range p {
		masked[i] = p[i] ^ reversibleMask
	}
	if _, err := m.to.Write(masked); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (m *masking) Close() error { return nil }

type unmasking struct{ from io.Reader }

func (u *unmasking) Read(p []byte) (int, error) {
	n, err := u.from.Read(p)
	for i := range p[:n] {
		p[i] ^= reversibleMask
	}
	return n, err
}

var _ crypto.StreamCipher = (*reversible)(nil)

// tableSource hands out records an entity at a time, from a map a test writes by hand.
type tableSource struct {
	rows map[string][]Record
	// asked records which entities were read and from when, so that a test can assert that an
	// incremental asked for a period rather than for everything.
	asked   map[string]time.Time
	failure error
}

func newSource() *tableSource {
	return &tableSource{rows: map[string][]Record{}, asked: map[string]time.Time{}}
}

func (s *tableSource) Records(_ context.Context, entity Entity, since time.Time, yield func(Record) error) error {
	s.asked[entity.Name] = since
	if s.failure != nil {
		return s.failure
	}
	for _, record := range s.rows[entity.Name] {
		if !since.IsZero() && !record.UpdatedAt.After(since) {
			continue
		}
		if err := yield(record); err != nil {
			return err
		}
	}
	return nil
}

var _ Source = (*tableSource)(nil)

// blobStore is the object store the media come from.
type blobStore struct {
	content map[string]string
	opened  []string
}

func newBlobs() *blobStore { return &blobStore{content: map[string]string{}} }

func (b *blobStore) Open(_ context.Context, digest string) (io.ReadCloser, error) {
	b.opened = append(b.opened, digest)
	content, found := b.content[digest]
	if !found {
		return nil, shared.ErrNotFound
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

var _ Media = (*blobStore)(nil)

// key is an archive key of the length the cipher wants.
func key() secret.Bytes { return secret.NewBytes(bytes.Repeat([]byte{0x11}, 32)) }
