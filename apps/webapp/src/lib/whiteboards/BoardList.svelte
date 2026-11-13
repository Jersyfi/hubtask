<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The same board as a list (work surface §3, P-13): every object under the frame it stands in,
  // in reading order, for the keyboard and a screen reader. *Show* returns to the surface on it.
  import { Button, Checkbox } from '@hubtask/design-system/components';
  import type { ReferenceTarget } from '@hubtask/sync-engine';
  import type { Element } from '@hubtask/whiteboard';

  import { t } from '../i18n/i18n.svelte.ts';
  import { listOfBoard } from './list.ts';

  interface Props {
    elements: readonly Element[];
    frames: readonly Element[];
    cardOf: (e: Element) => ReferenceTarget | undefined;
    votes: ReadonlyMap<string, number>;
    pins: readonly { thread: { element_id?: string | null }; count: number }[];
    writable: boolean;
    wordsOf: (e: Element) => string;
    onshow: (id: string) => void;
    ontick: (e: Element) => void;
  }

  const { elements, frames, cardOf, votes, pins, writable, wordsOf, onshow, ontick }: Props = $props();

  const groups = $derived(listOfBoard(elements, frames));
  const commentsOn = (id: string) => pins.filter((pin) => pin.thread.element_id === id).reduce((sum, pin) => sum + pin.count, 0);
</script>

<div class="list" data-board-list>
  {#each groups as group (group.frame?.id ?? '')}
    <section class="group" aria-labelledby={`board-list-${group.frame?.id ?? 'loose'}`}>
      <h2 class="group-title" id={`board-list-${group.frame?.id ?? 'loose'}`}>
        {group.frame ? group.frame.text || t('app.whiteboards.kind_frame') : t('app.whiteboards.list_loose')}
        <span class="count">{group.items.length}</span>
      </h2>
      <ul class="rows">
        {#each group.items as e (e.id)}
          {@const card = e.kind === 'CARD' ? cardOf(e) : undefined}
          <li class="row">
            {#if card?.available && writable}
              <Checkbox label={card.title ?? ''} checked={card.completed ?? false} isLabelHidden onchange={() => ontick(e)} />
            {/if}
            <span class="kind">{t(`app.whiteboards.kind_${e.kind.toLowerCase()}`)}</span>
            <span class="words">{card ? `${card.key ?? ''} ${card.title ?? t('app.whiteboards.card_unavailable')}` : wordsOf(e) || '—'}</span>
            {#if votes.get(e.id)}<span class="meta">{t('app.whiteboards.votes_on', { count: String(votes.get(e.id)) })}</span>{/if}
            {#if commentsOn(e.id) > 0}<span class="meta">{t('app.whiteboards.comment_pin', { count: String(commentsOn(e.id)) })}</span>{/if}
            <Button size="sm" tone="subtle" onclick={() => onshow(e.id)}>{t('app.whiteboards.list_show')}</Button>
          </li>
        {/each}
      </ul>
    </section>
  {:else}
    <p class="quiet">{t('app.whiteboards.blank_read')}</p>
  {/each}
</div>

<style>
  .list {
    position: absolute;
    inset: 0;
    overflow-y: auto;
    padding: calc(var(--sp-600) + var(--sp-200)) var(--sp-300) calc(var(--sp-600) + var(--sp-300));
    background: var(--bg-surface);
    display: grid;
    align-content: start;
    gap: var(--sp-300);
  }
  .group { display: grid; gap: var(--sp-100); max-inline-size: var(--layout-reading); margin-inline: auto; inline-size: 100%; }
  .group-title { margin: 0; display: flex; gap: var(--sp-100); align-items: baseline; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .count { color: var(--text-subtle); font-size: var(--fs-075); font-weight: var(--fw-regular); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; }
  .row {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    min-block-size: var(--density-control-md-min);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }
  .kind { color: var(--text-subtle); font-size: var(--fs-075); }
  .words { flex: 1 1 auto; min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { color: var(--text-secondary); font-size: var(--fs-075); }
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
