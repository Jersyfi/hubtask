// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// shelf is an in-memory upload store with the objects' files.
type shelf struct {
	objects map[shared.ID]media.Object
	links   map[shared.ID][]shared.ID
	hubOf   map[shared.ID]shared.ID
}

func newShelf() *shelf {
	return &shelf{objects: map[shared.ID]media.Object{}, links: map[shared.ID][]shared.ID{}, hubOf: map[shared.ID]shared.ID{}}
}

func (s *shelf) Find(_ context.Context, id shared.ID) (media.Object, error) {
	o, ok := s.objects[id]
	if !ok {
		return media.Object{}, shared.ErrNotFound
	}
	return o, nil
}
func (s *shelf) AdjustRefCount(_ context.Context, id shared.ID, delta int) error {
	o := s.objects[id]
	o.RefCount += delta
	s.objects[id] = o
	return nil
}
func (s *shelf) AttachToCi(_ context.Context, ci, m shared.ID) (bool, error) {
	if slices.Contains(s.links[ci], m) {
		return false, nil
	}
	s.links[ci] = append(s.links[ci], m)
	return true, nil
}
func (s *shelf) DetachFromCi(_ context.Context, ci, m shared.ID) (bool, error) {
	before := len(s.links[ci])
	s.links[ci] = slices.DeleteFunc(s.links[ci], func(id shared.ID) bool { return id == m })
	return len(s.links[ci]) < before, nil
}
func (s *shelf) CiMediaIDs(_ context.Context, ci shared.ID) ([]shared.ID, error) {
	out := slices.Clone(s.links[ci])
	slices.Sort(out)
	return out, nil
}
func (s *shelf) CountCiMedia(_ context.Context, ci shared.ID) (int, error) {
	return len(s.links[ci]), nil
}
func (s *shelf) ListForCi(context.Context, shared.ID, workrepo.Page) (mediarepo.ObjectPage, error) {
	return mediarepo.ObjectPage{}, nil
}
func (s *shelf) ReferencingCis(_ context.Context, m shared.ID) ([]shared.ID, error) {
	var hubs []shared.ID
	for ci, ids := range s.links {
		if slices.Contains(ids, m) {
			hubs = append(hubs, s.hubOf[ci])
		}
	}
	return hubs, nil
}

var (
	manual   = ciID("0201")
	strange  = ciID("0202")
	vaulted  = ciID("0203")
	unsealed = ciID("0204")
)

// UC-AST-01 check 12: a writer puts their upload on an object and takes it off again; the link
// moves the reference count, a retry changes nothing, the trail names the file by identifier only
// (rule 10). A file somebody else uploaded is put on only where the writer reads an object that
// holds it, and an upload that is not READY or not an attachment is refused.
func TestAWriterPutsAFileOnAnObjectAndTakesItOff(t *testing.T) {
	f, _ := assuredFixture()
	files := newShelf()
	trail := &trail{}
	f.inv.Files, f.inv.Reader, f.inv.Audit = files, seeing{}, trail
	files.objects[manual] = media.Object{ID: manual, Status: media.StatusReady, Usage: media.UsageAttachment, CreatedBy: anna}
	files.objects[strange] = media.Object{ID: strange, Status: media.StatusReady, Usage: media.UsageAttachment, CreatedBy: bert}
	files.objects[vaulted] = media.Object{ID: vaulted, Status: media.StatusReady, Usage: media.UsageAttachment, CreatedBy: anna}
	files.objects[unsealed] = media.Object{ID: unsealed, Status: media.StatusPending, Usage: media.UsageAttachment, CreatedBy: anna}
	files.links[vault], files.hubOf[vault] = []shared.ID{vaulted}, private

	attach, detach := AttachCiMedia{Inventory: f.inv}.Descriptor(), DetachCiMedia{Inventory: f.inv}.Descriptor()
	in := func(ci, m shared.ID) usecase.Input {
		return usecase.Input{"ci_id": ci.String(), "media_id": m.String()}
	}

	out, err := call(t, attach, as(anna), in(host, manual))
	if err != nil {
		t.Fatal(err)
	}
	if ids := out["media_ids"].([]string); len(ids) != 1 || ids[0] != manual.String() || out["ci_id"] != host.String() {
		t.Errorf("the answer: %v", out)
	}
	if _, err := call(t, attach, as(anna), in(host, manual)); err != nil || files.objects[manual].RefCount != 1 {
		t.Errorf("a retry: %v, count %d", err, files.objects[manual].RefCount)
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != CIFileAttachedAction || trail.entries[0].TargetID != host {
		t.Fatalf("the trail: %+v", trail.entries)
	}
	if changes := trail.entries[0].Changes; len(changes) != 1 || changes["media_id"] == nil {
		t.Errorf("the trail carries %v, want the file's identifier alone", changes)
	}

	// Bert's upload is not Anna's, and no object she reads holds it.
	if _, err := call(t, attach, as(anna), in(host, strange)); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody else's upload: %v", err)
	}
	// The vault's file: Bert reads no vault, Anna does and may put it on the host.
	if _, err := call(t, attach, as(bert), in(booking, vaulted)); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a file only a private hub's object holds: %v", err)
	}
	if _, err := call(t, attach, as(anna), in(host, vaulted)); err != nil {
		t.Errorf("a file the writer reads on another object: %v", err)
	}
	if _, err := call(t, attach, as(anna), in(host, unsealed)); err == nil {
		t.Error("an upload that is not READY was attached")
	}
	// Bert writes no private hub: refused before anything moves.
	if _, err := call(t, attach, as(bert), in(vault, manual)); err == nil {
		t.Error("a writer outside the hub attached a file")
	}

	out, err = call(t, detach, as(anna), in(host, manual))
	if err != nil {
		t.Fatal(err)
	}
	if ids := out["media_ids"].([]string); len(ids) != 1 || ids[0] != vaulted.String() {
		t.Errorf("after detaching: %v", ids)
	}
	if files.objects[manual].RefCount != 0 {
		t.Errorf("the count after detaching: %d", files.objects[manual].RefCount)
	}
	if _, err := call(t, detach, as(anna), in(host, manual)); err != nil || files.objects[manual].RefCount != 0 {
		t.Errorf("detaching again: %v, count %d", err, files.objects[manual].RefCount)
	}
}

// An object holds at most MaxFilesPerCI files; one it already holds is still answered.
func TestAnObjectHoldsABoundedNumberOfFiles(t *testing.T) {
	f, _ := assuredFixture()
	files := newShelf()
	f.inv.Files = files
	for i := range MaxFilesPerCI {
		id := shared.ID(fmt.Sprintf("file-%d", i))
		files.links[host] = append(files.links[host], id)
	}
	files.objects[manual] = media.Object{ID: manual, Status: media.StatusReady, Usage: media.UsageAttachment, CreatedBy: anna}
	_, err := call(t, AttachCiMedia{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{"ci_id": host.String(), "media_id": manual.String()})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("the 501st file: %v", err)
	}
}
