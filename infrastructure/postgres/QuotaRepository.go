// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// QuotaRepository is the §4 limits' surface (H-08): the overrides in the tenant's settings
// document, the live counts, and the billing ledger. One type for the three ports - they are
// three angles on one subject, and every method is bounded by the transaction it runs in.
type QuotaRepository struct{}

func NewQuotaRepository() QuotaRepository { return QuotaRepository{} }

var (
	_ repository.Store = QuotaRepository{}
	_ repository.Usage = QuotaRepository{}
	_ repository.Meter = QuotaRepository{}
)

// quotasDocument is the settings key's shape - the adapter's knowledge, never the application
// layer's (TenantPolicy's discipline). Pointers, because absence and 0 mean different things.
type quotasDocument struct {
	APIRequestsPerMinute  *int64 `json:"api_requests_per_minute,omitempty"`
	Items                 *int64 `json:"items,omitempty"`
	MediaBytes            *int64 `json:"media_bytes,omitempty"`
	AutomationRunsPerHour *int64 `json:"automation_runs_per_hour,omitempty"`
	WebhookTargets        *int64 `json:"webhook_targets,omitempty"`
	ExportJobs            *int64 `json:"export_jobs,omitempty"`
	AiTokensPerDay        *int64 `json:"ai_tokens_per_day,omitempty"`
}

// Overrides answers what the transaction's tenant configured.
func (QuotaRepository) Overrides(ctx context.Context) (repository.Overrides, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Overrides{}, err
	}

	raw, err := queries.TenantQuotaOverrides(ctx)
	if err != nil {
		if IsNoRows(err) {
			return repository.Overrides{}, nil
		}
		return repository.Overrides{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the quota overrides: %w", err))
	}

	var document quotasDocument
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		// Fail closed into the defaults rather than into an outage: a corrupt overrides key is
		// the operator's to repair, and every request refusing until then would be the worse
		// outcome. The write path below can only produce a valid document.
		return repository.Overrides{}, nil
	}
	return fromDocument(document), nil
}

// fromDocument and toDocument are the two mappings, named so that a test can walk them.
//
// They are a field list, which is the shape that goes wrong quietly: J-15 added a row to the
// document's struct and to neither of these, so an override an operator wrote came back unset with
// nothing failing. `TestEveryQuotaOverrideSurvivesTheDocument` walks every field through both.
func fromDocument(document quotasDocument) repository.Overrides {
	return repository.Overrides{
		APIRequestsPerMinute:  document.APIRequestsPerMinute,
		Items:                 document.Items,
		MediaBytes:            document.MediaBytes,
		AutomationRunsPerHour: document.AutomationRunsPerHour,
		WebhookTargets:        document.WebhookTargets,
		ExportJobs:            document.ExportJobs,
		AiTokensPerDay:        document.AiTokensPerDay,
	}
}

func toDocument(overrides repository.Overrides) quotasDocument {
	return quotasDocument{
		APIRequestsPerMinute:  overrides.APIRequestsPerMinute,
		Items:                 overrides.Items,
		MediaBytes:            overrides.MediaBytes,
		AutomationRunsPerHour: overrides.AutomationRunsPerHour,
		WebhookTargets:        overrides.WebhookTargets,
		ExportJobs:            overrides.ExportJobs,
		AiTokensPerDay:        overrides.AiTokensPerDay,
	}
}

// SetOverrides replaces the quotas key, guarded on the row version.
func (QuotaRepository) SetOverrides(
	ctx context.Context, overrides repository.Overrides, expectedVersion int, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	payload, err := json.Marshal(toDocument(overrides))
	if err != nil {
		return false, shared.Internalf("postgres: encoding the quota overrides: %w", err)
	}

	changed, err := queries.SetTenantQuotas(ctx, sqlc.SetTenantQuotasParams{
		Quotas: payload,
		Now:    pgtype.Timestamptz{Time: now, Valid: true},
		//nolint:gosec // G115: a row version, bounded far below either type's range
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the quota overrides: %w", err))
	}
	return changed > 0, nil
}

// Items counts the workspace's work items, trash included.
func (QuotaRepository) Items(ctx context.Context) (int64, error) {
	return count(ctx, "counting the items", func(q *sqlc.Queries) (int64, error) {
		return q.CountTenantItems(ctx)
	})
}

// MediaBytes sums the stored objects' sizes.
func (QuotaRepository) MediaBytes(ctx context.Context) (int64, error) {
	return count(ctx, "summing the media bytes", func(q *sqlc.Queries) (int64, error) {
		return q.SumTenantMediaBytes(ctx)
	})
}

// WebhookTargets counts the subscriptions.
func (QuotaRepository) WebhookTargets(ctx context.Context) (int64, error) {
	return count(ctx, "counting the webhook targets", func(q *sqlc.Queries) (int64, error) {
		return q.CountWebhookTargets(ctx)
	})
}

// AutomationRunsSince counts the runs that actually ran since the instant.
func (QuotaRepository) AutomationRunsSince(ctx context.Context, since time.Time) (int64, error) {
	return count(ctx, "counting the automation runs", func(q *sqlc.Queries) (int64, error) {
		return q.CountTenantRunsSince(ctx, pgtype.Timestamptz{Time: since, Valid: true})
	})
}

// LiveExports counts the pending and running export jobs.
func (QuotaRepository) LiveExports(ctx context.Context) (int64, error) {
	return count(ctx, "counting the live exports", func(q *sqlc.Queries) (int64, error) {
		return q.CountLiveTenantExports(ctx)
	})
}

// MeteredSince sums one metered thing since the instant.
//
// From the billing ledger, which every other count here refuses to read - the port says why: what
// it sums has no row of its own, so the tally is not a lagging copy of something countable but the
// only record there is. Generic in the metric, so the vocabulary stays the application layer's.
func (QuotaRepository) MeteredSince(
	ctx context.Context, metric string, since time.Time,
) (int64, error) {
	return count(ctx, "summing the metered usage", func(q *sqlc.Queries) (int64, error) {
		return q.SumTenantUsageSince(ctx, sqlc.SumTenantUsageSinceParams{
			Metric: metric,
			Since:  pgtype.Date{Time: since.UTC(), Valid: true},
		})
	})
}

// Add increases one metric's daily tally in the billing ledger.
func (QuotaRepository) Add(ctx context.Context, metric string, at time.Time, amount int64) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	err = queries.AddUsage(ctx, sqlc.AddUsageParams{
		Period: pgtype.Date{Time: at.UTC(), Valid: true},
		Metric: metric,
		Amount: amount,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("metering %s: %w", metric, err))
	}
	return nil
}

// count is the shared shape of the live counters.
func count(ctx context.Context, act string, read func(*sqlc.Queries) (int64, error)) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	value, err := read(queries)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("%s: %w", act, err))
	}
	return value, nil
}
