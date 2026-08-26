// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
)

// LocalStore writes archives into a directory: the self-hosting default, because ADR-0003's
// Raspberry Pi has a disk and not a bucket.
//
// It has two roots, not one, and that is the security shape rather than a convenience. The outer
// root is the installation's (HUBTASK_BACKUP_LOCAL_PATH), set by whoever runs the process and
// mounted as a volume; the inner one is the target's `path`, set by whoever configures the target
// through the API. A configured path is therefore *relative* and cannot leave the volume - which
// matters because the person configuring a target is an administrator of the instance and not of
// the machine, and "write my backups to /etc" is otherwise a supported request.
type LocalStore struct {
	// root is the resolved directory this target owns: the installation root joined with the
	// target's configured path, already proved to be inside it.
	root string
}

var (
	_ port.Store         = LocalStore{}
	_ port.SpaceReporter = LocalStore{}
)

// NewLocalStore resolves the target's directory under the installation's backup root.
func NewLocalStore(installationRoot, configuredPath string) (LocalStore, error) {
	root := strings.TrimSpace(installationRoot)
	if root == "" {
		return LocalStore{}, shared.ErrValidation.
			WithDetail("backup.local_root_not_configured").
			WithFields(shared.FieldError{
				Path: "/config/path", Code: "backup.local_root_not_configured",
			})
	}

	relative := strings.TrimPrefix(strings.TrimSpace(configuredPath), "/")
	if err := CheckPrefix(relative); err != nil {
		return LocalStore{}, err
	}

	resolved := filepath.Join(root, filepath.FromSlash(relative))
	// Belt to the braces CheckPrefix already provides: whatever the segments said, the result
	// stays under the installation's root.
	if inside, err := filepath.Rel(root, resolved); err != nil || strings.HasPrefix(inside, "..") {
		return LocalStore{}, keyInvalid(configuredPath)
	}
	return LocalStore{root: resolved}, nil
}

// Put writes the object atomically: into a temporary file in the same directory, then one
// rename. A reader sees the old archive or the new one, never half of either - which for a
// backup is the difference between one bad archive and a directory nobody can trust.
func (s LocalStore) Put(_ context.Context, key string, content io.Reader) (int64, error) {
	path, err := s.pathOf(key)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return 0, failed("preparing the directory", err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".archive-*")
	if err != nil {
		return 0, failed("opening the temporary file", err)
	}
	defer func() {
		// Present only when something below failed; a success renamed it away.
		_ = os.Remove(temp.Name())
	}()

	written, err := io.Copy(temp, content)
	if err != nil {
		_ = temp.Close()
		return 0, failed("writing the archive", err)
	}
	// An archive that is only in the page cache is an archive a power cut takes with it, and the
	// whole point of this file is to survive the machine that wrote it.
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return 0, failed("flushing the archive", err)
	}
	if err := temp.Close(); err != nil {
		return 0, failed("closing the archive", err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return 0, failed("placing the archive", err)
	}
	return written, nil
}

// Get opens the object for streaming. The caller closes it.
func (s LocalStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.pathOf(key)
	if err != nil {
		return nil, err
	}

	//nolint:gosec // G304: the path came from pathOf, which refuses everything that walks
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, notFound(key)
		}
		if errors.Is(err, fs.ErrPermission) {
			return nil, refused("opening the archive", err)
		}
		return nil, failed("opening the archive", err)
	}

	// A directory opens on this platform and reads as an error later, halfway through whatever
	// was consuming it. It is not an object, so it is absent - the answer every other adapter
	// gives, because no other protocol lets a caller open one at all.
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		_ = file.Close()
		if err != nil {
			return nil, failed("measuring the archive", err)
		}
		return nil, notFound(key)
	}
	return file, nil
}

// List walks the prefix. Recursive, and the keys come back whole rather than relative, so a
// caller can hand one straight back to Get.
func (s LocalStore) List(_ context.Context, prefix string) ([]port.Entry, error) {
	if err := CheckPrefix(prefix); err != nil {
		return nil, err
	}

	from := s.root
	if prefix != "" {
		from = filepath.Join(s.root, filepath.FromSlash(strings.TrimSuffix(prefix, "/")))
	}

	var entries []port.Entry
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			// A prefix nothing has been written under yet is an empty listing, not a failure:
			// "there are no backups here" is the answer a fresh target gives.
			return fs.SkipAll
		case err != nil:
			return err
		case entry.IsDir():
			return nil
		case strings.HasPrefix(entry.Name(), ".archive-"):
			// A write that was interrupted. It is not an archive and must not be listed as one.
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// Removed between the walk and the stat: it was not there when we looked.
				return nil
			}
			return err
		}
		relative, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		entries = append(entries, port.Entry{
			Key:        filepath.ToSlash(relative),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC(),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, refused("listing the target", err)
		}
		return nil, failed("listing the target", err)
	}
	return entries, nil
}

// Stat answers the object's size and age without reading it.
func (s LocalStore) Stat(_ context.Context, key string) (port.Entry, error) {
	path, err := s.pathOf(key)
	if err != nil {
		return port.Entry{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return port.Entry{}, notFound(key)
		}
		if errors.Is(err, fs.ErrPermission) {
			return port.Entry{}, refused("measuring the archive", err)
		}
		return port.Entry{}, failed("measuring the archive", err)
	}
	if info.IsDir() {
		// A directory is not an object. Answering its size would let a caller believe an archive
		// is there.
		return port.Entry{}, notFound(key)
	}
	return port.Entry{
		Key: key, Size: info.Size(), ModifiedAt: info.ModTime().UTC(),
	}, nil
}

// Delete removes the object. Removing what is not there succeeds.
func (s LocalStore) Delete(_ context.Context, key string) error {
	path, err := s.pathOf(key)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		if errors.Is(err, fs.ErrPermission) {
			return refused("removing the archive", err)
		}
		return failed("removing the archive", err)
	}
	return nil
}

// pathOf turns a key into a path inside this target's directory.
func (s LocalStore) pathOf(key string) (string, error) {
	if err := CheckKey(key); err != nil {
		return "", err
	}

	path := filepath.Join(s.root, filepath.FromSlash(key))
	if inside, err := filepath.Rel(s.root, path); err != nil || strings.HasPrefix(inside, "..") {
		return "", keyInvalid(key)
	}
	return path, nil
}
