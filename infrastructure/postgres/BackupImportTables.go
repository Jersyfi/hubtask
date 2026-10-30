// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// importers is one entry per entity a restore writes: the statement that writes a row, the one
// that asks whether the tenant already has it, and the one that empties the table.
//
// A table rather than ninety branches, and hand-written for the reason `exporters` is: what
// differs between the entries is a decision per entity rather than a pattern a generator could
// infer. Keyed by table name, which is the schema's vocabulary and the one the deletion journal is
// written in; the archive's own entity names stay on the other side of the port.
var importers = map[string]importer{
	"tenant": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTenant(ctx, sqlc.ImportTenantParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTenant(ctx, payload)
		},
		// No clear: the tenant row is the one a restore is standing inside. Emptying it would
		// remove the row `current_tenant_id()` names, and every policy in the schema compares
		// against that.
		clear: nil,
	},
	"account": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAccount(ctx, sqlc.ImportAccountParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAccount(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAccount(ctx)
		},
	},
	"account_group": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAccountGroup(ctx, sqlc.ImportAccountGroupParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAccountGroup(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAccountGroup(ctx)
		},
	},
	"account_group_member": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAccountGroupMember(ctx, sqlc.ImportAccountGroupMemberParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAccountGroupMember(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAccountGroupMember(ctx)
		},
	},
	"membership": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportMembership(ctx, sqlc.ImportMembershipParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsMembership(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearMembership(ctx)
		},
	},
	"custom_role": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCustomRole(ctx, sqlc.ImportCustomRoleParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCustomRole(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCustomRole(ctx)
		},
	},
	"custom_role_grant": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, _ bool) (int64, error) {
			return queries.ImportCustomRoleGrant(ctx, payload)
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCustomRoleGrant(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCustomRoleGrant(ctx)
		},
	},
	"config_revision": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, _ bool) (int64, error) {
			return queries.ImportConfigRevision(ctx, payload)
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsConfigRevision(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearConfigRevision(ctx)
		},
	},
	"config_proposal": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportConfigProposal(ctx, sqlc.ImportConfigProposalParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsConfigProposal(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearConfigProposal(ctx)
		},
	},
	"config_template": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, _ bool) (int64, error) {
			return queries.ImportConfigTemplate(ctx, payload)
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsConfigTemplate(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearConfigTemplate(ctx)
		},
	},
	"config_hub_template_use": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportConfigHubTemplateUse(ctx, sqlc.ImportConfigHubTemplateUseParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsConfigHubTemplateUse(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearConfigHubTemplateUse(ctx)
		},
	},
	"config_template_use": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportConfigTemplateUse(ctx, sqlc.ImportConfigTemplateUseParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsConfigTemplateUse(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearConfigTemplateUse(ctx)
		},
	},
	"container": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportContainer(ctx, sqlc.ImportContainerParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsContainer(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearContainer(ctx)
		},
	},
	"bucket": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportBucket(ctx, sqlc.ImportBucketParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsBucket(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearBucket(ctx)
		},
	},
	"label": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportLabel(ctx, sqlc.ImportLabelParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsLabel(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearLabel(ctx)
		},
	},
	"entry_type": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportEntryType(ctx, sqlc.ImportEntryTypeParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsEntryType(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearEntryType(ctx)
		},
	},
	"workflow": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportWorkflow(ctx, sqlc.ImportWorkflowParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsWorkflow(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearWorkflow(ctx)
		},
	},
	"workflow_scheme": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportWorkflowScheme(ctx, sqlc.ImportWorkflowSchemeParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsWorkflowScheme(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearWorkflowScheme(ctx)
		},
	},
	"business_calendar": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportBusinessCalendar(ctx, sqlc.ImportBusinessCalendarParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsBusinessCalendar(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearBusinessCalendar(ctx)
		},
	},
	"hub_business_calendar": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportHubCalendar(ctx, sqlc.ImportHubCalendarParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsHubCalendar(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearHubCalendar(ctx)
		},
	},
	"sla_policy": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportSlaPolicy(ctx, sqlc.ImportSlaPolicyParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsSlaPolicy(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearSlaPolicy(ctx)
		},
	},
	"reply_template": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportReplyTemplate(ctx, sqlc.ImportReplyTemplateParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsReplyTemplate(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearReplyTemplate(ctx)
		},
	},
	"entry_schedule": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportEntrySchedule(ctx, sqlc.ImportEntryScheduleParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsEntrySchedule(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearEntrySchedule(ctx)
		},
	},
	"plan_baseline": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPlanBaseline(ctx, sqlc.ImportPlanBaselineParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPlanBaseline(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPlanBaseline(ctx)
		},
	},
	"plan_baseline_entry": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPlanBaselineEntry(ctx, sqlc.ImportPlanBaselineEntryParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPlanBaselineEntry(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPlanBaselineEntry(ctx)
		},
	},
	"sprint": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportSprint(ctx, sqlc.ImportSprintParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsSprint(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearSprint(ctx)
		},
	},
	"sprint_entry": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportSprintEntry(ctx, sqlc.ImportSprintEntryParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsSprintEntry(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearSprintEntry(ctx)
		},
	},
	"release": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportRelease(ctx, sqlc.ImportReleaseParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsRelease(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearRelease(ctx)
		},
	},
	"release_entry": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportReleaseEntry(ctx, sqlc.ImportReleaseEntryParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsReleaseEntry(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearReleaseEntry(ctx)
		},
	},
	"entry_estimate": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportEntryEstimate(ctx, sqlc.ImportEntryEstimateParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsEntryEstimate(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearEntryEstimate(ctx)
		},
	},
	"time_work_type": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTimeWorkType(ctx, sqlc.ImportTimeWorkTypeParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTimeWorkType(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearTimeWorkType(ctx)
		},
	},
	"time_record": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTimeRecord(ctx, sqlc.ImportTimeRecordParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTimeRecord(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearTimeRecord(ctx)
		},
	},
	"time_timer": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTimeTimer(ctx, sqlc.ImportTimeTimerParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTimeTimer(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearTimeTimer(ctx)
		},
	},
	"timesheet": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTimesheet(ctx, sqlc.ImportTimesheetParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTimesheet(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearTimesheet(ctx)
		},
	},
	"time_policy": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTimePolicy(ctx, sqlc.ImportTimePolicyParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTimePolicy(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearTimePolicy(ctx)
		},
	},
	"time_capacity": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTimeCapacity(ctx, sqlc.ImportTimeCapacityParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTimeCapacity(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearTimeCapacity(ctx)
		},
	},
	"time_absence": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTimeAbsence(ctx, sqlc.ImportTimeAbsenceParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTimeAbsence(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearTimeAbsence(ctx)
		},
	},
	"time_cost_rate": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTimeCostRate(ctx, sqlc.ImportTimeCostRateParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTimeCostRate(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearTimeCostRate(ctx)
		},
	},
	"time_budget": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTimeBudget(ctx, sqlc.ImportTimeBudgetParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTimeBudget(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearTimeBudget(ctx)
		},
	},
	"goal_period": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportGoalPeriod(ctx, sqlc.ImportGoalPeriodParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsGoalPeriod(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearGoalPeriod(ctx)
		},
	},
	"goal": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportGoal(ctx, sqlc.ImportGoalParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsGoal(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearGoal(ctx)
		},
	},
	"goal_entry": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportGoalEntry(ctx, sqlc.ImportGoalEntryParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsGoalEntry(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearGoalEntry(ctx)
		},
	},
	"goal_check_in": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportGoalCheckIn(ctx, sqlc.ImportGoalCheckInParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsGoalCheckIn(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearGoalCheckIn(ctx)
		},
	},
	"portfolio": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPortfolio(ctx, sqlc.ImportPortfolioParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPortfolio(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPortfolio(ctx)
		},
	},
	"portfolio_item": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPortfolioItem(ctx, sqlc.ImportPortfolioItemParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPortfolioItem(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPortfolioItem(ctx)
		},
	},
	"portfolio_update": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPortfolioUpdate(ctx, sqlc.ImportPortfolioUpdateParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPortfolioUpdate(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPortfolioUpdate(ctx)
		},
	},
	"dashboard": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportDashboard(ctx, sqlc.ImportDashboardParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsDashboard(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearDashboard(ctx)
		},
	},
	"dashboard_digest": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportDashboardDigest(ctx, sqlc.ImportDashboardDigestParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsDashboardDigest(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearDashboardDigest(ctx)
		},
	},
	"agent_model": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAgentModel(ctx, sqlc.ImportAgentModelParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAgentModel(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAgentModel(ctx)
		},
	},
	"agent": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAgent(ctx, sqlc.ImportAgentParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAgent(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAgent(ctx)
		},
	},
	"agent_halt": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAgentHalt(ctx, sqlc.ImportAgentHaltParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAgentHalt(ctx)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAgentHalt(ctx)
		},
	},
	"agent_request": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAgentRequest(ctx, sqlc.ImportAgentRequestParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAgentRequest(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAgentRequest(ctx)
		},
	},
	"agent_job": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAgentJob(ctx, sqlc.ImportAgentJobParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAgentJob(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAgentJob(ctx)
		},
	},
	"agent_action": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAgentAction(ctx, sqlc.ImportAgentActionParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAgentAction(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAgentAction(ctx)
		},
	},
	"agent_eval_case": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAgentEvalCase(ctx, sqlc.ImportAgentEvalCaseParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAgentEvalCase(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAgentEvalCase(ctx)
		},
	},
	"agent_eval_run": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAgentEvalRun(ctx, sqlc.ImportAgentEvalRunParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAgentEvalRun(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAgentEvalRun(ctx)
		},
	},
	"ai_provenance": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAiProvenanceMark(ctx, sqlc.ImportAiProvenanceMarkParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAiProvenanceMark(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAiProvenanceMark(ctx)
		},
	},
	"approval": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportApproval(ctx, sqlc.ImportApprovalParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsApproval(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearApproval(ctx)
		},
	},
	"approval_decision": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportApprovalDecision(ctx, sqlc.ImportApprovalDecisionParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsApprovalDecision(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearApprovalDecision(ctx)
		},
	},
	"approval_delegation": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportApprovalDelegation(ctx, sqlc.ImportApprovalDelegationParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsApprovalDelegation(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearApprovalDelegation(ctx)
		},
	},
	"satisfaction_rating": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportSatisfactionRating(ctx, sqlc.ImportSatisfactionRatingParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsSatisfactionRating(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearSatisfactionRating(ctx)
		},
	},
	"entry_sla": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportEntrySla(ctx, sqlc.ImportEntrySlaParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsEntrySla(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearEntrySla(ctx)
		},
	},
	"link_type": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportLinkType(ctx, sqlc.ImportLinkTypeParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsLinkType(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearLinkType(ctx)
		},
	},
	"entry_link": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportEntryLink(ctx, sqlc.ImportEntryLinkParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsEntryLink(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearEntryLink(ctx)
		},
	},
	"page": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPage(ctx, sqlc.ImportPageParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPage(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPage(ctx)
		},
	},
	"whiteboard_media": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, _ bool) (int64, error) {
			return queries.ImportWhiteboardMedia(ctx, payload)
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsWhiteboardMedia(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearWhiteboardMedia(ctx)
		},
	},
	"whiteboard_version": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportWhiteboardVersion(ctx, sqlc.ImportWhiteboardVersionParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsWhiteboardVersion(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearWhiteboardVersion(ctx)
		},
	},
	"page_version": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPageVersion(ctx, sqlc.ImportPageVersionParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPageVersion(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPageVersion(ctx)
		},
	},
	"page_restriction": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, _ bool) (int64, error) {
			return queries.ImportPageRestriction(ctx, payload)
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPageRestriction(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPageRestriction(ctx)
		},
	},
	"page_share": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, _ bool) (int64, error) {
			return queries.ImportPageShare(ctx, payload)
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPageShare(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPageShare(ctx)
		},
	},
	"page_article": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPageArticle(ctx, sqlc.ImportPageArticleParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPageArticle(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPageArticle(ctx)
		},
	},
	"page_media": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, _ bool) (int64, error) {
			return queries.ImportPageMedia(ctx, payload)
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPageMedia(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPageMedia(ctx)
		},
	},
	"page_article_rating": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPageArticleRating(ctx, sqlc.ImportPageArticleRatingParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPageArticleRating(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPageArticleRating(ctx)
		},
	},
	"page_public_link": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPagePublicLink(ctx, sqlc.ImportPagePublicLinkParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPagePublicLink(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPagePublicLink(ctx)
		},
	},
	"personal_space": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPersonalSpace(ctx, sqlc.ImportPersonalSpaceParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPersonalSpace(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPersonalSpace(ctx)
		},
	},
	"whiteboard_comment": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportWhiteboardComment(ctx, sqlc.ImportWhiteboardCommentParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsWhiteboardComment(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearWhiteboardComment(ctx)
		},
	},
	"page_comment": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPageComment(ctx, sqlc.ImportPageCommentParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPageComment(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPageComment(ctx)
		},
	},
	"cmdb_connector": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbConnector(ctx, sqlc.ImportCmdbConnectorParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbConnector(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbConnector(ctx)
		},
	},
	"cmdb_affected": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbAffected(ctx, sqlc.ImportCmdbAffectedParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbAffected(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbAffected(ctx)
		},
	},
	"cmdb_campaign": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbCampaign(ctx, sqlc.ImportCmdbCampaignParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbCampaign(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbCampaign(ctx)
		},
	},
	"cmdb_attestation": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbAttestation(ctx, sqlc.ImportCmdbAttestationParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbAttestation(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbAttestation(ctx)
		},
	},
	"cmdb_ci_group": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbCiGroup(ctx, sqlc.ImportCmdbCiGroupParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbCiGroup(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbCiGroup(ctx)
		},
	},
	"cmdb_asset": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbAsset(ctx, sqlc.ImportCmdbAssetParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbAsset(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbAsset(ctx)
		},
	},
	"cmdb_software_alias": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbSoftwareAlias(ctx, sqlc.ImportCmdbSoftwareAliasParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbSoftwareAlias(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbSoftwareAlias(ctx)
		},
	},
	"cmdb_alert": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbAlert(ctx, sqlc.ImportCmdbAlertParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbAlert(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbAlert(ctx)
		},
	},
	"cmdb_class": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbClass(ctx, sqlc.ImportCmdbClassParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbClass(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbClass(ctx)
		},
	},
	"cmdb_relation_type": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbRelationType(ctx, sqlc.ImportCmdbRelationTypeParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbRelationType(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbRelationType(ctx)
		},
	},
	"cmdb_source": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbSource(ctx, sqlc.ImportCmdbSourceParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbSource(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbSource(ctx)
		},
	},
	"cmdb_ci": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbCi(ctx, sqlc.ImportCmdbCiParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbCi(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbCi(ctx)
		},
	},
	"cmdb_ci_identifier": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbCiIdentifier(ctx, sqlc.ImportCmdbCiIdentifierParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbCiIdentifier(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbCiIdentifier(ctx)
		},
	},
	"cmdb_relation": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbRelation(ctx, sqlc.ImportCmdbRelationParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbRelation(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbRelation(ctx)
		},
	},
	"cmdb_run": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbRun(ctx, sqlc.ImportCmdbRunParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbRun(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbRun(ctx)
		},
	},
	"cmdb_hold": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbHold(ctx, sqlc.ImportCmdbHoldParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbHold(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbHold(ctx)
		},
	},
	"cmdb_duplicate": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCmdbDuplicate(ctx, sqlc.ImportCmdbDuplicateParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCmdbDuplicate(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCmdbDuplicate(ctx)
		},
	},
	"whiteboard": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportWhiteboard(ctx, sqlc.ImportWhiteboardParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsWhiteboard(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearWhiteboard(ctx)
		},
	},
	"realtime_document": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportRealtimeDocument(ctx, sqlc.ImportRealtimeDocumentParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsRealtimeDocument(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearRealtimeDocument(ctx)
		},
	},
	"realtime_update": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportRealtimeUpdate(ctx, sqlc.ImportRealtimeUpdateParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsRealtimeUpdate(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearRealtimeUpdate(ctx)
		},
	},
	"realtime_snapshot": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportRealtimeSnapshot(ctx, sqlc.ImportRealtimeSnapshotParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsRealtimeSnapshot(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearRealtimeSnapshot(ctx)
		},
	},
	"setting_value": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportSettingValue(ctx, sqlc.ImportSettingValueParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsSettingValue(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearSettingValue(ctx)
		},
	},
	"custom_field_definition": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCustomFieldDefinition(ctx, sqlc.ImportCustomFieldDefinitionParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCustomFieldDefinition(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCustomFieldDefinition(ctx)
		},
	},
	"work_item": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportWorkItem(ctx, sqlc.ImportWorkItemParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsWorkItem(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearWorkItem(ctx)
		},
	},
	"item_label": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportItemLabel(ctx, sqlc.ImportItemLabelParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsItemLabel(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearItemLabel(ctx)
		},
	},
	"item_member": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportItemMember(ctx, sqlc.ImportItemMemberParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsItemMember(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearItemMember(ctx)
		},
	},
	"comment": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportComment(ctx, sqlc.ImportCommentParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsComment(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearComment(ctx)
		},
	},
	"activity_entry": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportActivityEntry(ctx, sqlc.ImportActivityEntryParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsActivityEntry(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearActivityEntry(ctx)
		},
	},
	"media_object": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportMediaObject(ctx, sqlc.ImportMediaObjectParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsMediaObject(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearMediaObject(ctx)
		},
	},
	"item_attachment": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportItemAttachment(ctx, sqlc.ImportItemAttachmentParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsItemAttachment(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearItemAttachment(ctx)
		},
	},
	"recurrence_rule": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportRecurrenceRule(ctx, sqlc.ImportRecurrenceRuleParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsRecurrenceRule(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearRecurrenceRule(ctx)
		},
	},
	"reminder": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportReminder(ctx, sqlc.ImportReminderParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsReminder(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearReminder(ctx)
		},
	},
	"saved_view": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportSavedView(ctx, sqlc.ImportSavedViewParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsSavedView(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearSavedView(ctx)
		},
	},
	"template": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportTemplate(ctx, sqlc.ImportTemplateParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsTemplate(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearTemplate(ctx)
		},
	},
	"jumble_entry": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportJumbleEntry(ctx, sqlc.ImportJumbleEntryParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsJumbleEntry(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearJumbleEntry(ctx)
		},
	},
	"auto_assign_policy": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAutoAssignPolicy(ctx, sqlc.ImportAutoAssignPolicyParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAutoAssignPolicy(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAutoAssignPolicy(ctx)
		},
	},
	"automation_rule": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportAutomationRule(ctx, sqlc.ImportAutomationRuleParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsAutomationRule(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearAutomationRule(ctx)
		},
	},
	"webhook_subscription": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportWebhookSubscription(ctx, sqlc.ImportWebhookSubscriptionParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsWebhookSubscription(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearWebhookSubscription(ctx)
		},
	},
	"calendar_feed": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportCalendarFeed(ctx, sqlc.ImportCalendarFeedParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsCalendarFeed(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearCalendarFeed(ctx)
		},
	},
	"notification_preference": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportNotificationPreference(ctx, sqlc.ImportNotificationPreferenceParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsNotificationPreference(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearNotificationPreference(ctx)
		},
	},
	"webauthn_credential": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPasskey(ctx, sqlc.ImportPasskeyParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPasskey(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPasskeys(ctx)
		},
	},
	"private_hub_access": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportPrivateHubAccess(ctx, sqlc.ImportPrivateHubAccessParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsPrivateHubAccess(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearPrivateHubAccess(ctx)
		},
	},
	"container_milestone": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportContainerMilestone(ctx, sqlc.ImportContainerMilestoneParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsContainerMilestone(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearContainerMilestone(ctx)
		},
	},
	"structure_preset": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportStructurePreset(ctx, sqlc.ImportStructurePresetParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsStructurePreset(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearStructurePreset(ctx)
		},
	},
	"retention_policy": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportRetentionPolicy(ctx, sqlc.ImportRetentionPolicyParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsRetentionPolicy(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearRetentionPolicy(ctx)
		},
	},
	"consent_record": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportConsentRecord(ctx, sqlc.ImportConsentRecordParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsConsentRecord(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearConsentRecord(ctx)
		},
	},
	"legal_hold": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportLegalHold(ctx, sqlc.ImportLegalHoldParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsLegalHold(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearLegalHold(ctx)
		},
	},
	"set_element": {
		write: func(ctx context.Context, queries *sqlc.Queries, payload []byte, overwrite bool) (int64, error) {
			return queries.ImportSetElement(ctx, sqlc.ImportSetElementParams{Payload: payload, Overwrite: overwrite})
		},
		holds: func(ctx context.Context, queries *sqlc.Queries, payload []byte) (bool, error) {
			return queries.HoldsSetElement(ctx, payload)
		},
		clear: func(ctx context.Context, queries *sqlc.Queries) (int64, error) {
			return queries.ClearSetElement(ctx)
		},
	},
}
