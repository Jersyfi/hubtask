// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A visible help text is two lines at most (design-system.md §4, "Help text"): a hint, a row's
// help, a section's or a page's introduction over about 120 characters in English keeps what is
// needed now and moves its why behind an ⓘ, as a `…_more` text. The texts left longer stand in
// `hints.json` with a one-word reason - a description is never moved (the owner's condition 1) -
// and that list only shrinks: a text that became short enough leaves it, and none joins it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const catalogue = JSON.parse(readFileSync(join(here, '..', '..', '..', 'locales', 'en.json'), 'utf8'));
const exceptions = JSON.parse(readFileSync(join(here, 'hints.json'), 'utf8'));

/** Two lines of a hint, in English characters. */
const LIMIT = 120;
/**
 * The texts the list held when it was written. It may only lose one: a text that leaves `hints.json`
 * leaves here too, and a code that is not here cannot join, whatever the count.
 */
const LISTED = new Set([
  'app.access.intro',
  'app.address.own.intro',
  'app.agents.change_hint',
  'app.agents.intro',
  'app.ai.arrival_hint',
  'app.ai.consent_hint',
  'app.ai.embedding_model_hint',
  'app.ai.kind_hint',
  'app.ai.opt_out_basis_hint',
  'app.apps.is_confidential_hint',
  'app.apps.secret_hint',
  'app.apps.uris_hint',
  'app.archive.intro',
  'app.assets.empty_hint',
  'app.assets.import_hint',
  'app.assets.intake_secret_hint',
  'app.audit.anchoring_intro',
  'app.audit.verify_intro',
  'app.backup.download_intro',
  'app.backup.download_passphrase_hint',
  'app.backup.encryption_hint',
  'app.backup.guided_description',
  'app.backup.operator_keeps_description',
  'app.backup.target_host_key_hint',
  'app.care.compare_hint',
  'app.changes.intro',
  'app.chatbots.intake_hint',
  'app.chatbots.intro',
  'app.chatbots.post_hook_hint',
  'app.chatbots.room_hint',
  'app.chatbots.widget_hint',
  'app.cmdb_admin.aliases_hint',
  'app.cmdb_admin.rules_hint',
  'app.code.hooks_intro',
  'app.confirm_address.intro',
  'app.dashboards.measure_hint',
  'app.desk.hub_calendars_hint',
  'app.desk.hub_policies_hint',
  'app.desk.itil_defaults_hint',
  'app.desk.policy_group_hint',
  'app.emergency_notice.intro_many',
  'app.emergency_notice.intro_many_owners',
  'app.emergency_notice.intro_one',
  'app.entries.categories_hint',
  'app.entries.language_hint',
  'app.entries.language_tag_hint',
  'app.entry_forms.intro',
  'app.fields.hidden_hint',
  'app.fields.restricted_hint',
  'app.fields.scope_hint',
  'app.filestores.intro',
  'app.filestores.share_hint',
  'app.flow.composer_expert_hint',
  'app.flow.gate_hint',
  'app.flow.probe_intro',
  'app.flow.relative_hint',
  'app.flow.stop_hint',
  'app.identity_provider.directories_hint_tid',
  'app.identity_provider.directories_narrow_hint',
  'app.identity_provider.directories_shared_hint',
  'app.identity_provider.domains_hint',
  'app.identity_provider.intro',
  'app.identity_provider.newcomers_hint',
  'app.identity_provider.provisioning_hint',
  'app.import.kind_CONFLUENCE_hint',
  'app.import.kind_CSV_hint',
  'app.import.kind_DOCX_hint',
  'app.import.kind_HTML_hint',
  'app.import.kind_JIRA_hint',
  'app.import.kind_MARKDOWN_hint',
  'app.import.kind_MICROSOFT_TODO_hint',
  'app.import.kind_XLSX_hint',
  'app.import.mapping_hint',
  'app.instance.ai_form_intro',
  'app.instance.dead_letters_intro',
  'app.instance.encryption_intro',
  'app.instance.export_intro',
  'app.instance.file_intro',
  'app.instance.host_hint',
  'app.instance.journal_intro_single',
  'app.instance.locked_intro',
  'app.instance.operator_service_intro',
  'app.instance.own_domains_intro',
  'app.instance.plan_features_hint',
  'app.instance.plan_short_name_hint',
  'app.instance.provider_form_intro',
  'app.instance.provider_withdraw_intro',
  'app.instance.provider_withdraw_now_intro',
  'app.instance.quotas_intro',
  'app.instance.settings_edit_hint',
  'app.instance.targets_intro',
  'app.instance.usage_off_description',
  'app.jumble.intake_hint',
  'app.jumble.intake_minted_hint',
  'app.jumble.intro',
  'app.mail.intro',
  'app.mail.mailboxes_hint',
  'app.mail.oauth_custom_hint',
  'app.merge.hint',
  'app.mfa.disable_hint',
  'app.mfa.recovery_hint',
  'app.mfa.replace_intro',
  'app.mfa.secret_hint',
  'app.modules.intro',
  'app.modules.off_wf_convert_hint',
  'app.modules.off_wf_pause_hint',
  'app.modules.on_hubs_hint',
  'app.passkeys.recovery_hint',
  'app.password.connect_intro',
  'app.people.address_hint_other',
  'app.people.address_hint_own',
  'app.people.invite_hint',
  'app.people.invite_hint_link',
  'app.people.invite_hint_neutral',
  'app.people.invite_link_hint',
  'app.people.invite_where_hint',
  'app.people.managed_hint',
  'app.people.managed_start_password_hint',
  'app.planning.take_baseline_hint',
  'app.planning.workspace_tolerance_hint',
  'app.policies.completion_hint',
  'app.privacy.deadline_hint',
  'app.privacy.preview_intro',
  'app.private_hubs.allowed_hint',
  'app.private_hubs.intro',
  'app.profile.address_intro',
  'app.profile.ai_keep_out_hint',
  'app.profile.celebrations_hint',
  'app.profile.device_hint',
  'app.profile.motion_hint',
  'app.profile.picture_hint',
  'app.pushes.intro',
  'app.redeem.provider_intro',
  'app.releases.none_hint',
  'app.retention.effective_intro',
  'app.roadmap.none_hint',
  'app.roles.intro',
  'app.rules.conditions_hint',
  'app.rules.mine_intro',
  'app.rules.runs_as_hint',
  'app.rules.stop_hint',
  'app.searchline.hint',
  'app.setup.code_hint',
  'app.sign_in.recovery_hint',
  'app.signin_settings.managed_accounts_hint',
  'app.signin_settings.own_branding_hint',
  'app.signin_settings.terms_version_hint',
  'app.signin_settings.way_passkey_hint',
  'app.structure.narrowing.flatten_hint',
  'app.tokens.installation_provisioning_hint',
  'app.tokens.minted_hint',
  'app.voice.alexa_hint',
  'app.voice.intro',
  'app.voice.invocation_hint',
  'app.voice.step_endpoint_hint',
  'app.voice.step_linking_hint',
  'app.voice.step_model_hint',
  'app.voice.step_skill_hint',
  'app.voice.step_test_hint',
  'app.webhooks.condition_hint',
  'app.webhooks.scope_hint',
  'app.webhooks.secret_hint',
  'app.whiteboards.file_hint',
  'app.whiteboards.hint_write',
  'app.workspace.edit_preset_hint',
  'app.workspace.private_hint',
  'voice.help',
]);
/** What a text left long may be. */
const REASONS = new Set(['description', 'needed', 'short', 'option', 'status', 'dialog', 'empty', 'unused', 'spoken', 'unseen']);

/** A code whose last part names visible help: `…_hint`, `…_help`, `…_intro`, `…_description` and their variants. */
const isHelp = (code) =>
  !code.startsWith('email.') && !code.endsWith('_more') && /(^|_)(hint|help|intro|description)(_|$)/.test(code.split('.').pop() ?? '');

test('every visible help text over two lines is on the exception list, with its reason', () => {
  const long = Object.keys(catalogue).filter((code) => isHelp(code) && catalogue[code].length > LIMIT);
  const unlisted = long.filter((code) => !(code in exceptions));
  assert.deepEqual(unlisted, [], `longer than ${LIMIT} characters: keep what is needed now and move the why to a …_more text behind an ⓘ`);
});

test('the exception list only shrinks', () => {
  const stale = Object.keys(exceptions).filter((code) => !(code in catalogue) || catalogue[code].length <= LIMIT || !isHelp(code));
  assert.deepEqual(stale, [], 'no longer long, or gone: remove it from hints.json');
  const joined = Object.keys(exceptions).filter((code) => !LISTED.has(code));
  assert.deepEqual(joined, [], 'the list only shrinks: keep what is needed now and move the why to a …_more text behind an ⓘ');
  const left = [...LISTED].filter((code) => !(code in exceptions));
  assert.deepEqual(left, [], 'gone from hints.json: remove it from LISTED too, so it cannot come back');
  const unreasoned = Object.entries(exceptions).filter(([, reason]) => !REASONS.has(reason));
  assert.deepEqual(unreasoned, [], `a reason is one of ${[...REASONS].join(', ')}`);
});

test('every text behind an ⓘ belongs to a visible one and is three sentences at most', () => {
  const more = Object.keys(catalogue).filter((code) => code.endsWith('_more') && isHelp(code.slice(0, -'_more'.length)));
  assert.ok(more.length > 0);
  for (const code of more) {
    assert.ok(code.slice(0, -'_more'.length) in catalogue, `${code} has no visible text it belongs to`);
    const sentences = catalogue[code].split(/[.!?]["”]?(?:\s+|$)/).filter((part) => part.trim() !== '');
    assert.ok(sentences.length <= 3, `${code} is ${sentences.length} sentences: three at most`);
  }
});

test('a list or a log page with more about its head lets its introduction end with the ⓘ', () => {
  // Where the bar carries the title, the page hands the ⓘ to the introduction: an introduction that
  // does not render it loses the ⓘ on a phone.
  const src = join(here, '..', 'src');
  const pages = readdirSync(src, { recursive: true }).filter((file) => file.endsWith('.svelte')).map((file) => [file, readFileSync(join(src, file), 'utf8')])
    .filter(([, text]) => /<(ListPage|LogPage)\b/.test(text) && /^[^/]*\bmore: /m.test(text) && /\{#snippet intro\(/.test(text));
  assert.ok(pages.length > 0);
  const silent = pages.filter(([, text]) => !/\{#snippet intro\(tip\)\}/.test(text) || !/\{@render tip\?\.\(\)\}/.test(text)).map(([file]) => file);
  assert.deepEqual(silent, [], 'take the ⓘ as `{#snippet intro(tip)}` and end the sentence with `{@render tip?.()}`');
});
