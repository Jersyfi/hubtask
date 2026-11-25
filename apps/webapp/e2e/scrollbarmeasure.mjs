// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The drawn vertical scrollbar of a scroll box, read from the pixels of a screenshot: how wide it
// is, whether its thumb is round, whether its track shows, and the thumb's colour. A browser does
// not report a scrollbar's own style, so the screen is the one witness.

/** The page's own scroll box: the root, whose bar stands at the window's edge. */
export const PAGE = 'page';

/**
 * Where the box's bar stands: its gutter, as the box's border box less its border and its client
 * width. `target` is a locator, an element handle, or `PAGE`.
 */
async function geometry(page, target) {
  const read = (node) => {
    const root = node === null;
    const box = root ? document.documentElement : node;
    const rect = root ? { left: 0, top: 0, right: innerWidth, bottom: innerHeight } : box.getBoundingClientRect();
    const style = getComputedStyle(box);
    const borderEnd = root ? 0 : Number.parseFloat(style.borderRightWidth);
    const borderTop = root ? 0 : Number.parseFloat(style.borderTopWidth);
    const right = rect.right - borderEnd;
    const gutter = root ? innerWidth - box.clientWidth : box.offsetWidth - Number.parseFloat(style.borderLeftWidth) - borderEnd - box.clientWidth;
    return { left: right - gutter, right, top: rect.top + borderTop, height: box.clientHeight };
  };
  return target === PAGE ? page.evaluate(read, null) : target.evaluate(read);
}

/** The colour `token` resolves to inside the box, as red, green and blue. */
export async function tokenColour(page, target, token) {
  const host = target === PAGE ? await page.evaluateHandle(() => document.body) : target.elementHandle ? await target.elementHandle() : target;
  return host.evaluate((node, name) => {
    const probe = document.createElement('span');
    probe.style.color = `var(${name})`;
    node.append(probe);
    const colour = getComputedStyle(probe).color;
    probe.remove();
    return colour.match(/[\d.]+/g).slice(0, 3).map(Number);
  }, token);
}

/** How far apart two colours are: the largest difference of a channel. */
export const distance = (a, b) => Math.max(...a.map((value, i) => Math.abs(value - b[i])));

/**
 * The bar of `target` as drawn now, along its track - the gutter less `sp-100` at either end, where
 * a surface's rounded corners are. The thumb paints the columns that differ from the gutter's last
 * one, which is air between the thumb and the edge. Returned: the gutter's width, how many columns
 * the thumb paints and how far it stands from either edge, its colour in its middle, how much its
 * top corner shows against its middle (a round thumb's corner shows less), and the colour of the
 * track below it beside the colour of the surface left of the gutter. `browser` decodes the
 * screenshot.
 */
export async function measureBar(browser, page, target) {
  const at = await geometry(page, target);
  const inset = Number.parseFloat(await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--sp-100')));
  const png = await page.screenshot();
  const reader = await browser.newPage();
  try {
    return await reader.evaluate(async ([data, g, end]) => {
      const image = new Image();
      image.src = `data:image/png;base64,${data}`;
      await image.decode();
      const canvas = document.createElement('canvas');
      [canvas.width, canvas.height] = [image.width, image.height];
      const context = canvas.getContext('2d');
      context.drawImage(image, 0, 0);
      const px = (x, y) => [...context.getImageData(Math.round(x), Math.round(y), 1, 1).data.slice(0, 3)];
      const far = (a, b) => Math.max(...a.map((value, i) => Math.abs(value - b[i])));
      const [left, right] = [Math.round(g.left), Math.round(g.right)];
      const [top, bottom] = [Math.round(g.top + end), Math.round(g.top + g.height - end)];
      const air = (y) => px(right - 1, y);
      const middle = Math.floor((left + right) / 2);
      const rows = [];
      for (let y = top; y < bottom; y += 1) if (far(px(middle, y), air(y)) > 6) rows.push(y);
      if (rows.length === 0) return { gutter: right - left, drawn: 0, rows: 0 };
      // The thumb is the longest run of such rows.
      let [thumbTop, thumbBottom] = [rows[0], rows[0]];
      for (let i = 0, start = rows[0]; i < rows.length; i += 1) {
        if (i > 0 && rows[i] !== rows[i - 1] + 1) start = rows[i];
        if (rows[i] - start > thumbBottom - thumbTop) [thumbTop, thumbBottom] = [start, rows[i]];
      }
      const centre = Math.floor((thumbTop + thumbBottom) / 2);
      const columns = [];
      for (let x = left; x < right; x += 1) if (far(px(x, centre), air(centre)) > 6) columns.push(x);
      const below = thumbBottom + Math.floor((bottom - thumbBottom) / 2);
      return {
        gutter: right - left,
        drawn: columns.length,
        fromStart: columns[0] - left,
        fromEnd: right - 1 - columns[columns.length - 1],
        rows: thumbBottom - thumbTop + 1,
        stray: rows.length - (thumbBottom - thumbTop + 1),
        corner: far(px(columns[0], thumbTop), air(thumbTop)),
        body: far(px(middle, centre), air(centre)),
        thumb: px(middle, centre),
        track: below < bottom - 1 && below > thumbBottom + 1 ? { colour: px(middle, below), beside: px(left - 1, below) } : undefined,
      };
    }, [png.toString('base64'), at, inset]);
  } finally {
    await reader.close();
  }
}
