// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Opening a settings screen at one of its rows: the palette's *Language* lands on the language
 * field, not on the top of *You* (UC-WRK-27 check 2).
 *
 * The screen marks the row `data-setting={id}` and knows nothing else; the row is found once it
 * is drawn, because the screen reads the account before it draws, scrolled into view and given
 * focus - its own control where it is one, the first control inside it otherwise, or the row
 * itself where it has none, so that the keyboard is where the eye is.
 */

const CONTROL = 'input:not([type="hidden"]), select, textarea, button, [tabindex]:not([tabindex="-1"])';

/** How long a screen may take to draw the row before the landing gives up and leaves it at the top. */
const PATIENCE_MS = 5_000;

function focusRow(row: HTMLElement): void {
  row.scrollIntoView({ block: 'center' });
  // In a radio group the chosen option is the one the keyboard reaches, so it is the one focused.
  const control = row.matches(CONTROL)
    ? row
    : (row.querySelector<HTMLElement>('input[type="radio"]:checked') ?? row.querySelector<HTMLElement>(CONTROL));
  if (control) {
    control.focus({ preventScroll: true });
    return;
  }
  row.tabIndex = -1;
  row.focus({ preventScroll: true });
}

/** Focuses the row marked `id` once it is drawn. The returned function stops waiting. */
export function land(id: string, root: ParentNode & Node = document.body): () => void {
  const selector = `[data-setting="${CSS.escape(id)}"]`;
  const found = () => root.querySelector<HTMLElement>(selector);
  const now = found();
  if (now) {
    focusRow(now);
    return () => {};
  }
  const observer = new MutationObserver(() => {
    const row = found();
    if (!row) return;
    stop();
    focusRow(row);
  });
  const timer = setTimeout(() => stop(), PATIENCE_MS);
  function stop() {
    observer.disconnect();
    clearTimeout(timer);
  }
  observer.observe(root, { childList: true, subtree: true });
  return stop;
}
