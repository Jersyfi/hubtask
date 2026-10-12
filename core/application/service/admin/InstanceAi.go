// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	integrationrepo "github.com/Jersyfi/hubtask/core/application/repository/integration"
	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The AI models the installation offers every workspace (ADR-0072 §1).
//
// One store with the workspaces' own providers, as the sign-in providers are: a row with no
// workspace is the installation's, read by every workspace that chose it and written by none - the
// policies of migration 0129. What is written here is who may change those rows: the scope and the
// operator register.
//
// Offering a model sends nothing anywhere. Each workspace switches it on itself, and that choice is
// its consent (ADR-0072 §3) - which is why offering asks no step-up beyond the raised session that
// carries `admin:tenants`, as a workspace configuring its own model asks none either.

const (
	ListInstanceAiProvidersName     = "ListInstanceAiProviders"
	ConfigureInstanceAiProviderName = "ConfigureInstanceAiProvider"
	RemoveInstanceAiProviderName    = "RemoveInstanceAiProvider"

	instanceAiTarget = "ai_provider"
)

const (
	instanceAiConfiguredAction audit.Action = "instance.ai_provider_configured"
	instanceAiRemovedAction    audit.Action = "instance.ai_provider_removed"
	instanceAiReadAction       audit.Action = "instance.ai_provider_read"
)

// InstanceAiWriter is the control plane's half of the AI provider store.
type InstanceAiWriter struct {
	// Instance carries the scope, the register, the journal, the clock and the identifiers.
	Instance InstanceWriter
	Offered  integrationrepo.OfferedAiProviders
	// Encryptor seals the key between the request and the store.
	Encryptor crypto.Encryptor
	// ThirdCountryConfirmed is the installation's confirmation of third-country transfers, the
	// same one a workspace's own model is held to: an offered model outside the EEA is a transfer
	// whoever chooses it makes.
	ThirdCountryConfirmed bool
}

// ListInstanceAiProviders answers what the installation offers.
type ListInstanceAiProviders struct{ Writer InstanceAiWriter }

// Execute reads them, without a key.
func (h ListInstanceAiProviders) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]integration.AiProvider, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return nil, err
	}
	var found []integration.AiProvider
	err := w.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := w.Offered.List(ctx)
			found = read
			return err
		})
	return found, err
}

// ConfigureInstanceAiProvider offers a model, or changes one.
type ConfigureInstanceAiProvider struct{ Writer InstanceAiWriter }

// ConfigureInstanceAiCommand is the input, typed. A zero ID offers a new model.
type ConfigureInstanceAiCommand struct {
	ID              shared.ID
	DisplayName     string
	Processor       string
	Kind            integration.AiProviderKind
	BaseURL         string
	CompletionModel string
	EmbeddingModel  string
	Jurisdiction    integration.AiJurisdiction
	// APIKey is present only when the caller sent one: absent keeps the stored key on a change,
	// present and empty clears it.
	APIKey        secret.Secret
	APIKeyPresent bool
}

// Execute validates, checks the transfer, seals the key and stores the lot - in that order, so a
// refused model leaves no envelope and no row behind.
func (h ConfigureInstanceAiProvider) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ConfigureInstanceAiCommand,
) (integration.AiProvider, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return integration.AiProvider{}, err
	}
	if err := w.notEnforced(ctx); err != nil {
		return integration.AiProvider{}, err
	}
	return w.configure(ctx, actor.AccountName, cmd)
}

// notEnforced refuses a write through the screen or hubctl while a file in `enforce` mode is the
// source: the next start would undo it (ADR-0070 §5), the reason WriteInstanceSettings refuses.
func (w InstanceAiWriter) notEnforced(ctx context.Context) error {
	if w.Instance.Settings == nil {
		return nil
	}
	return w.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		level, err := w.Instance.Settings.Read(ctx)
		if err != nil {
			return err
		}
		if level.IsEnforcedFromFile {
			return shared.ErrConflict.WithDetail("admin.instance_enforced_from_file").
				WithParams(map[string]string{"source": level.Source})
		}
		return nil
	})
}

// configure is the write, after whoever asked was allowed to: an operator through the API, or the
// instance file at start.
func (w InstanceAiWriter) configure(
	ctx context.Context, label string, cmd ConfigureInstanceAiCommand,
) (integration.AiProvider, error) {
	adding := cmd.ID.IsZero()
	id := cmd.ID
	if adding {
		id = w.Instance.IDs.NewID()
	}
	offered, err := integration.NewOfferedAiProvider(integration.NewOfferedAiProviderInput{
		ID: id, DisplayName: cmd.DisplayName, Processor: cmd.Processor, Kind: cmd.Kind,
		BaseURL: cmd.BaseURL, CompletionModel: cmd.CompletionModel, EmbeddingModel: cmd.EmbeddingModel,
		Jurisdiction: cmd.Jurisdiction, Now: w.Instance.Clock.Now(),
	})
	if err != nil {
		return integration.AiProvider{}, err
	}
	if offered.Jurisdiction.NeedsThirdCountryConfirmation() && !w.ThirdCountryConfirmed {
		return integration.AiProvider{}, shared.ErrForbidden.
			WithDetail("ai.third_country_not_confirmed").
			WithFields(shared.FieldError{Path: "/jurisdiction", Code: "ai.third_country_not_confirmed"})
	}

	var sealed *crypto.Sealed
	if cmd.APIKeyPresent && !cmd.APIKey.IsEmpty() {
		envelope, err := w.Encryptor.Seal(ctx, cmd.APIKey, integrationservice.OfferedAiKeyPurpose(id))
		if err != nil {
			return integration.AiProvider{}, err
		}
		sealed = &envelope
	}

	var stored integration.AiProvider
	err = w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		now := w.Instance.Clock.Now()
		var err error
		switch {
		case adding:
			stored, err = w.Offered.Insert(ctx, offered, sealed, now)
		case cmd.APIKeyPresent:
			stored, err = w.Offered.Update(ctx, offered, sealed, now)
		default:
			stored, err = w.Offered.UpdateKeepingKey(ctx, offered, now)
		}
		if err != nil {
			return err
		}
		return w.Instance.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.Instance.IDs.NewID(), OccurredAt: now, Action: string(instanceAiConfiguredAction),
			ActorLabel: label,
			// Configuration, never the key: the name, where it processes and which models.
			Details: map[string]any{
				"provider_id": stored.ID.String(), "jurisdiction": string(stored.Jurisdiction),
				"kind": string(stored.Kind), "completion_model": stored.CompletionModel,
				"embedding_model": stored.EmbeddingModel,
			},
		})
	})
	if err != nil {
		return integration.AiProvider{}, err
	}
	return stored, nil
}

// RemoveInstanceAiProvider withdraws an offered model. Every workspace that chose it stops getting
// answers at its next call; its own configuration, if it has one, is untouched (ADR-0072 §5).
type RemoveInstanceAiProvider struct{ Writer InstanceAiWriter }

// Execute removes it. An unknown model is not an error: removing it is somebody making sure.
func (h RemoveInstanceAiProvider) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return err
	}
	if err := w.notEnforced(ctx); err != nil {
		return err
	}
	return w.remove(ctx, actor.AccountName, id)
}

func (w InstanceAiWriter) remove(ctx context.Context, label string, id shared.ID) error {
	return w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		removed, err := w.Offered.Delete(ctx, id)
		if err != nil || !removed {
			return err
		}
		return w.Instance.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.Instance.IDs.NewID(), OccurredAt: w.Instance.Clock.Now(),
			Action: string(instanceAiRemovedAction), ActorLabel: label,
			Details: map[string]any{"provider_id": id.String()},
		})
	})
}

// ApplyFile is the instance file's door (ADR-0070 §5, D9 of the readiness record): the models the
// file names, matched to offered ones by name. `seed` offers them only where nothing is offered
// yet; `enforce` makes the file the list - named models are offered or changed, and every other
// offered model is withdrawn. A model whose key the file names by variable gets it; one whose key
// it does not name keeps the sealed one. The journal names the file.
func (w InstanceAiWriter) ApplyFile(
	ctx context.Context, models []ConfigureInstanceAiCommand, enforce bool, source string,
) error {
	var offered []integration.AiProvider
	if err := w.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		read, err := w.Offered.List(ctx)
		offered = read
		return err
	}); err != nil {
		return err
	}
	if !enforce && len(offered) > 0 {
		return nil
	}
	byName := map[string]shared.ID{}
	for _, model := range offered {
		byName[model.DisplayName] = model.ID
	}
	named := map[shared.ID]bool{}
	for _, cmd := range models {
		cmd.ID = byName[cmd.DisplayName]
		stored, err := w.configure(ctx, source, cmd)
		if err != nil {
			return err
		}
		named[stored.ID] = true
	}
	if !enforce {
		return nil
	}
	for _, model := range offered {
		if !named[model.ID] {
			if err := w.remove(ctx, source, model.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// OfferedAiOutput is what an operator reads of an offered model: everything but the key.
func OfferedAiOutput(offered integration.AiProvider) usecase.Output {
	out := usecase.Output{
		"id":               offered.ID.String(),
		"display_name":     offered.DisplayName,
		"processor":        offered.Processor,
		"kind":             string(offered.Kind),
		"base_url":         offered.BaseURL,
		"completion_model": offered.CompletionModel,
		"embedding_model":  offered.EmbeddingModel,
		"has_api_key":      offered.HasAPIKey,
		"jurisdiction":     string(offered.Jurisdiction),
		"created_at":       offered.CreatedAt,
		"version":          offered.Version,
	}
	if !offered.UpdatedAt.IsZero() {
		out["updated_at"] = offered.UpdatedAt
	}
	return out
}

func (h ListInstanceAiProviders) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListInstanceAiProvidersName,
		Summary: "The AI models this installation offers every workspace on it. A workspace sees " +
			"only each one's name, processor and jurisdiction; the address, the models and the " +
			"key are the operator's. The key is not among the fields, and no call answers it.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: instanceAiReadAction, TargetType: instanceAiTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListInstanceAiProviders) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	found, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(found))
	for _, offered := range found {
		rows = append(rows, OfferedAiOutput(offered))
	}
	return usecase.Output{"data": rows}, nil
}

func (h ConfigureInstanceAiProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfigureInstanceAiProviderName,
		Summary: "Offers an AI model to every workspace on this installation, or changes one. The " +
			"same rules as a workspace's own model, plus a name and a processor every workspace " +
			"is shown before it agrees. Offering switches nothing on: each workspace chooses the " +
			"model itself, and that choice is its consent. Sending no identifier adds one.",
		SideEffects: "Writes the configuration, seals the API key, and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		Input: []usecase.Field{
			{Name: "id", Kind: usecase.KindString,
				Description: "The offered model to change. Absent offers a new one."},
			{Name: "display_name", Kind: usecase.KindString, Required: true,
				Description: "The name workspaces choose it by. Unique among offered models."},
			{Name: "processor", Kind: usecase.KindString, Required: true,
				Description: "Who processes what is sent to it, as the operator declares it."},
			{Name: "kind", Kind: usecase.KindString, Required: true,
				Enum:        enumOf(integration.AiProviderKinds()),
				Description: "Which adapter answers. NOOP is refused: offering nothing offers nothing."},
			{Name: "base_url", Kind: usecase.KindString, Required: true,
				Description: "The endpoint the adapter calls."},
			{Name: "completion_model", Kind: usecase.KindString,
				Description: "The model a suggestion is asked of. Empty means it does not complete."},
			{Name: "embedding_model", Kind: usecase.KindString,
				Description: "The model a vector is asked of. Empty means it does not embed."},
			{Name: "api_key", Kind: usecase.KindString,
				Description: "Sealed on the way in and answered by nothing. Omit to keep the stored key; send empty to clear it."},
			{Name: "jurisdiction", Kind: usecase.KindString, Required: true,
				Enum:        enumOf(integration.AiJurisdictions()),
				Description: "Where the model processes what is sent to it, as the operator declares it."},
		},
		Audit: usecase.AuditDeclaration{
			Action: instanceAiConfiguredAction, TargetType: instanceAiTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ConfigureInstanceAiProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd := ConfigureInstanceAiCommand{
		DisplayName:     in.String("display_name"),
		Processor:       in.String("processor"),
		Kind:            integration.AiProviderKind(in.String("kind")),
		BaseURL:         in.String("base_url"),
		CompletionModel: in.String("completion_model"),
		EmbeddingModel:  in.String("embedding_model"),
		Jurisdiction:    integration.AiJurisdiction(in.String("jurisdiction")),
	}
	if in.Present("id") && in.String("id") != "" {
		id, err := in.ID("id")
		if err != nil {
			return nil, err
		}
		cmd.ID = id
	}
	if in.Present("api_key") {
		cmd.APIKeyPresent = true
		cmd.APIKey = secret.New(in.String("api_key"))
	}
	stored, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return OfferedAiOutput(stored), nil
}

func (h RemoveInstanceAiProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemoveInstanceAiProviderName,
		Summary: "Withdraws an AI model the installation offered, and its sealed key. Every " +
			"workspace that chose it gets no AI answer until it chooses again; nothing a workspace " +
			"configured itself is touched, and no suggestion anybody accepted is undone.",
		SideEffects: "Deletes the configuration and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "id", Kind: usecase.KindString, Required: true,
				Description: "The offered model to withdraw."},
		},
		Audit: usecase.AuditDeclaration{
			Action: instanceAiRemovedAction, TargetType: instanceAiTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RemoveInstanceAiProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return usecase.Output{}, nil
		}
		return nil, err
	}
	return usecase.Output{}, nil
}

// enumOf renders a closed set of typed values as the strings a descriptor declares.
func enumOf[T ~string](values []T) []string {
	declared := make([]string, 0, len(values))
	for _, value := range values {
		declared = append(declared, string(value))
	}
	return declared
}
