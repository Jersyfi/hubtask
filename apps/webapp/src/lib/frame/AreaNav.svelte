<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An area's own column - the administration, Your settings, the installation (ADR-0063
  // decision 7, ADR-0065 decision 3, ADR-0070 §5) - drawn with the hub level's parts (the
  // navigation concept §2.4, §2.6): the way back alone in the head as *‹ All hubs* is, the head
  // group's rows above a hairline as a hub's tools are, then each group folding as *Collections*
  // does, rows of one height with a figure where a screen has a state to act on.
  //
  // It replaces the workspace's levels rather than joining them: the reader is in a place of its
  // own. On a phone the same groups are a step of the Browse stack (`size="touch"`), the way back
  // then being the bar's.
  //
  // It knows no area: each hands it its own `SectionGroup[]`; who reaches it is the frame's.
  import { untrack } from 'svelte';

  import { NavGroup, NavItem, NavLevel, VisuallyHidden } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import type { SectionGroup, SectionRow } from '../navigation.ts';
  import { AREA_STEP_PATHS } from '../phone.ts';
  import { page } from './page.svelte.ts';

  interface Props {
    /** The area's name, for the landmark. Resolved text. */
    label: string;
    groups: readonly SectionGroup[];
    /** A short figure at a row's end, by the row's id: what asks for action there (user-experience.md §7.10). */
    figures?: Readonly<Record<string, string>>;
    /** The row the reader is on, by its id. */
    current?: string;
    /** Folded to its marks, from `expanded` up. */
    isRail?: boolean;
    /** The phone's Browse step: cards of touch rows, each one level down; the way back is the bar's. */
    size?: 'md' | 'touch';
    onnavigate: (path: string) => void;
  }

  const { label, groups, figures = {}, current, isRail = false, size = 'md', onnavigate }: Props = $props();

  const head = $derived(groups.find((group) => !group.code));
  const way = $derived(head?.rows.find((row) => row.id === 'back'));
  const heads = $derived(head?.rows.filter((row) => row.id !== 'back') ?? []);
  const parts = $derived(groups.filter((group) => group.code));
  const rows = $derived(groups.flatMap((group) => group.rows).filter((row) => row.id !== 'back'));

  // Every group open at first; folding one is this visit's, as a hub's parts fold.
  let open = $state<Record<string, boolean>>({});
  let moreOpen = $state<Record<string, boolean>>({});
  // The reader standing on a folded row finds its *More* open.
  $effect(() => {
    const holding = groups.find((group) => group.rows.some((row) => row.isFolded && row.id === current));
    if (holding && !untrack(() => moreOpen)[holding.id]) moreOpen = { ...untrack(() => moreOpen), [holding.id]: true };
  });

  // On a phone the installation, the administration's last group, opens as its own step.
  const pathOf = (row: SectionRow) => (size === 'touch' && row.path === '/instance' && row.routes.length === 0 ? AREA_STEP_PATHS.instance : row.path);
  const go = (row: SectionRow) => () => onnavigate(pathOf(row));
  // The step names itself for the bar.
  $effect(() => {
    if (size !== 'touch') return;
    return page.entitle(label);
  });
  const hidden = (group: SectionGroup) =>
    group.rows.filter((row) => row.isFolded).map((row) => ({ id: row.id, label: t(row.code), icon: row.icon, href: pathOf(row) }));
  const goId = (id: string) => {
    const found = rows.find((row) => row.id === id);
    if (found) onnavigate(pathOf(found));
  };
</script>

{#snippet row(each: SectionRow)}
  <NavItem
    label={t(each.code)}
    icon={each.icon}
    href={pathOf(each)}
    figure={figures[each.id]}
    isFigureUrgent={figures[each.id] !== undefined}
    isCurrent={size === 'md' && each.id === current}
    {isRail}
    {size}
    isDrillDown={size === 'touch'}
    onnavigate={go(each)}
  />
{/snippet}

{#snippet headRows()}
  {#each heads as each (each.id)}{@render row(each)}{/each}
{/snippet}

<nav class="area" aria-label={label} data-size={size} data-page-kind={size === 'touch' ? 'browse' : undefined}>
  {#if size === 'touch'}
    <VisuallyHidden as="h1">{label}</VisuallyHidden>
    {#if heads.length > 0}
      <ul class="card-list">{#each heads as each (each.id)}{@render row(each)}{/each}</ul>
    {/if}
    {#each parts as group (group.id)}
      {@const shown = group.rows.filter((each) => !each.isFolded)}
      <NavGroup label={t(group.code ?? '')} size="touch" hiddenRows={hidden(group)} hiddenLabel={t('app.admin.more')}
        bind:isHiddenOpen={() => moreOpen[group.id] ?? false, (next) => (moreOpen = { ...moreOpen, [group.id]: next })}
        onnavigate={goId}>
        {#each shown as each (each.id)}{@render row(each)}{/each}
      </NavGroup>
    {/each}
  {:else if isRail}
    <ul class="rail">
      {#each rows.filter((each) => !each.isFolded || each.id === current) as each (each.id)}{@render row(each)}{/each}
    </ul>
  {:else}
    <NavLevel levelKey={label} {label} tools={heads.length > 0 ? headRows : undefined} back={way ? { label: t(way.code), href: way.path, onback: go(way) } : undefined}>
      {#each parts as group (group.id)}
        {@const shown = group.rows.filter((each) => !each.isFolded)}
        {@const name = t(group.code ?? '')}
        <NavGroup label={name} collapseLabel={t('app.nav.fold', { name })}
          bind:isOpen={() => open[group.id] ?? true, (next) => (open = { ...open, [group.id]: next })}
          hiddenRows={hidden(group)} hiddenLabel={t('app.admin.more')} {current}
          bind:isHiddenOpen={() => moreOpen[group.id] ?? false, (next) => (moreOpen = { ...moreOpen, [group.id]: next })}
          onnavigate={goId}>
          {#each shown as each (each.id)}{@render row(each)}{/each}
        </NavGroup>
      {/each}
    </NavLevel>
  {/if}
</nav>

<style>
  .area { display: flex; flex-direction: column; flex: 1; min-block-size: 0; }
  .area[data-size='touch'] { padding-block-end: calc(var(--density-control-md-min) + var(--sp-200)); }
  .rail { margin: 0; padding: calc(var(--sp-050) + var(--sp-025)) var(--sp-100); list-style: none; }
  .card-list {
    margin: 0 0 var(--sp-150);
    padding: 0;
    overflow: hidden;
    list-style: none;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
</style>
