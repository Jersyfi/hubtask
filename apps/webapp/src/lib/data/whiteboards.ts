// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's whiteboards as the *Whiteboards* screen needs them: where a board's live document is,
 * the colours a picture of it is drawn in (the light theme's values, as an exported image is a
 * document, not a screen - rule 15), a file name for an export, and the native document read back.
 * Pure, so the shapes have tests without a browser.
 */

import { values } from '@hubtask/design-system';
import { box, type Colours, type Element } from '@hubtask/whiteboard';

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
export function boardFileName(title: string, extension: 'svg' | 'png' | 'pdf' | 'json'): string {
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

/** The colour another person's cursor is drawn in: one of the label colours, the same on every device. */
export function peerColour(clientId: number): LabelName {
  return BOARD_COLOURS[Math.abs(clientId) % BOARD_COLOURS.length] as LabelName;
}

/** The sticky notes with words lying wholly inside a frame: what a frame made entries turns (UC-CNV-02). */
export function notesInFrame(elements: readonly Element[], frame: Element): Element[] {
  const f = box(frame);
  return elements.filter((e) => {
    if (e.kind !== 'STICKY' || !(e.text ?? '').trim()) return false;
    const b = box(e);
    return b.left >= f.left && b.top >= f.top && b.right <= f.right && b.bottom <= f.bottom;
  });
}

/**
 * The key a person's votes are kept under: their account in an open session; in an anonymous one
 * a key their device minted for the session, so the board's document never names the voter.
 */
export function voterKey(anonymous: boolean, accountId: string, minted: () => string, kept?: string): string {
  return anonymous ? kept ?? minted() : accountId;
}

/** A timer's time left as minutes and seconds. */
export function clockOf(ms: number): string {
  const seconds = Math.ceil(Math.max(0, ms) / 1000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
}
