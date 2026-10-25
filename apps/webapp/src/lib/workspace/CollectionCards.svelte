<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's collections as cards (ux-inventory.md §E4): each with its mark, its name, its
  // description and what counts in it - the figure the server counted on the list's read
  // (UC-WRK-29), drawn as the collection's own head draws it. A collection that counts nothing
  // shows none rather than an invented one (P-11).
  //
  // A list of links, so a screen reader hears how many there are and each one's name first; the
  // whole card is the link's target, the rest of it is what the link says about the collection.

  import { Icon } from '@hubtask/design-system/components';

  import CollectionFigure from '../entries/CollectionFigure.svelte';
  import ContainerMark from './ContainerMark.svelte';
  import { t } from '../i18n/i18n.svelte.ts';

  import type { Container } from '@hubtask/sync-engine';

  interface Props {
    collections: readonly Container[];
    /**
     * The way to a new one, as the last card: where the collections are, as a board's next column
     * is a dashed column at its end (design-system.md §11.8). Absent for who may not make one.
     */
    create?: { readonly label: string; readonly onclick: () => void; readonly disabledReason?: string };
  }

  const { collections, create }: Props = $props();
</script>

<ul class="cards" aria-label={t('app.workspace.collections')}>
  {#each collections as collection (collection.id)}
    <li class="card" data-card={collection.id}>
      <a class="name" href={`/collections/${collection.id}`}>
        <ContainerMark look={collection} />
        <span>{collection.name}</span>
      </a>
      {#if collection.description}<p class="description">{collection.description}</p>{/if}
      {#if collection.figure}<div class="figure"><CollectionFigure figure={collection.figure} name={collection.name} /></div>{/if}
    </li>
  {/each}
  {#if create}
    <li class="new">
      <button type="button" class="add" data-opener="add-collection" aria-disabled={create.disabledReason ? 'true' : undefined} title={create.disabledReason} onclick={() => !create.disabledReason && create.onclick()}>
        <Icon name="plus" size="sm" />{create.label}
      </button>
    </li>
  {/if}
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
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-raised);
    box-shadow: var(--shadow-raised);
  }

  .card:hover { border-color: var(--border-default); }

  /* The next collection: dashed, quiet until pointed at, the brand's ink as every add in the lists. */
  .new { display: flex; }
  .add {
    display: flex;
    flex: 1;
    align-items: center;
    justify-content: center;
    gap: var(--sp-100);
    min-block-size: var(--sp-1000);
    border: var(--bw-thick) dashed var(--border-default);
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-brand);
    font: inherit;
    font-weight: var(--fw-medium);
    cursor: pointer;
  }
  .add:hover { border-color: var(--accent-primary); background: var(--bg-surface-hover); }
  .add[aria-disabled='true'] { color: var(--text-subtle); cursor: not-allowed; }
  .add:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .card:focus-within {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--bw-ring);
  }

  .name {
    display: inline-flex;
    align-items: center;
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

  .description {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
  }

  .figure { font-size: var(--fs-075); color: var(--text-secondary); }
</style>
