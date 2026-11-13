<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What happened in a hub, on its overview (work-surface §6.1, scene hub): sentences with the
  // person and the thing's tile - an entry's last step from its own history, a page's or a board's
  // last change from its row - newest first. Every read is the reader's own, so nothing the reader
  // may not read is named; an actor whose name is not resolved is the sentence true of its kind.

  import { untrack } from 'svelte';
  import { Avatar, ProductTile, type IconName } from '@hubtask/design-system/components';

  import { accounts } from '../data/accounts.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { pagesPath } from '../data/pages.svelte.ts';
  import { boardsPath } from '../data/whiteboards.svelte.ts';
  import { formatRelative } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { workflows } from '../data/workflows.svelte.ts';
  import { happenedVerb, latestHappenings, statusNameOf, type Happening } from './happened.ts';

  import type { ActivityPage, ItemQueryResult, PageList, WhiteboardList, WorkItem } from '@hubtask/sync-engine';

  /** How many sentences the overview says; the places themselves hold the rest. */
  const SHOWN = 5;

  interface Props {
    hubId: string;
    /** The modules on at the hub: pages and boards are read only where theirs is on. */
    modulesOn?: readonly string[];
  }

  const { hubId, modulesOn = [] }: Props = $props();

  function follow<T>(path: () => string | undefined, onready: (data: T) => void, body?: () => unknown): void {
    $effect(() => {
      const at = path();
      if (at === undefined) return;
      const sent = body?.();
      return untrack(() => engine.subscribe<T>({ path: at, ...(sent === undefined ? {} : { body: sent }) }, (next) => {
        if (next.status === 'ready') onready(next.data);
      }));
    });
  }

  let entries = $state<readonly WorkItem[] | undefined>(undefined);
  follow<ItemQueryResult>(() => '/items:query', (data) => (entries = data.data.slice(0, SHOWN)),
    () => ({ scope: { container_id: hubId, include_descendants: true }, sort: [{ field: 'updated_at', dir: 'DESC' }], page: { size: SHOWN } }));
  let pages = $state<PageList['data']>([]);
  follow<PageList>(() => (modulesOn.includes('KNOWLEDGE') ? pagesPath(hubId) : undefined), (data) => (pages = data.data));
  let boards = $state<WhiteboardList['data']>([]);
  follow<WhiteboardList>(() => (modulesOn.includes('WHITEBOARD') ? boardsPath(hubId) : undefined), (data) => (boards = data.data));

  // Each entry's last step, for who did what: one short read per entry, the history the panel reads.
  let steps = $state<Record<string, ActivityPage['data'][number] | null>>({});
  const entryIds = $derived((entries ?? []).map((entry) => `${entry.id}@${entry.version}`).join(','));
  $effect(() => {
    const ids = entryIds === '' ? [] : entryIds.split(',').map((pair) => pair.split('@')[0] ?? '');
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<ActivityPage>({ path: `/items/${id}/activity?size=1` }, (next) => {
        if (next.status === 'ready') steps = { ...steps, [id]: next.data.data[0] ?? null };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });

  // A status a step set is named as its workflow names it; read only where the module is on.
  $effect(() => untrack(() => workflows.open()));

  /** Everything that happened: the search over the hub, the latest change first. */
  const allHref = $derived.by(() => {
    const name = containers.find(hubId)?.name;
    return name ? `/search?${new URLSearchParams({ f: `hub:"${name.replaceAll('"', '')}" sort:-updated` })}` : undefined;
  });

  const happenings = $derived(entries === undefined ? undefined : latestHappenings({ entries, steps, pages, boards }, SHOWN));
  $effect(() => {
    accounts.resolve((happenings ?? []).flatMap((each) => (each.actorId ? [each.actorId] : [])));
  });

  function whoOf(each: Happening): string {
    if (each.actorId && each.actorId === actor.account?.id) return t('app.activity.actor_you');
    const named = each.actorId ? accounts.nameOf(each.actorId) : undefined;
    if (named) return named;
    const code = `app.activity.actor_${each.actorKind ?? 'UNKNOWN'}`;
    return t(messages.has(code) ? code : 'app.activity.actor_someone');
  }

  /** The sentence around its object, split where the object stands, so the object is a link. */
  function partsOf(each: Happening): [string, string] {
    const marker = '\u0001';
    const status = each.statusKey ? statusNameOf(each, hubId, workflows.list, t) : '';
    const said = t(`app.hub_overview.did_${happenedVerb(each)}`, { actor: whoOf(each), object: marker, status });
    const at = said.indexOf(marker);
    return at < 0 ? [said, ''] : [said.slice(0, at), said.slice(at + marker.length)];
  }

  const TILES: Record<Happening['kind'], { icon: IconName; product?: 'pages' | 'whiteboards' }> = {
    entry: { icon: 'square-check' },
    page: { icon: 'book-open', product: 'pages' },
    board: { icon: 'layout-template', product: 'whiteboards' },
  };
  const colourOf = (each: Happening) => (each.collectionId ? containers.find(each.collectionId)?.color_token ?? 'brand' : 'brand');
</script>

{#if happenings && happenings.length > 0}
  <section class="recent" aria-labelledby="hub-recent">
    <header class="heading">
      <h2 id="hub-recent">{t('app.hub_overview.recent')}</h2>
      {#if allHref}<a class="all" href={allHref} data-recent-all>{t('app.hub_overview.recent_all')}</a>{/if}
    </header>
    <ul>
      {#each happenings as each (each.kind + each.id)}
        {@const [before, after] = partsOf(each)}
        <li data-recent-entry={each.kind === 'entry' ? each.id : undefined} data-recent={each.kind}>
          <span class="face" aria-hidden="true"><Avatar name={whoOf(each)} src={each.actorId ? accounts.pictureOf(each.actorId) : undefined} size="sm" /></span>
          <span class="said">{before}<a href={each.href}><span class="tile"><ProductTile icon={TILES[each.kind].icon} product={TILES[each.kind].product} color={colourOf(each)} size="sm" /></span><span class="title">{each.title}</span></a>{after}</span>
          <span class="when">{formatRelative(each.at, messages.locale)}</span>
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .recent {
    display: flex;
    flex-direction: column;
    min-inline-size: 0;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-raised);
  }

  .heading {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }
  .heading h2 { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .heading .all { font-size: var(--fs-075); font-weight: var(--fw-regular); color: var(--text-brand); }

  ul { display: flex; flex-direction: column; margin: 0; padding: 0; list-style: none; }

  li {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150);
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }
  li + li { border-block-start: var(--bw-hairline) solid var(--border-subtle); }

  .face { flex: none; }
  .said { flex: 1; min-inline-size: 0; overflow-wrap: anywhere; }

  a { color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  a:hover .title { text-decoration: underline; }
  a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-sm); }
  .tile { display: inline-flex; vertical-align: middle; margin-inline: var(--sp-025) var(--sp-050); }

  .when { flex: none; white-space: nowrap; color: var(--text-subtle); }
</style>
