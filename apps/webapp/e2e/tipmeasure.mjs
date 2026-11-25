// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the walks of a surface with a tip measure - a reason bubble, a tooltip, a help tip - and
// what they hold it to (design-system.md §4, "Where a bubble stands"). The tip is read where the
// browser drew it: the bubble's `::before` by its resolved box, the help tip's own element by its
// box, never from the numbers that placed it.

import assert from 'node:assert/strict';

/**
 * The surface in `locator`, measured in the page: its box, its widest border, its corner radius,
 * the side its tip points to, the tip's centre and how far it reaches along the edge, how many
 * lines its words take, and the window it stands in.
 */
export function measureTip(locator) {
  return locator.evaluate((node) => {
    const box = node.getBoundingClientRect();
    const style = getComputedStyle(node);
    // Under CSS zoom (the workbench's zoom axis) a box is measured zoomed and a computed length is
    // not: lengths are taken into the window's pixels.
    const zoom = node.currentCSSZoom ?? 1;
    const px = (value) => (Number.parseFloat(value) || 0) * zoom;
    let centre;
    let reach;
    const part = node.querySelector(':scope > [aria-hidden="true"]:empty');
    if (part) {
      // The help tip's tip, a turned square: its box is what it covers.
      const tip = part.getBoundingClientRect();
      centre = { x: tip.left + tip.width / 2, y: tip.top + tip.height / 2 };
      reach = tip.width / 2;
    } else {
      // The bubble's, its `::before`: resolved offsets from the padding box, turned by 45°.
      const before = getComputedStyle(node, '::before');
      const size = px(before.width);
      centre = { x: box.left + node.clientLeft * zoom + px(before.left) + size / 2, y: box.top + node.clientTop * zoom + px(before.top) + size / 2 };
      reach = (size * Math.SQRT2) / 2;
    }
    const words = node.querySelector('.text, .words') ?? node;
    const lineHeight = px(getComputedStyle(words).lineHeight);
    return {
      box: { left: box.left, top: box.top, right: box.right, bottom: box.bottom, width: box.width, height: box.height },
      border: Math.max(px(style.borderTopWidth), px(style.borderRightWidth), px(style.borderBottomWidth), px(style.borderLeftWidth)) / zoom,
      radius: px(style.borderTopLeftRadius),
      side: node.dataset.hbtTip,
      zoom,
      centre,
      reach,
      lines: lineHeight > 0 ? Math.round(words.getBoundingClientRect().height / lineHeight) : 0,
      window: { width: document.documentElement.clientWidth, height: document.documentElement.clientHeight },
    };
  });
}

/** The middle of an element's box. */
export async function middleOf(locator) {
  const box = await locator.boundingBox();
  return { x: box.x + box.width / 2, y: box.y + box.height / 2, box };
}

/**
 * Holds a measured surface to the rules: inside the window, a border no wider than a hairline,
 * its tip on the edge that faces `aim` and within a pixel of `at` (where it points), and the tip's
 * outer extent outside the rounded corners of that edge.
 *
 * With `mayStop`, an element nearer the window's edge than the tip can reach is allowed for, as
 * the rule says: then the surface stands `nearEdge` from the window's edge on that side and the tip
 * at the end of its straight edge, as near the element as it can come.
 */
export function assertTip(measured, at, where, { side, mayStop = false, nearEdge = 8 * (measured.zoom ?? 1) } = {}) {
  const { box, centre, reach, radius } = measured;
  assert.ok(box.left >= 0 && box.top >= 0 && box.right <= measured.window.width + 0.5 && box.bottom <= measured.window.height + 0.5, `${where}: it stands outside the window (${JSON.stringify(box)})`);
  assert.ok(measured.border <= 1, `${where}: its border is ${measured.border} px, wider than a hairline`);
  if (side) assert.equal(measured.side, side, `${where}: the tip points ${measured.side}, not ${side}`);
  const isAcross = measured.side === 'top' || measured.side === 'bottom';
  const edge = { top: box.top, bottom: box.bottom, left: box.left, right: box.right }[measured.side];
  assert.ok(edge !== undefined, `${where}: no tip`);
  assert.ok(Math.abs((isAcross ? centre.y : centre.x) - edge) <= 1, `${where}: the tip is not on the ${measured.side} edge`);
  const along = isAcross ? centre.x : centre.y;
  const aimed = isAcross ? at.x : at.y;
  const start = isAcross ? box.left : box.top;
  const end = isAcross ? box.right : box.bottom;
  const span = isAcross ? measured.window.width : measured.window.height;
  if (mayStop && Math.abs(along - aimed) > 1) {
    const towardsEnd = aimed > along;
    const gap = towardsEnd ? span - end : start;
    assert.ok(Math.abs(gap - nearEdge) <= 0.5, `${where}: the tip stops short of its element (${along.toFixed(1)} for ${aimed.toFixed(1)}) while the surface is ${gap} px from the window's edge, not ${nearEdge}`);
    const left = towardsEnd ? end - (along + reach) : along - reach - start;
    assert.ok(Math.abs(left - measured.radius) <= 0.5, `${where}: the tip stops short of its element but not at the end of the straight edge`);
  } else {
    assert.ok(Math.abs(along - aimed) <= 1, `${where}: the tip points at ${along.toFixed(1)}, its element's middle is ${aimed.toFixed(1)}`);
  }
  assert.ok(along - reach >= start + radius - 0.5, `${where}: the tip enters the rounded corner at its edge's start (${(along - reach - start).toFixed(1)} px from it, radius ${radius})`);
  assert.ok(along + reach <= end - radius + 0.5, `${where}: the tip enters the rounded corner at its edge's end (${(end - along - reach).toFixed(1)} px from it, radius ${radius})`);
}
