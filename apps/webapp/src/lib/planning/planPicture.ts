// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The plan as a picture (UC-PLN-09 check 6): the rows drawn on a canvas - a title column, a bar per
 * entry over the plan's days, a diamond for a milestone, the critical chain in the danger tint - and
 * saved as PNG, or printed, which every browser offers to save as PDF. No renderer is shipped
 * (decided 2026-10-30: no PDF library); the picture is drawn from the same rows the chart draws.
 *
 * Every colour and the font are read from the page as it is themed (rule 15): the picture looks
 * like the screen it came from. Its geometry - row height, the title column - is the picture's
 * own, in canvas pixels, not the screen's layout.
 */

import type { GanttRow } from '../data/planning.ts';

const DAY_MS = 86_400_000;

interface Look {
  readonly text: string;
  readonly quiet: string;
  readonly bar: string;
  readonly critical: string;
  readonly line: string;
  readonly ground: string;
  readonly font: string;
}

function look(): Look {
  const style = getComputedStyle(document.documentElement);
  const token = (name: string) => style.getPropertyValue(name).trim();
  return {
    text: token('--text-primary'), quiet: token('--text-secondary'), bar: token('--accent-primary'),
    critical: token('--status-danger-accent'), line: token('--border-subtle'), ground: token('--bg-surface'),
    font: getComputedStyle(document.body).font,
  };
}

const dayOf = (day: string) => Date.parse(`${day}T00:00:00Z`) / DAY_MS;

/** Draws the rows over their whole reach and answers the canvas. */
export function drawPlan(rows: readonly GanttRow[], title: string, scale = 2): HTMLCanvasElement {
  const colours = look();
  const dated = rows.flatMap((row) => [row.start, row.due]).filter((day) => !!day).map(dayOf);
  const first = dated.length > 0 ? Math.min(...dated) : 0;
  const last = dated.length > 0 ? Math.max(...dated) : 0;
  const days = Math.max(last - first + 1, 1);
  const rowHeight = 24, header = 40, titleWidth = 280, dayWidth = Math.max(4, Math.min(24, 1200 / days));
  const width = titleWidth + days * dayWidth + 16, height = header + rows.length * rowHeight + 16;
  const canvas = document.createElement('canvas');
  canvas.width = width * scale;
  canvas.height = height * scale;
  const pen = canvas.getContext('2d');
  if (!pen) return canvas;
  pen.scale(scale, scale);
  pen.fillStyle = colours.ground;
  pen.fillRect(0, 0, width, height);
  pen.fillStyle = colours.text;
  pen.font = colours.font;
  pen.fillText(title, 8, 20);
  pen.strokeStyle = colours.line;
  for (let day = 0; day <= days; day += 7) {
    const x = titleWidth + day * dayWidth;
    pen.beginPath();
    pen.moveTo(x, header - 8);
    pen.lineTo(x, height - 8);
    pen.stroke();
    pen.fillStyle = colours.quiet;
    pen.fillText(new Date((first + day) * DAY_MS).toISOString().slice(0, 10), x + 2, header - 12);
  }
  rows.forEach((row, at) => {
    const y = header + at * rowHeight;
    pen.fillStyle = colours.text;
    const label = `${'  '.repeat(Math.min(row.depth, 6))}${row.title}`;
    pen.fillText(label.length > 40 ? `${label.slice(0, 39)}…` : label, 8, y + 16, titleWidth - 16);
    if (!row.due) return;
    pen.fillStyle = row.isCritical ? colours.critical : colours.bar;
    const end = titleWidth + (dayOf(row.due) - first + 1) * dayWidth;
    if (row.kind === 'milestone' || !row.start) {
      const x = end - dayWidth / 2, mid = y + rowHeight / 2;
      pen.beginPath();
      pen.moveTo(x, mid - 7);
      pen.lineTo(x + 7, mid);
      pen.lineTo(x, mid + 7);
      pen.lineTo(x - 7, mid);
      pen.closePath();
      pen.fill();
      return;
    }
    const start = titleWidth + (dayOf(row.start) - first) * dayWidth;
    pen.fillRect(start, y + (row.kind === 'summary' ? 9 : 5), Math.max(end - start, 2), row.kind === 'summary' ? 6 : 14);
  });
  return canvas;
}

/** The picture as PNG bytes. */
export function pngOf(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error('png'))), 'image/png'));
}

/**
 * Prints the picture alone, through a frame of its own so the app's chrome stays off the page; the
 * browser's print dialog saves it as PDF.
 */
export function printPicture(canvas: HTMLCanvasElement, title: string): void {
  const frame = document.createElement('iframe');
  frame.setAttribute('aria-hidden', 'true');
  frame.style.position = 'fixed';
  frame.style.inset = '0';
  frame.style.width = '0';
  frame.style.height = '0';
  frame.style.border = '0';
  document.body.append(frame);
  const page = frame.contentDocument;
  const view = frame.contentWindow;
  if (!page || !view) {
    frame.remove();
    return;
  }
  page.title = title;
  const image = page.createElement('img');
  image.alt = title;
  image.style.maxWidth = '100%';
  image.src = canvas.toDataURL('image/png');
  page.body.style.margin = '0';
  page.body.append(image);
  view.onafterprint = () => frame.remove();
  image.onload = () => {
    view.focus();
    view.print();
  };
}
