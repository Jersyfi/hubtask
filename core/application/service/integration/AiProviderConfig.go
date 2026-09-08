// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	ConfigureAiProviderName = "ConfigureAiProvider"
	ReadAiProviderName      = "ReadAiProvider"
	RemoveAiProviderName    = "RemoveAiProvider"

	// aiManage is the registry's scope (api-guidelines.md §7). Its own rather than shared with
	// the webhook subscriptions': deciding where this workspace's content may be sent is not the
	// same act as deciding who is told that it changed.
	aiManage = "ai:manage"

	aiProviderTarget = "ai_provider"
)

// The audit codes of the AI surface. ADR-0018 decision 7 asks for the use to be audited with the
// provider, the region, the model and the purpose; the first three are the entry's changes and the
// fourth is the action itself, which is what an action name is for.
const (
	AiProviderConfiguredAction audit.Action = "ai.provider_configured"
	AiProviderRemovedAction    audit.Action = "ai.provider_removed"
	AiProviderReadAction       audit.Action = "ai.provider_read"
)

// AiProviderWriter is what the three configuration use cases share.
type AiProviderWriter struct {
	Providers  repository.AiProviders
	Authorizer Authorizer
	// Encryptor seals the API key. The application never stores a plaintext and the repository
	// never holds a key: the sealing happens here, between the two (E-02).
	Encryptor  crypto.Encryptor
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// ThirdCountryConfirmed is the installation operator's confirmation that a provider outside
	// the EEA may be used (HUBTASK_AI_ALLOW_THIRD_COUNTRY_TRANSFER, data-protection.md §6,
	// ADR-0018 decision 7).
	//
	// The installation's and not the workspace's, deliberately. It is the operator who signs the
	// processing agreement and who owes the transfer impact assessment, and a workspace
	// administrator cannot take either of those on for them - so the friction sits where the
	// responsibility does. False by default, which is what makes it friction rather than a
	// formality.
	ThirdCountryConfirmed bool
}

// ConfigureAiProvider points a workspace at its AI provider, or switches it off.
type ConfigureAiProvider struct{ Writer AiProviderWriter }

// ConfigureAiProviderCommand is the input, typed.
type ConfigureAiProviderCommand struct {
	Kind            domain.AiProviderKind
	BaseURL         string
	CompletionModel string
	EmbeddingModel  string
	// APIKey is present only when the caller sent one. Absent keeps whatever is stored; present
	// and empty clears it, which is what a local provider that needs none wants.
	APIKey            secret.Secret
	APIKeyPresent     bool
	Jurisdiction      domain.AiJurisdiction
	ProcessingAllowed bool
}

// Execute validates, checks the transfer, seals the key and stores the lot.
//
// The order matters and is the identity provider's: everything that can be refused is refused
// before anything is sealed or written, so a rejected configuration leaves no envelope behind and
// no half-written row.
func (h ConfigureAiProvider) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ConfigureAiProviderCommand,
) (domain.AiProvider, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, aiRequest(AiProviderConfiguredAction)); err != nil {
		return domain.AiProvider{}, err
	}

	configured, err := domain.NewAiProvider(domain.NewAiProviderInput{
		TenantID: actor.TenantID, Kind: cmd.Kind, BaseURL: cmd.BaseURL,
		CompletionModel: cmd.CompletionModel, EmbeddingModel: cmd.EmbeddingModel,
		Jurisdiction: cmd.Jurisdiction, ProcessingAllowed: cmd.ProcessingAllowed,
		Now: w.Clock.Now(),
	})
	if err != nil {
		return domain.AiProvider{}, err
	}

	// PG-8, at configuration time rather than at call time. A rejected write is a decision
	// somebody can reconsider while they are still looking at the form; a rejected suggestion two
	// weeks later is an outage nobody can explain.
	if configured.Jurisdiction.NeedsThirdCountryConfirmation() && !w.ThirdCountryConfirmed {
		return domain.AiProvider{}, shared.ErrForbidden.
			WithDetail("ai.third_country_not_confirmed").
			WithFields(shared.FieldError{
				Path: "/jurisdiction", Code: "ai.third_country_not_confirmed",
			})
	}

	var sealed *crypto.Sealed
	if cmd.APIKeyPresent && !cmd.APIKey.IsEmpty() {
		envelope, err := w.Encryptor.Seal(ctx, cmd.APIKey, AiKeyPurpose(actor.TenantID))
		if err != nil {
			return domain.AiProvider{}, err
		}
		sealed = &envelope
	}

	var stored domain.AiProvider
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if cmd.APIKeyPresent {
			stored, err = w.Providers.Upsert(ctx, configured, sealed, w.Clock.Now())
		} else {
			stored, err = w.Providers.UpsertKeepingKey(ctx, configured, w.Clock.Now())
		}
		if err != nil {
			return err
		}
		return w.record(ctx, actor, AiProviderConfiguredAction, stored)
	})
	if err != nil {
		return domain.AiProvider{}, err
	}
	return stored, nil
}

// ReadAiProvider answers the configuration, never the key.
type ReadAiProvider struct{ Writer AiProviderWriter }

// Execute reads it.
func (h ReadAiProvider) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (domain.AiProvider, error) {
	w := h.Writer
	request := aiRequest(AiProviderReadAction)
	// A-4: where a workspace's content may be sent is configuration, and an auditor reads
	// configuration without being able to change it.
	request.Alternative = service.PermissionReadConfiguration
	if err := w.Authorizer.Authorize(ctx, actor, request); err != nil {
		return domain.AiProvider{}, err
	}

	var found domain.AiProvider
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			stored, err := w.Providers.Find(ctx)
			found = stored
			return err
		})
	if err != nil {
		return domain.AiProvider{}, err
	}
	return found, nil
}

// RemoveAiProvider takes the configuration away.
type RemoveAiProvider struct{ Writer AiProviderWriter }

// Execute removes it.
//
// Nothing else is undone. A suggestion somebody already accepted was that person's own write and
// stays exactly as it is, which is the whole reason accepting is a use case rather than a side
// effect (J-05).
func (h RemoveAiProvider) Execute(ctx context.Context, actor appshared.ActorContext) error {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, aiRequest(AiProviderRemovedAction)); err != nil {
		return err
	}

	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := w.Providers.Delete(ctx)
		if err != nil {
			return err
		}
		if !removed {
			// Nothing to remove is not a failure: the caller asked for it to be gone and it is.
			return nil
		}
		return w.record(ctx, actor, AiProviderRemovedAction, domain.AiProvider{TenantID: actor.TenantID})
	})
}

// aiRequest is the authorisation question the three share, spelled once so the permission, the
// scope and the target cannot drift between them.
func aiRequest(action audit.Action) access.Request {
	return access.Request{
		// STRUCTURE rather than AUTOMATION: choosing where this workspace's content may be sent
		// is shaping the workspace, not wiring it to something. It is the same permission the
		// quota standing and the backup targets are read under, and the auditor's read-only
		// alternative is added by the one read below (A-4).
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     action,
		TokenScope: aiManage,
		TargetType: aiProviderTarget,
	}
}

// record writes the trail entry.
//
// The kind, the jurisdiction and the models travel with it - all three are configuration, not
// credentials, and ADR-0018 decision 7 asks for exactly them. The key never does, and neither does
// the endpoint's user info, because there cannot be any: the domain refuses an address that
// carries credentials.
func (w AiProviderWriter) record(
	ctx context.Context, actor appshared.ActorContext,
	action audit.Action, configured domain.AiProvider,
) error {
	changes := []audit.Change{}
	if configured.Kind != "" {
		changes = append(changes,
			audit.Change{Field: "kind", Classification: audit.Open, To: string(configured.Kind)},
			audit.Change{Field: "jurisdiction", Classification: audit.Open,
				To: string(configured.Jurisdiction)},
			audit.Change{Field: "base_url", Classification: audit.Open, To: configured.BaseURL},
			audit.Change{Field: "completion_model", Classification: audit.Open,
				To: configured.CompletionModel},
			audit.Change{Field: "embedding_model", Classification: audit.Open,
				To: configured.EmbeddingModel},
			audit.Change{Field: "processing_allowed", Classification: audit.Open,
				To: boolText(configured.ProcessingAllowed)})
	}
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: w.Clock.Now(),
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: aiProviderTarget,
		// The workspace is the target: there is one provider and its identity is the workspace's.
		TargetID: actor.TenantID,
		Context:  audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:  audit.Changes(changes...),
	})
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// aiProviderOutput is the read shape, and the key is not in it - here as much as in the REST
// answer, because the registry serves MCP and automation from the same map.
func aiProviderOutput(configured domain.AiProvider) usecase.Output {
	out := usecase.Output{
		"kind":               string(configured.Kind),
		"base_url":           configured.BaseURL,
		"completion_model":   configured.CompletionModel,
		"embedding_model":    configured.EmbeddingModel,
		"has_api_key":        configured.HasAPIKey,
		"jurisdiction":       string(configured.Jurisdiction),
		"processing_allowed": configured.ProcessingAllowed,
		"created_at":         configured.CreatedAt,
		"version":            configured.Version,
	}
	if !configured.UpdatedAt.IsZero() {
		out["updated_at"] = configured.UpdatedAt
	}
	return out
}

func (h ConfigureAiProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfigureAiProviderName,
		Summary: "Points this workspace at the AI provider its suggestions come from, or switches " +
			"AI off: the adapter kind, the endpoint, the models, the jurisdiction the provider " +
			"operates in, the key, and whether this workspace's content may be sent at all. A " +
			"provider outside the EEA is refused unless the operator of this installation has " +
			"confirmed third-country transfers.",
		SideEffects: "Writes the configuration, seals the API key, and writes an audit entry.",
		TokenScope:  aiManage,
		Input: []usecase.Field{
			{Name: "kind", Kind: usecase.KindString, Required: true,
				Enum:        enumOf(domain.AiProviderKinds()),
				Description: "Which adapter answers. NOOP calls nothing and is how AI is switched off."},
			{Name: "base_url", Kind: usecase.KindString,
				Description: "The endpoint the adapter calls. Required for every kind but NOOP."},
			{Name: "completion_model", Kind: usecase.KindString,
				Description: "The model a suggestion is asked of. Empty means this provider does not complete."},
			{Name: "embedding_model", Kind: usecase.KindString,
				Description: "The model a vector is asked of. Empty means this provider does not embed."},
			{Name: "api_key", Kind: usecase.KindString,
				Description: "Sealed on the way in and answered by nothing. Omit to keep the stored key; send empty to clear it."},
			{Name: "jurisdiction", Kind: usecase.KindString, Required: true,
				Enum:        enumOf(domain.AiJurisdictions()),
				Description: "Where the provider processes what is sent to it, as the operator declares it."},
			{Name: "processing_allowed", Kind: usecase.KindBool,
				Description: "Whether this workspace's content may be sent at all. False unless it is said."},
		},
		Audit: usecase.AuditDeclaration{
			Action: AiProviderConfiguredAction, TargetType: aiProviderTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A workspace's AI configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ConfigureAiProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd := ConfigureAiProviderCommand{
		Kind:            domain.AiProviderKind(in.String("kind")),
		BaseURL:         in.String("base_url"),
		CompletionModel: in.String("completion_model"),
		EmbeddingModel:  in.String("embedding_model"),
		Jurisdiction:    domain.AiJurisdiction(in.String("jurisdiction")),
	}
	if in.Present("processing_allowed") {
		cmd.ProcessingAllowed = in.Bool("processing_allowed")
	}
	// Present-but-empty clears the key and absent keeps it, which is the one place in this input
	// where the difference between the two is the whole meaning (Input.Present, C-07).
	if in.Present("api_key") {
		cmd.APIKeyPresent = true
		cmd.APIKey = secret.New(in.String("api_key"))
	}

	configured, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return aiProviderOutput(configured), nil
}

func (h ReadAiProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadAiProviderName,
		Summary: "How this workspace uses AI, if it does: the adapter kind, the endpoint, the " +
			"models, the jurisdiction, and whether processing is allowed here. The API key is " +
			"not among the fields, and there is no call that answers it.",
		SideEffects: "None. Reads only.",
		TokenScope:  aiManage,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: AiProviderReadAction, TargetType: aiProviderTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadAiProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	configured, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return aiProviderOutput(configured), nil
}

func (h RemoveAiProvider) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemoveAiProviderName,
		Summary: "Removes the workspace's AI provider and its sealed key. The workspace falls " +
			"back to the installation's default, which calls nothing. No suggestion anybody " +
			"already accepted is undone: accepting one was that person's own write.",
		SideEffects: "Deletes the configuration and writes an audit entry.",
		TokenScope:  aiManage,
		Destructive: true,
		Audit: usecase.AuditDeclaration{
			Action: AiProviderRemovedAction, TargetType: aiProviderTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A workspace's AI configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RemoveAiProvider) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	if err := h.Execute(ctx, actor); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// enumOf renders a closed set of typed values as the strings a descriptor declares, so the
// contract's enum and the descriptor's come from the same list rather than from two copies of it
// (the lesson of a declared enum value the descriptor refused first).
func enumOf[T ~string](values []T) []string {
	declared := make([]string, 0, len(values))
	for _, value := range values {
		declared = append(declared, string(value))
	}
	return declared
}
