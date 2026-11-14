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

  import { tick as settled, untrack, type Snippet } from 'svelte';
  import { tokens } from '@hubtask/design-system';
  import {
    Avatar, AvatarGroup, Button, Checkbox, Dialog, FloatingBar, Icon, IconButton, Inline, Input, Kbd, Menu, Popover, ProductTile, Select, Stack, VisuallyHidden, type MenuItem,
  } from '@hubtask/design-system/components';
  import { LiveDocument, type LiveSocket, type LiveStatus } from '@hubtask/editor';
  import type {
    ReferenceTarget, WhiteboardBacklinkList, WhiteboardComment, WhiteboardCommentList, WhiteboardRead, WhiteboardVersion, WorkItem,
  } from '@hubtask/sync-engine';
  import {
    Board, DEFAULT_SIZE, Doc, Facilitation, PEN_WIDTH, ROUTES, SHAPES, align, bounds, box, connectorPath, copy, culled,
    connectorPoints, elementId, fit, fromMinimap, hit, labelPoint, lines, linkHost, minimap, outlinePath, paste, pdfOfJpeg, shapeWords, readCopied, snap, strokePath,
    strokesOf, template, toBoard, toSvg, touches, withGroups, withinFrame, zoomAt,
    type Element, type ElementKind, type Guide, type Pen, type Point, type Shape, type TemplateName, type Viewport,
  } from '@hubtask/whiteboard';
  import type { IconName } from '@hubtask/design-system/icons';

  import { announcer } from '../announce.svelte.ts';
  import EntryPicker from '../entries/EntryPicker.svelte';
  import { keyboardTool, tabTarget } from './keys.ts';
  import { isLevelWithIslands } from './islands.ts';
  import { DOCK, PHONE_DOCK, afterUse, neighbourOf, toolOfKey, type DockTool } from './dock.ts';
  import { keepThenReplace } from './restore.ts';
  import { sceneKey } from './scene.ts';
  import { followedViewport, peersOf, stillFollowing, type Peer } from './peers.ts';
  import BoardComments from './BoardComments.svelte';
  import BoardList from './BoardList.svelte';
  import BoardTemplates, { templateOf, type BoardStart } from './BoardTemplates.svelte';
  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { initialsOf } from '../data/grants.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { entryTypes, entryTypesOffered, offeredTypes, typesShown } from '../data/entrytypes.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { media } from '../data/media.svelte.ts';
  import { firstPageOf, type FirstPage } from '../files/firstPage.ts';
  import { viewer } from '../files/viewer.svelte.ts';
  import { markOf, viewOf as fileViewOf } from '../files/viewing.ts';
  import { formatFileSize } from '../i18n/bytes.ts';
  import { resolveRequest } from '../data/links.svelte.ts';
  import {
    BOARD_COLOURS, backlinkHref, linkAddress, boardCommentsPath, boardFileName, cardEntries, clockOf, entryOfNote, notesInFrame, peerColour, pictureColours, pinOf,
    placeCarried, threadsOf, voterKey, whiteboards,
  } from '../data/whiteboards.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { isLiveUnreachable } from '../data/liveunreachable.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { platform } from '../platform/index.ts';
  import { renderProblem } from '../problem.ts';
  import { unsentDocuments } from '../data/unsentdocs.ts';
  import { liveKeeper } from '../data/unsentkeeper.ts';

  interface Props {
    board: WhiteboardRead;
    /** The hub's collections, offered to hold a sticky note made an entry (UC-CNV-02). */
    collections: readonly { id: string; name: string; settings?: { entry_types?: Record<string, readonly string[]> } }[];
    onnavigate: (path: string) => void;
    /** The board's name and its star, in the top-left island beside the way back. */
    heading: Snippet;
    /** Where the way back in the top-left island leads: the hub's boards. */
    back?: string;
    /** The screen's own verbs on the board (rename, the trash), at the end of the board's menu. */
    boardMenu?: { items: readonly MenuItem[]; onselect: (id: string) => void };
    /** A board just made: an empty one opens with the templates as a sheet (scene wnew). */
    isNew?: boolean;
    /** What the new board's sheet was answered with, a template or blank. */
    onbegin?: (choice: BoardStart) => void;
    /** A thread to open at once, from the home's comments to the reader (scene whome 4). */
    thread?: string;
  }

  const { board, collections, onnavigate, heading, back, boardMenu, isNew = false, onbegin, thread }: Props = $props();

  type Tool = DockTool | 'LINK';

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
  // This device's own changes only: another participant's are never undone here.
  const undoable = model.history();
  let canUndo = $state(false);
  let canRedo = $state(false);
  const session = new Facilitation(doc);
  let elements = $state<Element[]>(untrack(() => saved));
  // Who put each note on the board, by account, as the live document knows it (`Board.makerOf`).
  let makers = $state<Map<string, string>>(new Map());
  let status = $state<LiveStatus | 'reading'>('connecting');
  let failedAttempts = $state(0);
  // Local changes the server has not had yet, kept on the device meanwhile (unsentdocs.ts).
  let unsent = $state(0);
  let isLive = $state(false);
  let tool = $state<Tool>('select');
  let shape = $state<Shape>('RECTANGLE');
  // A double click on a tool holds it; otherwise it springs back to select after one use (W-3).
  let held = $state(false);
  // The held space bar is the hand for as long as it is down.
  let spaceHeld = $state(false);
  const pen = $derived<Pen>(tool === 'HIGHLIGHTER' ? 'HIGHLIGHTER' : 'PEN');
  /** The surface or the same board as a list, for the keyboard and screen readers (§3). */
  let mode = $state<'surface' | 'list'>('surface');
  /** Where the comment tool was pressed: the next thread stands there. */
  let commentAt = $state<Point | undefined>(undefined);
  let selected = $state<string[]>([]);
  let editing = $state<string | undefined>(undefined);
  let draft = $state('');
  let view = $state<Viewport>({ x: -80, y: -80, zoom: 1, width: 1, height: 1 });
  let surface = $state<SVGSVGElement | undefined>(undefined);
  let toolbar = $state<HTMLDivElement | undefined>(undefined);
  let zoombar = $state<HTMLElement | null>(null);
  /** A tablet: the frame's middle widths, where the dock and the corner share the foot only one above the other. */
  const isTablet = $derived(!viewport.isCompact && viewport.isBelowExpanded);
  /** On a touch screen the pen, finger and two-finger tap are said once above the dock, until first used. */
  let hasTouched = $state(false);
  /** The session bar stands level with the top islands at the centre, below them where it would touch one (W-5). */
  let titleIsland = $state<HTMLElement | null>(null);
  let peopleIsland = $state<HTMLElement | null>(null);
  let titleWidth = $state(0);
  let peopleWidth = $state(0);
  let sessionWidth = $state(0);
  const isSessionLevel = $derived(
    !titleIsland || !peopleIsland || isLevelWithIslands(view.width, sessionWidth, [titleWidth, peopleWidth], Math.min(titleIsland.offsetLeft, peopleIsland.offsetLeft)),
  );
  let cards = $state<Map<string, ReferenceTarget>>(new Map());
  let isPickingEntry = $state(false);
  let picked = $state<WorkItem | undefined>(undefined);
  let isMakingEntry = $state(false);
  let entryKind = $state<string>('');
  let isLinking = $state(false);
  let linkDraft = $state('');
  let linkAt = $state<Point | undefined>(undefined);
  let isGridOn = $state(false);
  // The small map comes on request (W-1).
  let isMapShown = $state(false);
  let guides = $state<Guide[]>([]);
  let isTemplating = $state(false);
  let templateStart = $state<BoardStart>('RETRO_START_STOP_CONTINUE');
  /** The new board's sheet (scene wnew): open on a board just made and still empty. */
  let isBeginning = $state(untrack(() => isNew && writable && saved.length === 0));
  function begin(choice: BoardStart) {
    isBeginning = false;
    const name = templateOf(choice);
    if (name) putTemplate(name);
    onbegin?.(choice);
  }

  const byId = $derived(new Map(elements.map((e) => [e.id, e])));
  const shown = $derived(culled(elements, view, 200));
  const frames = $derived(elements.filter((e) => e.kind === 'FRAME'));
  const chosen = $derived(selected.length === 1 ? byId.get(selected[0] ?? '') : undefined);
  const viewBox = $derived(`${view.x} ${view.y} ${view.width / view.zoom} ${view.height / view.zoom}`);

  // A board opens showing all it holds, never larger than its own size, once the stage has been
  // measured - unless its address names a frame, which the live document's sync then opens on.
  let isFitted = false;
  $effect(() => {
    if (isFitted || view.width <= 1 || view.height <= 1) return;
    untrack(() => {
      isFitted = true;
      if (location.hash.length > 1) return;
      const first = viewport.isCompact ? elements.find((e) => e.kind === 'FRAME') : undefined;
      if (first) { view = fit(view, box(first)); return; }
      const all = bounds(elements);
      if (!all) return;
      const fitted = fit(view, all);
      view = fitted.zoom <= 1 ? fitted : { ...view, zoom: 1, x: (all.left + all.right) / 2 - view.width / 2, y: (all.top + all.bottom) / 2 - view.height / 2 };
    });
  });

  // --- The live document ---------------------------------------------------------------------------

  let timer: ReturnType<typeof setTimeout> | undefined;
  function changed() {
    // A reader whose board has no live document yet sees the scene as last saved.
    elements = !writable && model.size === 0 ? saved : model.elements();
    makers = new Map(elements.flatMap((e) => {
      const by = e.kind === 'STICKY' ? model.makerOf(e.id) : undefined;
      return by ? [[e.id, by] as const] : [];
    }));
    if (!writable || !isLive) return;
    clearTimeout(timer);
    timer = setTimeout(() => void save(), SAVE_AFTER);
  }

  // The scene the server holds: a save of the same again is not sent, so opening and leaving a
  // board write nothing (`sceneKey`).
  let heldScene = untrack(() => sceneKey(saved));
  async function save() {
    const scene = model.scene();
    const key = sceneKey(scene);
    if (key === heldScene) return;
    try {
      await whiteboards.saveScene(board, scene as never);
      heldScene = key;
    } catch (error) {
      toast.show({ text: t('app.whiteboards.save_failed', { reason: renderProblem(error as never, messages).message }), tone: 'danger' });
    }
  }

  let connection: LiveDocument | undefined;
  $effect(() => {
    return untrack(() => {
      const stop = model.observe(changed);
      const stopHistory = undoable.observe(() => { canUndo = undoable.canUndo; canRedo = undoable.canRedo; });
      const stopSession = session.observe(sessionChanged);
      connection = new LiveDocument(doc, {
        open: () => whiteboards.liveSocket(board.id) as unknown as LiveSocket,
        writable,
        onStatus: (next) => {
          status = next;
          failedAttempts = connection?.failedAttempts ?? 0;
        },
        keep: writable ? liveKeeper('whiteboard', board.id) : undefined,
        onUnsent: (count) => {
          unsent = count;
          unsentDocuments.set(`whiteboard:${board.id}`, count);
        },
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
        stopHistory();
        undoable.destroy();
        stopSession();
        live.awareness.off('change', peersChanged);
        live.destroy();
        unsentDocuments.forget(`whiteboard:${board.id}`);
        connection = undefined;
        doc.destroy();
      };
    });
  });

  // --- Who else is here (research §4.1: cursors, presence, follow) ---------------------------------

  let peers = $state<Peer[]>([]);
  let following = $state<number | undefined>(undefined);
  let cursorSent = 0;

  const viewOf = () => ({ x: view.x + view.width / view.zoom / 2, y: view.y + view.height / view.zoom / 2, zoom: view.zoom });

  function peersChanged() {
    const live = connection;
    if (!live) return;
    const next = peersOf(live.awareness.getStates(), doc.clientID, t('app.whiteboards.someone'));
    peers = next;
    following = stillFollowing(next, following);
    const centred = followedViewport(next, following, view);
    if (centred) view = { ...view, ...centred };
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
    accounts.resolve(state.data.data.map((target) => target.assignee_id));
  }

  const makerIds = $derived([...new Set(makers.values())].sort().join(','));
  $effect(() => {
    const ids = makerIds;
    if (ids) untrack(() => accounts.resolve(ids.split(',')));
  });
  /** The name at a note's foot: the first word of its maker's, as the board says people. */
  const makerName = (e: Element) => (accounts.nameOf(makers.get(e.id)) ?? '').trim().split(/\s+/)[0] ?? '';

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
    if (!model.add(element, actor.account?.id)) {
      toast.show({ text: t('whiteboards.too_many_elements', { maximum: '5000' }), tone: 'danger' });
      return undefined;
    }
    choose([element.id]);
    return element.id;
  }

  function putAll(next: Element[], said: string): void {
    if (next.length === 0) return;
    if (!model.addAll(next, actor.account?.id)) {
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

  let editor = $state<HTMLTextAreaElement | undefined>(undefined);
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
    | { kind: 'connect'; from: string; endX: number; endY: number; byHandle?: boolean }
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

  // Touches down at once: two that lift together without moving are a two-finger tap (undo).
  const touches2 = new Map<number, { x: number; y: number; at: number }>();
  let twoFinger = false;

  /** The tool after one use (W-3), and the dock's word for it. */
  function used() {
    if (tool === 'LINK') { tool = 'select'; return; }
    tool = afterUse(tool, held);
    if (tool === 'select') held = false;
  }

  function down(event: PointerEvent) {
    if (event.pointerType !== 'mouse') hasTouched = true;
    if (editing) commitEdit();
    if (following !== undefined) following = undefined;
    if (event.pointerType === 'touch') {
      touches2.set(event.pointerId, { x: event.clientX, y: event.clientY, at: performance.now() });
      if (touches2.size === 2) { twoFinger = true; gesture = undefined; return; }
    }
    const at = pointOf(event);
    const target = hit(elements, at);
    // A pointer that has already ended cannot be captured; the gesture goes on without it.
    try { surface?.setPointerCapture(event.pointerId); } catch { /* ended already */ }
    // On a tablet the pen draws whatever tool is chosen; the finger selects and pans (§7).
    const penDraws = event.pointerType === 'pen' && (tool === 'select' || tool === 'HAND');
    if (tool === 'HAND' && !penDraws || spaceHeld) {
      gesture = { kind: 'pan', startX: event.clientX, startY: event.clientY, viewX: view.x, viewY: view.y };
      return;
    }
    if (writable && tool === 'CONNECTOR' && target && target.kind !== 'CONNECTOR') {
      gesture = { kind: 'connect', from: target.id, endX: at.x, endY: at.y };
      return;
    }
    if (writable && (tool === 'STROKE' || tool === 'HIGHLIGHTER' || penDraws)) {
      gesture = { kind: 'draw', points: [at] };
      return;
    }
    if (writable && tool === 'ERASER') {
      gesture = { kind: 'erase' };
      erase(at);
      return;
    }
    if (tool === 'COMMENT') {
      commentAt = target ? undefined : at;
      if (target) choose([target.id]);
      else selected = [];
      openThread = undefined;
      void openSheet('comments');
      used();
      return;
    }
    if (writable && tool === 'LINK') {
      linkAt = at;
      linkDraft = '';
      isLinking = true;
      used();
      return;
    }
    if (writable && (tool === 'STICKY' || tool === 'SHAPE' || tool === 'TEXT' || tool === 'FRAME')) {
      // The press must not take the focus to the canvas: the words go into the note just placed,
      // and a held tool's next note would otherwise close as the last one's editor loses focus.
      event.preventDefault();
      const id = put(tool, at);
      used();
      if (id && (model.get(id)?.kind ?? '') !== 'FRAME') {
        edit(id);
        void settled().then(() => editor?.focus());
      }
      return;
    }
    if (target && target.kind !== 'FRAME') {
      if (event.shiftKey) choose(selected.includes(target.id) ? selected.filter((id) => id !== target.id) : [...selected, target.id]);
      else if (!selected.includes(target.id)) choose([target.id]);
      // A connector follows the objects it joins: chosen, it is changed from the bar, not dragged.
      if (writable && target.kind !== 'CONNECTOR') gesture = { kind: 'move', startX: at.x, startY: at.y, anchor: target.id, originX: target.x, originY: target.y };
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
    const touch = touches2.get(event.pointerId);
    touches2.delete(event.pointerId);
    if (twoFinger) {
      const still = touch && Math.hypot(event.clientX - touch.x, event.clientY - touch.y) < 12 && performance.now() - touch.at < 400;
      if (touches2.size === 0) {
        twoFinger = false;
        if (still && writable) undo();
      }
      return;
    }
    const g = gesture;
    gesture = undefined;
    guides = [];
    try { surface?.releasePointerCapture(event.pointerId); } catch { /* never captured */ }
    if (g?.kind === 'draw') {
      const strokes = strokesOf(g.points, pen, () => elementId(), pen === 'HIGHLIGHTER' ? 'amber' : undefined);
      if (!model.addAll(strokes, actor.account?.id)) toast.show({ text: t('whiteboards.too_many_elements', { maximum: '5000' }), tone: 'danger' });
      return;
    }
    if (g?.kind === 'connect' && g.byHandle) {
      const target = hit(elements, pointOf(event));
      if (target && target.kind !== 'CONNECTOR' && target.id !== g.from) connectTo(g.from, target.id);
      return;
    }
    if (g?.kind !== 'connect') return;
    const target = hit(elements, pointOf(event));
    if (target && target.kind !== 'CONNECTOR' && target.id !== g.from) {
      model.add({ id: elementId(), kind: 'CONNECTOR', from: g.from, to: target.id, route: 'STRAIGHT', start: 'NONE', end: 'ARROW', x: 0, y: 0, width: 0, height: 0 });
    }
    used();
  }

  /** Dragging one of the chosen object's edge handles draws an arrow to where it is let go (W-4). */
  function handleStart(event: PointerEvent, e: Element) {
    event.stopPropagation();
    surface?.setPointerCapture(event.pointerId);
    const at = pointOf(event);
    gesture = { kind: 'connect', from: e.id, endX: at.x, endY: at.y, byHandle: true };
  }

  /** "+" beside a note, or Tab while writing in it: the next note, the same colour, and its words. */
  function addNeighbour(of: Element) {
    const id = put(of.kind === 'SHAPE' ? 'SHAPE' : 'STICKY', neighbourOf(of), { color_token: of.color_token, width: of.width, height: of.height });
    if (id) edit(id);
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
    if (event.key === 'Tab') {
      const nextId = tabTarget(order.map((e) => e.id), selected[0], event.shiftKey);
      // Past the last object (or before the first) Tab is the page's again: focus leaves the board.
      if (nextId === undefined) return;
      event.preventDefault();
      const next = byId.get(nextId);
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
    if (event.key === 'Escape') {
      // Esc always leads back (W-3): a tool to select, a choice to none, then out to the dock.
      if (selected.length === 0 && tool === 'select') (toolbar ?? zoombar)?.querySelector<HTMLElement>('button, select, input')?.focus();
      selected = []; tool = 'select'; held = false; following = undefined; commentAt = undefined; return;
    }
    if (event.key === ' ' && !(writable && chosen?.kind === 'CARD')) {
      event.preventDefault();
      spaceHeld = true;
      return;
    }
    const mod = event.ctrlKey || event.metaKey;
    if (mod && (event.key === 'a' || event.key === 'A') && order.length > 0) {
      event.preventDefault();
      choose(order.map((e) => e.id));
      announcer.say(t('app.whiteboards.chose_all', { count: String(selected.length) }));
      return;
    }
    if (event.key === 'Enter' && chosen?.kind === 'CARD') { event.preventDefault(); openCard(chosen); return; }
    if (event.key === 'Enter' && chosen?.kind === 'FILE') { event.preventDefault(); void openFile(chosen.media_id); return; }
    if (!writable) return;
    if (mod && (event.key === 'z' || event.key === 'Z' || event.key === 'y' || event.key === 'Y')) {
      event.preventDefault();
      if (event.key === 'y' || event.key === 'Y' || event.shiftKey) redo();
      else undo();
      return;
    }
    if (event.key === ' ' && chosen?.kind === 'CARD') { event.preventDefault(); void tick(chosen); return; }
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
    if (event.key === 'n' || event.key === 'N') { event.preventDefault(); const id = put('STICKY', centre()); if (id) edit(id); return; }
    const next = toolOfKey(event.key, event.altKey || event.shiftKey);
    if (next) { event.preventDefault(); pickTool(next, false); }
  }

  function keyUp(event: KeyboardEvent) {
    if (event.key === ' ') spaceHeld = false;
  }

  /** Opens a card's entry, which is what its ↗ does for a pointer. */
  function openCard(e: Element) {
    const target = cardOf(e);
    if (target?.available) onnavigate(`/items/${target.id}`);
  }

  /**
   * Chooses a tool. Pressed with a pointer it waits for the board to be pressed; pressed from the
   * keyboard (a click with no pointer behind it) it acts at once, at the centre of the view.
   */
  function pickTool(next: Tool, byKeyboard: boolean, hold = false) {
    held = hold;
    tool = next;
    if (next === 'select') held = false;
    if (!byKeyboard) return;
    if (next === 'COMMENT') {
      tool = 'select';
      commentAt = undefined;
      void openSheet('comments');
      return;
    }
    const action = keyboardTool(next);
    if (action === 'place') {
      const id = put(next as ElementKind, centre());
      tool = 'select';
      if (id && (model.get(id)?.kind ?? '') !== 'FRAME') edit(id);
      else surface?.focus();
    } else if (action === 'link') {
      linkAt = centre();
      linkDraft = '';
      isLinking = true;
      tool = 'select';
    } else if (action === 'connect') {
      tool = 'select';
      announcer.say(t('app.whiteboards.connect_hint'));
    }
  }

  /** Draws a connector between the chosen object and another, as dragging from one to the other does. */
  function connectTo(from: string, to: string) {
    if (!to || to === from) return;
    model.add({ id: elementId(), kind: 'CONNECTOR', from, to, route: 'STRAIGHT', start: 'NONE', end: 'ARROW', x: 0, y: 0, width: 0, height: 0 });
    announcer.say(t('app.whiteboards.connected'));
  }

  /** The objects a picker in the side panel offers: every one but connectors, strokes and those left out. */
  function others(leaveOut: readonly string[]) {
    return elements
      .filter((e) => e.kind !== 'CONNECTOR' && e.kind !== 'STROKE' && !leaveOut.includes(e.id))
      .map((e) => ({ value: e.id, label: `${t(`app.whiteboards.kind_${e.kind.toLowerCase()}`)}: ${wordsOf(e)}` }));
  }

  function remove() {
    undoable.stopCapturing();
    model.remove(selected);
    undoable.stopCapturing();
    selected = [];
    // Everyone drawing sees it gone at once, so the way back is offered where it happened.
    toast.show({ text: t('app.whiteboards.removed'), tone: 'info', undo: () => undo() });
  }

  /** Takes back this device's last change, for everyone drawing. */
  function undo() {
    selected = [];
    announcer.say(t(undoable.undo() ? 'app.whiteboards.undone' : 'app.whiteboards.nothing_to_undo'));
  }

  function redo() {
    selected = [];
    announcer.say(t(undoable.redo() ? 'app.whiteboards.redone' : 'app.whiteboards.nothing_to_redo'));
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
    void carry(next);
  }

  /** Pictures and files pasted from another board are placed on this one, or its readers read none. */
  async function carry(next: readonly Element[]) {
    const failed = await placeCarried(next, (id) => whiteboards.placeMedia(board.id, id));
    if (failed.length > 0) toast.show({ text: t('app.whiteboards.carried_unplaced', { count: String(failed.length) }), tone: 'warning' });
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

  const linkError = $derived(!linkDraft.trim() || linkAddress(linkDraft) ? undefined : t('whiteboards.link_url_invalid'));

  function putLink() {
    const url = linkAddress(linkDraft);
    isLinking = false;
    if (!url) return;
    put('LINK', linkAt ?? centre(), { url });
  }

  // --- Pictures and files (UC-CNV-08) -------------------------------------------------------------

  let isUploading = $state(false);

  /** An upload made one of the board's files, then drawn: a picture as itself, anything else as its card. */
  async function putFile(file: File) {
    const at = centre();
    isUploading = true;
    try {
      const object = await media.upload(file, 'ATTACHMENT', { idempotencyKey: crypto.randomUUID() });
      await whiteboards.placeMedia(board.id, object.id);
      put(PICTURES.has(object.content_type ?? '') ? 'IMAGE' : 'FILE', at, { media_id: object.id });
      showFileHint();
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isUploading = false;
    }
  }

  /** What lies on the board as a picture (F-4); every other file is a card. */
  const PICTURES = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/svg+xml']);

  /** Opens the file viewer on one of the board's files, with every other file of the board beside it (F-1). */
  function openFile(mediaId: string | undefined) {
    if (!mediaId) return;
    const files = elements
      .filter((e) => (e.kind === 'FILE' || e.kind === 'IMAGE') && e.media_id)
      .map((e) => media.recordOf(e.media_id))
      .filter((record): record is NonNullable<typeof record> => record !== undefined);
    const index = files.findIndex((each) => each.id === mediaId);
    if (index < 0) {
      toast.show({ text: t('app.whiteboards.file_unavailable'), tone: 'danger' });
      return;
    }
    viewer.open({ files, index, owner: { title: board.title } });
  }

  $effect(() => {
    media.resolve(elements.filter((e) => e.kind === 'FILE' || e.kind === 'IMAGE').map((e) => e.media_id));
  });

  // A PDF card shows its first page and its page count, drawn once per file.
  let firstPages = $state<Record<string, FirstPage>>({});
  $effect(() => {
    for (const e of elements) {
      const record = e.kind === 'FILE' ? media.recordOf(e.media_id) : undefined;
      if (!record || record.id in firstPages || fileViewOf(record) !== 'pdf') continue;
      void firstPageOf(record.id, record.size).then((page) => { if (page) firstPages = { ...firstPages, [record.id]: page }; });
    }
  });

  /** The card's second line: pages where known, and the size. */
  function fileLineOf(mediaId: string | undefined): string {
    const record = media.recordOf(mediaId);
    if (!record) return '';
    const size = formatFileSize(record.size, messages.locale, t);
    const pages = firstPages[record.id]?.pages;
    return pages ? `${t('app.whiteboards.file_pages', { count: String(pages) })} · ${size}` : size;
  }

  // The one-time hint (scene `wbfiles`): what happens to a dropped file, once per device.
  const FILE_HINT = 'hubtask.board.file-hint';
  let isFileHintShown = $state(false);
  function showFileHint() {
    let seen = false;
    try { seen = localStorage.getItem(FILE_HINT) === 'seen'; localStorage.setItem(FILE_HINT, 'seen'); } catch { /* shown this visit only */ }
    if (!seen) isFileHintShown = true;
  }

  // --- Templates (research §4.3) -------------------------------------------------------------------

  function putTemplate(name: TemplateName) {
    isTemplating = false;
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
    const collection = collections.find((each) => each.id === collectionId);
    return offeredTypes(entryTypes.all, 'TASK', entryTypesOffered(collection?.settings, 'TASK'), board.hub_id);
  };
  let entryCollection = $state<string>('');
  const kinds = $derived(entryCollection ? kindsIn(entryCollection) : []);
  const notesToTurn = $derived(
    chosen?.kind === 'FRAME' ? notesInFrame(elements, chosen)
    : selected.map((id) => byId.get(id)).filter((e): e is Element => !!e && (e.kind === 'STICKY' || e.kind === 'SHAPE') && !!(e.text ?? '').trim()),
  );

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

  /** The version a person asked to bring back, while they confirm it replaces the board for everyone. */
  let restoring = $state<WhiteboardVersion | undefined>(undefined);
  let isRestoreAsked = $state(false);

  /**
   * An older version brought back: its objects in place of the scene, for everyone drawing. What
   * stood is saved and named first, so a later save of the same drawing session cannot fold over
   * it in the history; and the change is one step to undo.
   */
  async function bringBack(seq: number) {
    isRestoreAsked = false;
    restoring = undefined;
    try {
      const version = await whiteboards.version(board.id, seq);
      const when = formatDateTime(version.created_at, messages.locale);
      const back = await keepThenReplace({
        // Not save(), which reports and goes on: a state that could not be kept is not replaced.
        save: async () => {
          const scene = model.scene();
          await whiteboards.saveScene(board, scene as never);
          heldScene = sceneKey(scene);
        },
        versions: () => whiteboards.versions(board.id),
        name: (kept, name) => whiteboards.nameVersion(board.id, kept, name),
        replace: () => {
          undoable.stopCapturing();
          const replaced = model.replace((version.elements ?? []) as Element[]);
          undoable.stopCapturing();
          return replaced;
        },
      }, seq, t('app.whiteboards.before_restore', { when }));
      if (!back) return;
      selected = [];
      toast.show({ text: t('app.whiteboards.version_back', { when }), tone: 'success', undo: () => undo() });
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
  let openThread = $state<string | undefined>(untrack(() => thread));
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
  /** The objects a comment is pinned to: their vote count steps aside from the pin at the same corner. */
  const pinned = $derived(new Set(pins.map((pin) => pin.thread.element_id).filter((id): id is string => !!id)));
  const voteX = (e: { id: string; x: number; width: number }) => e.x + e.width - (pinned.has(e.id) ? 36 / view.zoom : 0);

  /** A new thread stands at the chosen object's corner, or in the middle of the view. */
  function pinHere(): { element_id?: string; x: number; y: number } {
    if (chosen) {
      const at = pinOf({ id: '', parent_id: null, element_id: chosen.id, x: 0, y: 0, resolved: false, deleted: false }, byId);
      return { element_id: chosen.id, x: Math.round(at.x), y: Math.round(at.y) };
    }
    const at = commentAt ?? centre();
    return { x: Math.round(at.x), y: Math.round(at.y) };
  }

  function showThread(comment: WhiteboardComment) {
    const at = pinOf(comment, byId);
    following = undefined;
    view = { ...view, x: at.x - view.width / view.zoom / 2, y: at.y - view.height / view.zoom / 2 };
    if (comment.element_id && byId.has(comment.element_id)) choose([comment.element_id]);
  }

  // --- The minimap ---------------------------------------------------------------------------------

  const map = $derived(isMapShown && elements.length > 0 ? minimap(elements, view, MAP) : undefined);
  function mapClick(event: MouseEvent) {
    const target = event.currentTarget as SVGSVGElement;
    const rect = target.getBoundingClientRect();
    if (!map || rect.width === 0) return;
    following = undefined;
    view = fromMinimap(map, view, { x: ((event.clientX - rect.x) / rect.width) * MAP.width, y: ((event.clientY - rect.y) / rect.height) * MAP.height });
  }

  const statusText = $derived(
    status === 'refused' ? t('app.whiteboards.status_refused')
    : unsent > 0 ? t('app.whiteboards.status_unsent', { count: String(unsent) })
    : isLiveUnreachable(status, failedAttempts, live.state) ? t('app.whiteboards.status_unreachable')
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

  // --- The side sheet and the menus (user-experience.md §7.2) --------------------------------------

  /** One sheet open at a time beside the canvas, closed until its control in the head opens it. */
  type Sheet = 'session' | 'comments' | 'history' | 'links' | 'frames' | 'export';
  let sheet = $state<Sheet | undefined>(untrack(() => (thread ? 'comments' : undefined)));
  let sheetTitle = $state<HTMLHeadingElement | undefined>(undefined);
  /** On a phone the selection bar waits to be asked for; elsewhere it stands above the selection. */
  let barAsked = $state(false);
  $effect(() => { if (selected.length === 0) untrack(() => { barAsked = false; isMakingEntry = false; }); });
  const openThreads = $derived(threadsOf(comments, false).length);

  async function toggleSheet(next: Sheet) {
    sheet = sheet === next ? undefined : next;
    if (!sheet) return;
    if (sheet === 'history' && !history) void openHistory();
    if (sheet === 'links' && !backlinks) void openBacklinks();
    await settled();
    sheetTitle?.focus();
  }

  async function openSheet(next: Sheet) {
    if (sheet !== next) await toggleSheet(next);
  }

  function closeSheet() {
    const was = sheet;
    sheet = undefined;
    const opener = document.querySelector<HTMLElement>(`[data-board-sheet="${was}"]`);
    if (opener) opener.focus();
    else surface?.focus();
  }

  const menuItems = $derived<MenuItem[]>([
    ...(writable ? [{ id: 'template', label: t('app.whiteboards.template_put_menu'), icon: 'layout-template' as const }] : []),
    { id: 'frames', label: t('app.whiteboards.frames'), icon: 'frame' },
    { id: 'links', label: t('app.whiteboards.backlinks'), icon: 'link' },
    // No export on a phone (§7 Whiteboard).
    ...(viewport.isCompact ? [] : [{ id: 'export', label: t('app.whiteboards.export'), icon: 'external-link' as const }]),
    { id: 'grid', label: t(isGridOn ? 'app.whiteboards.grid_off' : 'app.whiteboards.grid_on'), icon: 'columns-3', hasSeparatorBefore: true },
    ...(boardMenu?.items ?? []).map((item, i) => (i === 0 ? { ...item, hasSeparatorBefore: true } : item)),
  ]);

  function menuChose(id: string) {
    if (id === 'template') isTemplating = true;
    else if (id === 'grid') isGridOn = !isGridOn;
    else if (id === 'frames' || id === 'links' || id === 'export') void toggleSheet(id as Sheet);
    else boardMenu?.onselect(id);
  }

  const ICONS: Record<Tool, IconName> = {
    select: 'mouse-pointer-2', HAND: 'hand', STICKY: 'sticky-note', SHAPE: 'shapes', TEXT: 'type', CONNECTOR: 'move-up-right',
    FRAME: 'frame', STROKE: 'pencil', HIGHLIGHTER: 'highlighter', ERASER: 'eraser', COMMENT: 'message-circle', LINK: 'link',
  };
  /** The dock as drawn here: a reader has the hand and the comment; a phone the four of §7, the rest under ⋯. */
  const dockGroups = $derived(
    DOCK.map((group) => ({
      group: group.group,
      tools: group.tools.filter((each) => (writable || each.tool === 'select' || each.tool === 'HAND' || each.tool === 'COMMENT') && (!viewport.isCompact || PHONE_DOCK.includes(each.tool))),
    })).filter((group) => group.tools.length > 0),
  );
  const foldedTools = $derived(
    viewport.isCompact && writable ? DOCK.flatMap((group) => group.tools).filter((each) => !PHONE_DOCK.includes(each.tool)) : [],
  );
  /** A picture or file from the insert menu: the platform's chooser alone, no control drawn (user-experience.md §5.10). */
  function chooseFile(): void {
    const input = document.createElement('input');
    input.type = 'file';
    input.onchange = () => {
      const file = input.files?.[0];
      if (file) void putFile(file);
    };
    input.click();
  }

  const insertItems = $derived<MenuItem[]>([
    ...foldedTools.map((each) => ({ id: `tool:${each.tool}`, label: t(`app.whiteboards.tool_${each.tool.toLowerCase()}`), icon: ICONS[each.tool] })),
    ...(viewport.isCompact
      ? [{ id: 'redo', label: t('app.whiteboards.redo'), icon: 'redo-2' as const, disabledReason: canRedo ? undefined : t('app.whiteboards.nothing_to_redo') }]
      : []),
    { id: 'tool:LINK', hasSeparatorBefore: foldedTools.length > 0, label: t('app.whiteboards.tool_link'), icon: 'link' },
    { id: 'file', label: isUploading ? t('app.whiteboards.uploading') : t('app.whiteboards.put_file'), icon: 'paperclip', disabledReason: isUploading ? t('app.whiteboards.uploading') : undefined },
    { id: 'entry', label: t('app.whiteboards.put_entry'), icon: 'square-check' },
    // Templates wait for a larger screen (§7 Whiteboard).
    ...(viewport.isCompact ? [] : [{ id: 'template', label: t('app.whiteboards.template_put_menu'), icon: 'layout-template' as const, hasSeparatorBefore: true }]),
  ]);

  function insertChose(id: string) {
    if (id.startsWith('tool:')) pickTool(id.slice(5) as Tool, false);
    else if (id === 'redo') redo();
    else if (id === 'file') chooseFile();
    else if (id === 'entry') isPickingEntry = true;
    else if (id === 'template') isTemplating = true;
  }
  // --- The selection bar (W-4, W-6) ---------------------------------------------------------------

  const chosenAll = $derived(selected.map((id) => byId.get(id)).filter((e): e is Element => !!e));
  const showsBar = $derived(
    mode === 'surface' && chosenAll.length > 0 && !editing && gesture?.kind !== 'move' && gesture?.kind !== 'resize' && (!viewport.isCompact || barAsked),
  );
  /** Where the selection stands on the screen: the bar sits above it, centred. */
  const selectionOnScreen = $derived.by(() => {
    // A connector chosen alone stands where its line runs, between the objects it joins.
    const lines = chosenAll.filter((e) => e.kind === 'CONNECTOR').flatMap((e) => connectorPoints(e, byId));
    const area = bounds(chosenAll.filter((e) => e.kind !== 'CONNECTOR'))
      ?? bounds(lines.map((p, i): Element => ({ id: `line-${i}`, kind: 'TEXT', x: p.x, y: p.y, width: 0, height: 0 })));
    if (!area) return undefined;
    return {
      x: ((area.left + area.right) / 2 - view.x) * view.zoom,
      top: (area.top - view.y) * view.zoom,
      bottom: (area.bottom - view.y) * view.zoom,
    };
  });
  let barWidth = $state(0);
  const colourable = $derived(chosenAll.length > 0 && chosenAll.every((e) => e.kind === 'STICKY' || e.kind === 'SHAPE' || e.kind === 'STROKE'));
  const sizeable = $derived(chosen && (chosen.kind === 'STICKY' || chosen.kind === 'SHAPE' || chosen.kind === 'TEXT'));
  const SIZES = { S: 0.75, M: 1, L: 1.5 } as const;
  type Size = keyof typeof SIZES;
  const sizeNow = $derived.by<Size>(() => {
    if (!chosen) return 'M';
    const base = DEFAULT_SIZE[chosen.kind].width || 1;
    const ratio = chosen.width / base;
    return ratio < 0.875 ? 'S' : ratio > 1.25 ? 'L' : 'M';
  });
  function sizeTo(size: Size) {
    if (!chosen) return;
    const base = DEFAULT_SIZE[chosen.kind];
    model.update(chosen.id, { width: Math.round(base.width * SIZES[size]), height: Math.round(base.height * SIZES[size]) });
  }
  function colourAll(colour: (typeof BOARD_COLOURS)[number]) {
    for (const e of chosenAll) if (!e.locked) model.update(e.id, { color_token: colour });
  }
  /** An arrow from the chosen object: the next press on another object ends it. */
  function arrowFrom() {
    if (!chosen) return;
    pickTool('CONNECTOR', false);
    announcer.say(t('app.whiteboards.connect_hint_pointer'));
  }
  function commentHere() {
    commentAt = undefined;
    openThread = undefined;
    void openSheet('comments');
  }
  function lockAll() {
    const locking = chosenAll.some((e) => !e.locked);
    for (const e of chosenAll) model.update(e.id, { locked: locking });
    announcer.say(t(locking ? 'app.whiteboards.locked' : 'app.whiteboards.unlocked'));
  }
  /** Lines the chosen objects up along the first one's top edge. */
  function alignTop() {
    const top = Math.min(...chosenAll.map((e) => e.y));
    for (const e of chosenAll) if (!e.locked && e.y !== top) model.move([e.id], 0, top - e.y);
  }
  /** Lays the chosen notes out in rows, a gap between, from where the first stands. */
  function arrangeGrid() {
    const notes = chosenAll.filter((e) => e.kind !== 'CONNECTOR' && e.kind !== 'FRAME' && !e.locked);
    if (notes.length < 2) return;
    const columns = Math.ceil(Math.sqrt(notes.length));
    const left = Math.min(...notes.map((e) => e.x));
    const top = Math.min(...notes.map((e) => e.y));
    const w = Math.max(...notes.map((e) => e.width)) + 20;
    const h = Math.max(...notes.map((e) => e.height)) + 20;
    notes.forEach((e, i) => model.move([e.id], left + (i % columns) * w - e.x, top + Math.floor(i / columns) * h - e.y));
  }
  const moreItems = $derived<MenuItem[]>([
    ...(chosen && chosen.kind !== 'CARD' && chosen.kind !== 'CONNECTOR' && chosen.kind !== 'STROKE' && chosen.kind !== 'LINK' ? [{ id: 'edit', label: t('app.whiteboards.edit_words'), icon: 'pencil' as const }] : []),
    ...(chosen?.kind === 'CARD' && cardOf(chosen)?.available ? [{ id: 'open', label: t('app.whiteboards.card_open_entry'), icon: 'arrow-right' as const }] : []),
    ...(chosen?.kind === 'FILE' ? [{ id: 'file', label: t('app.whiteboards.file_open_button'), icon: 'file-text' as const }] : []),
    ...(chosen?.kind === 'SHAPE' ? SHAPES.map((each) => ({ id: `shape:${each}`, label: t(`app.whiteboards.shape_${each.toLowerCase()}`), icon: 'shapes' as const })) : []),
    ...(chosen?.kind === 'CONNECTOR' ? ROUTES.map((each) => ({ id: `route:${each}`, label: t(`app.whiteboards.route_${each.toLowerCase()}`), icon: 'move-up-right' as const })) : []),
    ...(chosen?.kind === 'LINK' ? [{ id: 'address', label: t('app.whiteboards.link_address'), icon: 'link' as const }] : []),
    ...(chosen?.kind === 'CONNECTOR' ? [] : [{ id: 'connect', label: t('app.whiteboards.connect_to'), icon: 'move-up-right' as const, hasSeparatorBefore: true }]),
    { id: 'add', label: t('app.whiteboards.add_to_selection'), icon: 'plus' },
    ...(isGrouped ? [{ id: 'ungroup', label: t('app.whiteboards.ungroup'), icon: 'group' as const }] : selected.length > 1 ? [{ id: 'group', label: t('app.whiteboards.group'), icon: 'group' as const }] : []),
    { id: 'duplicate', label: t('app.whiteboards.duplicate'), icon: 'copy' },
    ...(chosen ? [{ id: 'front', label: t('app.whiteboards.front'), icon: 'layers' as const }, { id: 'back', label: t('app.whiteboards.back'), icon: 'layers' as const }] : []),
    { id: 'remove', label: t('app.whiteboards.remove'), icon: 'trash', isDestructive: true, hasSeparatorBefore: true },
  ]);
  /** The ⋯ of the bar, and the keyboard's way to connect and to add to the choice (WCAG 2.5.7). */
  let picking = $state<'connect' | 'add' | 'address' | undefined>(undefined);
  let pickedId = $state('');
  function moreChose(id: string) {
    const one = chosen;
    if (id === 'edit' && one) edit(one.id);
    else if (id === 'open' && one) openCard(one);
    else if (id === 'file' && one) void openFile(one.media_id);
    else if (id.startsWith('shape:') && one) model.update(one.id, { shape: id.slice(6) as Shape });
    else if (id.startsWith('route:') && one) model.update(one.id, { route: id.slice(6) as Element['route'] });
    else if (id === 'connect' || id === 'add' || id === 'address') { picking = id; pickedId = id === 'address' ? one?.url ?? '' : ''; }
    else if (id === 'group') group();
    else if (id === 'ungroup') ungroup();
    else if (id === 'duplicate') duplicate();
    else if (id === 'front' && one) model.front(one.id);
    else if (id === 'back' && one) model.back(one.id);
    else if (id === 'remove') remove();
  }
  function picked2() {
    const kind = picking;
    picking = undefined;
    if (kind === 'connect' && chosen) connectTo(chosen.id, pickedId);
    else if (kind === 'add' && pickedId) choose([...selected, pickedId]);
    else if (kind === 'address' && chosen) {
      const url = linkAddress(pickedId);
      if (url) model.update(chosen.id, { url });
      else toast.show({ text: t('whiteboards.link_url_invalid'), tone: 'danger' });
    }
  }

  // --- Following and the frames paged on a phone (W-5, §7) ----------------------------------------

  let isPeopleOpen = $state(false);
  const followed = $derived(following === undefined ? undefined : peers.find((peer) => peer.id === following));
  let framePage = $state(0);
  function showFrame(i: number) {
    const frame = frames[i];
    if (!frame) return;
    framePage = i;
    view = fit(view, box(frame));
  }
  function addMinute() {
    session.extendTimer(60_000, Date.now());
  }

  const editingElement = $derived(editing ? byId.get(editing) : undefined);
  const drafting = $derived(gesture?.kind === 'draw' ? gesture.points : undefined);
</script>

<svelte:window onkeyup={keyUp} onblur={() => (spaceHeld = false)} oncopy={(event) => copied(event, false)} oncut={(event) => copied(event, true)} onpaste={pasted} />

<div class="board" data-board data-page-kind="work">
  <div
    class="stage"
    class:following={followed !== undefined}
    style:--follow={followed ? tokens.label[peerColour(followed.id)].fg : undefined}
    data-sheet={sheet ? '' : undefined}
    data-mode={mode}
    bind:clientWidth={view.width}
    bind:clientHeight={view.height}
  >
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <svg
      bind:this={surface}
      class="canvas"
      class:drawing={tool !== 'select' && tool !== 'HAND' && !spaceHeld}
      class:panning={tool === 'HAND' || spaceHeld}
      role="application"
      aria-label={t('app.whiteboards.canvas', { title: board.title, count: String(elements.length) })}
      aria-roledescription={t('app.whiteboards.canvas_role')}
      aria-describedby="board-canvas-hint"
      tabindex="0"
      {viewBox}
      onpointerdown={down}
      onpointermove={moveTo}
      onpointerup={up}
      onwheel={wheel}
      onkeydown={key}
      ondblclick={(event) => {
        const target = hit(elements, pointOf(event));
        if (target && (target.kind === 'FILE' || target.kind === 'IMAGE')) openFile(target.media_id);
        else if (target) edit(target.id);
      }}
    >
      <defs>
        <marker id="wb-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z" class="head" />
        </marker>
        <pattern id="wb-grid" width={GRID} height={GRID} patternUnits="userSpaceOnUse">
          <circle cx="1" cy="1" r="1" class="dot" />
        </pattern>
      </defs>
      <!-- The dots are the surface's texture; the grid switched on is what objects snap to. -->
      <rect class="grid" class:on={isGridOn} x={view.x} y={view.y} width={view.width / view.zoom} height={view.height / view.zoom} fill="url(#wb-grid)" />
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
          {@const record = media.recordOf(e.media_id)}
          {@const page = record ? firstPages[record.id] : undefined}
          {@const foot = Math.min(e.height * 0.4, 64)}
          {@const mark = record ? markOf(record) : ''}
          <!-- The scene's file card: the first page above, the mark, the name and pages · size below (F-4). -->
          <g class="card file" class:chosen={selected.includes(e.id)} data-board-file={record ? fileViewOf(record) : 'card'}>
            <path d={outlinePath(e)} />
            <rect x={e.x + 1} y={e.y + 1} width={Math.max(e.width - 2, 0)} height={Math.max(e.height - foot - 1, 0)} class="file-top" />
            {#if page}
              <image href={page.url} x={e.x + e.width * 0.3} y={e.y + 12} width={e.width * 0.4} height={Math.max(e.height - foot - 18, 0)} preserveAspectRatio="xMidYMin meet" />
            {/if}
            {#if mark}
              <rect x={e.x + 10} y={e.y + 10} width={mark.length * 8 + 12} height={18} rx={3} class="file-mark" />
              <text x={e.x + 16} y={e.y + 23} class="file-mark-text">{mark}</text>
            {/if}
            <text x={e.x + 12} y={e.y + e.height - foot + 24} class="words">{lines(media.fileNameOf(e.media_id) ?? t('app.whiteboards.card_unavailable'), e.width - 48, 14)[0] ?? ''}</text>
            <text x={e.x + 12} y={e.y + e.height - foot + 46} class="key">{e.text ? (lines(e.text, e.width - 48, 11)[0] ?? '') : fileLineOf(e.media_id)}</text>
            <g
              role="button"
              tabindex="-1"
              class="open"
              aria-label={t('app.whiteboards.file_open', { name: media.fileNameOf(e.media_id) ?? '' })}
              onclick={() => openFile(e.media_id)}
              onkeydown={(event) => { if (event.key === 'Enter') openFile(e.media_id); }}
            ><text x={e.x + e.width - 12} y={e.y + e.height - foot + 24} text-anchor="end" class="open">↗</text></g>
          </g>
        {:else if e.kind === 'CARD'}
          {@const target = cardOf(e)}
          <g class="card" class:chosen={selected.includes(e.id)} class:done={target?.completed}>
            <path d={outlinePath(e)} />
            {#if target?.available}
              {@const state = target.status_name ?? t(`app.whiteboards.card_state_${(target.status_category ?? (target.completed ? 'DONE' : 'TODO')).toLowerCase()}`)}
              {@const person = target.assignee_id ? (accounts.nameOf(target.assignee_id) ?? '') : ''}
              <!-- A note made a task is that task, live: key, person, status and tick (W-6). The key opens it. -->
              <rect class="card-top" x={e.x} y={e.y} width={e.width} height="3" />
              <a
                href={`/items/${target.id}`}
                onclick={(event) => { event.preventDefault(); onnavigate(`/items/${target.id}`); }}
                aria-label={t('app.whiteboards.card_open', { title: target.title ?? '' })}
              ><text x={e.x + 12} y={e.y + 24} class="key open-key">{target.key ?? '↗'}</text></a>
              {#each lines(target.title ?? '', e.width - 36, 14).slice(0, Math.max(1, Math.floor((e.height - 76) / 18))) as row, i (i)}
                <text x={e.x + 12} y={e.y + 48 + i * 18} class="words">{row}</text>
              {/each}
              {#if target.assignee_id}
                <g class="person" data-card-person={target.assignee_id}>
                  <title>{person}</title>
                  <circle cx={e.x + e.width - 22} cy={e.y + 19} r="10" />
                  <text x={e.x + e.width - 22} y={e.y + 22.5} text-anchor="middle">{initialsOf(person)}</text>
                </g>
              {/if}
              <g class={`state ${(target.status_category ?? (target.completed ? 'DONE' : 'TODO')).toLowerCase()}`}>
                <rect x={e.x + 42} y={e.y + e.height - 33} width={Math.min(e.width - 54, 16 + state.length * 7.5)} height="20" rx="4" />
                <text x={e.x + 50} y={e.y + e.height - 19}>{state}</text>
              </g>
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
                  <rect x={e.x + 12} y={e.y + e.height - 34} width="22" height="22" rx="4" />
                  {#if target.completed}<text x={e.x + 23} y={e.y + e.height - 17} text-anchor="middle">✓</text>{/if}
                </g>
              {/if}
            {:else}
              <text x={e.x + 12} y={e.y + 30} class="words quiet">{t('app.whiteboards.card_unavailable')}</text>
            {/if}
          </g>
        {:else}
          <g class={`object ${e.kind.toLowerCase()}`} class:chosen={selected.includes(e.id)} class:locked={e.locked}>
            {#if e.kind !== 'TEXT'}<path d={outlinePath(e)} style:fill={e.kind === 'STICKY' || e.color_token ? fillOf(e) : undefined} />{/if}
            {#if editing !== e.id && e.kind === 'SHAPE'}
              {@const placed = shapeWords(e, e.text ?? '', 14)}
              {#each placed.rows as row, i (i)}
                <text x={placed.x} y={row.y} text-anchor="middle" class="words">{row.text}</text>
              {/each}
            {:else if editing !== e.id}
              {@const by = e.kind === 'STICKY' ? makerName(e) : ''}
              {#each lines(e.text ?? '', e.width - 24, 14).slice(0, Math.max(1, Math.floor((e.height - (by ? 34 : 12)) / 18))) as row, i (i)}
                <text x={e.x + 12} y={e.y + 24 + i * 18} class="words">{row}</text>
              {/each}
              {#if by}<text x={e.x + 12} y={e.y + e.height - 12} class="maker" data-note-maker={makers.get(e.id)}>{by}</text>{/if}
            {/if}
          </g>
        {/if}
        {#if results.counts.get(e.id)}
          <g class="votes" aria-label={t('app.whiteboards.votes_on', { count: String(results.counts.get(e.id)) })}>
            <circle cx={voteX(e)} cy={e.y} r={14 / view.zoom} />
            <text x={voteX(e)} y={e.y + 5 / view.zoom} text-anchor="middle" font-size={13 / view.zoom}>{results.counts.get(e.id)}</text>
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
        {@const r = 5 / view.zoom}
        <!-- Drag an edge's dot to draw an arrow, the corner to size it; "+" adds the neighbour (W-4). -->
        {#if chosen.kind !== 'FRAME'}
          {#each [[chosen.x + chosen.width / 2, chosen.y], [chosen.x + chosen.width, chosen.y + chosen.height / 2], [chosen.x + chosen.width / 2, chosen.y + chosen.height], [chosen.x, chosen.y + chosen.height / 2]] as [cx, cy], i (i)}
            <circle class="dot-handle" {cx} {cy} {r} onpointerdown={(event) => handleStart(event, chosen)} role="presentation" data-board-handle />
          {/each}
        {/if}
        <rect class="handle" x={chosen.x + chosen.width - r} y={chosen.y + chosen.height - r} width={2 * r} height={2 * r} onpointerdown={(event) => resizeStart(event, chosen)} role="presentation" />
        {#if chosen.kind === 'STICKY' || chosen.kind === 'SHAPE'}
          <g class="plus" role="button" tabindex="-1" aria-label={t('app.whiteboards.add_neighbour')} transform={`translate(${chosen.x + chosen.width + 14 / view.zoom} ${chosen.y + chosen.height / 2}) scale(${1 / view.zoom})`}
            onpointerdown={(event) => { event.stopPropagation(); addNeighbour(chosen); }} data-board-plus>
            <circle r="10" /><path d="M -5 0 H 5 M 0 -5 V 5" />
          </g>
        {/if}
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
          onpointerdown={(event) => { event.stopPropagation(); openThread = pin.thread.id; sheet = 'comments'; }}
          onkeydown={(event) => { if (event.key === 'Enter') { openThread = pin.thread.id; sheet = 'comments'; } }}
        >
          <!-- Unseen: the pin's press is layout.target square around its drawn mark (§7). -->
          <rect class="pin-hit" x="-5" y="-39" width="44" height="44" />
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
    {#if showsBar && writable && selectionOnScreen}
      <!-- The bar above the selection, in a fixed order (W-4): colour, size, arrow, comment, lock, task from it, ⋯. -->
      <FloatingBar
        class={['selbar', selectionOnScreen.top < 120 && 'below']}
        role="toolbar"
        aria-label={selected.length > 1 ? t('app.whiteboards.several', { count: String(selected.length) }) : t(`app.whiteboards.kind_${chosen?.kind.toLowerCase() ?? 'sticky'}`)}
        bind:width={barWidth}
        at={{ inlineStart: `${Math.min(Math.max(selectionOnScreen.x, barWidth / 2 + 8), view.width - barWidth / 2 - 8)}px`, blockStart: `${selectionOnScreen.top < 120 ? selectionOnScreen.bottom + 16 : selectionOnScreen.top - 16}px` }}
        data-board-bar
      >
        {#if selected.length > 1}
          <span class="bar-words">{t('app.whiteboards.several', { count: String(selected.length) })}</span>
          <span class="seam" aria-hidden="true"></span>
        {/if}
        {#if colourable}
          <div class="swatches" role="radiogroup" aria-label={t('app.whiteboards.colour')}>
            {#each BOARD_COLOURS as colour (colour)}
              <button type="button" role="radio" aria-checked={chosen?.color_token === colour} aria-label={t(`app.colour.${colour}`)} class="swatch" style:background={tokens.label[colour].bg} onclick={() => colourAll(colour)}></button>
            {/each}
          </div>
          <span class="seam" aria-hidden="true"></span>
        {/if}
        {#if sizeable}
          <Menu label={t('app.whiteboards.size')} items={(['S', 'M', 'L'] as const).map((each) => ({ id: each, label: t(`app.whiteboards.size_${each.toLowerCase()}`) }))} onselect={(id: string) => sizeTo(id as 'S' | 'M' | 'L')} placement={{ side: 'block-start', align: 'center' }}>
            {#snippet trigger(props)}
              <Button size="sm" tone="subtle" icon="type" data-bar="size" {...props}>{sizeNow}</Button>
            {/snippet}
          </Menu>
        {/if}
        {#if chosen && chosen.kind !== 'CONNECTOR' && chosen.kind !== 'STROKE'}
          <IconButton size="sm" icon="move-up-right" label={t('app.whiteboards.arrow_from')} data-bar="arrow" onclick={arrowFrom} />
        {/if}
        <IconButton size="sm" icon="message-circle" label={t('app.whiteboards.comment_here')} data-bar="comment" onclick={commentHere} />
        <IconButton size="sm" icon="lock" tone="subtle" aria-pressed={chosenAll.every((e) => e.locked)} label={t(chosenAll.every((e) => e.locked) ? 'app.whiteboards.unlock' : 'app.whiteboards.lock')} data-bar="lock" onclick={lockAll} />
        {#if selected.length > 1}
          <IconButton size="sm" icon="align-start-vertical" label={t('app.whiteboards.align_top')} onclick={alignTop} />
          <IconButton size="sm" icon="layout-grid" label={t('app.whiteboards.arrange')} onclick={arrangeGrid} />
        {/if}
        {#if notesToTurn.length > 0}
          <span class="seam" aria-hidden="true"></span>
          <Button size="sm" tone={isMakingEntry ? 'primary' : 'secondary'} icon="square-check" aria-expanded={isMakingEntry} data-bar="task"
            onclick={() => { isMakingEntry = !isMakingEntry; entryCollection = entryCollection || collections[0]?.id || ''; entryKind = ''; }}>
            {notesToTurn.length > 1 || chosen?.kind === 'FRAME' ? t('app.whiteboards.make_entries', { count: String(notesToTurn.length) }) : t('app.whiteboards.make_entry')}
          </Button>
        {/if}
        <Menu label={t('app.whiteboards.more')} items={moreItems} onselect={moreChose} placement={{ side: 'block-start', align: 'end' }}>
          {#snippet trigger(props)}
            <IconButton size="sm" icon="ellipsis" label={t('app.whiteboards.more')} data-bar="more" {...props} />
          {/snippet}
        </Menu>
        {#if isMakingEntry}
          <!-- Task from it asks collection and type (W-6): the words become the title, the note the card. -->
          <div class="make" role="group" aria-labelledby="board-make-title" data-board-make>
            <p class="make-title" id="board-make-title">{notesToTurn.length > 1 ? t('app.whiteboards.make_entries_title', { count: String(notesToTurn.length) }) : t('app.whiteboards.make_entry_title')}</p>
            <p class="quiet small">{t('app.whiteboards.make_entry_hint')}</p>
            {#if collections.length === 0}
              <p class="quiet">{t('app.whiteboards.no_collections')}</p>
            {:else}
              <Select size="sm" label={t('app.whiteboards.entry_in')} bind:value={entryCollection} options={collections.map((each) => ({ value: each.id, label: each.name }))} />
              {#if kinds.length > 1}
                <div class="kinds" role="radiogroup" aria-label={t('app.whiteboards.entry_type')}>
                  {#each kinds as each, i (each.key)}
                    <button type="button" role="radio" class="kind" aria-checked={entryKind ? entryKind === each.key : i === 0} onclick={() => (entryKind = each.key)}>{each.name ?? t('app.whiteboards.entry_type_task')}</button>
                  {/each}
                </div>
              {/if}
              <div class="make-foot">
                <Button size="sm" tone="subtle" onclick={() => (isMakingEntry = false)}>{t('app.links.cancel')}</Button>
                <Button size="sm" tone="primary" onclick={() => void makeEntries()}>{notesToTurn.length > 1 ? t('app.whiteboards.make_entries_confirm', { count: String(notesToTurn.length) }) : t('app.whiteboards.make_entry_confirm')}</Button>
              </div>
            {/if}
          </div>
        {/if}
      </FloatingBar>
    {/if}
    {#if editingElement}
      <textarea
        class="editor"
        bind:this={editor}
        aria-label={t('app.whiteboards.words')}
        style:inset-inline-start={`${(editingElement.x - view.x) * view.zoom}px`}
        style:inset-block-start={`${(editingElement.y - view.y) * view.zoom}px`}
        style:inline-size={`${editingElement.width * view.zoom}px`}
        style:block-size={`${editingElement.height * view.zoom}px`}
        bind:value={draft}
        onblur={commitEdit}
        onkeydown={(event) => {
          if (event.key === 'Escape') { event.preventDefault(); commitEdit(); surface?.focus(); }
          else if (event.key === 'Tab' && !event.shiftKey && (editingElement.kind === 'STICKY' || editingElement.kind === 'SHAPE')) {
            event.preventDefault();
            const from = editingElement;
            commitEdit();
            addNeighbour(model.get(from.id) ?? from);
          }
        }}
        maxlength="2000"
      ></textarea>
    {/if}
    <VisuallyHidden><span id="board-canvas-hint">{t(writable ? 'app.whiteboards.hint_write' : 'app.whiteboards.hint_read')}</span></VisuallyHidden>
    {#if elements.length === 0 && (isLive || status !== 'connecting')}
      <div class="blank" class:beginning={isBeginning} data-board-blank>
        {#if writable}
          <!-- The first use's hint (scene wnew 2): the three ways in, as keys. -->
          <span class="blank-mark"><ProductTile icon="layout-template" product="whiteboards" size="md" /></span>
          <p class="blank-title">{t('app.whiteboards.first_title')}</p>
          {#if viewport.isCompact}
            <p class="quiet">{t('app.whiteboards.first_phone')}</p>
          {:else}
            <p class="quiet keys" data-board-first-use>
              <Kbd>N</Kbd> {t('app.whiteboards.first_note')} · <Kbd>{t('app.whiteboards.first_double')}</Kbd> {t('app.whiteboards.first_double_does')} · <Kbd>{t('app.whiteboards.first_space')}</Kbd> {t('app.whiteboards.first_space_does')}
            </p>
          {/if}
          {#if !isBeginning}
            <Button size="sm" tone="secondary" icon="layout-template" onclick={() => (isTemplating = true)}>{t('app.whiteboards.template_put_menu')}</Button>
          {/if}
        {:else}
          <p class="blank-title">{t('app.whiteboards.blank_read')}</p>
        {/if}
      </div>
    {/if}
    {#if isBeginning}
      <!-- The overlay ladder's own dialog: closing it without a choice leaves the board empty. -->
      <Dialog size="l" isOpen={true} title={t('app.whiteboards.begin_title')} dismissLabel={t('app.whiteboards.begin_leave_empty')}
        onClose={() => { if (isBeginning) begin('BLANK'); }}>
        <div class="begin" data-board-begin>
          <p class="quiet">{t('app.whiteboards.begin_body')}</p>
          <BoardTemplates look="tiles" bind:chosen={templateStart} onpick={begin} />
        </div>
        {#snippet actions()}
          <Button tone="secondary" onclick={() => begin('BLANK')}>{t('app.whiteboards.begin_leave_empty')}</Button>
        {/snippet}
      </Dialog>
    {/if}

    {#if mode === 'list'}
      <BoardList {elements} {frames} cardOf={cardOf} votes={results.counts} {pins} {writable} {wordsOf}
        onshow={(id) => { mode = 'surface'; const e = byId.get(id); if (e) { choose([id]); view = fit(view, box(e.kind === 'FRAME' ? e : { ...e, x: e.x - 200, y: e.y - 200, width: e.width + 400, height: e.height + 400 })); } }}
        ontick={(e) => void tick(e)} />
    {/if}

    <!-- The four islands and the dock (W-1): no head above the board. -->
    <FloatingBar class="island top-left" data-board-island="title" bind:element={titleIsland} bind:width={titleWidth}>
      {#if back}<IconButton size="sm" icon="chevron-left" label={t('app.whiteboards.back_to_boards')} onclick={() => onnavigate(back)} />{/if}
      <div class="name">{@render heading()}</div>
      <span class="status" class:live={isLive && unsent === 0 && status !== 'offline'} aria-live="polite" data-board-status>
        {#if isLive && unsent === 0 && status !== 'offline' && status !== 'refused'}<Icon name="check" size="sm" /><VisuallyHidden>{statusText}</VisuallyHidden>{:else}{statusText}{/if}
      </span>
      {#if !viewport.isCompact}
        <div class="modes" role="group" aria-label={t('app.whiteboards.mode')}>
          <button type="button" class="mode" aria-pressed={mode === 'surface'} onclick={() => (mode = 'surface')} data-board-mode="surface"><Icon name="layout-template" size="sm" />{t('app.whiteboards.mode_surface')}</button>
          <button type="button" class="mode" aria-pressed={mode === 'list'} onclick={() => (mode = 'list')} data-board-mode="list"><Icon name="list" size="sm" />{t('app.whiteboards.mode_list')}</button>
        </div>
      {/if}
      <Menu label={t('app.whiteboards.board_menu')} items={menuItems} onselect={menuChose} placement={{ side: 'block-end', align: 'end' }}>
        {#snippet trigger(props)}
          <IconButton size="sm" icon="ellipsis" label={t('app.whiteboards.board_menu')} data-board-menu {...props} />
        {/snippet}
      </Menu>
    </FloatingBar>

    <FloatingBar class="island top-right" data-board-island="people" bind:element={peopleIsland} bind:width={peopleWidth}>
      {#if peers.length > 0}
        <Popover label={t('app.whiteboards.here_now')} bind:isOpen={isPeopleOpen} placement={{ side: 'block-end', align: 'end' }}>
          {#snippet trigger(props)}
            <button type="button" class="faces" data-board-here {...props}>
              <AvatarGroup people={peers.map((peer) => ({ name: peer.name }))} max={3} size="sm" overflowLabel={t('app.whiteboards.here_more', { count: Math.max(0, peers.length - 3) })} />
              <VisuallyHidden>{t('app.whiteboards.here_now')}: {peers.map((peer) => peer.name).join(', ')}</VisuallyHidden>
            </button>
          {/snippet}
          <div class="people" data-board-people>
            <p class="people-title">{t('app.whiteboards.here_count', { count: String(peers.length + 1) })}</p>
            {#each peers as peer (peer.id)}
              <div class="person">
                <Avatar name={peer.name} size="sm" />
                <span class="person-name">{peer.name}</span>
                {#if peer.view}
                  <Button size="sm" tone="subtle" aria-pressed={following === peer.id} onclick={() => { following = following === peer.id ? undefined : peer.id; peersChanged(); }}>
                    {t(following === peer.id ? 'app.whiteboards.following_you' : 'app.whiteboards.follow')}
                  </Button>
                {/if}
              </div>
            {/each}
            <div class="person"><Avatar name={actor.account?.display_name ?? ''} size="sm" /><span class="person-name">{t('app.whiteboards.you')}</span></div>
            <p class="quiet small">{t('app.whiteboards.follow_ends')}</p>
          </div>
        </Popover>
        <span class="seam" aria-hidden="true"></span>
      {/if}
      {#if !viewport.isCompact || voting || timerState}
        <Button size="sm" tone="subtle" icon="timer" aria-label={viewport.isCompact || isTablet ? t('app.whiteboards.moderate') : undefined} aria-pressed={sheet === 'session'} data-board-sheet="session" onclick={() => toggleSheet('session')}>
          {viewport.isCompact || isTablet ? '' : t('app.whiteboards.moderate')}
        </Button>
      {/if}
      <Button size="sm" tone="subtle" icon="message-square" aria-pressed={sheet === 'comments'} aria-label={openThreads > 0 ? t('app.whiteboards.comments_open', { count: openThreads }) : t('app.whiteboards.comments_button')} data-board-sheet="comments" onclick={() => toggleSheet('comments')}>
        {openThreads > 0 ? String(openThreads) : ''}
      </Button>
      {#if !viewport.isCompact}
        <IconButton size="sm" icon="rotate-ccw-clock" tone="subtle" aria-pressed={sheet === 'history'} label={t('app.whiteboards.history')} data-board-sheet="history" onclick={() => toggleSheet('history')} />
      {/if}
    </FloatingBar>

    {#if followed}
      <p class="follow-note" data-board-following>{t('app.whiteboards.following_note', { name: followed.name })}</p>
    {/if}

    {#if viewport.isCompact && frames.length > 1 && mode === 'surface'}
      <!-- On a phone the frames are paged at the top (§7). -->
      <FloatingBar class="island pager" role="group" aria-label={t('app.whiteboards.frames')} data-board-pager>
        <IconButton size="sm" icon="chevron-left" label={t('app.whiteboards.frame_previous')} disabledReason={framePage > 0 ? undefined : t('app.whiteboards.frame_first')} onclick={() => showFrame(framePage - 1)} />
        <span class="pager-name">{frames[framePage]?.text || t('app.whiteboards.kind_frame')} · {framePage + 1}/{frames.length}</span>
        <IconButton size="sm" icon="chevron-right" label={t('app.whiteboards.frame_next')} disabledReason={framePage < frames.length - 1 ? undefined : t('app.whiteboards.frame_last')} onclick={() => showFrame(framePage + 1)} />
      </FloatingBar>
    {/if}

    {#if remaining || voting}
      <!-- The session bar: the running timer and vote, the same for everyone (W-5). -->
      <FloatingBar class={['island session-bar', !isSessionLevel && 'below', !isSessionLevel && !!sheet && sheet !== 'frames' && !viewport.isCompact && 'beside']} role="group" aria-label={t('app.whiteboards.session')} data-board-session bind:width={sessionWidth}>
        {#if remaining}
          <Icon name="timer" size="sm" />
          <span class="clock" class:over={!remaining.running && remaining.left === 0} role="timer" aria-label={t('app.whiteboards.timer_left', { time: clockOf(remaining.left) })}>{clockOf(remaining.left)}</span>
          {#if writable}
            {#if remaining.running}
              <IconButton size="sm" icon="pause" label={t('app.whiteboards.timer_pause')} onclick={() => session.pauseTimer(Date.now())} />
            {:else if remaining.left > 0}
              <IconButton size="sm" icon="play" label={t('app.whiteboards.timer_resume')} onclick={() => session.resumeTimer(Date.now())} />
            {/if}
            <Button size="sm" tone="subtle" icon="plus" onclick={() => addMinute()}>{t('app.whiteboards.timer_minute')}</Button>
            <IconButton size="sm" icon="x" label={t('app.whiteboards.timer_stop')} onclick={() => session.stopTimer()} />
          {/if}
        {/if}
        {#if remaining && voting}<span class="seam" aria-hidden="true"></span>{/if}
        {#if voting}
          <Icon name="vote" size="sm" />
          <span class="vote-words">
            <strong>{t(voting.anonymous ? 'app.whiteboards.vote_bar_anonymous' : 'app.whiteboards.vote_bar')}</strong>
            <span class="quiet small">{voting.open ? t('app.whiteboards.vote_bar_left', { remaining: String(votesLeft), count: String(voting.perPerson), voters: String(results.voters) }) : t('app.whiteboards.vote_bar_closed', { voters: String(results.voters) })}</span>
          </span>
          {#if writable && voting.open}
            <Button size="sm" tone="primary" onclick={() => session.endVoting()}>{t('app.whiteboards.vote_show')}</Button>
          {:else if writable}
            <Button size="sm" tone="subtle" onclick={() => session.clearVoting()}>{t('app.whiteboards.voting_clear')}</Button>
          {/if}
        {/if}
      </FloatingBar>
    {/if}

    {#if isFileHintShown}
      <FloatingBar class="island file-hint" role="status" data-board-file-hint>
        <div><strong>{t('app.whiteboards.file_hint_title')}</strong><p>{t('app.whiteboards.file_hint')}</p></div>
        <IconButton icon="x" size="sm" tone="subtle" label={t('app.whiteboards.file_hint_close')} onclick={() => (isFileHintShown = false)} />
      </FloatingBar>
    {/if}
    {#if writable}
      <FloatingBar class="island bottom-left" role="group" aria-label={t('app.whiteboards.history_steps')}>
        <IconButton size="sm" icon="undo-2" label={t('app.whiteboards.undo')} aria-keyshortcuts="Control+Z Meta+Z" disabledReason={canUndo ? undefined : t('app.whiteboards.nothing_to_undo')} onclick={undo} />
        {#if !viewport.isCompact}<IconButton size="sm" icon="redo-2" label={t('app.whiteboards.redo')} aria-keyshortcuts="Control+Shift+Z Meta+Shift+Z" disabledReason={canRedo ? undefined : t('app.whiteboards.nothing_to_redo')} onclick={redo} />{/if}
      </FloatingBar>
    {/if}

    {#if mode === 'surface'}
      <div class="dock" role="toolbar" aria-label={t('app.whiteboards.tools')} bind:this={toolbar} data-board-tools>
        {#each dockGroups as group, g (group.group)}
          {#if g > 0}<span class="seam" aria-hidden="true"></span>{/if}
          <div class="dock-group" role="group" aria-label={t(`app.whiteboards.group_${group.group}`)} data-dock-group={group.group}>
            {#each group.tools as each (each.tool)}
              <span class="dock-tool">
                <IconButton
                  size="sm"
                  icon={ICONS[each.tool]}
                  tone="subtle"
                  aria-pressed={tool === each.tool}
                  aria-keyshortcuts={each.key}
                  label={`${t(`app.whiteboards.tool_${each.tool.toLowerCase()}`)} (${each.key})`}
                  data-tool={each.tool}
                  onclick={(event: MouseEvent) => pickTool(each.tool, event.detail === 0, event.detail === 2)}
                />
                {#if !viewport.isCompact}<kbd class="key" aria-hidden="true">{each.key}</kbd>{/if}
                {#if tool === each.tool && held}<span class="held" aria-hidden="true"></span>{/if}
              </span>
            {/each}
          </div>
        {/each}
        {#if writable}
          <Menu label={t('app.whiteboards.insert')} items={insertItems} onselect={insertChose} placement={{ side: 'block-start', align: 'center' }}>
            {#snippet trigger(props)}
              {#if viewport.isCompact}
                <IconButton icon="ellipsis" label={t('app.whiteboards.insert')} data-board-insert {...props} />
              {:else if isTablet}
                <IconButton size="sm" icon="plus" label={t('app.whiteboards.insert')} data-board-insert {...props} />
              {:else}
                <Button size="sm" tone="secondary" icon="plus" data-board-insert {...props}>{t('app.whiteboards.insert_short')}</Button>
              {/if}
            {/snippet}
          </Menu>
        {/if}
        {#if viewport.isCompact && selected.length > 0 && writable}
          <IconButton icon="sliders-horizontal" label={t('app.whiteboards.properties')} onclick={() => (barAsked = !barAsked)} />
        {/if}
      </div>
      {#if tool === 'SHAPE'}
        <!-- The shapes in a fan at their tool (W-2). -->
        <div class="fan" role="radiogroup" aria-label={t('app.whiteboards.shape')} data-board-fan>
          {#each SHAPES as each (each)}
            <button type="button" role="radio" class="fan-shape" aria-checked={shape === each} onclick={() => (shape = each)}>{t(`app.whiteboards.shape_${each.toLowerCase()}`)}</button>
          {/each}
        </div>
      {/if}
    {/if}

    {#if viewport.isCoarse && !viewport.isCompact && writable && mode === 'surface' && !hasTouched}
      <p class="touch-hint" data-board-touch-hint><Icon name="pencil" size="sm" />{t('app.whiteboards.touch_hint')}</p>
    {/if}

    <div class="corner" class:raised={isTablet}>
      {#if map}
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
        <svg class="minimap" role="img" aria-label={t('app.whiteboards.minimap')} viewBox={`0 0 ${MAP.width} ${MAP.height}`} onclick={mapClick}>
          {#each map.boxes as b (b.id)}
            <path class={`mini ${b.kind.toLowerCase()}`} d={b.path} />
          {/each}
          <rect class="mini-view" x={map.view.left} y={map.view.top} width={map.view.width} height={map.view.height} />
        </svg>
      {/if}
      <FloatingBar class="island zoombar" role="group" aria-label={t('app.whiteboards.view_controls')} bind:element={zoombar}>
        {#if !viewport.isCompact && view.width >= 1100}
          <Button size="sm" tone="subtle" icon="frame" aria-pressed={sheet === 'frames'} data-board-sheet="frames" onclick={() => toggleSheet('frames')}>{t('app.whiteboards.tool_frame')}</Button>
        {:else if !viewport.isCompact}
          <IconButton size="sm" icon="frame" tone="subtle" aria-pressed={sheet === 'frames'} label={t('app.whiteboards.frames')} data-board-sheet="frames" onclick={() => toggleSheet('frames')} />
        {/if}
        <IconButton size="sm" icon="minus" label={t('app.whiteboards.zoom_out')} onclick={() => (view = zoomAt(view, { x: view.width / 2, y: view.height / 2 }, 1 / 1.25))} />
        <span class="zoom">{Math.round(view.zoom * 100)} %</span>
        <IconButton size="sm" icon="plus" label={t('app.whiteboards.zoom_in')} onclick={() => (view = zoomAt(view, { x: view.width / 2, y: view.height / 2 }, 1.25))} />
        <IconButton size="sm" icon="maximize-2" label={t('app.whiteboards.fit')} onclick={() => { const all = bounds(elements); if (all) view = fit(view, all); }} />
        <IconButton size="sm" icon="map" tone="subtle" aria-pressed={isMapShown} label={t('app.whiteboards.minimap')} onclick={() => (isMapShown = !isMapShown)} />
      </FloatingBar>
    </div>

    {#if sheet}
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <aside class="sheet" class:at-corner={sheet === 'frames'} aria-label={t('app.whiteboards.side')} data-board-side onkeydown={(event) => { if (event.key === 'Escape' && sheet) { event.stopPropagation(); closeSheet(); } }}>
        {#if sheet}
          <section class="part" aria-labelledby="board-sheet-title" data-board-part={sheet}>
            <div class="part-head">
              <h2 class="part-title" id="board-sheet-title" tabindex="-1" bind:this={sheetTitle}>{t(`app.whiteboards.sheet_${sheet}`)}</h2>
              <IconButton size="sm" icon="x" label={t('app.whiteboards.sheet_close')} onclick={closeSheet} />
            </div>
            {#if sheet === 'session'}
              <Stack gap="150">
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
              </Stack>
            {:else if sheet === 'comments'}
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
            {:else if sheet === 'history'}
              <!-- A log as the page system draws one: newest first, a named version bold, its verbs under ⋯. -->
              {#if !history}
                <Button size="sm" tone="subtle" icon="clock" onclick={() => void openHistory()}>{t('app.whiteboards.history_show')}</Button>
              {:else}
                <ol class="rows" aria-label={t('app.whiteboards.history')}>
                  {#each history as version (version.seq)}
                    <li class="row version" data-board-version={version.seq}>
                      <Icon name={version.name ? 'tag' : 'clock'} size="sm" />
                      <span class="row-text">
                        <span class="row-name">{version.name ?? formatDateTime(version.created_at, messages.locale)}</span>
                        <span class="quiet small">{version.name ? `${formatDateTime(version.created_at, messages.locale)} · ` : ''}{t('app.whiteboards.objects', { count: String(version.element_count) })}</span>
                      </span>
                      {#if writable && isLive}
                        <Menu label={t('app.whiteboards.version_more')}
                          items={[{ id: 'restore', label: t('app.whiteboards.version_restore'), icon: 'rotate-ccw' }, { id: 'name', label: t('app.whiteboards.version_name'), icon: 'tag' }]}
                          onselect={(id) => { if (id === 'restore') { restoring = version; isRestoreAsked = true; } else { namingSeq = version.seq; nameDraft = version.name ?? ''; } }}>
                          {#snippet trigger(props)}<IconButton {...props} size="sm" tone="subtle" icon="ellipsis" label={t('app.whiteboards.version_more')} />{/snippet}
                        </Menu>
                      {/if}
                      {#if namingSeq === version.seq}
                        <span class="row-form">
                          <Input size="sm" label={t('app.whiteboards.version_name')} bind:value={nameDraft} maxlength={120} />
                          <Button size="sm" tone="primary" onclick={() => void nameVersion(version.seq)}>{t('app.whiteboards.version_name_save')}</Button>
                        </span>
                      {/if}
                    </li>
                  {:else}
                    <li class="quiet none">{t('app.whiteboards.history_empty')}</li>
                  {/each}
                </ol>
              {/if}
            {:else if sheet === 'links'}
              <Stack gap="050">
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
            {:else if sheet === 'frames'}
              <!-- The frames in reading order, numbered as the phone pages them; a press fits one. -->
              <ol class="rows" aria-label={t('app.whiteboards.frames')}>
                {#each frames as frame, at (frame.id)}
                  <li>
                    <button type="button" class="row choice" class:current={selected.includes(frame.id)} data-board-frame-row={frame.id} onclick={() => { view = fit(view, box(frame)); choose([frame.id]); }}>
                      <span class="number">{at + 1}</span>
                      <span class="row-text">
                        <span class="row-name">{frame.text || t('app.whiteboards.kind_frame')}</span>
                        <span class="quiet small">{t('app.whiteboards.objects', { count: String(withinFrame(elements, frame).length - 1) })}</span>
                      </span>
                    </button>
                  </li>
                {:else}
                  <li class="quiet none">{t('app.whiteboards.no_frames')}</li>
                {/each}
              </ol>
            {:else if sheet === 'export'}
              <Stack gap="050">
                <Inline gap="050">
                  <Button size="sm" tone="subtle" onclick={() => void saveSvg(which)}>SVG</Button>
                  <Button size="sm" tone="subtle" onclick={() => void savePng(which)}>PNG</Button>
                  <Button size="sm" tone="subtle" onclick={() => void savePdf(which)}>PDF</Button>
                  <Button size="sm" tone="subtle" onclick={() => void saveNative()}>{t('app.whiteboards.export_native')}</Button>
                </Inline>
                <p class="quiet">{t(which === 'frame' ? 'app.whiteboards.export_frame' : which === 'selection' ? 'app.whiteboards.export_selection' : 'app.whiteboards.export_board')}</p>
              </Stack>
            {/if}
          </section>
        {/if}
      </aside>
    {/if}
  </div>
</div>

{#if isPickingEntry}
  <Dialog size="l" bind:isOpen={isPickingEntry} title={t('app.whiteboards.put_entry')} dismissLabel={t('app.links.cancel')} onClose={() => (picked = undefined)}
    primary={{ label: t('app.whiteboards.put_entry_confirm'), disabledReason: picked ? undefined : t('app.links.choose_first'), onclick: putEntry }}>
    <EntryPicker chosen={picked} onchoose={(item) => (picked = item)} />
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isPickingEntry = false)}>{t('app.links.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if restoring && isRestoreAsked}
  {@const asked = restoring}
  <Dialog size="s" bind:isOpen={isRestoreAsked} title={t('app.whiteboards.restore_title')} dismissLabel={t('app.links.cancel')} onClose={() => (restoring = undefined)}>
    <p>{t('app.whiteboards.restore_body', { when: formatDateTime(asked.created_at, messages.locale) })}</p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => { isRestoreAsked = false; restoring = undefined; }}>{t('app.links.cancel')}</Button>
      <Button tone="primary" onclick={() => void bringBack(asked.seq)}>{t('app.whiteboards.restore_confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isTemplating}
  <Dialog size="l" bind:isOpen={isTemplating} title={t('app.whiteboards.template_put_menu')} dismissLabel={t('app.links.cancel')}
    primary={{ label: t('app.whiteboards.template_put_button'), onclick: () => { const name = templateOf(templateStart); if (name) putTemplate(name); } }}>
    <BoardTemplates bind:chosen={templateStart} withBlank={false} />
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isTemplating = false)}>{t('app.links.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if picking}
  <Dialog size="s" isOpen={true} title={t(picking === 'connect' ? 'app.whiteboards.connect_to' : picking === 'add' ? 'app.whiteboards.add_to_selection' : 'app.whiteboards.link_address')} dismissLabel={t('app.links.cancel')} onClose={() => (picking = undefined)}>
    {#if picking === 'address'}
      <Input label={t('app.whiteboards.link_address')} type="url" bind:value={pickedId} placeholder="https://" />
    {:else}
      <Select label={t(picking === 'connect' ? 'app.whiteboards.connect_to' : 'app.whiteboards.add_to_selection')} placeholder={t('app.whiteboards.connect_to_choose')} bind:value={pickedId} options={others(picking === 'connect' && chosen ? [chosen.id] : selected)} />
    {/if}
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (picking = undefined)}>{t('app.links.cancel')}</Button>
      <Button tone="primary" disabledReason={pickedId ? undefined : t('app.links.choose_first')} onclick={picked2}>{t('app.whiteboards.pick_confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isLinking}
  <Dialog size="s" bind:isOpen={isLinking} title={t('app.whiteboards.link_put')} dismissLabel={t('app.links.cancel')}>
    <Input label={t('app.whiteboards.link_address')} type="url" bind:value={linkDraft} error={linkError} placeholder="https://" />
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isLinking = false)}>{t('app.links.cancel')}</Button>
      <Button tone="primary" disabledReason={linkDraft.trim() && !linkError ? undefined : t('whiteboards.link_url_invalid')} onclick={putLink}>{t('app.whiteboards.link_put_confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  /* The board is the screen and belongs to its content (W-1): no head above it; the controls are
     islands in the four corners and a dock at the foot. Below `expanded` the frame gives the page
     no height of its own, so the board takes the window's less the app bar: no bottom bar stands
     under it, a phone hiding it on a board (`AppFrame`). */
  .board { position: relative; block-size: 100%; min-block-size: 0; }
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    .board { block-size: calc(100dvh - var(--layout-appbar-height)); }
  }
  .stage { position: relative; block-size: 100%; min-block-size: 0; overflow: hidden; background: var(--bg-surface-sunken); }
  /* Following someone: a frame in their colour round the board (W-5). */
  .stage.following { box-shadow: inset 0 0 0 var(--bw-ring) var(--follow); }

  .board :global(.file-hint) { inset-inline-start: var(--sp-200); inset-block-end: calc(var(--sp-1000) + var(--sp-200)); align-items: start; max-inline-size: calc(var(--sp-1600) * 5); padding: var(--sp-150); }
  .board :global(.file-hint p) { margin: var(--sp-050) 0 0; color: var(--text-secondary); font-size: var(--fs-090); }
  /* The islands and the selection bar are the one FloatingBar; here only where they stand. */
  .board :global(:is(.island, .selbar)) { position: absolute; }
  .dock, .fan {
    position: absolute;
    display: flex;
    align-items: center;
    gap: var(--sp-025);
    box-sizing: border-box;
    padding: var(--sp-050);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
  }
  .board :global(.top-left) { inset-block-start: var(--sp-200); inset-inline-start: var(--sp-200); max-inline-size: calc(50% - var(--sp-300)); }
  .board :global(.top-right) { inset-block-start: var(--sp-200); inset-inline-end: var(--sp-200); }
  .board :global(.bottom-left) { inset-block-end: var(--sp-200); inset-inline-start: var(--sp-200); }
  .name { min-inline-size: 0; flex: 0 1 auto; padding-inline: var(--sp-050); }
  /* A long status gives way before the board's name does. */
  .status { display: inline-block; flex: 0 100 auto; min-inline-size: 0; overflow: hidden; color: var(--text-secondary); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
  .status.live { color: var(--text-success); }
  .modes { display: inline-flex; padding: var(--sp-025); border-radius: var(--r-md); background: var(--bg-surface-sunken); }
  .mode {
    all: unset;
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-sm);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    cursor: pointer;
  }
  .mode[aria-pressed='true'] { background: var(--bg-raised); color: var(--text-primary); font-weight: var(--fw-semibold); box-shadow: var(--shadow-raised); }
  .mode:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .seam { align-self: stretch; inline-size: var(--bw-hairline); margin-inline: var(--sp-050); background: var(--border-subtle); }
  .faces { all: unset; display: inline-flex; cursor: pointer; border-radius: var(--r-md); padding: var(--sp-025); }
  .faces:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .people { display: grid; gap: var(--sp-100); min-inline-size: var(--layout-gallery-card); }
  .people-title { margin: 0; font-weight: var(--fw-semibold); }
  .person { display: flex; align-items: center; gap: var(--sp-100); }
  .person-name { flex: 1 1 auto; }
  .small { font-size: var(--fs-075); }
  .follow-note {
    position: absolute;
    inset-block-start: 0;
    inset-inline: 0;
    margin: 0 auto;
    inline-size: fit-content;
    padding: var(--sp-025) var(--sp-150);
    border-radius: 0 0 var(--r-md) var(--r-md);
    background: var(--follow);
    color: var(--text-inverse);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
  }
  .board :global(.session-bar) {
    inset-block-start: var(--sp-200);
    inset-inline: 0;
    margin-inline: auto;
    inline-size: fit-content;
    max-inline-size: calc(100% - 2 * var(--sp-200));
    gap: var(--sp-100);
    padding-inline: var(--sp-150);
  }
  .board :global(.session-bar.below) { inset-block-start: calc(var(--sp-200) + var(--density-control-md-min) + var(--sp-200)); }
  /* Below the islands an open sheet stands at the same height: the bar centres in the room beside it. */
  .board :global(.session-bar.beside) {
    inset-inline-end: calc(var(--sp-200) + min(var(--layout-pane-width), calc(100% - 2 * var(--sp-200))) + var(--sp-200));
    max-inline-size: calc(100% - min(var(--layout-pane-width), calc(100% - 2 * var(--sp-200))) - 4 * var(--sp-200));
  }
  .vote-words { display: grid; line-height: var(--lh-tight); }
  .board :global(.pager) { inset-block-start: calc(var(--sp-100) + var(--density-control-md-min) + var(--sp-200)); inset-inline: 0; margin-inline: auto; inline-size: fit-content; }
  .pager-name { font-size: var(--fs-075); padding-inline: var(--sp-050); }

  /* The dock at the foot, grouped, each tool with its key (W-2). */
  .dock {
    inset-block-end: var(--sp-200);
    inset-inline: 0;
    margin-inline: auto;
    inline-size: fit-content;
    max-inline-size: calc(100% - 2 * var(--sp-200));
    overflow-x: auto;
  }
  .dock-group { display: flex; gap: var(--sp-025); }
  .dock-tool { position: relative; display: inline-flex; }
  .key {
    position: absolute;
    inset-block-end: 0;
    inset-inline-end: var(--sp-025);
    color: var(--text-subtle);
    font-family: inherit;
    font-size: var(--fs-050);
    line-height: 1;
    pointer-events: none;
  }
  .held { position: absolute; inset-block-start: var(--sp-025); inset-inline-end: var(--sp-025); inline-size: var(--sp-050); block-size: var(--sp-050); border-radius: var(--r-full); background: var(--accent-primary); }
  .fan {
    inset-block-end: calc(var(--sp-200) + var(--density-control-md-min) + var(--sp-300));
    inset-inline: 0;
    margin-inline: auto;
    inline-size: fit-content;
  }
  .fan-shape { all: unset; padding: var(--sp-050) var(--sp-100); border-radius: var(--r-sm); font-size: var(--fs-075); cursor: pointer; }
  .fan-shape[aria-checked='true'] { background: var(--accent-primary-subtle); color: var(--text-brand); font-weight: var(--fw-semibold); }
  .fan-shape:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }

  .corner {
    position: absolute;
    inset-block-end: var(--sp-200);
    inset-inline-end: var(--sp-200);
    display: grid;
    justify-items: end;
    gap: var(--sp-100);
  }
  .corner :global(.island) { position: static; }
  /* On a tablet the dock takes the foot's middle and the corner stands above it. */
  .corner.raised { inset-block-end: calc(var(--sp-200) + var(--density-control-md-min) + var(--sp-300)); }
  .touch-hint {
    position: absolute;
    inset-block-end: calc(var(--sp-200) + var(--density-control-md-min) + var(--sp-300));
    inset-inline: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    inline-size: fit-content;
    margin: 0 auto;
    padding: var(--sp-050) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-raised);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    pointer-events: none;
  }
  .zoom { min-inline-size: var(--sp-500); text-align: center; color: var(--text-primary); font-size: var(--fs-075); font-variant-numeric: tabular-nums; }
  .minimap {
    inline-size: var(--layout-gallery-card);
    block-size: var(--sp-1600);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-raised);
    box-shadow: var(--shadow-raised);
    cursor: pointer;
  }

  /* The bar above the selection (W-4), its foot on the selection's top edge. */
  .board :global(.selbar) { translate: -50% -100%; z-index: 1; white-space: nowrap; }
  .board :global(.selbar.below) { translate: -50% 0; }
  /* The bar's one primary act, tinted as the brand while its form is shut (scene wsel). */
  .bar-words { padding-inline: var(--sp-050); font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .make {
    position: absolute;
    inset-block-start: calc(100% + var(--sp-100));
    inset-inline-end: 0;
    inline-size: var(--layout-pane-width);
    max-inline-size: 90vw;
    display: grid;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
    white-space: normal;
  }
  .make-title { margin: 0; font-weight: var(--fw-semibold); }
  .make-foot { display: flex; justify-content: end; gap: var(--sp-100); }
  .kinds { display: flex; padding: var(--sp-025); border-radius: var(--r-md); background: var(--bg-surface-sunken); }
  .kind { all: unset; flex: 1 1 0; text-align: center; padding: var(--sp-050); border-radius: var(--r-sm); font-size: var(--fs-075); cursor: pointer; }
  .kind[aria-checked='true'] { background: var(--bg-raised); font-weight: var(--fw-semibold); box-shadow: var(--shadow-raised); }
  .kind:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }

  /* An empty board says what to do in its middle, where the eye is. */
  .blank {
    position: absolute;
    inset: 0;
    margin: auto;
    inline-size: fit-content;
    block-size: fit-content;
    display: grid;
    justify-items: center;
    gap: var(--sp-100);
    max-inline-size: min(calc(100% - 2 * var(--sp-300)), var(--layout-pane-width));
    text-align: center;
    pointer-events: none;
  }
  .blank :global(button) { pointer-events: auto; }
  .blank-title { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .blank.beginning { inset-block: calc(var(--sp-1600) + var(--sp-400)) auto; margin-block: 0; }
  .keys { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: var(--sp-050); }
  .begin { display: grid; gap: var(--sp-200); }

  /* The sheet lies over the canvas below the top-right island, so opening it never moves the board. */
  .sheet {
    position: absolute;
    inset-block-start: calc(var(--sp-200) + var(--density-control-md-min) + var(--sp-200));
    inset-inline-end: var(--sp-200);
    max-block-size: calc(100% - 2 * var(--sp-200) - 2 * var(--density-control-md-min) - 3 * var(--sp-200));
    box-sizing: border-box;
    inline-size: min(var(--layout-pane-width), calc(100% - 2 * var(--sp-200)));
    display: grid;
    align-content: start;
    gap: var(--sp-200);
    padding: var(--sp-200);
    overflow-y: auto;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
    z-index: 2;
  }
  .sheet.at-corner {
    inset-block: auto calc(var(--sp-200) + var(--density-control-md-min) + var(--sp-200));
    inline-size: min(calc(var(--layout-pane-width) * 0.8), calc(100% - 2 * var(--sp-200)));
  }
  .part { display: grid; gap: var(--sp-150); }
  .rows { display: grid; gap: var(--sp-025); margin: 0; padding: 0; list-style: none; }
  .row { display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: var(--sp-100); padding: var(--sp-100); border-radius: var(--r-md); color: var(--text-secondary); }
  .row.choice { inline-size: 100%; border: 0; background: none; font: inherit; text-align: start; cursor: pointer; }
  .row.choice:hover { background: var(--bg-surface-hover); }
  .row.choice.current { background: var(--accent-primary-subtle); }
  .row-text { display: grid; min-inline-size: 0; }
  .row-name { overflow: hidden; color: var(--text-primary); font-weight: var(--fw-semibold); text-overflow: ellipsis; white-space: nowrap; }
  .row-form { display: flex; grid-column: 1 / -1; align-items: end; gap: var(--sp-050); }
  .number { display: inline-grid; place-items: center; inline-size: var(--sp-300); block-size: var(--sp-300); border-radius: var(--r-md); background: var(--bg-surface-sunken); color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-semibold); font-variant-numeric: tabular-nums; }
  .small { font-size: var(--fs-075); }
  .none { padding: var(--sp-200) 0; text-align: center; }
  .part-head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .part-title { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .part-title:focus { outline: none; }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .board :global(.top-left) { inset-block-start: var(--sp-100); inset-inline-start: var(--sp-100); max-inline-size: calc(100% - 2 * var(--sp-100) - var(--sp-1600)); }
    .board :global(.top-right) { inset-block-start: var(--sp-100); inset-inline-end: var(--sp-100); }
    .status { display: none; }
    .dock { inset-block-end: var(--sp-100); }
    .board :global(.bottom-left) { display: none; }
    .corner { inset-block-end: calc(var(--sp-100) + var(--density-control-md-min) + var(--sp-300)); inset-inline-end: var(--sp-100); }
    .sheet {
      inset-block: auto 0;
      inset-inline: 0;
      inline-size: auto;
      max-block-size: 60%;
      border-radius: var(--r-xl) var(--r-xl) 0 0;
    }
  }
  .clock { font-variant-numeric: tabular-nums; font-weight: var(--fw-semibold); color: var(--text-primary); }
  .clock.over { color: var(--text-danger); }
  .canvas { display: block; inline-size: 100%; block-size: 100%; touch-action: none; cursor: grab; }
  .canvas.drawing { cursor: crosshair; }
  .canvas.panning { cursor: grab; }
  .canvas:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--bw-ring)); }
  .dot { fill: var(--border-default); }
  .grid { pointer-events: none; opacity: 0.6; }
  .grid.on { opacity: 1; }
  /* A frame is a lighter field the dots still show through, its edge one hairline. */
  .frame path { fill: var(--bg-canvas); fill-opacity: 0.6; stroke: var(--border-strong); }
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
  .maker { fill: var(--text-secondary); font-size: var(--fs-065); }
  .card path { fill: var(--bg-raised); stroke: var(--border-default); }
  .card path.pending { stroke-dasharray: 4 4; }
  .card .file-top { fill: var(--bg-surface-sunken); }
  .card .file-mark { fill: var(--bg-emphasis); }
  .card .file-mark-text { fill: var(--text-inverse); font-size: var(--fs-065); font-weight: var(--fw-bold); }
  .card.done .words { text-decoration: line-through; fill: var(--text-subtle); }
  .card .key { fill: var(--text-subtle); font-size: var(--fs-075); }
  .card .open { fill: var(--text-brand); font-size: var(--fs-100); }
  .card .open-key { cursor: pointer; }
  .card .person circle { fill: var(--accent-primary-subtle); stroke: var(--bg-raised); }
  .card .person text { fill: var(--text-brand); font-size: var(--fs-065); font-weight: var(--fw-semibold); }
  .tick rect { fill: var(--bg-surface); stroke: var(--border-strong); }
  .tick.done rect { stroke: var(--text-primary); }
  .tick text { fill: var(--text-primary); font-size: var(--fs-100); }
  .chosen path { stroke: var(--accent-primary); stroke-width: 3; }
  .drafting { fill: none; stroke: var(--accent-primary); stroke-dasharray: 4 4; stroke-width: 2; }
  .guide { stroke: var(--accent-primary); stroke-width: 1; stroke-dasharray: 4 3; pointer-events: none; }
  .handle { fill: var(--accent-primary); cursor: nwse-resize; }
  .dot-handle { fill: var(--bg-raised); stroke: var(--accent-primary); stroke-width: 1.5; cursor: crosshair; }
  .plus { cursor: pointer; }
  .plus circle { fill: var(--accent-primary-subtle); stroke: var(--accent-primary); }
  .plus path { stroke: var(--accent-primary); stroke-width: 1.5; }
  .card-top { fill: var(--accent-primary); }
  .state rect { fill: var(--bg-surface-sunken); stroke: var(--border-default); }
  .state text { fill: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .state.in_progress rect { fill: var(--accent-primary-subtle); stroke: var(--accent-primary); }
  .state.in_progress text { fill: var(--text-brand); }
  .state.done rect { fill: var(--bg-surface); }
  .votes circle { fill: var(--accent-primary); }
  .votes text { fill: var(--text-inverse); font-weight: var(--fw-semibold); }
  .pin { cursor: pointer; }
  .pin .pin-hit { fill: transparent; }
  .pin path { fill: var(--accent-primary); stroke: var(--bg-surface); stroke-width: 1.5; }
  .pin text { fill: var(--text-inverse); font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .pin.open path { stroke: var(--text-primary); stroke-width: 2.5; }
  .pin.resolved path { fill: var(--border-strong); }
  .peer { pointer-events: none; }
  .peer path { fill: currentColor; stroke: var(--bg-surface); stroke-width: 1; }
  .peer text { fill: currentColor; font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .peer-name { font-weight: var(--fw-semibold); }
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
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .swatches { display: flex; gap: var(--sp-050); padding-inline: var(--sp-050); }
  .swatch { inline-size: var(--sp-200); block-size: var(--sp-200); padding: 0; border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-full); cursor: pointer; }
  .swatch[aria-checked='true'] { outline: var(--bw-ring) solid var(--accent-primary); outline-offset: var(--bw-hairline); }
  .choices { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .choice:not(.row) { all: unset; cursor: pointer; padding: var(--sp-050); border-radius: var(--r-sm); color: var(--text-primary); }
  .choice:hover, .choice:focus-visible { background: var(--bg-surface-hover); }
</style>
