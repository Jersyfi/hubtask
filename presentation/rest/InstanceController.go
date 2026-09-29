// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The level above the workspaces, as the control plane reaches it (ADR-0070 §2, §5).
//
// One API and three clients: `hubctl admin`, the instance dashboard, and the file an operator
// checks into a repository. The controller holds no rules - whose credential may reach any of this
// is decided inwards of here, behind the scope and the register both.

const (
	readInstanceSettingsUseCase  = "ReadInstanceSettings"
	writeInstanceSettingsUseCase = "WriteInstanceSettings"
	readInstanceOverviewUseCase  = "ReadInstanceOverview"
	listInstanceJournalUseCase   = "ListInstanceJournal"
	listOperatorsUseCase         = "ListOperators"
	addOperatorUseCase           = "AddOperator"
	removeOperatorUseCase        = "RemoveOperator"
)

// The four operations below are written out rather than taken through the identity helper, for the
// reason `ListServiceAccounts` is: the helper's closure takes no context, and an operation with no
// parameters gives the linter nothing to trace the request's context through.

// ReadInstanceSettings answers GET /admin/settings.
func (c *RestController) ReadInstanceSettings(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(
		r.Context(), readInstanceSettingsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, instanceSettingsResponse(out))
}

// WriteInstanceSettings answers PUT /admin/settings.
func (c *RestController) WriteInstanceSettings(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.InstanceSettings
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{}
	if body.SignIn != nil {
		in["sign_in"] = instanceSettingMap(*body.SignIn)
	}
	if body.Legal != nil {
		in["legal"] = instanceSettingMap(*body.Legal)
	}
	if body.Localisation != nil {
		in["localisation"] = instanceSettingMap(*body.Localisation)
	}
	if body.Quotas != nil {
		in["quotas"] = instanceSettingMap(*body.Quotas)
	}
	if body.BlocklistFile != nil {
		in["blocklist_file"] = *body.BlocklistFile
	}

	out, err := c.UseCases.Invoke(r.Context(), writeInstanceSettingsUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, instanceSettingsResponse(out))
}

// ListOperators answers GET /admin/operators.
func (c *RestController) ListOperators(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), listOperatorsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["data"].([]usecase.Output)
	operators := make([]openapi.Operator, 0, len(rows))
	for _, row := range rows {
		operator := openapi.Operator{
			TenantId:  uuidValue(row.String("tenant_id")),
			AccountId: uuidValue(row.String("account_id")),
			AddedAt:   timeValue(row["added_at"]),
		}
		if by := row.String("added_by"); by != "" {
			id := uuidValue(by)
			operator.AddedBy = &id
		}
		operators = append(operators, operator)
	}
	writeJSON(w, r, http.StatusOK, operators)
}

// AddOperator answers POST /admin/operators.
func (c *RestController) AddOperator(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.OperatorAdd
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	// Either form, and the registry refuses a request that carries neither: the workspace and the
	// address for a screen, the identifier for a script that already has one.
	in := usecase.Input{}
	if body.AccountId != nil {
		in["account_id"] = body.AccountId.String()
	}
	if body.Workspace != nil {
		in["workspace"] = *body.Workspace
	}
	if body.Email != nil {
		in["email"] = string(*body.Email)
	}

	if _, err := c.UseCases.Invoke(r.Context(), addOperatorUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveOperator answers DELETE /admin/operators/{accountId}.
func (c *RestController) RemoveOperator(
	w http.ResponseWriter, r *http.Request, accountID openapi.AccountId,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), removeOperatorUseCase, actor, usecase.Input{
			"account_id": accountID.String(),
		})
	}, func(usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// instanceSettingMap flattens the contract's `{value, locked}` documents into the catalogue's.
func instanceSettingMap(sent map[string]openapi.InstanceSetting) map[string]any {
	settings := make(map[string]any, len(sent))
	for name, setting := range sent {
		// A switch a client sent back without a value is one it is clearing, which is what the
		// write means by "the installation stops deciding it" - so `set` travels no further than
		// the read that carried it.
		if setting.Value == nil {
			continue
		}
		settings[name] = map[string]any{"value": setting.Value, "locked": setting.Locked}
	}
	return settings
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
	if file := out.String("blocklist_file"); file != "" {
		answer.BlocklistFile = &file
	}
	if source := out.String("source"); source != "" {
		answer.Source = &source
	}
	if enforced, held := out["is_enforced_from_file"].(bool); held {
		answer.IsEnforcedFromFile = &enforced
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
		settings[name] = openapi.InstanceSetting{Set: set, Value: entry["value"], Locked: locked}
	}
	return settings
}

const elevateSessionUseCase = "ElevateSession"

// ElevateSession answers POST /auth/sessions:elevate.
func (c *RestController) ElevateSession(
	w http.ResponseWriter, r *http.Request, params openapi.ElevateSessionParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), elevateSessionUseCase, actor, usecase.Input{
			"step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
		})
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, openapi.SessionElevation{
			ElevatedUntil:    timeValue(out["elevated_until"]),
			RemainingSeconds: out.Int("remaining_seconds"),
		})
	})
}

// ReadInstanceOverview answers GET /admin/overview.
func (c *RestController) ReadInstanceOverview(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(
		r.Context(), readInstanceOverviewUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, openapi.InstanceOverview{
		WorkspacesActive:          censusCount(out["workspaces_active"]),
		WorkspacesSuspended:       censusCount(out["workspaces_suspended"]),
		WorkspacesPendingDeletion: censusCount(out["workspaces_pending_deletion"]),
		AccountsActive:            censusCount(out["accounts_active"]),
		AccountsTotal:             censusCount(out["accounts_total"]),
	})
}

// ListInstanceJournal answers GET /admin/journal.
func (c *RestController) ListInstanceJournal(
	w http.ResponseWriter, r *http.Request, params openapi.ListInstanceJournalParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{}
	if params.Cursor != nil {
		in["cursor"] = *params.Cursor
	}
	if params.Size != nil {
		in["limit"] = *params.Size
	}

	out, err := c.UseCases.Invoke(r.Context(), listInstanceJournalUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["data"].([]usecase.Output)
	entries := make([]openapi.InstanceJournalEntry, 0, len(rows))
	for _, row := range rows {
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
		entries = append(entries, entry)
	}
	writeJSON(w, r, http.StatusOK, openapi.InstanceJournalPage{
		Data: entries,
		Page: pageResponse(out),
	})
}

// censusCount narrows one of the census's numbers. The catalogue is untyped by construction - it is
// one shape for three channels - and a count comes out of the database as an `int64`, which the
// contract calls an integer.
func censusCount(value any) int {
	counted, _ := value.(int64)
	return int(counted)
}
