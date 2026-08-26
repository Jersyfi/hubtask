// SPDX-License-Identifier: BUSL-1.1
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
