<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A whiteboard drawn and drawn on (M4, UC-CNV-01 … UC-CNV-05): an infinite canvas of sticky
  // notes, shapes, text, connectors attached to objects, frames, cards of entries, freehand strokes
  // and web links, bound to the board's live document, so several people draw at once and see each
  // other's cursors. A reader is connected read-only: they see the drawing live and draw nothing.
  // Only what the viewport shows is drawn (`culled`).
  //
  // **A card is never a copy** (P-19): it names its entry, and what it shows is resolved for the
  // reader; ticking it completes the entry. A sticky note becomes an entry through the ordinary
  // creation of an entry, held to the type's form, and is then that entry's card in its place.
  //
  // **Votes and the timer** live in the live document beside the objects (`Facilitation`), never in
  // the saved scene; an anonymous session keeps each person's votes under a key their device minted.
  //
  // The keyboard reaches everything (research §4.7): Tab and Shift+Tab walk the objects, the
  // arrows move the chosen one, N puts a sticky note in the middle, Enter edits its words, V votes,
  // Ctrl or ⌘ with C, X, V and D copy, cut, paste and duplicate, Delete takes it off; the choice is
  // announced.

  import { untrack } from 'svelte';
  import { tokens } from '@hubtask/design-system';
  import { Button, Checkbox, Dialog, IconButton, Inline, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';
  import { LiveDocument, type LiveSocket, type LiveStatus } from '@hubtask/editor';
  import type {
    ReferenceTarget, WhiteboardBacklinkList, WhiteboardComment, WhiteboardCommentList, WhiteboardRead, WhiteboardVersion, WorkItem,
  } from '@hubtask/sync-engine';
  import {
    Board, DEFAULT_SIZE, Doc, Facilitation, PEN_WIDTH, ROUTES, SHAPES, TEMPLATES, align, bounds, box, connectorPath, copy, culled,
    elementId, fit, fromMinimap, hit, labelPoint, lines, linkHost, minimap, outlinePath, paste, pdfOfJpeg, readCopied, snap, strokePath,
    strokesOf, template, toBoard, toSvg, touches, withGroups, withinFrame, zoomAt,
    type Element, type ElementKind, type Guide, type Pen, type Point, type Shape, type TemplateName, type Viewport,
  } from '@hubtask/whiteboard';

  import { announcer } from '../announce.svelte.ts';
  import EntryPicker from '../entries/EntryPicker.svelte';
  import BoardComments from './BoardComments.svelte';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { entryTypes, entryTypesOffered, offeredTypes, typesShown } from '../data/entrytypes.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { media } from '../data/media.svelte.ts';
  import { resolveRequest } from '../data/links.svelte.ts';
  import {
    BOARD_COLOURS, backlinkHref, boardCommentsPath, boardFileName, cardEntries, clockOf, entryOfNote, notesInFrame, peerColour, pictureColours, pinOf,
    threadsOf, voterKey, whiteboards,
  } from '../data/whiteboards.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { platform } from '../platform/index.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    board: WhiteboardRead;
    /** The hub's collections, offered to hold a sticky note made an entry (UC-CNV-02). */
    collections: readonly { id: string; name: string; settings?: { entry_types?: Record<string, readonly string[]> } }[];
    onnavigate: (path: string) => void;
  }

  const { board, collections, onnavigate }: Props = $props();

  type Tool = 'select' | 'ERASER' | ElementKind;

  // A pause in the drawing before the scene is saved beside the live document.
  const SAVE_AFTER = 1200;
  const NUDGE = 10;
  const GRID = 20;
  const CURSOR_EVERY = 50;
  const MAP = { width: 224, height: 128 };

  const writable = untrack(() => board.writable);
  const saved = untrack(() => (board.elements ?? []) as Element[]);
  const doc = new Doc();
  const model = new Board(doc);
  const session = new Facilitation(doc);
  let elements = $state<Element[]>(untrack(() => saved));
  let status = $state<LiveStatus | 'reading'>('connecting');
  let isLive = $state(false);
  let tool = $state<Tool>('select');
  let shape = $state<Shape>('RECTANGLE');
  let pen = $state<Pen>('PEN');
  let selected = $state<string[]>([]);
  let editing = $state<string | undefined>(undefined);
  let draft = $state('');
  let view = $state<Viewport>({ x: -80, y: -80, zoom: 1, width: 1, height: 1 });
  let surface = $state<SVGSVGElement | undefined>(undefined);
  let cards = $state<Map<string, ReferenceTarget>>(new Map());
  let isPickingEntry = $state(false);
  let picked = $state<WorkItem | undefined>(undefined);
  let isMakingEntry = $state(false);
  let entryKind = $state<string>('');
  let isLinking = $state(false);
  let linkDraft = $state('');
  let linkAt = $state<Point | undefined>(undefined);
  let isGridOn = $state(false);
  let isMapShown = $state(true);
  let guides = $state<Guide[]>([]);
  let chosenTemplate = $state<string>('');

  const byId = $derived(new Map(elements.map((e) => [e.id, e])));
  const shown = $derived(culled(elements, view, 200));
  const frames = $derived(elements.filter((e) => e.kind === 'FRAME'));
  const chosen = $derived(selected.length === 1 ? byId.get(selected[0] ?? '') : undefined);
  const viewBox = $derived(`${view.x} ${view.y} ${view.width / view.zoom} ${view.height / view.zoom}`);

  // --- The live document ---------------------------------------------------------------------------

  let timer: ReturnType<typeof setTimeout> | undefined;
  function changed() {
    // A reader whose board has no live document yet sees the scene as last saved.
    elements = !writable && model.size === 0 ? saved : model.elements();
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

  let connection: LiveDocument | undefined;
  $effect(() => {
    return untrack(() => {
      const stop = model.observe(changed);
      const stopSession = session.observe(sessionChanged);
      connection = new LiveDocument(doc, {
        open: () => whiteboards.liveSocket(board.id) as unknown as LiveSocket,
        writable,
        onStatus: (next) => (status = next),
        onSynced: () => {
          if (isLive) return;
          isLive = true;
          // A board with a saved scene and no live document yet: the first writer seeds it.
          if (writable) model.seed(saved);
          changed();
          // An address naming a frame (`/whiteboards/<id>#<frame>`, a page's embed) opens on it.
          const named = decodeURIComponent(location.hash.slice(1));
          const target = named ? model.get(named) : undefined;
          if (target?.kind === 'FRAME') { view = fit(view, box(target)); selected = [target.id]; }
        },
      });
      const live = connection;
      live.awareness.setLocalState({ name: actor.account?.display_name ?? '', view: viewOf() });
      live.awareness.on('change', peersChanged);
      return () => {
        clearTimeout(timer);
        if (isLive && writable) void save();
        stop();
        stopSession();
        live.awareness.off('change', peersChanged);
        live.destroy();
        connection = undefined;
        doc.destroy();
      };
    });
  });

  // --- Who else is here (research §4.1: cursors, presence, follow) ---------------------------------

  interface Peer { id: number; name: string; cursor?: Point; view?: { x: number; y: number; zoom: number } }
  let peers = $state<Peer[]>([]);
  let following = $state<number | undefined>(undefined);
  let cursorSent = 0;

  const viewOf = () => ({ x: view.x + view.width / view.zoom / 2, y: view.y + view.height / view.zoom / 2, zoom: view.zoom });

  function peersChanged() {
    const live = connection;
    if (!live) return;
    const next: Peer[] = [];
    live.awareness.getStates().forEach((state, id) => {
      if (id === doc.clientID || !state) return;
      const s = state as { name?: unknown; cursor?: Point; view?: Peer['view'] };
      next.push({ id, name: typeof s.name === 'string' && s.name ? s.name : t('app.whiteboards.someone'), cursor: s.cursor, view: s.view });
    });
    peers = next;
    const leader = following === undefined ? undefined : next.find((p) => p.id === following);
    if (following !== undefined && !leader) following = undefined;
    if (leader?.view) {
      const zoom = leader.view.zoom;
      view = { ...view, zoom, x: leader.view.x - view.width / zoom / 2, y: leader.view.y - view.height / zoom / 2 };
    }
  }

  function tellWhere(at?: Point) {
    const live = connection;
    const now = performance.now();
    if (!live || now - cursorSent < CURSOR_EVERY) return;
    cursorSent = now;
    live.awareness.setLocalStateField('view', viewOf());
    if (at) live.awareness.setLocalStateField('cursor', { x: Math.round(at.x), y: Math.round(at.y) });
  }

  $effect(() => {
    void view.x; void view.y; void view.zoom;
    untrack(() => tellWhere());
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

  function centre(): Point {
    return { x: view.x + view.width / view.zoom / 2, y: view.y + view.height / view.zoom / 2 };
  }

  function put(kind: ElementKind, at: Point, extra: Partial<Element> = {}): string | undefined {
    const size = DEFAULT_SIZE[kind];
    const left = Math.round(at.x - size.width / 2);
    const top = Math.round(at.y - size.height / 2);
    const element: Element = {
      id: elementId(), kind, x: isGridOn ? snap(left, GRID) : left, y: isGridOn ? snap(top, GRID) : top, ...size,
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

  function putAll(next: Element[], said: string): void {
    if (next.length === 0) return;
    if (!model.addAll(next)) {
      toast.show({ text: t('whiteboards.too_many_elements', { maximum: '5000' }), tone: 'danger' });
      return;
    }
    selected = next.filter((e) => e.kind !== 'CONNECTOR').map((e) => e.id);
    announcer.say(said);
  }

  function choose(ids: string[]) {
    selected = withGroups(model.elements(), ids);
    const one = ids.length === 1 ? model.get(ids[0] ?? '') : undefined;
    if (one) announcer.say(t('app.whiteboards.chosen', { kind: t(`app.whiteboards.kind_${one.kind.toLowerCase()}`), text: wordsOf(one) }));
  }

  function wordsOf(e: Element): string {
    if (e.kind === 'CARD') return cardOf(e)?.title ?? t('app.whiteboards.card_unavailable');
    if (e.kind === 'LINK') return e.url ?? '';
    if (e.kind === 'IMAGE' || e.kind === 'FILE') return e.text || media.fileNameOf(e.media_id) || '';
    return e.text ?? '';
  }

  function edit(id: string) {
    const e = model.get(id);
    if (!writable || !e || e.kind === 'CARD' || e.kind === 'CONNECTOR' || e.kind === 'STROKE' || e.kind === 'LINK' || e.locked) return;
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
    | { kind: 'move'; startX: number; startY: number; anchor: string; originX: number; originY: number }
    | { kind: 'resize'; id: string; originX: number; originY: number }
    | { kind: 'connect'; from: string; endX: number; endY: number }
    | { kind: 'draw'; points: Point[] }
    | { kind: 'erase' };
  let gesture = $state<Gesture | undefined>(undefined);

  function pointOf(event: PointerEvent | WheelEvent | MouseEvent): Point {
    const rect = surface?.getBoundingClientRect();
    return toBoard(view, { x: event.clientX - (rect?.x ?? 0), y: event.clientY - (rect?.y ?? 0) });
  }

  function erase(at: Point) {
    const gone = shown.filter((e) => e.kind === 'STROKE' && !e.locked && touches(e, at, 8 / view.zoom)).map((e) => e.id);
    if (gone.length > 0) model.remove(gone);
  }

  function down(event: PointerEvent) {
    if (editing) commitEdit();
    if (following !== undefined) following = undefined;
    const at = pointOf(event);
    const target = hit(elements, at);
    surface?.setPointerCapture(event.pointerId);
    if (writable && tool === 'CONNECTOR' && target) {
      gesture = { kind: 'connect', from: target.id, endX: at.x, endY: at.y };
      return;
    }
    if (writable && tool === 'STROKE') {
      gesture = { kind: 'draw', points: [at] };
      return;
    }
    if (writable && tool === 'ERASER') {
      gesture = { kind: 'erase' };
      erase(at);
      return;
    }
    if (writable && tool === 'LINK') {
      linkAt = at;
      linkDraft = '';
      isLinking = true;
      tool = 'select';
      return;
    }
    if (writable && tool !== 'select' && tool !== 'CONNECTOR' && tool !== 'ERASER') {
      const id = put(tool, at);
      tool = 'select';
      if (id && (model.get(id)?.kind ?? '') !== 'FRAME') edit(id);
      return;
    }
    if (target && target.kind !== 'FRAME') {
      if (event.shiftKey) choose(selected.includes(target.id) ? selected.filter((id) => id !== target.id) : [...selected, target.id]);
      else if (!selected.includes(target.id)) choose([target.id]);
      if (writable) gesture = { kind: 'move', startX: at.x, startY: at.y, anchor: target.id, originX: target.x, originY: target.y };
      return;
    }
    if (target && !event.shiftKey) choose([target.id]);
    else if (!target) selected = [];
    gesture = { kind: 'pan', startX: event.clientX, startY: event.clientY, viewX: view.x, viewY: view.y };
  }

  function moveTo(event: PointerEvent) {
    const at = pointOf(event);
    tellWhere(at);
    const g = gesture;
    if (!g) return;
    if (g.kind === 'pan') {
      view = { ...view, x: g.viewX - (event.clientX - g.startX) / view.zoom, y: g.viewY - (event.clientY - g.startY) / view.zoom };
    } else if (g.kind === 'move') {
      moveSelection(g, at);
    } else if (g.kind === 'resize') {
      const w = Math.max(24, Math.round(at.x - g.originX));
      const h = Math.max(24, Math.round(at.y - g.originY));
      model.update(g.id, { width: isGridOn ? Math.max(GRID, snap(w, GRID)) : w, height: isGridOn ? Math.max(GRID, snap(h, GRID)) : h });
    } else if (g.kind === 'draw') {
      g.points.push(at);
      gesture = { kind: 'draw', points: g.points };
    } else if (g.kind === 'erase') {
      erase(at);
    } else {
      gesture = { ...g, endX: at.x, endY: at.y };
    }
  }

  /**
   * Moves the chosen objects with the pointer: the object under it lands on the grid when the grid
   * is on, otherwise aligned to an edge or centre of an object near it, its guide drawn.
   */
  function moveSelection(g: Extract<Gesture, { kind: 'move' }>, at: Point) {
    const anchor = model.get(g.anchor);
    if (!anchor) return;
    let x = Math.round(g.originX + at.x - g.startX);
    let y = Math.round(g.originY + at.y - g.startY);
    if (isGridOn) {
      x = snap(x, GRID);
      y = snap(y, GRID);
      guides = [];
    } else {
      const moving = new Set(selected);
      const all = selected.map((id) => model.get(id)).filter((e): e is Element => !!e && e.kind !== 'CONNECTOR');
      const area = bounds(all);
      if (area) {
        const dx = x - anchor.x;
        const dy = y - anchor.y;
        const aligned = align(
          // design-system-lint-ignore: a box on the board, not a side of the page.
          { left: area.left + dx, top: area.top + dy, right: area.right + dx, bottom: area.bottom + dy },
          shown.filter((e) => !moving.has(e.id) && e.kind !== 'FRAME' && e.kind !== 'STROKE'),
          6 / view.zoom,
        );
        x += Math.round(aligned.dx);
        y += Math.round(aligned.dy);
        guides = aligned.guides;
      }
    }
    if (x !== anchor.x || y !== anchor.y) model.move(selected, x - anchor.x, y - anchor.y);
  }

  function up(event: PointerEvent) {
    const g = gesture;
    gesture = undefined;
    guides = [];
    surface?.releasePointerCapture(event.pointerId);
    if (g?.kind === 'draw') {
      const strokes = strokesOf(g.points, pen, () => elementId(), pen === 'HIGHLIGHTER' ? 'amber' : undefined);
      if (!model.addAll(strokes)) toast.show({ text: t('whiteboards.too_many_elements', { maximum: '5000' }), tone: 'danger' });
      return;
    }
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
    following = undefined;
    const rect = surface?.getBoundingClientRect();
    const screen = { x: event.clientX - (rect?.x ?? 0), y: event.clientY - (rect?.y ?? 0) };
    if (event.ctrlKey || event.metaKey) view = zoomAt(view, screen, event.deltaY < 0 ? 1.1 : 1 / 1.1);
    else view = { ...view, x: view.x + event.deltaX / view.zoom, y: view.y + event.deltaY / view.zoom };
  }

  // --- Keyboard and clipboard ----------------------------------------------------------------------

  function key(event: KeyboardEvent) {
    if (editing) return;
    const order = elements.filter((e) => e.kind !== 'CONNECTOR');
    if (event.key === 'Tab' && order.length > 0) {
      event.preventDefault();
      const at = order.findIndex((e) => e.id === selected[0]);
      const next = order[(at + (event.shiftKey ? order.length - 1 : 1) + (at < 0 && event.shiftKey ? 1 : 0)) % order.length];
      if (next) {
        selected = [next.id];
        choose([next.id]);
        const b = box(next);
        const seen = { x: view.x, y: view.y, w: view.width / view.zoom, h: view.height / view.zoom };
        if (b.left < seen.x || b.top < seen.y || b.right > seen.x + seen.w || b.bottom > seen.y + seen.h) {
          view = { ...view, x: b.left - seen.w / 4, y: b.top - seen.h / 4 };
        }
      }
      return;
    }
    if (event.key === 'Escape') { selected = []; tool = 'select'; following = undefined; return; }
    if (!writable) return;
    const mod = event.ctrlKey || event.metaKey;
    if (mod && (event.key === 'd' || event.key === 'D') && selected.length > 0) {
      event.preventDefault();
      duplicate();
      return;
    }
    const step = event.shiftKey ? NUDGE * 5 : NUDGE;
    const moves: Record<string, [number, number]> = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] };
    const by = moves[event.key];
    if (by && selected.length > 0) { event.preventDefault(); model.move(selected, by[0], by[1]); return; }
    if ((event.key === 'Delete' || event.key === 'Backspace') && selected.length > 0) { event.preventDefault(); remove(); return; }
    if (event.key === 'Enter' && chosen) { event.preventDefault(); edit(chosen.id); return; }
    if (mod) return;
    if ((event.key === 'v' || event.key === 'V') && chosen && voting?.open) { event.preventDefault(); vote(chosen.id); return; }
    if (event.key === 'n' || event.key === 'N') { event.preventDefault(); const id = put('STICKY', centre()); if (id) edit(id); }
  }

  function remove() {
    model.remove(selected);
    selected = [];
    announcer.say(t('app.whiteboards.removed'));
  }

  function duplicate() {
    const copied = copy(elements, selected);
    const area = bounds(copied.elements);
    if (!area) return;
    const pasted = paste(copied, { x: (area.left + area.right) / 2 + 24, y: (area.top + area.bottom) / 2 + 24 }, () => elementId());
    putAll(pasted, t('app.whiteboards.pasted', { count: String(pasted.length) }));
  }

  const focused = () => !!surface && document.activeElement === surface && !editing;

  function copied(event: ClipboardEvent, cut: boolean) {
    if (!focused() || selected.length === 0 || !event.clipboardData) return;
    event.preventDefault();
    event.clipboardData.setData('text/plain', JSON.stringify(copy(elements, selected)));
    announcer.say(t('app.whiteboards.copied', { count: String(selected.length) }));
    if (cut && writable) remove();
  }

  function pasted(event: ClipboardEvent) {
    if (!focused() || !writable) return;
    const what = readCopied(event.clipboardData?.getData('text/plain') ?? '');
    if (!what) return;
    event.preventDefault();
    const next = paste(what, centre(), () => elementId());
    putAll(next, t('app.whiteboards.pasted', { count: String(next.length) }));
  }

  // --- Groups --------------------------------------------------------------------------------------

  const isGrouped = $derived(selected.some((id) => !!byId.get(id)?.group));
  function group() {
    model.group(selected, elementId());
    announcer.say(t('app.whiteboards.grouped', { count: String(selected.length) }));
  }
  function ungroup() {
    model.group(selected, null);
    announcer.say(t('app.whiteboards.ungrouped'));
  }

  // --- Web links -----------------------------------------------------------------------------------

  const linkError = $derived.by(() => {
    if (!linkDraft.trim()) return undefined;
    try {
      const url = new URL(linkDraft.trim());
      return url.protocol === 'https:' || url.protocol === 'http:' ? undefined : t('whiteboards.link_url_invalid');
    } catch {
      return t('whiteboards.link_url_invalid');
    }
  });

  function putLink() {
    const url = linkDraft.trim();
    isLinking = false;
    if (!url || linkError) return;
    put('LINK', linkAt ?? centre(), { url });
  }

  // --- Pictures and files (UC-CNV-08) -------------------------------------------------------------

  let isUploading = $state(false);

  /** An upload made one of the board's files, then drawn: a picture as itself, anything else as its card. */
  async function putFile(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file) return;
    const at = centre();
    isUploading = true;
    try {
      const object = await media.upload(file, 'ATTACHMENT', { idempotencyKey: crypto.randomUUID() });
      await whiteboards.placeMedia(board.id, object.id);
      const picture = (object.content_type ?? '').startsWith('image/');
      put(picture ? 'IMAGE' : 'FILE', at, { media_id: object.id });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isUploading = false;
    }
  }

  async function openFile(mediaId: string | undefined) {
    if (!mediaId) return;
    const url = await media.downloadUrl(mediaId, Date.now());
    if (url) window.open(url, '_blank', 'noopener');
    else toast.show({ text: t('app.whiteboards.file_unavailable'), tone: 'danger' });
  }

  $effect(() => {
    media.resolve(elements.filter((e) => e.kind === 'FILE' || e.kind === 'IMAGE').map((e) => e.media_id));
  });

  // --- Templates (research §4.3) -------------------------------------------------------------------

  function putTemplate(name: TemplateName) {
    chosenTemplate = '';
    const next = template(name, { at: centre(), words: (word) => t(`app.whiteboards.word_${word}`), newId: () => elementId() });
    putAll(next, t('app.whiteboards.template_put', { name: t(`app.whiteboards.template_${name.toLowerCase()}`) }));
    const area = bounds(next);
    if (area) view = fit(view, area);
  }

  // --- A sticky note made an entry; a frame of them at once; an entry put on the board (UC-CNV-02) -

  /** The entry types a collection offers at the task level, where the workflows module is on (UC-FLW-01). */
  $effect(() => untrack(() => (writable ? entryTypes.open() : undefined)));
  const kindsIn = (collectionId: string) => {
    if (!typesShown(manifest.value)) return [];
    const settings = collections.find((each) => each.id === collectionId)?.settings;
    return offeredTypes(entryTypes.all, 'TASK', entryTypesOffered(settings, 'TASK'));
  };
  let entryCollection = $state<string>('');
  const kinds = $derived(entryCollection ? kindsIn(entryCollection) : []);
  const notesToTurn = $derived(chosen?.kind === 'FRAME' ? notesInFrame(elements, chosen) : chosen?.kind === 'STICKY' || chosen?.kind === 'SHAPE' ? [chosen] : []);

  async function makeEntries() {
    isMakingEntry = false;
    const notes = notesToTurn;
    const collectionId = entryCollection;
    if (!collectionId) return;
    if (notes.length === 0 || notes.every((note) => !(note.text ?? '').trim())) {
      toast.show({ text: t('app.whiteboards.entry_needs_words'), tone: 'warning' });
      return;
    }
    const kind = kinds.find((each) => each.key === entryKind) ?? kinds[0];
    let made = 0;
    let lastTitle = '';
    for (const note of notes) {
      const body = entryOfNote(collectionId, note.text ?? '', kind);
      if (!body) continue;
      try {
        const entry = await items.create(body as never, crypto.randomUUID());
        model.toCard(note.id, entry.id);
        made += 1;
        lastTitle = entry.title;
      } catch (error) {
        toast.show({ text: t('app.whiteboards.entry_failed', { reason: renderProblem(error as never, messages).message }), tone: 'danger' });
        break;
      }
    }
    if (made === 1) toast.show({ text: t('app.whiteboards.entry_made', { title: lastTitle }), tone: 'success' });
    else if (made > 1) toast.show({ text: t('app.whiteboards.entries_made', { count: String(made) }), tone: 'success' });
  }

  function putEntry() {
    const entry = picked;
    isPickingEntry = false;
    picked = undefined;
    if (entry) put('CARD', centre(), { target: { kind: 'ENTRY', id: entry.id } });
  }

  // --- Voting and the timer (research §4.2) --------------------------------------------------------

  let voting = $state(untrack(() => session.voting));
  let results = $state(untrack(() => session.results()));
  let timerState = $state(untrack(() => session.timer));
  let now = $state(Date.now());
  let votesPerPerson = $state('3');
  let isAnonymous = $state(false);
  let timerMinutes = $state('5');
  let wasRunning = false;

  function sessionChanged() {
    voting = session.voting;
    results = session.results();
    timerState = session.timer;
  }

  const minted = new Map<string, string>();
  function myKey(): string {
    const current = voting;
    const account = actor.account?.id ?? '';
    if (!current) return account;
    let kept = minted.get(current.id);
    if (current.anonymous && !kept) {
      try { kept = localStorage.getItem(`hubtask.board.voter.${current.id}`) ?? undefined; } catch { kept = undefined; }
    }
    const key = voterKey(current.anonymous, account, () => crypto.randomUUID(), kept);
    if (current.anonymous && key !== kept) {
      minted.set(current.id, key);
      try { localStorage.setItem(`hubtask.board.voter.${current.id}`, key); } catch { /* a private window keeps the key for this visit only */ }
    }
    return key;
  }

  const myVotes = $derived(voting ? (void results, session.votesOf(myKey())) : []);
  const votesLeft = $derived(voting ? Math.max(0, voting.perPerson - myVotes.length) : 0);

  function vote(id: string) {
    if (session.vote(myKey(), id)) announcer.say(t('app.whiteboards.voted', { remaining: String(Math.max(0, votesLeft - 1)) }));
    else announcer.say(t('app.whiteboards.no_votes_left'));
  }

  function unvote(id: string) {
    if (session.unvote(myKey(), id)) announcer.say(t('app.whiteboards.unvoted'));
  }

  function startVoting() {
    session.startVoting({ id: elementId(), perPerson: Number(votesPerPerson), anonymous: isAnonymous, startedBy: actor.account?.id ?? '' });
    announcer.say(t('app.whiteboards.voting_started'));
  }

  const remaining = $derived(timerState ? session.remaining(now) : undefined);
  $effect(() => {
    if (!timerState?.endsAt) return;
    const every = setInterval(() => (now = Date.now()), 250);
    return () => clearInterval(every);
  });
  $effect(() => {
    const running = remaining?.running ?? false;
    untrack(() => {
      if (wasRunning && !running && timerState?.endsAt !== undefined) announcer.say(t('app.whiteboards.time_up'));
      wasRunning = running;
    });
  });

  const resultsShown = $derived(
    [...results.counts].slice(0, 10).map(([id, count]) => ({ id, count, element: byId.get(id) })).filter((row) => row.element),
  );

  // --- Export (UC-CNV-03) --------------------------------------------------------------------------

  type Which = 'board' | 'frame' | 'selection';
  const which = $derived<Which>(chosen?.kind === 'FRAME' ? 'frame' : selected.length > 1 ? 'selection' : 'board');

  function picture(part: Which): Element[] {
    if (part === 'frame' && chosen?.kind === 'FRAME') return withinFrame(elements, chosen);
    if (part === 'selection' && selected.length > 0) {
      const ids = new Set(selected);
      return elements.filter((e) => ids.has(e.id) || (e.kind === 'CONNECTOR' && ids.has(e.from ?? '') && ids.has(e.to ?? '')));
    }
    return elements;
  }

  /**
   * The pictures of an export as their own bytes (`data:`), so the file stands on its own and the
   * canvas a PNG or PDF is drawn on is not tainted. A picture the browser may not read back - a
   * storage of another origin that does not allow it - is left out and drawn as its place and name.
   */
  async function inlined(part: readonly Element[]): Promise<Map<string, string>> {
    const out = new Map<string, string>();
    const ids = [...new Set(part.flatMap((e) => (e.kind === 'IMAGE' && e.media_id ? [e.media_id] : [])))];
    await Promise.all(ids.map(async (id) => {
      try {
        const url = await media.downloadUrl(id, Date.now());
        if (!url) return;
        const image = new Image();
        image.crossOrigin = 'anonymous';
        await new Promise<void>((resolve, reject) => {
          image.onload = () => resolve();
          image.onerror = () => reject(new Error('picture'));
          image.src = url;
        });
        const canvas = document.createElement('canvas');
        canvas.width = image.naturalWidth;
        canvas.height = image.naturalHeight;
        canvas.getContext('2d')?.drawImage(image, 0, 0);
        out.set(id, canvas.toDataURL('image/png'));
      } catch {
        // Unreadable here: the export draws its place and name.
      }
    }));
    return out;
  }

  async function svgOf(part: Which): Promise<string> {
    const shown = picture(part);
    const pictures = await inlined(shown);
    return toSvg(shown, {
      title: board.title, colours: pictureColours(), unavailable: t('app.whiteboards.card_unavailable'),
      media: (id) => ({ href: pictures.get(id), name: media.fileNameOf(id) }),
      card: (id) => {
        const target = cards.get(id.toLowerCase());
        return target?.available ? { title: target.title ?? '', key: target.key ?? null, completed: target.completed ?? false } : undefined;
      },
    });
  }

  async function saveSvg(part: Which) {
    platform.saveFile(new Blob([await svgOf(part)], { type: 'image/svg+xml' }), boardFileName(board.title, 'svg'));
  }

  /** The picture drawn on a canvas at twice its size, for PNG and PDF. */
  async function rendered(part: Which): Promise<{ canvas: HTMLCanvasElement; width: number; height: number }> {
    const url = URL.createObjectURL(new Blob([await svgOf(part)], { type: 'image/svg+xml' }));
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
      return { canvas, width: image.width, height: image.height };
    } finally {
      URL.revokeObjectURL(url);
    }
  }

  async function savePng(part: Which) {
    try {
      const { canvas } = await rendered(part);
      const png = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/png'));
      if (png) platform.saveFile(png, boardFileName(board.title, 'png'));
    } catch {
      toast.show({ text: t('app.whiteboards.export_failed'), tone: 'danger' });
    }
  }

  async function savePdf(part: Which) {
    try {
      const { canvas, width, height } = await rendered(part);
      const jpeg = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', 0.92));
      if (!jpeg) throw new Error('picture');
      const bytes = pdfOfJpeg(new Uint8Array(await jpeg.arrayBuffer()), canvas.width, canvas.height, board.title, { width, height });
      platform.saveFile(new Blob([bytes as BlobPart], { type: 'application/pdf' }), boardFileName(board.title, 'pdf'));
    } catch {
      toast.show({ text: t('app.whiteboards.export_failed'), tone: 'danger' });
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

  // --- The history (UC-CNV-06) --------------------------------------------------------------------

  let history = $state<readonly WhiteboardVersion[] | undefined>(undefined);
  let namingSeq = $state<number | undefined>(undefined);
  let nameDraft = $state('');

  async function openHistory() {
    try {
      history = await whiteboards.versions(board.id);
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  /** An older version brought back: its objects in place of the scene, for everyone drawing; the save that follows is a version of its own. */
  async function bringBack(seq: number) {
    try {
      const version = await whiteboards.version(board.id, seq);
      if (!model.replace((version.elements ?? []) as Element[])) return;
      selected = [];
      toast.show({ text: t('app.whiteboards.version_back', { when: formatDateTime(version.created_at, messages.locale) }), tone: 'success' });
      await save();
      await openHistory();
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  async function nameVersion(seq: number) {
    namingSeq = undefined;
    try {
      await whiteboards.nameVersion(board.id, seq, nameDraft);
      await openHistory();
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  // --- What points at the board (UC-CNV-07) ------------------------------------------------------

  let backlinks = $state<WhiteboardBacklinkList['data'] | undefined>(undefined);

  async function openBacklinks() {
    try {
      backlinks = await whiteboards.backlinks(board.id);
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  // --- Comments pinned to objects and places (UC-CNV-09) -----------------------------------------

  let comments = $state<WhiteboardComment[]>([]);
  let openThread = $state<string | undefined>(undefined);
  let withResolved = $state(false);
  $effect(() => {
    const id = board.id;
    return untrack(() =>
      engine.subscribe<WhiteboardCommentList>({ path: boardCommentsPath(id) }, (next) => {
        if (next.status === 'ready') comments = next.data.data;
      }),
    );
  });
  const pins = $derived(threadsOf(comments, withResolved).map(({ thread, replies }) => ({ thread, count: 1 + replies.length, at: pinOf(thread, byId) })));

  /** A new thread stands at the chosen object's corner, or in the middle of the view. */
  function pinHere(): { element_id?: string; x: number; y: number } {
    if (chosen) {
      const at = pinOf({ id: '', parent_id: null, element_id: chosen.id, x: 0, y: 0, resolved: false, deleted: false }, byId);
      return { element_id: chosen.id, x: Math.round(at.x), y: Math.round(at.y) };
    }
    const at = centre();
    return { x: Math.round(at.x), y: Math.round(at.y) };
  }

  function showThread(comment: WhiteboardComment) {
    const at = pinOf(comment, byId);
    following = undefined;
    view = { ...view, x: at.x - view.width / view.zoom / 2, y: at.y - view.height / view.zoom / 2 };
    if (comment.element_id && byId.has(comment.element_id)) choose([comment.element_id]);
  }

  // --- The minimap ---------------------------------------------------------------------------------

  const map = $derived(isMapShown ? minimap(elements, view, MAP) : undefined);
  function mapClick(event: MouseEvent) {
    const target = event.currentTarget as SVGSVGElement;
    const rect = target.getBoundingClientRect();
    if (!map || rect.width === 0) return;
    following = undefined;
    view = fromMinimap(map, view, { x: ((event.clientX - rect.x) / rect.width) * MAP.width, y: ((event.clientY - rect.y) / rect.height) * MAP.height });
  }

  const statusText = $derived(
    status === 'refused' ? t('app.whiteboards.status_refused')
    : status === 'offline' ? t('app.whiteboards.status_offline')
    : !isLive ? t('app.whiteboards.status_connecting')
    : writable ? t('app.whiteboards.status_live')
    : t('app.whiteboards.status_reading'),
  );

  const fillOf = (e: Element) =>
    e.color_token && (BOARD_COLOURS as readonly string[]).includes(e.color_token)
      ? tokens.label[e.color_token as (typeof BOARD_COLOURS)[number]].bg
      : tokens.bg.raised;
  const inkOf = (e: Element) =>
    e.color_token && (BOARD_COLOURS as readonly string[]).includes(e.color_token)
      ? tokens.label[e.color_token as (typeof BOARD_COLOURS)[number]].fg
      : tokens.text.primary;
  const tools: readonly { tool: Tool; icon: 'hand' | 'square' | 'pencil' | 'arrow-right' | 'layout-template' | 'file-text' | 'zap' | 'x' | 'link' }[] = [
    { tool: 'select', icon: 'hand' }, { tool: 'STICKY', icon: 'square' }, { tool: 'SHAPE', icon: 'layout-template' },
    { tool: 'TEXT', icon: 'file-text' }, { tool: 'CONNECTOR', icon: 'arrow-right' }, { tool: 'FRAME', icon: 'file-text' },
    { tool: 'STROKE', icon: 'pencil' }, { tool: 'ERASER', icon: 'x' }, { tool: 'LINK', icon: 'link' },
  ];
  const editingElement = $derived(editing ? byId.get(editing) : undefined);
  const drafting = $derived(gesture?.kind === 'draw' ? gesture.points : undefined);
</script>

<svelte:window oncopy={(event) => copied(event, false)} oncut={(event) => copied(event, true)} onpaste={pasted} />

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
        {#if tool === 'STROKE'}
          <Select size="sm" label={t('app.whiteboards.pen')} bind:value={pen} options={(['PEN', 'HIGHLIGHTER'] as const).map((each) => ({ value: each, label: t(`app.whiteboards.pen_${each.toLowerCase()}`) }))} />
        {/if}
        <Button size="sm" tone="subtle" icon="plus" onclick={() => (isPickingEntry = true)}>{t('app.whiteboards.put_entry')}</Button>
        <label class="upload">
          <span>{isUploading ? t('app.whiteboards.uploading') : t('app.whiteboards.put_file')}</span>
          <input type="file" disabled={isUploading} onchange={(event) => void putFile(event)} />
        </label>
        <Select
          size="sm"
          label={t('app.whiteboards.template')}
          placeholder={t('app.whiteboards.template_choose')}
          bind:value={chosenTemplate}
          options={TEMPLATES.map((each) => ({ value: each, label: t(`app.whiteboards.template_${each.toLowerCase()}`) }))}
          onchange={(event: Event) => { const name = (event.currentTarget as HTMLSelectElement).value; if (name) putTemplate(name as TemplateName); }}
        />
      {/if}
      <IconButton size="sm" icon="minus" label={t('app.whiteboards.zoom_out')} onclick={() => (view = zoomAt(view, { x: view.width / 2, y: view.height / 2 }, 1 / 1.25))} />
      <span class="zoom">{Math.round(view.zoom * 100)}%</span>
      <IconButton size="sm" icon="plus" label={t('app.whiteboards.zoom_in')} onclick={() => (view = zoomAt(view, { x: view.width / 2, y: view.height / 2 }, 1.25))} />
      <Button size="sm" tone="subtle" onclick={() => { const all = bounds(elements); if (all) view = fit(view, all); }}>{t('app.whiteboards.fit')}</Button>
      <Button size="sm" tone={isGridOn ? 'primary' : 'subtle'} icon="columns-3" aria-pressed={isGridOn} onclick={() => (isGridOn = !isGridOn)}>{t('app.whiteboards.grid')}</Button>
      <Button size="sm" tone={isMapShown ? 'primary' : 'subtle'} icon="compass" aria-pressed={isMapShown} onclick={() => (isMapShown = !isMapShown)}>{t('app.whiteboards.minimap')}</Button>
      {#if remaining}
        <span class="clock" class:over={!remaining.running && remaining.left === 0} role="timer" aria-label={t('app.whiteboards.timer_left', { time: clockOf(remaining.left) })}>{clockOf(remaining.left)}</span>
      {/if}
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
      ondblclick={(event) => { const target = hit(elements, pointOf(event)); if (target) edit(target.id); }}
    >
      <defs>
        <marker id="wb-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z" class="head" />
        </marker>
        <pattern id="wb-grid" width={GRID} height={GRID} patternUnits="userSpaceOnUse">
          <circle cx="1" cy="1" r="1" class="dot" />
        </pattern>
      </defs>
      {#if isGridOn}
        <rect class="grid" x={view.x} y={view.y} width={view.width / view.zoom} height={view.height / view.zoom} fill="url(#wb-grid)" />
      {/if}
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
        {:else if e.kind === 'STROKE'}
          <path
            class="stroke"
            class:highlighter={e.pen === 'HIGHLIGHTER'}
            class:chosen={selected.includes(e.id)}
            d={strokePath(e)}
            style:stroke={inkOf(e)}
            style:stroke-width={String(PEN_WIDTH[e.pen ?? 'PEN'])}
          />
        {:else if e.kind === 'LINK'}
          <g class="card" class:chosen={selected.includes(e.id)}>
            <path d={outlinePath(e)} />
            <text x={e.x + 12} y={e.y + 24} class="words">{linkHost(e.url ?? '')}</text>
            <text x={e.x + 12} y={e.y + 46} class="key">{lines(e.url ?? '', e.width - 48, 11)[0] ?? ''}</text>
            <a href={e.url} target="_blank" rel="noopener noreferrer" aria-label={t('app.whiteboards.link_open', { host: linkHost(e.url ?? '') })}>
              <text x={e.x + e.width - 12} y={e.y + 24} text-anchor="end" class="open">↗</text>
            </a>
          </g>
        {:else if e.kind === 'IMAGE'}
          {@const href = media.coverUrl(e.media_id, Date.now())}
          <g class="card" class:chosen={selected.includes(e.id)}>
            {#if href}
              <image {href} x={e.x} y={e.y} width={e.width} height={e.height} preserveAspectRatio="xMidYMid meet">
                <title>{wordsOf(e)}</title>
              </image>
            {:else}
              <path d={outlinePath(e)} class="pending" />
              <text x={e.x + 12} y={e.y + 24} class="key">{media.fileNameOf(e.media_id) ?? t('app.whiteboards.card_unavailable')}</text>
            {/if}
            {#if e.text}<text x={e.x} y={e.y + e.height + 18} class="words">{e.text}</text>{/if}
          </g>
        {:else if e.kind === 'FILE'}
          <g class="card" class:chosen={selected.includes(e.id)}>
            <path d={outlinePath(e)} />
            <text x={e.x + 12} y={e.y + 28} class="words">{lines(media.fileNameOf(e.media_id) ?? t('app.whiteboards.card_unavailable'), e.width - 48, 14)[0] ?? ''}</text>
            {#if e.text}<text x={e.x + 12} y={e.y + 50} class="key">{lines(e.text, e.width - 48, 11)[0] ?? ''}</text>{/if}
            <g
              role="button"
              tabindex="-1"
              class="open"
              aria-label={t('app.whiteboards.file_open', { name: media.fileNameOf(e.media_id) ?? '' })}
              onclick={() => void openFile(e.media_id)}
              onkeydown={(event) => { if (event.key === 'Enter') void openFile(e.media_id); }}
            ><text x={e.x + e.width - 12} y={e.y + 28} text-anchor="end" class="open">↗</text></g>
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
        {#if results.counts.get(e.id)}
          <g class="votes" aria-label={t('app.whiteboards.votes_on', { count: String(results.counts.get(e.id)) })}>
            <circle cx={e.x + e.width} cy={e.y} r={14 / view.zoom} />
            <text x={e.x + e.width} y={e.y + 5 / view.zoom} text-anchor="middle" font-size={13 / view.zoom}>{results.counts.get(e.id)}</text>
          </g>
        {/if}
      {/each}
      {#each guides as guide, i (i)}
        {#if guide.axis === 'x'}
          <line class="guide" x1={guide.at} x2={guide.at} y1={guide.from} y2={guide.to} />
        {:else}
          <line class="guide" y1={guide.at} y2={guide.at} x1={guide.from} x2={guide.to} />
        {/if}
      {/each}
      {#if drafting && drafting.length > 0}
        <polyline class="stroke" class:highlighter={pen === 'HIGHLIGHTER'} points={drafting.map((p) => `${p.x},${p.y}`).join(' ')} style:stroke-width={String(PEN_WIDTH[pen])} />
      {/if}
      {#if gesture?.kind === 'connect'}
        {@const from = byId.get(gesture.from)}
        {#if from}<path class="drafting" d={`M ${from.x + from.width / 2} ${from.y + from.height / 2} L ${gesture.endX} ${gesture.endY}`} />{/if}
      {/if}
      {#if writable && chosen && chosen.kind !== 'CONNECTOR' && chosen.kind !== 'STROKE' && !chosen.locked}
        <rect class="handle" x={chosen.x + chosen.width - 6} y={chosen.y + chosen.height - 6} width="12" height="12" onpointerdown={(event) => resizeStart(event, chosen)} role="presentation" />
      {/if}
      {#each pins as pin (pin.thread.id)}
        <g
          class="pin"
          class:open={openThread === pin.thread.id}
          class:resolved={pin.thread.resolved}
          role="button"
          tabindex="-1"
          aria-label={t('app.whiteboards.comment_pin', { count: String(pin.count) })}
          transform={`translate(${pin.at.x} ${pin.at.y}) scale(${1 / view.zoom})`}
          onpointerdown={(event) => { event.stopPropagation(); openThread = pin.thread.id; }}
          onkeydown={(event) => { if (event.key === 'Enter') openThread = pin.thread.id; }}
        >
          <path d="M 0 0 L 0 -22 Q 0 -30 8 -30 L 26 -30 Q 34 -30 34 -22 L 34 -12 Q 34 -4 26 -4 L 8 -4 Z" />
          <text x="17" y="-12" text-anchor="middle">{pin.count}</text>
        </g>
      {/each}
      {#each peers as peer (peer.id)}
        {#if peer.cursor}
          <g class="peer" transform={`translate(${peer.cursor.x} ${peer.cursor.y}) scale(${1 / view.zoom})`} style:color={tokens.label[peerColour(peer.id)].fg}>
            <path d="M 0 0 L 0 16 L 4 12 L 8 20 L 11 19 L 7 11 L 12 11 Z" />
            <text x="14" y="26">{peer.name}</text>
          </g>
        {/if}
      {/each}
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
    {#if map}
      <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
      <svg class="minimap" role="img" aria-label={t('app.whiteboards.minimap')} viewBox={`0 0 ${MAP.width} ${MAP.height}`} onclick={mapClick}>
        {#each map.boxes as b (b.id)}
          <rect class={`mini ${b.kind.toLowerCase()}`} x={b.left} y={b.top} width={b.width} height={b.height} />
        {/each}
        <rect class="mini-view" x={map.view.left} y={map.view.top} width={map.view.width} height={map.view.height} />
      </svg>
    {/if}
  </div>

  <aside class="side" aria-label={t('app.whiteboards.side')}>
    <Stack gap="150">
      {#if chosen}
        <Stack gap="100">
          <h3 class="heading">{t(`app.whiteboards.kind_${chosen.kind.toLowerCase()}`)}</h3>
          {#if writable && !chosen.locked}
            {#if chosen.kind === 'STICKY' || chosen.kind === 'SHAPE' || chosen.kind === 'STROKE'}
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
            {#if chosen.kind === 'LINK'}
              <Input size="sm" label={t('app.whiteboards.link_address')} value={chosen.url ?? ''} onchange={(event: Event) => { const url = (event.currentTarget as HTMLInputElement).value.trim(); if (url) model.update(chosen.id, { url }); }} />
            {/if}
            {#if chosen.kind === 'STICKY' || chosen.kind === 'SHAPE' || (chosen.kind === 'FRAME' && notesToTurn.length > 0)}
              <Button size="sm" tone="secondary" icon="square-check" onclick={() => { isMakingEntry = !isMakingEntry; entryCollection = collections[0]?.id ?? ''; entryKind = ''; }}>
                {chosen.kind === 'FRAME' ? t('app.whiteboards.make_entries', { count: String(notesToTurn.length) }) : t('app.whiteboards.make_entry')}
              </Button>
              {#if isMakingEntry}
                {#if collections.length === 0}
                  <p class="quiet">{t('app.whiteboards.no_collections')}</p>
                {:else}
                  <Select size="sm" label={t('app.whiteboards.entry_in')} bind:value={entryCollection} options={collections.map((each) => ({ value: each.id, label: each.name }))} />
                  {#if kinds.length > 1}
                    <Select size="sm" label={t('app.whiteboards.entry_type')} bind:value={entryKind} options={kinds.map((each) => ({ value: each.key, label: each.name ?? t('app.whiteboards.entry_type_task') }))} />
                  {/if}
                  <Button size="sm" tone="primary" onclick={() => void makeEntries()}>{t('app.whiteboards.make_entry_confirm')}</Button>
                {/if}
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
      {:else if selected.length > 1}
        <Stack gap="100">
          <h3 class="heading">{t('app.whiteboards.several', { count: String(selected.length) })}</h3>
          {#if writable}
            <Inline gap="050">
              {#if isGrouped}
                <Button size="sm" tone="subtle" icon="folder-plus" onclick={ungroup}>{t('app.whiteboards.ungroup')}</Button>
              {:else}
                <Button size="sm" tone="subtle" icon="folder-plus" onclick={group}>{t('app.whiteboards.group')}</Button>
              {/if}
              <Button size="sm" tone="subtle" icon="copy" onclick={duplicate}>{t('app.whiteboards.duplicate')}</Button>
              <Button size="sm" tone="danger" icon="trash" onclick={remove}>{t('app.whiteboards.remove')}</Button>
            </Inline>
          {/if}
        </Stack>
      {:else}
        <p class="quiet">{t(writable ? 'app.whiteboards.hint_write' : 'app.whiteboards.hint_read')}</p>
      {/if}

      {#if voting || writable}
        <Stack gap="050">
          <h3 class="heading">{t('app.whiteboards.voting')}</h3>
          {#if voting?.open}
            <p class="quiet">{t(voting.anonymous ? 'app.whiteboards.voting_open_anonymous' : 'app.whiteboards.voting_open', { remaining: String(votesLeft), count: String(voting.perPerson) })}</p>
            {#if writable && chosen}
              <Inline gap="050">
                <Button size="sm" tone="secondary" icon="star" disabledReason={votesLeft > 0 ? undefined : t('app.whiteboards.no_votes_left')} onclick={() => vote(chosen.id)}>{t('app.whiteboards.vote')}</Button>
                {#if myVotes.includes(chosen.id)}
                  <Button size="sm" tone="subtle" onclick={() => unvote(chosen.id)}>{t('app.whiteboards.unvote')}</Button>
                {/if}
              </Inline>
            {/if}
            {#if writable}<Button size="sm" tone="subtle" onclick={() => session.endVoting()}>{t('app.whiteboards.voting_end')}</Button>{/if}
          {:else if writable}
            <Inline gap="050" align="end">
              <Select size="sm" label={t('app.whiteboards.votes_per_person')} bind:value={votesPerPerson} options={['1', '2', '3', '5', '10'].map((n) => ({ value: n, label: n }))} />
              <Checkbox label={t('app.whiteboards.anonymous')} bind:checked={isAnonymous} />
            </Inline>
            <Button size="sm" tone="secondary" icon="star" onclick={startVoting}>{t('app.whiteboards.voting_start')}</Button>
          {/if}
          {#if voting}
            <p class="quiet">{t('app.whiteboards.voters', { count: String(results.voters) })}</p>
            <ol class="choices" aria-label={t('app.whiteboards.results')}>
              {#each resultsShown as row (row.id)}
                <li><button type="button" class="choice" onclick={() => row.element && choose([row.element.id])}>{row.count} · {wordsOf(row.element as Element) || t(`app.whiteboards.kind_${(row.element as Element).kind.toLowerCase()}`)}</button></li>
              {/each}
            </ol>
            {#if writable && !voting.open}<Button size="sm" tone="subtle" onclick={() => session.clearVoting()}>{t('app.whiteboards.voting_clear')}</Button>{/if}
          {/if}
        </Stack>
      {/if}

      {#if timerState || writable}
        <Stack gap="050">
          <h3 class="heading">{t('app.whiteboards.timer')}</h3>
          {#if remaining}
            <p class="quiet">{remaining.running ? t('app.whiteboards.timer_left', { time: clockOf(remaining.left) }) : remaining.left === 0 ? t('app.whiteboards.time_up') : t('app.whiteboards.timer_paused', { time: clockOf(remaining.left) })}</p>
          {/if}
          {#if writable}
            <Inline gap="050" align="end">
              {#if !remaining || (!remaining.running && remaining.left === 0)}
                <Select size="sm" label={t('app.whiteboards.timer_minutes')} bind:value={timerMinutes} options={['1', '3', '5', '10', '15', '30'].map((n) => ({ value: n, label: n }))} />
                <Button size="sm" tone="secondary" icon="clock" onclick={() => session.startTimer(Number(timerMinutes) * 60 * 1000, Date.now())}>{t('app.whiteboards.timer_start')}</Button>
              {:else if remaining.running}
                <Button size="sm" tone="subtle" icon="pause" onclick={() => session.pauseTimer(Date.now())}>{t('app.whiteboards.timer_pause')}</Button>
              {:else}
                <Button size="sm" tone="subtle" icon="play" onclick={() => session.resumeTimer(Date.now())}>{t('app.whiteboards.timer_resume')}</Button>
              {/if}
              {#if remaining}<Button size="sm" tone="subtle" onclick={() => session.stopTimer()}>{t('app.whiteboards.timer_stop')}</Button>{/if}
            </Inline>
          {/if}
        </Stack>
      {/if}

      <Stack gap="050">
        <h3 class="heading">{t('app.whiteboards.here_now')}</h3>
        {#each peers as peer (peer.id)}
          <Inline gap="050" align="center" justify="between">
            <span class="peer-name" style:color={tokens.label[peerColour(peer.id)].fg}>{peer.name}</span>
            {#if peer.view}
              <Button size="sm" tone={following === peer.id ? 'primary' : 'subtle'} icon="eye" aria-pressed={following === peer.id} onclick={() => { following = following === peer.id ? undefined : peer.id; peersChanged(); }}>
                {t(following === peer.id ? 'app.whiteboards.unfollow' : 'app.whiteboards.follow')}
              </Button>
            {/if}
          </Inline>
        {:else}
          <p class="quiet">{t('app.whiteboards.alone')}</p>
        {/each}
      </Stack>

      <BoardComments
        boardId={board.id}
        {writable}
        {comments}
        pin={pinHere}
        pinLabel={chosen ? t('app.whiteboards.comment_at_object', { kind: t(`app.whiteboards.kind_${chosen.kind.toLowerCase()}`) }) : t('app.whiteboards.comment_at_view')}
        bind:open={openThread}
        bind:withResolved
        onshow={showThread}
      />

      <Stack gap="050">
        <h3 class="heading">{t('app.whiteboards.history')}</h3>
        {#if !history}
          <Button size="sm" tone="subtle" icon="clock" onclick={() => void openHistory()}>{t('app.whiteboards.history_show')}</Button>
        {:else}
          <ol class="choices" aria-label={t('app.whiteboards.history')}>
            {#each history as version (version.seq)}
              <li class="version">
                <span>{version.name ?? formatDateTime(version.created_at, messages.locale)} · {t('app.whiteboards.objects', { count: String(version.element_count) })}</span>
                {#if writable && isLive}
                  <Inline gap="050">
                    <Button size="sm" tone="subtle" icon="rotate-ccw" onclick={() => void bringBack(version.seq)}>{t('app.whiteboards.version_restore')}</Button>
                    <Button size="sm" tone="subtle" icon="tag" onclick={() => { namingSeq = version.seq; nameDraft = version.name ?? ''; }}>{t('app.whiteboards.version_name')}</Button>
                  </Inline>
                  {#if namingSeq === version.seq}
                    <Inline gap="050" align="end">
                      <Input size="sm" label={t('app.whiteboards.version_name')} bind:value={nameDraft} maxlength={120} />
                      <Button size="sm" tone="primary" onclick={() => void nameVersion(version.seq)}>{t('app.whiteboards.version_name_save')}</Button>
                    </Inline>
                  {/if}
                {/if}
              </li>
            {:else}
              <li class="quiet">{t('app.whiteboards.history_empty')}</li>
            {/each}
          </ol>
        {/if}
      </Stack>

      <Stack gap="050">
        <h3 class="heading">{t('app.whiteboards.backlinks')}</h3>
        {#if !backlinks}
          <Button size="sm" tone="subtle" icon="link" onclick={() => void openBacklinks()}>{t('app.whiteboards.backlinks_show')}</Button>
        {:else}
          <ul class="choices" aria-label={t('app.whiteboards.backlinks')}>
            {#each backlinks as link (link.source_kind + link.source_id)}
              <li>
                <a href={backlinkHref(link)}>{link.title}</a>
                <span class="quiet">{t(`app.whiteboards.backlink.${link.source_kind}`)} · {t(`app.whiteboards.strength.${link.strength}`)}</span>
              </li>
            {:else}
              <li class="quiet">{t('app.whiteboards.no_backlinks')}</li>
            {/each}
          </ul>
        {/if}
      </Stack>

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
          <Button size="sm" tone="subtle" onclick={() => void saveSvg(which)}>SVG</Button>
          <Button size="sm" tone="subtle" onclick={() => void savePng(which)}>PNG</Button>
          <Button size="sm" tone="subtle" onclick={() => void savePdf(which)}>PDF</Button>
          <Button size="sm" tone="subtle" onclick={() => void saveNative()}>{t('app.whiteboards.export_native')}</Button>
        </Inline>
        <p class="quiet">{t(which === 'frame' ? 'app.whiteboards.export_frame' : which === 'selection' ? 'app.whiteboards.export_selection' : 'app.whiteboards.export_board')}</p>
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

{#if isLinking}
  <Dialog bind:isOpen={isLinking} title={t('app.whiteboards.link_put')} dismissLabel={t('app.links.cancel')}>
    <Input label={t('app.whiteboards.link_address')} type="url" bind:value={linkDraft} error={linkError} placeholder="https://" />
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isLinking = false)}>{t('app.links.cancel')}</Button>
      <Button tone="primary" disabledReason={linkDraft.trim() && !linkError ? undefined : t('whiteboards.link_url_invalid')} onclick={putLink}>{t('app.whiteboards.link_put_confirm')}</Button>
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
  .clock { font-variant-numeric: tabular-nums; font-weight: var(--fw-semibold); color: var(--text-primary); }
  .clock.over { color: var(--text-danger); }
  .stage { position: relative; min-block-size: var(--sp-1000); overflow: hidden; border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface-sunken); }
  .canvas { display: block; inline-size: 100%; block-size: 100%; touch-action: none; cursor: grab; }
  .canvas.drawing { cursor: crosshair; }
  .canvas:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--bw-ring)); }
  .dot { fill: var(--border-default); }
  .grid { pointer-events: none; }
  .frame path { fill: none; stroke: var(--border-strong); stroke-dasharray: 8 6; }
  .frame-name { fill: var(--text-secondary); font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .connector path { fill: none; stroke: var(--text-secondary); stroke-width: 2; }
  .connector .label { fill: var(--text-secondary); font-size: var(--fs-075); }
  .head { fill: var(--text-secondary); }
  .stroke { fill: none; stroke: var(--text-primary); stroke-linecap: round; stroke-linejoin: round; }
  .stroke.highlighter { stroke-opacity: 0.4; }
  .stroke.chosen { stroke: var(--accent-primary); }
  .object path { stroke: none; }
  .object.shape path { fill: var(--bg-raised); stroke: var(--text-primary); stroke-width: 1.5; }
  .words { fill: var(--text-primary); font-size: var(--fs-100); }
  .card path { fill: var(--bg-raised); stroke: var(--border-default); }
  .card path.pending { stroke-dasharray: 4 4; }
  .upload { display: inline-flex; align-items: center; cursor: pointer; color: var(--text-primary); font-size: var(--fs-075); }
  .upload input { inline-size: 0; block-size: 0; opacity: 0; }
  .card.done .words { text-decoration: line-through; fill: var(--text-subtle); }
  .card .key { fill: var(--text-subtle); font-size: var(--fs-075); }
  .card .open { fill: var(--text-brand); font-size: var(--fs-100); }
  .tick rect { fill: var(--bg-surface); stroke: var(--border-strong); }
  .tick.done rect { stroke: var(--text-primary); }
  .tick text { fill: var(--text-primary); font-size: var(--fs-100); }
  .chosen path { stroke: var(--accent-primary); stroke-width: 3; }
  .drafting { fill: none; stroke: var(--accent-primary); stroke-dasharray: 4 4; stroke-width: 2; }
  .guide { stroke: var(--accent-primary); stroke-width: 1; stroke-dasharray: 4 3; pointer-events: none; }
  .handle { fill: var(--accent-primary); cursor: nwse-resize; }
  .votes circle { fill: var(--accent-primary); }
  .votes text { fill: var(--text-inverse); font-weight: var(--fw-semibold); }
  .pin { cursor: pointer; }
  .pin path { fill: var(--accent-primary); stroke: var(--bg-surface); stroke-width: 1.5; }
  .pin text { fill: var(--text-inverse); font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .pin.open path { stroke: var(--text-primary); stroke-width: 2.5; }
  .pin.resolved path { fill: var(--border-strong); }
  .peer { pointer-events: none; }
  .peer path { fill: currentColor; stroke: var(--bg-surface); stroke-width: 1; }
  .peer text { fill: currentColor; font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .peer-name { font-weight: var(--fw-semibold); }
  .minimap {
    position: absolute;
    inset-block-end: var(--sp-100);
    inset-inline-end: var(--sp-100);
    inline-size: var(--layout-gallery-card);
    block-size: var(--sp-1600);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-sm);
    background: var(--bg-surface);
    cursor: pointer;
  }
  .mini { fill: var(--border-strong); }
  .mini.frame { fill: none; stroke: var(--border-default); }
  .mini-view { fill: none; stroke: var(--accent-primary); stroke-width: 2; }
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
  .version { display: grid; gap: var(--sp-025); }
  .choice { all: unset; cursor: pointer; padding: var(--sp-050); border-radius: var(--r-sm); color: var(--text-primary); }
  .choice:hover, .choice:focus-visible { background: var(--bg-surface-hover); }
</style>
