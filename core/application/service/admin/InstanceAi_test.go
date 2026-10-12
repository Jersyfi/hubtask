// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
)

// The AI models the installation offers (ADR-0072 §1, UC-AI-06 check 1), through the registry.

// offeredStore is the offered models, in memory.
type offeredStore struct {
	rows map[shared.ID]integration.AiProvider
	keys map[shared.ID]*cryptoport.Sealed
}

func newOfferedStore() *offeredStore {
	return &offeredStore{rows: map[shared.ID]integration.AiProvider{}, keys: map[shared.ID]*cryptoport.Sealed{}}
}

func (s *offeredStore) List(context.Context) ([]integration.AiProvider, error) {
	listed := make([]integration.AiProvider, 0, len(s.rows))
	for _, row := range s.rows {
		listed = append(listed, row)
	}
	return listed, nil
}

func (s *offeredStore) Find(_ context.Context, id shared.ID) (integration.AiProvider, error) {
	row, held := s.rows[id]
	if !held {
		return integration.AiProvider{}, shared.ErrNotFound.WithDetail("ai.offered_model_unknown")
	}
	return row, nil
}

func (s *offeredStore) FindWithKey(ctx context.Context, id shared.ID) (integration.AiProvider, *cryptoport.Sealed, error) {
	row, err := s.Find(ctx, id)
	return row, s.keys[id], err
}

func (s *offeredStore) Insert(_ context.Context, offered integration.AiProvider, sealed *cryptoport.Sealed, _ time.Time) (integration.AiProvider, error) {
	offered.HasAPIKey = sealed != nil
	s.rows[offered.ID], s.keys[offered.ID] = offered, sealed
	return offered, nil
}

func (s *offeredStore) Update(ctx context.Context, offered integration.AiProvider, sealed *cryptoport.Sealed, now time.Time) (integration.AiProvider, error) {
	if _, held := s.rows[offered.ID]; !held {
		return integration.AiProvider{}, shared.ErrNotFound.WithDetail("ai.offered_model_unknown")
	}
	return s.Insert(ctx, offered, sealed, now)
}

func (s *offeredStore) UpdateKeepingKey(ctx context.Context, offered integration.AiProvider, now time.Time) (integration.AiProvider, error) {
	return s.Update(ctx, offered, s.keys[offered.ID], now)
}

func (s *offeredStore) Delete(_ context.Context, id shared.ID) (bool, error) {
	_, held := s.rows[id]
	delete(s.rows, id)
	return held, nil
}

func offeredRegistry(t *testing.T, confirmed bool) (*usecase.Registry, *offeredStore, *plainEnvelope, *journalStore) {
	t.Helper()
	instance, _, journal := newInstanceWriter(newRegister(operatorID))
	store, envelope := newOfferedStore(), &plainEnvelope{}
	writer := InstanceAiWriter{Instance: instance, Offered: store, Encryptor: envelope, ThirdCountryConfirmed: confirmed}
	registry, err := usecase.NewRegistry(nil,
		ListInstanceAiProviders{Writer: writer}.Descriptor(),
		ConfigureInstanceAiProvider{Writer: writer}.Descriptor(),
		RemoveInstanceAiProvider{Writer: writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("not registrable: %v", err)
	}
	return registry, store, envelope, journal
}

func offeredInput() usecase.Input {
	return usecase.Input{
		"display_name": "Large (EU)", "processor": "A processor", "kind": "OPENAI_COMPATIBLE",
		"base_url": "https://offered.example/v1", "completion_model": "large",
		"embedding_model": "embed", "jurisdiction": "EEA", "api_key": "the-key",
	}
}

func TestAnOperatorOffersChangesAndWithdrawsAModel(t *testing.T) {
	registry, store, envelope, journal := offeredRegistry(t, false)

	created, err := registry.Invoke(t.Context(), ConfigureInstanceAiProviderName, operator(), offeredInput())
	if err != nil {
		t.Fatalf("offering: %v", err)
	}
	id := shared.ID(created.String("id"))
	if created["has_api_key"] != true || store.rows[id].ProcessingAllowed {
		t.Errorf("offered as %v", created)
	}
	// The key is sealed for this model, and never answered.
	if len(envelope.purposes) != 1 || !strings.HasSuffix(string(envelope.purposes[0]), "offered:"+id.String()) {
		t.Errorf("sealed under %v", envelope.purposes)
	}
	answer, _ := json.Marshal(created)
	if strings.Contains(string(answer), "the-key") || strings.Contains(string(answer), "sealed:") {
		t.Errorf("the answer carries the key: %s", answer)
	}

	// A change without a key keeps the one sealed.
	change := offeredInput()
	delete(change, "api_key")
	change["id"], change["display_name"] = id.String(), "Large (EU), renamed"
	if _, err := registry.Invoke(t.Context(), ConfigureInstanceAiProviderName, operator(), change); err != nil {
		t.Fatalf("changing: %v", err)
	}
	if store.keys[id] == nil || store.rows[id].DisplayName != "Large (EU), renamed" {
		t.Errorf("the change lost the key or the name: %+v", store.rows[id])
	}

	listed, err := registry.Invoke(t.Context(), ListInstanceAiProvidersName, operator(), usecase.Input{})
	if rows, _ := listed["data"].([]usecase.Output); err != nil || len(rows) != 1 {
		t.Errorf("listed %v (%v)", listed, err)
	}

	if _, err := registry.Invoke(t.Context(), RemoveInstanceAiProviderName, operator(), usecase.Input{"id": id.String()}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if len(store.rows) != 0 {
		t.Error("the model is still offered")
	}
	actions := []string{}
	for _, entry := range journal.entries {
		actions = append(actions, entry.Action)
		if details, _ := json.Marshal(entry.Details); strings.Contains(string(details), "the-key") {
			t.Errorf("the journal carries the key: %s", details)
		}
	}
	if strings.Join(actions, ",") != "instance.ai_provider_configured,instance.ai_provider_configured,instance.ai_provider_removed" {
		t.Errorf("journalled %v", actions)
	}
}

func TestOnlyARegisteredOperatorOffersAModel(t *testing.T) {
	registry, store, _, _ := offeredRegistry(t, false)

	outsider := operator()
	outsider.AccountID = secondOperator
	if _, err := registry.Invoke(t.Context(), ConfigureInstanceAiProviderName, outsider, offeredInput()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("an account outside the register answered %v", err)
	}
	workspaceAdmin := operator()
	workspaceAdmin.Scopes = nil
	if _, err := registry.Invoke(t.Context(), ListInstanceAiProvidersName, workspaceAdmin, usecase.Input{}); err == nil {
		t.Error("a credential without admin:tenants listed the offered models")
	}
	if len(store.rows) != 0 {
		t.Error("a refused offer was stored")
	}
}

func TestAnOfferedModelIsHeldToTheWorkspacesRules(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		amend     func(usecase.Input)
		confirmed bool
		code      string
	}{
		{"a third country without the confirmation", func(in usecase.Input) { in["jurisdiction"] = "THIRD_COUNTRY" }, false, "ai.third_country_not_confirmed"},
		// Required fields are the registry's to refuse, before the use case runs.
		{"no processor", func(in usecase.Input) { in["processor"] = "" }, false, "usecase.input_invalid"},
		{"offering nothing", func(in usecase.Input) { in["kind"] = "NOOP" }, false, "ai.offered_needs_a_model"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			registry, store, envelope, _ := offeredRegistry(t, testCase.confirmed)
			in := offeredInput()
			testCase.amend(in)
			_, err := registry.Invoke(t.Context(), ConfigureInstanceAiProviderName, operator(), in)
			if got := shared.AsError(err); got == nil || got.DetailCode != testCase.code {
				t.Errorf("got %v, want %s", err, testCase.code)
			}
			if len(store.rows) != 0 || len(envelope.purposes) != 0 {
				t.Error("a refused model left a row or an envelope")
			}
		})
	}

	registry, _, _, _ := offeredRegistry(t, true)
	in := offeredInput()
	in["jurisdiction"] = "THIRD_COUNTRY"
	if _, err := registry.Invoke(t.Context(), ConfigureInstanceAiProviderName, operator(), in); err != nil {
		t.Errorf("a confirmed installation was refused a third country: %v", err)
	}
}
