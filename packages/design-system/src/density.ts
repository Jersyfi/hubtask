// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A list's density, comfortable or compact, remembered on the device (WB-4, design-system.md
// §11.10): a property of the device like the theme, not of the account. The storage is handed in,
// so the client decides where the device keeps it and a test can hand in a map.

/** The two densities a list offers; `spacious` stays a region's choice, not a list's. */
export type ListDensity = 'comfortable' | 'compact';

/** The part of `Storage` this needs - and all `localStorage` may be when it throws. */
export interface DensityStore {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

const PREFIX = 'hubtask.density.';

/** The density a list was left at on this device, or `comfortable` where nothing is kept. */
export function readDensity(store: DensityStore | undefined, list: string): ListDensity {
  try {
    return store?.getItem(PREFIX + list) === 'compact' ? 'compact' : 'comfortable';
  } catch {
    // Private windows and full quotas throw; a list still has a density.
    return 'comfortable';
  }
}

/** Keeps the density on this device; a store that refuses leaves the list as it is now. */
export function writeDensity(store: DensityStore | undefined, list: string, density: ListDensity): void {
  try {
    store?.setItem(PREFIX + list, density);
  } catch {
    // As above: the choice holds for this visit.
  }
}
