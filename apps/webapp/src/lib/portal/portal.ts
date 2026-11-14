// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The requester portal's arithmetic, without a screen: what a typed answer becomes for a question
 * of the catalogue, and how a goal of the request stands for the requester (UC-SVC-05).
 */

import type { EntryType } from '@hubtask/sync-engine';

import { offeredTypes } from '../data/entrytypes.ts';

/** A question as the catalogue asks it: a custom field's key, kind and the values a choice offers. */
export interface Question {
  readonly key: string;
  readonly kind: string;
  readonly options: readonly string[];
}

/** The kinds the portal draws as one line of text; every other kind it draws by its own control. */
const TEXT_KINDS = new Set(['TEXT', 'URL', 'RICH_TEXT']);

/** How the portal draws a question. A kind it cannot answer (a person, a reference) is left out. */
export type Control = 'text' | 'number' | 'date' | 'choice' | 'choices' | 'yes-no';

export function controlOf(question: Question): Control | undefined {
  if (TEXT_KINDS.has(question.kind)) return 'text';
  switch (question.kind) {
    case 'NUMBER':
    case 'DURATION':
      return 'number';
    case 'DATE':
      return 'date';
    case 'SELECT':
      return 'choice';
    case 'MULTI_SELECT':
      return 'choices';
    case 'BOOL':
      return 'yes-no';
    default:
      return undefined;
  }
}

/** The questions the portal can draw, in the catalogue's order. */
export const askable = (questions: readonly Question[]): readonly Question[] => questions.filter((q) => controlOf(q) !== undefined);

/**
 * The answers as `custom_fields`: an unanswered question is left out, so the form's own default
 * or preset applies and a required one is refused by the server where it belongs. A number that
 * is not one is left out for the same reason.
 */
export function answersOf(questions: readonly Question[], typed: Readonly<Record<string, unknown>>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const question of questions) {
    const value = typed[question.key];
    switch (controlOf(question)) {
      case 'number': {
        const text = typeof value === 'string' ? value.trim().replace(',', '.') : '';
        const number = Number(text);
        if (text !== '' && Number.isFinite(number)) out[question.key] = number;
        break;
      }
      case 'choices':
        if (Array.isArray(value) && value.length > 0) out[question.key] = value.filter((v) => question.options.includes(v as string));
        break;
      case 'yes-no':
        if (typeof value === 'boolean') out[question.key] = value;
        break;
      case 'choice':
        if (typeof value === 'string' && question.options.includes(value)) out[question.key] = value;
        break;
      case 'text':
      case 'date':
        if (typeof value === 'string' && value.trim() !== '') out[question.key] = value.trim();
        break;
      default:
        break;
    }
  }
  return out;
}

/** A goal of the request as its requester reads it. */
export interface Goal {
  readonly standing: 'RUNNING' | 'PAUSED' | 'MET' | 'BREACHED';
  readonly due_at: string | null;
  readonly at_risk_at: string | null;
}

export type GoalTone = 'info' | 'success' | 'warning' | 'danger' | 'neutral';

/** The tone a goal is drawn in at a moment: a running goal past its warning is a warning. */
export function goalTone(goal: Goal, now: Date): GoalTone {
  switch (goal.standing) {
    case 'MET':
      return 'success';
    case 'BREACHED':
      return 'danger';
    case 'PAUSED':
      return 'neutral';
    default:
      return goal.at_risk_at !== null && Date.parse(goal.at_risk_at) <= now.getTime() ? 'warning' : 'info';
  }
}

/** The words typed so far that are worth asking articles for: nothing below three letters. */
export function deflectionText(title: string, notes: string): string {
  const text = `${title} ${notes}`.trim().replace(/\s+/gu, ' ');
  return text.length < 3 ? '' : text.slice(0, 500);
}

/** The service a portal is for, as the catalogue names it. */
export interface Desk {
  readonly name: string;
  readonly description: string;
  readonly icon: string;
  readonly color_token: string;
  /** The desk's own logo (check 9); null or absent draws the icon and the colour. */
  readonly logo_media_id?: string | null;
}

/**
 * The desk as its team sees it at its own portal, where the catalogue is refused to it: the
 * collection's own name, description and look, and the logo read for the team; undefined until
 * the collection is read.
 */
export function teamDeskOf(
  collection: { readonly name: string; readonly description?: string | null; readonly icon?: string | null; readonly color_token?: string | null } | undefined,
  logo: string | null | undefined,
): Desk | undefined {
  if (!collection) return undefined;
  return { name: collection.name, description: collection.description ?? '', icon: collection.icon ?? '', color_token: collection.color_token ?? '', logo_media_id: logo ?? null };
}

/** The logo the desk is drawn by, or undefined for its icon and colour (UC-SVC-05 check 9). */
export function deskLogoOf(desk: Desk | undefined): string | undefined {
  return desk?.logo_media_id || undefined;
}

/**
 * The portal's head (UC-SVC-05 check 8): the desk's name and description, the portal's own words
 * where the desk leaves one empty or is not known yet.
 */
export function deskHead(desk: Desk | undefined, words: { title: string; intro: string }): { title: string; subtitle: string } {
  return { title: desk?.name || words.title, subtitle: desk?.description || words.intro };
}

/**
 * Whether the portal's catalogue was refused because the reader is not a requester there - the
 * desk's own team opening its portal from the collection - rather than broken: a 403 on the
 * catalogue is that, and is said as such instead of as a role code with a reference.
 */
export const isTeamView = (failure: { readonly status?: number } | undefined): boolean => failure?.status === 403;

/**
 * Whether a collection acts as a desk: the types it offers by name (its own or its hub's
 * `entry_types`) include one whose form leaves the requester portal open. A collection offering
 * every type is no desk though a request type of the workspace is usable there: a template taken
 * by the workspace would otherwise turn every Scrum or Kanban collection into one.
 */
export function actsAsDesk(types: readonly EntryType[], offered: readonly string[] | undefined, hub?: string | null): boolean {
  if (!offered || offered.length === 0) return false;
  return offeredTypes(types, 'TASK', offered, hub)
    .some((type) => offered.includes(type.key) && (type.form?.channels ?? []).some((rule) => rule.channel === 'PORTAL' && rule.closed !== true));
}

/**
 * The request types a new desk offers (the *Desk* kind of a new collection, the navigation
 * concept §5.1): the hub's task types whose form leaves the requester portal open. Offering them
 * by name is what makes the collection act as a desk; none means the hub has no portal type yet.
 */
export function portalTypesOf(types: readonly EntryType[], hub: string): string[] {
  return offeredTypes(types, 'TASK', undefined, hub)
    .filter((type) => (type.form?.channels ?? []).some((rule) => rule.channel === 'PORTAL' && rule.closed !== true))
    .map((type) => type.key);
}

/** A portal the signed-in account uses as a requester (`portals` on `GET /accounts/me`). */
export interface PortalPlace { readonly collection_id: string; readonly name: string }

/**
 * Where a requester who holds nothing but portals starts: their one portal, else nowhere - with
 * several, the start lists them. Somebody with access anywhere starts on their day as before.
 */
export function portalStart(hasNoAccess: boolean, portals: readonly PortalPlace[], last?: string): string | undefined {
  if (!hasNoAccess || portals.length === 0) return undefined;
  const chosen = portals.find((each) => each.collection_id === last) ?? portals[0];
  return `/portal/${chosen?.collection_id}`;
}

/**
 * Whether the reader gets the outsiders' frame (PO-2, E-13): READ nowhere, a portal somewhere.
 * Only the web app's frame follows it; every door refuses them as before.
 */
export const isOutsider = (hasNoAccess: boolean, portals: readonly PortalPlace[]): boolean => hasNoAccess && portals.length > 0;

/** The tabs of the outsiders' frame, in their order (PO-2, scene `portalext`). */
export type PortalTab = 'start' | 'requests' | 'approvals' | 'knowledge';

/** The address of a tab of one portal. */
export function portalTabPath(collectionId: string, tab: PortalTab): string {
  return tab === 'start' ? `/portal/${collectionId}` : `/portal/${collectionId}/${tab}`;
}

/** The tab a route of the portal belongs to; the article reads under *knowledge*, a request under *requests*. */
export function portalTabOf(routeName: string | null | undefined): PortalTab | undefined {
  switch (routeName) {
    case 'portal':
      return 'start';
    case 'portal-requests':
    case 'portal-request':
      return 'requests';
    case 'portal-approvals':
      return 'approvals';
    case 'portal-knowledge':
    case 'portal-article':
      return 'knowledge';
    default:
      return undefined;
  }
}

/**
 * The routes an outsider may stand on: the portal's own and their account's. Every other address
 * leads back to the portal rather than to *no access* (PO-2, PO-5).
 */
const OUTSIDER_ROUTES = new Set([
  'portal', 'portal-requests', 'portal-request', 'portal-approvals', 'portal-knowledge', 'portal-article',
  'profile', 'profile-section', 'security', 'security-section', 'second-factor-new', 'sessions', 'devices',
  'appearance', 'notifications', 'item',
]);
export const isOutsiderRoute = (routeName: string | null | undefined): boolean => OUTSIDER_ROUTES.has(routeName ?? '');

/** A request's state as the portal words it (P-12): the server's status name, else its category's word. */
export interface RequestState {
  readonly label: string;
  /** The category's vocabulary state, for the lozenge's tone and glyph. */
  readonly category: 'todo' | 'progress' | 'done';
}

interface Stateful {
  readonly status_name?: string | null;
  readonly status_category?: string | null;
  readonly is_completed: boolean;
}

export function requestState(request: Stateful, words: { readonly received: string; readonly working: string; readonly done: string }): RequestState {
  const category = request.is_completed || request.status_category === 'DONE'
    ? 'done'
    : request.status_category === 'IN_PROGRESS' ? 'progress' : 'todo';
  const name = request.status_name?.trim();
  const fallback = category === 'done' ? words.done : category === 'progress' ? words.working : words.received;
  return { label: name !== undefined && name !== '' ? name : fallback, category };
}

/**
 * What comes next for a request, as one line under its title (P-12): closed on its day; else when
 * the team last replied; else by when the desk promised to answer - its running SLA goal, which
 * the server never takes from a team's OLA -; else when it arrived or last moved.
 */
export type NextStep =
  | { readonly kind: 'done'; readonly at: string }
  | { readonly kind: 'replied'; readonly at: string }
  | { readonly kind: 'promised'; readonly at: string }
  | { readonly kind: 'working'; readonly at: string }
  | { readonly kind: 'received'; readonly at: string };

export function nextStep(request: Stateful & {
  readonly completed_at: string | null;
  readonly created_at: string;
  readonly updated_at: string;
  readonly last_reply_at?: string | null;
  readonly next_due_at?: string | null;
}): NextStep {
  const { category } = requestState(request, { received: '', working: '', done: '' });
  if (category === 'done') return { kind: 'done', at: request.completed_at ?? request.updated_at };
  if (request.last_reply_at) return { kind: 'replied', at: request.last_reply_at };
  if (request.next_due_at) return { kind: 'promised', at: request.next_due_at };
  if (category === 'progress') return { kind: 'working', at: request.updated_at };
  return { kind: 'received', at: request.created_at };
}

/** One request of several portals, for *my requests · everywhere* (PO-1): newest first, at most `limit`. */
export function newestEverywhere<R extends { readonly created_at: string; readonly id: string }>(
  lists: readonly (readonly R[])[], limit: number,
): R[] {
  const seen = new Set<string>();
  return lists.flat()
    .filter((each) => (seen.has(each.id) ? false : (seen.add(each.id), true)))
    .sort((a, b) => b.created_at.localeCompare(a.created_at) || a.id.localeCompare(b.id))
    .slice(0, limit);
}

/** A space's pages as a tree for the reader's side column: each page with its depth, parents first. */
export interface SpacePage { readonly page_id: string; readonly parent_id: string; readonly title: string; readonly icon: string }

export function spaceOutline(root: string, pages: readonly SpacePage[]): { readonly page: SpacePage; readonly depth: number }[] {
  const depth = new Map<string, number>([[root, 0]]);
  const out: { page: SpacePage; depth: number }[] = [];
  for (const page of pages) {
    const level = (depth.get(page.parent_id) ?? 0) + 1;
    depth.set(page.page_id, level);
    out.push({ page, depth: level });
  }
  return out;
}

/** Whether words find a request type or an article title: every word, in any order, ignoring case. */
export function wordsFind(words: string, ...texts: readonly (string | null | undefined)[]): boolean {
  const said = words.toLocaleLowerCase().split(/\s+/).filter((each) => each !== '');
  if (said.length === 0) return false;
  const haystack = texts.filter(Boolean).join(' ').toLocaleLowerCase();
  return said.every((word) => haystack.includes(word));
}

/**
 * An article's words as a requester reads them (user-experience.md §5.6, §7.4): every internal
 * address - a mention, a card, a live view, a picture whose bytes the portal does not serve - is
 * left out, and a line left empty by it goes with it, so a requester never reads an identifier.
 */
export function articleText(body: string): string {
  return body
    .replace(/!?\[[^\]]*\]\(hubtask:[a-z]+\/[0-9a-fA-F-]{36}(?:#[A-Za-z0-9_-]{1,64})?\)/gu, '')
    .replace(/^::(?:toc|children)\s*$/gmu, '')
    .replace(/[ \t]+$/gmu, '')
    .replace(/\n{3,}/gu, '\n\n')
    .trim();
}
