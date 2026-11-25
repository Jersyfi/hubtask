// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { intentOf } from './reason.ts';

// What scrollbar.css needs from script: whether the system draws overlay scrollbars, and which
// surface scrolls now. Called once by each client's entry.

/** Whether the system's scrollbars overlay the content: a box that always scrolls keeps all its width. */
function probe(root: HTMLElement): void {
  const box = document.createElement('div');
  box.setAttribute('data-scrollbar-probe', '');
  document.body.append(box);
  const overlay = box.offsetWidth - box.clientWidth === 0;
  box.remove();
  if (overlay) root.dataset.scrollbars = 'overlay';
  else delete root.dataset.scrollbars;
}

/**
 * Asks the system whether its scrollbars overlay the content (macOS with a trackpad, a phone).
 * Such a system keeps its own bar: `data-scrollbars='overlay'` on the root turns the product's rule
 * off. It asks again when the window is resized, shown or focused, so a switch between overlay
 * and classic bars applies without a reload. While a surface scrolls, and for `--dur-intent`
 * after, it carries `data-scrolling`, which draws its thumb in full. Returns the stop, which
 * removes the listeners, the timers and both attributes.
 */
export function watchScrollbars(root: HTMLElement = document.documentElement): () => void {
  const ask = () => probe(root);
  const askWhenShown = () => {
    if (document.visibilityState === 'visible') ask();
  };
  ask();

  const settling = new Map<Element, ReturnType<typeof setTimeout>>();
  const settle = (surface: Element) => {
    settling.delete(surface);
    surface.removeAttribute('data-scrolling');
  };
  const onScroll = (event: Event) => {
    const surface = event.target instanceof Element ? event.target : root;
    clearTimeout(settling.get(surface));
    if (!surface.hasAttribute('data-scrolling')) surface.setAttribute('data-scrolling', '');
    settling.set(surface, setTimeout(() => settle(surface), intentOf(surface)));
  };

  document.addEventListener('scroll', onScroll, { capture: true, passive: true });
  window.addEventListener('resize', ask);
  window.addEventListener('focus', ask);
  document.addEventListener('visibilitychange', askWhenShown);
  return () => {
    document.removeEventListener('scroll', onScroll, { capture: true });
    window.removeEventListener('resize', ask);
    window.removeEventListener('focus', ask);
    document.removeEventListener('visibilitychange', askWhenShown);
    for (const [surface, timer] of settling) {
      clearTimeout(timer);
      settle(surface);
    }
    delete root.dataset.scrollbars;
  };
}
