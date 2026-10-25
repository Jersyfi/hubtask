// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"context"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/voice"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ShowVoiceSettingsName = "ShowVoiceSettings"
	SaveVoiceSettingsName = "SaveVoiceSettings"
	TryVoiceSentenceName  = "TryVoiceSentence"
	ShowAlexaModelName    = "ShowAlexaModel"

	automationManage = "automation:manage"
	itemsRead        = "items:read"
	settingsTarget   = "voice_settings"

	voiceSettingsReadAction  = audit.Action("voice.settings_read")
	voiceSettingsSavedAction = audit.Action("voice.settings_saved")
	voiceSentenceTriedAction = audit.Action("voice.sentence_tried")
)

// Authorizer is the permission question.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// SettingsWriter is what the settings use cases share.
type SettingsWriter struct {
	Settings   repository.Settings
	Assistant  Assistant
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// SkillIDs are the installation's own published skills (HUBTASK_ALEXA_SKILL_IDS): with one, a
	// workspace needs no skill of its own.
	SkillIDs []string
}

// ShowVoiceSettings answers the workspace's voice settings.
type ShowVoiceSettings struct{ Writer SettingsWriter }

// SaveVoiceSettings replaces them.
type SaveVoiceSettings struct{ Writer SettingsWriter }

// TryVoiceSentence says what a sentence would do, without doing it.
type TryVoiceSentence struct{ Writer SettingsWriter }

// ShowAlexaModel answers the interaction model to paste into the Alexa developer console.
type ShowAlexaModel struct{ Writer SettingsWriter }

func (w SettingsWriter) authorize(ctx context.Context, actor appshared.ActorContext, action audit.Action, scope string) error {
	return w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionAutomation, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: scope, TargetType: settingsTarget,
	})
}

func (h ShowVoiceSettings) invoke(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, voiceSettingsReadAction, automationManage); err != nil {
		return nil, err
	}
	var settings domain.Settings
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Settings.Find(ctx)
		settings = found
		return err
	})
	if err != nil {
		return nil, err
	}
	return settingsOutput(settings, w.SkillIDs), nil
}

func (h SaveVoiceSettings) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, voiceSettingsSavedAction, automationManage); err != nil {
		return nil, err
	}
	settings, err := settingsOf(in, w.IDs)
	if err != nil {
		return nil, err
	}
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	now := w.Clock.Now()
	if err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return w.Settings.Save(ctx, settings, actor.AccountID, now)
	}); err != nil {
		return nil, err
	}
	if w.Audit != nil {
		_ = w.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now, Action: voiceSettingsSavedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: settingsTarget, TargetID: actor.TenantID,
			Changes: audit.Changes(
				audit.Change{Field: "enabled", Classification: audit.Open, To: boolWord(settings.Enabled)},
				audit.Change{Field: "skill_id", Classification: audit.Open, To: settings.SkillID},
				audit.Change{Field: "invocation_name", Classification: audit.Open, To: settings.InvocationName},
			),
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	}
	return settingsOutput(settings, w.SkillIDs), nil
}

func (h TryVoiceSentence) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, voiceSentenceTriedAction, automationManage); err != nil {
		return nil, err
	}
	sentence := strings.TrimSpace(in.String("sentence"))
	if sentence == "" {
		return nil, shared.ErrValidation.WithDetail("voice.sentence_required").
			WithFields(shared.FieldError{Path: "/sentence", Code: "voice.sentence_required"})
	}
	meant, err := w.Assistant.Interpret(ctx, actor, sentence, domain.LanguageOf(in.String("locale")))
	if err != nil {
		return nil, err
	}
	things := make([]any, 0, len(meant.Things))
	for _, thing := range meant.Things {
		things = append(things, map[string]any{"title": thing.Title, "amount": thing.Amount})
	}
	out := usecase.Output{"things": things, "found": meant.Found, "default": meant.Default}
	if meant.Found {
		out["place"] = placeOutput(meant.Place)
	}
	ambiguous := make([]any, 0, len(meant.Ambiguous))
	for _, place := range meant.Ambiguous {
		ambiguous = append(ambiguous, placeOutput(place))
	}
	out["ambiguous"] = ambiguous
	return out, nil
}

func (h ShowAlexaModel) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor, voiceSettingsReadAction, automationManage); err != nil {
		return nil, err
	}
	var settings domain.Settings
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Settings.Find(ctx)
		settings = found
		return err
	})
	if err != nil {
		return nil, err
	}
	language := domain.LanguageOf(in.String("locale"))
	return usecase.Output{"model": AlexaModel(language, settings.Invocation(language))}, nil
}

func placeOutput(place domain.Place) map[string]any {
	out := map[string]any{"collection_id": place.CollectionID.String(), "name": place.Name}
	if !place.LabelID.IsZero() {
		out["label_id"] = place.LabelID.String()
	}
	return out
}

func settingsOutput(settings domain.Settings, installationSkills []string) usecase.Output {
	aliases := make([]any, 0, len(settings.Aliases))
	for _, alias := range settings.Aliases {
		row := map[string]any{"phrase": alias.Phrase, "collection_id": alias.CollectionID.String()}
		if !alias.LabelID.IsZero() {
			row["label_id"] = alias.LabelID.String()
		}
		aliases = append(aliases, row)
	}
	out := usecase.Output{
		"enabled": settings.Enabled, "aliases": aliases,
		"installation_skill": len(installationSkills) > 0,
	}
	if !settings.DefaultCollectionID.IsZero() {
		out["default_collection_id"] = settings.DefaultCollectionID.String()
	}
	if settings.SkillID != "" {
		out["skill_id"] = settings.SkillID
	}
	if settings.InvocationName != "" {
		out["invocation_name"] = settings.InvocationName
	}
	return out
}

func settingsOf(in usecase.Input, ids clock.IDGenerator) (domain.Settings, error) {
	settings := domain.Settings{
		Enabled: in.Bool("enabled"), SkillID: strings.TrimSpace(in.String("skill_id")),
		InvocationName: domain.NormalInvocation(in.String("invocation_name")),
	}
	var err error
	if settings.DefaultCollectionID, err = in.ID("default_collection_id"); err != nil {
		return domain.Settings{}, err
	}
	rows, _ := in["aliases"].([]any)
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		alias := domain.Alias{ID: ids.NewID()}
		alias.Phrase, _ = row["phrase"].(string)
		if text, _ := row["collection_id"].(string); text != "" {
			if alias.CollectionID, err = shared.ParseID(text); err != nil {
				return domain.Settings{}, shared.ErrValidation.WithDetail("voice.alias_place_required").
					WithFields(shared.FieldError{Path: "/aliases", Code: "voice.alias_place_required"})
			}
		}
		if text, _ := row["label_id"].(string); text != "" {
			if alias.LabelID, err = shared.ParseID(text); err != nil {
				return domain.Settings{}, shared.ErrValidation.WithDetail("voice.alias_invalid").
					WithFields(shared.FieldError{Path: "/aliases", Code: "voice.alias_invalid"})
			}
		}
		settings.Aliases = append(settings.Aliases, alias)
	}
	return settings, nil
}

func boolWord(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// Descriptor is the catalogue entry.
func (h ShowVoiceSettings) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ShowVoiceSettingsName,
		Summary:     "The workspace's voice settings: whether it takes spoken entries, the default list, its own skill and the names people may say for a place.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationManage,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{Action: voiceSettingsReadAction, TargetType: settingsTarget, Severity: audit.SeverityInfo},
		Handler:     usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h SaveVoiceSettings) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        SaveVoiceSettingsName,
		Summary:     "Replaces the workspace's voice settings and spoken names in one write.",
		SideEffects: "Stores the settings and writes an audit entry.",
		TokenScope:  automationManage,
		Input: []usecase.Field{
			{Name: "enabled", Kind: usecase.KindBool, Description: "Whether spoken entries are taken."},
			{Name: "default_collection_id", Kind: usecase.KindID, Description: "Where a thing said without a place goes."},
			{Name: "skill_id", Kind: usecase.KindString, Description: "The workspace's own Alexa skill, amzn1.ask.skill.…; empty for the installation's."},
			{Name: "invocation_name", Kind: usecase.KindString, Description: "What follows \"Alexa, sag\": two plain words; empty for the language's default, \"listen helfer\" or \"list helper\"."},
			{Name: "aliases", Kind: usecase.KindList, Description: "The names people may say: objects of phrase, collection_id and, for a category, label_id."},
		},
		Audit: usecase.AuditDeclaration{
			Action: voiceSettingsSavedAction, TargetType: settingsTarget, Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "Settings of the workspace, not of an entry."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h TryVoiceSentence) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        TryVoiceSentenceName,
		Summary:     "Reads a sentence as a voice assistant would hand it over and answers what it would add where - without adding anything.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationManage,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "sentence", Kind: usecase.KindString, Required: true, Description: "What would be said, e.g. \"Butter auf die Einkaufsliste\"."},
			{Name: "locale", Kind: usecase.KindString, Description: "de-DE or en-US; English where absent."},
		},
		Audit:   usecase.AuditDeclaration{Action: voiceSentenceTriedAction, TargetType: settingsTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h ShowAlexaModel) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ShowAlexaModelName,
		Summary:     "The Alexa interaction model to paste into the Alexa developer console. The same for every workspace: its lists reach Alexa with every answer, so new ones need no new model.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationManage,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "locale", Kind: usecase.KindString, Description: "de-DE or en-US."},
		},
		Audit:   usecase.AuditDeclaration{Action: voiceSettingsReadAction, TargetType: settingsTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}
