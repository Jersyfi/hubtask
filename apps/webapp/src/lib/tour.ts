// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The tour's content (design-system.md §8): steps on the real interface, each a route and an
 * element found by `data-tour`, and the last which is not a step - *now make one*.
 *
 * What the workspace holds decides which steps can be walked: a collection's list and board and
 * an entry need a collection and an entry to point at, and a fresh workspace has neither. Those
 * steps are left out rather than pointed at nothing, and the last then points at where the
 * first hub is made. A step whose element this width never draws - the products' column below
 * `expanded`, the palette's way in on a phone - is not a step there, so the count a person reads is
 * the count of what the workspace and the width can show; a step whose element does not arrive
 * is still passed over when it is reached.
 */

export type StepId = 'hubs' | 'products' | 'capture' | 'palette' | 'layouts' | 'entry' | 'levels' | 'jumble' | 'search' | 'profile' | 'make';

export interface Step {
  readonly id: StepId;
  /** Where the element is. */
  readonly route: string;
  /** How the element is found once the route is drawn. */
  readonly selector: string;
  /** Where the mark sits relative to the cut-out. */
  readonly side: 'block-end' | 'inline-end';
  /** The words' key under `app.tour.` where they differ from the step's id. */
  readonly words?: string;
}

/** What the workspace holds, as far as the tour needs to know. */
export interface Workspace {
  /** Any collection's identifier, or nothing in a workspace without one. */
  readonly collectionId?: string;
  /** Any entry's identifier, or nothing. */
  readonly itemId?: string;
  /** Whether the jumble is the reader's to see: `READ` on the workspace itself. */
  readonly hasJumble: boolean;
  /** The first row of the products band, where any product is on (user-experience.md §3.4). */
  readonly firstProduct?: string;
  /**
   * How the frame is drawn: the tree pinned as a column, behind the bar's ☰ (below `expanded`), or
   * a phone's (`compact`), whose bar has no palette.
   */
  readonly frame?: 'pinned' | 'drawer' | 'phone';
}

/** The steps a workspace can be walked through, in order. */
export function stepsFor(workspace: Workspace): readonly Step[] {
  const frame = workspace.frame ?? 'pinned';
  const steps: Step[] = [
    // Below `expanded` the tree is behind the bar's ☰, which is what the step points at and says.
    frame === 'pinned'
      ? { id: 'hubs', route: '/', selector: '[data-tour="hubs"]', side: 'inline-end' }
      : { id: 'hubs', route: '/', selector: '[data-tour="hubs"]', side: 'block-end', words: 'hubs_drawer' },
    // *Your products live here*: the band's first row in the column, which only `expanded` pins.
    ...(workspace.firstProduct && frame === 'pinned'
      ? [{ id: 'products' as const, route: '/', selector: `[data-tour="hubs"] [data-node="${workspace.firstProduct}"]`, side: 'inline-end' as const }]
      : []),
    // Capture from every page (ux-inventory.md §E1): the bar's button, or the bottom bar's ＋ on a
    // phone - the one act among its destinations.
    { id: 'capture', route: '/', selector: '[data-tour="capture"], nav button[data-action]', side: 'block-end' },
    // A phone's search is the bottom bar's: its bar draws no palette.
    ...(frame === 'phone' ? [] : [{ id: 'palette' as const, route: '/', selector: '[data-tour="palette"]', side: 'block-end' as const }]),
  ];
  if (workspace.collectionId) {
    steps.push({ id: 'layouts', route: `/collections/${workspace.collectionId}`, selector: '[data-tour="layouts"]', side: 'block-end' });
  }
  if (workspace.itemId) {
    steps.push({ id: 'entry', route: `/items/${workspace.itemId}`, selector: '[data-tour="entry"]', side: 'block-end' });
    // The levels under an entry, where its type holds any; a list's entry holds none, and the step
    // is passed over there.
    steps.push({ id: 'levels', route: `/items/${workspace.itemId}`, selector: '[data-tour="levels"]', side: 'block-end' });
  } else if (workspace.collectionId) {
    // No entry yet - a fresh workspace: the levels are told at the list's "show what is inside",
    // which is where they show before an entry holds any.
    steps.push({ id: 'levels', route: `/collections/${workspace.collectionId}`, selector: '[data-tour="levels-inside"]', side: 'block-end' });
  }
  if (workspace.hasJumble) {
    steps.push({ id: 'jumble', route: '/jumble', selector: '[data-tour="jumble"]', side: 'block-end' });
  }
  steps.push(
    { id: 'search', route: '/search', selector: '[data-tour="search"]', side: 'block-end' },
    { id: 'profile', route: '/profile', selector: '[data-tour="profile"]', side: 'block-end' },
  );
  // The last: the entry the tour leads to. Where a collection exists, its add control; where
  // none does, the hub that has to come first.
  steps.push(
    workspace.collectionId
      ? { id: 'make', route: `/collections/${workspace.collectionId}`, selector: '[data-opener="add-entry"]', side: 'block-end' }
      : { id: 'make', route: '/', selector: '[data-tour="hubs"]', side: 'inline-end' },
  );
  return steps;
}

/**
 * Whether the tour may start at this address: only on the overview, where its first step is.
 * Its first step navigates, so a start anywhere else would replace an address somebody opened by
 * link or bookmark with the overview; it starts instead when they first arrive there.
 */
export function startsAt(path: string): boolean {
  return path === '/';
}

/** The key under which this tab remembers the tour: `walked` to its end or skipped, or `at:<n>`, the step it stands at. */
export const TOUR_SESSION_KEY = 'hubtask.tour';

/** The key under which this tab remembers the hub the starter made, which the tour walks first. */
export const STARTER_HUB_SESSION_KEY = 'hubtask.starter.hub';

/**
 * The hubs in the order the tour looks for a collection in them: the one the starter made first
 * (user-experience.md §3.1, the tour on what the starter made), then the rest as listed.
 */
export function hubsInTourOrder(hubs: readonly string[], made: string | undefined): string[] {
  return made && hubs.includes(made) ? [made, ...hubs.filter((each) => each !== made)] : [...hubs];
}
