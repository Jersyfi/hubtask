// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	privacyrepo "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The workspace's position on people keeping their own content out of AI (UC-PRV-05 checks 9 and
// 11), through the registry.

// keepers answers a fixed list of people keeping their content out.
type keepers struct {
	privacyrepo.Consents
	standing []privacyrepo.Objector
}

func (k keepers) Standing(context.Context, string) ([]privacyrepo.Objector, error) {
	return k.standing, nil
}

func positionRegistry(t *testing.T, world *sourceWorld, store *providerStore, objectors ...privacyrepo.Objector) *usecase.Registry {
	t.Helper()
	writer := AiSourceWriter{
		Rules: AiRules{
			Instance: world, Workspaces: sourceWorkspaces{world},
			Offered: sourceOffered{world}, Choices: sourceChoices{world},
			Consents: keepers{standing: objectors},
		},
		Authorizer: providerAllow{}, Audit: store, UnitOfWork: providerDirect{},
		Clock: providerClock{}, Reindex: sourceReseeder{world},
	}
	registry, err := usecase.NewRegistry(nil, ReadAiSource{Writer: writer}.Descriptor(),
		SetAiPersonOptOut{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("not registrable: %v", err)
	}
	return registry
}

func TestTheDefaultLetsEachPersonKeepTheirContentOut(t *testing.T) {
	world := &sourceWorld{}
	out := readSourceVia(t, positionRegistry(t, world, &providerStore{},
		privacyrepo.Objector{AccountID: shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")}))
	if out["person_opt_out"] != "OFFERED" || out["person_opt_out_origin"] != "DEFAULT" || out["keeping_out_count"] != 1 {
		t.Errorf("answered %v", out)
	}
	for _, absent := range []string{"person_opt_out_basis", "person_opt_out_contact", "person_opt_out_since", "person_opt_out_lock"} {
		if _, held := out[absent]; held {
			t.Errorf("%s answered under OFFERED", absent)
		}
	}
}

// Check 9: the workspace names the basis when it makes AI part of everybody's work, cannot do so
// without one, and the trail records the basis - never whom to ask.
func TestAWorkspaceMakesAiPartOfEverybodysWorkNamingTheBasis(t *testing.T) {
	world := &sourceWorld{}
	store := &providerStore{}
	registry := positionRegistry(t, world, store)

	for input, code := range map[string]string{
		"":        "ai.opt_out_basis_required",
		"CONSENT": "usecase.input_invalid", // the registry knows the closed set first
	} {
		_, err := registry.Invoke(t.Context(), SetAiPersonOptOutName, providerActor(),
			usecase.Input{"person_opt_out": "NOT_OFFERED", "basis": input})
		if got := shared.AsError(err); got == nil || got.DetailCode != code {
			t.Errorf("basis %q answered %v, want %s", input, err, code)
		}
	}
	if world.workspace.Settings.AiPersonOptOut != "" || len(store.entries) != 0 {
		t.Fatal("a refused position was written")
	}

	out, err := registry.Invoke(t.Context(), SetAiPersonOptOutName, providerActor(), usecase.Input{
		"person_opt_out": "NOT_OFFERED", "basis": "WORKS_AGREEMENT", "contact": "The works council",
	})
	if err != nil {
		t.Fatalf("setting: %v", err)
	}
	if out["person_opt_out"] != "NOT_OFFERED" || out["person_opt_out_basis"] != "WORKS_AGREEMENT" ||
		out["person_opt_out_contact"] != "The works council" || out["person_opt_out_origin"] != "WORKSPACE" {
		t.Errorf("answered %v", out)
	}
	since, held := out["person_opt_out_since"].(time.Time)
	if !held || since.IsZero() {
		t.Errorf("no moment for NOT_OFFERED: %v", out["person_opt_out_since"])
	}
	if world.reseeded != 1 {
		t.Errorf("reseeded %d times, want once: whose entries may be embedded moved", world.reseeded)
	}
	entry := store.entries[len(store.entries)-1]
	if entry.Action != AiPersonOptOutSetAction {
		t.Fatalf("recorded %s", entry.Action)
	}
	if recorded := fmt.Sprint(entry.Changes); strings.Contains(recorded, "works council") ||
		!strings.Contains(recorded, "WORKS_AGREEMENT") {
		t.Errorf("the trail should name the basis and never whom to ask (rule 10): %s", recorded)
	}

	// A changed basis keeps the moment: nobody is told again about the same position.
	again, err := registry.Invoke(t.Context(), SetAiPersonOptOutName, providerActor(), usecase.Input{
		"person_opt_out": "NOT_OFFERED", "basis": "LEGAL_OBLIGATION",
	})
	if err != nil {
		t.Fatalf("changing the basis: %v", err)
	}
	if kept, _ := again["person_opt_out_since"].(time.Time); !kept.Equal(since) {
		t.Errorf("the moment moved from %v to %v", since, kept)
	}

	// Back to OFFERED clears the basis, whom to ask and the moment.
	back, err := registry.Invoke(t.Context(), SetAiPersonOptOutName, providerActor(), usecase.Input{"person_opt_out": "OFFERED"})
	if err != nil {
		t.Fatalf("going back: %v", err)
	}
	if back["person_opt_out"] != "OFFERED" || world.workspace.Settings.AiOptOutBasis != "" ||
		world.workspace.Settings.AiOptOutContact != "" || !world.workspace.Settings.AiOptOutSince.IsZero() {
		t.Errorf("going back left %v / %+v", back, world.workspace.Settings)
	}
}

// Check 11: under the installation's lock the workspace cannot choose NOT_OFFERED, the answer
// says who locked it, and OFFERED stays its own.
func TestTheInstallationsLockHoldsTheWorkspaceAtOffered(t *testing.T) {
	offered := domain.AiOptOutOffered
	world := &sourceWorld{level: identityrepo.InstanceLevel{Ai: domain.AiLayer{
		PersonOptOut: &offered, Locks: map[domain.AiSwitch]bool{domain.AiSwitchPersonOptOut: true},
	}}}
	registry := positionRegistry(t, world, &providerStore{})

	_, err := registry.Invoke(t.Context(), SetAiPersonOptOutName, providerActor(), usecase.Input{
		"person_opt_out": "NOT_OFFERED", "basis": "EMPLOYMENT_CONTRACT",
	})
	got := shared.AsError(err)
	if got == nil || got.DetailCode != "auth.policy_locked" || got.Params["origin"] != "INSTANCE" {
		t.Fatalf("answered %v", err)
	}
	if _, err := registry.Invoke(t.Context(), SetAiPersonOptOutName, providerActor(),
		usecase.Input{"person_opt_out": "OFFERED"}); err != nil {
		t.Errorf("OFFERED refused under the lock that holds it there: %v", err)
	}
	out := readSourceVia(t, registry)
	if out["person_opt_out_lock"] != "INSTANCE" || out["person_opt_out_origin"] != "INSTANCE" {
		t.Errorf("the screen cannot say who decided: %v", out)
	}
}

// A NOT_OFFERED stored before the installation placed its lock no longer stands.
func TestAPositionStoredBeforeALockIsIgnored(t *testing.T) {
	offered := domain.AiOptOutOffered
	world := &sourceWorld{level: identityrepo.InstanceLevel{Ai: domain.AiLayer{
		PersonOptOut: &offered, Locks: map[domain.AiSwitch]bool{domain.AiSwitchPersonOptOut: true},
	}}}
	world.workspace.Settings.AiPersonOptOut = "NOT_OFFERED"
	world.workspace.Settings.AiOptOutBasis = "WORKS_AGREEMENT"
	out := readSourceVia(t, positionRegistry(t, world, &providerStore{}))
	if out["person_opt_out"] != "OFFERED" {
		t.Errorf("a stale position stands: %v", out)
	}
	if _, held := out["person_opt_out_basis"]; held {
		t.Errorf("the basis of a position that does not stand is shown: %v", out)
	}
}

func readSourceVia(t *testing.T, registry *usecase.Registry) usecase.Output {
	t.Helper()
	out, err := registry.Invoke(t.Context(), ReadAiSourceName, providerActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	return out
}

var (
	_ audit.Sink           = (*providerStore)(nil)
	_ privacyrepo.Consents = keepers{}
)
