<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The strip of *People*'s four sibling lists (page-system §4.3), the same on each: a tab replaces
  // the address, so a reload or a link keeps it. Tabs without a panel - each list is its own page.
  // People, Groups and Roles carry their counts once read; *Who may do what* is not a list of one
  // kind and has none.
  import { untrack } from 'svelte';

  import { Tabs } from '@hubtask/design-system/components';

  import { groups } from '../data/groups.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { roles } from '../data/roles.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { PEOPLE_TABS, tabPath } from './peoplepage.ts';

  const { current, onnavigate }: { current: string; onnavigate?: (path: string) => void } = $props();

  const TENANT = { scopeType: 'TENANT' } as const;

  $effect(() =>
    untrack(() => {
      const closers = [groups.open(), roles.open()];
      return () => closers.forEach((close) => close());
    }),
  );

  const counts = $derived<Record<string, number | undefined>>({
    people: (() => {
      const holders = people.holders({}, TENANT);
      return holders.length === 0 ? undefined : holders.filter((holder) => holder.accountId).length;
    })(),
    groups: groups.state.status === 'ready' ? groups.all.length : undefined,
    roles: roles.catalogue ? roles.catalogue.shipped.length + roles.catalogue.custom.length : undefined,
  });
</script>

<Tabs
  label={t('app.people.title')}
  selected={current}
  onselect={(next) => onnavigate?.(tabPath(next))}
  tabs={PEOPLE_TABS.map((each) => ({ id: each.id, label: t(each.code), count: counts[each.id] }))}
/>
