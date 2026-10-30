// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/agent"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// AgentRepository stores the governed agents module (migration 0251, M9). A held act's input and
// a case's expectation are `jsonb`, written as the use case handed them.
type AgentRepository struct{}

var _ repository.Store = AgentRepository{}

// NewAgentRepository answers the adapter.
func NewAgentRepository() AgentRepository { return AgentRepository{} }

func agentError(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return shared.ErrNotFound.WithDetail("agents.not_found")
	}
	var typed *shared.Error
	if errors.As(err, &typed) {
		return err
	}
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

func modelOf(r sqlc.FindAgentModelRow) (domain.Model, error) {
	keys, err := ids(r.ID, r.OfferedID, r.ConsentBy)
	if err != nil {
		return domain.Model{}, err
	}
	var days *int
	if r.RetentionDays != nil {
		d := int(*r.RetentionDays)
		days = &d
	}
	return domain.Model{
		ID: keys[0], Key: r.Key, Name: r.Name, Source: domain.Source(r.Source), OfferedID: keys[1],
		Model: r.Model, Region: domain.Region(r.Region), RetentionDays: days, ConsentBy: keys[2],
		ConsentAt: timeFrom(r.ConsentAt), Revision: int(r.Revision), Retired: r.Retired,
		UpdatedAt: timeFrom(r.UpdatedAt), Version: int(r.Version),
	}, nil
}

func (AgentRepository) ListModels(ctx context.Context, max int) ([]domain.Model, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAgentModels(ctx, int32(max))
	if err != nil {
		return nil, agentError(err, "listing agent models")
	}
	out := make([]domain.Model, 0, len(rows))
	for _, r := range rows {
		m, err := modelOf(sqlc.FindAgentModelRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (AgentRepository) FindModel(ctx context.Context, id shared.ID) (domain.Model, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Model{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Model{}, err
	}
	row, err := queries.FindAgentModel(ctx, key)
	if err != nil {
		return domain.Model{}, agentError(err, "reading an agent model")
	}
	return modelOf(row)
}

func retention(days *int) *int32 {
	if days == nil {
		return nil
	}
	d := int32(*days)
	return &d
}

func (AgentRepository) InsertModel(ctx context.Context, m domain.Model, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(m.ID, m.OfferedID, m.ConsentBy)
	if err != nil {
		return err
	}
	err = queries.InsertAgentModel(ctx, sqlc.InsertAgentModelParams{
		ID: keys[0], Key: m.Key, Name: m.Name, Source: string(m.Source), OfferedID: keys[1], Model: m.Model,
		Region: string(m.Region), RetentionDays: retention(m.RetentionDays), ConsentBy: keys[2],
		ConsentAt: timestampOf(m.ConsentAt), At: timestampOf(at),
	})
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("agent_models.key_taken").
			WithFields(shared.FieldError{Path: "/key", Code: "agent_models.key_taken"})
	}
	return agentError(err, "writing an agent model")
}

func (AgentRepository) UpdateModel(ctx context.Context, m domain.Model, expected int, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(m.ID, m.OfferedID, m.ConsentBy)
	if err != nil {
		return err
	}
	n, err := queries.UpdateAgentModel(ctx, sqlc.UpdateAgentModelParams{
		ID: keys[0], Name: m.Name, Source: string(m.Source), OfferedID: keys[1], Model: m.Model,
		Region: string(m.Region), RetentionDays: retention(m.RetentionDays), ConsentBy: keys[2],
		ConsentAt: timestampOf(m.ConsentAt), Revision: int32(m.Revision), Retired: m.Retired,
		At: timestampOf(at), ExpectedVersion: int32(expected),
	})
	if err != nil {
		return agentError(err, "changing an agent model")
	}
	if n == 0 {
		return shared.ErrVersionConflict
	}
	return nil
}

func agentOf(r sqlc.FindAgentRow) (domain.Agent, error) {
	keys, err := ids(r.ID, r.AccountID, r.OwnerID, r.ModelID, r.ApprovedBy, r.SuspendedBy, r.CreatedBy)
	if err != nil {
		return domain.Agent{}, err
	}
	return domain.Agent{
		ID: keys[0], AccountID: keys[1], Name: r.Name, Purpose: r.Purpose, Kind: domain.Kind(r.Kind),
		OwnerID: keys[2], RiskClass: domain.RiskClass(r.RiskClass), Scopes: r.Scopes, ModelID: keys[3],
		Policy: domain.Policy{Reversible: domain.Rule(r.PolicyReversible), Irreversible: domain.Rule(r.PolicyIrreversible)},
		Budget: domain.Budget{
			TokensPerDay: r.TokensPerDay, ActionsPerHour: int(r.ActionsPerHour), ConcurrentRuns: int(r.ConcurrentRuns),
		},
		MinAccuracy: int(r.MinAccuracy), Status: domain.Status(r.Status), ApprovedBy: keys[4],
		ApprovedAt: optionalTime(r.ApprovedAt), ApprovedModelRevision: int(r.ApprovedModelRevision),
		SuspendedBy: keys[5], SuspendedAt: optionalTime(r.SuspendedAt), CreatedBy: keys[6],
		CreatedAt: timeFrom(r.CreatedAt), UpdatedAt: timeFrom(r.UpdatedAt), Version: int(r.Version),
	}, nil
}

func (AgentRepository) ListAgents(ctx context.Context, owner, model shared.ID, max int) ([]domain.Agent, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuids(owner, model)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAgents(ctx, sqlc.ListAgentsParams{OwnerID: keys[0], ModelID: keys[1], Max: int32(max)})
	if err != nil {
		return nil, agentError(err, "listing agents")
	}
	out := make([]domain.Agent, 0, len(rows))
	for _, r := range rows {
		a, err := agentOf(sqlc.FindAgentRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (AgentRepository) FindAgent(ctx context.Context, id shared.ID) (domain.Agent, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Agent{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Agent{}, err
	}
	row, err := queries.FindAgent(ctx, key)
	if err != nil {
		return domain.Agent{}, agentError(err, "reading an agent")
	}
	return agentOf(row)
}

func (AgentRepository) FindAgentByAccount(ctx context.Context, account shared.ID) (domain.Agent, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Agent{}, err
	}
	key, err := uuidOf(account)
	if err != nil {
		return domain.Agent{}, err
	}
	row, err := queries.FindAgentByAccount(ctx, key)
	if err != nil {
		return domain.Agent{}, agentError(err, "reading an account's agent")
	}
	return agentOf(sqlc.FindAgentRow(row))
}

func (AgentRepository) InsertAgent(ctx context.Context, a domain.Agent) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(a.ID, a.AccountID, a.OwnerID, a.ModelID, a.CreatedBy)
	if err != nil {
		return err
	}
	err = queries.InsertAgent(ctx, sqlc.InsertAgentParams{
		ID: keys[0], AccountID: keys[1], Name: a.Name, Purpose: a.Purpose, Kind: string(a.Kind), OwnerID: keys[2],
		RiskClass: string(a.RiskClass), Scopes: nonNil(a.Scopes), ModelID: keys[3],
		PolicyReversible: string(a.Policy.Reversible), PolicyIrreversible: string(a.Policy.Irreversible),
		TokensPerDay: a.Budget.TokensPerDay, ActionsPerHour: int32(a.Budget.ActionsPerHour),
		ConcurrentRuns: int32(a.Budget.ConcurrentRuns), MinAccuracy: int16(a.MinAccuracy), Status: string(a.Status),
		CreatedBy: keys[4], At: timestampOf(a.CreatedAt),
	})
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("agents.account_taken")
	}
	return agentError(err, "writing an agent")
}

func (AgentRepository) UpdateAgent(ctx context.Context, a domain.Agent, expected int, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(a.ID, a.OwnerID, a.ModelID, a.ApprovedBy, a.SuspendedBy)
	if err != nil {
		return err
	}
	n, err := queries.UpdateAgent(ctx, sqlc.UpdateAgentParams{
		ID: keys[0], Name: a.Name, Purpose: a.Purpose, Kind: string(a.Kind), OwnerID: keys[1],
		RiskClass: string(a.RiskClass), Scopes: nonNil(a.Scopes), ModelID: keys[2],
		PolicyReversible: string(a.Policy.Reversible), PolicyIrreversible: string(a.Policy.Irreversible),
		TokensPerDay: a.Budget.TokensPerDay, ActionsPerHour: int32(a.Budget.ActionsPerHour),
		ConcurrentRuns: int32(a.Budget.ConcurrentRuns), MinAccuracy: int16(a.MinAccuracy), Status: string(a.Status),
		ApprovedBy: keys[3], ApprovedAt: optionalTimestamp(a.ApprovedAt),
		ApprovedModelRevision: int32(a.ApprovedModelRevision), SuspendedBy: keys[4],
		SuspendedAt: optionalTimestamp(a.SuspendedAt), At: timestampOf(at), ExpectedVersion: int32(expected),
	})
	if err != nil {
		return agentError(err, "changing an agent")
	}
	if n == 0 {
		return shared.ErrVersionConflict
	}
	return nil
}

func (AgentRepository) Halt(ctx context.Context) (repository.Halt, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Halt{}, err
	}
	row, err := queries.FindAgentHalt(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.Halt{}, nil
	}
	if err != nil {
		return repository.Halt{}, agentError(err, "reading the agents' kill switch")
	}
	by, err := optionalID(row.SetBy)
	if err != nil {
		return repository.Halt{}, err
	}
	return repository.Halt{Halted: row.Halted, SetBy: by, SetAt: timeFrom(row.SetAt)}, nil
}

func (AgentRepository) PutHalt(ctx context.Context, id shared.ID, h repository.Halt) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(id, h.SetBy)
	if err != nil {
		return err
	}
	return agentError(queries.PutAgentHalt(ctx, sqlc.PutAgentHaltParams{
		ID: keys[0], Halted: h.Halted, SetBy: keys[1], At: timestampOf(h.SetAt),
	}), "writing the agents' kill switch")
}

func (AgentRepository) RecordAction(ctx context.Context, a domain.Action) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(a.ID, a.AgentID, a.RequestID)
	if err != nil {
		return err
	}
	return agentError(queries.InsertAgentAction(ctx, sqlc.InsertAgentActionParams{
		ID: keys[0], AgentID: keys[1], UseCase: a.UseCase, Class: string(a.Class), Decision: string(a.Decision),
		Code: a.Code, Tokens: a.Tokens, RequestID: keys[2], At: timestampOf(a.At),
	}), "recording an agent's act")
}

func (AgentRepository) Usage(ctx context.Context, agent shared.ID, hourFrom, dayFrom time.Time) (domain.Usage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Usage{}, err
	}
	key, err := uuidOf(agent)
	if err != nil {
		return domain.Usage{}, err
	}
	row, err := queries.AgentUsage(ctx, sqlc.AgentUsageParams{
		HourFrom: timestampOf(hourFrom), DayFrom: timestampOf(dayFrom), AgentID: key,
	})
	if err != nil {
		return domain.Usage{}, agentError(err, "counting an agent's acts")
	}
	return domain.Usage{ActionsLastHour: int(row.Actions), TokensToday: row.Tokens, NotifiedLastHour: int(row.Notified)}, nil
}

func (AgentRepository) ListActions(ctx context.Context, agent shared.ID, before *time.Time, max int) ([]domain.Action, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(agent)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAgentActions(ctx, sqlc.ListAgentActionsParams{
		AgentID: key, Before: optionalTimestamp(before), Max: int32(max),
	})
	if err != nil {
		return nil, agentError(err, "listing an agent's acts")
	}
	out := make([]domain.Action, 0, len(rows))
	for _, r := range rows {
		keys, err := ids(r.ID, r.AgentID, r.RequestID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.Action{
			ID: keys[0], AgentID: keys[1], UseCase: r.UseCase, Class: domain.Class(r.Class),
			Decision: domain.Decision(r.Decision), Code: r.Code, Tokens: r.Tokens, RequestID: keys[2], At: timeFrom(r.At),
		})
	}
	return out, nil
}

func (AgentRepository) Metering(ctx context.Context, agent shared.ID, from time.Time, max int) ([]repository.Metering, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := optionalUUID(agent)
	if err != nil {
		return nil, err
	}
	rows, err := queries.AgentMetering(ctx, sqlc.AgentMeteringParams{From: timestampOf(from), AgentID: key, Max: int32(max)})
	if err != nil {
		return nil, agentError(err, "metering agents")
	}
	out := make([]repository.Metering, 0, len(rows))
	for _, r := range rows {
		id, err := idFrom(r.AgentID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.Metering{
			AgentID: id, Day: r.Day.Time.UTC(), Allowed: int(r.Allowed), Held: int(r.Held), Refused: int(r.Refused),
			Tokens: r.Tokens,
		})
	}
	return out, nil
}

func requestOf(r sqlc.FindAgentRequestRow) (domain.Request, error) {
	keys, err := ids(r.ID, r.AgentID, r.DecidedBy)
	if err != nil {
		return domain.Request{}, err
	}
	var input map[string]any
	if err := json.Unmarshal(r.Input, &input); err != nil {
		return domain.Request{}, shared.ErrInternal.WithDetail("agents.stored_shape").WithCause(err)
	}
	return domain.Request{
		ID: keys[0], AgentID: keys[1], UseCase: r.UseCase, Class: domain.Class(r.Class), Input: input,
		Status: domain.RequestStatus(r.Status), DecidedBy: keys[2], DecidedAt: optionalTime(r.DecidedAt),
		Code: r.Code, CreatedAt: timeFrom(r.CreatedAt), Version: int(r.Version),
	}, nil
}

func (AgentRepository) InsertRequest(ctx context.Context, r domain.Request) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(r.ID, r.AgentID)
	if err != nil {
		return err
	}
	input, err := json.Marshal(r.Input)
	if err != nil {
		return shared.ErrValidation.WithDetail("agents.input_unreadable").WithCause(err)
	}
	return agentError(queries.InsertAgentRequest(ctx, sqlc.InsertAgentRequestParams{
		ID: keys[0], AgentID: keys[1], UseCase: r.UseCase, Class: string(r.Class), Input: input,
		At: timestampOf(r.CreatedAt),
	}), "holding an agent's act")
}

func (AgentRepository) FindRequest(ctx context.Context, id shared.ID) (domain.Request, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Request{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Request{}, err
	}
	row, err := queries.FindAgentRequest(ctx, key)
	if err != nil {
		return domain.Request{}, agentError(err, "reading a held act")
	}
	return requestOf(row)
}

func (AgentRepository) ListRequests(ctx context.Context, status domain.RequestStatus, agent shared.ID, max int) ([]domain.Request, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := optionalUUID(agent)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAgentRequests(ctx, sqlc.ListAgentRequestsParams{
		Status: optionalText(string(status)), AgentID: key, Max: int32(max),
	})
	if err != nil {
		return nil, agentError(err, "listing held acts")
	}
	out := make([]domain.Request, 0, len(rows))
	for _, r := range rows {
		request, err := requestOf(sqlc.FindAgentRequestRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, request)
	}
	return out, nil
}

func (AgentRepository) DecideRequest(ctx context.Context, r domain.Request, expected int) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	keys, err := uuids(r.ID, r.DecidedBy)
	if err != nil {
		return false, err
	}
	n, err := queries.DecideAgentRequest(ctx, sqlc.DecideAgentRequestParams{
		Status: string(r.Status), DecidedBy: keys[1], DecidedAt: optionalTimestamp(r.DecidedAt), Code: r.Code,
		ID: keys[0], ExpectedVersion: int32(expected),
	})
	if err != nil {
		return false, agentError(err, "deciding a held act")
	}
	return n > 0, nil
}

func (AgentRepository) FailRequest(ctx context.Context, id shared.ID, code string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	_, err = queries.FailAgentRequest(ctx, sqlc.FailAgentRequestParams{Code: code, ID: key})
	return agentError(err, "failing a held act")
}

func (AgentRepository) ListCases(ctx context.Context, agent shared.ID, max int) ([]domain.Case, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(agent)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAgentCases(ctx, sqlc.ListAgentCasesParams{AgentID: key, Max: int32(max)})
	if err != nil {
		return nil, agentError(err, "listing an agent's cases")
	}
	out := make([]domain.Case, 0, len(rows))
	for _, r := range rows {
		keys, err := ids(r.ID, r.AgentID, r.CreatedBy)
		if err != nil {
			return nil, err
		}
		var expected map[string]any
		if err := json.Unmarshal(r.Expected, &expected); err != nil {
			return nil, shared.ErrInternal.WithDetail("agents.stored_shape").WithCause(err)
		}
		out = append(out, domain.Case{
			ID: keys[0], AgentID: keys[1], Title: r.Title, Notes: r.Notes, Expected: expected, CreatedBy: keys[2],
			CreatedAt: timeFrom(r.CreatedAt),
		})
	}
	return out, nil
}

func (AgentRepository) InsertCase(ctx context.Context, c domain.Case) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(c.ID, c.AgentID, c.CreatedBy)
	if err != nil {
		return err
	}
	expected, err := json.Marshal(c.Expected)
	if err != nil {
		return shared.ErrValidation.WithDetail("agent_cases.expected_invalid").WithCause(err)
	}
	return agentError(queries.InsertAgentCase(ctx, sqlc.InsertAgentCaseParams{
		ID: keys[0], AgentID: keys[1], Title: c.Title, Notes: c.Notes, Expected: expected, CreatedBy: keys[2],
		At: timestampOf(c.CreatedAt),
	}), "writing an agent's case")
}

func (AgentRepository) DeleteCase(ctx context.Context, agent, id shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	keys, err := uuids(agent, id)
	if err != nil {
		return false, err
	}
	n, err := queries.DeleteAgentCase(ctx, sqlc.DeleteAgentCaseParams{AgentID: keys[0], ID: keys[1]})
	return n > 0, agentError(err, "deleting an agent's case")
}

func (AgentRepository) InsertRun(ctx context.Context, r domain.Run) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(r.ID, r.AgentID, r.By)
	if err != nil {
		return err
	}
	return agentError(queries.InsertAgentRun(ctx, sqlc.InsertAgentRunParams{
		ID: keys[0], AgentID: keys[1], ModelRevision: int32(r.ModelRevision), Cases: int32(r.Cases),
		Passed: int32(r.Passed), RunBy: keys[2], At: timestampOf(r.At),
	}), "writing an evaluation run")
}

func (AgentRepository) ListRuns(ctx context.Context, agent shared.ID, max int) ([]domain.Run, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(agent)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAgentRuns(ctx, sqlc.ListAgentRunsParams{AgentID: key, Max: int32(max)})
	if err != nil {
		return nil, agentError(err, "listing evaluation runs")
	}
	out := make([]domain.Run, 0, len(rows))
	for _, r := range rows {
		keys, err := ids(r.ID, r.AgentID, r.RunBy)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.Run{
			ID: keys[0], AgentID: keys[1], ModelRevision: int(r.ModelRevision), Cases: int(r.Cases),
			Passed: int(r.Passed), By: keys[2], At: timeFrom(r.At),
		})
	}
	return out, nil
}

func (AgentRepository) InsertMark(ctx context.Context, m domain.Mark) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(m.ID, m.TargetID, m.SuggestionID, m.AgentID, m.AcceptedBy)
	if err != nil {
		return err
	}
	return agentError(queries.InsertAiProvenance(ctx, sqlc.InsertAiProvenanceParams{
		ID: keys[0], TargetType: m.TargetType, TargetID: keys[1], Fields: nonNil(m.Fields), SuggestionID: keys[2],
		AgentID: keys[3], Model: m.Model, PromptID: m.PromptID, PromptVersion: m.PromptVersion, AcceptedBy: keys[4],
		AcceptedAt: timestampOf(m.AcceptedAt),
	}), "marking an accepted AI value")
}

func (AgentRepository) ListMarks(ctx context.Context, targetType string, target shared.ID, max int) ([]domain.Mark, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(target)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAiProvenance(ctx, sqlc.ListAiProvenanceParams{TargetType: targetType, TargetID: key, Max: int32(max)})
	if err != nil {
		return nil, agentError(err, "listing AI provenance")
	}
	out := make([]domain.Mark, 0, len(rows))
	for _, r := range rows {
		keys, err := ids(r.ID, r.TargetID, r.SuggestionID, r.AgentID, r.AcceptedBy)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.Mark{
			ID: keys[0], TargetType: r.TargetType, TargetID: keys[1], Fields: r.Fields, SuggestionID: keys[2],
			AgentID: keys[3], Model: r.Model, PromptID: r.PromptID, PromptVersion: r.PromptVersion, AcceptedBy: keys[4],
			AcceptedAt: timeFrom(r.AcceptedAt),
		})
	}
	return out, nil
}

func jobOf(r sqlc.FindAgentJobRow) (domain.Job, error) {
	keys, err := ids(r.ID, r.AgentID, r.TargetID, r.StartedBy)
	if err != nil {
		return domain.Job{}, err
	}
	return domain.Job{
		ID: keys[0], AgentID: keys[1], TargetType: domain.TargetType(r.TargetType), TargetID: keys[2],
		Status: domain.JobStatus(r.Status), Code: r.Code, Proposals: int(r.Proposals), Tokens: r.Tokens,
		StartedBy: keys[3], StartedAt: timeFrom(r.StartedAt), EndedAt: optionalTime(r.EndedAt),
	}, nil
}

func (AgentRepository) InsertJob(ctx context.Context, j domain.Job) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuids(j.ID, j.AgentID, j.TargetID)
	if err != nil {
		return err
	}
	by, err := optionalUUID(j.StartedBy)
	if err != nil {
		return err
	}
	return agentError(queries.InsertAgentJob(ctx, sqlc.InsertAgentJobParams{
		ID: keys[0], AgentID: keys[1], TargetType: string(j.TargetType), TargetID: keys[2],
		StartedBy: by, StartedAt: timestampOf(j.StartedAt),
	}), "starting an agent's job")
}

func (AgentRepository) EndJob(ctx context.Context, j domain.Job) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(j.ID)
	if err != nil {
		return false, err
	}
	n, err := queries.EndAgentJob(ctx, sqlc.EndAgentJobParams{
		Status: string(j.Status), Code: j.Code, Proposals: int32(j.Proposals), Tokens: j.Tokens,
		EndedAt: optionalTimestamp(j.EndedAt), ID: key,
	})
	return n == 1, agentError(err, "ending an agent's job")
}

func (AgentRepository) FindJob(ctx context.Context, id shared.ID) (domain.Job, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Job{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Job{}, err
	}
	row, err := queries.FindAgentJob(ctx, key)
	if err != nil {
		return domain.Job{}, agentError(err, "reading an agent's job")
	}
	return jobOf(row)
}

func (AgentRepository) ListJobs(ctx context.Context, agent shared.ID, max int) ([]domain.Job, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(agent)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListAgentJobs(ctx, sqlc.ListAgentJobsParams{AgentID: key, Max: int32(max)})
	if err != nil {
		return nil, agentError(err, "listing an agent's jobs")
	}
	out := make([]domain.Job, 0, len(rows))
	for _, r := range rows {
		j, err := jobOf(sqlc.FindAgentJobRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func (AgentRepository) RunningJobs(ctx context.Context, agent shared.ID, since time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	key, err := uuidOf(agent)
	if err != nil {
		return 0, err
	}
	n, err := queries.AgentJobsRunning(ctx, sqlc.AgentJobsRunningParams{AgentID: key, Since: timestampOf(since)})
	return int(n), agentError(err, "counting an agent's running jobs")
}

// AgentName answers an agent's name, read in the caller's transaction: what a notice about it says.
func (r AgentRepository) AgentName(ctx context.Context, id shared.ID) (string, error) {
	a, err := r.FindAgent(ctx, id)
	return a.Name, err
}
