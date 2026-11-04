<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The search's other kinds below its entries (user-experience.md §5.3): pages through the pages'
  // own search over every space the reader may read, boards by their title from each hub's list,
  // goals by their title from the goals the reader may read, configuration items by their name
  // from each hub's inventory (its first page), and hubs and collections by name from what the tree holds - each with one `ListRow` anatomy,
  // each only where its module is on. Every read is the reader's own, so what they may not read is
  // not found.
  import { untrack } from 'svelte';
  import { Icon, ListRow, Stack } from '@hubtask/design-system/components';
  import type { ConfigurationItemList, GoalList, PageHitList, WhiteboardList } from '@hubtask/sync-engine';

  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { pages } from '../data/pages.svelte.ts';
  import { boardsPath } from '../data/whiteboards.svelte.ts';
  import { cisPath, matchingCis } from '../data/cmdb.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { markOf } from '../workspace/look.ts';
  import { nameMatches, shows, type SearchKind } from './kinds.ts';

  /** How long the words rest before the pages are asked: one read per pause, not per key. */
  const SETTLE_MS = 250;
  const SHOWN = 10;

  interface Props {
    words: string;
    kind: SearchKind;
    modulesOn: readonly string[] | undefined;
    /** How many hits this draws, so the screen says "nothing matches" only when nothing does. */
    oncount: (count: number) => void;
    onnavigate?: (path: string) => void;
  }
  const { words, kind, modulesOn, oncount, onnavigate }: Props = $props();

  const asked = $derived(words.trim());
  const isOn = (module: string) => modulesOn?.includes(module) ?? false;

  let pageHits = $state<PageHitList['data']>([]);
  $effect(() => {
    const text = asked;
    if (text === '' || !shows(kind, 'pages') || !isOn('KNOWLEDGE')) {
      pageHits = [];
      return;
    }
    let isCurrent = true;
    const timer = setTimeout(() => {
      // A refused search finds nothing here: the entries above say what went wrong with the words.
      void pages.search(text, undefined, SHOWN).then(
        (hits) => { if (isCurrent) pageHits = hits; },
        () => { if (isCurrent) pageHits = []; },
      );
    }, SETTLE_MS);
    return () => { isCurrent = false; clearTimeout(timer); };
  });

  // The boards are read once per hub while the search is open, and matched as the words change.
  const wantsBoards = $derived(asked !== '' && shows(kind, 'boards') && isOn('WHITEBOARD'));
  const hubKey = $derived(wantsBoards ? containers.hubs.map((hub) => hub.id).join(',') : '');
  let boardsAt = $state<Record<string, WhiteboardList['data']>>({});
  $effect(() => {
    const ids = hubKey === '' ? [] : hubKey.split(',');
    return untrack(() => {
      const stops = ids.map((hubId) =>
        engine.subscribe<WhiteboardList>({ path: boardsPath(hubId) }, (next) => {
          if (next.status === 'ready') boardsAt = { ...boardsAt, [hubId]: next.data.data };
        }),
      );
      return () => stops.forEach((stop) => stop());
    });
  });
  const boardHits = $derived(
    wantsBoards ? Object.values(boardsAt).flat().filter((board) => nameMatches(board.title, asked)).slice(0, SHOWN) : [],
  );

  const wantsGoals = $derived(asked !== '' && shows(kind, 'goals') && isOn('GOALS'));
  let goals = $state<GoalList['data']>([]);
  $effect(() => {
    if (!wantsGoals) return;
    return untrack(() => engine.subscribe<GoalList>({ path: '/goals' }, (next) => { if (next.status === 'ready') goals = next.data.data; }));
  });
  const goalHits = $derived(wantsGoals ? goals.filter((goal) => nameMatches(goal.title, asked)).slice(0, SHOWN) : []);

  const wantsCis = $derived(asked !== '' && shows(kind, 'assets') && isOn('ASSETS'));
  const ciHubKey = $derived(wantsCis ? containers.hubs.map((hub) => hub.id).join(',') : '');
  let cisAt = $state<Record<string, ConfigurationItemList['data']>>({});
  $effect(() => {
    const ids = ciHubKey === '' ? [] : ciHubKey.split(',');
    return untrack(() => {
      const stops = ids.map((hubId) =>
        engine.subscribe<ConfigurationItemList>({ path: cisPath(hubId) }, (next) => {
          if (next.status === 'ready') cisAt = { ...cisAt, [hubId]: next.data.data };
        }),
      );
      return () => stops.forEach((stop) => stop());
    });
  });
  const ciHits = $derived(wantsCis ? matchingCis(Object.values(cisAt).flat(), asked, SHOWN) : []);

  const placeHits = $derived(
    asked !== '' && shows(kind, 'places')
      ? [...containers.hubs, ...containers.loadedCollections].filter((place) => nameMatches(place.name, asked)).slice(0, SHOWN)
      : [],
  );

  $effect(() => oncount(pageHits.length + boardHits.length + goalHits.length + ciHits.length + placeHits.length));

  const hubName = (id: string) => containers.find(id)?.name ?? '';
  const go = (path: string) => onnavigate?.(path);
</script>

{#snippet section(id: string, title: string, rows: readonly { id: string; title: string; meta: string; href: string; icon: import('@hubtask/design-system/components').IconName }[])}
  <section aria-labelledby={id}>
    <Stack gap="100">
      <h2 class="heading" id={id}>{title}</h2>
      <ul class="rows">
        {#each rows as row (row.id)}
          <li>
            <ListRow href={row.href} onactivate={() => go(row.href)} label={row.title}>
              {#snippet leading()}<Icon name={row.icon} size="sm" />{/snippet}
              <span class="name" title={row.title}>{row.title}</span>
              {#if row.meta}<span class="meta">{row.meta}</span>{/if}
            </ListRow>
          </li>
        {/each}
      </ul>
    </Stack>
  </section>
{/snippet}

{#if pageHits.length > 0}
  {@render section('search-pages', t('app.search.kind.pages'), pageHits.map((hit) => ({
    id: hit.id, title: hit.title, href: `/pages/${hit.id}`, icon: 'book-open',
    meta: [hubName(hit.hub_id), hit.excerpt.replace(/[«»]/gu, '')].filter(Boolean).join(' · '),
  })))}
{/if}
{#if boardHits.length > 0}
  {@render section('search-boards', t('app.search.kind.boards'), boardHits.map((board) => ({
    id: board.id, title: board.title, href: `/whiteboards/${board.id}`, icon: 'layout-template', meta: hubName(board.hub_id),
  })))}
{/if}
{#if goalHits.length > 0}
  {@render section('search-goals', t('app.search.kind.goals'), goalHits.map((goal) => ({
    id: goal.id, title: goal.title, href: `/goals/${goal.id}`, icon: 'flag', meta: goal.hub_id ? hubName(goal.hub_id) : '',
  })))}
{/if}
{#if ciHits.length > 0}
  {@render section('search-assets', t('app.search.kind.assets'), ciHits.map((ci) => ({
    id: ci.id, title: ci.name, href: `/cis/${ci.id}`, icon: 'shield', meta: hubName(ci.hub_id),
  })))}
{/if}
{#if placeHits.length > 0}
  {@render section('search-places', t('app.search.kind.places'), placeHits.map((place) => ({
    id: place.id, title: place.name, href: place.type === 'HUB' ? `/hubs/${place.id}` : `/collections/${place.id}`, icon: markOf(place),
    meta: place.type === 'COLLECTION' && place.parent_id ? hubName(place.parent_id) : '',
  })))}
{/if}

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .rows { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .name { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { display: block; overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
</style>
