<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A whiteboard drawn and drawn on (M4, UC-CNV-01 … UC-CNV-03): an infinite canvas of sticky
  // notes, shapes, text, connectors attached to objects, frames and cards of entries, bound to the
  // board's live document for a writer, so several people draw at once. A reader is not connected
  // and sees the scene as last saved. Only what the viewport shows is drawn (`culled`).
  //
  // **A card is never a copy** (P-19): it names its entry, and what it shows is resolved for the
  // reader; ticking it completes the entry. A sticky note becomes an entry through the ordinary
  // creation of an entry, held to the type's form, and is then that entry's card in its place.
  //
  // The keyboard reaches everything (research §4.7): Tab and Shift+Tab walk the objects, the
  // arrows move the chosen one, N puts a sticky note in the middle, Enter edits its words, Delete
  // takes it off; the choice is announced.

  import { untrack } from 'svelte';
  import { tokens } from '@hubtask/design-system';
  import { Button, Dialog, IconButton, Inline, Select, Stack, Textarea } from '@hubtask/design-system/components';
  import { LiveDocument, type LiveSocket, type LiveStatus } from '@hubtask/editor';
  import type { ReferenceTarget, WhiteboardRead, WorkItem } from '@hubtask/sync-engine';
  import {
    Board, DEFAULT_SIZE, Doc, ROUTES, SHAPES, bounds, box, connectorPath, culled, elementId, fit, hit, labelPoint, lines, outlinePath, toBoard, toSvg,
    withinFrame, zoomAt, type Element, type ElementKind, type Shape, type Viewport,
  } from '@hubtask/whiteboard';

  import { announcer } from '../announce.svelte.ts';
  import EntryPicker from '../entries/EntryPicker.svelte';
  import { engine } from '../data/engine.ts';
  import { items } from '../data/items.svelte.ts';
  import { resolveRequest } from '../data/links.svelte.ts';
  import { BOARD_COLOURS, boardFileName, cardEntries, pictureColours, whiteboards } from '../data/whiteboards.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { platform } from '../platform/index.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    board: WhiteboardRead;
    /** The hub's collections, offered to hold a sticky note made an entry (UC-CNV-02). */
    collections: readonly { id: string; name: string }[];
    onnavigate: (path: string) => void;
  }

  const { board, collections, onnavigate }: Props = $props();

  type Tool = 'select' | ElementKind;

  // A pause in the drawing before the scene is saved beside the live document.
  const SAVE_AFTER = 1200;
  const NUDGE = 10;

  const writable = untrack(() => board.writable);
  const doc = new Doc();
  const model = new Board(doc);
  let elements = $state<Element[]>([]);
  let status = $state<LiveStatus | 'reading'>('reading');
  let isLive = $state(false);
  let tool = $state<Tool>('select');
  let shape = $state<Shape>('RECTANGLE');
  let selected = $state<string[]>([]);
  let editing = $state<string | undefined>(undefined);
  let draft = $state('');
  let view = $state<Viewport>({ x: -80, y: -80, zoom: 1, width: 1, height: 1 });
  let surface = $state<SVGSVGElement | undefined>(undefined);
  let cards = $state<Map<string, ReferenceTarget>>(new Map());
  let isPickingEntry = $state(false);
  let picked = $state<WorkItem | undefined>(undefined);
  let isMakingEntry = $state(false);

  const byId = $derived(new Map(elements.map((e) => [e.id, e])));
  const shown = $derived(culled(elements, view, 200));
  const frames = $derived(elements.filter((e) => e.kind === 'FRAME'));
  const chosen = $derived(selected.length === 1 ? byId.get(selected[0] ?? '') : undefined);
  const viewBox = $derived(`${view.x} ${view.y} ${view.width / view.zoom} ${view.height / view.zoom}`);

  // --- The live document ---------------------------------------------------------------------------

  let timer: ReturnType<typeof setTimeout> | undefined;
  function changed() {
    elements = model.elements();
    if (!writable || !isLive) return;
    clearTimeout(timer);
    timer = setTimeout(() => void save(), SAVE_AFTER);
  }

  async function save() {
    try {
      await whiteboards.saveScene(board, model.scene() as never);
    } catch (error) {
      toast.show({ text: t('app.whiteboards.save_failed', { reason: renderProblem(error as never, messages).message }), tone: 'danger' });
    }
  }

  $effect(() => {
    return untrack(() => {
      const stop = model.observe(changed);
      if (!writable) {
        model.seed((board.elements ?? []) as Element[]);
        elements = model.elements();
        return () => { stop(); doc.destroy(); };
      }
      const connection = new LiveDocument(doc, {
        open: () => whiteboards.liveSocket(board.id) as unknown as LiveSocket,
        writable: true,
        onStatus: (next) => (status = next),
        onSynced: () => {
          if (isLive) return;
          isLive = true;
          // A board with a saved scene and no live document yet: the first writer seeds it.
          model.seed((board.elements ?? []) as Element[]);
          elements = model.elements();
        },
      });
      return () => {
        clearTimeout(timer);
        if (isLive) void save();
        stop();
        connection.destroy();
        doc.destroy();
      };
    });
  });

  // --- Cards, resolved for the reader --------------------------------------------------------------

  const entryIds = $derived(cardEntries(elements).join(','));
  $effect(() => {
    const ids = entryIds;
    if (!ids) return;
    untrack(() => void resolveCards(ids.split(',')));
  });

  let resolving = 0;
  async function resolveCards(ids: string[]) {
    const mine = (resolving += 1);
    const state = await engine.refresh<{ data: ReferenceTarget[] }>(resolveRequest(ids.map((id) => ({ kind: 'ENTRY' as const, id }))));
    if (mine !== resolving || state.status !== 'ready') return;
    cards = new Map(state.data.data.map((target) => [target.id.toLowerCase(), target]));
  }

  const cardOf = (e: Element) => (e.target ? cards.get(e.target.id.toLowerCase()) : undefined);

  async function tick(e: Element) {
    const target = cardOf(e);
    if (!target?.available || !e.target) return;
    try {
      await items.setCompleted(e.target.id, !target.completed, crypto.randomUUID());
      await resolveCards(cardEntries(elements));
    } catch (error) {
      toast.show({ text: t('app.whiteboards.card_failed', { reason: renderProblem(error as never, messages).message }), tone: 'danger' });
    }
  }

  // --- Drawing -------------------------------------------------------------------------------------

  function centre(): { x: number; y: number } {
    return { x: view.x + view.width / view.zoom / 2, y: view.y + view.height / view.zoom / 2 };
  }

  function put(kind: ElementKind, at: { x: number; y: number }, extra: Partial<Element> = {}): string | undefined {
    const size = DEFAULT_SIZE[kind];
    const element: Element = {
      id: elementId(), kind, x: Math.round(at.x - size.width / 2), y: Math.round(at.y - size.height / 2), ...size,
      ...(kind === 'STICKY' ? { color_token: 'amber', text: '' } : {}),
      ...(kind === 'SHAPE' ? { shape, text: '' } : {}),
      ...(kind === 'TEXT' ? { text: '' } : {}),
      ...(kind === 'FRAME' ? { text: t('app.whiteboards.frame_name', { n: String(frames.length + 1) }) } : {}),
      ...extra,
    };
    if (!model.add(element)) {
      toast.show({ text: t('whiteboards.too_many_elements', { maximum: '5000' }), tone: 'danger' });
      return undefined;
    }
    choose([element.id]);
    return element.id;
  }

  function choose(ids: string[]) {
    selected = ids;
    const one = ids.length === 1 ? model.get(ids[0] ?? '') : undefined;
    if (one) announcer.say(t('app.whiteboards.chosen', { kind: t(`app.whiteboards.kind_${one.kind.toLowerCase()}`), text: wordsOf(one) }));
  }

  function wordsOf(e: Element): string {
    if (e.kind === 'CARD') return cardOf(e)?.title ?? t('app.whiteboards.card_unavailable');
    return e.text ?? '';
  }

  function edit(id: string) {
    const e = model.get(id);
    if (!writable || !e || e.kind === 'CARD' || e.kind === 'CONNECTOR' || e.locked) return;
    editing = id;
    draft = e.text ?? '';
  }

  function commitEdit() {
    if (editing) model.update(editing, { text: draft });
    editing = undefined;
  }

  // --- Pointer -------------------------------------------------------------------------------------

  type Gesture =
    | { kind: 'pan'; startX: number; startY: number; viewX: number; viewY: number }
    | { kind: 'move'; lastX: number; lastY: number }
    | { kind: 'resize'; id: string; originX: number; originY: number }
    | { kind: 'connect'; from: string; endX: number; endY: number };
  let gesture = $state<Gesture | undefined>(undefined);

  function pointOf(event: PointerEvent | WheelEvent): { x: number; y: number } {
    const rect = surface?.getBoundingClientRect();
    return toBoard(view, { x: event.clientX - (rect?.x ?? 0), y: event.clientY - (rect?.y ?? 0) });
  }

  function down(event: PointerEvent) {
    if (editing) commitEdit();
    const at = pointOf(event);
    const target = hit(elements, at);
    surface?.setPointerCapture(event.pointerId);
    if (writable && tool === 'CONNECTOR' && target) {
      gesture = { kind: 'connect', from: target.id, endX: at.x, endY: at.y };
      return;
    }
    if (writable && tool !== 'select' && tool !== 'CONNECTOR') {
      const id = put(tool, at);
      tool = 'select';
      if (id && (model.get(id)?.kind ?? '') !== 'FRAME') edit(id);
      return;
    }
    if (target && target.kind !== 'FRAME') {
      if (event.shiftKey) choose(selected.includes(target.id) ? selected.filter((id) => id !== target.id) : [...selected, target.id]);
      else if (!selected.includes(target.id)) choose([target.id]);
      if (writable) gesture = { kind: 'move', lastX: at.x, lastY: at.y };
      return;
    }
    if (target && !event.shiftKey) choose([target.id]);
    else if (!target) selected = [];
    gesture = { kind: 'pan', startX: event.clientX, startY: event.clientY, viewX: view.x, viewY: view.y };
  }

  function moveTo(event: PointerEvent) {
    const g = gesture;
    if (!g) return;
    const at = pointOf(event);
    if (g.kind === 'pan') {
      view = { ...view, x: g.viewX - (event.clientX - g.startX) / view.zoom, y: g.viewY - (event.clientY - g.startY) / view.zoom };
    } else if (g.kind === 'move') {
      model.move(selected, Math.round(at.x - g.lastX), Math.round(at.y - g.lastY));
      gesture = { ...g, lastX: g.lastX + Math.round(at.x - g.lastX), lastY: g.lastY + Math.round(at.y - g.lastY) };
    } else if (g.kind === 'resize') {
      model.update(g.id, { width: Math.max(24, Math.round(at.x - g.originX)), height: Math.max(24, Math.round(at.y - g.originY)) });
    } else {
      gesture = { ...g, endX: at.x, endY: at.y };
    }
  }

  function up(event: PointerEvent) {
    const g = gesture;
    gesture = undefined;
    surface?.releasePointerCapture(event.pointerId);
    if (g?.kind !== 'connect') return;
    const target = hit(elements, pointOf(event));
    if (target && target.id !== g.from) {
      model.add({ id: elementId(), kind: 'CONNECTOR', from: g.from, to: target.id, route: 'STRAIGHT', start: 'NONE', end: 'ARROW', x: 0, y: 0, width: 0, height: 0 });
    }
    tool = 'select';
  }

  function resizeStart(event: PointerEvent, e: Element) {
    event.stopPropagation();
    surface?.setPointerCapture(event.pointerId);
    gesture = { kind: 'resize', id: e.id, originX: e.x, originY: e.y };
  }

  function wheel(event: WheelEvent) {
    event.preventDefault();
    const rect = surface?.getBoundingClientRect();
    const screen = { x: event.clientX - (rect?.x ?? 0), y: event.clientY - (rect?.y ?? 0) };
    if (event.ctrlKey || event.metaKey) view = zoomAt(view, screen, event.deltaY < 0 ? 1.1 : 1 / 1.1);
    else view = { ...view, x: view.x + event.deltaX / view.zoom, y: view.y + event.deltaY / view.zoom };
  }

  // --- Keyboard ------------------------------------------------------------------------------------

  function key(event: KeyboardEvent) {
    if (editing) return;
    const order = elements.filter((e) => e.kind !== 'CONNECTOR');
    if (event.key === 'Tab' && order.length > 0) {
      event.preventDefault();
      const at = order.findIndex((e) => e.id === selected[0]);
      const next = order[(at + (event.shiftKey ? order.length - 1 : 1) + (at < 0 && event.shiftKey ? 1 : 0)) % order.length];
      if (next) {
        choose([next.id]);
        const b = box(next);
        const seen = { x: view.x, y: view.y, w: view.width / view.zoom, h: view.height / view.zoom };
        if (b.left < seen.x || b.top < seen.y || b.right > seen.x + seen.w || b.bottom > seen.y + seen.h) {
          view = { ...view, x: b.left - seen.w / 4, y: b.top - seen.h / 4 };
        }
      }
      return;
    }
    if (event.key === 'Escape') { selected = []; tool = 'select'; return; }
    if (!writable) return;
    const step = event.shiftKey ? NUDGE * 5 : NUDGE;
    const moves: Record<string, [number, number]> = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] };
    const by = moves[event.key];
    if (by && selected.length > 0) { event.preventDefault(); model.move(selected, by[0], by[1]); return; }
    if ((event.key === 'Delete' || event.key === 'Backspace') && selected.length > 0) { event.preventDefault(); remove(); return; }
    if (event.key === 'Enter' && chosen) { event.preventDefault(); edit(chosen.id); return; }
    if (event.key === 'n' || event.key === 'N') { event.preventDefault(); const id = put('STICKY', centre()); if (id) edit(id); }
  }

  function remove() {
    model.remove(selected);
    selected = [];
    announcer.say(t('app.whiteboards.removed'));
  }

  // --- A sticky note made an entry; an entry put on the board (UC-CNV-02) --------------------------

  async function makeEntry(collectionId: string) {
    isMakingEntry = false;
    const note = chosen;
    if (!note || note.kind !== 'STICKY') return;
    const title = (note.text ?? '').trim();
    if (!title) {
      toast.show({ text: t('app.whiteboards.entry_needs_words'), tone: 'warning' });
      return;
    }
    try {
      const entry = await items.create({ type: 'TASK', collection_id: collectionId, title }, crypto.randomUUID());
      model.toCard(note.id, entry.id);
      toast.show({ text: t('app.whiteboards.entry_made', { title: entry.title }), tone: 'success' });
    } catch (error) {
      toast.show({ text: t('app.whiteboards.entry_failed', { reason: renderProblem(error as never, messages).message }), tone: 'danger' });
    }
  }

  function putEntry() {
    const entry = picked;
    isPickingEntry = false;
    picked = undefined;
    if (entry) put('CARD', centre(), { target: { kind: 'ENTRY', id: entry.id } });
  }

  // --- Export (UC-CNV-03) --------------------------------------------------------------------------

  function picture(which: 'board' | 'frame' | 'selection'): Element[] {
    if (which === 'frame' && chosen?.kind === 'FRAME') return withinFrame(elements, chosen);
    if (which === 'selection' && selected.length > 0) {
      const ids = new Set(selected);
      return elements.filter((e) => ids.has(e.id) || (e.kind === 'CONNECTOR' && ids.has(e.from ?? '') && ids.has(e.to ?? '')));
    }
    return elements;
  }

  function svgOf(which: 'board' | 'frame' | 'selection'): string {
    return toSvg(picture(which), {
      title: board.title, colours: pictureColours(), unavailable: t('app.whiteboards.card_unavailable'),
      card: (id) => {
        const target = cards.get(id.toLowerCase());
        return target?.available ? { title: target.title ?? '', key: target.key ?? null, completed: target.completed ?? false } : undefined;
      },
    });
  }

  function saveSvg(which: 'board' | 'frame' | 'selection') {
    platform.saveFile(new Blob([svgOf(which)], { type: 'image/svg+xml' }), boardFileName(board.title, 'svg'));
  }

  async function savePng(which: 'board' | 'frame' | 'selection') {
    const svg = svgOf(which);
    const url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }));
    try {
      const image = new Image();
      await new Promise<void>((resolve, reject) => {
        image.onload = () => resolve();
        image.onerror = () => reject(new Error('picture'));
        image.src = url;
      });
      const canvas = document.createElement('canvas');
      const scale = 2;
      canvas.width = image.width * scale;
      canvas.height = image.height * scale;
      const context = canvas.getContext('2d');
      context?.scale(scale, scale);
      context?.drawImage(image, 0, 0);
      const png = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/png'));
      if (png) platform.saveFile(png, boardFileName(board.title, 'png'));
    } catch {
      toast.show({ text: t('app.whiteboards.export_failed'), tone: 'danger' });
    } finally {
      URL.revokeObjectURL(url);
    }
  }

  async function saveNative() {
    try {
      const native = await whiteboards.exportNative(board.id, chosen?.kind === 'FRAME' ? chosen.id : undefined);
      platform.saveFile(new Blob([JSON.stringify(native, null, 2)], { type: 'application/json' }), boardFileName(board.title, 'json'));
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  const statusText = $derived(
    !writable ? t('app.whiteboards.status_reading')
    : status === 'connected' && isLive ? t('app.whiteboards.status_live')
    : status === 'refused' ? t('app.whiteboards.status_refused')
    : status === 'offline' ? t('app.whiteboards.status_offline')
    : t('app.whiteboards.status_connecting'),
  );

  const fillOf = (e: Element) =>
    e.color_token && (BOARD_COLOURS as readonly string[]).includes(e.color_token)
      ? tokens.label[e.color_token as (typeof BOARD_COLOURS)[number]].bg
      : tokens.bg.raised;
  const tools: readonly { tool: Tool; icon: 'hand' | 'square' | 'pencil' | 'arrow-right' | 'layout-template' | 'file-text' }[] = [
    { tool: 'select', icon: 'hand' }, { tool: 'STICKY', icon: 'square' }, { tool: 'SHAPE', icon: 'layout-template' },
    { tool: 'TEXT', icon: 'pencil' }, { tool: 'CONNECTOR', icon: 'arrow-right' }, { tool: 'FRAME', icon: 'file-text' },
  ];
  const editingElement = $derived(editing ? byId.get(editing) : undefined);
</script>

<div class="board">
  <div class="bar" role="toolbar" aria-label={t('app.whiteboards.tools')}>
    <Inline gap="050" align="center">
      {#if writable}
        {#each tools as each (each.tool)}
          <Button size="sm" tone={tool === each.tool ? 'primary' : 'subtle'} icon={each.icon} aria-pressed={tool === each.tool} onclick={() => (tool = each.tool)}>
            {t(`app.whiteboards.tool_${each.tool.toLowerCase()}`)}
          </Button>
        {/each}
        {#if tool === 'SHAPE'}
          <Select size="sm" label={t('app.whiteboards.shape')} bind:value={shape} options={SHAPES.map((each) => ({ value: each, label: t(`app.whiteboards.shape_${each.toLowerCase()}`) }))} />
        {/if}
        <Button size="sm" tone="subtle" icon="plus" onclick={() => (isPickingEntry = true)}>{t('app.whiteboards.put_entry')}</Button>
      {/if}
      <IconButton size="sm" icon="minus" label={t('app.whiteboards.zoom_out')} onclick={() => (view = zoomAt(view, { x: view.width / 2, y: view.height / 2 }, 1 / 1.25))} />
      <span class="zoom">{Math.round(view.zoom * 100)}%</span>
      <IconButton size="sm" icon="plus" label={t('app.whiteboards.zoom_in')} onclick={() => (view = zoomAt(view, { x: view.width / 2, y: view.height / 2 }, 1.25))} />
      <Button size="sm" tone="subtle" onclick={() => { const all = bounds(elements); if (all) view = fit(view, all); }}>{t('app.whiteboards.fit')}</Button>
      <span class="status" aria-live="polite">{statusText}</span>
    </Inline>
  </div>

  <div class="stage" bind:clientWidth={view.width} bind:clientHeight={view.height}>
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <svg
      bind:this={surface}
      class="canvas"
      class:drawing={tool !== 'select'}
      role="application"
      aria-label={t('app.whiteboards.canvas', { title: board.title, count: String(elements.length) })}
      aria-roledescription={t('app.whiteboards.canvas_role')}
      tabindex="0"
      {viewBox}
      onpointerdown={down}
      onpointermove={moveTo}
      onpointerup={up}
      onwheel={wheel}
      onkeydown={key}
      ondblclick={(event) => { const target = hit(elements, pointOf(event as unknown as PointerEvent)); if (target) edit(target.id); }}
    >
      <defs>
        <marker id="wb-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z" class="head" />
        </marker>
      </defs>
      {#each shown as e (e.id)}
        {#if e.kind === 'FRAME'}
          <g class="frame" class:chosen={selected.includes(e.id)}>
            <path d={outlinePath(e)} />
            <text x={e.x} y={e.y - 8} class="frame-name">{e.text}</text>
          </g>
        {:else if e.kind === 'CONNECTOR'}
          <g class="connector" class:chosen={selected.includes(e.id)}>
            <path d={connectorPath(e, byId)} marker-end={e.end === 'NONE' ? undefined : 'url(#wb-arrow)'} marker-start={e.start === 'ARROW' ? 'url(#wb-arrow)' : undefined} />
            {#if e.text}
              {@const at = labelPoint(e, byId)}
              <text x={at.x} y={at.y - 6} text-anchor="middle" class="label">{e.text}</text>
            {/if}
          </g>
        {:else if e.kind === 'CARD'}
          {@const target = cardOf(e)}
          <g class="card" class:chosen={selected.includes(e.id)} class:done={target?.completed}>
            <path d={outlinePath(e)} />
            {#if target?.available}
              <text x={e.x + 12} y={e.y + 22} class="key">{target.key ?? ''}</text>
              {#each lines(target.title ?? '', e.width - 48, 14).slice(0, 3) as row, i (i)}
                <text x={e.x + 12} y={e.y + 44 + i * 18} class="words">{row}</text>
              {/each}
              <a
                href={`/items/${target.id}`}
                onclick={(event) => { event.preventDefault(); onnavigate(`/items/${target.id}`); }}
                aria-label={t('app.whiteboards.card_open', { title: target.title ?? '' })}
              ><text x={e.x + e.width - 12} y={e.y + 22} text-anchor="end" class="open">↗</text></a>
              {#if writable}
                <g
                  role="checkbox"
                  aria-checked={target.completed ?? false}
                  aria-label={t('app.whiteboards.card_tick', { title: target.title ?? '' })}
                  tabindex="-1"
                  class="tick"
                  class:done={target.completed}
                  onpointerdown={(event) => { event.stopPropagation(); void tick(e); }}
                  onkeydown={(event) => { if (event.key === ' ') void tick(e); }}
                >
                  <rect x={e.x + e.width - 34} y={e.y + e.height - 34} width="22" height="22" rx="4" />
                  {#if target.completed}<text x={e.x + e.width - 23} y={e.y + e.height - 17} text-anchor="middle">✓</text>{/if}
                </g>
              {/if}
            {:else}
              <text x={e.x + 12} y={e.y + 30} class="words quiet">{t('app.whiteboards.card_unavailable')}</text>
            {/if}
          </g>
        {:else}
          <g class={`object ${e.kind.toLowerCase()}`} class:chosen={selected.includes(e.id)} class:locked={e.locked}>
            {#if e.kind !== 'TEXT'}<path d={outlinePath(e)} style:fill={e.kind === 'STICKY' || e.color_token ? fillOf(e) : undefined} />{/if}
            {#if editing !== e.id}
              {#each lines(e.text ?? '', e.width - 24, 14).slice(0, Math.max(1, Math.floor((e.height - 12) / 18))) as row, i (i)}
                <text x={e.x + 12} y={e.y + 24 + i * 18} class="words">{row}</text>
              {/each}
            {/if}
          </g>
        {/if}
      {/each}
      {#if gesture?.kind === 'connect'}
        {@const from = byId.get(gesture.from)}
        {#if from}<path class="drafting" d={`M ${from.x + from.width / 2} ${from.y + from.height / 2} L ${gesture.endX} ${gesture.endY}`} />{/if}
      {/if}
      {#if writable && chosen && chosen.kind !== 'CONNECTOR' && !chosen.locked}
        <rect class="handle" x={chosen.x + chosen.width - 6} y={chosen.y + chosen.height - 6} width="12" height="12" onpointerdown={(event) => resizeStart(event, chosen)} role="presentation" />
      {/if}
    </svg>
    {#if editingElement}
      <textarea
        class="editor"
        aria-label={t('app.whiteboards.words')}
        style:inset-inline-start={`${(editingElement.x - view.x) * view.zoom}px`}
        style:inset-block-start={`${(editingElement.y - view.y) * view.zoom}px`}
        style:inline-size={`${editingElement.width * view.zoom}px`}
        style:block-size={`${editingElement.height * view.zoom}px`}
        bind:value={draft}
        onblur={commitEdit}
        onkeydown={(event) => { if (event.key === 'Escape') { event.preventDefault(); commitEdit(); surface?.focus(); } }}
        maxlength="2000"
      ></textarea>
    {/if}
  </div>

  <aside class="side" aria-label={t('app.whiteboards.side')}>
    <Stack gap="150">
      {#if chosen}
        <Stack gap="100">
          <h3 class="heading">{t(`app.whiteboards.kind_${chosen.kind.toLowerCase()}`)}</h3>
          {#if writable && !chosen.locked}
            {#if chosen.kind === 'STICKY' || chosen.kind === 'SHAPE'}
              <div class="swatches" role="radiogroup" aria-label={t('app.whiteboards.colour')}>
                {#each BOARD_COLOURS as colour (colour)}
                  <button
                    type="button"
                    role="radio"
                    aria-checked={chosen.color_token === colour}
                    aria-label={t(`app.colour.${colour}`)}
                    class="swatch"
                    style:background={tokens.label[colour].bg}
                    onclick={() => model.update(chosen.id, { color_token: colour })}
                  ></button>
                {/each}
              </div>
            {/if}
            {#if chosen.kind === 'SHAPE'}
              <Select size="sm" label={t('app.whiteboards.shape')} value={chosen.shape ?? 'RECTANGLE'} options={SHAPES.map((each) => ({ value: each, label: t(`app.whiteboards.shape_${each.toLowerCase()}`) }))} onchange={(event: Event) => model.update(chosen.id, { shape: (event.currentTarget as HTMLSelectElement).value as Shape })} />
            {/if}
            {#if chosen.kind === 'CONNECTOR'}
              <Select size="sm" label={t('app.whiteboards.route')} value={chosen.route ?? 'STRAIGHT'} options={ROUTES.map((each) => ({ value: each, label: t(`app.whiteboards.route_${each.toLowerCase()}`) }))} onchange={(event: Event) => model.update(chosen.id, { route: (event.currentTarget as HTMLSelectElement).value as Element['route'] })} />
              <Textarea label={t('app.whiteboards.label')} rows={2} value={chosen.text ?? ''} onchange={(event: Event) => model.update(chosen.id, { text: (event.currentTarget as HTMLTextAreaElement).value })} />
            {/if}
            {#if chosen.kind === 'FRAME'}
              <Textarea label={t('app.whiteboards.frame_title')} rows={1} value={chosen.text ?? ''} onchange={(event: Event) => model.update(chosen.id, { text: (event.currentTarget as HTMLTextAreaElement).value })} />
            {/if}
            {#if chosen.kind === 'STICKY'}
              <Button size="sm" tone="secondary" icon="square-check" onclick={() => (isMakingEntry = !isMakingEntry)}>{t('app.whiteboards.make_entry')}</Button>
              {#if isMakingEntry}
                <ul class="choices" aria-label={t('app.whiteboards.entry_in')}>
                  {#each collections as each (each.id)}
                    <li><button type="button" class="choice" onclick={() => makeEntry(each.id)}>{each.name}</button></li>
                  {:else}
                    <li class="quiet">{t('app.whiteboards.no_collections')}</li>
                  {/each}
                </ul>
              {/if}
            {/if}
            <Inline gap="050">
              <Button size="sm" tone="subtle" onclick={() => model.front(chosen.id)}>{t('app.whiteboards.front')}</Button>
              <Button size="sm" tone="subtle" onclick={() => model.back(chosen.id)}>{t('app.whiteboards.back')}</Button>
              <Button size="sm" tone="danger" icon="trash" onclick={remove}>{t('app.whiteboards.remove')}</Button>
            </Inline>
          {/if}
          {#if writable}
            <Button size="sm" tone="subtle" icon="lock" onclick={() => model.update(chosen.id, { locked: !chosen.locked })}>
              {t(chosen.locked ? 'app.whiteboards.unlock' : 'app.whiteboards.lock')}
            </Button>
          {/if}
        </Stack>
      {:else}
        <p class="quiet">{t(writable ? 'app.whiteboards.hint_write' : 'app.whiteboards.hint_read')}</p>
      {/if}

      <Stack gap="050">
        <h3 class="heading">{t('app.whiteboards.frames')}</h3>
        {#each frames as frame (frame.id)}
          <button type="button" class="choice" onclick={() => { view = fit(view, box(frame)); choose([frame.id]); }}>{frame.text || t('app.whiteboards.kind_frame')}</button>
        {:else}
          <p class="quiet">{t('app.whiteboards.no_frames')}</p>
        {/each}
      </Stack>

      <Stack gap="050">
        <h3 class="heading">{t('app.whiteboards.export')}</h3>
        <Inline gap="050">
          <Button size="sm" tone="subtle" onclick={() => saveSvg(chosen?.kind === 'FRAME' ? 'frame' : selected.length > 1 ? 'selection' : 'board')}>SVG</Button>
          <Button size="sm" tone="subtle" onclick={() => void savePng(chosen?.kind === 'FRAME' ? 'frame' : selected.length > 1 ? 'selection' : 'board')}>PNG</Button>
          <Button size="sm" tone="subtle" onclick={() => void saveNative()}>{t('app.whiteboards.export_native')}</Button>
        </Inline>
        <p class="quiet">{t(chosen?.kind === 'FRAME' ? 'app.whiteboards.export_frame' : selected.length > 1 ? 'app.whiteboards.export_selection' : 'app.whiteboards.export_board')}</p>
      </Stack>
    </Stack>
  </aside>
</div>

{#if isPickingEntry}
  <Dialog bind:isOpen={isPickingEntry} title={t('app.whiteboards.put_entry')} dismissLabel={t('app.links.cancel')} onClose={() => (picked = undefined)}>
    <EntryPicker chosen={picked} onchoose={(item) => (picked = item)} />
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isPickingEntry = false)}>{t('app.links.cancel')}</Button>
      <Button tone="primary" disabledReason={picked ? undefined : t('app.links.choose_first')} onclick={putEntry}>{t('app.whiteboards.put_entry_confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .board {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto minmax(0, 1fr);
    gap: var(--sp-100);
    block-size: 100%;
    min-block-size: var(--sp-1000);
  }
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) {
    .board { grid-template-columns: minmax(0, 1fr) var(--layout-panel-width); }
    .side { grid-row: 1 / span 2; grid-column: 2; }
  }
  .bar { padding-block: var(--sp-050); }
  .zoom, .status { color: var(--text-subtle); font-size: var(--fs-075); }
  .stage { position: relative; min-block-size: var(--sp-1000); overflow: hidden; border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface-sunken); }
  .canvas { display: block; inline-size: 100%; block-size: 100%; touch-action: none; cursor: grab; }
  .canvas.drawing { cursor: crosshair; }
  .canvas:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--bw-ring)); }
  .frame path { fill: none; stroke: var(--border-strong); stroke-dasharray: 8 6; }
  .frame-name { fill: var(--text-secondary); font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .connector path { fill: none; stroke: var(--text-secondary); stroke-width: 2; }
  .connector .label { fill: var(--text-secondary); font-size: var(--fs-075); }
  .head { fill: var(--text-secondary); }
  .object path { stroke: none; }
  .object.shape path { fill: var(--bg-raised); stroke: var(--text-primary); stroke-width: 1.5; }
  .words { fill: var(--text-primary); font-size: var(--fs-100); }
  .card path { fill: var(--bg-raised); stroke: var(--border-default); }
  .card.done .words { text-decoration: line-through; fill: var(--text-subtle); }
  .card .key { fill: var(--text-subtle); font-size: var(--fs-075); }
  .card .open { fill: var(--text-brand); font-size: var(--fs-100); }
  .tick rect { fill: var(--bg-surface); stroke: var(--border-strong); }
  .tick.done rect { stroke: var(--text-primary); }
  .tick text { fill: var(--text-primary); font-size: var(--fs-100); }
  .chosen path { stroke: var(--accent-primary); stroke-width: 3; }
  .drafting { fill: none; stroke: var(--accent-primary); stroke-dasharray: 4 4; stroke-width: 2; }
  .handle { fill: var(--accent-primary); cursor: nwse-resize; }
  .editor {
    position: absolute;
    box-sizing: border-box;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--accent-primary);
    background: var(--bg-raised);
    color: var(--text-primary);
    font: inherit;
    resize: none;
  }
  .side { display: grid; align-content: start; gap: var(--sp-150); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .swatches { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .swatch { inline-size: var(--sp-300); block-size: var(--sp-300); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-sm); cursor: pointer; }
  .swatch[aria-checked='true'] { outline: var(--bw-ring) solid var(--focus-ring); }
  .choices { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .choice { all: unset; cursor: pointer; padding: var(--sp-050); border-radius: var(--r-sm); color: var(--text-primary); }
  .choice:hover, .choice:focus-visible { background: var(--bg-surface-hover); }
</style>
