// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
)

// unindexedCascades are the foreign keys that followed a delete without an index when this test
// came: each is judged on its own - how many rows the referencing table holds against how often
// the parent is deleted, and what one more index costs every write - rather than indexed in one
// sweep. The list only shrinks; a new foreign key is never added to it.
var unindexedCascades = map[string]bool{
	"account.account_avatar_media_fkey":                                    true,
	"account_group_member.account_group_member_account_id_fkey":            true,
	"account_group_member.account_group_member_group_id_fkey":              true,
	"account_identity.account_identity_provider_id_fkey":                   true,
	"account_mfa.account_mfa_account_fkey":                                 true,
	"account_mfa.account_mfa_tenant_id_fkey":                               true,
	"account_recovery_code.account_recovery_code_account_fkey":             true,
	"account_recovery_code.account_recovery_code_tenant_id_fkey":           true,
	"ai_source.ai_source_offered_provider_id_fkey":                         true,
	"ai_suggestion.ai_suggestion_agent_id_fkey":                            true,
	"auth_pending.auth_pending_account_fkey":                               true,
	"auth_pending.auth_pending_link_provider_id_fkey":                      true,
	"auth_pending.auth_pending_session_fkey":                               true,
	"backup_run.backup_run_parent_run_id_fkey":                             true,
	"backup_run.backup_run_schedule_id_fkey":                               true,
	"backup_run.backup_run_tenant_id_fkey":                                 true,
	"backup_schedule.backup_schedule_target_id_fkey":                       true,
	"backup_schedule.backup_schedule_tenant_id_fkey":                       true,
	"backup_target.backup_target_tenant_id_fkey":                           true,
	"calendar_feed.calendar_feed_view_id_fkey":                             true,
	"calendar_push_authorization.calendar_push_authorization_account_fkey": true,
	"chat_bot.chat_bot_collection_fkey":                                    true,
	"chat_bot.chat_bot_run_as_fkey":                                        true,
	"cmdb_asset.cmdb_asset_assigned_fkey":                                  true,
	"cmdb_asset.cmdb_asset_contract_fkey":                                  true,
	"cmdb_asset.cmdb_asset_request_fkey":                                   true,
	"cmdb_asset.cmdb_asset_stockroom_fkey":                                 true,
	"cmdb_ci_group.cmdb_ci_group_service_fkey":                             true,
	"cmdb_hold.cmdb_hold_run_fkey":                                         true,
	"code_link.code_link_hook_fkey":                                        true,
	"comment.comment_parent_comment_id_fkey":                               true,
	"container.container_logo_media_fkey":                                  true,
	"file_store_authorization.file_store_authorization_account_fkey":       true,
	"goal.goal_hub_fkey":                                                   true,
	"identity_provider.identity_provider_newcomer_group_fk":                true,
	"item_attachment.item_attachment_item_id_fkey":                         true,
	"item_label.item_label_item_id_fkey":                                   true,
	"item_member.item_member_item_id_fkey":                                 true,
	"membership.membership_group_id_fkey":                                  true,
	"notification.notification_actor_id_fkey":                              true,
	"notification.notification_item_id_fkey":                               true,
	"notification.notification_private_hub_access_id_fkey":                 true,
	"notification.notification_recipient_id_fkey":                          true,
	"notification.notification_rule_id_fkey":                               true,
	"notification.notification_subscription_id_fkey":                       true,
	"oauth_code.oauth_code_grant_fkey":                                     true,
	"oauth_code.oauth_code_tenant_id_fkey":                                 true,
	"oauth_grant.oauth_grant_account_fkey":                                 true,
	"oauth_grant.oauth_grant_client_fkey":                                  true,
	"oidc_flow.oidc_flow_invited_account_fkey":                             true,
	"oidc_flow.oidc_flow_pending_fkey":                                     true,
	"oidc_flow.oidc_flow_session_fkey":                                     true,
	"oidc_flow.oidc_flow_tenant_id_fkey":                                   true,
	"page.page_owner_fkey":                                                 true,
	"page_article.page_article_published_by_fkey":                          true,
	"page_article_rating.page_article_rating_account_fkey":                 true,
	"page_comment.page_comment_author_fkey":                                true,
	"page_comment.page_comment_parent_fkey":                                true,
	"page_public_link.page_public_link_created_by_fkey":                    true,
	"page_restriction.page_restriction_account_fkey":                       true,
	"page_restriction.page_restriction_granted_by_fkey":                    true,
	"page_share.page_share_granted_by_fkey":                                true,
	"page_version.page_version_author_fkey":                                true,
	"personal_space.personal_space_hub_fkey":                               true,
	"portfolio_item.portfolio_item_container_fkey":                         true,
	"portfolio_public_link.portfolio_public_link_created_by_fkey":          true,
	"privacy_incident.privacy_incident_tenant_id_fkey":                     true,
	"private_hub_access.private_hub_access_opened_by_fkey":                 true,
	"public_form.public_form_created_by_fkey":                              true,
	"realtime_update.realtime_update_author_fkey":                          true,
	"recurrence_rule.recurrence_rule_source_item_id_fkey":                  true,
	"recurrence_rule.recurrence_rule_tenant_id_fkey":                       true,
	"reminder.reminder_item_id_fkey":                                       true,
	"rule_occurrence.rule_occurrence_item_id_fkey":                         true,
	"session.session_account_fkey":                                         true,
	"session.session_grant_fkey":                                           true,
	"session.session_viewing_as_fkey":                                      true,
	"session_refresh_token.session_refresh_token_session_fkey":             true,
	"session_refresh_token.session_refresh_token_tenant_id_fkey":           true,
	"structure_preset.structure_preset_owner_fkey":                         true,
	"whiteboard_comment.whiteboard_comment_author_fkey":                    true,
	"whiteboard_comment.whiteboard_comment_parent_fkey":                    true,
	"whiteboard_version.whiteboard_version_author_fkey":                    true,
	"work_item.work_item_bucket_id_fkey":                                   true,
}

// Every foreign key the database follows on a delete - CASCADE, SET NULL, SET DEFAULT - has an
// index whose leading columns are its own. Without one, each deleted parent row scans the
// referencing table's whole tenant: a purge of 500 entries became 500 scans of entry_reference
// and overran the worker's statement timeout (known-traps.md, "A cascade without an index").
func TestEveryCascadingForeignKeyHasAnIndex(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)

	rows, err := admin.Query(ctx, `
		SELECT c.conrelid::regclass::text, c.conname
		FROM pg_constraint c
		JOIN pg_namespace n ON n.oid = c.connamespace AND n.nspname = 'public'
		WHERE c.contype = 'f' AND c.confdeltype IN ('c', 'n', 'd')
		  -- A partition is checked through its parent's index.
		  AND NOT EXISTS (SELECT 1 FROM pg_inherits i WHERE i.inhrelid = c.conrelid)
		  AND NOT EXISTS (
		    SELECT 1 FROM pg_index x
		    WHERE x.indrelid = c.conrelid AND x.indisvalid
		      AND array_length(c.conkey, 1) <= x.indnkeyatts
		      AND (SELECT array_agg(k ORDER BY k) FROM unnest(c.conkey) AS k)
		        = (SELECT array_agg(k ORDER BY k)
		           FROM unnest((x.indkey::int2[])[0:array_length(c.conkey, 1) - 1]) AS k)
		  )
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatalf("reading the foreign keys: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var table, constraint string
		if err := rows.Scan(&table, &constraint); err != nil {
			t.Fatalf("reading the foreign keys: %v", err)
		}
		if unindexedCascades[table+"."+constraint] {
			continue
		}
		t.Errorf("%s.%s follows a delete without an index on its columns: add one, built "+
			"CONCURRENTLY in a migration without a transaction", table, constraint)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the foreign keys: %v", err)
	}
}
