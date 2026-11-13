// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What *Erstellen* offers where one stands (work-surface concept §6.12, C-1…C-5). Pure: the
 * questions are asked by the frame - the role at the hub, the modules on there, whether one may
 * capture - and this only says which kinds follow, so the offer is tested without mounting.
 *
 * A kind one may not create here is absent, never grey (C-2, P-05). Posts are not offered: no
 * post exists yet, and a kind that cannot be made is no offer.
 */

export type CreateKind = 'entry' | 'page' | 'whiteboard' | 'goal' | 'dashboard' | 'asset' | 'time' | 'collection';

/** The letter beside each kind, which jumps to it while *Erstellen* is open (C-5). */
export const LETTERS: Readonly<Partial<Record<CreateKind, string>>> = { entry: 'E', page: 'S', whiteboard: 'W', goal: 'Z', dashboard: 'D', asset: 'K', time: 'T' };

/** Page, whiteboard and dashboard open at once and empty; the rest in their own dialog (C-3). */
export const AT_ONCE: ReadonlySet<CreateKind> = new Set(['page', 'whiteboard', 'dashboard']);

/** The tile each kind shows: its product's, which identifies and means nothing (WB-2). */
export const TILES: Readonly<Record<CreateKind, { readonly icon: string; readonly product?: 'pages' | 'whiteboards' | 'goals' | 'assets' | 'time' }>> = {
  entry: { icon: 'square-check' },
  page: { icon: 'book-open', product: 'pages' },
  whiteboard: { icon: 'layout-template', product: 'whiteboards' },
  goal: { icon: 'flag', product: 'goals' },
  dashboard: { icon: 'layout-grid', product: 'goals' },
  asset: { icon: 'shield', product: 'assets' },
  time: { icon: 'clock', product: 'time' },
  collection: { icon: 'copy' },
};

/** The answers the frame gives about where one stands. */
export interface Here {
  readonly mayCapture: boolean;
  /** The hub one stands in; without one, only what needs no hub is offered. */
  readonly hubId?: string;
  /** The modules on at that hub (or in the workspace without one). */
  readonly modulesOn: readonly string[];
  /** The role there writes entries, pages and boards. */
  readonly mayWrite: boolean;
  /** The role there shapes the hub: a collection may be added. */
  readonly mayShape: boolean;
}

/** The kinds one may create here, in the order *Erstellen* draws them. */
export function kindsHere(here: Here): CreateKind[] {
  const on = (module: string) => here.modulesOn.includes(module);
  const inHub = here.hubId !== undefined && here.mayWrite;
  return [
    ...(here.mayCapture ? ['entry' as const] : []),
    ...(inHub && on('KNOWLEDGE') ? ['page' as const] : []),
    ...(inHub && on('WHITEBOARD') ? ['whiteboard' as const] : []),
    ...(inHub && on('GOALS') ? ['goal' as const, 'dashboard' as const] : []),
    ...(inHub && on('CMDB') ? ['asset' as const] : []),
    ...(on('TIME') && here.mayCapture ? ['time' as const] : []),
    ...(here.hubId !== undefined && here.mayShape ? ['collection' as const] : []),
  ];
}

/** The kinds created last that may still be created here, newest first, at most three. */
export function lastHere(last: readonly CreateKind[], here: readonly CreateKind[]): CreateKind[] {
  return last.filter((kind) => here.includes(kind)).slice(0, 3);
}

/** Notes a kind as created last: it moves to the front. */
export function noteLast(last: readonly CreateKind[], kind: CreateKind): CreateKind[] {
  return [kind, ...last.filter((each) => each !== kind)].slice(0, 8);
}

/** The kind a letter jumps to, among those offered here. */
export function kindOfLetter(letter: string, here: readonly CreateKind[]): CreateKind | undefined {
  return here.find((kind) => LETTERS[kind] === letter.toUpperCase());
}
