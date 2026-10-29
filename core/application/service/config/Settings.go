// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"strings"

	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const (
	updateWorkspace   = "UpdateWorkspace"
	workspaceSource   = "WORKSPACE"
	signInPrefix      = "sign_in."
	legalPrefix       = "legal."
	localisationKey   = "localisation."
	aiPrefix          = "ai."
	aiSourcesKey      = "ai.sources"
	aiPersonOptOutKey = "ai.person_opt_out"
	basisAttribute    = "basis"
	contactAttribute  = "contact"
)

// settingArea is one area of catalogue settings the `settings` section holds, read through the
// use case its settings screen reads and written through the one that screen writes, so a lock
// or a loosening the screen refuses is refused to a plan alike (P-18). Only a value the workspace
// decided itself is a block; one left to the installation, the plan or Hubtask's default is not.
type settingArea struct {
	read     string
	prefixes []string
	// rows reads the read's answer as one row per catalogue key: `value`, `source`, `locked_by`
	// and whatever its write needs besides.
	rows func(answer map[string]any) map[string]map[string]any
	// write is the operation changing or resetting one setting.
	write func(change domain.Change, row map[string]any) (operation, error)
}

func settingAreas() []settingArea {
	return []settingArea{
		{read: ReadWorkspaceName, prefixes: []string{signInPrefix, legalPrefix, localisationKey}, rows: workspaceSettingRows, write: workspaceSettingOp},
		{read: "ReadAiSource", prefixes: []string{aiPrefix}, rows: aiSettingRows, write: aiSettingOp},
	}
}

// ReadWorkspaceName is the workspace's read, shared by the structure section and the settings.
const ReadWorkspaceName = "ReadWorkspace"

func (a settingArea) holds(key string) bool {
	for _, prefix := range a.prefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// areaOf is the area a settings key belongs to.
func areaOf(key string) (settingArea, bool) {
	for _, area := range settingAreas() {
		if area.holds(key) {
			return area, true
		}
	}
	return settingArea{}, false
}

// signInPaths is where ReadWorkspace's `sign_in_policy` answers each switch the workspace may
// decide; `rotation_from` is a moment, not configuration, and stays out of the document.
var signInPaths = map[identity.PolicySwitch][]string{
	identity.SwitchMfaRequiredFor:     {"mfa_required_for"},
	identity.SwitchMethods:            {"methods"},
	identity.SwitchSessionMaxDays:     {"session", "max_days"},
	identity.SwitchSessionIdleMinutes: {"session", "idle_minutes"},
	identity.SwitchManagedAccounts:    {"managed_accounts"},
	identity.SwitchPrivateHubs:        {"private_hubs"},
}

func signInPath(name identity.PolicySwitch) ([]string, bool) {
	if name == identity.SwitchRotationFrom {
		return nil, false
	}
	if path, held := signInPaths[name]; held {
		return path, true
	}
	return []string{"password", string(name)}, true
}

func dig(m map[string]any, path ...string) map[string]any {
	for _, step := range path {
		next, _ := m[step].(map[string]any)
		if next == nil {
			return nil
		}
		m = next
	}
	return m
}

// workspaceSettingRows reads the sign-in rule, the legal links and the localisation from the
// workspace's answer.
func workspaceSettingRows(answer map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	policy, _ := answer["sign_in_policy"].(map[string]any)
	if policy != nil {
		for _, name := range identity.PolicySwitches() {
			path, held := signInPath(name)
			if !held {
				continue
			}
			if cell := dig(policy, path...); cell != nil {
				out[signInPrefix+string(name)] = map[string]any{
					settingValueField: cell["value"], "source": cell["source"], "locked_by": cell["lock"],
				}
			}
		}
		for _, link := range identity.LegalSettingNames() {
			if cell := dig(policy, "legal", string(link)); cell != nil {
				out[legalPrefix+string(link)] = map[string]any{
					settingValueField: cell["value"], "source": cell["source"], "locked_by": cell["lock"],
				}
			}
		}
	}
	installation, _ := answer["installation_defaults"].(map[string]any)
	for name, field := range localisationFields {
		row := map[string]any{"source": "DEFAULT", "locked_by": nil, settingValueField: installation[name]}
		if text(installation[name]) != "" {
			row["source"] = "INSTANCE"
		}
		if value := text(answer[field]); value != "" {
			row[settingValueField], row["source"] = value, workspaceSource
		}
		out[localisationKey+name] = row
	}
	return out
}

// localisationFields is the workspace's field for each localisation setting.
var localisationFields = map[string]string{
	"locale": "default_locale", "time_zone": "default_time_zone", "week_start": "default_week_start",
}

// workspaceSettingOp writes one workspace setting through UpdateWorkspace. A sign-in switch or a
// legal link is a change to the sign-in rule and carries the apply's fresh proof (security.md
// §9), which is why the plan merges every one of them into a single call.
func workspaceSettingOp(change domain.Change, _ map[string]any) (operation, error) {
	path := change.Path()
	value := change.After[settingValueField]
	switch {
	case strings.HasPrefix(change.Key, localisationKey):
		field, held := localisationFields[strings.TrimPrefix(change.Key, localisationKey)]
		if !held {
			return operation{}, refused("setting.unknown", path, nil)
		}
		if change.Op == domain.OpRemove {
			if field != "default_week_start" {
				return operation{}, refused("config.not_removable", path, nil)
			}
			value = ""
		}
		return operation{UseCase: updateWorkspace, Path: path, Input: usecase.Input{field: value}}, nil
	case strings.HasPrefix(change.Key, signInPrefix), strings.HasPrefix(change.Key, legalPrefix):
		name := change.Key[strings.Index(change.Key, ".")+1:]
		if !knownSignIn(change.Key) {
			return operation{}, refused("setting.unknown", path, nil)
		}
		if change.Op == domain.OpRemove {
			// The rule has no way back to the level above: a workspace's switch is tightened or
			// left, never handed back (identity/WorkspacePolicy.go).
			return operation{}, refused("config.not_removable", path, nil)
		}
		return operation{UseCase: updateWorkspace, Path: path, StepUp: true,
			Input: usecase.Input{"sign_in_policy": map[string]any{name: value}}}, nil
	}
	return operation{}, refused("setting.unknown", path, nil)
}

func knownSignIn(key string) bool {
	if name, held := strings.CutPrefix(key, signInPrefix); held {
		_, ok := signInPath(identity.PolicySwitch(name))
		return ok && contains(policySwitchNames(), name)
	}
	if name, held := strings.CutPrefix(key, legalPrefix); held {
		for _, link := range identity.LegalSettingNames() {
			if string(link) == name {
				return true
			}
		}
	}
	return false
}

func policySwitchNames() []string {
	names := make([]string, 0, len(identity.PolicySwitches()))
	for _, name := range identity.PolicySwitches() {
		names = append(names, string(name))
	}
	return names
}

// aiSettingRows reads the workspace's narrowing of AI sources and its position on a person's
// opt-out from the AI answer, with what ChooseAiSource needs to keep the source chosen.
func aiSettingRows(answer map[string]any) map[string]map[string]any {
	sources := map[string]any{
		settingValueField: answer["sources"], "source": answer["sources_origin"], "locked_by": answer["sources_lock"],
		"chosen": answer["source"], "offered_model_id": answer["offered_model_id"],
	}
	optOut := map[string]any{
		settingValueField: answer["person_opt_out"], "source": answer["person_opt_out_origin"],
		"locked_by": answer["person_opt_out_lock"],
		basisAttribute: answer["person_opt_out_basis"], contactAttribute: answer["person_opt_out_contact"],
	}
	return map[string]map[string]any{aiSourcesKey: sources, aiPersonOptOutKey: optOut}
}

// aiSettingOp narrows the sources - a removed block takes what the installation allows again - or
// sets the opt-out position with its legal basis.
func aiSettingOp(change domain.Change, row map[string]any) (operation, error) {
	path := change.Path()
	switch change.Key {
	case aiSourcesKey:
		chosen := text(row["chosen"])
		if chosen == "" {
			chosen = "NONE"
		}
		in := usecase.Input{"source": chosen, "sources": change.After[settingValueField]}
		if change.Op == domain.OpRemove {
			in["sources"] = "INHERIT"
		}
		if model := text(row["offered_model_id"]); model != "" {
			in["offered_model_id"] = model
		}
		return operation{UseCase: "ChooseAiSource", Path: path, Input: in}, nil
	case aiPersonOptOutKey:
		if change.Op == domain.OpRemove {
			return operation{}, refused("config.not_removable", path, nil)
		}
		in := usecase.Input{"person_opt_out": change.After[settingValueField]}
		for _, attribute := range []string{basisAttribute, contactAttribute} {
			if value := text(change.After[attribute]); value != "" {
				in[attribute] = value
			}
		}
		return operation{UseCase: "SetAiPersonOptOut", Path: path, Input: in}, nil
	}
	return operation{}, refused("setting.unknown", path, nil)
}

// settingAttributes is what a settings block may name besides its value.
func settingAttributes(key string) []string {
	if key == aiPersonOptOutKey {
		return []string{settingValueField, basisAttribute, contactAttribute}
	}
	return []string{settingValueField}
}

// settingBlock is a row as the document keeps it: only where the workspace decided the value.
func settingBlock(key string, row map[string]any) domain.Block {
	if text(row["source"]) != workspaceSource {
		return nil
	}
	block := domain.Block{}
	for _, attribute := range settingAttributes(key) {
		if value, held := row[attribute]; held && value != nil && value != "" {
			block[attribute] = value
		}
	}
	return block
}

// settingsOps is the operations of every settings change in the plan, the workspace's merged into
// one UpdateWorkspace call so that the sign-in rule's proof is asked for once.
func settingsOps(changes []domain.Change, rows map[string]map[string]any) ([]operation, []shared.FieldError) {
	var ops []operation
	var refusals []shared.FieldError
	merged := -1
	for _, change := range changes {
		for attribute := range change.After {
			if !contains(settingAttributes(change.Key), attribute) {
				refusals = append(refusals, domain.Refusal("config.attribute_unknown", change.Path()+"."+attribute, nil))
			}
		}
		if change.Key == settingKey(domain.AgentApplyKey) {
			op, refusal := agentApplyOp(change)
			if refusal != nil {
				refusals = append(refusals, *refusal)
				continue
			}
			ops = append(ops, op)
			continue
		}
		area, held := areaOf(change.Key)
		if !held {
			refusals = append(refusals, domain.Refusal("setting.unknown", change.Path(), nil))
			continue
		}
		op, err := area.write(change, rows[change.Key])
		if err != nil {
			refusals = append(refusals, fieldsOf(err, change.Path())...)
			continue
		}
		if op.UseCase != updateWorkspace {
			ops = append(ops, op)
			continue
		}
		if merged < 0 {
			merged = len(ops)
			op.Input = mergeInput(usecase.Input{}, op.Input)
			ops = append(ops, op)
			continue
		}
		ops[merged].Input = mergeInput(ops[merged].Input, op.Input)
		ops[merged].StepUp = ops[merged].StepUp || op.StepUp
		ops[merged].Path = sectionSettings
	}
	return ops, refusals
}

// mergeInput adds one UpdateWorkspace input to another, the sign-in rule's flat patch merged.
func mergeInput(into, from usecase.Input) usecase.Input {
	for name, value := range from {
		patch, isPatch := value.(map[string]any)
		if !isPatch {
			into[name] = value
			continue
		}
		existing, _ := into[name].(map[string]any)
		if existing == nil {
			existing = map[string]any{}
		}
		for k, v := range patch {
			existing[k] = v
		}
		into[name] = existing
	}
	return into
}

func agentApplyOp(change domain.Change) (operation, *shared.FieldError) {
	if change.Op != domain.OpChange {
		refusal := domain.Refusal("config.not_removable", change.Path(), nil)
		return operation{}, &refusal
	}
	return operation{UseCase: SetConfigPolicyName, Path: change.Path(), Input: usecase.Input{
		"agent_apply": change.After[settingValueField],
	}}, nil
}

// definitionOf is a setting's declaration in the catalogue.
func definitionOf(key string) (setting.Definition, bool) {
	catalogue, err := settingservice.Catalogue()
	if err != nil {
		return setting.Definition{}, false
	}
	for _, definition := range catalogue.Definitions() {
		if string(definition.Key) == key {
			return definition, true
		}
	}
	return setting.Definition{}, false
}
