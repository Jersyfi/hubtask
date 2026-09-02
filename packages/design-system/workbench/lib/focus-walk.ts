// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Rule 5, made observable: "Focus is always visible. The app is fully operable by keyboard or it
// is not."
//
// Two things go wrong with keyboard operation and neither is visible at rest. The ring can be
// missing or clipped on one control out of eight, and the order can be a different order from the
// one the layout implies - which is what happens the first time a component is positioned rather
// than laid out. The walk shows both: it steps focus through the pane in tab order and reports
// the sequence, so a reader compares a list against what they see.
//
// It computes the order from the DOM rather than dispatching Tab keys, because a synthetic Tab
// does not move focus in any browser - only the browser's own key handling does, and it is not
// scriptable. That is a real limitation and the honest consequence is stated: this finds an order
// that disagrees with the layout, and it does not find a focus trap. A trap needs a driven
// browser, which is F5's decision (ADR-0037).

const FOCUSABLE = [
  'a[href]',
  'button',
  'input',
  'select',
  'textarea',
  'summary',
  'audio[controls]',
  'video[controls]',
  '[contenteditable]:not([contenteditable="false"])',
  '[tabindex]',
].join(',');

export interface Stop {
  readonly index: number;
  /** What a person would call this control: its accessible name, or failing that its shape. */
  readonly label: string;
  readonly element: HTMLElement;
  /** A positive tabindex is a defect by itself - it makes the order depend on a number. */
  readonly positiveTabIndex: boolean;
}

function accessibleName(element: HTMLElement): string {
  const aria = element.getAttribute('aria-label');
  if (aria) return aria;
  const labelledBy = element.getAttribute('aria-labelledby');
  if (labelledBy) {
    const parts = labelledBy
      .split(/\s+/)
      .map((id) => element.ownerDocument.getElementById(id)?.textContent?.trim())
      .filter(Boolean);
    if (parts.length > 0) return parts.join(' ');
  }
  if (element instanceof HTMLInputElement && element.labels?.length) {
    const text = element.labels[0]?.textContent?.trim();
    if (text) return text;
  }
  const text = element.textContent?.trim();
  if (text) return text.length > 48 ? `${text.slice(0, 48)}…` : text;
  return `<${element.tagName.toLowerCase()}> with no accessible name`;
}

const isVisible = (element: HTMLElement) =>
  element.offsetParent !== null || element.getClientRects().length > 0;

/**
 * The stops in tab order: document order for tabindex 0 and native focusables, and ascending
 * tabindex before all of them, which is what the browser does and what makes a positive tabindex
 * worth reporting.
 */
export function tabOrder(root: HTMLElement): Stop[] {
  const candidates = [...root.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((element) => {
    if (element.hasAttribute('disabled') || element.getAttribute('aria-hidden') === 'true') return false;
    if (element.tabIndex < 0) return false;
    return isVisible(element);
  });

  const positive = candidates.filter((element) => element.tabIndex > 0);
  positive.sort((a, b) => a.tabIndex - b.tabIndex);
  const natural = candidates.filter((element) => element.tabIndex === 0);

  return [...positive, ...natural].map((element, index) => ({
    index: index + 1,
    label: accessibleName(element),
    element,
    positiveTabIndex: element.tabIndex > 0,
  }));
}

/**
 * Walks the stops, leaving focus on each long enough to see the ring. Returns a function that
 * stops it, because a walk that cannot be interrupted is one nobody starts twice.
 */
export function walk(stops: readonly Stop[], onStep: (stop: Stop | null) => void, stepMs = 420): () => void {
  let index = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  const step = () => {
    const stop = stops[index];
    if (!stop) {
      onStep(null);
      return;
    }
    // `focusVisible` where the browser has it. Where it does not, :focus-visible still matches
    // when the last interaction was a key press - which is why the panel says to start the walk
    // from the keyboard. A ring the workbench drew itself would prove nothing about the
    // component's own ring, so nothing here draws one.
    stop.element.focus({ focusVisible: true });
    onStep(stop);
    index += 1;
    timer = setTimeout(step, stepMs);
  };

  step();
  return () => {
    if (timer !== undefined) clearTimeout(timer);
    onStep(null);
  };
}
