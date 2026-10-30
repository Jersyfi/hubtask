// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// exporters is one entry per archived entity: the statement that reads it, and how its key becomes
// the cursor of the next page.
//
// A table rather than thirty branches, and hand-written rather than generated, because what it
// records is a decision per entity - which columns identify a row and whether the schema can date
// a change - and a generator would have to be told all of that anyway. The statements themselves
// are in db/queries/BackupExport.sql and are as alike as the schema permits; what differs here is
// only the shape of the key.
//
// Keyed by table name, which is the schema's vocabulary and the one the deletion markers are
// written in. The archive's own entity names stay on the other side of the port.
var exporters = map[string]exporter{
	"tenant": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTenants(ctx, sqlc.ExportTenantsParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTenantsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"account": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAccounts(ctx, sqlc.ExportAccountsParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAccountsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"account_group": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAccountGroups(ctx, sqlc.ExportAccountGroupsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAccountGroupsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"account_group_member": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAccountGroupMembers(ctx, sqlc.ExportAccountGroupMembersParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAccountGroupMembersRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"membership": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportMemberships(ctx, sqlc.ExportMembershipsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportMembershipsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"custom_role": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCustomRoles(ctx, sqlc.ExportCustomRolesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCustomRolesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"custom_role_grant": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCustomRoleGrants(ctx, sqlc.ExportCustomRoleGrantsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCustomRoleGrantsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"config_revision": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportConfigRevisions(ctx, sqlc.ExportConfigRevisionsParams{
				AfterID: cursorNumber(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportConfigRevisionsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"config_proposal": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportConfigProposals(ctx, sqlc.ExportConfigProposalsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportConfigProposalsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"config_template": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportConfigTemplates(ctx, sqlc.ExportConfigTemplatesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportConfigTemplatesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"config_template_use": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportConfigTemplateUses(ctx, sqlc.ExportConfigTemplateUsesParams{
				AfterID: cursorText(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportConfigTemplateUsesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"container": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportContainers(ctx, sqlc.ExportContainersParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportContainersRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"bucket": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportBuckets(ctx, sqlc.ExportBucketsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportBucketsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"label": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportLabels(ctx, sqlc.ExportLabelsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportLabelsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"entry_type": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportEntryTypes(ctx, sqlc.ExportEntryTypesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportEntryTypesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"workflow": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportWorkflows(ctx, sqlc.ExportWorkflowsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportWorkflowsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"workflow_scheme": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportWorkflowSchemes(ctx, sqlc.ExportWorkflowSchemesParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorText(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportWorkflowSchemesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"business_calendar": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportBusinessCalendars(ctx, sqlc.ExportBusinessCalendarsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportBusinessCalendarsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"sla_policy": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportSlaPolicies(ctx, sqlc.ExportSlaPoliciesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportSlaPoliciesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"reply_template": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportReplyTemplates(ctx, sqlc.ExportReplyTemplatesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportReplyTemplatesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"entry_schedule": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportEntrySchedules(ctx, sqlc.ExportEntrySchedulesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportEntrySchedulesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"plan_baseline": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPlanBaselines(ctx, sqlc.ExportPlanBaselinesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPlanBaselinesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"plan_baseline_entry": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPlanBaselineEntries(ctx, sqlc.ExportPlanBaselineEntriesParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPlanBaselineEntriesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"sprint": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportSprints(ctx, sqlc.ExportSprintsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportSprintsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"sprint_entry": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportSprintEntries(ctx, sqlc.ExportSprintEntriesParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportSprintEntriesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"entry_estimate": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportEntryEstimates(ctx, sqlc.ExportEntryEstimatesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportEntryEstimatesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"time_work_type": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTimeWorkTypes(ctx, sqlc.ExportTimeWorkTypesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTimeWorkTypesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"time_record": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTimeRecords(ctx, sqlc.ExportTimeRecordsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTimeRecordsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"time_timer": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTimeTimers(ctx, sqlc.ExportTimeTimersParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTimeTimersRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"timesheet": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTimesheets(ctx, sqlc.ExportTimesheetsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTimesheetsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"time_policy": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTimePolicies(ctx, sqlc.ExportTimePoliciesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTimePoliciesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"time_capacity": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTimeCapacities(ctx, sqlc.ExportTimeCapacitiesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTimeCapacitiesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"time_absence": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTimeAbsences(ctx, sqlc.ExportTimeAbsencesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTimeAbsencesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"time_cost_rate": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTimeCostRates(ctx, sqlc.ExportTimeCostRatesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTimeCostRatesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"time_budget": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTimeBudgets(ctx, sqlc.ExportTimeBudgetsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTimeBudgetsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"goal_period": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportGoalPeriods(ctx, sqlc.ExportGoalPeriodsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportGoalPeriodsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"goal": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportGoals(ctx, sqlc.ExportGoalsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportGoalsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"goal_entry": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportGoalEntries(ctx, sqlc.ExportGoalEntriesParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportGoalEntriesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"goal_check_in": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportGoalCheckIns(ctx, sqlc.ExportGoalCheckInsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportGoalCheckInsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"portfolio": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPortfolios(ctx, sqlc.ExportPortfoliosParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPortfoliosRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"portfolio_item": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPortfolioItems(ctx, sqlc.ExportPortfolioItemsParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPortfolioItemsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"portfolio_update": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPortfolioUpdates(ctx, sqlc.ExportPortfolioUpdatesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPortfolioUpdatesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"dashboard": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportDashboards(ctx, sqlc.ExportDashboardsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportDashboardsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"dashboard_digest": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportDashboardDigests(ctx, sqlc.ExportDashboardDigestsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportDashboardDigestsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"agent_model": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAgentModels(ctx, sqlc.ExportAgentModelsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAgentModelsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"agent": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAgents(ctx, sqlc.ExportAgentsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAgentsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"agent_halt": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAgentHalts(ctx, sqlc.ExportAgentHaltsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAgentHaltsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"agent_request": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAgentRequests(ctx, sqlc.ExportAgentRequestsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAgentRequestsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"agent_job": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAgentJobs(ctx, sqlc.ExportAgentJobsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAgentJobsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"agent_action": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAgentActions(ctx, sqlc.ExportAgentActionsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAgentActionsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"agent_eval_case": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAgentEvalCases(ctx, sqlc.ExportAgentEvalCasesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAgentEvalCasesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"agent_eval_run": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAgentEvalRuns(ctx, sqlc.ExportAgentEvalRunsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAgentEvalRunsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"ai_provenance": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAiProvenance(ctx, sqlc.ExportAiProvenanceParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAiProvenanceRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"satisfaction_rating": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportSatisfactionRatings(ctx, sqlc.ExportSatisfactionRatingsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportSatisfactionRatingsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"entry_sla": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportEntrySlas(ctx, sqlc.ExportEntrySlasParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportEntrySlasRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"link_type": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportLinkTypes(ctx, sqlc.ExportLinkTypesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportLinkTypesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"entry_link": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportEntryLinks(ctx, sqlc.ExportEntryLinksParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportEntryLinksRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"page": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPages(ctx, sqlc.ExportPagesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPagesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"whiteboard_media": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportWhiteboardMedia(ctx, sqlc.ExportWhiteboardMediaParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportWhiteboardMediaRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"whiteboard_version": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportWhiteboardVersions(ctx, sqlc.ExportWhiteboardVersionsParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorNumber(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportWhiteboardVersionsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"page_version": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPageVersions(ctx, sqlc.ExportPageVersionsParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorNumber(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPageVersionsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"page_restriction": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPageRestrictions(ctx, sqlc.ExportPageRestrictionsParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPageRestrictionsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"page_share": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPageShares(ctx, sqlc.ExportPageSharesParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPageSharesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"page_article": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPageArticles(ctx, sqlc.ExportPageArticlesParams{
				AfterID: cursorUUID(page.After, 0), AfterSecond: cursorUUID(page.After, 1), Batch: page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPageArticlesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"page_article_rating": {
		delta: false,
		keys:  3,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPageArticleRatings(ctx, sqlc.ExportPageArticleRatingsParams{
				AfterID: cursorUUID(page.After, 0), AfterSecond: cursorUUID(page.After, 1),
				AfterThird: cursorUUID(page.After, 2), Batch: page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPageArticleRatingsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"page_public_link": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPagePublicLinks(ctx, sqlc.ExportPagePublicLinksParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPagePublicLinksRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"personal_space": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPersonalSpaces(ctx, sqlc.ExportPersonalSpacesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPersonalSpacesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"page_comment": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPageComments(ctx, sqlc.ExportPageCommentsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPageCommentsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_connector": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbConnectors(ctx, sqlc.ExportCmdbConnectorsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbConnectorsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_alert": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbAlerts(ctx, sqlc.ExportCmdbAlertsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbAlertsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_class": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbClasses(ctx, sqlc.ExportCmdbClassesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbClassesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_relation_type": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbRelationTypes(ctx, sqlc.ExportCmdbRelationTypesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbRelationTypesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_source": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbSources(ctx, sqlc.ExportCmdbSourcesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbSourcesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_ci": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbCis(ctx, sqlc.ExportCmdbCisParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbCisRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_ci_identifier": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbCiIdentifiers(ctx, sqlc.ExportCmdbCiIdentifiersParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorText(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbCiIdentifiersRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_relation": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbRelations(ctx, sqlc.ExportCmdbRelationsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbRelationsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_run": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbRuns(ctx, sqlc.ExportCmdbRunsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbRunsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_hold": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbHolds(ctx, sqlc.ExportCmdbHoldsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbHoldsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"cmdb_duplicate": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCmdbDuplicates(ctx, sqlc.ExportCmdbDuplicatesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCmdbDuplicatesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"whiteboard": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportWhiteboards(ctx, sqlc.ExportWhiteboardsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportWhiteboardsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"realtime_document": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportRealtimeDocuments(ctx, sqlc.ExportRealtimeDocumentsParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorText(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportRealtimeDocumentsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"realtime_update": {
		delta: false,
		keys:  3,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportRealtimeUpdates(ctx, sqlc.ExportRealtimeUpdatesParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorText(page.After, 1),
				AfterThird:  cursorNumber(page.After, 2),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportRealtimeUpdatesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"realtime_snapshot": {
		delta: false,
		keys:  3,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportRealtimeSnapshots(ctx, sqlc.ExportRealtimeSnapshotsParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorText(page.After, 1),
				AfterThird:  cursorNumber(page.After, 2),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportRealtimeSnapshotsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"setting_value": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportSettingValues(ctx, sqlc.ExportSettingValuesParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportSettingValuesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"custom_field_definition": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCustomFieldDefinitions(ctx, sqlc.ExportCustomFieldDefinitionsParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCustomFieldDefinitionsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"work_item": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportWorkItems(ctx, sqlc.ExportWorkItemsParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportWorkItemsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"item_label": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportItemLabels(ctx, sqlc.ExportItemLabelsParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportItemLabelsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"item_member": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportItemMembers(ctx, sqlc.ExportItemMembersParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportItemMembersRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"comment": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportComments(ctx, sqlc.ExportCommentsParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCommentsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"activity_entry": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportActivityEntries(ctx, sqlc.ExportActivityEntriesParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportActivityEntriesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"media_object": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportMediaObjects(ctx, sqlc.ExportMediaObjectsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportMediaObjectsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"item_attachment": {
		delta: false,
		keys:  2,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportItemAttachments(ctx, sqlc.ExportItemAttachmentsParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorUUID(page.After, 1),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportItemAttachmentsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"recurrence_rule": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportRecurrenceRules(ctx, sqlc.ExportRecurrenceRulesParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportRecurrenceRulesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"reminder": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportReminders(ctx, sqlc.ExportRemindersParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportRemindersRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"saved_view": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportSavedViews(ctx, sqlc.ExportSavedViewsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportSavedViewsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"template": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportTemplates(ctx, sqlc.ExportTemplatesParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportTemplatesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"jumble_entry": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportJumbleEntries(ctx, sqlc.ExportJumbleEntriesParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportJumbleEntriesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"auto_assign_policy": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAutoAssignPolicies(ctx, sqlc.ExportAutoAssignPoliciesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAutoAssignPoliciesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"automation_rule": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAutomationRules(ctx, sqlc.ExportAutomationRulesParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAutomationRulesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"webhook_subscription": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportWebhookSubscriptions(ctx, sqlc.ExportWebhookSubscriptionsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportWebhookSubscriptionsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"calendar_feed": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportCalendarFeeds(ctx, sqlc.ExportCalendarFeedsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportCalendarFeedsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"notification_preference": {
		delta: true,
		keys:  3,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportNotificationPreferences(ctx, sqlc.ExportNotificationPreferencesParams{
				Since:       optionalTimestamp(page.Since),
				AfterAt:     page.AfterAt,
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorText(page.After, 1),
				AfterThird:  cursorText(page.After, 2),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportNotificationPreferencesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"retention_policy": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportRetentionPolicies(ctx, sqlc.ExportRetentionPoliciesParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorText(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportRetentionPoliciesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"consent_record": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportConsentRecords(ctx, sqlc.ExportConsentRecordsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportConsentRecordsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"legal_hold": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportLegalHolds(ctx, sqlc.ExportLegalHoldsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportLegalHoldsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"webauthn_credential": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPasskeys(ctx, sqlc.ExportPasskeysParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPasskeysRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"private_hub_access": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportPrivateHubAccesses(ctx, sqlc.ExportPrivateHubAccessesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportPrivateHubAccessesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"container_milestone": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportContainerMilestones(ctx, sqlc.ExportContainerMilestonesParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportContainerMilestonesRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"structure_preset": {
		delta: false,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportStructurePresets(ctx, sqlc.ExportStructurePresetsParams{
				AfterID: cursorUUID(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportStructurePresetsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"set_element": {
		delta: false,
		keys:  3,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportSetElements(ctx, sqlc.ExportSetElementsParams{
				AfterID:     cursorUUID(page.After, 0),
				AfterSecond: cursorText(page.After, 1),
				AfterThird:  cursorUUID(page.After, 2),
				Batch:       page.Batch,
			})
			return convert(rows, func(r sqlc.ExportSetElementsRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
	"audit_log": {
		delta: true,
		keys:  1,
		read: func(ctx context.Context, queries *sqlc.Queries, page exportPage) ([]exportRow, error) {
			rows, err := queries.ExportAudit(ctx, sqlc.ExportAuditParams{
				Since:   optionalTimestamp(page.Since),
				AfterAt: page.AfterAt,
				AfterID: cursorNumber(page.After, 0),
				Batch:   page.Batch,
			})
			return convert(rows, func(r sqlc.ExportAuditRow) exportRow {
				return exportRow{RecordID: r.RecordID, ChangedAt: r.ChangedAt, Payload: r.Payload}
			}), err
		},
	},
}
