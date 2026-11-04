<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Templates* at the installation (user-experience.md §8.4, UC-INS-18): which templates the
  // workspaces are offered, withheld or held to as a default per plan - a row of its own, as the
  // workspace's *Templates* is, rather than the end of the settings screen.

  import { untrack } from 'svelte';

  import { PageHeader, Stack } from '@hubtask/design-system/components';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import InstallationPresets from '../lib/instance/InstallationPresets.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  const { onnavigate }: { onnavigate: (path: string) => void } = $props();

  // An installation configured from a file in enforce mode offers no write here either.
  $effect(() => {
    if (!instance.isElevated) return;
    return untrack(() => instance.openSettings());
  });
  const isEnforced = $derived(instance.settings.status === 'ready' && instance.settings.data.is_enforced_from_file === true);

  $effect(() => page.entitle(t('app.instance.templates')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.templates')} isTitleInBar={viewport.isCompact} breadcrumb={installationTrail({ id: 'screen', label: t('app.instance.templates') })} />
  <InstanceGate onleave={() => onnavigate('/')}>
    <!-- A level a single installation does not have (UC-INS-18 check 4): its column has no row. -->
    {#if manifest.isMultiTenant}<InstallationPresets {isEnforced} />{/if}
  </InstanceGate>
</Stack>
