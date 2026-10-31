// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ServiceDeskRepository stores business calendars, SLA policies and entries' SLA records
// (migration 0240).
type ServiceDeskRepository struct{}

func NewServiceDeskRepository() ServiceDeskRepository { return ServiceDeskRepository{} }

var (
	_ repository.Calendars = ServiceDeskCalendars{}
	_ repository.Policies  = ServiceDeskPolicies{}
	_ repository.Records   = ServiceDeskRecords{}
)

// ServiceDeskCalendars, ServiceDeskPolicies and ServiceDeskRecords are the three ports.
type (
	ServiceDeskCalendars struct{}
	ServiceDeskPolicies  struct{}
	ServiceDeskRecords   struct{}
)

func (ServiceDeskRepository) Calendars() ServiceDeskCalendars { return ServiceDeskCalendars{} }
func (ServiceDeskRepository) Policies() ServiceDeskPolicies   { return ServiceDeskPolicies{} }
func (ServiceDeskRepository) Records() ServiceDeskRecords     { return ServiceDeskRecords{} }

func (ServiceDeskCalendars) Insert(ctx context.Context, cal servicedesk.Calendar) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(cal.ID)
	if err != nil {
		return err
	}
	tenant, err := uuidOf(cal.TenantID)
	if err != nil {
		return err
	}
	spec, err := calendarJSON(cal.Calendar)
	if err != nil {
		return shared.ErrInternal.WithDetail("calendars.unwritable").WithCause(err)
	}
	if err := queries.InsertBusinessCalendar(ctx, sqlc.InsertBusinessCalendarParams{
		ID: id, TenantID: tenant, Key: cal.Key, Name: cal.Name, Spec: spec, Now: timestampOf(cal.CreatedAt),
	}); err != nil {
		if isUniqueViolation(err) {
			return shared.ErrConflict.WithDetail("calendars.key_taken").WithParams(map[string]string{"key": cal.Key})
		}
		return queryFailed("writing a business calendar", err)
	}
	return nil
}

func (ServiceDeskCalendars) Update(ctx context.Context, cal servicedesk.Calendar, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	spec, err := calendarJSON(cal.Calendar)
	if err != nil {
		return shared.ErrInternal.WithDetail("calendars.unwritable").WithCause(err)
	}
	affected, err := queries.UpdateBusinessCalendar(ctx, sqlc.UpdateBusinessCalendarParams{
		Name: cal.Name, Spec: spec, Now: timestampOf(cal.UpdatedAt), Key: cal.Key,
		ExpectedVersion: int32(expectedVersion), //nolint:gosec // a version is far below 2^31
	})
	if err != nil {
		return queryFailed("updating a business calendar", err)
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("calendars.version_conflict")
	}
	return nil
}

func calendarFrom(row sqlc.BusinessCalendar) (servicedesk.Calendar, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return servicedesk.Calendar{}, err
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return servicedesk.Calendar{}, err
	}
	cal, err := calendarFromJSON(row.Spec)
	if err != nil {
		return servicedesk.Calendar{}, shared.ErrInternal.WithDetail("calendars.unreadable").WithCause(err)
	}
	return servicedesk.Calendar{
		ID: id, TenantID: tenant, Key: row.Key, Name: row.Name, Calendar: cal,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, Version: int(row.Version),
	}, nil
}

func (ServiceDeskCalendars) Find(ctx context.Context, key string) (servicedesk.Calendar, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.Calendar{}, err
	}
	row, err := queries.FindBusinessCalendar(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return servicedesk.Calendar{}, shared.ErrNotFound.WithDetail("calendars.not_found")
		}
		return servicedesk.Calendar{}, queryFailed("reading a business calendar", err)
	}
	return calendarFrom(row)
}

func (ServiceDeskCalendars) List(ctx context.Context) ([]servicedesk.Calendar, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListBusinessCalendars(ctx)
	if err != nil {
		return nil, queryFailed("listing business calendars", err)
	}
	out := make([]servicedesk.Calendar, 0, len(rows))
	for _, row := range rows {
		cal, err := calendarFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, cal)
	}
	return out, nil
}

func (ServiceDeskCalendars) Delete(ctx context.Context, key string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteBusinessCalendar(ctx, key)
	if err != nil {
		if isForeignKeyViolation(err) {
			return shared.ErrConflict.WithDetail("calendars.in_use").WithParams(map[string]string{"key": key})
		}
		return queryFailed("deleting a business calendar", err)
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("calendars.not_found")
	}
	return nil
}

// goalDocument is a policy's goal as stored.
type goalDocument struct {
	Kind          string         `json:"kind"`
	Minutes       int            `json:"minutes"`
	ByPriority    map[string]int `json:"by_priority,omitempty"`
	PauseStatuses []string       `json:"pause_statuses,omitempty"`
	WarnPercent   int            `json:"warn_percent,omitempty"`
}

func goalsJSON(goals []servicedesk.SLAGoal) ([]byte, error) {
	docs := make([]goalDocument, 0, len(goals))
	for _, goal := range goals {
		docs = append(docs, goalDocument{
			Kind: string(goal.Kind), Minutes: goal.Minutes, ByPriority: goal.ByPriority,
			PauseStatuses: goal.PauseStatuses, WarnPercent: goal.WarnPercent,
		})
	}
	return json.Marshal(docs)
}

func goalsFromJSON(raw []byte) ([]servicedesk.SLAGoal, error) {
	var docs []goalDocument
	if err := json.Unmarshal(raw, &docs); err != nil {
		return nil, err
	}
	goals := make([]servicedesk.SLAGoal, 0, len(docs))
	for _, doc := range docs {
		goals = append(goals, servicedesk.SLAGoal{
			Kind: servicedesk.GoalKind(doc.Kind), Minutes: doc.Minutes, ByPriority: doc.ByPriority,
			PauseStatuses: doc.PauseStatuses, WarnPercent: doc.WarnPercent,
		})
	}
	return goals, nil
}

func optionalKey(key string) *string {
	if key == "" {
		return nil
	}
	return &key
}

func policyWriteFailed(policy servicedesk.SLAPolicy, err error) error {
	switch {
	case isUniqueViolation(err):
		return shared.ErrConflict.WithDetail("sla_policies.key_taken").WithParams(map[string]string{"key": policy.Key})
	case isForeignKeyViolation(err):
		return shared.ErrValidation.WithDetail("sla_policies.calendar_unknown").
			WithFields(shared.FieldError{Path: "/calendar", Code: "sla_policies.calendar_unknown"})
	}
	return queryFailed("writing an SLA policy", err)
}

func (ServiceDeskPolicies) Insert(ctx context.Context, policy servicedesk.SLAPolicy) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(policy.ID)
	if err != nil {
		return err
	}
	tenant, err := uuidOf(policy.TenantID)
	if err != nil {
		return err
	}
	goals, err := goalsJSON(policy.Goals)
	if err != nil {
		return shared.ErrInternal.WithDetail("sla_policies.unwritable").WithCause(err)
	}
	if err := queries.InsertSlaPolicy(ctx, sqlc.InsertSlaPolicyParams{
		ID: id, TenantID: tenant, Key: policy.Key, Name: policy.Name, CalendarKey: optionalKey(policy.CalendarKey),
		EntryTypes: policy.EntryTypes, Organisations: keysOrEmpty(policy.Organisations), PriorityField: policy.PriorityField,
		Goals: goals, Position: int32(policy.Position), //nolint:gosec // a position is small
		Enabled: policy.Enabled, Now: timestampOf(policy.CreatedAt),
	}); err != nil {
		return policyWriteFailed(policy, err)
	}
	return nil
}

func (ServiceDeskPolicies) Update(ctx context.Context, policy servicedesk.SLAPolicy, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	goals, err := goalsJSON(policy.Goals)
	if err != nil {
		return shared.ErrInternal.WithDetail("sla_policies.unwritable").WithCause(err)
	}
	affected, err := queries.UpdateSlaPolicy(ctx, sqlc.UpdateSlaPolicyParams{
		Name: policy.Name, CalendarKey: optionalKey(policy.CalendarKey), EntryTypes: policy.EntryTypes,
		Organisations: keysOrEmpty(policy.Organisations), PriorityField: policy.PriorityField, Goals: goals,
		Position: int32(policy.Position), //nolint:gosec // a position is small
		Enabled:  policy.Enabled, Now: timestampOf(policy.UpdatedAt), Key: policy.Key,
		ExpectedVersion: int32(expectedVersion), //nolint:gosec // a version is far below 2^31
	})
	if err != nil {
		return policyWriteFailed(policy, err)
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("sla_policies.version_conflict")
	}
	return nil
}

// keysOrEmpty sends no keys as an empty array: the column holds no NULL.
func keysOrEmpty(keys []string) []string {
	if keys == nil {
		return []string{}
	}
	return keys
}

func policyFrom(row sqlc.SlaPolicy) (servicedesk.SLAPolicy, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return servicedesk.SLAPolicy{}, err
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return servicedesk.SLAPolicy{}, err
	}
	goals, err := goalsFromJSON(row.Goals)
	if err != nil {
		return servicedesk.SLAPolicy{}, shared.ErrInternal.WithDetail("sla_policies.unreadable").WithCause(err)
	}
	policy := servicedesk.SLAPolicy{
		ID: id, TenantID: tenant, Key: row.Key, Name: row.Name, EntryTypes: row.EntryTypes,
		Organisations: row.Organisations, PriorityField: row.PriorityField, Goals: goals, Position: int(row.Position), Enabled: row.Enabled,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, Version: int(row.Version),
	}
	if row.CalendarKey != nil {
		policy.CalendarKey = *row.CalendarKey
	}
	return policy, nil
}

func (ServiceDeskPolicies) Find(ctx context.Context, key string) (servicedesk.SLAPolicy, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.SLAPolicy{}, err
	}
	row, err := queries.FindSlaPolicy(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return servicedesk.SLAPolicy{}, shared.ErrNotFound.WithDetail("sla_policies.not_found")
		}
		return servicedesk.SLAPolicy{}, queryFailed("reading an SLA policy", err)
	}
	return policyFrom(row)
}

func (ServiceDeskPolicies) List(ctx context.Context) ([]servicedesk.SLAPolicy, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListSlaPolicies(ctx)
	if err != nil {
		return nil, queryFailed("listing SLA policies", err)
	}
	out := make([]servicedesk.SLAPolicy, 0, len(rows))
	for _, row := range rows {
		policy, err := policyFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, policy)
	}
	return out, nil
}

func (ServiceDeskPolicies) Delete(ctx context.Context, key string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteSlaPolicy(ctx, key)
	if err != nil {
		return queryFailed("deleting an SLA policy", err)
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("sla_policies.not_found")
	}
	return nil
}

func (ServiceDeskRecords) Save(ctx context.Context, record servicedesk.EntrySLA, now time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(record.ID)
	if err != nil {
		return err
	}
	tenant, err := uuidOf(record.TenantID)
	if err != nil {
		return err
	}
	item, err := uuidOf(record.ItemID)
	if err != nil {
		return err
	}
	if err := queries.UpsertEntrySla(ctx, sqlc.UpsertEntrySlaParams{
		ID: id, TenantID: tenant, ItemID: item, Goal: string(record.Goal), PolicyKey: record.PolicyKey,
		TargetSeconds: int64(record.Target / time.Second), Standing: string(record.Standing),
		ArmedAt: timestampOf(record.ArmedAt), DueAt: timestampOrNull(record.DueAt),
		AtRiskAt: timestampOrNull(record.AtRiskAt), MetAt: timestampOrNull(record.MetAt),
		BreachedAt: timestampOrNull(record.BreachedAt), Now: timestampOf(now),
	}); err != nil {
		return queryFailed("writing an entry's SLA record", err)
	}
	return nil
}

func (ServiceDeskRecords) OfItem(ctx context.Context, itemID shared.ID) ([]servicedesk.EntrySLA, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListEntrySla(ctx, item)
	if err != nil {
		return nil, queryFailed("reading an entry's SLA records", err)
	}
	out := make([]servicedesk.EntrySLA, 0, len(rows))
	for _, row := range rows {
		record, err := recordFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, nil
}

func recordFrom(row sqlc.EntrySla) (servicedesk.EntrySLA, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return servicedesk.EntrySLA{}, err
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return servicedesk.EntrySLA{}, err
	}
	item, err := idFrom(row.ItemID)
	if err != nil {
		return servicedesk.EntrySLA{}, err
	}
	return servicedesk.EntrySLA{
		ID: id, TenantID: tenant, ItemID: item, Goal: servicedesk.GoalKind(row.Goal), PolicyKey: row.PolicyKey,
		Target: time.Duration(row.TargetSeconds) * time.Second, Standing: servicedesk.Standing(row.Standing),
		ArmedAt: row.ArmedAt.Time, DueAt: optionalTime(row.DueAt), AtRiskAt: optionalTime(row.AtRiskAt),
		MetAt: optionalTime(row.MetAt), BreachedAt: optionalTime(row.BreachedAt), Version: int(row.Version),
	}, nil
}

func (ServiceDeskRecords) Report(ctx context.Context, from, to time.Time) ([]servicedesk.ReportRow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ReportEntrySla(ctx, sqlc.ReportEntrySlaParams{
		From: pgtype.Timestamptz{Time: from, Valid: true}, To: pgtype.Timestamptz{Time: to, Valid: true},
	})
	if err != nil {
		return nil, queryFailed("reporting SLA records", err)
	}
	out := make([]servicedesk.ReportRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, servicedesk.ReportRow{
			PolicyKey: row.PolicyKey, Goal: servicedesk.GoalKind(row.Goal),
			Standing: servicedesk.Standing(row.Standing), Entries: int(row.Entries),
		})
	}
	return out, nil
}
