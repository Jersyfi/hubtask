<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The head sentence of a hub's *Releases* and *Sprints* (scenes planrel 1, plansprints): what is
  // coming, the next release and its day in bold, what went out, and the sprint running.
  import type { Release, Sprint } from '@hubtask/sync-engine';

  import { releaseGroups } from '../data/releases.svelte.ts';
  import { formatPlainDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { sprintDaysLeft } from '../today.ts';

  interface Props {
    releases: readonly Release[];
    running?: Pick<Sprint, 'name' | 'end_on'>;
    today: string;
  }

  const { releases, running, today }: Props = $props();

  const groups = $derived(releaseGroups(releases, today));
  const next = $derived(groups.planned[0]);
</script>

<span data-head-figure>{#if groups.planned.length > 0}{t('app.releases.figure_planned', { count: String(groups.planned.length) })}{/if}{#if next}{groups.planned.length > 0 ? ' · ' : ''}<b>{t(groups.late.has(next.id) ? 'app.releases.figure_late' : 'app.releases.figure_next', { name: next.name, day: formatPlainDay(next.release_on, messages.locale) })}</b>{/if}{#if groups.released.length > 0}{' · '}{t('app.releases.figure_released', { count: String(groups.released.length) })}{/if}{#if running}{' · '}{t('app.releases.figure_sprint', { name: running.name, count: String(sprintDaysLeft(running.end_on, today)) })}{/if}</span>
