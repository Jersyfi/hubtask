// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which layout a person last chose for a collection, on this device (UC-WRK-28 check 6,
 * design-system.md §11.10).
 *
 * A device convenience, so it lives in the browser and is sent nowhere; per account as well as per
 * collection, because a shared browser is two people's device and each chose their own. Every read
 * and write falls back in silence where storage is refused. A remembered layout the collection no
 * longer offers - its structure narrowed, the client cannot draw it - is not used: the collection's
 * own default is, and then the list.
 */

export interface LayoutStore {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

export const layoutKey = (accountId: string, collectionId: string): string =>
  `hubtask.layout.${accountId}.${collectionId}`;

/** The browser's store, or none where reading it throws (a private window, blocked site data). */
export function deviceStore(): LayoutStore | undefined {
  try {
    return globalThis.localStorage ?? undefined;
  } catch {
    return undefined;
  }
}

/**
 * The layout a collection opens with: the remembered one where it is offered and drawable, else the
 * collection's default where that is, else the first drawable one.
 */
export function openingLayout(options: {
  readonly store: LayoutStore | undefined;
  readonly accountId: string | undefined;
  readonly collectionId: string;
  /** What the collection offers (`structure.views.offered`); empty when the read carried none. */
  readonly offered: readonly string[];
  /** What the collection opens with (`structure.views.default`), if the read carried one. */
  readonly preferred: string | undefined;
  /** What this client can draw. */
  readonly drawable: readonly string[];
}): string {
  const usable = (layout: string | null | undefined): layout is string =>
    typeof layout === 'string' &&
    options.drawable.includes(layout) &&
    (options.offered.length === 0 || options.offered.includes(layout));

  let remembered: string | null = null;
  if (options.store && options.accountId) {
    try {
      remembered = options.store.getItem(layoutKey(options.accountId, options.collectionId));
    } catch {
      remembered = null;
    }
  }
  if (usable(remembered)) return remembered;
  if (usable(options.preferred)) return options.preferred;
  return options.drawable.find((layout) => usable(layout)) ?? options.drawable[0] ?? 'LIST_COLLAPSED';
}

/** Keeps the person's choice for this collection. Silent where storage is refused. */
export function rememberLayout(
  store: LayoutStore | undefined,
  accountId: string | undefined,
  collectionId: string,
  layout: string,
): void {
  if (!store || !accountId) return;
  try {
    store.setItem(layoutKey(accountId, collectionId), layout);
  } catch {
    // A browser that refuses storage keeps the choice for as long as the screen is open.
  }
}
