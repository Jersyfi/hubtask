// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a screen does when its chunk cannot be loaded. After an upgrade the old release's hashed
// chunks are gone, so a tab opened before it fails on the first screen it had not loaded yet:
// that is not the connection, and reloading onto the new release is the remedy. The served
// document's entry script tells the releases apart; a reload already made for that entry is not
// repeated, so a release that is missing a chunk itself cannot loop.

export type ChunkFailure = 'reload' | 'updated' | 'retry' | 'offline';

const ENTRY = /<script[^>]*type="module"[^>]*src="([^"]+)"/;

// entryOf answers the module entry a served document loads, or null.
export function entryOf(html: string): string | null {
  return ENTRY.exec(html)?.[1] ?? null;
}

export function whenAChunkFails(state: {
  online: boolean;
  current: string | null;
  served: string | null;
  reloadedFor: string | null;
}): ChunkFailure {
  if (!state.online) return 'offline';
  if (state.served === null || state.current === null || state.served === state.current) return 'retry';
  return state.reloadedFor === state.served ? 'updated' : 'reload';
}

const RELOADED_KEY = 'hubtask.reloaded-for';

// currentEntry answers the entry script this document runs, as a path.
export function currentEntry(): string | null {
  const script = document.querySelector<HTMLScriptElement>('script[type="module"][src]');
  return script ? new URL(script.src, location.href).pathname : null;
}

// servedEntry reads the document the server answers now, bypassing every cache.
export async function servedEntry(): Promise<string | null> {
  try {
    const response = await fetch('/', { cache: 'no-store', credentials: 'same-origin' });
    return response.ok ? entryOf(await response.text()) : null;
  } catch {
    return null;
  }
}

function reloadedFor(): string | null {
  try {
    return sessionStorage.getItem(RELOADED_KEY);
  } catch {
    return null;
  }
}

// settle decides and, for 'reload', reloads the page at the same address onto the new release.
export async function settle(): Promise<ChunkFailure> {
  const served = await servedEntry();
  const outcome = whenAChunkFails({ online: navigator.onLine, current: currentEntry(), served, reloadedFor: reloadedFor() });
  if (outcome === 'reload' && served !== null) {
    try {
      sessionStorage.setItem(RELOADED_KEY, served);
    } catch {
      // Without the mark a second failure would reload again; 'updated' then needs one more try.
    }
    location.reload();
  }
  return outcome;
}
