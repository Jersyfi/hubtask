<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Modules* under *How we work* (UC-INS-21): the workspace's module switches.

  import { PageHeader, Stack } from '@hubtask/design-system/components';

  import ModuleSwitches from '../lib/workspace/ModuleSwitches.svelte';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  $effect(() => page.entitle(t('app.modules.title')));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.modules.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'modules', label: t('app.modules.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  <p class="quiet">{t('app.modules.intro')}</p>

  <ModuleSwitches />
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
