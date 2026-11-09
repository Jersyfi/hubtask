<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The strip of *Machines and apps*' two sibling lists (page-system §4.3): a tab replaces the
  // address, so a reload or a link keeps it. Tabs without a panel - each list is its own page.
  import { Tabs } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { MACHINE_TABS, machinesTabPath } from './machinespage.ts';

  const { current, onnavigate }: { current: string; onnavigate?: (path: string) => void } = $props();
</script>

<Tabs label={t('app.machines.title')} selected={current} onselect={(next) => onnavigate?.(machinesTabPath(next))} tabs={MACHINE_TABS.map((each) => ({ id: each.id, label: t(each.code) }))} />
