<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A whiteboard, or one frame of it, drawn live where a page embeds it (`![](hubtask:whiteboard/<id>#<frame>)`,
  // research §4.5): connected to the board's live document read-only, so what people draw shows
  // as they draw it. A reader who may not read the board sees why, never the board. Drawing happens
  // on the board itself, which the view opens.

  import { untrack } from 'svelte';
  import { tokens } from '@hubtask/design-system';
  import { Button } from '@hubtask/design-system/components';
  import { LiveDocument, type LiveSocket } from '@hubtask/editor';
  import type { ReferenceTarget, WhiteboardRead } from '@hubtask/sync-engine';
  import {
    Board, Doc, PEN_WIDTH, bounds, connectorPath, lines, linkHost, outlinePath, strokePath, withinFrame, type Element,
  } from '@hubtask/whiteboard';

  import { engine } from '../data/engine.ts';
  import { resolveRequest } from '../data/links.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { BOARD_COLOURS, boardPath, cardEntries, whiteboards } from '../data/whiteboards.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    boardId: string;
    /** One frame of the board, by its identifier; the whole board without. */
    frame?: string | null;
    onopen: (path: string) => void;
  }

  const { boardId, frame, onopen }: Props = $props();

  const read = untrack(() => resource<WhiteboardRead>(boardPath(boardId)));
  const board = $derived(read.state.status === 'ready' ? read.state.data : undefined);
  const refusal = $derived(read.state.status === 'failed' ? renderProblem(read.state.error, messages).message : undefined);

  const doc = new Doc();
  const model = new Board(doc);
  let live = $state<Element[] | undefined>(undefined);
  let cards = $state<Map<string, ReferenceTarget>>(new Map());

  $effect(() => {
    const id = board?.id;
    if (!id) return;
    return untrack(() => {
      const stop = model.observe(() => (live = model.size > 0 ? model.elements() : undefined));
      const connection = new LiveDocument(doc, {
        open: () => whiteboards.liveSocket(id) as unknown as LiveSocket,
        writable: false,
        onSynced: () => (live = model.size > 0 ? model.elements() : undefined),
      });
      return () => {
        stop();
        connection.destroy();
      };
    });
  });
  $effect(() => () => doc.destroy());

  const all = $derived(live ?? ((board?.elements ?? []) as Element[]));
  const frameElement = $derived(frame ? all.find((e) => e.id === frame && e.kind === 'FRAME') : undefined);
  const elements = $derived(frameElement ? withinFrame(all, frameElement) : all);
  const byId = $derived(new Map(elements.map((e) => [e.id, e])));
  const area = $derived(bounds(elements));
  const viewBox = $derived(area ? `${area.left - 24} ${area.top - 40} ${area.right - area.left + 48} ${area.bottom - area.top + 64}` : '0 0 400 200');

  const entryIds = $derived(cardEntries(elements).join(','));
  $effect(() => {
    const ids = entryIds;
    if (!ids) return;
    untrack(async () => {
      const state = await engine.refresh<{ data: ReferenceTarget[] }>(resolveRequest(ids.split(',').map((id) => ({ kind: 'ENTRY' as const, id }))));
      if (state.status === 'ready') cards = new Map(state.data.data.map((target) => [target.id.toLowerCase(), target]));
    });
  });

  const colour = (e: Element, part: 'bg' | 'fg') =>
    e.color_token && (BOARD_COLOURS as readonly string[]).includes(e.color_token) ? tokens.label[e.color_token as (typeof BOARD_COLOURS)[number]][part] : undefined;
  const path = $derived(frameElement ? `/whiteboards/${boardId}#${frameElement.id}` : `/whiteboards/${boardId}`);
</script>

<figure class="view">
  {#if refusal}
    <p class="quiet">{refusal}</p>
  {:else if !board}
    <p class="quiet">{t('app.whiteboards.loading')}</p>
  {:else}
    <figcaption class="head">
      <span class="name">{frameElement?.text ? `${board.title} · ${frameElement.text}` : board.title}</span>
      <Button size="sm" tone="subtle" icon="external-link" onclick={() => onopen(path)}>{t('app.whiteboards.open_board')}</Button>
    </figcaption>
    {#if frame && !frameElement}
      <p class="quiet">{t('whiteboards.frame_not_found')}</p>
    {:else}
      <svg class="picture" {viewBox} role="img" aria-label={t('app.whiteboards.canvas_live', { title: board.title, count: String(elements.length) })}>
        <defs>
          <marker id={`wbv-arrow-${boardId}`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" class="head-mark" />
          </marker>
        </defs>
        {#each elements as e (e.id)}
          {#if e.kind === 'FRAME'}
            <path class="frame" d={outlinePath(e)} />
            <text x={e.x} y={e.y - 8} class="frame-name">{e.text}</text>
          {:else if e.kind === 'CONNECTOR'}
            <path class="connector" d={connectorPath(e, byId)} marker-end={e.end === 'NONE' ? undefined : `url(#wbv-arrow-${boardId})`} />
          {:else if e.kind === 'STROKE'}
            <path class="stroke" class:highlighter={e.pen === 'HIGHLIGHTER'} d={strokePath(e)} style:stroke={colour(e, 'fg')} style:stroke-width={String(PEN_WIDTH[e.pen ?? 'PEN'])} />
          {:else if e.kind === 'CARD' || e.kind === 'LINK'}
            {@const target = e.target ? cards.get(e.target.id.toLowerCase()) : undefined}
            <path class="card" d={outlinePath(e)} />
            <text x={e.x + 12} y={e.y + 26} class="words">
              {e.kind === 'LINK' ? linkHost(e.url ?? '') : target?.available ? target.title : t('app.whiteboards.card_unavailable')}
            </text>
          {:else}
            {#if e.kind !== 'TEXT'}<path class={`object ${e.kind.toLowerCase()}`} d={outlinePath(e)} style:fill={colour(e, 'bg')} />{/if}
            {#each lines(e.text ?? '', e.width - 24, 14).slice(0, Math.max(1, Math.floor((e.height - 12) / 18))) as row, i (i)}
              <text x={e.x + 12} y={e.y + 24 + i * 18} class="words">{row}</text>
            {/each}
          {/if}
        {/each}
      </svg>
    {/if}
  {/if}
</figure>

<style>
  .view { margin: 0; display: grid; gap: var(--sp-050); padding: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface-sunken); }
  .head { display: flex; justify-content: space-between; align-items: center; gap: var(--sp-100); }
  .name { font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .picture { inline-size: 100%; max-block-size: calc(var(--sp-1600) * 4); }
  .frame { fill: none; stroke: var(--border-strong); stroke-dasharray: 8 6; }
  .frame-name { fill: var(--text-secondary); font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .connector { fill: none; stroke: var(--text-secondary); stroke-width: 2; }
  .head-mark { fill: var(--text-secondary); }
  .stroke { fill: none; stroke: var(--text-primary); stroke-linecap: round; stroke-linejoin: round; }
  .stroke.highlighter { stroke-opacity: 0.4; }
  .object { fill: var(--bg-raised); }
  .object.shape { stroke: var(--text-primary); stroke-width: 1.5; }
  .card { fill: var(--bg-raised); stroke: var(--border-default); }
  .words { fill: var(--text-primary); font-size: var(--fs-100); }
</style>
