<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection's or hub's milestone, in its head (UC-WRK-29 check 4): the day everything in it
  // was last done. Mounted only where `showsMilestone` says so - never with celebrations off
  // (check 7). It stays when entries are added afterwards: what it says stays true.

  import type { ContainerMilestone } from '@hubtask/sync-engine';
  import { Icon } from '@hubtask/design-system/components';

  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { formatDay } from '../i18n/datetime.ts';

  interface Props {
    milestone: ContainerMilestone;
  }

  const { milestone }: Props = $props();
</script>

<span class="milestone" data-milestone>
  <Icon name="circle-check" size="sm" />{t('app.milestone.reached', { date: formatDay(milestone.reached_at, messages.locale) })}
</span>

<style>
  .milestone {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    color: var(--text-secondary);
  }
</style>
