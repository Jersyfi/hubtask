// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A product's tour, walked (user-experience.md §3.4): started by the first visit of its home - its
 * first step is the mark pointing at the create act -, from the product's first empty state and
 * from *Help*. Ending it, walked or skipped, writes the product to the account's `products_seen`,
 * so the first-visit mark is not shown again on any device; a write that does not land shows it
 * once more at the next visit, which is the honest outcome.
 */

import type { Account } from '@hubtask/sync-engine';

import { actor } from '../data/account.svelte.ts';
import { preferences } from '../data/preferences.svelte.ts';
import { arrival } from '../tour.svelte.ts';
import { productTourSteps, withSeen, type ProductStep } from './producttour.ts';

class ProductTourStore {
  #module = $state<string | undefined>(undefined);
  #steps = $state<readonly ProductStep[]>([]);
  #index = $state(-1);
  #target = $state<HTMLElement | null>(null);
  #navigate: ((path: string) => void) | undefined;
  /** Which walk is current, so a slow arrival from an earlier step is thrown away. */
  #walk = 0;
  /** The modules whose mark this tab already showed, so a write still in flight shows none twice. */
  #shown = new Set<string>();

  get isOpen(): boolean {
    return this.#index >= 0 && this.#index < this.#steps.length;
  }

  get module(): string | undefined {
    return this.isOpen ? this.#module : undefined;
  }

  get step(): ProductStep | undefined {
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

  attach(navigate: (path: string) => void): void {
    this.#navigate = navigate;
  }

  /** Whether this tab already showed `module`'s first mark. */
  wasShown(module: string): boolean {
    return this.#shown.has(module);
  }

  /** Starts `module`'s tour at its first step: the create act on its home. */
  async start(module: string): Promise<void> {
    const steps = productTourSteps(module);
    if (steps.length === 0) return;
    this.#shown.add(module);
    this.#module = module;
    this.#steps = steps;
    await this.#go(0, 1);
  }

  async next(): Promise<void> {
    if (!this.isOpen) return;
    if (this.isLast) {
      await this.#end();
      return;
    }
    await this.#go(this.#index + 1, 1);
  }

  async back(): Promise<void> {
    if (!this.isOpen || this.#index === 0) return;
    await this.#go(this.#index - 1, -1);
  }

  async skip(): Promise<void> {
    await this.#end();
  }

  async #end(): Promise<void> {
    const module = this.#module;
    this.#walk += 1;
    this.#index = -1;
    this.#target = null;
    if (module) await this.#write(module);
  }

  async #write(module: string): Promise<void> {
    const account = actor.account;
    // A key of `productTourSteps`, which answers only for the contract's modules.
    const key = module as NonNullable<Account['products_seen']>[number];
    if (!account || account.products_seen?.includes(key)) return;
    try {
      await preferences.setAccount(account.id, { products_seen: withSeen(account.products_seen, key) });
    } catch {
      // Not written: the mark comes back at the next visit in another tab.
    }
  }

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
        return;
      }
      at += direction;
    }
    await this.#end();
  }
}

export const productTour = new ProductTourStore();
