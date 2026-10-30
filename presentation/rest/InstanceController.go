// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The level above the workspaces, as the control plane reaches it (ADR-0070 §2, §5).
//
// One API and three clients: `hubctl admin`, the instance dashboard, and the file an operator
// checks into a repository. The controller holds no rules - whose credential may reach any of this
// is decided inwards of here, behind the scope and the register both.

// operatorResponse maps one operator of the instance.
func operatorResponse(row usecase.Output) openapi.Operator {
	operator := openapi.Operator{
		TenantId:  uuidValue(row.String("tenant_id")),
		AccountId: uuidValue(row.String("account_id")),
		AddedAt:   timeValue(row["added_at"]),
	}
	if by := row.String("added_by"); by != "" {
		id := uuidValue(by)
		operator.AddedBy = &id
	}
	operator.DisplayName = optionalStringOutput(row, "display_name")
	if email := row.String("email"); email != "" {
		address := openapi_types.Email(email)
		operator.Email = &address
	}
	operator.SignInName = optionalStringOutput(row, "sign_in_name")
	if kind := row.String("kind"); kind != "" {
		typed := openapi.OperatorKind(kind)
		operator.Kind = &typed
	}
	operator.WorkspaceSlug = optionalStringOutput(row, "workspace_slug")
	operator.WorkspaceName = optionalStringOutput(row, "workspace_name")
	operator.AddedByName = optionalStringOutput(row, "added_by_name")
	return operator
}

func instanceSettingsResponse(out usecase.Output) openapi.InstanceSettings {
	answer := openapi.InstanceSettings{}
	if settings := instanceSettingsOf(out["sign_in"]); settings != nil {
		answer.SignIn = &settings
	}
	if legal := instanceSettingsOf(out["legal"]); legal != nil {
		answer.Legal = &legal
	}
	if localisation := instanceSettingsOf(out["localisation"]); localisation != nil {
		answer.Localisation = &localisation
	}
	if quotas := instanceSettingsOf(out["quotas"]); quotas != nil {
		answer.Quotas = &quotas
	}
	if ai := instanceSettingsOf(out["ai"]); ai != nil {
		answer.Ai = &ai
	}
	if file := out.String("blocklist_file"); file != "" {
		answer.BlocklistFile = &file
	}
	if metering, held := out["metering"].(bool); held {
		answer.Metering = &metering
	}
	if listed, held := out["reserved_short_names"].([]any); held {
		reserved := make([]string, 0, len(listed))
		for _, name := range listed {
			if text, isText := name.(string); isText {
				reserved = append(reserved, text)
			}
		}
		answer.ReservedShortNames = &reserved
	}
	if source := out.String("source"); source != "" {
		answer.Source = &source
	}
	if enforced, held := out["is_enforced_from_file"].(bool); held {
		answer.IsEnforcedFromFile = &enforced
	}
	if version := out.Int("version"); version > 0 {
		answer.Version = &version
	}
	return answer
}

func instanceSettingsOf(value any) map[string]openapi.InstanceSetting {
	document, isOutput := value.(usecase.Output)
	if !isOutput {
		return nil
	}
	settings := make(map[string]openapi.InstanceSetting, len(document))
	for name, raw := range document {
		entry, held := raw.(usecase.Output)
		if !held {
			continue
		}
		locked, _ := entry["locked"].(bool)
		set, _ := entry["set"].(bool)
		setting := openapi.InstanceSetting{Set: set, Value: entry["value"], Locked: locked}
		if kind := entry.String("kind"); kind != "" {
			declared := openapi.InstanceSettingKind(kind)
			setting.Kind = &declared
		}
		if options, held := entry["options"].([]any); held {
			setting.Options = &options
		}
		if minimum, held := entry["minimum"].(int); held {
			setting.Minimum = &minimum
		}
		if maximum, held := entry["maximum"].(int); held {
			setting.Maximum = &maximum
		}
		if position, held := entry["position"].(int); held {
			setting.Position = &position
		}
		if may := entry.String("workspace_may"); may != "" {
			declared := openapi.InstanceSettingWorkspaceMay(may)
			setting.WorkspaceMay = &declared
		}
		settings[name] = setting
	}
	return settings
}

// instanceJournalEntryResponse maps one entry of the installation's journal.
func instanceJournalEntryResponse(row usecase.Output) openapi.InstanceJournalEntry {
	entry := openapi.InstanceJournalEntry{
		Id:         uuidValue(row.String("id")),
		OccurredAt: timeValue(row["occurred_at"]),
		Action:     row.String("action"),
	}
	if tenantID := row.String("tenant_id"); tenantID != "" {
		id := uuidValue(tenantID)
		entry.TenantId = &id
	}
	if slug := row.String("tenant_slug"); slug != "" {
		entry.TenantSlug = &slug
	}
	if label := row.String("actor_label"); label != "" {
		entry.ActorLabel = &label
	}
	if details, held := row["details"].(map[string]any); held && len(details) > 0 {
		entry.Details = &details
	}
	return entry
}

// censusCount narrows one of the census's numbers. The catalogue is untyped by construction - it is
// one shape for three channels - and a count comes out of the database as an `int64`, which the
// contract calls an integer.
func censusCount(value any) int {
	counted, _ := value.(int64)
	return int(counted)
}
