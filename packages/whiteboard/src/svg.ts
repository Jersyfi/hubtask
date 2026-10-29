// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The board drawn as a picture: the outline of each shape (which the screen draws too), and a
 * whole SVG document of the board, a frame or a selection, for export (UC-CNV-03). The colours
 * are handed in - the application reads them from the design system's values (rule 15) - and a
 * card's words are what the reader sees of its entry, resolved before the export.
 */

import { PEN_WIDTH, strokePath } from './draw.ts';
import { bounds, connectorPath, labelPoint } from './geometry.ts';
import type { Element } from './scene.ts';

/** The outline of a sticky note, a shape, a frame or a card, as an SVG path. */
export function outlinePath(e: Element): string {
  const { x, y, width: w, height: h } = e;
  const n = (value: number) => Math.round(value * 10) / 10;
  switch (e.kind === 'SHAPE' ? e.shape ?? 'RECTANGLE' : 'RECTANGLE') {
    case 'ELLIPSE': {
      const rx = w / 2;
      const ry = h / 2;
      return `M ${n(x)} ${n(y + ry)} A ${n(rx)} ${n(ry)} 0 1 0 ${n(x + w)} ${n(y + ry)} A ${n(rx)} ${n(ry)} 0 1 0 ${n(x)} ${n(y + ry)} Z`;
    }
    case 'DIAMOND':
      return `M ${n(x + w / 2)} ${n(y)} L ${n(x + w)} ${n(y + h / 2)} L ${n(x + w / 2)} ${n(y + h)} L ${n(x)} ${n(y + h / 2)} Z`;
    case 'PARALLELOGRAM': {
      const slant = Math.min(w / 4, h / 2);
      return `M ${n(x + slant)} ${n(y)} L ${n(x + w)} ${n(y)} L ${n(x + w - slant)} ${n(y + h)} L ${n(x)} ${n(y + h)} Z`;
    }
    case 'TRIANGLE':
      return `M ${n(x + w / 2)} ${n(y)} L ${n(x + w)} ${n(y + h)} L ${n(x)} ${n(y + h)} Z`;
    case 'ROUNDED': {
      const r = Math.min(w, h) / 4;
      return `M ${n(x + r)} ${n(y)} H ${n(x + w - r)} Q ${n(x + w)} ${n(y)} ${n(x + w)} ${n(y + r)} V ${n(y + h - r)} ` +
        `Q ${n(x + w)} ${n(y + h)} ${n(x + w - r)} ${n(y + h)} H ${n(x + r)} Q ${n(x)} ${n(y + h)} ${n(x)} ${n(y + h - r)} ` +
        `V ${n(y + r)} Q ${n(x)} ${n(y)} ${n(x + r)} ${n(y)} Z`;
    }
    default:
      return `M ${n(x)} ${n(y)} H ${n(x + w)} V ${n(y + h)} H ${n(x)} Z`;
  }
}

/** The colours a picture of the board is drawn in, from the design system's values. */
export interface Colours {
  /** The page behind the board. */
  paper: string;
  /** Words and outlines. */
  ink: string;
  /** Connectors and a frame's border. */
  line: string;
  /** A sticky note, shape or card of no colour of its own. */
  surface: string;
  /** A label token's colour by its name. */
  label: (token: string) => string | undefined;
}

/** What a card shows in a picture: the entry as the reader sees it. */
export interface CardWords { title: string; key?: string | null; completed?: boolean }

export interface PictureOptions {
  colours: Colours;
  title: string;
  /** The words a card shows; a card the reader cannot see is drawn without them. */
  card?: (entryId: string) => CardWords | undefined;
  /** What a card shows of an entry out of reach. */
  unavailable: string;
  margin?: number;
}

const escape = (value: string) =>
  value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;');

/** A text broken into the lines a box of a width holds, roughly by the characters of a font. */
export function lines(text: string, width: number, size: number): string[] {
  const perLine = Math.max(4, Math.floor(width / (size * 0.55)));
  const out: string[] = [];
  for (const paragraph of text.split('\n')) {
    let line = '';
    for (const word of paragraph.split(/\s+/)) {
      if (!word) continue;
      if (line && line.length + 1 + word.length > perLine) {
        out.push(line);
        line = word;
      } else line = line ? `${line} ${word}` : word;
    }
    out.push(line);
  }
  return out;
}

function words(text: string, x: number, y: number, width: number, size: number, fill: string, anchor = 'start'): string {
  const rows = lines(text, width, size);
  return rows
    .map((row, i) => `<text x="${x}" y="${y + size * (i + 1) * 1.25}" font-size="${size}" fill="${fill}" text-anchor="${anchor}">${escape(row)}</text>`)
    .join('');
}

/** A web link's host, what its card is headed by; the address itself where it has none. */
export function linkHost(address: string): string {
  try {
    return new URL(address).host || address;
  } catch {
    return address;
  }
}

/** A whole SVG document of the objects given, cropped to them with a margin. */
export function toSvg(elements: readonly Element[], options: PictureOptions): string {
  const { colours } = options;
  const margin = options.margin ?? 32;
  const area = bounds(elements) ?? { left: 0, top: 0, right: 1, bottom: 1 };
  const left = area.left - margin;
  const top = area.top - margin;
  const width = area.right - area.left + 2 * margin;
  const height = area.bottom - area.top + 2 * margin;
  const byId = new Map(elements.map((e) => [e.id, e]));
  const parts: string[] = [];
  for (const e of elements) {
    const fill = (e.color_token && colours.label(e.color_token)) || colours.surface;
    switch (e.kind) {
      case 'FRAME':
        parts.push(`<path d="${outlinePath(e)}" fill="none" stroke="${colours.line}" stroke-dasharray="8 6"/>`);
        parts.push(words(e.text ?? '', e.x, e.y - 24, e.width, 14, colours.ink));
        break;
      case 'CONNECTOR': {
        const marker = e.end === 'NONE' ? '' : ' marker-end="url(#arrow)"';
        const startMarker = e.start === 'ARROW' ? ' marker-start="url(#arrow-start)"' : '';
        parts.push(`<path d="${connectorPath(e, byId)}" fill="none" stroke="${colours.line}" stroke-width="2"${marker}${startMarker}/>`);
        if (e.text) {
          const at = labelPoint(e, byId);
          parts.push(words(e.text, at.x, at.y - 20, 200, 12, colours.ink, 'middle'));
        }
        break;
      }
      case 'TEXT':
        parts.push(words(e.text ?? '', e.x, e.y, e.width, 16, colours.ink));
        break;
      case 'CARD': {
        const shown = e.target ? options.card?.(e.target.id) : undefined;
        parts.push(`<path d="${outlinePath(e)}" fill="${colours.surface}" stroke="${colours.line}"/>`);
        const head = shown ? [shown.key, shown.completed ? '✓' : ''].filter(Boolean).join(' ') : '';
        if (head) parts.push(words(head, e.x + 12, e.y + 4, e.width - 24, 12, colours.line));
        parts.push(words(shown?.title ?? options.unavailable, e.x + 12, e.y + (head ? 22 : 8), e.width - 24, 14, colours.ink));
        break;
      }
      case 'STROKE': {
        const pen = e.pen ?? 'PEN';
        const ink = (e.color_token && colours.label(e.color_token)) || colours.ink;
        const opacity = pen === 'HIGHLIGHTER' ? ' stroke-opacity="0.4"' : '';
        parts.push(`<path d="${strokePath(e)}" fill="none" stroke="${ink}" stroke-width="${PEN_WIDTH[pen]}" stroke-linecap="round" stroke-linejoin="round"${opacity}/>`);
        break;
      }
      case 'LINK': {
        parts.push(`<path d="${outlinePath(e)}" fill="${colours.surface}" stroke="${colours.line}"/>`);
        parts.push(words(linkHost(e.url ?? ''), e.x + 12, e.y + 4, e.width - 24, 14, colours.ink));
        parts.push(words(e.url ?? '', e.x + 12, e.y + 26, e.width - 24, 11, colours.line));
        break;
      }
      default:
        parts.push(`<path d="${outlinePath(e)}" fill="${fill}" stroke="${e.kind === 'SHAPE' ? colours.ink : 'none'}"/>`);
        if (e.text) parts.push(words(e.text, e.x + 12, e.y + 4, e.width - 24, 14, colours.ink));
    }
  }
  const arrow = (id: string, orient: string) =>
    `<marker id="${id}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="${orient}">` +
    `<path d="M 0 0 L 10 5 L 0 10 z" fill="${colours.line}"/></marker>`;
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${left} ${top} ${width} ${height}" width="${Math.ceil(width)}" height="${Math.ceil(height)}" font-family="sans-serif">` +
    `<title>${escape(options.title)}</title><defs>${arrow('arrow', 'auto')}${arrow('arrow-start', 'auto-start-reverse')}</defs>` +
    `<rect x="${left}" y="${top}" width="${width}" height="${height}" fill="${colours.paper}"/>${parts.join('')}</svg>`
  );
}
