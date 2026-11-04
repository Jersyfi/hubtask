<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The search's other kinds below its entries (user-experience.md §5.3): pages through the pages'
  // own search over every space the reader may read, the modules' named objects through the
  // sources the palette reads too (`objects.ts`), and hubs and collections by name from what the
  // tree holds - each with one `ListRow` anatomy, each only where its module is on.
  import { untrack } from 'svelte';
  import { Icon, ListRow, Stack } from '@hubtask/design-system/components';
  import type { PageHitList } from '@hubtask/sync-engine';

  import { containers } from '../data/containers.svelte.ts';
  import { pages } from '../data/pages.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { markOf } from '../workspace/look.ts';
  import { nameMatches, shows, type SearchKind } from './kinds.ts';
  import { OBJECT_SOURCES, objectHits, readObjects, readsOf, sourcesWanted, type ObjectRow } from './objects.svelte.ts';

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

  // Each list is read once while the search is open, and matched as the words change.
  const sources = $derived(sourcesWanted(modulesOn, (each) => shows(kind, each)));
  const reads = $derived(
    asked === ''
      ? []
      : readsOf(sources, { hubs: containers.hubs.map((hub) => hub.id), collections: containers.loadedCollections.map((collection) => collection.id) }),
  );
  /** The reads by their paths, so a new word does not read anew. */
  const readKey = $derived(reads.map((read) => read.path).join(' '));
  let objectsRead = $state<Record<string, readonly ObjectRow[]>>({});
  $effect(() => {
    void readKey;
    return untrack(() => readObjects(reads, (path, rows) => { objectsRead = { ...objectsRead, [path]: rows }; }));
  });
  // A list read for a kind no longer wanted is kept but not drawn.
  const objectRows = $derived(readKey === '' ? [] : objectHits(objectsRead, asked, SHOWN).filter((row) => sources.some((source) => source.kind === row.kind)));

  const placeHits = $derived(
    asked !== '' && shows(kind, 'places')
      ? [...containers.hubs, ...containers.loadedCollections].filter((place) => nameMatches(place.name, asked)).slice(0, SHOWN)
      : [],
  );

  $effect(() => oncount(pageHits.length + objectRows.length + placeHits.length));

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
{#each OBJECT_SOURCES as source (source.kind)}
  {@const rows = objectRows.filter((row) => row.kind === source.kind)}
  {#if rows.length > 0}
    {@render section(`search-${source.kind}`, t(`app.search.kind.${source.kind}`), rows.map((row) => ({
      id: row.id, title: row.title, href: row.href, icon: source.icon, meta: row.placeId ? hubName(row.placeId) : '',
    })))}
  {/if}
{/each}
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
