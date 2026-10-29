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
