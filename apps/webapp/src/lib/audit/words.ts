// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * An audit entry's action in words (page-system §7.6, PS-15): *Approval read*, *Role changed*. The
 * action code stays under *Details for support*; on the row the thing it was done to - the
 * target's kind, else the code's first part - and the verb its last part names.
 */

/** What the words are read from: the catalogue, or a stand-in in a test. */
export interface Catalogue {
  t: (code: string, params?: Record<string, string>) => string;
  has: (code: string) => boolean;
}

/** The verb an action's last part names: `token_created` → `created`, `signed_in` as a whole. */
function verbOf(action: string, words: Catalogue): string | undefined {
  const last = action.slice(action.lastIndexOf('.') + 1);
  for (const candidate of [last, last.split('_').slice(-2).join('_'), last.slice(last.lastIndexOf('_') + 1)]) {
    if (words.has(`app.audit.verb.${candidate}`)) return words.t(`app.audit.verb.${candidate}`);
  }
  return undefined;
}

/** The kind of thing, by the target's type or else the action's first part. */
export function thingWord(type: string | null | undefined, action: string, words: Catalogue): string {
  for (const candidate of [type, action.split('.')[0]]) {
    if (candidate && words.has(`app.audit.thing.${candidate}`)) return words.t(`app.audit.thing.${candidate}`);
  }
  return words.t('app.audit.thing_other');
}

/** The action as a person reads it. */
export function actionWords(action: string, type: string | null | undefined, words: Catalogue): string {
  const thing = thingWord(type, action, words);
  const verb = verbOf(action, words);
  return verb ? words.t('app.audit.what', { thing, verb }) : words.t('app.audit.what_other', { thing });
}

/** The kinds of thing the trail records (`docs/audit/event-matrix.md`), for the filter's choice. */
export const TARGET_TYPES: readonly string[] = [
  'access_token', 'account', 'agent', 'agent_model', 'agent_policy', 'agent_request',
  'ai_budget', 'ai_provider', 'ai_source', 'ai_suggestion', 'approval', 'approval_delegation',
  'audit_anchoring', 'audit_trail', 'automation_rule', 'automation_run', 'backup_run',
  'backup_schedule', 'backup_target', 'bucket', 'business_calendar', 'calendar_feed',
  'calendar_push', 'chat_bot', 'cmdb_alert', 'cmdb_asset', 'cmdb_campaign', 'cmdb_ci',
  'cmdb_ci_group', 'cmdb_class', 'cmdb_connector', 'cmdb_relation', 'cmdb_relation_type',
  'cmdb_run', 'cmdb_software_alias', 'cmdb_source', 'code_hook', 'comment', 'configuration',
  'consent_record', 'container', 'custom_field', 'custom_role', 'dashboard',
  'data_subject_request', 'desk', 'entry_link', 'entry_requester', 'entry_sla', 'entry_type',
  'file_link', 'file_store_connection', 'glance', 'goal', 'goal_period', 'group', 'http_request',
  'hub_business_calendar', 'identity_provider', 'import', 'installation', 'installation_preset',
  'instance', 'item', 'job', 'jumble_entry', 'label', 'ldap_directory', 'legal_hold',
  'link_type', 'mail_settings', 'mailbox', 'media', 'membership', 'module', 'oauth_client',
  'oauth_grant', 'page', 'page_article', 'page_comment', 'page_public_links', 'page_space',
  'plan', 'plan_baseline', 'plan_tolerance', 'platform_subscription', 'portfolio', 'preset',
  'public_form', 'public_forms', 'public_roadmaps', 'quota', 'recurrence', 'reference',
  'release', 'reminder', 'reply_template', 'request', 'requester_organisation', 'restore_run',
  'retention_policy', 'saved_view', 'search', 'session', 'sla_policy', 'sprint', 'sync_device',
  'template', 'tenant', 'time_absence', 'time_billing_rate', 'time_budget', 'time_capacity',
  'time_cost_rate', 'time_policy', 'time_record', 'time_timer', 'time_work_type', 'timesheet',
  'trash', 'trigger', 'voice_settings', 'webhook_delivery', 'webhook_subscription', 'whiteboard',
  'whiteboard_comment', 'work_item', 'workflow', 'workspace'
];
