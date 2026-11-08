// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the page kinds of wave 6 share (docs/backlog/page-system/ §7): the head every kind but the
// record page draws, as the props of `PageHeader` a kind passes through. A kind adds nothing beside
// its slots, so a screen fills them and the head is the same everywhere.

import type { Snippet } from 'svelte';

import type { MenuItem } from './overlay.ts';
import type { PageAction } from './PageHeader.svelte';
import type { Crumb } from './structure.ts';

export type PageKind = 'list' | 'record' | 'settings' | 'editor' | 'overview' | 'log' | 'create' | 'work' | 'detail';

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
