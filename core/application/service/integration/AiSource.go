// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	privacyrepo "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	privacydomain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// Which AI source answers a workspace (ADR-0072 §2, §3): its own model, a model the installation
// offers, or none - within what the installation allows.
//
// **The choice is the consent to an offered model.** Choosing one by name, with its processor and
// jurisdiction on the screen, is the one control UC-AI-05 asks for; choosing the workspace's own
// model or none withdraws it. The own model's consent stays on its own configuration, untouched.
// No installation or plan value writes either.

const (
	ReadAiSourceName   = "ReadAiSource"
	ChooseAiSourceName = "ChooseAiSource"

	aiSourceTarget = "ai_source"

	// inheritSources clears the workspace's own narrowing: it takes what the installation allows.
	inheritSources = "INHERIT"
)

const (
	AiSourceChosenAction audit.Action = "ai.source_chosen"
	AiSourceReadAction   audit.Action = "ai.source_read"
)

// AiRules reads what decides a workspace's AI: the installation's level, the workspace's own
// narrowing, the models offered, and the workspace's choice. Read inside the caller's transaction,
// in the workspace's scope - the instance level has no row level security and the offered rows are
// admitted by the read policy - so one read answers the whole state at one moment.
type AiRules struct {
	Instance   identityrepo.InstanceSettings
	Workspaces identityrepo.Workspaces
	Offered    repository.OfferedAiProviders
	Choices    repository.AiChoices
	// Own is the workspace's own configuration, read by Select where the choice is the own model.
	Own repository.AiProviders
	// Consents counts who keeps their content out, for the number the screen shows before the
	// workspace changes its position (P-04). Nil counts nobody.
	Consents privacyrepo.Consents
}

// AiState is a workspace's AI at one moment.
type AiState struct {
	Rule    domain.AiRule
	Choice  domain.AiChoice
	Offered []domain.AiProvider
	// Narrowing is the workspace's own narrowing, empty where it narrowed nothing.
	Narrowing domain.AiSources
	// SearchRebuilding is true while vectors of another model than the one now in use are still
	// stored: search by meaning is being rebuilt, and search by words works meanwhile.
	SearchRebuilding bool
	// OptOutContact and OptOutSince belong to the workspace's NOT_OFFERED: whom to ask, and
	// since when - empty and zero under OFFERED.
	OptOutContact string
	OptOutSince   time.Time
	// KeepingOut is how many people's newest ai_processing record is a withdrawal: a number, never
	// who (UC-PRV-05 check 12).
	KeepingOut int
}

// ChosenOffered answers the offered model the choice names, when it still exists.
func (s AiState) ChosenOffered() (domain.AiProvider, bool) {
	for _, offered := range s.Offered {
		if offered.ID == s.Choice.OfferedProviderID {
			return offered, true
		}
	}
	return domain.AiProvider{}, false
}

// Resolve reads the state inside the caller's transaction.
func (r AiRules) Resolve(ctx context.Context, tenantID shared.ID) (AiState, error) {
	level, err := r.Instance.Read(ctx)
	if err != nil {
		return AiState{}, err
	}
	workspace, err := r.Workspaces.Find(ctx)
	if err != nil {
		return AiState{}, err
	}
	offered, err := r.Offered.List(ctx)
	if err != nil {
		return AiState{}, err
	}
	choice, err := r.Choices.Find(ctx)
	if err != nil {
		return AiState{}, err
	}
	choice.TenantID = tenantID

	narrowing := domain.AiSources(workspace.Settings.AiSources)
	workspaceLayer := domain.AiLayer{}
	if narrowing.Valid() {
		workspaceLayer.Sources = &narrowing
	}
	if position := domain.AiPersonOptOut(workspace.Settings.AiPersonOptOut); position.Valid() {
		workspaceLayer.PersonOptOut = &position
		workspaceLayer.Basis = domain.AiOptOutBasis(workspace.Settings.AiOptOutBasis)
	}
	state := AiState{
		Rule:      domain.EffectiveAi(level.Ai, domain.AiLayer{}, workspaceLayer, len(offered) > 0),
		Choice:    choice,
		Offered:   offered,
		Narrowing: narrowing,
	}
	if !state.Rule.OptOutOffered() {
		state.OptOutContact = workspace.Settings.AiOptOutContact
		state.OptOutSince = workspace.Settings.AiOptOutSince
	}
	return state, nil
}

// keepingOut counts the people whose newest ai_processing record is a withdrawal. Only the two
// reads that show it ask, not every AI call that resolves the rule.
func (r AiRules) keepingOut(ctx context.Context) (int, error) {
	if r.Consents == nil {
		return 0, nil
	}
	standing, err := r.Consents.Standing(ctx, privacydomain.PurposeAi)
	return len(standing), err
}

// AiSourceWriter is what the two use cases share.
type AiSourceWriter struct {
	Rules      AiRules
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Reindex seeds the re-embedding a change of the model behind search needs. Nil where the
	// installation has no semantic search to rebuild.
	Reindex EmbeddingReseeder
	// Spaces answers whether search by meaning is being rebuilt. Nil answers never.
	Spaces workrepo.EmbeddingSpaces
}

// EmbeddingReseeder queues a re-embedding pass in the transaction's workspace.
type EmbeddingReseeder interface {
	Reseed(ctx context.Context, tenantID shared.ID) error
}

// EmbeddingSeeder seeds the embedding pass in the transaction's workspace: the write that changed
// the model behind search seeds it, and nothing visits workspaces (ADR-0072 §6). The same job and
// the same deduplication key the entries' own writes use, so a burst of changes is one pass.
type EmbeddingSeeder struct {
	Jobs  EmbeddingJobs
	Delay time.Duration
	Clock clock.Clock
}

// EmbeddingJobs is the queue's one method this needs.
type EmbeddingJobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Reseed queues the pass.
func (s EmbeddingSeeder) Reseed(ctx context.Context, tenantID shared.ID) error {
	_, err := s.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindAiEmbed, TenantID: tenantID,
		DedupeKey: string(queue.KindAiEmbed) + ":" + tenantID.String(),
		RunAt:     s.Clock.Now().Add(s.Delay),
	})
	return err
}

// rebuilding asks, inside the caller's transaction, whether the model in use still has entries
// embedded under another one.
func (w AiSourceWriter) rebuilding(ctx context.Context, tenantID shared.ID) (bool, error) {
	if w.Spaces == nil || w.Rules.Own == nil {
		return false, nil
	}
	selection, err := w.Rules.Select(ctx, tenantID)
	if err != nil || selection.Refusal != "" || selection.EmbeddingSpace == "" {
		return false, err
	}
	return w.Spaces.OtherSpaceRemains(ctx, selection.EmbeddingSpace)
}

// ReadAiSource answers the workspace's AI: what it may use, what it chose, and what is offered.
type ReadAiSource struct{ Writer AiSourceWriter }

// Execute reads it.
func (h ReadAiSource) Execute(ctx context.Context, actor appshared.ActorContext) (AiState, error) {
	w := h.Writer
	request := aiSourceRequest(AiSourceReadAction)
	request.Alternative = service.PermissionReadConfiguration
	if err := w.Authorizer.Authorize(ctx, actor, request); err != nil {
		return AiState{}, err
	}
	var state AiState
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if state, err = w.Rules.Resolve(ctx, actor.TenantID); err != nil {
			return err
		}
		if state.KeepingOut, err = w.Rules.keepingOut(ctx); err != nil {
			return err
		}
		state.SearchRebuilding, err = w.rebuilding(ctx, actor.TenantID)
		return err
	})
	return state, err
}

// ChooseAiSource sets which source answers the workspace, and its own narrowing.
type ChooseAiSource struct{ Writer AiSourceWriter }

// ChooseAiSourceCommand is the input, typed. Sources is the narrowing: empty keeps it, INHERIT
// clears it, a value narrows to it.
type ChooseAiSourceCommand struct {
	Source            domain.AiSourceKind
	OfferedProviderID shared.ID
	Sources           string
}

// Execute narrows first, then checks the choice against the rule that results: a workspace that
// narrows itself to its own model and chooses the offered one in the same request is refused, not
// half-written.
func (h ChooseAiSource) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChooseAiSourceCommand,
) (AiState, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, aiSourceRequest(AiSourceChosenAction)); err != nil {
		return AiState{}, err
	}
	now := w.Clock.Now()
	choice, err := domain.NewAiChoice(actor.TenantID, cmd.Source, cmd.OfferedProviderID, now)
	if err != nil {
		return AiState{}, err
	}

	var state AiState
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := w.Rules.Resolve(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		if err := h.narrow(ctx, before, cmd.Sources); err != nil {
			return err
		}
		after, err := w.Rules.Resolve(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		if !after.Rule.Permits(choice.Source) {
			return domain.SourceNotAllowed()
		}
		if choice.Source == domain.AiSourceOffered {
			if _, err := w.Rules.Offered.Find(ctx, choice.OfferedProviderID); err != nil {
				if errors.Is(err, shared.ErrNotFound) {
					return shared.ErrValidation.WithDetail("ai.offered_model_unknown").
						WithFields(shared.FieldError{Path: "/offered_model_id", Code: "ai.offered_model_unknown"})
				}
				return err
			}
		}
		if _, err := w.Rules.Choices.Save(ctx, choice); err != nil {
			return err
		}
		state, err = w.Rules.Resolve(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		if err := h.reseedIfMoved(ctx, actor.TenantID, before, state); err != nil {
			return err
		}
		return h.record(ctx, actor, before, state)
	})
	if err != nil {
		return AiState{}, err
	}
	return state, nil
}

// narrow writes the workspace's own narrowing, checked against what the levels above allow and
// their lock (UC-AI-06 check 2).
func (h ChooseAiSource) narrow(ctx context.Context, before AiState, sent string) error {
	if sent == "" {
		return nil
	}
	next := ""
	if sent != inheritSources {
		if err := before.Rule.Narrowed(domain.AiSources(sent)); err != nil {
			return err
		}
		next = sent
	}
	if next == string(before.Narrowing) {
		return nil
	}
	workspace, err := h.Writer.Rules.Workspaces.Find(ctx)
	if err != nil {
		return err
	}
	workspace.Settings.AiSources = next
	written, err := h.Writer.Rules.Workspaces.Update(ctx, workspace, workspace.Version, h.Writer.Clock.Now())
	if err != nil {
		return err
	}
	if !written {
		return shared.ErrConflict.WithDetail("workspace.version_conflict")
	}
	return nil
}

// reseedIfMoved queues a re-embedding in this workspace when the model behind search changed: the
// write that changed it seeds it, and nothing visits workspaces (ADR-0072 §6).
func (h ChooseAiSource) reseedIfMoved(ctx context.Context, tenantID shared.ID, before, after AiState) error {
	if h.Writer.Reindex == nil || before.Choice.Source == after.Choice.Source &&
		before.Choice.OfferedProviderID == after.Choice.OfferedProviderID {
		return nil
	}
	return h.Writer.Reindex.Reseed(ctx, tenantID)
}

// record writes the trail entry: the source from and to and, for an offered one, its name and
// jurisdiction - the consent given or withdrawn (UC-AI-03 check 4).
func (h ChooseAiSource) record(ctx context.Context, actor appshared.ActorContext, before, after AiState) error {
	changes := []audit.Change{
		{Field: "source", Classification: audit.Open,
			From: string(before.Choice.Source), To: string(after.Choice.Source)},
	}
	if offered, held := after.ChosenOffered(); held && after.Choice.Source == domain.AiSourceOffered {
		changes = append(changes,
			audit.Change{Field: "offered_model", Classification: audit.Open, To: offered.DisplayName},
			audit.Change{Field: "jurisdiction", Classification: audit.Open, To: string(offered.Jurisdiction)})
	}
	if before.Narrowing != after.Narrowing {
		changes = append(changes, audit.Change{Field: "sources", Classification: audit.Open,
			From: string(before.Narrowing), To: string(after.Narrowing)})
	}
	return h.Writer.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: h.Writer.Clock.Now(),
		Action: AiSourceChosenAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: aiSourceTarget, TargetID: actor.TenantID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// aiSourceRequest is the configuration's question: STRUCTURE at the workspace, `ai:manage`.
func aiSourceRequest(action audit.Action) access.Request {
	request := aiRequest(action)
	request.TargetType = aiSourceTarget
	return request
}

// AiSourceOutput is the read shape. An offered model is answered by its name, its processor and
// its jurisdiction only - never its address, its models or its key (UC-AI-05 check 3) - and only
// where the levels above the workspace allow offered models at all.
func AiSourceOutput(state AiState) usecase.Output {
	rule := state.Rule
	out := usecase.Output{
		"source":                  string(state.Choice.Source),
		"sources":                 string(rule.Sources),
		"installation_sources":    string(rule.Above),
		"sources_origin":          string(rule.OriginOf(domain.AiSwitchSources)),
		"min_jurisdiction":        string(rule.MinJurisdiction),
		"min_jurisdiction_origin": string(rule.OriginOf(domain.AiSwitchMinJurisdiction)),
		"version":                 state.Choice.Version,
		"search_rebuilding":       state.SearchRebuilding,
	}
	if lock := rule.LockOf(domain.AiSwitchSources); lock != "" {
		out["sources_lock"] = string(lock)
	}
	if lock := rule.LockOf(domain.AiSwitchMinJurisdiction); lock != "" {
		out["min_jurisdiction_lock"] = string(lock)
	}
	if state.Narrowing != "" {
		out["narrowing"] = string(state.Narrowing)
	}
	if !state.Choice.OfferedProviderID.IsZero() {
		out["offered_model_id"] = state.Choice.OfferedProviderID.String()
	}
	offered := make([]usecase.Output, 0, len(state.Offered))
	if rule.Above.AllowsOffered() {
		for _, model := range state.Offered {
			offered = append(offered, usecase.Output{
				"id":           model.ID.String(),
				"display_name": model.DisplayName,
				"processor":    model.Processor,
				"jurisdiction": string(model.Jurisdiction),
			})
		}
	}
	out["offered"] = offered
	if !state.Choice.UpdatedAt.IsZero() {
		out["updated_at"] = state.Choice.UpdatedAt
	}
	out["person_opt_out"] = string(rule.PersonOptOut)
	out["person_opt_out_origin"] = string(rule.OriginOf(domain.AiSwitchPersonOptOut))
	if lock := rule.LockOf(domain.AiSwitchPersonOptOut); lock != "" {
		out["person_opt_out_lock"] = string(lock)
	}
	if rule.Basis != "" {
		out["person_opt_out_basis"] = string(rule.Basis)
	}
	if state.OptOutContact != "" {
		out["person_opt_out_contact"] = state.OptOutContact
	}
	if !state.OptOutSince.IsZero() {
		out["person_opt_out_since"] = state.OptOutSince
	}
	out["keeping_out_count"] = state.KeepingOut
	return out
}

func (h ReadAiSource) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadAiSourceName,
		Summary: "Which AI answers this workspace: its own model, a model the installation offers, " +
			"or none - and what the installation allows, with who set it. An offered model is " +
			"named with its processor and where it processes content, and nothing more.",
		SideEffects: "None. Reads only.",
		TokenScope:  aiManage,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: AiSourceReadAction, TargetType: aiSourceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadAiSource) invoke(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
	state, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return AiSourceOutput(state), nil
}

func (h ChooseAiSource) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ChooseAiSourceName,
		Summary: "Chooses which AI answers this workspace. Choosing a model the installation " +
			"offers is this workspace's consent to send its content to it; choosing its own model " +
			"or none withdraws that consent. No AI is always allowed; the others only where the " +
			"installation allows them. Optionally narrows what this workspace allows itself.",
		SideEffects: "Writes the choice, and the narrowing where sent, and an audit entry; queues " +
			"rebuilding search by meaning when the model behind it changed.",
		TokenScope: aiManage,
		Input: []usecase.Field{
			{Name: "source", Kind: usecase.KindString, Required: true,
				Enum:        enumOf(domain.AiSourceKinds()),
				Description: "OWN, OFFERED or NONE."},
			{Name: "offered_model_id", Kind: usecase.KindString,
				Description: "The offered model, by its identifier. Only with OFFERED."},
			{Name: "sources", Kind: usecase.KindString,
				Enum:        append(enumOf(domain.AiSourcesValues()), inheritSources),
				Description: "This workspace's own narrowing of what it may use: a value narrows, INHERIT takes what the installation allows, absent keeps it."},
		},
		Audit: usecase.AuditDeclaration{
			Action: AiSourceChosenAction, TargetType: aiSourceTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A workspace's AI configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ChooseAiSource) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	cmd := ChooseAiSourceCommand{
		Source:  domain.AiSourceKind(in.String("source")),
		Sources: in.String("sources"),
	}
	if raw := in.String("offered_model_id"); raw != "" {
		id, err := in.ID("offered_model_id")
		if err != nil {
			return nil, err
		}
		cmd.OfferedProviderID = id
	}
	state, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return AiSourceOutput(state), nil
}
