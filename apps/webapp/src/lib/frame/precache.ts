// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The screens are their own chunks (F-10), fetched when their route is first reached. Offline
// that is too late: the copy holds a hub's pages and boards, but a screen this tab never opened
// has no code to draw them (offline-sync.md §14). So once a signed-in session is idle every
// screen's chunk the reader can open is fetched, one at a time so the work in front is not slowed;
// the module system keeps them for this tab, and their immutable caching for the next.

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

/** Where a signed-in session stands for the precache: whether it is, and what the manifest says. */
export type Reach = { readonly signedIn: boolean; readonly administration: boolean | undefined };

/**
 * precacheByReach is the precache of one tab: every screen but the administration area once
 * somebody is signed in, and that area once the manifest says they reach it - at the first read
 * or after a role granted later - each once, the second after the first. Somebody who cannot open
 * the area never fetches its chunk, which is the largest there is.
 */
export function precacheByReach(
  views: Readonly<Record<string, Loader>>,
  parts: Loader[],
  conditions: Conditions = browser,
): (reach: Reach) => void {
  const { AdministrationArea: administration, ...everyone } = views;
  let queue = Promise.resolve();
  let first = false;
  let area = administration === undefined;
  const after = (loaders: Loader[]) => {
    queue = queue.then(() => precacheWhenIdle(loaders, conditions));
  };
  return ({ signedIn, administration: reaches }) => {
    if (!signedIn) return;
    if (!first) {
      first = true;
      after([...Object.values(everyone), ...parts]);
    }
    if (!area && reaches === true && administration) {
      area = true;
      after([administration]);
    }
  };
}
