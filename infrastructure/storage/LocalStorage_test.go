// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/storage"
)

func localStore(t *testing.T) LocalStorage {
	t.Helper()
	return NewLocalStorage(t.TempDir())
}

func TestAnObjectRoundTripsWithItsType(t *testing.T) {
	store := localStore(t)
	content := []byte("eight bytes and then some")

	err := store.Put(t.Context(), port.Upload{
		Key: "media/01/two", Content: bytes.NewReader(content),
		Size: int64(len(content)), ContentType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("putting: %v", err)
	}

	object, err := store.Get(t.Context(), "media/01/two")
	if err != nil {
		t.Fatalf("getting: %v", err)
	}
	defer object.Content.Close()

	read, err := io.ReadAll(object.Content)
	if err != nil || !bytes.Equal(read, content) {
		t.Fatalf("read %q (%v), want the object back", read, err)
	}
	if object.ContentType != "application/pdf" || object.Size != int64(len(content)) {
		t.Errorf("object %q/%d, want the stored type and size", object.ContentType, object.Size)
	}
}

func TestAMissingObjectIsNotFound(t *testing.T) {
	store := localStore(t)

	if _, err := store.Get(t.Context(), "media/none"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a missing object answered %v", err)
	}
}

func TestDeletionIsIdempotentAndComplete(t *testing.T) {
	store := localStore(t)
	if err := store.Put(t.Context(), port.Upload{
		Key: "media/gone", Content: strings.NewReader("bytes"), Size: 5, ContentType: "text/plain",
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(t.Context(), "media/gone"); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if _, err := store.Get(t.Context(), "media/gone"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the object survived its deletion: %v", err)
	}
	if err := store.Delete(t.Context(), "media/gone"); err != nil {
		t.Fatalf("deleting what is gone must succeed: %v", err)
	}
}

// arc42 §8.4's defence in depth: keys are minted, and a key that walks is refused anyway.
func TestAKeyCannotWalkOutOfTheRoot(t *testing.T) {
	root := t.TempDir()
	store := NewLocalStorage(filepath.Join(root, "media"))

	hostage := filepath.Join(root, "hostage")
	if err := os.WriteFile(hostage, []byte("untouchable"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{
		"../hostage", "a/../../hostage", "/etc/passwd", "", "a//b", "./a", "a/.", "x" + typeSuffix,
	} {
		err := store.Put(t.Context(), port.Upload{
			Key: key, Content: strings.NewReader("intruder"), Size: 8, ContentType: "text/plain",
		})
		if shared.AsError(err).DetailCode != "storage.key_invalid" {
			t.Errorf("key %q was answered with %v, want storage.key_invalid", key, err)
		}
		if _, err := store.Get(t.Context(), key); shared.AsError(err).DetailCode != "storage.key_invalid" {
			t.Errorf("key %q was readable: %v", key, err)
		}
		if err := store.Delete(t.Context(), key); shared.AsError(err).DetailCode != "storage.key_invalid" {
			t.Errorf("key %q was deletable: %v", key, err)
		}
	}

	if kept, err := os.ReadFile(hostage); err != nil || string(kept) != "untouchable" {
		t.Fatalf("the file outside the root was touched: %q, %v", kept, err)
	}
}

// The guard's refusal travels through the adapter as itself: a size refusal must stay a size
// refusal, and the half-written temporary file must not survive it.
func TestAGuardRefusalMidStreamLeavesNothingBehind(t *testing.T) {
	root := t.TempDir()
	store := NewLocalStorage(root)

	inspection, err := Inspect(io.LimitReader(neverEnding{}, 1<<20), "", 1024)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Put(t.Context(), port.Upload{
		Key: "media/too-big", Content: inspection.Content, Size: -1, ContentType: "text/plain",
	})
	if got := shared.AsError(err).DetailCode; got != "media.too_large" {
		t.Fatalf("the refusal arrived as %q", got)
	}

	entries, err := os.ReadDir(filepath.Join(root, "media"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Errorf("the refused upload left %q behind", entry.Name())
	}
	if _, err := store.Get(t.Context(), "media/too-big"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the refused object is servable: %v", err)
	}
}

func TestADeclaredSizeThatLiesIsRefused(t *testing.T) {
	store := localStore(t)

	err := store.Put(t.Context(), port.Upload{
		Key: "media/short", Content: strings.NewReader("only-this"), Size: 4096,
		ContentType: "text/plain",
	})
	if got := shared.AsError(err).DetailCode; got != "media.size_mismatch" {
		t.Fatalf("refused as %q", got)
	}
	if _, err := store.Get(t.Context(), "media/short"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("the mismatched object is servable: %v", err)
	}
}
