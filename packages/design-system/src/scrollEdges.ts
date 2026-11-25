// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Attachment } from 'svelte/attachments';

// The scroll shades of every overlay's list: a shade only at an edge with more content beyond it.
// `scrollEdges.css` draws them from the attributes set here. A script rather than a scroll timeline
// (as `Table` has), because not every engine this product supports has the timeline; without the
// script no shade shows, which is safer than one that says there is more when there is not.

/** Under a pixel is no scroll: a zoomed page rounds the scroll position to a fraction. */
const SLACK = 1;

/** Which edges of a scroll box have more content beyond them. */
export function edgesOf(box: { scrollTop: number; scrollHeight: number; clientHeight: number }): { start: boolean; end: boolean } {
  const room = box.scrollHeight - box.clientHeight;
  if (room <= SLACK) return { start: false, end: false };
  return { start: box.scrollTop > SLACK, end: box.scrollTop < room - SLACK };
}

/**
 * Put on the element that scrolls: marks its parent, the frame that does not scroll, with
 * `data-scroll-edges`, and with `data-more-start` and `data-more-end` while there is more above or
 * below. The frame is where the shades are drawn, over the rows and inside its own rounded corners,
 * so it must have the scroller's box at the edges it shades and be a containing block.
 *
 * Asked again on a scroll, when the frame or the content changes size (window, density, text size)
 * and when rows come or go.
 */
export const scrollEdges: Attachment<HTMLElement> = (scroller) => {
  const frame = scroller.parentElement;
  if (!frame) return;
  frame.setAttribute('data-scroll-edges', '');

  const update = () => {
    const { start, end } = edgesOf(scroller);
    frame.toggleAttribute('data-more-start', start);
    frame.toggleAttribute('data-more-end', end);
  };

  const sizes = new ResizeObserver(update);
  const observeSizes = () => {
    sizes.disconnect();
    sizes.observe(frame);
    sizes.observe(scroller);
    for (const child of scroller.children) sizes.observe(child);
  };
  const rows = new MutationObserver(() => {
    observeSizes();
    update();
  });

  scroller.addEventListener('scroll', update, { passive: true });
  rows.observe(scroller, { childList: true, subtree: true });
  observeSizes();
  update();

  return () => {
    scroller.removeEventListener('scroll', update);
    sizes.disconnect();
    rows.disconnect();
    frame.removeAttribute('data-scroll-edges');
    frame.removeAttribute('data-more-start');
    frame.removeAttribute('data-more-end');
  };
};
