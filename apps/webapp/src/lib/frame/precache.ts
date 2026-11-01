// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The screens are their own chunks (F-10), fetched when their route is first reached. Offline
// that is too late: the copy holds a hub's pages and boards, but a screen this tab never opened
// has no code to draw them (offline-sync.md §13). So once a signed-in session is idle every
// screen's chunk is fetched, one at a time so the work in front is not slowed; the module system
// keeps them for this tab, and their immutable caching for the next.

type Loader = () => Promise<unknown>;

type Conditions = {
  idle: (run: () => void) => () => void;
  online: () => boolean;
  saveData?: () => boolean;
};

const browser: Conditions = {
  idle: (run) => {
    if (typeof requestIdleCallback === 'function') {
      const handle = requestIdleCallback(run, { timeout: 10_000 });
      return () => cancelIdleCallback(handle);
    }
    const handle = setTimeout(run, 2_000);
    return () => clearTimeout(handle);
  },
  online: () => navigator.onLine,
  saveData: () => (navigator as Navigator & { connection?: { saveData?: boolean } }).connection?.saveData === true,
};

// precacheWhenIdle fetches every loader once the page is idle; it settles when all are done.
export function precacheWhenIdle(loaders: Loader[], conditions: Conditions = browser): Promise<void> {
  return new Promise((resolve) => {
    conditions.idle(() => {
      void (async () => {
        if (!conditions.online() || conditions.saveData?.()) return resolve();
        for (const load of loaders) {
          if (!conditions.online()) break;
          try {
            await load();
          } catch {
            // A chunk that cannot be fetched now is fetched when its route is reached.
          }
        }
        resolve();
      })();
    });
  });
}
