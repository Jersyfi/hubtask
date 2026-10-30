<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who sees a dashboard (UC-GOL-04 checks 3 and 14): its owner alone, everybody who reads its
  // place, or the members of chosen groups who read it - each with the numbers their own reading
  // allows. The groups are read only once the reader picks them.

  import { CheckboxGroup, Select, Stack } from '@hubtask/design-system/components';
  import { untrack } from 'svelte';

  import { groups } from '../data/groups.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  export type Sharing = 'PRIVATE' | 'SCOPE' | 'GROUPS';

  interface Props {
    sharing: Sharing;
    groupIds: string[];
  }

  let { sharing = $bindable(), groupIds = $bindable() }: Props = $props();

  $effect(() => {
    if (sharing === 'GROUPS') return untrack(() => groups.open());
  });
</script>

<Stack gap="150">
  <Select label={t('app.dashboards.col_sharing')} hint={t('app.dashboards.sharing_hint')} bind:value={sharing}
    options={(['PRIVATE', 'SCOPE', 'GROUPS'] as const).map((value) => ({ value, label: t(`app.dashboards.sharing_${value}`) }))} />
  {#if sharing === 'GROUPS'}
    <CheckboxGroup label={t('app.dashboards.groups')} hint={groups.all.length === 0 ? t('app.dashboards.groups_none') : undefined}
      error={groupIds.length === 0 ? t('app.dashboards.groups_needed') : undefined}
      options={groups.all.map((group) => ({ value: group.id, label: group.name }))}
      selected={groupIds} onChange={(next) => { groupIds = [...next]; }} />
  {/if}
</Stack>
