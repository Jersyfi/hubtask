// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The area a press lands on a control, as a finger finds it: from the control's centre outwards,
 * each pixel the topmost element there is the control (or inside it) counts - a hit area drawn by
 * a pseudo-element included, a neighbour drawn over it excluded. The drawn box says nothing of it.
 * Answers `{ width, height }` in CSS px, with the control scrolled into view; a control at the end
 * of its scroller is measured with the scroller at its end, as a person scrolls to press it.
 */
export async function pressArea(locator) {
  await locator.scrollIntoViewIfNeeded();
  return locator.evaluate((element) => {
    for (let scroller = element.parentElement; scroller; scroller = scroller.parentElement) {
      if (!['auto', 'scroll'].includes(getComputedStyle(scroller).overflowY)) continue;
      if (scroller.getBoundingClientRect().bottom - element.getBoundingClientRect().bottom < element.offsetHeight) scroller.scrollTop = scroller.scrollHeight;
      break;
    }
    const box = element.getBoundingClientRect();
    const x = box.left + box.width / 2;
    const y = box.top + box.height / 2;
    const reach = (dx, dy) => {
      let steps = 0;
      for (; steps < 64; steps++) {
        const hit = document.elementFromPoint(x + dx * steps, y + dy * steps);
        if (!hit || !element.contains(hit)) break;
      }
      return steps;
    };
    return { width: reach(-1, 0) + reach(1, 0) - 1, height: reach(0, -1) + reach(0, 1) - 1 };
  });
}
