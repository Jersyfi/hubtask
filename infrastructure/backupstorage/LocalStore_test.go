// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
)

// What is here is what BK-1 cannot reach: the containment the local adapter provides and the
// three other adapters get from their protocol. The behaviour every adapter shares is in
// test/backup, run against all four.

func localStore(t *testing.T, path string) backupstorage.LocalStore {
	t.Helper()
	store, err := backupstorage.NewLocalStore(t.TempDir(), path)
	if err != nil {
		t.Fatalf("opening the target: %v", err)
	}
	return store
}

// The security shape of this adapter: two roots, not one. The installation's is set by whoever
// runs the process; the target's is set by whoever configures the target through the API, and
// that person is an administrator of the instance rather than of the machine.
func TestATargetCannotBeConfiguredOutOfItsVolume(t *testing.T) {
	root := t.TempDir()

	for _, path := range []string{"../elsewhere", "backups/../../etc", "..", "./..", "/../etc"} {
		if _, err := backupstorage.NewLocalStore(root, path); err == nil {
			t.Errorf("a target configured at %q was accepted", path)
		}
	}

	// An absolute path that is a plain name is read as relative to the volume rather than
	// refused: "/backups" is what an operator types, and under this adapter it means the same
	// place as "backups" - the volume is the whole of the filesystem this target can see.
	store, err := backupstorage.NewLocalStore(root, "/backups")
	if err != nil {
		t.Fatalf("a plain absolute path was refused: %v", err)
	}
	if _, err := store.Put(t.Context(), "a.hbk", strings.NewReader("x")); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "backups", "a.hbk")); err != nil {
		t.Fatalf("the archive did not land inside the volume: %v", err)
	}
}

func TestAnInstallationWithNoBackupRootRefusesALocalTarget(t *testing.T) {
	_, err := backupstorage.NewLocalStore("  ", "backups")
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a target with nowhere to write was accepted: %v", err)
	}
}

// A key is minted by this system and never user text, so a key that walks is a defect - and it is
// refused all the same, because the consequence here is writing into somebody else's directory.
func TestAKeyCannotLeaveTheTarget(t *testing.T) {
	store := localStore(t, "backups")

	for _, key := range []string{
		"", "/absolute", "../outside", "a/../../outside", "a//b", "a/./b",
		"a\\b", "with\nnewline", "with\x00null", strings.Repeat("k", 901),
	} {
		if _, err := store.Put(t.Context(), key, strings.NewReader("x")); err == nil {
			t.Errorf("the key %q was written", key)
		}
		if _, err := store.Get(t.Context(), key); err == nil {
			t.Errorf("the key %q was read", key)
		}
		if err := store.Delete(t.Context(), key); err == nil {
			t.Errorf("the key %q was deleted", key)
		}
	}
}

// A write that was interrupted leaves a temporary file behind. It is not an archive and must not
// be listed as one - a restore offering it would offer half a backup.
func TestAnInterruptedWriteIsNotListedAsAnArchive(t *testing.T) {
	root := t.TempDir()
	store, err := backupstorage.NewLocalStore(root, "backups")
	if err != nil {
		t.Fatalf("opening the target: %v", err)
	}
	if _, err := store.Put(t.Context(), "good.hbk", strings.NewReader("archive")); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "backups", ".archive-123456"), []byte("half"), 0o600); err != nil {
		t.Fatalf("planting the leftover: %v", err)
	}

	entries, err := store.List(t.Context(), "")
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != "good.hbk" {
		t.Fatalf("the listing is %v", entries)
	}
}

// A directory is not an object. Answering its size would let a caller believe an archive is
// there.
func TestADirectoryIsNotAnArchive(t *testing.T) {
	store := localStore(t, "backups")
	if _, err := store.Put(t.Context(), "2026/08/archive.hbk", strings.NewReader("x")); err != nil {
		t.Fatalf("writing: %v", err)
	}

	if _, err := store.Stat(t.Context(), "2026/08"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("stat on a directory: %v", err)
	}
	if _, err := store.Get(t.Context(), "2026"); err == nil {
		t.Fatal("a directory opened as an archive")
	}
}

// A directory can say how much room is left, which is exactly why the port makes the report
// optional rather than a sixth method: a bucket cannot.
func TestADirectoryReportsItsFreeSpace(t *testing.T) {
	store := localStore(t, "backups")

	reporter, answers := any(store).(port.SpaceReporter)
	if !answers {
		t.Fatal("a directory does not implement the space report")
	}
	free, err := reporter.FreeBytes(t.Context())
	if err != nil {
		t.Skipf("this platform cannot answer: %v", err)
	}
	if free <= 0 {
		t.Fatalf("the filesystem reports %d bytes free", free)
	}
}

// An archive is written whole or not at all: the bytes go to a temporary file and one rename
// puts them in place. A reader sees the old archive or the new one, never half of either.
func TestAnArchiveIsReplacedAtomically(t *testing.T) {
	root := t.TempDir()
	store, err := backupstorage.NewLocalStore(root, "backups")
	if err != nil {
		t.Fatalf("opening the target: %v", err)
	}
	if _, err := store.Put(t.Context(), "a.hbk", strings.NewReader("the first")); err != nil {
		t.Fatalf("writing: %v", err)
	}

	written, err := store.Put(t.Context(), "a.hbk", strings.NewReader("the second archive"))
	if err != nil {
		t.Fatalf("rewriting: %v", err)
	}
	if written != int64(len("the second archive")) {
		t.Fatalf("wrote %d bytes", written)
	}

	content, err := store.Get(t.Context(), "a.hbk")
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	defer func() { _ = content.Close() }()
	read, err := io.ReadAll(content)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if string(read) != "the second archive" {
		t.Fatalf("read %q", read)
	}

	// And nothing is left over from either write.
	leftovers, err := filepath.Glob(filepath.Join(root, "backups", ".archive-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("leftovers %v (%v)", leftovers, err)
	}
}

// A prefix nothing has been written under yet is an empty listing rather than a failure: it is
// the answer a fresh target gives, and a restore has to be able to ask.
func TestListingAFreshTargetIsEmptyRatherThanAnError(t *testing.T) {
	store := localStore(t, "backups")

	entries, err := store.List(t.Context(), "")
	if err != nil {
		t.Fatalf("listing a fresh target: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("a fresh target lists %v", entries)
	}
	if entries, err = store.List(t.Context(), "2026/08"); err != nil || len(entries) != 0 {
		t.Fatalf("listing an empty prefix: %v, %v", entries, err)
	}
}
