// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
)

// memoryStore is the target the converter's archive is written to and the applier reads it from:
// a map, alive for one job. An import's records are a file somebody uploaded, bounded by the
// upload limit, and a target they would have to reach would be of no use to the applier that
// reads them a moment later - so the archive never leaves the process (decision 7).
//
// Guarded, because the writer's producer runs in a goroutine of its own.
type memoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemoryStore() *memoryStore { return &memoryStore{objects: map[string][]byte{}} }

var _ backupstorage.Store = (*memoryStore)(nil)

func (s *memoryStore) Put(_ context.Context, key string, content io.Reader) (int64, error) {
	written, err := io.ReadAll(content)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return backupstorage.Entry{Key: key, Size: int64(len(object)), ModifiedAt: time.Time{}}, nil
}

func (s *memoryStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}
