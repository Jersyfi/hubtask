// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	port "github.com/Jersyfi/hubtask/core/port/storage"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// LocalStorage keeps objects in a directory: the self-hosting default (ADR-0003 - a Raspberry
// Pi has a disk, not a bucket).
//
// The content type travels in a sidecar file beside the object, because a filesystem has no
// metadata slot for it and guessing it back at read time would undo the sniff the upload guard
// did (T-11). The sidecar is written after the content and read before it, so a crash between
// the two leaves an object without a type - which reads as an absent object rather than one with
// an invented type.
type LocalStorage struct {
	root string
}

// NewLocalStorage takes the configured directory (HUBTASK_STORAGE_LOCAL_PATH).
func NewLocalStorage(root string) LocalStorage {
	return LocalStorage{root: root}
}

var _ port.ObjectStore = LocalStorage{}

// typeSuffix marks the sidecar. An object key never collides with one: keys are minted
// UUID-shaped names, and pathOf refuses the suffix outright as defence in depth.
const typeSuffix = ".content-type"

// Put writes the object atomically: the bytes into a temporary file in the same directory, then
// one rename - a reader can see the old object or the new one, never half of either.
func (s LocalStorage) Put(_ context.Context, upload port.Upload) error {
	path, err := s.pathOf(upload.Key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return ioFailed("preparing the directory", err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return ioFailed("opening the temporary file", err)
	}
	defer func() {
		// Present only when something above failed; a success renamed it away.
		_ = os.Remove(temp.Name())
	}()

	written, err := io.Copy(temp, upload.Content)
	if err != nil {
		_ = temp.Close()
		// The guard's refusal travels as it is - a size refusal must stay a size refusal - and
		// everything else is the disk's problem, coded rather than quoted (T-18).
		if shared.AsError(err).DetailCode != "" {
			return err
		}
		return ioFailed("writing the object", err)
	}
	if err := temp.Close(); err != nil {
		return ioFailed("closing the object", err)
	}
	if upload.Size >= 0 && written != upload.Size {
		return shared.ErrValidation.
			WithDetail("media.size_mismatch").
			WithParams(map[string]string{
				"declared": fmt.Sprint(upload.Size), "written": fmt.Sprint(written),
			}).
			WithFields(shared.FieldError{Path: "/content", Code: "media.size_mismatch"})
	}

	if err := os.WriteFile(path+typeSuffix, []byte(upload.ContentType), 0o600); err != nil {
		return ioFailed("writing the content type", err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return ioFailed("placing the object", err)
	}
	return nil
}

// Get opens the object for streaming. The caller closes it.
func (s LocalStorage) Get(_ context.Context, key string) (port.Object, error) {
	path, err := s.pathOf(key)
	if err != nil {
		return port.Object{}, err
	}

	//nolint:gosec // G304: the path came from pathOf, which refuses everything that walks
	contentType, err := os.ReadFile(path + typeSuffix)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return port.Object{}, shared.ErrNotFound
		}
		return port.Object{}, ioFailed("reading the content type", err)
	}
	//nolint:gosec // G304: the path came from pathOf, which refuses everything that walks
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return port.Object{}, shared.ErrNotFound
		}
		return port.Object{}, ioFailed("opening the object", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return port.Object{}, ioFailed("measuring the object", err)
	}

	return port.Object{
		Content:     file,
		Size:        info.Size(),
		ContentType: string(contentType),
	}, nil
}

// Delete removes the object and its sidecar. Removing what is not there succeeds - deletion is
// the state the caller asked for.
func (s LocalStorage) Delete(_ context.Context, key string) error {
	path, err := s.pathOf(key)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return ioFailed("removing the object", err)
	}
	if err := os.Remove(path + typeSuffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return ioFailed("removing the content type", err)
	}
	return nil
}

// pathOf turns a key into a path under the root, and refuses everything else.
//
// Keys are minted by the application and never user text, so a key that walks - "..", an
// absolute path, an empty segment - is a defect, not a request. Refused all the same: defence
// in depth is cheaper than certainty about every future caller (arc42 §8.4).
func (s LocalStorage) pathOf(key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || strings.HasSuffix(key, typeSuffix) {
		return "", keyInvalid(key)
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", keyInvalid(key)
		}
	}

	path := filepath.Join(s.root, filepath.FromSlash(key))
	// The belt to the braces above: whatever the segments said, the result stays under the root.
	if relative, err := filepath.Rel(s.root, path); err != nil ||
		strings.HasPrefix(relative, "..") {
		return "", keyInvalid(key)
	}
	return path, nil
}

func keyInvalid(key string) error {
	return shared.ErrInternal.
		WithDetail("storage.key_invalid").
		WithCause(fmt.Errorf("the key %q is not a name this store hands out", key))
}

// ioFailed is the disk's problems, coded rather than quoted: a path in an error can carry a
// key, and a key is a capability (T-18, security.md §8).
func ioFailed(doing string, err error) error {
	return shared.ErrUnavailable.
		WithDetail("storage.io_failed").
		WithCause(fmt.Errorf("%s: %w", doing, err))
}
