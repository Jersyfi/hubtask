<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entries of a collection, one level at a time.
  //
  // `LIST_COLLAPSED` and `LIST_EXPANDED` are not two components and not a prop: they are whether a
  // row's children are shown, one row at a time, and expanding is what fetches them. A layout that
  // read the whole subtree would be fetching levels nobody has opened.
  //
  // Which types may be created where comes from the **manifest** (`lib/data/capability.ts`), never
  // from the three names this product happens to have today: `domain-model.md` §2's extension
  // example — a new type is a profile entry and no code change — is only true if nothing here
  // spells them out.

  import { untrack } from 'svelte';

  import {
    Badge,
    Button,
    Checkbox,
    Dialog,
    EmptyState,
    ErrorState,
    focusFirst,
    Icon,
    IconButton,
    Inline,
    Input,
    LabelChip,
    LabelPicker,
    Popover,
    provideReasonGroup,
    rankIntent,
    rankTarget,
    Select,
    Skeleton,
    Stack,
    TaskRow,
    Textarea,
    type RankCommand,
  } from '@hubtask/design-system/components';
  import type { BulkResult, EffectiveStructure, WorkItem } from '@hubtask/sync-engine';

  import { capture, captureRulesOf, parseLine, unitsOf, type CaptureRules, type CapturedLine } from './capture.ts';
  import { narrow, partition, type Sections } from './simplelist.ts';
  import { classify, edgeInset, HOLD_MS as GESTURE_HOLD_MS, startsAtEdge, swipeThreshold, type Point } from './swipe.ts';

  import type { Archival } from '../data/containers.ts';
  import { containers } from '../data/containers.svelte.ts';

  import type { ItemsQuery } from '../data/items.svelte.ts';

  import EntryActions from './EntryActions.svelte';
  import EntryMenu from './EntryMenu.svelte';
  import CategoriesDialog from './CategoriesDialog.svelte';
  import LanguagePicker from './LanguagePicker.svelte';
  import Lookalikes from './Lookalikes.svelte';
  import { createDrag } from './dragging.svelte.ts';
  import { attachEntryGestures } from './entryGestures.ts';
  import { entryMenu } from './entryMenu.ts';
  import { offerUndo } from './undo.svelte.ts';

  import { announcer } from '../announce.svelte.ts';
  import { celebration } from '../celebration.svelte.ts';
  import CelebrationSlot from './CelebrationSlot.svelte';
  import { acceptsChild, childTypes, placementOf, rootTypes, supports } from '../data/capability.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { queue } from '../data/queue.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { selection } from '../data/selection.svelte.ts';
  import DueMark from './DueMark.svelte';
  import ReplicaMark from '../frame/ReplicaMark.svelte';
  import { outcomeOf } from '../data/bulk.ts';
  import { archivalOfItem, isBelowArchived } from '../data/lifecycle.ts';
  import { anchorFor } from '../data/rank.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { textLanguages } from '../data/query.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import PeopleMarks from '../people/PeopleMarks.svelte';
  import { renderProblem } from '../problem.ts';

  interface Props {
    collectionId: string;
    /**
     * The entry whose subtree this list is, instead of the collection's top level (ADR-0061,
     * design-system.md §11.8): the same flatten, the same rows, the same menus, mounted one level
     * down. With a root, the top level is the root's children, "add" creates under the root with
     * the types the manifest lets it take, direct children open and deeper levels start closed,
     * and the expansion is kept per entry on this device (design-system.md §11.10). The id is what design-system.md
     * §11.8 calls `rootId`; the type, the parent and the depth travel with it because the add form
     * and "move out" need them and the list should not read the entry a second time.
     */
    root?: Pick<WorkItem, 'id' | 'type' | 'parent_id' | 'depth'>;
    /** Whether the collection is archived — the entries in it are then read-only too (I-C3). */
    isReadOnly?: boolean;
    /**
     * While read-only, the sentence the page's one notice says and that notice's id: the rows say
     * nothing of their own, and the completion box each keeps - it is how a row says done - names
     * the notice as its description rather than repeating it (design-system.md §4, case 5).
     */
    readOnlyReason?: string;
    readOnlyNoticeId?: string;
    /** What the reader has asked of this level: a filter, an order. Built from the manifest. */
    query?: ItemsQuery;
    /**
     * `LIST_EXPANDED` rather than `LIST_COLLAPSED`: every row that takes children shows them.
     *
     * The layout is not a second component and not a prop on the row — it is
     * whether the children are shown, and expanding is still what fetches them. So this opens the
     * rows as they arrive rather than reading a subtree the reader has not asked for.
     */
    isExpanded?: boolean;
    /**
     * What the last bulk did, by entry.
     *
     * Handed in rather than read from a store, because the report belongs to the rows: a refusal
     * is about an entry and the place a reader looks for it is the row it happened to. An entry
     * that is not in the map was not part of that bulk.
     */
    lastResults?: ReadonlyMap<string, BulkResult>;
    /**
     * Asked to open an entry beside the list rather than on its page (ADR-0061 decision 4). The
     * view decides, because whether there is room beside the list is the view's width; a list
     * without it leaves the rows as the links they are.
     */
    onopen?: (itemId: string) => void;
    /** The entry open beside the list, so its row is announced as current. */
    currentId?: string;
    /** Where a link the list cannot follow itself leads: the entry's page, a copy made from it. */
    onnavigate?: (path: string) => void;
    /** Whether the reader may share an entry here (`MANAGE_MEMBERS` along the path, D5). */
    mayShare?: boolean;
    /**
     * Why the reader may not change this entry, in their words, or nothing where they may - from
     * their role along the path (UC-WRK-22 checks 1, 2). The view decides, because the role is the
     * view's to know; a row it answers for has no control that writes it, only its completion mark
     * with the reason.
     */
    changeRefusal: (item: WorkItem) => string | undefined;
    /** Whether the reader may create entries here: the add controls are drawn only then. */
    mayCreate: boolean;
    /**
     * Whether rows can be picked for the bulk bar. Only where the screen draws that bar: a list that
     * drew checkboxes nothing acts on would be asking the reader to select for nothing.
     */
    isSelectable?: boolean;
    /**
     * The collection's structure in force, as its record carries it (domain-model.md §2): what the
     * capture line recognises - several lines, a leading amount - and which types carry an
     * amount. Absent from a server that does not answer one, and then nothing is recognised.
     */
    structure?: EffectiveStructure;
    /**
     * Whether the reader may change the collection's shape (`STRUCTURE`): a simple list then
     * offers to rename its *Done* section and to choose its categories where it is used. Absent
     * for everybody else (P-05).
     */
    mayShape?: boolean;
    /** The collection's version, which a change of its structure is made against. */
    containerVersion?: number;
  }

  const {
    collectionId,
    root,
    isReadOnly = false,
    readOnlyReason,
    readOnlyNoticeId,
    query,
    isExpanded = false,
    lastResults,
    onopen,
    currentId,
    onnavigate,
    mayShare = false,
    changeRefusal,
    mayCreate,
    isSelectable = false,
    structure,
    mayShape = false,
    containerVersion,
  }: Props = $props();

  /**
   * The simple arrangement (UC-WRK-25, record D6): a collection's own list whose levels in force
   * are one. Its rows sit in sections - open under their categories, *Done*, *Recently* - rather
   * than in one tree, a tap on a row ticks it, a swipe ticks or reopens it, and a still hold opens
   * its menu (ADR-0088). Every other list is drawn as it always was.
   */
  const isSimple = $derived(!root && (structure?.settings?.levels?.length ?? 0) === 1);
  const simpleLevel = $derived(structure?.settings?.levels?.[0]);
  /** What the collection calls its done entries, where somebody named them (UC-WRK-25 check 3). */
  const doneName = $derived(simpleLevel?.done_name ?? undefined);
  /** The labels the collection groups by, in their order (UC-WRK-25 check 5). */
  const categories = $derived(
    simpleLevel ? (structure?.settings?.fields?.[simpleLevel.type]?.categories ?? []) : [],
  );

  /** Whether the shape controls are drawn: the right, an active collection, a version to write against. */
  const shapes = $derived(isSimple && mayShape && !isReadOnly && containerVersion !== undefined);
  let isChoosingCategories = $state(false);
  let renamingDone = $state(false);
  let draftDoneName = $state('');

  /**
   * Names the *Done* section (UC-WRK-25 check 3): the `levels` setting is written whole, the unit
   * a structure stores, with this level's done name changed and every other part as it is.
   */
  async function saveDoneName() {
    if (!structure?.settings?.levels || !simpleLevel || containerVersion === undefined) return;
    writeFailure = undefined;
    try {
      const name = draftDoneName.trim();
      const levels = structure.settings.levels.map((level) =>
        level.type === simpleLevel?.type ? { ...level, done_name: name === '' ? null : name } : level,
      );
      await containers.setStructure(collectionId, { settings: { levels } }, containerVersion);
      announcer.say(t('app.entries.done_name_saved_announced'));
      renamingDone = false;
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /** The clock the sections are drawn at, moved on when the oldest entry under *Done* leaves it. */
  let now = $state(Date.now());

  provideReasonGroup(() => ({ id: readOnlyNoticeId ?? '', reason: isReadOnly && readOnlyNoticeId ? inheritedReason() : undefined }));

  /** The types the top level of this list may hold: the collection's roots, or the root's children. */
  const topTypes = () => (root ? childTypes(root.type) : rootTypes());

  /** Where this device keeps which levels of a subtree are open (design-system.md §11.10). */
  const expansionKey = $derived(root ? `hubtask.subtree.${root.id}` : undefined);

  function rememberedExpansion(): string[] | undefined {
    if (!expansionKey) return undefined;
    try {
      const kept = sessionStorage.getItem(expansionKey);
      return kept ? (JSON.parse(kept) as string[]) : undefined;
    } catch {
      return undefined;
    }
  }

  /** The sentence one row shows about the last bulk, or nothing where it was not in it. */
  function bulkNote(itemId: string): string | undefined {
    const result = lastResults?.get(itemId);
    if (!result) return undefined;
    if (outcomeOf(result) === 'applied') return undefined;
    // The server's own sentence wherever it sent one — including for a rollback, whose problem
    // names the operation that caused it. The client's own is the fallback for the shape the
    // schema describes, which carries no problem at all.
    return result.problem
      ? renderProblem(result.problem as never, messages).message
      : t('app.bulk.rolled_back');
  }

  /** The entries whose children are shown. Expanding one is what reads its level. */
  let expanded = $state<string[]>(rememberedExpansion() ?? []);
  /** Whether the subtree's default - direct children open - has been applied once they arrived. */
  let hasDefaultExpansion = rememberedExpansion() !== undefined;

  // `untrack`, because the listener writes the store and writing it reads it,
  // so an effect that subscribes while tracking that read cancels itself before the answer lands.
  // A subtree is read whole - every level under the root in one query, unfiltered as every child
  // level is: a level per branch would be a request per branch, twice after every write.
  $effect(() => {
    const wanted = collectionId;
    const asked = query;
    const under = root?.id;
    return untrack(() => (under ? items.openSubtree(under) : items.openCollection(wanted, asked)));
  });

  // In a subtree, the direct children that take children start open and deeper levels closed
  // (design-system.md §11.10) - applied once, when the first level has arrived, unless this device remembers
  // a choice. Kept whenever it changes, and only for a subtree.
  $effect(() => {
    if (!root || hasDefaultExpansion) return;
    const first = items.childrenOf(root.id);
    if (first.length === 0) return;
    hasDefaultExpansion = true;
    expanded = first.filter((item) => childTypes(item.type).length > 0).map((item) => item.id);
  });
  $effect(() => {
    // Read first, so the effect follows `expanded` from its first run and not only once the
    // default has been applied - an early return before the read would track nothing.
    const kept = JSON.stringify(expanded);
    if (!expansionKey || !hasDefaultExpansion) return;
    try {
      sessionStorage.setItem(expansionKey, kept);
    } catch {
      // A browser that refuses storage keeps the choice for as long as the page is open.
    }
  });

  /** Opens or closes every level of the subtree: the section head's one control over all of them. */
  export function expandAll(isOpen: boolean) {
    hasDefaultExpansion = true;
    expanded = isOpen ? rows.filter((row) => row.takesChildren).map((row) => row.item.id) : [];
  }

  // What `LIST_EXPANDED` means, one row at a time: a row that takes children is opened, and
  // opening it is what reads its level — so the rows that arrive are opened in turn rather than a
  // whole subtree being fetched at once.
  $effect(() => {
    if (!isExpanded) return;
    const opened = rows
      .filter((row) => row.takesChildren && !expanded.includes(row.item.id))
      .map((row) => row.item.id);
    if (opened.length > 0) expanded = [...expanded, ...opened];
  });



  // The collection's labels, read once for the whole list: every row picks from the same set,
  // because a label belongs to a collection (I-W3).
  $effect(() => {
    const wanted = collectionId;
    return untrack(() => labels.open(wanted));
  });

  const available = $derived(
    labels.of(collectionId).map((entry) => ({
      id: entry.id,
      name: entry.name,
      colorToken: entry.color_token,
      description: entry.description,
    })),
  );

  async function toggleLabel(item: WorkItem, labelId: string) {
    writeFailure = undefined;
    try {
      const isOn = (item.label_ids ?? []).includes(labelId);
      await labels.setOnItem(item.id, labelId, !isOn, crypto.randomUUID());
      // Taking one off is at once, with the way back (UC-WRK-28 check 3); putting one on is its own
      // undo.
      if (isOn) offerUndo({ kind: 'label-removed', itemId: item.id, labelId, title: item.title });
      else announcer.say(t('app.labels.toggled_announced'));
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  interface Row {
    readonly item: WorkItem;
    readonly depth: number;
    readonly takesChildren: boolean;
    /**
     * The level this row is ranked within, and where it sits in it.
     *
     * Carried on the row rather than looked up when a command runs, because the flattened list is
     * not the level: a row at depth 2 is ranked among its parent's children, and the rows above it
     * on screen belong to other levels. `:reorder` moves an entry within its own level and nowhere
     * else, so the level is what a command has to be answered against.
     */
    readonly siblings: readonly WorkItem[];
    readonly index: number;
    /** The entry this one sits inside, or `null` for the top level of the collection. */
    readonly parentId: string | null;
    /**
     * Where this row stands in the archive — its own, or inherited from what holds it.
     *
     * Three values rather than a boolean, because they are three different offers: an archived row
     * has an unarchive control here, a row under an archived one has that control on the thing
     * above it, and an active row has neither (I-C3, I-W4).
     */
    readonly archival: Archival;
    /** In the simple arrangement, the section the row is drawn in; ranking stays within it. */
    readonly section?: 'open' | 'done' | 'recently';
  }

  /**
   * Whether a row may be written to at all. Archived is read-only, whichever way it got there; and
   * so is a row the reader's role may not change.
   */
  const isFrozen = (row: Row) => row.archival !== 'active' || changeRefusal(row.item) !== undefined;

  /**
   * Whether the reader's role keeps them from changing this row: then its changing controls are
   * absent rather than off (P-05), and only the completion mark stays, with the reason.
   */
  const isRoleRefused = (row: Row) => changeRefusal(row.item) !== undefined;

  /** Why it may not, in the reader's words. There is no `disabled` boolean anywhere. */
  function frozenReason(row: Row): string | undefined {
    // A read-only list is one area with one reason, the page's notice: it outlasts any row's own
    // archive or the reader's role, since nothing in it can be changed until it is lifted (I-C3).
    if (isReadOnly) return inheritedReason();
    const refusal = changeRefusal(row.item);
    if (refusal) return refusal;
    if (row.archival === 'archived') return t('app.entries.archived');
    return undefined;
  }

  /** Why a row of a read-only list cannot be changed: the page's notice, said in its words. */
  function inheritedReason(): string {
    return readOnlyReason ?? t('app.entries.read_only');
  }

  /** The visible rows, in the order the eye walks them. */
  function flatten(
    parents: readonly WorkItem[],
    depth: number,
    parentId: string | null,
    isUnderArchive: boolean,
  ): Row[] {
    const rows: Row[] = [];
    parents.forEach((item, index) => {
      const archival = archivalOfItem(item, isUnderArchive);
      // Whether it *may* hold children is the manifest's answer; whether it *does* is not known
      // until it is opened, which is why the twist appears for any type that takes them.
      rows.push({
        item,
        depth,
        takesChildren: childTypes(item.type).length > 0,
        siblings: parents,
        index,
        parentId,
        archival,
      });
      if (expanded.includes(item.id)) {
        // Only an archived collection reaches the rows under an entry - `isReadOnly` here. An
        // archived entry freezes itself, not its children (lifecycle.ts, `isBelowArchived`).
        rows.push(
          ...flatten(items.childrenOf(item.id), depth + 1, item.id, isBelowArchived(archival, isReadOnly)),
        );
      }
    });
    return rows;
  }

  /** The simple arrangement's sections, from the level the list reads (`simplelist.ts`). */
  const sections = $derived<Sections<WorkItem> | undefined>(
    isSimple
      ? partition(items.inCollection(collectionId), now, categories, new Set(available.map((label) => label.id)))
      : undefined,
  );

  // A timer to the moment the oldest entry under *Done* reaches its 24 hours, so the sections
  // re-sort then rather than at the next read.
  $effect(() => {
    const boundary = sections?.nextBoundary;
    if (boundary === undefined) return;
    const timer = setTimeout(() => (now = Date.now()), Math.max(0, boundary - Date.now()) + 1);
    return () => clearTimeout(timer);
  });

  /** The title the capture line asks for, which narrows *Recently* (UC-WRK-25 check 4). */
  const askedTitle = $derived.by(() =>
    addingUnder === 'root' ? parseLine(draftTitle.split(/\r?\n/u)[0] ?? '', captureRules).title : '',
  );
  const recentlyShown = $derived(sections ? narrow(sections.recently, askedTitle) : []);

  /** The rows of one section: ranked among their own group, so a drag and a rank stay in it. */
  function sectionRows(entries: readonly WorkItem[], section: 'open' | 'done' | 'recently'): Row[] {
    return entries.map((item, index) => ({
      item,
      depth: 0,
      takesChildren: false,
      siblings: entries,
      index,
      parentId: null,
      archival: archivalOfItem(item, isReadOnly),
      section,
    }));
  }

  const rows = $derived(
    sections
      ? [
          ...sections.open.flatMap((group) => sectionRows(group.entries, 'open')),
          ...sectionRows(sections.done, 'done'),
          ...sectionRows(recentlyShown, 'recently'),
        ]
      : flatten(root ? items.childrenOf(root.id) : items.inCollection(collectionId), 0, root?.id ?? null, isReadOnly),
  );
  const rowById = $derived(new Map(rows.map((row) => [row.item.id, row])));

  /** A category's heading: its label's name, or the words for the entries that carry none. */
  function categoryName(category: string | null): string {
    if (category === null) return t('app.entries.no_category');
    return available.find((label) => label.id === category)?.name ?? t('app.entries.no_category');
  }

  /** The row whose amount is being changed from its menu. */
  let amending = $state<Row | undefined>(undefined);
  let draftAmount = $state('');
  /** The row whose category is being chosen from its menu. */
  let categorising = $state<Row | undefined>(undefined);

  // In the collection's list only an open row's level is read; a subtree arrived whole, so a
  // closed row there already knows what it hides and nothing more is read.
  const readIds = $derived(root ? [] : expanded);
  // Keyed by the ids as one string: reading a level writes the store, the store rebuilds the rows,
  // and a fresh array of the same ids would re-run this effect and re-read the level - forever.
  const readKey = $derived(readIds.join(' '));
  $effect(() => {
    const open = readKey === '' ? [] : readKey.split(' ');
    return untrack(() => {
      const stops = open.map((id) => items.openChildren(id));
      return () => {
        for (const stop of stops) stop();
      };
    });
  });

  /** What a row hides or shows: its direct children's completion, once they have been read. */
  function progressOf(itemId: string): { done: number; total: number } | undefined {
    if (!items.hasChildrenOf(itemId)) return undefined;
    const children = items.childrenOf(itemId);
    return { done: children.filter((child) => child.completion?.is_completed).length, total: children.length };
  }

  /** The moment standing, and which row of this level carries it. */
  const moment = $derived(celebration.current);
  const slotRowId = $derived.by(() => {
    if (!moment) return undefined;
    if (moment.tier === 2 && moment.reason === 'parent' && moment.item.parent_id
      && rows.some((row) => row.item.id === moment.item.parent_id)) {
      return moment.item.parent_id;
    }
    return moment.item.id;
  });
  // Leaving the screen lets the moment go: a slot nobody is looking at is not a moment.
  $effect(() => () => celebration.dismiss());

  /**
   * The entries on screen, in the order they are drawn.
   *
   * What a range is measured through and what "select every entry on screen" means. Rows that have
   * left — filtered away, moved, trashed by somebody else — are dropped from the selection here,
   * because a bar acting on an entry nobody can see is a bar acting in the dark.
   */
  const visibleIds = $derived(rows.map((row) => row.item.id));

  $effect(() => {
    selection.keepVisible(visibleIds);
  });
  // Not called `state`: a variable of that name collides with the `$state` rune in what the
  // compiler generates, and the error it produces names a line that looks unrelated.
  const levelState = $derived(items.stateOf(root ? `subtree:${root.id}` : `container:${collectionId}`));
  const failure = $derived(
    levelState?.status === 'failed' ? renderProblem(levelState.error, messages) : undefined,
  );

  /** Which parent the add form is open under: an entry's id, `'root'`, or nothing. */
  let addingUnder = $state<string | null>(null);
  let draftTitle = $state('');
  let draftType = $state('');
  /** Whether the control that opened the form already named the kind, so the form asks no more. */
  let isTypeNamed = $state(false);
  let isSaving = $state(false);
  let writeFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  /** Whether the collection's fields in force give this type an amount. */
  const offersAmount = (type: string) =>
    (structure?.settings?.fields?.[type]?.capabilities ?? []).includes('AMOUNT');

  /**
   * What the capture line recognises (UC-WRK-25 checks 1, 2): the one rule every capture field reads
   * a collection by (`captureRulesOf`). Lines split only at the top of the collection: a line added
   * inside an entry is that entry's one child.
   */
  const captureRules = $derived.by((): CaptureRules => {
    const rules = captureRulesOf(structure, draftType, unitsOf(t('app.entries.capture_units')));
    return { ...rules, splitsLines: rules.splitsLines && !root && addingUnder === 'root' };
  });

  /** The person took the amount back into the title for this submit, from the chip. */
  let keepsWhole = $state(false);
  /** What the line reads its first line as, shown before submitting so a split can be undone. */
  const preview = $derived(
    captureRules.readsAmount && !keepsWhole ? parseLine(draftTitle.split(/\r?\n/u)[0] ?? '', captureRules) : undefined,
  );
  /** What the last submit left in the field, and why. */
  let captureNote = $state<string | undefined>(undefined);

  const offeredTypes = $derived(
    addingUnder === null
      ? []
      : addingUnder === 'root'
        ? topTypes()
        : childTypes(rows.find((row) => row.item.id === addingUnder)?.item.type ?? ''),
  );

  /**
   * What the control at the end of the top level says: "Add an entry" in a collection; in a
   * subtree the name of what it adds, in the catalogue's words for the manifest's type, because
   * "Add a work package" under a task says which level the reader is adding to.
   */
  const addLabel = $derived(
    root && topTypes().length === 1 ? t('app.entries.add_typed', { type: topTypes()[0] ?? '' }) : t('app.entries.add'),
  );

  /** The language a new entry is written in: the person's own, preselected, and changeable. */
  let draftLanguage = $state('');
  const languages = $derived(textLanguages(manifest.value));

  /**
   * The page head's primary action: opens the form at the list's end and focuses into it,
   * exactly as the button at the end does. Exported so the view can ask without owning the
   * form's state, which is the list's.
   */
  export function addEntry() {
    if (isReadOnly || !mayCreate || topTypes().length === 0) return;
    startAdding('root');
  }

  function startAdding(under: string, type?: string) {
    addingUnder = under;
    draftTitle = '';
    keepsWhole = false;
    captureNote = undefined;
    draftLanguage = actor.locale ?? messages.locale;
    writeFailure = undefined;
    isTypeNamed = type !== undefined;
    // The type the control named, or the first the manifest offers, so the common case needs no
    // choice at all.
    draftType =
      type ??
      (under === 'root'
        ? topTypes()
        : childTypes(rows.find((row) => row.item.id === under)?.item.type ?? ''))[0] ??
      '';
  }

  /**
   * The controls at the end of the top level: one "Add an entry" in a collection, and in a subtree
   * one per type the entry may hold - "Add a work package" and "Add an activity" under a task - so
   * that the reader names the level by the control rather than by a chooser in the form.
   */
  const addControls = $derived(
    root && topTypes().length > 1
      ? topTypes().map((type) => ({ type, label: t('app.entries.add_typed', { type }) }))
      : [{ type: undefined, label: addLabel }],
  );

  /** A line as it is put back into the field when it was not added: amount first, as typed. */
  const asTyped = (line: CapturedLine) => (line.amount ? `${line.amount} ${line.title}` : line.title);

  async function create() {
    if (draftTitle.trim() === '' || draftType === '' || addingUnder === null) return;
    const under = addingUnder;
    const { lines, rest } = capture(draftTitle, { ...captureRules, readsAmount: captureRules.readsAmount && !keepsWhole });
    if (lines.length === 0) return;
    isSaving = true;
    writeFailure = undefined;
    captureNote = undefined;
    // The language travels only when the person chose one: omitted is the contract's own
    // default - the creator's locale - and sending the same thing would say it twice.
    const language = draftLanguage.trim() ? { content_language: draftLanguage.trim() } : {};
    const placement =
      under === 'root'
        ? root
          ? { parent_id: root.id }
          : { collection_id: collectionId }
        : { parent_id: under };
    // One entry per line, in the order typed, each its own request: a line the server refuses is
    // that line's, and the others are added rather than held back by it. Created lines leave the
    // field; refused ones stay in it with the reason, and so do the lines past the limit.
    const refused: CapturedLine[] = [];
    let created = 0;
    for (const line of lines) {
      try {
        await items.create(
          { type: draftType, ...placement, title: line.title, ...(line.amount ? { amount: line.amount } : {}), ...language },
          crypto.randomUUID(),
        );
        created += 1;
      } catch (error) {
        refused.push(line);
        writeFailure ??= renderProblem(error as never, messages);
      }
    }
    isSaving = false;
    keepsWhole = false;
    // The new entries appear because the write invalidated the level and the engine re-read it.
    // Nothing is inserted into the list by hand here.
    //
    // Expanding the parent is not decoration: a child created under a collapsed row is a row
    // nobody can see, so the reader would press Create and watch nothing happen. It opens the
    // level it was just added to, which is also what fetches it.
    if (created > 0 && under !== 'root' && !expanded.includes(under)) {
      expanded = [...expanded, under];
    }
    if (created === 1 && lines.length === 1) {
      announcer.say(t('app.entries.created_announced', { title: lines[0]?.title ?? '' }));
    } else if (created > 0) {
      announcer.say(t('app.entries.created_lines_announced', { count: created }));
    }
    draftTitle = [...refused.map(asTyped), rest].filter((each) => each !== '').join('\n');
    if (rest !== '') {
      captureNote = t('app.entries.capture_rest', { count: rest.split('\n').length });
    }
    // A capture line stays open for the next thing to add; a form for one entry closes.
    if (draftTitle === '' && !captureRules.splitsLines) addingUnder = null;
  }

  /** Enter adds and Shift+Enter starts a line; nothing is added while an input method composes. */
  function onCaptureKey(event: KeyboardEvent) {
    if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
    event.preventDefault();
    void create();
  }

  /**
   * Ranks a row within its level — the whole command path, in one function.
   *
   * **Built first, and the pointer path is measured against it.** WCAG 2.2 SC 2.5.7 wants a
   * single-pointer alternative to every dragging movement, and a rank change is a command before
   * it is a gesture: a menu item and a key press are this call, and so is a drag. Built the other
   * way round it would be a retrofit, and the retrofit is what an audit finds.
   *
   * The `Idempotency-Key` is minted **per intent**. Pressing "move down" twice is two intents and
   * moves the entry twice, which is what the reader asked for; one press delivered twice — a proxy
   * retry, a flaky connection — is one key and the server replays its own answer rather than
   * moving anything a second time.
   *
   * Nothing is inserted into the list by hand. The write invalidates `/items` and the level is
   * read again, so the order on screen is the server's order rather than this component's guess at
   * it — which is also what makes a second client's move appear rather than being overwritten.
   */
  async function rank(row: Row, command: RankCommand) {
    const target = rankTarget(command, row.index, row.siblings.length);
    if (target === null) return;
    await rankTo(row, target);
  }

  /** The call itself, once something — a command, a key or a drag — has named the position. */
  async function rankTo(row: Row, target: number) {
    writeFailure = undefined;
    try {
      await items.reorder(
        row.item.id,
        anchorFor(row.siblings, row.item.id, target),
        row.item.version,
        crypto.randomUUID(),
      );
      // Said out loud, because a rank change is invisible to a screen reader: the focused control
      // did not change and neither did its name, and the list quietly rearranged itself.
      announcer.say(
        t('app.rank.announced', {
          title: row.item.title,
          position: target + 1,
          count: row.siblings.length,
        }),
      );
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /**
   * The keyboard shortcuts, on the level rather than on a row.
   *
   * Listened for rather than bound in the markup: a `<div>` with an `onkeydown` is a static element
   * with an interaction, which Svelte warns about and is right to — the handler here belongs to the
   * *list*, and the events reach it from the row controls that are properly focusable. Attaching it
   * in an effect also means it is removed when the list is, with nothing for a caller to forget.
   */
  let level = $state<HTMLElement | null>(null);

  $effect(() => {
    const node = level;
    if (!node) return;

    const onKeydown = (event: KeyboardEvent) => {
      const id = (event.target as Element | null)?.closest?.('[data-row]')?.getAttribute('data-row');
      const row = rows.find((each) => each.item.id === id);
      if (!row || isFrozen(row)) return;

      const target = rankIntent(event, row.index, row.siblings.length);
      if (target === null) return;
      // Alt+Down scrolls in some engines. The command is answered, so the scroll is not.
      event.preventDefault();
      void rank(row, keyCommand(event));
    };

    node.addEventListener('keydown', onKeydown);
    return () => node.removeEventListener('keydown', onKeydown);
  });

  /** Which command a press was, once `rankIntent` has said it is one at all. */
  function keyCommand(event: KeyboardEvent): RankCommand {
    if (event.key === 'ArrowUp') return event.shiftKey ? 'top' : 'up';
    return event.shiftKey ? 'bottom' : 'down';
  }

  /**
   * The entry this row would move inside: the sibling directly above it.
   *
   * "Another parent" is a position the reader can see, so it is a command rather than a picker —
   * the sibling above is the one destination that needs no list of everything. Whether the move is
   * permitted is the **manifest's** answer and not this component's: a work package holds
   * activities on one installation and something else on the next, and `domain-model.md` §2's
   * extension example is only true if nothing here spells the three names out.
   */
  const previousSibling = (row: Row): WorkItem | undefined => row.siblings[row.index - 1];

  /** Why an entry cannot move inside the one above it, or nothing. */
  function insideReason(row: Row): string | undefined {
    const frozen = frozenReason(row);
    if (frozen) return frozen;
    const target = previousSibling(row);
    if (!target) return t('app.move.nothing_above');
    const verdict = acceptsChild(target.type, row.item.type, row.depth);
    if (verdict.status === 'permitted') return undefined;
    // The server's own code where there is one, so a prediction and a refusal read the same way.
    return verdict.status === 'refused' ? t(verdict.code, verdict.params) : t('app.move.moving');
  }

  /** The entry a row moves out to: its parent's parent, or `null` for the collection's top level. */
  function grandParentOf(row: Row): string | null {
    const parent = rows.find((each) => each.item.id === row.parentId);
    // In a subtree the top level's parent is the root, which is not a row: its parent is the root's.
    return parent ? parent.parentId : root && row.parentId === root.id ? root.parent_id ?? null : null;
  }

  /**
   * Why a row cannot move out one level, or nothing.
   *
   * The server answers a move with the placement rules (`Place`): an activity may sit in a task or
   * a work package and never at the top of a collection, so "out" from a task is a refusal the
   * screen can know beforehand (P-05). The destination's stored depth goes to the verdict; where
   * the entry above is not loaded here, the move is offered and the server answers.
   */
  function outReason(row: Row): string | undefined {
    const frozen = frozenReason(row);
    if (frozen) return frozen;
    if (!row.parentId) return t('app.move.already_top');
    const grandParentId = grandParentOf(row);
    const above =
      grandParentId === null
        ? null
        : (rows.find((each) => each.item.id === grandParentId)?.item ??
          (root?.id === grandParentId ? root : items.find(grandParentId)));
    const verdict = placementOf(
      row.item.type,
      above === null ? null : above?.depth === undefined ? undefined : { type: above.type, depth: above.depth },
    );
    return verdict.status === 'refused' ? t(verdict.code, verdict.params) : undefined;
  }

  /**
   * The row's menu: the one every surface draws (UC-WRK-28 check 1), with this level's reasons. A
   * simple list's adds its amount and its category, which its still hold opens with *Select* first
   * (ADR-0088).
   */
  function menuItems(row: Row) {
    return entryMenu(
      {
        surface: root ? 'subtree' : isSimple ? 'simple' : isSelectable ? 'list' : 'subtree',
        item: {
          title: row.item.title,
          isCompleted: row.item.completion?.is_completed ?? false,
          hasDue: Boolean(row.item.due_at),
        },
        archival: row.archival,
        isCollectionReadOnly: isReadOnly,
        mayChange: !isRoleRefused(row),
        mayCreate,
        mayShare,
        carries: (capability) => supports(row.item.type, capability).status === 'permitted',
        level: {
          index: row.index,
          count: row.siblings.length,
          aboveTitle: previousSibling(row)?.title,
          insideReason: insideReason(row),
          outReason: outReason(row),
        },
        simple: {
          amount: offersAmount(row.item.type),
          category: categories.length > 0 && supports(row.item.type, 'LABELS').status === 'permitted',
        },
      },
      t,
    );
  }

  /** The menus of the rows, so a right click, a key or a long press can open one. */
  const menus = $state<Record<string, EntryMenu | undefined>>({});
  let actions = $state<EntryActions | undefined>(undefined);

  /**
   * The pointer path, and it reaches the same call.
   *
   * A drag names a **position**, which is the number the menu items already name, so the two
   * cannot come to disagree about where the fourth row lands — and a drag that ends where it
   * began writes nothing, which is what makes thinking better of it free.
   *
   * The level is the row's own, and a drag stays in it: `:reorder` is a rank within a level, and a
   * gesture that reparented an entry by drifting over another level's rows would be performing
   * `:move` without the reader asking for it. Changing the parent is the menu's, where it is a
   * decision rather than a slip.
   */
  const rowIdAt = (target: EventTarget | null) =>
    (target as Element | null)?.closest?.('[data-row]')?.getAttribute('data-row') ?? undefined;

  /**
   * A right click, the context-menu key and a long press open the row's menu (ADR-0088): a still
   * hold opens it on release, *Select* first; a hold on the grip that moves is the drag's.
   */
  $effect(() => {
    const node = level;
    // The simple arrangement reads a finger its own way, in the effect below.
    if (!node || isSimple) return;
    return attachEntryGestures(node, {
      idAt: rowIdAt,
      open: (id) => menus[id]?.open(0),
      ignore: '[data-grip]',
    });
  });

  /**
   * `Ctrl`/`⌘`-click picks a row (UC-WRK-28 check 4), where the screen has a bar to act on it. On the
   * level itself, for the reason the key handler beside it is there: a `<div>` carrying an
   * interaction is a static element with one, and Svelte is right to warn.
   */
  $effect(() => {
    const node = level;
    if (!node || !isSelectable) return;
    // The capture phase, because the row is a link and the router listens for the click after it.
    const onClick = (event: MouseEvent) => {
      if (!(event.metaKey || event.ctrlKey)) return;
      const id = rowIdAt(event.target);
      if (!id) return;
      event.preventDefault();
      event.stopPropagation();
      selection.pick(visibleIds, id);
    };
    node.addEventListener('click', onClick, true);
    return () => node.removeEventListener('click', onClick, true);
  });

  /**
   * A simple list's row under a finger and a pointer (UC-WRK-25 check 6, ADR-0088): a tap anywhere
   * on the row but its controls ticks it, so the target is the whole row and never only the small
   * box; a swipe towards the inline end ticks an open entry and reopens a done one; a still hold
   * opens the entry menu when the finger lifts, *Select* first, and ticks nothing. The grip still
   * drags, and `Ctrl`/`Cmd`-click still selects. A tap on the checkbox itself is the checkbox's,
   * and the row lets it be, so nothing is ticked twice.
   */
  $effect(() => {
    const node = level;
    if (!node || !isSimple) return;
    let press: { id: string; start: Point; acted: boolean } | undefined;
    let swallowsClick = false;
    const idAt = (target: EventTarget | null) =>
      (target as Element | null)?.closest?.('[data-row]')?.getAttribute('data-row') ?? undefined;
    const isRtl = () => getComputedStyle(node).direction === 'rtl';
    const onDown = (event: PointerEvent) => {
      swallowsClick = false;
      press = undefined;
      if (event.pointerType === 'mouse' || selection.isOn) return;
      const id = idAt(event.target);
      if (!id || (event.target as Element).closest('[data-grip]')) return;
      // The screen edge's band is the platform's back gesture.
      if (startsAtEdge(event.clientX, window.innerWidth, edgeInset(node))) return;
      press = { id, start: { x: event.clientX, y: event.clientY, at: event.timeStamp }, acted: false };
    };
    const onMove = (event: PointerEvent) => {
      if (!press || press.acted) return;
      const gesture = classify(
        press.start, { x: event.clientX, y: event.clientY, at: event.timeStamp }, swipeThreshold(node), isRtl(),
      );
      if (gesture === 'swipe-end') {
        press.acted = true;
        swallowsClick = true;
        const row = rowById.get(press.id);
        if (row && !isFrozen(row)) void toggleComplete(row.item);
      } else if (gesture !== 'pending' && gesture !== 'hold') {
        press = undefined;
      }
    };
    const onUp = (event: PointerEvent) => {
      if (press && !press.acted && event.timeStamp - press.start.at >= GESTURE_HOLD_MS) {
        menus[press.id]?.open(0);
        swallowsClick = true;
      }
      press = undefined;
    };
    const onCancel = () => (press = undefined);
    // A right click and the context-menu key open the same menu, as on every surface
    // (`entryGestures.ts`); a finger's long press is the release's, not the browser's menu.
    const onContextMenu = (event: MouseEvent) => {
      const id = idAt(event.target);
      if (!id) return;
      if ((event as PointerEvent).pointerType === 'touch' || press !== undefined) {
        event.preventDefault();
        return;
      }
      if (event.shiftKey) return;
      event.preventDefault();
      menus[id]?.open(0);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'ContextMenu' && !(event.key === 'F10' && event.shiftKey)) return;
      const id = idAt(event.target);
      if (!id) return;
      event.preventDefault();
      menus[id]?.open(0);
    };
    // The capture phase, so that a press a gesture already answered reaches no control.
    const onClick = (event: MouseEvent) => {
      const id = idAt(event.target);
      if (!id) return;
      if (swallowsClick) {
        swallowsClick = false;
        event.preventDefault();
        event.stopPropagation();
        return;
      }
      if (event.metaKey || event.ctrlKey) {
        if (selection.isOn) return;
        event.preventDefault();
        event.stopPropagation();
        selection.pick(visibleIds, id);
        return;
      }
      if (selection.isOn) return;
      if ((event.target as Element).closest('button, a, input, textarea, select, label, summary, [role="menu"], [role="menuitem"]')) return;
      const row = rowById.get(id);
      if (row && !isFrozen(row)) void toggleComplete(row.item);
    };
    node.addEventListener('pointerdown', onDown);
    node.addEventListener('pointermove', onMove);
    node.addEventListener('pointerup', onUp);
    node.addEventListener('pointercancel', onCancel);
    node.addEventListener('click', onClick, true);
    node.addEventListener('contextmenu', onContextMenu);
    node.addEventListener('keydown', onKey);
    return () => {
      node.removeEventListener('pointerdown', onDown);
      node.removeEventListener('pointermove', onMove);
      node.removeEventListener('pointerup', onUp);
      node.removeEventListener('pointercancel', onCancel);
      node.removeEventListener('click', onClick, true);
      node.removeEventListener('contextmenu', onContextMenu);
      node.removeEventListener('keydown', onKey);
    };
  });

  /** Writes the amount from the row menu's dialog: empty clears it. */
  async function saveAmount() {
    const row = amending;
    if (!row) return;
    writeFailure = undefined;
    try {
      await items.update(row.item.id, { amount: draftAmount.trim() }, row.item.version);
      announcer.say(t('app.entries.amount_saved_announced', { title: row.item.title }));
      amending = undefined;
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /**
   * Puts an entry under one category: the chosen label on, every other category label off, other
   * labels left alone - an entry sits under one aisle, and its other labels are labels.
   */
  async function chooseCategory(row: Row, category: string | null) {
    writeFailure = undefined;
    try {
      const carried = row.item.label_ids ?? [];
      for (const each of categories) {
        const wanted = each === category;
        if (wanted !== carried.includes(each)) {
          await labels.setOnItem(row.item.id, each, wanted, crypto.randomUUID());
        }
      }
      announcer.say(t('app.labels.toggled_announced'));
      categorising = undefined;
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  // A simple list opens with its capture line ready at the top (ux-inventory E5).
  $effect(() => {
    if (isSimple && !isReadOnly && mayCreate && addingUnder === null && topTypes().length > 0) {
      untrack(() => startAdding('root'));
    }
  });

  const drag = createDrag({
    start: (grip) => {
      const id = grip.closest('[data-row]')?.getAttribute('data-row');
      const row = rows.find((each) => each.item.id === id);
      if (!row || isFrozen(row) || !level) return null;
      return {
        id: row.item.id,
        index: row.index,
        level: {
          key: row.parentId ?? 'root',
          // The level's rows in drawn order — the siblings, not everything on screen. A row at
          // depth 2 is ranked among its parent's children, and the rows above it belong to others.
          elements: row.siblings
            .map((sibling) => level?.querySelector<HTMLElement>(`[data-row="${sibling.id}"]`))
            .filter((element): element is HTMLElement => element !== null),
        },
      };
    },
    ondrop: ({ id, to }) => {
      const row = rows.find((each) => each.item.id === id);
      if (!row) return;
      void rankTo(row, to);
    },
    // A finger held still on the grip and lifted asks for the menu, as anywhere else on the row.
    onhold: (id) => menus[id]?.open(0),
  });

  $effect(() => {
    const node = level;
    return node ? drag.attach(node) : undefined;
  });

  /** One chosen item: what only the list knows answers here, the rest is the screen's actions. */
  function chose(row: Row, id: string) {
    if (id === 'select') {
      selection.pick(visibleIds, row.item.id);
      return;
    }
    if (id === 'open' && !onopen) {
      onnavigate?.(`/items/${row.item.id}`);
      return;
    }
    // A simple list's own items: its amount and its category.
    if (id === 'amount') {
      amending = row;
      draftAmount = row.item.amount ?? '';
      return;
    }
    if (id === 'category') {
      categorising = row;
      return;
    }
    void actions?.run(row.item, id, row.siblings);
  }

  async function toggleComplete(item: WorkItem) {
    writeFailure = undefined;
    try {
      // Re-read rather than predicted: with `completionPolicy = ROLLUP` a parent completes when its
      // children do (I-W5), and that is a change this client learns by asking.
      const answered = await items.setCompleted(item.id, !item.completion?.is_completed, crypto.randomUUID());
      // The checkbox that changed keeps focus, but its name did not change and a screen reader
      // hears nothing else (4.1.3).
      announcer.say(
        t(item.completion?.is_completed ? 'app.entries.reopened_announced' : 'app.entries.completed_announced', {
          title: item.title,
        }),
      );
      // The moment, if it is one (§7): decided from the copy, mounted at the tier the table says.
      if (answered.completion?.is_completed) void celebration.celebrate(answered);
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }
</script>

<Stack gap="200">
  {#if levelState === undefined || levelState.status === 'loading' || levelState.status === 'idle'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if failure}
    <ErrorState
      title={failure.message}
      reference={failure.reference}
      referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')}
      onRetry={() => (root ? items.openSubtree(root.id) : items.openCollection(collectionId))()}
    />
  {:else}
    <ReplicaMark state={levelState} />
    {#if writeFailure}
      <p class="failure" role="alert">{writeFailure.message}</p>
    {/if}

    <!-- I-W6, on the screen. A label belongs to a collection and a column belongs to a board, so
         an entry carried to another collection leaves both behind — and the operation reports what
         it left rather than dropping it in silence. Showing it is the part that makes the report
         worth having; a client that swallowed it would turn a designed behaviour into data loss. -->
    <EntryActions bind:this={actions} onopen={(id) => (onopen ? onopen(id) : onnavigate?.(`/items/${id}`))} {onnavigate} />

    {#if rows.length === 0}
      <!-- The form belongs here as well as below. §4.1's empty state is the only one that carries a
           call to action, and a call to action that sets a flag nothing renders is a button that
           does nothing — which is what this was until it was pressed. -->
      {#if addingUnder === 'root'}
        {@render addForm()}
      {:else}
        <EmptyState kind="unused" title={t(root ? 'app.item.no_children' : 'app.entries.none')} icon="task">
          {#snippet action()}
            {#if !isReadOnly && mayCreate && topTypes().length > 0}
              {#each addControls as control (control.label)}
                <Button data-opener="add-entry" onclick={() => startAdding('root', control.type)}>{control.label}</Button>
              {/each}
            {/if}
          {/snippet}
        </EmptyState>
      {/if}
    {:else}
      <!-- The level, and the element the shortcuts are listened for on. `data-row` is what a key
           press finds its way back from the focused control to the row by; the handler is added in
           an effect rather than bound here, because a `<div>` with an `onkeydown` is a static
           element with an interaction and Svelte is right to warn about one. -->
      {#if isSimple && addingUnder === 'root'}
        <!-- A simple list's capture line is at its top, where the next thing to add is typed. -->
        {@render addForm()}
      {/if}
      {#if shapes && simpleLevel && supports(simpleLevel.type, 'LABELS').status === 'permitted'}
        <div>
          <Button size="sm" tone="subtle" icon="tag" onclick={() => (isChoosingCategories = true)}>
            {t('app.entries.categories')}
          </Button>
        </div>
      {/if}
      <div class="level" bind:this={level} data-simple={isSimple ? '' : undefined}>
        {#if moment && moment.tier === 3}
          <!-- The rare one takes the level: a slot no taller than the token, at its foot. -->
          <CelebrationSlot current={moment} />
        {/if}
        {#if sections}
          <!-- The simple arrangement (UC-WRK-25): what is still to get, under its categories in
               their order; what was ticked in the last 24 hours, under the collection's name for
               it; and what was ticked before, most used first. -->
          {#each sections.open as group (group.category ?? '')}
            {#if group.category !== null || sections.open.length > 1}
              <h2 class="section-head">{categoryName(group.category)}</h2>
            {/if}
            {#each group.entries as entry (entry.id)}
              {@const row = rowById.get(entry.id)}
              {#if row}{@render entryRow(row)}{/if}
            {/each}
          {/each}
          {#if sections.done.length > 0}
            {#if renamingDone}
              <Inline gap="100">
                <Input label={t('app.entries.done_name_label')} bind:value={draftDoneName} maxlength={60} />
                <Button size="sm" onclick={saveDoneName}>{t('app.entries.save')}</Button>
                <Button size="sm" tone="secondary" onclick={() => (renamingDone = false)}>{t('app.entries.cancel')}</Button>
              </Inline>
            {:else}
              <div class="section-row">
                <h2 class="section-head" data-section="done">{doneName ?? t('app.entries.done_section')}</h2>
                {#if shapes}
                  <IconButton
                    icon="pencil"
                    size="sm"
                    label={t('app.entries.rename_done')}
                    onclick={() => {
                      draftDoneName = doneName ?? '';
                      renamingDone = true;
                    }}
                  />
                {/if}
              </div>
            {/if}
            {#each sections.done as entry (entry.id)}
              {@const row = rowById.get(entry.id)}
              {#if row}{@render entryRow(row)}{/if}
            {/each}
          {/if}
          {#if recentlyShown.length > 0}
            <h2 class="section-head" data-section="recently">{t('app.entries.recently')}</h2>
            {#each recentlyShown as entry (entry.id)}
              {@const row = rowById.get(entry.id)}
              {#if row}{@render entryRow(row)}{/if}
            {/each}
          {/if}
        {:else}
          {#each rows as row (row.item.id)}
            {@render entryRow(row)}
          {/each}
        {/if}
      </div>

      {#if !isReadOnly && mayCreate && topTypes().length > 0}
        {#if isSimple && addingUnder === 'root'}
          <!-- Drawn at the top. -->
        {:else if addingUnder === 'root'}
          {@render addForm()}
        {:else}
          <Inline gap="100">
            {#each addControls as control (control.label)}
              <Button tone="secondary" icon="plus" data-opener="add-entry" onclick={() => startAdding('root', control.type)}>
                {control.label}
              </Button>
            {/each}
          </Inline>
        {/if}
      {/if}
    {/if}
  {/if}
</Stack>

{#snippet entryRow(row: Row)}
    <!-- Two ways into the mode from a row, and they are the two a pointer has. A modifier
         press is the fine pointer's — and it is caught in the capture phase, because the
         row is a link and the router listens for the click after it. A press held is the
         coarse pointer's, which is what a finger has instead of a modifier; the threshold
         is the drag helper's hold, so the two gestures cannot disagree about what a hold is.
         Both go through `selection.pick`, which turns the mode on. -->
    <div
      class="row"
      data-row={row.item.id}
      data-celebrating={moment && moment.tier !== 3 && slotRowId === row.item.id ? '' : undefined}
      data-archived={row.archival !== 'active' ? '' : undefined}
      data-dragging={drag.id === row.item.id ? '' : undefined}
      data-drop={drag.id !== null &&
      drag.id !== row.item.id &&
      drag.levelKey === (row.parentId ?? 'root') &&
      drag.position === row.index
        ? ''
        : undefined}
      style:--drag-offset={drag.id === row.item.id ? drag.offset : undefined}
    >
      <!-- The pick, first in the row and first in the tab order, so a keyboard reaches it
           the way a pointer does — and drawn only while somebody is selecting (ADR-0063
           decision 8). Shift is read from the event rather than from a mode of its own: one
           code path for the pointer and the keyboard is what keeps the two from drifting
           apart, and `selection.pick` is where it lives. -->
      {#if isSelectable && selection.isOn}
      <span class="pick">
        <Checkbox
          label={t('app.bulk.select', { title: row.item.title })}
          isLabelHidden={true}
          checked={selection.has(row.item.id)}
          onclick={(event: MouseEvent) =>
            selection.pick(visibleIds, row.item.id, { range: event.shiftKey })}
          onkeydown={(event: KeyboardEvent) => {
            if (event.key !== ' ' && event.key !== 'Enter') return;
            event.preventDefault();
            selection.pick(visibleIds, row.item.id, { range: event.shiftKey });
          }}
        />
      </span>
      {/if}
      <!-- A picture, not a control. The single-pointer alternative SC 2.5.7 asks for is the
           menu at the end of the row, and it is a real one — so a second focusable element
           that does nothing for the keyboard would be noise in the tab order rather than
           access. -->
      {#if !isRoleRefused(row) && !isReadOnly && (!isSimple || row.section === 'open')}
        <span class="grip" data-grip aria-hidden="true">
          <Icon name="grip-vertical" size="sm" />
        </span>
      {/if}
      {#if moment && moment.tier !== 3 && slotRowId === row.item.id}
        <!-- Tier 1 on the completed row, tier 2 across the parent's - or the completed row
             where the parent is not on this screen. -->
        <CelebrationSlot current={moment} />
      {/if}
      <TaskRow
        type={row.item.type}
        title={row.item.amount ? t('app.entries.amount_title', { amount: row.item.amount, title: row.item.title }) : row.item.title}
        depth={row.depth}
        href={isSimple ? undefined : `/items/${row.item.id}`}
        onOpen={onopen && !isSimple ? () => onopen(row.item.id) : undefined}
        isCurrent={currentId === row.item.id}
        pendingLabel={queue.isPending(row.item.id) ? t('app.sync.pending_entry') : undefined}
        isCompleted={row.item.completion?.is_completed ?? false}
        expansion={!row.takesChildren
          ? 'leaf'
          : expanded.includes(row.item.id)
            ? 'expanded'
            : 'collapsed'}
        completeLabel={t(
          row.item.completion?.is_completed ? 'app.entries.reopen' : 'app.entries.complete',
          { title: row.item.title },
        )}
        completeDisabledReason={frozenReason(row)}
        expandLabel={t(
          expanded.includes(row.item.id) ? 'app.entries.collapse' : 'app.entries.expand',
          { title: row.item.title },
        )}
        onToggleComplete={() => toggleComplete(row.item)}
        onToggleExpand={() =>
          (expanded = expanded.includes(row.item.id)
            ? expanded.filter((id) => id !== row.item.id)
            : [...expanded, row.item.id])}
      >
        {#snippet trailing()}
          <!-- What a row that takes children holds, done of total, once its level has been
               read (ADR-0061 decision 4): a closed branch still says how much is inside. -->
          {#if row.takesChildren}
            {@const progress = progressOf(row.item.id)}
            {#if progress && progress.total > 0}
              <span class="progress" aria-label={t('app.entries.progress', { done: String(progress.done), total: String(progress.total) })}>{progress.done}/{progress.total}</span>
            {/if}
          {/if}
          <!-- Who it belongs to, and who else is on it. Drawn from the identifiers the entry
               already carries: the names come from the accounts cache rather than from an
               expansion this server does not serve. -->
          <PeopleMarks assigneeId={row.item.assignee_id} memberIds={row.item.member_ids ?? []} />
          <!-- When it is wanted, with the word as well as the tone (rule 3). -->
          <DueMark item={row.item} />

          <!-- Rule 3: an entry archived in its own right is not told apart by being dimmer. It
               says the word, so the state reads in greyscale and to a screen reader. In a
               read-only list the page's notice says it once instead, and no row repeats it. -->
          {#if row.archival === 'archived' && !isReadOnly}
            <Badge icon="archive">{t('app.entries.archived_label')}</Badge>
          {/if}

          <!-- The labels this entry carries, and a way to change them — but only for a type
               whose profile has LABELS. The manifest answers that; an ACTIVITY has none, and
               §2 says a field whose capability is off is refused rather than ignored, so it is
               not offered here at all. -->
          {#if supports(row.item.type, 'LABELS').status === 'permitted'}
            {#each (row.item.label_ids ?? []) as labelId (labelId)}
              {@const entry = available.find((each) => each.id === labelId)}
              <!-- In a simple list a category is the heading the entry sits under, not a chip. -->
              {#if entry && !(isSimple && categories.includes(labelId))}
                <LabelChip
                  name={entry.name}
                  colorToken={entry.colorToken}
                  description={entry.description}
                  removeLabel={isFrozen(row)
                    ? undefined
                    : t('app.labels.remove_from', { name: entry.name, title: row.item.title })}
                  onRemove={() => toggleLabel(row.item, labelId)}
                />
              {/if}
            {/each}

            <!-- A simple list keeps its rows to a tick and a title; a category is chosen from the
                 row's menu, other labels on the entry's page. -->
            {#if !isFrozen(row) && !isSimple}
              <Popover label={t('app.labels.on_entry', { title: row.item.title })}>
                {#snippet trigger(props)}
                  <Button size="sm" tone="subtle" icon="tag" {...props}>
                    {t('app.labels.choose')}
                  </Button>
                {/snippet}
                <LabelPicker
                  label={t('app.labels.on_entry', { title: row.item.title })}
                  labels={available}
                  selected={row.item.label_ids ?? []}
                  filterLabel={t('app.labels.filter')}
                  locale={messages.locale}
                  emptyLabel={t('app.labels.none_yet')}
                  noMatchLabel={t('app.labels.no_match')}
                  onToggle={(labelId) => toggleLabel(row.item, labelId)}
                />
              </Popover>
            {/if}
          {/if}

          {#if row.archival === 'active' && mayCreate && row.takesChildren}
            <!-- The label names what it adds. The control at the end of the list adds to the
                 collection and this one adds inside this entry; sharing "Add an entry"
                 between them made a list of seven rows carry eight controls of that name,
                 seven of which do something else. Icon-only, so the row's controls stop
                 competing with its title for width. -->
            <IconButton
              icon="plus"
              label={childTypes(row.item.type).length === 1
                ? t('app.entries.add_typed_inside', { type: childTypes(row.item.type)[0] ?? '', title: row.item.title })
                : t('app.entries.add_child', { title: row.item.title })}
              size="sm"
              onclick={() => startAdding(row.item.id)}
            />
          {/if}

          <!-- The single-pointer alternative SC 2.5.7 asks for, and the path that was built
               first: every position a drag can reach is an item here, each with the reason it
               cannot be used rather than a control that goes grey for no stated cause. -->
          <!-- The entry menu every surface shares (UC-WRK-28 check 1); a simple list's still hold
               opens it, Select first (ADR-0088). -->
          <EntryMenu
            bind:this={menus[row.item.id]}
            title={row.item.title}
            items={menuItems(row)}
            onselect={(id) => chose(row, id)}
          />
        {/snippet}
      </TaskRow>

    </div>

    {#if bulkNote(row.item.id)}
      <p class="bulk-note">{bulkNote(row.item.id)}</p>
    {/if}

    {#if addingUnder === row.item.id}
      {@render addForm()}
    {/if}
{/snippet}

{#snippet addForm()}
  <!-- Opened by a button that stays where it is; the focus still moves in, because the next Tab
       from the button would otherwise skip the form the reader just asked for (2.4.3). -->
  <Stack gap="150" {@attach focusFirst({ returnTo: '[data-opener="add-entry"]' })}>
    <!-- The chooser appears only when there is a choice: at most installations a work package takes
         activities and nothing else, and a select with one option is a decision nobody has. Nor
         when the control that opened the form already named the kind. -->
    <!-- One line or several (UC-WRK-25 check 1): a paste fills it, Enter adds what it holds, and
         Shift+Enter starts a line. A textarea rather than an input, because a single-line input
         drops the line breaks of a pasted list. -->
    <Textarea
      label={captureRules.splitsLines ? t('app.entries.capture_label') : t('app.entries.new_title')}
      hint={captureRules.splitsLines ? t('app.entries.capture_hint') : undefined}
      rows={1}
      bind:value={draftTitle}
      onkeydown={onCaptureKey}
      oninput={() => (captureNote = undefined)}
    />
    <!-- What already looks like it, while it is typed (UC-AI-04 check 7): nothing when nothing is. -->
    <Lookalikes title={draftTitle} />
    {#if preview?.amount}
      <!-- The split the line will make, before it makes it (UC-WRK-25 check 2): pressing the chip
           keeps the number in the title for this submit. -->
      <div>
        <Button size="sm" tone="subtle" icon="x" onclick={() => (keepsWhole = true)}>
          {t('app.entries.capture_amount_chip', { amount: preview.amount })}
        </Button>
      </div>
    {/if}
    {#if captureNote}
      <p class="note" role="status">{captureNote}</p>
    {/if}
    <!-- The kind stays in sight where there is a choice - it decides what the entry is - and a
         select with one option is no choice and is not drawn. -->
    {#if offeredTypes.length > 1 && !isTypeNamed}
      <Select
        label={t('app.entries.type_label')}
        size="sm"
        bind:value={draftType}
        options={offeredTypes.map((type) => ({ value: type, label: t('app.item.type_name', { type }) }))}
      />
    {/if}
    <!-- The language is what a quick capture rarely needs, so it waits under *More*
         (ux-inventory E5). -->
    <details class="more">
      <summary>{t('app.entries.capture_more')}</summary>
      <Stack gap="150">
        <!-- Which language it is written in (i18n-l10n.md §6 line 9): the person's own
             preselected, the installation's list to choose from, a typed tag for one outside it. -->
        <LanguagePicker
          {languages}
          bind:value={draftLanguage}
          size="sm"
          label={t('app.entries.language')}
          otherLabel={t('app.entries.language_other')}
          tagLabel={t('app.entries.language_tag')}
          tagHint={t('app.entries.language_tag_hint')}
        />
      </Stack>
    </details>
    <Inline gap="100">
      <!-- Off with a reason rather than silently doing nothing. `create()` returns early on an
           empty title, and a button that answers a press with nothing is the silent ignoring this
           system has a rule against — there is no `disabled` boolean anywhere for exactly this. -->
      <Button
        size="sm"
        isBusy={isSaving}
        busyLabel={t('app.entries.creating')}
        disabledReason={draftTitle.trim() === '' ? t('app.entries.title_required') : undefined}
        onclick={create}
      >
        {t('app.entries.create')}
      </Button>
      {#if !(isSimple && addingUnder === 'root')}
        <!-- A simple list's capture line is always there; there is nothing to cancel. -->
        <Button size="sm" tone="secondary" onclick={() => (addingUnder = null)}>
          {t('app.entries.cancel')}
        </Button>
      {/if}
    </Inline>
  </Stack>
{/snippet}

{#if isChoosingCategories && structure && simpleLevel && containerVersion !== undefined}
  <CategoriesDialog
    {collectionId}
    version={containerVersion}
    {structure}
    itemType={simpleLevel.type}
    onclose={() => (isChoosingCategories = false)}
  />
{/if}

{#if amending}
  <Dialog
    isOpen={true}
    title={t('app.entries.change_amount')}
    dismissLabel={t('app.entries.cancel')}
    onClose={() => (amending = undefined)}
  >
    <Stack gap="150">
      <Input label={t('app.entries.amount_label')} bind:value={draftAmount} maxlength={40} />
      <Inline gap="100">
        <Button size="sm" onclick={saveAmount}>{t('app.entries.save')}</Button>
        <Button size="sm" tone="secondary" onclick={() => (amending = undefined)}>{t('app.entries.cancel')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

{#if categorising}
  {@const row = categorising}
  <Dialog
    isOpen={true}
    title={t('app.entries.choose_category')}
    dismissLabel={t('app.entries.cancel')}
    onClose={() => (categorising = undefined)}
  >
    <Stack gap="100">
      {#each categories.filter((each) => available.some((label) => label.id === each)) as category (category)}
        <Button
          tone={(row.item.label_ids ?? []).includes(category) ? 'primary' : 'secondary'}
          onclick={() => chooseCategory(row, category)}
        >
          {categoryName(category)}
        </Button>
      {/each}
      <Button tone="subtle" onclick={() => chooseCategory(row, null)}>{t('app.entries.no_category')}</Button>
    </Stack>
  </Dialog>
{/if}

<style>
  /* What `Stack gap="050"` was, written here because the level now owns a state of its own: a row
     being dragged is drawn differently, and a primitive that decorated would stop being one. */
  .level { display: flex; flex-direction: column; gap: var(--sp-050); }

  /* The level is what a row's indent is measured against (`TaskRow` caps it below `medium` by a
     container query), so it is a container: the width of the tree, not of the screen, decides. */
  .level { container-type: inline-size; }

  .row { display: flex; align-items: center; gap: var(--sp-050); }

  /* A simple list's row ticks under a tap and a sideways swipe; scrolling stays the browser's. */
  .level[data-simple] .row { touch-action: pan-y; cursor: pointer; }

  .section-row { display: flex; align-items: center; gap: var(--sp-050); }

  .section-head {
    margin: var(--sp-150) 0 0;
    font-size: var(--fs-075);
    font-weight: 600;
    color: var(--text-subtle);
  }

  .progress {
    font-size: var(--fs-075);
    font-variant-numeric: tabular-nums;
    color: var(--text-subtle);
    white-space: nowrap;
  }

  /* The rows and the level are the slots a moment sits over. */
  .level { position: relative; }
  .row[data-celebrating] { position: relative; }

  /* The badge beside it is what *says* archived; this only makes the row recede, so what can be
     acted on reads first. Rule 3 is kept by the badge, not by this. */
  .row[data-archived] { color: var(--text-subtle); }

  .row > :global(*:last-child) { flex: 1; min-width: 0; }

  /* In a narrow tree the task row stacks its controls under its title (`TaskRow`), so the pick
     and the grip keep to the title's line rather than floating beside the middle of the block:
     aligned to the start, and as tall as the title's line is - the spacious control and the
     row's padding - so the checkbox sits centred on it. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a container query cannot read a custom property. */
  @container (inline-size < 600px) {
    .row { align-items: start; }

    .pick,
    .grip { min-block-size: calc(var(--density-control-md-min) + 2 * var(--density-row-block)); }
  }

  /* `touch-action: none` is what makes a drag possible on a touch screen at all: without it the
     browser claims the gesture for scrolling and the pointer events stop arriving after the first
     few. It is on the grip alone, so the page still scrolls everywhere else. */
  .pick { display: flex; align-items: center; }

  .bulk-note { margin: 0 0 0 var(--sp-300); color: var(--text-danger); font-size: var(--fs-075); }

  .grip {
    display: inline-flex;
    flex: none;
    align-items: center;
    color: var(--text-subtle);
    cursor: grab;
    touch-action: none;
  }

  /* Rule 6: the movement is a `translate` and nothing else, and it is direct manipulation rather
     than decoration — the row is under the reader's finger. */
  .row[data-dragging] {
    translate: 0 var(--drag-offset);
    /* Rule 1: it is off the surface while it is being carried, so it is raised. */
    box-shadow: var(--shadow-overlay);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    /* Out of the way of the measuring: the element under the pointer must be the list, not the
       row being carried over it. */
    pointer-events: none;
  }

  .row[data-dragging] .grip { cursor: grabbing; }

  /* Where it would land. Rule 3: an outline and not a tint, so it reads in greyscale and to a
     reader who does not perceive the accent. */
  .row[data-drop] {
    outline: var(--bw-thick) dashed var(--accent-primary);
    outline-offset: var(--sp-025);
    border-radius: var(--r-md);
  }

  /* Rule 6's floor. Under a reduced-motion preference the row does not travel; the state and the
     landing slot still say what is happening, which is the colour change the rule fixes as the
     least a movement may reduce to. */
  @media (prefers-reduced-motion: reduce) {
    .row[data-dragging] { translate: none; }
  }

  :global([data-motion='reduced']) .row[data-dragging] { translate: none; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }

  .note { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); max-width: 64ch; }

  .more summary { cursor: pointer; color: var(--text-subtle); font-size: var(--fs-075); }

  /* Rule 1: a standalone notice, so it is raised rather than tinted. Rule 3: it is a list of what
     was lost and why, so nothing about it rests on the colour. */
</style>
