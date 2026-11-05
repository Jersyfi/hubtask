// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The tour, walked: which step stands, the element it points at, and the three verbs.
 *
 * **It starts on the first sign-in of an account whose `onboarding_completed_at` is null**, once
 * the account has arrived; the help menu restarts it by clearing the field. **Skipping writes the
 * field**, and so does **walking it to its end**: a tour that came back on every sign-in would
 * teach people to skip. The last step leads to the creation dialog; the first completion after it
 * is still the first celebration (design-system.md §7), which this tab remembers as `led` rather
 * than reading it from the field.
 *
 * Each step is a route and an element: the tour navigates, waits for the element to be drawn,
 * and points at it. An element that never arrives is not pointed at nothing - the step is passed
 * over, forwards or backwards, whichever way the person was going.
 */

import { actor } from './data/account.svelte.ts';
import { engine } from './data/engine.ts';
import { holdsHere } from './data/capability.svelte.ts';
import { manifest } from './data/capabilities.svelte.ts';
import { products } from './navigation.ts';
import { preferences } from './data/preferences.svelte.ts';
import { viewport } from './frame/viewport.svelte.ts';
import { hubsInTourOrder, STARTER_HUB_SESSION_KEY, TOUR_SESSION_KEY, stepsFor, type Step } from './tour.ts';

/** How long a route may take to draw the element a step points at. */
const ARRIVAL_ATTEMPTS = 40;
const ARRIVAL_INTERVAL_MS = 50;

type Document = Record<string, unknown>;

class TourStore {
  #steps = $state<readonly Step[]>([]);
  #index = $state(-1);
  #target = $state<HTMLElement | null>(null);
  #navigate: ((path: string) => void) | undefined;
  /** Which walk is current, so a slow arrival from an earlier step is thrown away. */
  #walk = 0;

  get isOpen(): boolean {
    return this.#index >= 0 && this.#index < this.#steps.length;
  }

  get step(): Step | undefined {
    return this.isOpen ? this.#steps[this.#index] : undefined;
  }

  get index(): number {
    return this.#index;
  }

  get count(): number {
    return this.#steps.length;
  }

  get target(): HTMLElement | null {
    return this.#target;
  }

  get isLast(): boolean {
    return this.isOpen && this.#index === this.#steps.length - 1;
  }

  /** The frame hands in how to navigate; the router is the frame's, not this module's. */
  attach(navigate: (path: string) => void): void {
    this.#navigate = navigate;
  }

  /** Whether the account the frame just learned about should be led through: never taken, and not walked in this tab. */
  shouldStart(): boolean {
    const account = actor.account;
    if (!account || account.onboarding_completed_at) return false;
    const walked = session()?.getItem(TOUR_SESSION_KEY);
    return walked !== 'walked' && walked !== 'led';
  }

  /** Whether the tour led to the creation dialog in this tab and no entry was completed since. */
  get awaitsFirstCompletion(): boolean {
    return session()?.getItem(TOUR_SESSION_KEY) === 'led';
  }

  /**
   * The step this tab was at when the page was loaded anew - a newer release is loaded at the
   * next navigation (api-guidelines.md §8), and the tour's own moves are navigations - or
   * nothing where it was at none.
   */
  resumesAt(): number | undefined {
    const at = /^at:(\d+)$/.exec(session()?.getItem(TOUR_SESSION_KEY) ?? '')?.[1];
    return at === undefined ? undefined : Number(at);
  }

  /** Starts at the first step, or at `at`, reading what the workspace holds from the copy. */
  async start(at = 0): Promise<void> {
    const workspace = await this.#workspace();
    // The jumble's step only for somebody the jumble is drawn for: a step pointing at a screen
    // that answers "nothing here" would be a tour of a refusal.
    const frame = viewport.isCompact ? 'phone' : viewport.isBelowExpanded ? 'drawer' : 'pinned';
    this.#steps = stepsFor({ ...workspace, hasJumble: holdsHere('READ') === true, firstProduct: products({ modulesOn: manifest.value?.modules_on })[0]?.id, frame });
    await this.#go(Math.min(at, this.#steps.length - 1), 1);
  }

  /** The starter made `hubId`: the tour in this tab walks it first. */
  starterMade(hubId: string): void {
    try {
      session()?.setItem(STARTER_HUB_SESSION_KEY, hubId);
    } catch {
      // A tab that cannot remember it walks the first hub listed, as before the starter.
    }
  }

  /** The help menu: the field cleared, so the tour is the account's again, and started. */
  async restart(): Promise<void> {
    session()?.removeItem(TOUR_SESSION_KEY);
    const accountId = actor.account?.id;
    try {
      if (accountId) await preferences.setAccount(accountId, { onboarding_completed_at: null });
    } catch {
      // The person asked for the tour now; a field that could not be cleared costs them the
      // next sign-in's start, not this walk.
    }
    await this.start();
  }

  async next(): Promise<void> {
    if (!this.isOpen) return;
    if (this.isLast) {
      await this.#lead();
      return;
    }
    await this.#go(this.#index + 1, 1);
  }

  async back(): Promise<void> {
    if (!this.isOpen || this.#index === 0) return;
    await this.#go(this.#index - 1, -1);
  }

  /** Skipping ends the tour and writes the field: it is not offered again. */
  async skip(): Promise<void> {
    this.#close();
    session()?.setItem(TOUR_SESSION_KEY, 'walked');
    await this.#write();
  }

  /** The first completion: the tour's end and the first moment coincide (§8). */
  async complete(): Promise<void> {
    this.#close();
    session()?.setItem(TOUR_SESSION_KEY, 'walked');
    await this.#write();
  }

  /**
   * The last step's verb: the mark goes, the control it pointed at is pressed - the creation
   * dialog opens, or the hub's - and the field is written; the first moment waits for the completion.
   */
  async #lead(): Promise<void> {
    const control = this.#target;
    this.#close();
    session()?.setItem(TOUR_SESSION_KEY, 'led');
    void this.#write();
    control?.focus();
    // The seventh points at a control where a collection exists; where none does it points at
    // the tree, and the person makes the hub themselves.
    if (control && (control.matches('button, a[href]') || control.querySelector('button'))) {
      (control.matches('button, a[href]') ? control : control.querySelector<HTMLElement>('button'))?.click();
    }
  }

  #close(): void {
    this.#walk += 1;
    this.#index = -1;
    this.#target = null;
  }

  async #write(): Promise<void> {
    const accountId = actor.account?.id;
    if (!accountId) return;
    try {
      await preferences.setAccount(accountId, { onboarding_completed_at: new Date().toISOString() });
    } catch {
      // A server that could not be reached keeps the field null, and the tour comes back on the
      // next sign-in - which is the honest outcome of a write that did not land.
    }
  }

  /** Goes to a step: navigates, waits for its element, and passes it over in `direction` if it never comes. */
  async #go(index: number, direction: 1 | -1): Promise<void> {
    const walk = ++this.#walk;
    let at = index;
    while (at >= 0 && at < this.#steps.length) {
      const step = this.#steps[at]!;
      this.#index = at;
      this.#target = null;
      this.#navigate?.(step.route);
      const element = await arrival(step.selector);
      if (walk !== this.#walk) return;
      if (element) {
        this.#target = element;
        session()?.setItem(TOUR_SESSION_KEY, `at:${at}`);
        return;
      }
      at += direction;
    }
    // Nothing left to point at in that direction: the tour ends as walked.
    this.#close();
    session()?.setItem(TOUR_SESSION_KEY, 'walked');
    await this.#write();
  }

  /**
   * Any collection and any entry the workspace holds, for the steps that need one. Asked of the
   * server through the engine rather than of the copy: the tour starts on the first sign-in,
   * which is before the copy has been synchronised, and the engine answers from the copy anyway
   * where the server is away.
   */
  async #workspace(): Promise<{ collectionId?: string; itemId?: string }> {
    try {
      // A collection is listed under its hub, so the hubs come first and the first hub with one
      // wins - the starter's before the workspace's default, which every workspace holds.
      const hubs = await engine.refresh<{ data?: readonly Document[] }>({ path: '/containers?type=HUB&page_size=200' });
      const listed = (hubs.status === 'ready' ? (hubs.data.data ?? []) : []).flatMap((hub) => (typeof hub.id === 'string' ? [hub.id] : []));
      let collection: Document | undefined;
      for (const hub of hubsInTourOrder(listed, session()?.getItem(STARTER_HUB_SESSION_KEY) ?? undefined)) {
        const collections = await engine.refresh<{ data?: readonly Document[] }>({ path: `/containers?parent_id=${hub}&page_size=1` });
        collection = collections.status === 'ready' ? collections.data.data?.[0] : undefined;
        if (collection) break;
      }
      if (typeof collection?.id !== 'string') return {};
      const items = await engine.refresh<{ data?: readonly Document[] }>({ path: `/items?collection_id=${collection.id}&page_size=1` });
      const item = items.status === 'ready' ? items.data.data?.[0] : undefined;
      return { collectionId: collection.id, ...(typeof item?.id === 'string' ? { itemId: item.id } : {}) };
    } catch {
      return {};
    }
  }
}

/** The element a step points at, once the route has drawn it - or nothing after the attempts. */
export async function arrival(selector: string): Promise<HTMLElement | null> {
  for (let attempt = 0; attempt < ARRIVAL_ATTEMPTS; attempt += 1) {
    const element = document.querySelector<HTMLElement>(selector);
    if (element && element.getClientRects().length > 0) return element;
    await new Promise((resolve) => setTimeout(resolve, ARRIVAL_INTERVAL_MS));
  }
  return null;
}

/** `sessionStorage` where it can be reached: what is walked in this tab dies with the tab. */
function session(): Storage | undefined {
  try {
    return globalThis.sessionStorage;
  } catch {
    return undefined;
  }
}

export const tour = new TourStore();
