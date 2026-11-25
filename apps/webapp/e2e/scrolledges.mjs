// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a scroll box's shades are (the design system's scrollEdges): the frame's attributes, and
// each shade as the engine paints it - whether it shows, its box and its corners - beside the
// rounded rectangle of the surface behind it. Shared by the walks that hold the shades.

/**
 * Reads the shades of the box `scroller` (a locator of the element that scrolls). `start` and
 * `end` are the frame's attributes; `shades.start` and `shades.end` what is painted at each edge;
 * `outside` names every painted shade corner that stands past the surface's rounded rectangle.
 */
export function readEdges(scroller) {
  return scroller.evaluate((node) => {
    const frame = node.parentElement;
    const px = (value) => Number.parseFloat(value) || 0;
    // The surface behind the shades: the frame itself, or the nearest box under it with a colour.
    let surface = frame;
    while (surface && getComputedStyle(surface).backgroundColor === 'rgba(0, 0, 0, 0)') surface = surface.parentElement;

    /** An element's padding box and the radii of its inner corners: what its background fills. */
    const paddingBox = (element) => {
      const style = getComputedStyle(element);
      const rect = element.getBoundingClientRect();
      const [top, right, bottom, left] = ['Top', 'Right', 'Bottom', 'Left'].map((side) => px(style[`border${side}Width`]));
      const box = { x0: rect.left + left, y0: rect.top + top, x1: rect.right - right, y1: rect.bottom - bottom };
      const outer = ['TopLeft', 'TopRight', 'BottomRight', 'BottomLeft'].map((corner) => px(style[`border${corner}Radius`]));
      return { box, radii: [outer[0] - Math.max(top, left), outer[1] - Math.max(top, right), outer[2] - Math.max(bottom, right), outer[3] - Math.max(bottom, left)].map((each) => Math.max(0, each)) };
    };

    /** Whether a point lies inside a rounded rectangle, half a pixel of rounding allowed. */
    const inside = ({ box, radii }, x, y) => {
      const slack = 0.5;
      if (x < box.x0 - slack || x > box.x1 + slack || y < box.y0 - slack || y > box.y1 + slack) return false;
      const centres = [
        [box.x0 + radii[0], box.y0 + radii[0], radii[0], x < box.x0 + radii[0] && y < box.y0 + radii[0]],
        [box.x1 - radii[1], box.y0 + radii[1], radii[1], x > box.x1 - radii[1] && y < box.y0 + radii[1]],
        [box.x1 - radii[2], box.y1 - radii[2], radii[2], x > box.x1 - radii[2] && y > box.y1 - radii[2]],
        [box.x0 + radii[3], box.y1 - radii[3], radii[3], x < box.x0 + radii[3] && y > box.y1 - radii[3]],
      ];
      return centres.every(([cx, cy, r, isInCorner]) => !isInCorner || Math.hypot(x - cx, y - cy) <= r + slack);
    };

    const behind = paddingBox(surface);
    const origin = paddingBox(frame).box;
    const shadeOf = (which, edge) => {
      const style = getComputedStyle(frame, which);
      const box = { x0: origin.x0 + px(style.left), y0: origin.y0 + px(style.top) };
      box.x1 = box.x0 + px(style.width);
      box.y1 = box.y0 + px(style.height);
      const radii = [style.borderTopLeftRadius, style.borderTopRightRadius, style.borderBottomRightRadius, style.borderBottomLeftRadius].map(px)
        .map((each) => Math.min(each, (box.x1 - box.x0) / 2, (box.y1 - box.y0) / 2));
      const strip = px(style.backgroundSize.split(' ')[1]);
      // The painted strip: the shade's top or bottom part, cut to its own rounded corners.
      const painted = edge === 'start' ? { x0: box.x0, y0: box.y0, x1: box.x1, y1: box.y0 + strip } : { x0: box.x0, y0: box.y1 - strip, x1: box.x1, y1: box.y1 };
      // Every point of the shade's outline in the strip, its corners' arcs sampled degree by degree.
      const outside = [];
      const corners = edge === 'start' ? [0, 1] : [2, 3];
      for (const corner of corners) {
        const r = radii[corner];
        const cx = corner === 0 || corner === 3 ? box.x0 + r : box.x1 - r;
        const cy = corner === 0 || corner === 1 ? box.y0 + r : box.y1 - r;
        for (let degree = 0; degree <= 90; degree++) {
          const angle = (degree * Math.PI) / 180;
          const x = cx + (corner === 0 || corner === 3 ? -1 : 1) * r * Math.cos(angle);
          const y = cy + (corner === 0 || corner === 1 ? -1 : 1) * r * Math.sin(angle);
          if (!inside(behind, x, y)) outside.push(`${edge} corner ${corner} at ${x.toFixed(1)},${y.toFixed(1)}`);
        }
      }
      return {
        isPainted: Number(style.opacity) > 0 && style.backgroundImage !== 'none' && strip > 0 && style.content !== 'none',
        isPositioned: style.position === 'absolute',
        zIndex: style.zIndex,
        box: painted,
        radii,
        outside,
      };
    };

    return {
      isFrame: frame.hasAttribute('data-scroll-edges'),
      start: frame.hasAttribute('data-more-start'),
      end: frame.hasAttribute('data-more-end'),
      scrolls: node.scrollHeight > node.clientHeight + 1,
      surfaceRadii: behind.radii,
      shades: { start: shadeOf('::before', 'start'), end: shadeOf('::after', 'end') },
    };
  });
}

/** Scrolls the box to a share of its room - 0 its top, 1 its end - and waits for the frame to say so. */
export async function scrollToShare(scroller, share) {
  await scroller.evaluate((node, at) => {
    node.scrollTop = Math.round((node.scrollHeight - node.clientHeight) * at);
  }, share);
  await scroller.evaluate((node, at) => new Promise((done) => {
    const frame = node.parentElement;
    const wanted = () => frame.hasAttribute('data-more-start') === at > 0 && frame.hasAttribute('data-more-end') === at < 1;
    if (wanted()) return done();
    const watch = new MutationObserver(() => { if (wanted()) { watch.disconnect(); done(); } });
    watch.observe(frame, { attributes: true });
  }), share);
}

/**
 * The pixels of a rectangle of the page, with a shade shown and with it taken away: whether the
 * shade changes them. The frame's attribute is taken away and put back by hand, as nothing scrolls
 * in between.
 */
export async function shadeChanges(page, scroller, edge, clip) {
  const attribute = edge === 'start' ? 'data-more-start' : 'data-more-end';
  const shown = await page.screenshot({ clip });
  await scroller.evaluate((node, name) => node.parentElement.removeAttribute(name), attribute);
  const hidden = await page.screenshot({ clip });
  await scroller.evaluate((node, name) => node.parentElement.setAttribute(name, ''), attribute);
  return !shown.equals(hidden);
}

/**
 * How much a shade must darken the strip at its edge, in either theme: about what a light surface
 * shows, so that the shade is seen on a dark one too and is not only measurable there.
 */
export const SHADE_DEPTH = 0.04;

/**
 * The mean lightness of a rectangle of the page with a shade shown and with it taken away, 0 black
 * and 1 white: a shade that darkens the surface leaves `shown` below `hidden`, in either theme.
 * The pixels are read in a blank page of the same browser, so no page's own policy refuses the
 * image they are decoded from.
 */
export async function shadeLightness(page, scroller, edge, clip) {
  const [shown, hidden] = await rowLightness(page, scroller, edge, clip);
  const mean = (rows) => rows.reduce((sum, each) => sum + each, 0) / rows.length;
  return { shown: mean(shown), hidden: mean(hidden) };
}

/** The mean lightness of each row of pixels of a rectangle, with the shade shown and taken away. */
async function rowLightness(page, scroller, edge, clip) {
  const attribute = edge === 'start' ? 'data-more-start' : 'data-more-end';
  const shown = await page.screenshot({ clip });
  await scroller.evaluate((node, name) => node.parentElement.removeAttribute(name), attribute);
  const hidden = await page.screenshot({ clip });
  await scroller.evaluate((node, name) => node.parentElement.setAttribute(name, ''), attribute);
  const blank = await page.context().browser().newPage();
  try {
    return await blank.evaluate(async (images) => Promise.all(images.map(async (base64) => {
      const bytes = Uint8Array.from(atob(base64), (each) => each.charCodeAt(0));
      const bitmap = await createImageBitmap(new Blob([bytes], { type: 'image/png' }));
      const canvas = new OffscreenCanvas(bitmap.width, bitmap.height);
      const context = canvas.getContext('2d');
      context.drawImage(bitmap, 0, 0);
      const { data } = context.getImageData(0, 0, bitmap.width, bitmap.height);
      const rows = [];
      for (let y = 0; y < bitmap.height; y++) {
        let sum = 0;
        for (let x = 0; x < bitmap.width; x++) {
          const at = (y * bitmap.width + x) * 4;
          sum += (0.2126 * data[at] + 0.7152 * data[at + 1] + 0.0722 * data[at + 2]) / 255;
        }
        rows.push(sum / bitmap.width);
      }
      return rows;
    })), [shown.toString('base64'), hidden.toString('base64')]);
  } finally {
    await blank.close();
  }
}

/**
 * How much of each row of pixels a shade covers, from the surface's inner edge inward, the first row
 * the one against the surface's border. A soft fade is darkest there and lightens row by row; a
 * strip the shade leaves untouched between it and the border shows as a line where it begins.
 */
export async function shadeProfile(page, scroller, edge, rows = 12) {
  const inner = await scroller.evaluate((node) => {
    let surface = node.parentElement;
    while (surface && getComputedStyle(surface).backgroundColor === 'rgba(0, 0, 0, 0)') surface = surface.parentElement;
    const rect = surface.getBoundingClientRect();
    const style = getComputedStyle(surface);
    return {
      x: rect.left + rect.width / 4,
      width: rect.width / 2,
      top: rect.top + Number.parseFloat(style.borderTopWidth),
      bottom: rect.bottom - Number.parseFloat(style.borderBottomWidth),
    };
  });
  const y = edge === 'start' ? Math.ceil(inner.top) : Math.floor(inner.bottom) - rows;
  const [shown, hidden] = await rowLightness(page, scroller, edge, { x: inner.x, y, width: inner.width, height: rows });
  // The share covered rather than the lightness lost: alike over bare surface, a lit row and text.
  const depth = hidden.map((each, at) => 1 - shown[at] / each);
  return edge === 'start' ? depth : depth.reverse();
}

/** The rows of a shade's profile darker than the row before them, nearer the border: an edge. */
export function edgesInProfile(depth, slack = 0.03) {
  const found = [];
  for (let at = 1; at < depth.length; at++) {
    if (depth[at] > depth[at - 1] + slack) found.push(`row ${at}: ${depth[at].toFixed(3)} after ${depth[at - 1].toFixed(3)}`);
  }
  return found;
}

/** Asserts a shade is a soft fade from the surface's border inward, with no edge of its own. */
export async function assertSoftFade(assert, page, scroller, edge, where) {
  const depth = await shadeProfile(page, scroller, edge);
  const shown = depth.map((each) => each.toFixed(3)).join(' ');
  assert.ok(depth[0] > 0, `${where}: the shade at the ${edge} leaves the row against the surface's border untouched (${shown})`);
  assert.deepEqual(edgesInProfile(depth), [], `${where}: the shade at the ${edge} has an edge of its own (${shown})`);
}

/** The frame's outer box: where its corners and its edges are, for a clip. */
export function frameBox(scroller) {
  return scroller.evaluate((node) => {
    const rect = node.parentElement.getBoundingClientRect();
    return { x: rect.left, y: rect.top, width: rect.width, height: rect.height };
  });
}

/**
 * Asserts the shades a box shows: `start` and `end` as the frame says and as the engine paints,
 * each painted one over the rows and inside the surface's rounded corners.
 */
export function assertShades(assert, read, want, where) {
  assert.equal(read.isFrame, true, `${where}: the scroll box has no frame for its shades`);
  assert.equal(read.start, want.start, `${where}: the frame says ${read.start ? 'more' : 'nothing'} above`);
  assert.equal(read.end, want.end, `${where}: the frame says ${read.end ? 'more' : 'nothing'} below`);
  for (const edge of ['start', 'end']) {
    const shade = read.shades[edge];
    assert.equal(shade.isPainted, want[edge], `${where}: a shade at the ${edge} is ${shade.isPainted ? '' : 'not '}painted`);
    if (!shade.isPainted) continue;
    assert.equal(shade.isPositioned, true, `${where}: the ${edge} shade is not laid over the rows`);
    assert.ok(shade.box.y1 > shade.box.y0, `${where}: the ${edge} shade has no height`);
    assert.deepEqual(shade.outside, [], `${where}: the ${edge} shade stands past the surface's rounded corners (radii ${read.surfaceRadii.join(', ')})`);
  }
}
