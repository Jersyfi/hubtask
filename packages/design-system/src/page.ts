// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the page kinds of wave 6 share (docs/backlog/page-system/ §7): the head every kind but the
// record page draws, as the props of `PageHeader` a kind passes through. A kind adds nothing beside
// its slots, so a screen fills them and the head is the same everywhere.

import { getContext, setContext, type Snippet } from 'svelte';

import type { MenuItem } from './overlay.ts';
import type { PageAction } from './PageHeader.svelte';
import type { Crumb } from './structure.ts';

export type PageKind = 'list' | 'record' | 'settings' | 'editor' | 'overview' | 'log' | 'create' | 'work' | 'detail' | 'standalone' | 'browse';

export interface PageHead {
  readonly title: string;
  readonly subtitle?: string;
  readonly breadcrumb?: { trail: readonly Crumb[]; label: string; expandLabel: string; onnavigate?: (id: string) => void };
  /** The one act that creates: the head's primary (PS-3). */
  readonly primary?: PageAction;
  readonly secondary?: readonly [] | readonly [PageAction] | readonly [PageAction, PageAction];
  readonly menu?: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void };
  /** The one-line figure under the title: *9 people · 3 groups · 12 roles*. */
  readonly figure?: Snippet;
}

/** The way back from a section or an object to its list, below `expanded`. */
export interface PageBack {
  readonly label: string;
  readonly onback: () => void;
}

/**
 * The application's bar, where it carries *‹ the level above* (the navigation concept §9: on a
 * phone the way back stands only in the bar). A page kind that has a way back of its own hands it
 * to the bar instead of drawing it a second time on the page.
 */
export interface PageBar {
  /** Whether the bar draws a way back now; the page then draws none of its own. */
  readonly hasBack: boolean;
  /** Gives the bar the page's own way back - an open section's *‹ Settings* - until the cleanup. */
  carry(back: PageBack): () => void;
  /** Takes the head's folded ⋯ into the bar where the bar carries the title (`PageHeader`'s `onmenu`). */
  offer(menu: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void; opener?: string } | undefined): void;
}

const BAR = Symbol('hubtask.page-bar');

/** Called by the application's frame while it initialises. */
export function providePageBar(bar: PageBar): void {
  setContext(BAR, bar);
}

/** Called by a page kind while it initialises; `undefined` outside a frame that has such a bar. */
export function pageBar(): PageBar | undefined {
  return getContext<PageBar | undefined>(BAR);
}
