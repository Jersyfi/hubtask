<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The strip of *People*'s four sibling lists (page-system §4.3), the same on each: a tab replaces
  // the address, so a reload or a link keeps it. Tabs without a panel - each list is its own page.
  import { Tabs } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { PEOPLE_TABS, tabPath } from './peoplepage.ts';

  const { current, onnavigate }: { current: string; onnavigate?: (path: string) => void } = $props();
</script>

<Tabs label={t('app.people.title')} selected={current} onselect={(next) => onnavigate?.(tabPath(next))} tabs={PEOPLE_TABS.map((each) => ({ id: each.id, label: t(each.code) }))} />
