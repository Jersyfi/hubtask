<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Modules* under *How we work* (UC-INS-21): the workspace's module switches, and the plans'
  // time tolerance the workspace decides (UC-PLN-04 check 7) while Planning is on - a settings page
  // of one section, one setting row per module (page-system §9.1).

  import { SettingsLayout, SettingsSection } from '@hubtask/design-system/components';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { isModuleOn } from '../lib/frame/moduleroute.ts';
  import ModuleSwitches from '../lib/workspace/ModuleSwitches.svelte';
  import WorkspaceTolerance from '../lib/workspace/WorkspaceTolerance.svelte';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';

  interface Props {
    onnavigate?: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  const title = $derived(t('app.modules.title'));
  let isFileOpen = $state(false);
  const fileAction = $derived(viewAsFileAction(() => (isFileOpen = true)));

  $effect(() => page.entitle(title));
</script>

<SettingsLayout
  head={{
    title,
    breadcrumb: {
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'modules', label: title },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    },
    secondary: fileAction ? [fileAction] : [],
  }}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: [{ id: 'modules', label: title, href: '/administration/modules' }], current: 'modules' }}
  view="detail"
  back={{ label: t('app.admin.title'), onback: () => onnavigate?.('/administration') }}
>
  <!-- Switches that act at once, so no save bar (PS-10). -->
  <SettingsSection {title} description={t('app.modules.intro')}>
    <ModuleSwitches {onnavigate} bind:isFileOpen isTriggerHidden={fileAction !== undefined} />
    {#if isModuleOn('PLANNING', manifest.value?.modules_on)}<WorkspaceTolerance />{/if}
  </SettingsSection>
</SettingsLayout>
