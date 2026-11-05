// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's whiteboards as the *Whiteboards* screen needs them: where a board's live document is,
 * the colours a picture of it is drawn in (the light theme's values, as an exported image is a
 * document, not a screen - rule 15), a file name for an export, and the native document read back.
 * Pure, so the shapes have tests without a browser.
 */

import { values } from '@hubtask/design-system';
import { box, withinFrame, type Colours, type Element } from '@hubtask/whiteboard';

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

/**
 * Places the uploads that pictures and files carried onto a board draw - pasted from another board
 * or read from an export - so the board's readers read them: a reader is judged by the boards
 * holding an upload, not by the scene (UC-CNV-08). Answers the uploads that could not be placed;
 * those draw as a placeholder.
 */
export async function placeCarried(elements: readonly Element[], place: (mediaId: string) => Promise<unknown>): Promise<string[]> {
  const ids = [...new Set(elements.flatMap((e) => ((e.kind === 'IMAGE' || e.kind === 'FILE') && e.media_id ? [e.media_id] : [])))];
  const outcomes = await Promise.allSettled(ids.map((id) => place(id)));
  return ids.filter((_, i) => outcomes[i]?.status === 'rejected');
}

/** The longest web address a link keeps (`whiteboards.link_url_invalid`). */
const MAX_URL_LENGTH = 2048;

/**
 * A typed web address as a link keeps it: parsed and written back, so a space in it is encoded as
 * the server takes it; anything but http(s), without a host or too long is none. A stored address
 * the server refuses would stop every later save of the board.
 */
export function linkAddress(typed: string): string | undefined {
  try {
    const url = new URL(typed.trim());
    if ((url.protocol !== 'https:' && url.protocol !== 'http:') || !url.host) return undefined;
    return url.href.length <= MAX_URL_LENGTH ? url.href : undefined;
  } catch {
    return undefined;
  }
}

/** The entries a board's cards draw, once each. */
export function cardEntries(elements: readonly Element[]): string[] {
  return [...new Set(elements.flatMap((e) => (e.kind === 'CARD' && e.target ? [e.target.id] : [])))];
}

/**
 * What a board says, in reading order (SC 1.1.1, 1.3.1): frame by frame, top to bottom and left
 * to right, each with the words of the objects it holds in the same order, and then what lies in
 * no frame. An object two frames hold is said in the first. Lines, connectors and empty objects
 * say nothing and are left out; `wordsOf` gives a card or link the words its picture shows.
 */
export function boardInWords(
  elements: readonly Element[],
  wordsOf: (e: Element) => string,
): { readonly frame?: string; readonly key: string; readonly words: readonly string[] }[] {
  const reading = (a: Element, b: Element) => a.y - b.y || a.x - b.x || a.id.localeCompare(b.id);
  const sayable = (e: Element) => e.kind !== 'FRAME' && e.kind !== 'CONNECTOR' && e.kind !== 'STROKE';
  const said = new Set<string>();
  const out: { frame?: string; key: string; words: string[] }[] = [];
  const take = (held: readonly Element[]) =>
    held.filter((e) => sayable(e) && !said.has(e.id)).sort(reading).flatMap((e) => {
      said.add(e.id);
      const words = wordsOf(e).trim();
      return words ? [words] : [];
    });
  for (const frame of elements.filter((e) => e.kind === 'FRAME').sort(reading)) {
    const words = take(withinFrame(elements, frame));
    if (words.length > 0 || frame.text) out.push({ frame: frame.text ?? '', key: frame.id, words });
  }
  const loose = take(elements);
  if (loose.length > 0) out.push({ key: '', words: loose });
  return out;
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

/**
 * What a sticky note made an entry is created with (UC-CNV-02): a task of the collection chosen,
 * the note's words its title, and the entry type chosen where it is not the level's own.
 */
export function entryOfNote(collectionId: string, words: string, kind?: { key: string; system: boolean }): {
  type: 'TASK'; collection_id: string; title: string; entry_type?: string;
} | undefined {
  const title = words.trim();
  if (!title) return undefined;
  return { type: 'TASK', collection_id: collectionId, title, ...(kind && !kind.system ? { entry_type: kind.key } : {}) };
}

/** A timer's time left as minutes and seconds. */
export function clockOf(ms: number): string {
  const seconds = Math.ceil(Math.max(0, ms) / 1000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
}

/** Where a backlink of a board leads: its page, its entry or the other board. */
export function backlinkHref(link: { page_id: string | null; entry_id: string | null; whiteboard_id: string | null }): string {
  if (link.page_id) return `/pages/${link.page_id}`;
  if (link.whiteboard_id) return `/whiteboards/${link.whiteboard_id}`;
  return `/items/${link.entry_id}`;
}

/** A comment as the pins need it (schema WhiteboardComment). */
interface Pinned {
  id: string;
  parent_id: string | null;
  element_id: string | null;
  x: number;
  y: number;
  resolved: boolean;
  deleted: boolean;
}

/**
 * Where a thread's pin stands (UC-CNV-09): at the top corner of its object while the object stands,
 * at the place it was pinned once the object is gone.
 */
export function pinOf(comment: Pinned, byId: ReadonlyMap<string, Element>): { x: number; y: number } {
  const element = comment.element_id ? byId.get(comment.element_id) : undefined;
  if (!element) return { x: comment.x, y: comment.y };
  const at = box(element);
  return { x: at.right, y: at.top };
}

/** A board's threads with their replies, oldest first; a resolved thread only when asked for. */
export function threadsOf<C extends Pinned>(comments: readonly C[], withResolved: boolean): { thread: C; replies: C[] }[] {
  const replies = new Map<string, C[]>();
  for (const comment of comments) {
    if (!comment.parent_id) continue;
    replies.set(comment.parent_id, [...(replies.get(comment.parent_id) ?? []), comment]);
  }
  return comments
    .filter((comment) => !comment.parent_id && (withResolved || !comment.resolved))
    .filter((comment) => !comment.deleted || (replies.get(comment.id)?.length ?? 0) > 0)
    .map((thread) => ({ thread, replies: replies.get(thread.id) ?? [] }));
}

/** The address a comment names a person by, and the words it is drawn as. */
export const mentionOf = (accountId: string, name: string) => `[@${name.replace(/[[\]]/gu, '')}](hubtask:person/${accountId})`;

const MENTION = /\[@([^\]]*)\]\(hubtask:person\/([0-9a-fA-F-]{36})\)/gu;

/** A comment's words cut into text and the people it names, so a person is drawn as their name. */
export function commentParts(body: string): ({ text: string } | { person: string; name: string })[] {
  const out: ({ text: string } | { person: string; name: string })[] = [];
  let last = 0;
  for (const match of body.matchAll(MENTION)) {
    const at = match.index ?? 0;
    if (at > last) out.push({ text: body.slice(last, at) });
    out.push({ person: (match[2] ?? '').toLowerCase(), name: match[1] ?? '' });
    last = at + match[0].length;
  }
  if (last < body.length) out.push({ text: body.slice(last) });
  return out;
}
