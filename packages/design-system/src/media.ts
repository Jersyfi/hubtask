// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { createSubscriber } from 'svelte/reactivity';

// Whether the window is a phone's, asked at the moment something opens - a status's steps as a
// sheet rather than a popover (work-surface concept §7). Layout itself stays in media queries; this
// answers only the question a stylesheet cannot: which surface to open.

/** Below `primitive.breakpoint.medium`: the compact width, where a menu becomes a sheet. */
// design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query string cannot read a custom property.
const COMPACT = '(width < 600px)';

export function isCompactWidth(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia(COMPACT).matches;
}

/** Calls back whenever the window crosses into or out of the compact width; returns the stop. */
export function watchCompactWidth(onchange: (isCompact: boolean) => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return () => {};
  const query = window.matchMedia(COMPACT);
  const listener = () => onchange(query.matches);
  query.addEventListener('change', listener);
  return () => query.removeEventListener('change', listener);
}

// The one name of every overlay's close control - a dialog's ✕, a sheet's ✕, a menu become a sheet
// (work-surface concept §7). The application gives it once, in its language, so that no caller
// hands down a word of its own and the ✕ is not *Cancel* here and *Close* there. Text is the
// caller's (ADR-0011).
// Read reactively: an overlay drawn before the language arrived, or kept in the page while closed,
// takes the name once it is given or changes.
let closeLabelName: string | undefined;
let notify: (() => void) | undefined;
const follow = createSubscriber((update) => {
  notify = update;
  return () => (notify = undefined);
});

export function setCloseLabel(label: string | undefined): void {
  if (label === closeLabelName) return;
  closeLabelName = label;
  notify?.();
}

export function closeName(): string | undefined {
  follow();
  return closeLabelName;
}

// The name of every `HelpTip`'s ⓘ - "More about Expires" - given once by the application in its
// language, as the close's name is, so no caller writes the words and no ⓘ is left without a name.
// A function of the subject, called where the name is read: its words follow the language there.
let helpNameOf: ((subject: string) => string) | undefined;

export function setHelpName(name: ((subject: string) => string) | undefined): void {
  if (name === helpNameOf) return;
  helpNameOf = name;
  notify?.();
}

export function helpName(subject: string): string | undefined {
  follow();
  return helpNameOf?.(subject);
}
