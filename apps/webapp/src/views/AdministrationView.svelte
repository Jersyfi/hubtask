<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The administration area's front door.
  //
  // **Reaching this screen at all is the server's decision.** The frame offers the entry only
  // where `GET /quotas` succeeds, which is the area's condition as the server enforces it —
  // `STRUCTURE` or the auditor's `READ_CONFIGURATION`. Somebody who types the address anyway finds
  // the screens themselves refusing, in the server's own words, which is where a refusal belongs.
  //
  // **Every screen under here is listed, not hidden by permission.** A screen a reader can open
  // and be refused by is honest; a list that quietly shortened itself would leave somebody unable
  // to tell whether a thing exists at all. The individual refusals are each screen's to render.
  //
  // The list grows as this milestone adds screens — people, tokens, jumble, automation, backup,
  // retention, audit. Nothing here is a route table: it links to `lib/routes.ts`'s addresses.

  import { ListRow, Stack } from '@hubtask/design-system/components';

  import { t } from '../lib/i18n/i18n.svelte.ts';

  const screens = [
    { path: '/administration/workspace', label: 'app.admin.workspace', hint: 'app.admin.workspace_hint' },
    { path: '/administration/people', label: 'app.admin.people', hint: 'app.admin.people_hint' },
    { path: '/administration/groups', label: 'app.admin.groups', hint: 'app.admin.groups_hint' },
    { path: '/administration/permissions', label: 'app.admin.permissions', hint: 'app.admin.permissions_hint' },
    {
      path: '/administration/service-accounts',
      label: 'app.admin.service_accounts',
      hint: 'app.admin.service_accounts_hint',
    },
    { path: '/administration/apps', label: 'app.admin.apps', hint: 'app.admin.apps_hint' },
    { path: '/administration/rules', label: 'app.admin.rules', hint: 'app.admin.rules_hint' },
    { path: '/administration/runs', label: 'app.admin.runs', hint: 'app.admin.runs_hint' },
    { path: '/administration/webhooks', label: 'app.admin.webhooks', hint: 'app.admin.webhooks_hint' },
    { path: '/administration/quotas', label: 'app.admin.quotas', hint: 'app.admin.quotas_hint' },
    { path: '/administration/backup', label: 'app.admin.backup', hint: 'app.admin.backup_hint' },
    { path: '/administration/retention', label: 'app.admin.retention', hint: 'app.admin.retention_hint' },
    {
      path: '/administration/identity-provider',
      label: 'app.admin.identity_provider',
      hint: 'app.admin.identity_provider_hint',
    },
  ];
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.admin.title')}</h1>
    <p class="quiet">{t('app.admin.intro')}</p>

    <Stack gap="100">
      {#each screens as screen (screen.path)}
        <!-- An `href` rather than a handler: these are addresses, and a row that was only a button
             could not be opened in a second tab, bookmarked, or read out as a link. The frame's
             own interception turns the click into a navigation without a reload. -->
        <ListRow href={screen.path}>
          <Stack gap="025">
            <span class="name">{t(screen.label)}</span>
            <span class="hint">{t(screen.hint)}</span>
          </Stack>
        </ListRow>
      {/each}
    </Stack>
  </Stack>
</div>

<style>
  /* Rule 4: a column that grows with its text and stops before it becomes a line nobody can read. */
  .screen { max-width: 60ch; }

  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .quiet { margin: 0; color: var(--text-secondary); }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); }

  .hint { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
