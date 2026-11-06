// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var stranger = shared.MustParseID("0192f000-0000-7000-8000-0000000000fe")

type savedPresets struct{ rows map[shared.ID]domain.Preset }

func (s *savedPresets) Find(_ context.Context, id shared.ID) (domain.Preset, error) {
	p, ok := s.rows[id]
	if !ok {
		return domain.Preset{}, shared.ErrNotFound.WithDetail("presets.not_found")
	}
	return p, nil
}

func (s *savedPresets) ListOffered(context.Context, shared.ID) ([]domain.Preset, error) {
	return nil, nil
}

func (s *savedPresets) Count(_ context.Context, origin domain.PresetOrigin, owner shared.ID) (int, error) {
	n := 0
	for _, p := range s.rows {
		if p.Origin == origin && (origin != domain.PresetOriginOwn || p.OwnerID == owner) {
			n++
		}
	}
	return n, nil
}

func (s *savedPresets) Insert(_ context.Context, p domain.Preset, _ time.Time) error {
	p.Version = 1
	s.rows[p.ID] = p
	return nil
}

func (s *savedPresets) Update(_ context.Context, p domain.Preset, expected int) error {
	if s.rows[p.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	p.Version = expected + 1
	s.rows[p.ID] = p
	return nil
}

func (s *savedPresets) Delete(_ context.Context, id shared.ID, expected int) error {
	if s.rows[id].Version != expected {
		return shared.ErrVersionConflict
	}
	delete(s.rows, id)
	return nil
}

// shapeKeeper refuses the right to change the workspace's shape and lets every read through.
type shapeKeeper struct {
	*authorizer
	refuseShape bool
}

func (a *shapeKeeper) Authorize(ctx context.Context, actor appshared.ActorContext, r access.Request) error {
	if a.refuseShape && r.Permission == service.PermissionStructure {
		return shared.ErrForbidden.WithDetail("access.not_permitted")
	}
	return a.authorizer.Authorize(ctx, actor, r)
}

type presetHarness struct {
	*structureHarness
	presets  *savedPresets
	gate     *shapeKeeper
	registry *usecase.Registry
}

func newPresetHarness(t *testing.T) *presetHarness {
	t.Helper()
	h := newStructureHarness(t)
	presets := &savedPresets{rows: map[shared.ID]domain.Preset{}}
	gate := &shapeKeeper{authorizer: h.authorizer}
	desk := PresetDesk{
		Presets: presets, Structure: h.handler.Structure, Authorizer: gate, Audit: h.audit,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, Text: text.Composing{},
	}
	registry, err := usecase.NewRegistry(nil,
		SavePreset{Desk: desk}.Descriptor(), ChangePreset{Desk: desk}.Descriptor(), WithdrawPreset{Desk: desk}.Descriptor())
	if err != nil {
		t.Fatalf("the catalogue refused the entries: %v", err)
	}
	return &presetHarness{structureHarness: h, presets: presets, gate: gate, registry: registry}
}

func (h *presetHarness) call(t *testing.T, name string, who appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	t.Helper()
	return h.registry.Invoke(context.Background(), name, who, in)
}

func (h *presetHarness) lastAction(t *testing.T) string {
	t.Helper()
	if len(h.audit.entries) == 0 {
		t.Fatal("nothing is on the trail")
	}
	return string(h.audit.entries[len(h.audit.entries)-1].Action)
}

// UC-WRK-30 check 1: a collection's settings in force are saved under a name, for the person or -
// with the right to change the workspace's shape - for the workspace; the trail never carries the
// name (rule 10).
func TestACollectionsSettingsAreSavedAsAPreset(t *testing.T) {
	h := newPresetHarness(t)

	own, err := h.call(t, SavePresetName, actor(), usecase.Input{
		"name": "Weekly shop", "origin": "OWN", "container_id": collectionID.String(),
	})
	if err != nil {
		t.Fatalf("saving an own preset: %v", err)
	}
	if own["origin"] != "OWN" || own["name"] != "Weekly shop" || own["settings"] == nil {
		t.Fatalf("the own preset came back as %v", own)
	}
	if h.lastAction(t) != string(PresetSavedAction) {
		t.Fatalf("the save is not on the trail: %s", h.lastAction(t))
	}
	for _, change := range h.audit.entries[len(h.audit.entries)-1].Changes {
		if change == "Weekly shop" {
			t.Fatal("the trail carries the preset's name")
		}
	}

	h.gate.refuseShape = true
	if _, err := h.call(t, SavePresetName, actor(), usecase.Input{
		"name": "Client project", "origin": "WORKSPACE", "container_id": collectionID.String(),
	}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a workspace preset without the right: %v", err)
	}
	if len(h.presets.rows) != 1 {
		t.Fatalf("a refused save was stored: %d presets", len(h.presets.rows))
	}
	h.gate.refuseShape = false
	if _, err := h.call(t, SavePresetName, actor(), usecase.Input{
		"name": "Client project", "origin": "WORKSPACE", "container_id": collectionID.String(),
	}); err != nil {
		t.Fatalf("a workspace preset with the right: %v", err)
	}
}

// A person's own presets end at their ceiling: the next one is refused, not stored.
func TestAnOwnPresetPastTheCeilingIsRefused(t *testing.T) {
	h := newPresetHarness(t)
	for i := range domain.MaxOwnPresets {
		id := shared.MustParseID("0192f000-0000-7000-8000-0000000a" + leftPad(i))
		h.presets.rows[id] = domain.Preset{ID: id, Origin: domain.PresetOriginOwn, OwnerID: accountID, Version: 1}
	}
	if _, err := h.call(t, SavePresetName, actor(), usecase.Input{
		"name": "One too many", "origin": "OWN", "container_id": collectionID.String(),
	}); err == nil {
		t.Fatal("a preset past the ceiling was stored")
	}
	if len(h.presets.rows) != domain.MaxOwnPresets {
		t.Fatalf("the ceiling let %d presets in", len(h.presets.rows))
	}
}

func leftPad(i int) string {
	const digits = "0123456789"
	out := []byte("0000")
	for p := 3; p >= 0; p-- {
		out[p] = digits[i%10]
		i /= 10
	}
	return string(out)
}

// UC-WRK-30 check 3: a workspace preset is renamed, changed and withdrawn by whoever may change
// the workspace's shape; a stale version is refused, somebody else's own preset is not found, and
// the workspace's default is not withdrawn.
func TestAWorkspacePresetIsKeptByWhoeverShapesTheWorkspace(t *testing.T) {
	h := newPresetHarness(t)
	saved, err := h.call(t, SavePresetName, actor(), usecase.Input{
		"name": "Client project", "origin": "WORKSPACE", "container_id": collectionID.String(),
	})
	if err != nil {
		t.Fatalf("saving: %v", err)
	}
	id := saved["id"].(string)

	h.gate.refuseShape = true
	if _, err := h.call(t, ChangePresetName, actor(), usecase.Input{"preset_id": id, "name": "Clients"}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("renamed without the right: %v", err)
	}
	if _, err := h.call(t, WithdrawPresetName, actor(), usecase.Input{"preset_id": id}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("withdrawn without the right: %v", err)
	}
	h.gate.refuseShape = false

	if _, err := h.call(t, ChangePresetName, actor(), usecase.Input{"preset_id": id, "name": "Clients", "expected_version": 7}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale rename: %v", err)
	}
	renamed, err := h.call(t, ChangePresetName, actor(), usecase.Input{
		"preset_id": id, "name": "Clients", "settings": map[string]any{"celebration": "SUBTLE"},
	})
	if err != nil {
		t.Fatalf("renaming and changing: %v", err)
	}
	if renamed["name"] != "Clients" || renamed["version"] != 2 {
		t.Fatalf("the change came back as %v", renamed)
	}
	if h.lastAction(t) != string(PresetChangedAction) {
		t.Fatalf("the change is not on the trail: %s", h.lastAction(t))
	}
	before := len(h.audit.entries)
	if _, err := h.call(t, ChangePresetName, actor(), usecase.Input{"preset_id": id, "name": "Clients"}); err != nil {
		t.Fatalf("writing what is stored: %v", err)
	}
	if len(h.audit.entries) != before || h.presets.rows[shared.MustParseID(id)].Version != 2 {
		t.Fatal("a change that moved nothing was written")
	}

	other := actor()
	other.AccountID = stranger
	mine, err := h.call(t, SavePresetName, actor(), usecase.Input{"name": "Mine", "origin": "OWN", "container_id": collectionID.String()})
	if err != nil {
		t.Fatalf("saving an own preset: %v", err)
	}
	if _, err := h.call(t, ChangePresetName, other, usecase.Input{"preset_id": mine["id"].(string), "name": "Taken"}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("somebody else's own preset was changed: %v", err)
	}

	h.store.workspace.DefaultPreset = h.presets.rows[shared.MustParseID(id)].Key
	if _, err := h.call(t, WithdrawPresetName, actor(), usecase.Input{"preset_id": id}); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "presets.workspace_default" {
		t.Fatalf("the workspace's default was withdrawn: %v", err)
	}
	h.store.workspace.DefaultPreset = ""
	if _, err := h.call(t, WithdrawPresetName, actor(), usecase.Input{"preset_id": id}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if _, kept := h.presets.rows[shared.MustParseID(id)]; kept || h.lastAction(t) != string(PresetWithdrawnAction) {
		t.Fatalf("the withdrawal: kept %v, trail %s", kept, h.lastAction(t))
	}
}

// Naming a built-in preset and a saved one at once is refused rather than one silently winning.
func TestNamingTwoPresetsAtOnceIsRefused(t *testing.T) {
	_, err := presetFrom(usecase.Input{"preset": "LIST", "preset_id": stranger.String()}, "preset", "preset_id")
	if shared.AsError(err) == nil || shared.AsError(err).DetailCode != "presets.both_named" {
		t.Fatalf("both named: %v", err)
	}
	key, err := presetFrom(usecase.Input{"preset_id": stranger.String()}, "preset", "preset_id")
	if err != nil || key != domain.SavedPresetKey(stranger) {
		t.Fatalf("a saved preset's key: %q %v", key, err)
	}
	if builtIn, savedID := keySplit(key); builtIn != nil || savedID != stranger.String() {
		t.Fatalf("the split: %v %v", builtIn, savedID)
	}
	if builtIn, savedID := keySplit(domain.PresetList); builtIn != string(domain.PresetList) || savedID != nil {
		t.Fatalf("a built-in's split: %v %v", builtIn, savedID)
	}
}
