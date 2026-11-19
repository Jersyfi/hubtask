<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's collections as cards (ux-inventory.md §E4, the navigation concept's mockup): each with
  // its mark and its name, then one line - what counts in it, the figure the server counted on the
  // list's read (UC-WRK-29), and its running sprint - and a bar where a read says how far it is:
  // the running sprint's completed over its committed entries, or a PROGRESS figure's own bar. A
  // collection that counts nothing shows none rather than an invented one (P-11): an OPEN or DUE
  // figure has no total, so it draws no bar.
  //
  // A list of links, so a screen reader hears how many there are and each one's name first; the
  // whole card is the link's target, the rest of it is what the link says about the collection.

  import { untrack } from 'svelte';
  import { AvatarGroup } from '@hubtask/design-system/components';

  import CollectionFigure from '../entries/CollectionFigure.svelte';
  import ContainerMark from './ContainerMark.svelte';
  import { accounts } from '../data/accounts.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { people } from '../data/people.svelte.ts';
  import { sprintsPath } from '../data/sprints.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  import type { Container, SprintList } from '@hubtask/sync-engine';

  interface Props {
    collections: readonly Container[];
    /** Whether PLANNING is on at the hub: only then is a collection's running sprint read. */
    planning?: boolean;
  }

  const { collections, planning = false }: Props = $props();

  // The same read as the sidebar's sprint mark, so the engine answers both from one request.
  let running = $state<Record<string, SprintList['data'][number] | null>>({});
  const sprintKey = $derived(planning ? collections.map((each) => each.id).join(',') : '');
  $effect(() => {
    const ids = sprintKey === '' ? [] : sprintKey.split(',');
    return untrack(() => {
      const stops = ids.map((id) => engine.subscribe<SprintList>({ path: sprintsPath(id) }, (next) => {
        if (next.status !== 'ready') return;
        running = { ...running, [id]: (next.data.data ?? []).find((sprint) => sprint.state === 'ACTIVE') ?? null };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });

  // Who holds a role at each collection, as the head's faces read them (PlacePeople): the card shows
  // who works there, not a list of names.
  const pathOf = (collection: Container) => ({ hubId: collection.parent_id ?? undefined, collectionId: collection.id });
  $effect(() => {
    const stops = collections.map((collection) => people.open(pathOf(collection)));
    return () => stops.forEach((stop) => stop());
  });
  const membersOf = (collection: Container) => [...new Set(people.along(pathOf(collection)).map((membership) => membership.account_id))];
  $effect(() => {
    accounts.resolve(collections.flatMap(membersOf));
  });

  /** How far the running sprint is, in percent of its committed entries, where it committed any. */
  function sprintShare(id: string): number | undefined {
    const sprint = running[id];
    if (!sprint || sprint.committed_count <= 0) return undefined;
    return Math.min(100, Math.round((sprint.completed_count / sprint.committed_count) * 100));
  }
</script>

<ul class="cards" aria-label={t('app.workspace.collections')}>
  {#each collections as collection (collection.id)}
    {@const sprint = running[collection.id]}
    {@const share = collection.figure?.kind === 'PROGRESS' ? undefined : sprintShare(collection.id)}
    {@const members = membersOf(collection)}
    <li class="card" data-card={collection.id}>
      <div class="top">
        <a class="name" href={`/collections/${collection.id}`}>
          <ContainerMark look={collection} hasDefaultTile />
          <span class="words">
            <span class="title">{collection.name}</span>
            {#if collection.description}<span class="description">{collection.description}</span>{/if}
          </span>
        </a>
        {#if members.length > 0}
          <span class="people" data-card-people aria-hidden="true">
            <AvatarGroup people={members.map((id) => ({ name: accounts.nameOf(id) ?? t('app.people.unnamed'), src: accounts.pictureOf(id) }))} size="sm" max={4}
              overflowLabel={t('app.people.more_members', { count: String(Math.max(members.length - 4, 0)) })} />
          </span>
        {/if}
      </div>
      {#if collection.figure || sprint || share !== undefined}
        <div class="figure">
          {#if collection.figure}<CollectionFigure figure={collection.figure} name={collection.name} />{/if}
          {#if collection.figure && sprint}<span aria-hidden="true">·</span>{/if}
          {#if sprint}<span data-card-sprint>{sprint.name}</span>{/if}
          {#if share !== undefined && sprint}<span class="share" data-card-share>{t('app.workspace.card_done_share', { share })}</span>{/if}
        </div>
      {/if}
    </li>
  {/each}
</ul>

<style>
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 28ch), 1fr));
    gap: var(--sp-150);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .card {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-raised);
  }

  .card:hover { border-color: var(--border-default); }

  .card:focus-within {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--bw-ring);
  }

  .name {
    display: inline-flex;
    align-items: flex-start;
    min-inline-size: 0;
    gap: var(--sp-100);
    color: var(--text-primary);
    font-weight: var(--fw-semibold);
    text-decoration: none;
  }

  .name:focus-visible { outline: none; }

  /* The whole card answers the pointer, the link alone the keyboard and the reader. */
  .name::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: inherit;
  }

  .top { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--sp-100); min-inline-size: 0; }
  .words { display: grid; gap: var(--sp-025); min-inline-size: 0; }
  .title { overflow-wrap: anywhere; }
  .people { flex: none; }
  .share { margin-inline-start: auto; }

  .description {
    margin: 0;
    font-weight: var(--fw-regular);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
  }

  .figure { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); font-size: var(--fs-075); color: var(--text-secondary); }
</style>
