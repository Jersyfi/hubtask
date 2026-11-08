// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's space as the *Pages* screen draws it: the tree from the server's flat rows, the rows a
 * reader sees with the branches they opened, Hubtask's own templates in the reader's words, and
 * the small sums the screen needs. Pure, so the shapes have tests without a browser.
 */

import type { Page, PageSubpage } from '@hubtask/sync-engine';

export type Translate = (code: string, params?: Record<string, string>) => string;

/** One page in the tree, with the pages under it in their order. */
export interface PageNode {
  readonly page: Page;
  readonly depth: number;
  readonly children: PageNode[];
}

/**
 * The tree from the flat list. Siblings sort by `position` as text, as the server orders them
 * (`PageList`); a page whose parent is not in the list (in the trash, archived and not asked for)
 * stands at the top, so nothing the reader may read is lost from the tree.
 */
export function pageTree(rows: readonly Page[]): PageNode[] {
  const byParent = new Map<string, Page[]>();
  const ids = new Set(rows.map((row) => row.id));
  for (const row of rows) {
    const parent = row.parent_id && ids.has(row.parent_id) ? row.parent_id : '';
    const siblings = byParent.get(parent) ?? [];
    siblings.push(row);
    byParent.set(parent, siblings);
  }
  const build = (parent: string, depth: number, seen: Set<string>): PageNode[] =>
    (byParent.get(parent) ?? [])
      .slice()
      .sort((a, b) => (a.position < b.position ? -1 : a.position > b.position ? 1 : a.id.localeCompare(b.id)))
      .filter((page) => !seen.has(page.id))
      .map((page) => {
        seen.add(page.id);
        return { page, depth, children: build(page.id, depth + 1, seen) };
      });
  return build('', 0, new Set());
}

/** The pages above one, from the top: the branches to open so the page can be seen in the tree. */
export function ancestorsOf(rows: readonly Page[], pageId: string): string[] {
  const byId = new Map(rows.map((row) => [row.id, row]));
  const out: string[] = [];
  let at = byId.get(pageId)?.parent_id ?? null;
  while (at && byId.has(at) && !out.includes(at)) {
    out.unshift(at);
    at = byId.get(at)?.parent_id ?? null;
  }
  return out;
}

/** A page and every page under it, the ones a move must not land in (`pages.parent_cycle`). */
export function subtreeOf(rows: readonly Page[], pageId: string): Set<string> {
  const out = new Set([pageId]);
  let grew = true;
  while (grew) {
    grew = false;
    for (const row of rows) {
      if (row.parent_id && out.has(row.parent_id) && !out.has(row.id)) {
        out.add(row.id);
        grew = true;
      }
    }
  }
  return out;
}

/** Where a page's live document is reached (offline-sync.md §13). */
export const livePath = (pageId: string) => `/realtime/documents/page/${encodeURIComponent(pageId)}`;

/** Which of the eight caret colours a writer draws in: the same for a person on every screen. */
export function caretOf(personId: string): number {
  let hash = 0;
  for (const ch of personId) hash = (hash * 31 + ch.charCodeAt(0)) >>> 0;
  return hash % 8;
}

// --- Hubtask's own templates ----------------------------------------------------------------------

/**
 * The templates Hubtask brings, in the reader's words (UC-DOC-05 check 4). Not stored: a space's
 * own templates are its pages marked as one, and these are written when chosen, in the language of
 * the person who chose them - a German team's meeting notes start in German.
 */
export const SHIPPED_TEMPLATES = ['meeting', 'decision', 'howto', 'brief', 'retro'] as const;
export type ShippedTemplate = (typeof SHIPPED_TEMPLATES)[number];

const SECTIONS: Readonly<Record<ShippedTemplate, readonly string[]>> = {
  meeting: ['attendees', 'agenda', 'notes', 'actions'],
  decision: ['context', 'options', 'decision', 'consequences'],
  howto: ['goal', 'before', 'steps', 'check'],
  brief: ['why', 'what', 'scope', 'people', 'dates'],
  retro: ['well', 'better', 'try'],
};

/** A shipped template's title and Markdown body, in the reader's words. */
export function shippedTemplate(key: ShippedTemplate, t: Translate): { title: string; body: string } {
  const lines: string[] = [t(`app.pages.template.${key}.lead`), ''];
  for (const section of SECTIONS[key]) {
    lines.push(`## ${t(`app.pages.template.${key}.${section}`)}`, '');
    lines.push(key === 'meeting' && section === 'actions' ? '- [ ] ' : key === 'howto' && section === 'steps' ? '1. ' : '- ', '');
  }
  return { title: t(`app.pages.template.${key}.title`), body: lines.join('\n').trimEnd() + '\n' };
}

/**
 * Hubtask's space templates (UC-DOC-05 check 6): a first page with pages under it, written at once
 * in the reader's words - each subpage one of the shipped page templates.
 */
export const SHIPPED_SPACES = ['team', 'project', 'knowledge'] as const;
export type ShippedSpace = (typeof SHIPPED_SPACES)[number];

const SPACE_PAGES: Readonly<Record<ShippedSpace, readonly ShippedTemplate[]>> = {
  team: ['meeting', 'decision', 'retro'],
  project: ['brief', 'meeting', 'decision', 'retro'],
  knowledge: ['howto', 'decision'],
};

/** A shipped space template as the body of one write: the first page and its subpages. */
export function shippedSpace(key: ShippedSpace, t: Translate): { title: string; body: string; subpages: PageSubpage[] } {
  return {
    title: t(`app.pages.space.${key}.title`),
    body: `${t(`app.pages.space.${key}.lead`)}\n\n::children\n`,
    subpages: SPACE_PAGES[key].map((page) => shippedTemplate(page, t)),
  };
}

/** Whether a page has pages under it among the rows read - a template offering its subpages. */
export function hasSubpages(rows: readonly Pick<Page, 'parent_id'>[], id: string): boolean {
  return rows.some((row) => row.parent_id === id);
}

/**
 * Where a page stands in its space's way of writing (UC-DOC-15): a live document, never published,
 * with a draft waiting, or published with nothing waiting.
 */
export type DraftState = 'live' | 'unpublished' | 'draft' | 'published';

export function draftStateOf(page: Pick<Page, 'drafts' | 'unpublished' | 'draft'>): DraftState {
  if (!page.drafts) return page.unpublished ? 'unpublished' : 'live';
  if (page.unpublished) return 'unpublished';
  return page.draft ? 'draft' : 'published';
}

/** The body a writer edits: the draft waiting, else the page as published. */
export function writtenBody(page: Pick<Page, 'body' | 'draft'>): string {
  return page.draft?.body ?? page.body ?? '';
}

/** How long a verification runs from today, the choices the screen offers (UC-DOC-12). */
export const VERIFY_DAYS = [30, 90, 180, 365] as const;

/** The instant a verification of so many days from `now` runs until. */
export function verifiedUntil(now: Date, days: number): string {
  return new Date(now.getTime() + days * 86_400_000).toISOString();
}

/** A file name for an exported page: its path in the export, with the folders as dashes. */
export function exportFileName(path: string): string {
  return path.replace(/[\\/]+/gu, '-').replace(/^-+/u, '') || 'page.md';
}

/** The media type a downloaded export is saved with. */
export const EXPORT_TYPES = {
  MARKDOWN: 'text/markdown',
  HTML: 'text/html',
  DOCX: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
} as const;

/** An exported file's bytes: its text, or its base64 decoded (a DOCX document). */
export function exportBytes(file: { content: string; encoding: 'TEXT' | 'BASE64' }): Uint8Array {
  if (file.encoding === 'TEXT') return new TextEncoder().encode(file.content);
  const binary = atob(file.content);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

/** The address a page's public link opens, on this server: the token is the whole credential. */
export const publicLinkAddress = (origin: string, token: string) => `${origin.replace(/\/+$/u, '')}/api/v1/public/pages/${token}`;

/**
 * The page a space opens on (user-experience.md §7.1): its first top page in the tree's order, not
 * a template. A space opens on a page, never on a blank "open a page".
 */
export function homePageOf(rows: readonly Page[]): string | undefined {
  const tree = pageTree(rows);
  return (tree.find((node) => !node.page.template) ?? tree[0])?.page.id;
}

/**
 * The other writers in a page's live document, by the name their caret carries, once each: the
 * reader's own connection is never among them, and an unnamed caret is nobody to show.
 */
export function writersHere(states: ReadonlyMap<number, unknown>, ownClient: number): string[] {
  const names = new Set<string>();
  states.forEach((state, client) => {
    if (client === ownClient || !state || typeof state !== 'object') return;
    const name = (state as { user?: { name?: unknown } }).user?.name;
    if (typeof name === 'string' && name.trim() !== '') names.add(name.trim());
  });
  return [...names];
}

const ADDRESS = /(!?)\[[^\]]*\]\(hubtask:([a-z]+)\/([0-9a-fA-F-]{36})(?:#[A-Za-z0-9_-]{1,64})?\)/gu;

/**
 * What a page's public link shows and leaves out, counted from its body (UC-DOC-08 check 5): its
 * pictures, the pages it mentions (shown as their title only where that page is public itself)
 * and every other mention, which the public page leaves out. Each object counted once.
 */
export function publicPreviewOf(body: string): { media: number; pages: number; others: number } {
  const seen = { media: new Set<string>(), pages: new Set<string>(), others: new Set<string>() };
  for (const match of body.matchAll(ADDRESS)) {
    const kind = match[2] ?? '';
    const id = (match[3] ?? '').toLowerCase();
    if (kind === 'media') seen.media.add(id);
    else if (kind === 'page') seen.pages.add(id);
    else seen.others.add(`${kind}/${id}`);
  }
  return { media: seen.media.size, pages: seen.pages.size, others: seen.others.size };
}

/**
 * The labels a page carries after a writer types more: each word in the server's written form
 * (lower case, once, alphabetical), so the chip drawn is the label the server keeps.
 */
export function withLabels(current: readonly string[], typed: string): string[] {
  const words = typed.split(/[\s,]+/u).map((word) => word.trim().toLowerCase()).filter(Boolean);
  return [...new Set([...current, ...words])].sort();
}

/** How far back a search's *Changed* choice reaches; `any` does not narrow. */
export const CHANGED_WITHIN = { any: 0, week: 7, month: 30, year: 365 } as const;
export type ChangedWithin = keyof typeof CHANGED_WITHIN;

/** The moment a *Changed* choice starts at, or undefined for `any`. */
export function changedFrom(choice: ChangedWithin, now: Date): string | undefined {
  const days = CHANGED_WITHIN[choice];
  return days === 0 ? undefined : new Date(now.getTime() - days * 86_400_000).toISOString();
}
