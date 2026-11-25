// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { intentOf } from './reason.ts';

// What scrollbar.css needs from script: whether the system draws overlay scrollbars, and which
// surface scrolls now. Called once by each client's entry.

/**
 * Asks the system whether its scrollbars overlay the content (macOS with a trackpad, a phone): a
 * box that always scrolls takes no room for its bar then. Such a system keeps its own bar -
 * `data-scrollbars='overlay'` on the root turns the product's rule off. While a surface scrolls,
 * and for `--dur-intent` after, it carries `data-scrolling`, which draws its thumb in full.
 * Returns the stop.
 */
export function watchScrollbars(root: HTMLElement = document.documentElement): () => void {
  const probe = document.createElement('div');
  probe.setAttribute('data-scrollbar-probe', '');
  document.body.append(probe);
  const overlay = probe.offsetWidth - probe.clientWidth === 0;
  probe.remove();
  if (overlay) root.dataset.scrollbars = 'overlay';
  else delete root.dataset.scrollbars;

  const settling = new WeakMap<Element, ReturnType<typeof setTimeout>>();
  const onScroll = (event: Event) => {
    const surface = event.target instanceof Element ? event.target : root;
    clearTimeout(settling.get(surface));
    if (!surface.hasAttribute('data-scrolling')) surface.setAttribute('data-scrolling', '');
    settling.set(surface, setTimeout(() => surface.removeAttribute('data-scrolling'), intentOf(surface)));
  };
  document.addEventListener('scroll', onScroll, { capture: true, passive: true });
  return () => document.removeEventListener('scroll', onScroll, { capture: true });
}
