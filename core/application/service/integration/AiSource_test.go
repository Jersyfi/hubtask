// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// A workspace's choice of AI source (ADR-0072 §2, §3; UC-AI-05 checks 1, 3, 6, 7; UC-AI-06 checks
// 2, 3; UC-AI-02 check 5), through the registry.

var (
	offeredLarge = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	offeredSmall = shared.MustParseID("0192f000-0000-7000-8000-0000000000e2")
)

type sourceWorld struct {
	level     identityrepo.InstanceLevel
	workspace identity.Workspace
	offered   []domain.AiProvider
	choice    *domain.AiChoice
	reseeded  int
}

func (w *sourceWorld) Read(context.Context) (identityrepo.InstanceLevel, error) { return w.level, nil }
func (w *sourceWorld) Write(context.Context, identityrepo.InstanceLevel, shared.ID, time.Time) error {
	return nil
}

type sourceWorkspaces struct{ world *sourceWorld }

func (s sourceWorkspaces) Find(context.Context) (identity.Workspace, error) {
	return s.world.workspace, nil
}

func (s sourceWorkspaces) Update(_ context.Context, changed identity.Workspace, expected int, _ time.Time) (bool, error) {
	if expected != s.world.workspace.Version {
		return false, nil
	}
	changed.Version++
	s.world.workspace = changed
	return true, nil
}

type sourceOffered struct{ world *sourceWorld }

func (s sourceOffered) List(context.Context) ([]domain.AiProvider, error) {
	return s.world.offered, nil
}
func (s sourceOffered) Find(_ context.Context, id shared.ID) (domain.AiProvider, error) {
	for _, offered := range s.world.offered {
		if offered.ID == id {
			return offered, nil
		}
	}
	return domain.AiProvider{}, shared.ErrNotFound.WithDetail("ai.offered_model_unknown")
}
func (s sourceOffered) FindWithKey(ctx context.Context, id shared.ID) (domain.AiProvider, *crypto.Sealed, error) {
	found, err := s.Find(ctx, id)
	return found, nil, err
}
func (sourceOffered) Insert(context.Context, domain.AiProvider, *crypto.Sealed, time.Time) (domain.AiProvider, error) {
	return domain.AiProvider{}, errors.New("not here")
}
func (sourceOffered) Update(context.Context, domain.AiProvider, *crypto.Sealed, time.Time) (domain.AiProvider, error) {
	return domain.AiProvider{}, errors.New("not here")
}
func (sourceOffered) UpdateKeepingKey(context.Context, domain.AiProvider, time.Time) (domain.AiProvider, error) {
	return domain.AiProvider{}, errors.New("not here")
}
func (sourceOffered) Delete(context.Context, shared.ID) (bool, error) { return false, nil }

type sourceChoices struct{ world *sourceWorld }

func (s sourceChoices) Find(context.Context) (domain.AiChoice, error) {
	if s.world.choice == nil {
		return domain.DefaultAiChoice(""), nil
	}
	return *s.world.choice, nil
}
func (s sourceChoices) Save(_ context.Context, choice domain.AiChoice) (domain.AiChoice, error) {
	if s.world.choice != nil {
		choice.Version = s.world.choice.Version + 1
	}
	s.world.choice = &choice
	return choice, nil
}

type sourceReseeder struct{ world *sourceWorld }

func (s sourceReseeder) Reseed(context.Context, shared.ID) error {
	s.world.reseeded++
	return nil
}

func offeredModel(id shared.ID, name string) domain.AiProvider {
	return domain.AiProvider{
		ID: id, DisplayName: name, Processor: "A processor", Kind: domain.AiOpenAiCompatible,
		BaseURL: "https://secret-address.example/v1", CompletionModel: "secret-model",
		EmbeddingModel: "secret-embed", HasAPIKey: true, Jurisdiction: domain.AiEEA,
	}
}

func sourceRegistry(t *testing.T, world *sourceWorld) *usecase.Registry {
	t.Helper()
	store := &providerStore{active: "k1"}
	writer := AiSourceWriter{
		Rules: AiRules{
			Instance: world, Workspaces: sourceWorkspaces{world},
			Offered: sourceOffered{world}, Choices: sourceChoices{world},
		},
		Authorizer: providerAllow{}, Audit: store, UnitOfWork: providerDirect{},
		Clock: providerClock{}, Reindex: sourceReseeder{world},
	}
	registry, err := usecase.NewRegistry(nil, ReadAiSource{Writer: writer}.Descriptor(), ChooseAiSource{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("not registrable: %v", err)
	}
	return registry
}

func instanceSays(sources domain.AiSources, locked bool) identityrepo.InstanceLevel {
	return identityrepo.InstanceLevel{Ai: domain.AiLayer{
		Sources: &sources, Locks: map[domain.AiSwitch]bool{domain.AiSwitchSources: locked},
	}}
}

func readSource(t *testing.T, registry *usecase.Registry) usecase.Output {
	t.Helper()
	out, err := registry.Invoke(t.Context(), ReadAiSourceName, providerActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	return out
}

// D1: nothing offered and nothing decided is today's screen - the workspace's own model and
// nothing else (UC-AI-06 check 8).
func TestAPrivateInstallationSeesOnlyItsOwnModel(t *testing.T) {
	out := readSource(t, sourceRegistry(t, &sourceWorld{}))
	if out["sources"] != "OWN" || out["source"] != "OWN" {
		t.Errorf("answered %v", out)
	}
	if offered, _ := out["offered"].([]usecase.Output); len(offered) != 0 {
		t.Errorf("a private installation was offered %v", offered)
	}
	if _, locked := out["sources_lock"]; locked {
		t.Error("a private installation meets a lock")
	}
}

// UC-AI-05 checks 1, 3 and 6: every offered model by name, processor and jurisdiction - and not
// one byte of its address, its models or its key.
func TestAWorkspaceSeesOfferedModelsByNameAndNothingMore(t *testing.T) {
	world := &sourceWorld{
		level:   instanceSays(domain.AiSourcesEither, false),
		offered: []domain.AiProvider{offeredModel(offeredLarge, "Large (EU)"), offeredModel(offeredSmall, "Small (EU)")},
	}
	out := readSource(t, sourceRegistry(t, world))

	offered, _ := out["offered"].([]usecase.Output)
	if len(offered) != 2 || offered[0]["display_name"] != "Large (EU)" || offered[0]["processor"] != "A processor" ||
		offered[0]["jurisdiction"] != "EEA" {
		t.Fatalf("offered %v", offered)
	}
	answer, _ := json.Marshal(out)
	for _, secret := range []string{"secret-address", "secret-model", "secret-embed", "has_api_key", "base_url"} {
		if strings.Contains(string(answer), secret) {
			t.Errorf("the workspace's answer carries %q: %s", secret, answer)
		}
	}
}

// UC-AI-06 check 3, UC-AI-02 check 5: the answer offers exactly what the rule allows.
func TestTheAnswerOffersExactlyTheAllowedSources(t *testing.T) {
	for _, testCase := range []struct {
		rule          domain.AiSources
		wantSources   string
		wantOfferedIn bool
	}{
		{domain.AiSourcesOwn, "OWN", false},
		{domain.AiSourcesOffered, "OFFERED", true},
		{domain.AiSourcesEither, "EITHER", true},
		{domain.AiSourcesNone, "NONE", false},
	} {
		t.Run(string(testCase.rule), func(t *testing.T) {
			world := &sourceWorld{
				level:   instanceSays(testCase.rule, true),
				offered: []domain.AiProvider{offeredModel(offeredLarge, "Large (EU)")},
			}
			out := readSource(t, sourceRegistry(t, world))
			if out["sources"] != testCase.wantSources || out["sources_lock"] != "INSTANCE" {
				t.Errorf("answered %v", out)
			}
			offered, _ := out["offered"].([]usecase.Output)
			if (len(offered) > 0) != testCase.wantOfferedIn {
				t.Errorf("offered %v under %s", offered, testCase.rule)
			}
		})
	}
}

// UC-AI-05 checks 2 and 7: choosing the offered model is one write; the own configuration is not
// touched by it, and choosing back is one write too.
func TestAWorkspaceSwitchesBetweenOfferedAndOwn(t *testing.T) {
	world := &sourceWorld{
		level:   instanceSays(domain.AiSourcesEither, false),
		offered: []domain.AiProvider{offeredModel(offeredLarge, "Large (EU)")},
	}
	registry := sourceRegistry(t, world)

	out, err := registry.Invoke(t.Context(), ChooseAiSourceName, providerActor(), usecase.Input{
		"source": "OFFERED", "offered_model_id": offeredLarge.String(),
	})
	if err != nil {
		t.Fatalf("choosing the offered model: %v", err)
	}
	if out["source"] != "OFFERED" || out["offered_model_id"] != offeredLarge.String() {
		t.Errorf("answered %v", out)
	}
	if world.reseeded != 1 {
		t.Errorf("switching the model behind search seeded %d re-embeddings, want one", world.reseeded)
	}
	if _, err := registry.Invoke(t.Context(), ChooseAiSourceName, providerActor(), usecase.Input{"source": "OWN"}); err != nil {
		t.Fatalf("choosing back: %v", err)
	}
	if world.choice.Source != domain.AiSourceOwn || !world.choice.OfferedProviderID.IsZero() {
		t.Errorf("the choice is %+v", world.choice)
	}
}

func TestASourceTheRuleExcludesIsRefusedAndNoneIsAlwaysAllowed(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		rule  domain.AiSources
		input usecase.Input
		code  string
	}{
		{"offered where only own is allowed", domain.AiSourcesOwn,
			usecase.Input{"source": "OFFERED", "offered_model_id": offeredLarge.String()}, "ai.source_not_allowed"},
		{"own where only offered is allowed", domain.AiSourcesOffered, usecase.Input{"source": "OWN"}, "ai.source_not_allowed"},
		{"a model nobody offers", domain.AiSourcesEither,
			usecase.Input{"source": "OFFERED", "offered_model_id": offeredSmall.String()}, "ai.offered_model_unknown"},
		{"none under a lock to offered", domain.AiSourcesOffered, usecase.Input{"source": "NONE"}, ""},
		{"none under a lock to none", domain.AiSourcesNone, usecase.Input{"source": "NONE"}, ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			world := &sourceWorld{
				level:   instanceSays(testCase.rule, true),
				offered: []domain.AiProvider{offeredModel(offeredLarge, "Large (EU)")},
			}
			_, err := sourceRegistry(t, world).Invoke(t.Context(), ChooseAiSourceName, providerActor(), testCase.input)
			got := shared.AsError(err)
			if (got == nil) != (testCase.code == "") || (got != nil && got.DetailCode != testCase.code) {
				t.Errorf("got %v, want %q", err, testCase.code)
			}
			if testCase.code != "" && world.choice != nil {
				t.Error("a refused choice was stored")
			}
		})
	}
}

// UC-AI-06 check 2: a workspace narrows where nothing is locked, never widens, and the narrowing
// lives in its archived settings.
func TestAWorkspaceNarrowsWhereItMayAndOnlyThere(t *testing.T) {
	open := &sourceWorld{level: instanceSays(domain.AiSourcesEither, false),
		offered: []domain.AiProvider{offeredModel(offeredLarge, "Large (EU)")}}
	registry := sourceRegistry(t, open)
	out, err := registry.Invoke(t.Context(), ChooseAiSourceName, providerActor(), usecase.Input{"source": "OWN", "sources": "OWN"})
	if err != nil {
		t.Fatalf("narrowing: %v", err)
	}
	if out["sources"] != "OWN" || out["narrowing"] != "OWN" || open.workspace.Settings.AiSources != "OWN" {
		t.Errorf("narrowed to %v, settings %q", out, open.workspace.Settings.AiSources)
	}
	// Narrowed to its own, the offered model is no longer a choice.
	if _, err := registry.Invoke(t.Context(), ChooseAiSourceName, providerActor(), usecase.Input{
		"source": "OFFERED", "offered_model_id": offeredLarge.String(),
	}); shared.AsError(err) == nil || shared.AsError(err).DetailCode != "ai.source_not_allowed" {
		t.Errorf("an offered choice past the workspace's own narrowing answered %v", err)
	}
	// INHERIT takes back what the installation allows.
	if out, err := registry.Invoke(t.Context(), ChooseAiSourceName, providerActor(), usecase.Input{"source": "OWN", "sources": "INHERIT"}); err != nil || out["sources"] != "EITHER" {
		t.Errorf("inheriting answered %v (%v)", out, err)
	}

	locked := &sourceWorld{level: instanceSays(domain.AiSourcesEither, true),
		offered: []domain.AiProvider{offeredModel(offeredLarge, "Large (EU)")}}
	_, err = sourceRegistry(t, locked).Invoke(t.Context(), ChooseAiSourceName, providerActor(), usecase.Input{"source": "OWN", "sources": "OWN"})
	if got := shared.AsError(err); got == nil || got.DetailCode != "auth.policy_locked" {
		t.Errorf("a narrowing under a lock answered %v", err)
	}

	narrow := &sourceWorld{level: instanceSays(domain.AiSourcesOwn, false)}
	_, err = sourceRegistry(t, narrow).Invoke(t.Context(), ChooseAiSourceName, providerActor(), usecase.Input{"source": "OWN", "sources": "EITHER"})
	if got := shared.AsError(err); got == nil || got.DetailCode != "auth.policy_loosens" {
		t.Errorf("a widening answered %v", err)
	}
}

// The model behind search moves only when the source moves: choosing the same again seeds nothing.
func TestChoosingTheSameSourceAgainSeedsNothing(t *testing.T) {
	world := &sourceWorld{}
	registry := sourceRegistry(t, world)
	for range 2 {
		if _, err := registry.Invoke(t.Context(), ChooseAiSourceName, providerActor(), usecase.Input{"source": "OWN"}); err != nil {
			t.Fatalf("choosing: %v", err)
		}
	}
	if world.reseeded != 0 {
		t.Errorf("an unchanged source seeded %d re-embeddings", world.reseeded)
	}
}
