// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What an entry's menu lists, wherever the entry is shown (UC-WRK-28 check 1).
 *
 * One answer for the list, the subtree, the board, the timeline and search - and the entry's own
 * page and *Today* when they draw it - so the same entry offers the same actions in the same order
 * in every place. Pure: what the reader may do, what the type carries and where the entry stands
 * arrive as a context; the words arrive through `t`. A surface adds only what is its own: the
 * board's columns after the moves, a simple list's amount and category after *Open*.
 *
 * Absent and off are different answers (P-05, design-system.md §4): what the reader's role may not
 * do and what the type does not carry is not listed; what is impossible *now* - an archived entry,
 * the first of its list - is listed with its reason. A read-only area (an archived collection, or
 * something above the entry archived) lists only what does not write the entry, without a reason
 * on every row: the area's one notice says why.
 */

import type { MenuItem } from '@hubtask/design-system/components';

import type { Archival } from '../data/containers.ts';

/** `simple` is a collection of one level drawn as a simple list (UC-WRK-25): no levels to move between. */
export type EntrySurface = 'list' | 'simple' | 'subtree' | 'board' | 'timeline' | 'search' | 'today' | 'page';

/** Where the entry stands in its level, for the four rank commands and the two moves. */
export interface EntryLevel {
  readonly index: number;
  readonly count: number;
  /** The title of the sibling directly above, which *Move inside* names; absent at the top. */
  readonly aboveTitle?: string;
  /** Why it cannot move inside the one above, already in the reader's words, or nothing. */
  readonly insideReason?: string;
  /** Why it cannot move out one level, already in the reader's words, or nothing. */
  readonly outReason?: string;
}

export interface EntryMenuContext {
  readonly surface: EntrySurface;
  readonly item: {
    readonly title: string;
    readonly isCompleted: boolean;
    readonly hasDue: boolean;
  };
  /** The entry's own archival, or inherited from what holds it. */
  readonly archival: Archival;
  /** Whether the collection itself is read-only (archived, I-C3). */
  readonly isCollectionReadOnly: boolean;
  /** Whether the reader's role may change this entry; `undefined` while the role is being read. */
  readonly mayChange: boolean | undefined;
  /** Whether the reader may create entries in this collection. */
  readonly mayCreate: boolean;
  /** Whether the reader may grant roles on the entry (`MANAGE_MEMBERS` along its path). */
  readonly mayShare: boolean;
  /** What the entry's type carries, by the manifest's capability names. */
  readonly carries: (capability: string) => boolean;
  /** The level, where the surface knows it; `undefined` while it is being read. */
  readonly level: EntryLevel | undefined;
  /** What only this surface offers, drawn after the moves (the board's columns). */
  readonly extras?: readonly MenuItem[];
  /**
   * What a simple list offers of its own, drawn after *Open*: the amount where the type carries one
   * there, the category where the list has categories and the type carries labels.
   */
  readonly simple?: { readonly amount: boolean; readonly category: boolean };
  /**
   * The entry's own page (UC-WRK-28 check 2), where the reader may not share: *People*, to read who
   * has access, stands where *Share* would (UC-WRK-21 check 7). Elsewhere a reader who may not
   * share is offered neither.
   */
  readonly readsPeople?: boolean;
}

type Translate = (code: string, params?: Record<string, string | number>) => string;

/** The surfaces where selecting acts: a bulk bar is there to act on what is picked. */
const SELECTING: ReadonlySet<EntrySurface> = new Set(['list', 'simple', 'board']);

const RANKS = [
  { id: 'up', label: 'app.rank.up', isForward: false },
  { id: 'down', label: 'app.rank.down', isForward: true },
  { id: 'top', label: 'app.rank.top', isForward: false },
  { id: 'bottom', label: 'app.rank.bottom', isForward: true },
] as const;

/** The menu's items for one entry, in their one order (D2: *Select* first, the trash last). */
export function entryMenu(context: EntryMenuContext, t: Translate): MenuItem[] {
  // On its own page the entry is open already: *Open* is every other surface's.
  const opens: MenuItem[] = context.surface === 'page' ? [] : [{ id: 'open', label: t('app.entry_menu.open') }];
  const copyLink: MenuItem = { id: 'copy-link', label: t('app.entry_menu.copy_link') };
  const mayDuplicate = context.mayCreate && !context.isCollectionReadOnly;
  const duplicate: MenuItem = { id: 'duplicate', label: t('app.duplicate.title') };
  /** Who has access, read only: the page's, where the reader may not share. */
  const people = (): MenuItem[] => (context.readsPeople && !context.mayShare ? [{ id: 'share', label: t('app.people.title') }] : []);

  // Until the role is known nothing that writes is drawn, and no reason either.
  if (context.mayChange === undefined) return [...opens, copyLink];

  // A read-only area: what does not write the entry, and nothing that would only say "archived"
  // on every row.
  if (context.archival === 'inherited' || context.isCollectionReadOnly) {
    return [...opens, ...people(), copyLink, ...(mayDuplicate ? [{ ...duplicate, hasSeparatorBefore: true }] : [])];
  }

  // The role may not change it: a copy where the reader may create one, and the ways to look at it.
  if (!context.mayChange) {
    return [
      ...opens,
      ...(mayDuplicate ? [{ ...duplicate, hasSeparatorBefore: opens.length > 0 }] : []),
      ...(context.mayShare ? [{ id: 'share', label: t('app.people.share') }] : people()),
      copyLink,
    ];
  }

  // A writer. An entry archived in its own right keeps every item with the reason (pattern 2),
  // except the way back and the trash (I-W4, the state machine's "archived → trashed").
  const archived = context.archival === 'archived' ? t('app.entries.archived') : undefined;
  const items: MenuItem[] = [];

  // *Select* first: on a list and a board a still hold opens this menu rather than selecting (ADR-0088).
  if (SELECTING.has(context.surface)) items.push({ id: 'select', label: t('app.entry_menu.select') });
  items.push(...opens);
  const isSimple = context.surface === 'simple';
  if (isSimple && context.simple?.amount) {
    items.push({ id: 'amount', label: t('app.entries.change_amount'), disabledReason: archived });
  }
  if (isSimple && context.simple?.category) {
    items.push({ id: 'category', label: t('app.entries.choose_category'), disabledReason: archived });
  }
  if (context.carries('COMPLETION')) {
    items.push({
      id: context.item.isCompleted ? 'reopen' : 'complete',
      label: t(context.item.isCompleted ? 'app.entry_menu.reopen' : 'app.entry_menu.complete'),
      disabledReason: archived,
    });
  }
  if (context.carries('DUE_DATE')) {
    items.push({ id: 'dates', label: t(context.item.hasDue ? 'app.due.edit' : 'app.due.set'), disabledReason: archived });
  }
  if (context.carries('ASSIGNMENT')) {
    items.push({ id: 'person', label: t('app.entry_menu.person'), disabledReason: archived });
  }
  if (context.carries('LABELS')) {
    items.push({ id: 'labels', label: t('app.entry_menu.labels'), disabledReason: archived });
  }

  const level = context.level;
  const moving = t('app.move.moving');
  // A simple list ranks only its open entries - a done one sits under *Done*, by when - and has no
  // level to move inside or out of.
  const ranks = isSimple && context.item.isCompleted ? [] : RANKS;
  ranks.forEach((command, position) => {
    const isAtEdge =
      level !== undefined && (command.isForward ? level.index >= level.count - 1 : level.index <= 0);
    items.push({
      id: command.id,
      label: t(command.label),
      disabledReason:
        archived ??
        (level === undefined
          ? moving
          : isAtEdge
            ? t(command.isForward ? 'app.rank.already_last' : 'app.rank.already_first')
            : undefined),
      hasSeparatorBefore: position === 0,
    });
  });
  if (!isSimple) {
    items.push({
      id: 'inside',
      label: level?.aboveTitle ? t('app.move.inside', { name: level.aboveTitle }) : t('app.move.inside_above'),
      disabledReason: archived ?? (level === undefined ? moving : level.insideReason),
    });
    items.push({
      id: 'out',
      label: t('app.move.out'),
      disabledReason: archived ?? (level === undefined ? moving : level.outReason),
    });
  }
  items.push({
    id: 'elsewhere',
    label: t('app.move.elsewhere'),
    disabledReason: archived,
    ...(ranks.length === 0 ? { hasSeparatorBefore: true } : {}),
  });

  context.extras?.forEach((extra, position) => {
    items.push({ ...extra, disabledReason: archived ?? extra.disabledReason, hasSeparatorBefore: position === 0 });
  });

  if (mayDuplicate) items.push({ ...duplicate, hasSeparatorBefore: true });
  const sharing = context.mayShare ? [{ id: 'share', label: t('app.people.share') }] : people();
  sharing.forEach((each) => items.push({ ...each, hasSeparatorBefore: !mayDuplicate }));
  items.push({ ...copyLink, hasSeparatorBefore: !mayDuplicate && sharing.length === 0 });

  items.push({
    id: context.archival === 'archived' ? 'unarchive' : 'archive',
    label: t(context.archival === 'archived' ? 'app.entries.unarchive' : 'app.entries.archive'),
    hasSeparatorBefore: true,
  });
  items.push({ id: 'trash', label: t('app.entries.trash'), isDestructive: true, hasSeparatorBefore: true });
  return items;
}

/** The ids of a menu, in order: what the engine walks compare across the surfaces. */
export const idsOf = (items: readonly MenuItem[]): string[] => items.map((item) => item.id);
