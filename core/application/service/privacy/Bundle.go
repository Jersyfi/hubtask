// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"sync"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// bundle is the one archive an installation-wide case writes (UC-PRV-06 check 3): a zip whose
// members are the per-workspace Hubtask archives, unchanged, under `<tenant id>/`.
//
// It is a target the archive writer writes into. Every Put becomes a member, and the members
// stream through a single Put of the outer object on the real target. Zip rather than tar because a
// zip member needs no size ahead of time - the archive writer only learns a file's size as it
// writes it - and stored rather than deflated, because the members are compressed already.
//
// Only Put: a full archive is all the writer asks of a target. Anything else answers an internal
// error, so a writer that started reading back would fail loudly rather than read nothing.
type bundle struct {
	key    string
	pipe   *io.PipeWriter
	zip    *zip.Writer
	result chan bundleResult
	mu     sync.Mutex
}

type bundleResult struct {
	bytes int64
	err   error
}

var _ backupstorage.Store = (*bundle)(nil)

// openBundle starts the outer object at the target. Close finishes it, Abort discards it.
func openBundle(ctx context.Context, target backupstorage.Store, key string) *bundle {
	reader, writer := io.Pipe()
	b := &bundle{key: key, pipe: writer, zip: zip.NewWriter(writer), result: make(chan bundleResult, 1)}
	concurrency.Go(ctx, "privacy.bundle", func(ctx context.Context) {
		written, err := target.Put(ctx, key, reader)
		// The target stopped reading - a failure, or a Put that returned early - so the writing
		// side must not block on a pipe nobody drains.
		_ = reader.CloseWithError(errBundleClosed)
		b.result <- bundleResult{bytes: written, err: err}
	})
	return b
}

var errBundleClosed = errors.New("privacy: the bundle's target stopped reading")

// Put writes one member and answers how many bytes of it were taken, which the archive writer
// compares with what it sent.
func (b *bundle) Put(_ context.Context, key string, content io.Reader) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	member, err := b.zip.CreateHeader(&zip.FileHeader{Name: key, Method: zip.Store})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail(backupstorage.CodeTargetFailed).WithCause(err)
	}
	written, err := io.Copy(member, content)
	if err != nil {
		return written, shared.ErrUnavailable.WithDetail(backupstorage.CodeTargetFailed).WithCause(err)
	}
	return written, nil
}

// Close writes the zip's directory, ends the outer object and answers the target's verdict.
func (b *bundle) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.zip.Close(); err != nil {
		_ = b.pipe.CloseWithError(err)
		<-b.result
		return shared.ErrUnavailable.WithDetail(backupstorage.CodeTargetFailed).WithCause(err)
	}
	_ = b.pipe.Close()
	result := <-b.result
	return result.err
}

// Abort ends the outer object with an error, so the target keeps no half of it.
func (b *bundle) Abort(cause error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	_ = b.pipe.CloseWithError(cause)
	<-b.result
}

func (b *bundle) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, shared.Internalf("privacy: a bundle is written, never read")
}

func (b *bundle) List(context.Context, string) ([]backupstorage.Entry, error) {
	return nil, shared.Internalf("privacy: a bundle is written, never listed")
}

func (b *bundle) Stat(context.Context, string) (backupstorage.Entry, error) {
	return backupstorage.Entry{}, shared.Internalf("privacy: a bundle is written, never read")
}

func (b *bundle) Delete(context.Context, string) error {
	return shared.Internalf("privacy: a bundle is written, never changed")
}
