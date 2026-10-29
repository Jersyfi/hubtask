// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's whiteboards as the *Whiteboards* screen needs them: where a board's live document is,
 * the colours a picture of it is drawn in (the light theme's values, as an exported image is a
 * document, not a screen - rule 15), a file name for an export, and the native document read back.
 * Pure, so the shapes have tests without a browser.
 */

import { values } from '@hubtask/design-system';
import type { Colours, Element } from '@hubtask/whiteboard';

export const liveBoardPath = (boardId: string) => `/realtime/documents/whiteboard/${encodeURIComponent(boardId)}`;

/** The ten label colours a sticky note or shape may take, in the design system's order. */
export const BOARD_COLOURS = ['amber', 'lime', 'green', 'teal', 'blue', 'violet', 'magenta', 'red', 'orange', 'slate'] as const;

type LabelName = (typeof BOARD_COLOURS)[number];

/** The colours of an exported picture. */
export function pictureColours(): Colours {
  const light = values.light;
  return {
    paper: light.bg.surface,
    ink: light.text.primary,
    line: light.text.secondary,
    surface: light.bg.raised,
    label: (token) => (BOARD_COLOURS as readonly string[]).includes(token) ? light.label[token as LabelName].bg : undefined,
  };
}

/** The name an export is saved under: the board's title made a file name. */
export function boardFileName(title: string, extension: 'svg' | 'png' | 'json'): string {
  const base = title.normalize('NFKD').replace(/[^\p{L}\p{N}]+/gu, '-').replace(/^-+|-+$/gu, '').slice(0, 80) || 'whiteboard';
  return `${base}.${extension}`;
}

/** What a native document holds: the title and the objects, or nothing for a file that is not one. */
export function readNative(text: string): { title: string; elements: Element[] } | undefined {
  try {
    const parsed = JSON.parse(text) as { format?: unknown; version?: unknown; title?: unknown; elements?: unknown };
    if (parsed.format !== 'hubtask.whiteboard' || parsed.version !== 1 || !Array.isArray(parsed.elements)) return undefined;
    return { title: typeof parsed.title === 'string' ? parsed.title : '', elements: parsed.elements as Element[] };
  } catch {
    return undefined;
  }
}

/** The entries a board's cards draw, once each. */
export function cardEntries(elements: readonly Element[]): string[] {
  return [...new Set(elements.flatMap((e) => (e.kind === 'CARD' && e.target ? [e.target.id] : [])))];
}
