// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The tour's content (design-system.md §8, F6-14): six steps on the real interface, each a route
 * and an element found by `data-tour`, and the seventh which is not a step - *now make one*.
 *
 * What the workspace holds decides which steps can be walked: a collection's list and board and
 * an entry need a collection and an entry to point at, and a fresh workspace has neither. Those
 * steps are left out rather than pointed at nothing, and the seventh then points at where the
 * first hub is made. The count a person reads is the count of what they will actually see.
 */

export type StepId = 'hubs' | 'layouts' | 'entry' | 'jumble' | 'search' | 'profile' | 'make';

export interface Step {
  readonly id: StepId;
  /** Where the element is. */
  readonly route: string;
  /** How the element is found once the route is drawn. */
  readonly selector: string;
  /** Where the mark sits relative to the cut-out. */
  readonly side: 'block-end' | 'inline-end';
}

/** What the workspace holds, as far as the tour needs to know. */
export interface Workspace {
  /** Any collection's identifier, or nothing in a workspace without one. */
  readonly collectionId?: string;
  /** Any entry's identifier, or nothing. */
  readonly itemId?: string;
}

/** The steps a workspace can be walked through, in order. */
export function stepsFor(workspace: Workspace): readonly Step[] {
  const steps: Step[] = [{ id: 'hubs', route: '/', selector: '[data-tour="hubs"]', side: 'inline-end' }];
  if (workspace.collectionId) {
    steps.push({ id: 'layouts', route: `/collections/${workspace.collectionId}`, selector: '[data-tour="layouts"]', side: 'block-end' });
  }
  if (workspace.itemId) {
    steps.push({ id: 'entry', route: `/items/${workspace.itemId}`, selector: '[data-tour="entry"]', side: 'block-end' });
  }
  steps.push(
    { id: 'jumble', route: '/jumble', selector: '[data-tour="jumble"]', side: 'block-end' },
    { id: 'search', route: '/search', selector: '[data-tour="search"]', side: 'block-end' },
    { id: 'profile', route: '/profile', selector: '[data-tour="profile"]', side: 'block-end' },
  );
  // The seventh: the entry the tour leads to. Where a collection exists, its add control; where
  // none does, the hub that has to come first.
  steps.push(
    workspace.collectionId
      ? { id: 'make', route: `/collections/${workspace.collectionId}`, selector: '[data-opener="add-entry"]', side: 'block-end' }
      : { id: 'make', route: '/', selector: '[data-tour="hubs"]', side: 'inline-end' },
  );
  return steps;
}

/** The key under which this tab remembers that the tour was walked to its end or skipped. */
export const TOUR_SESSION_KEY = 'hubtask.tour';
