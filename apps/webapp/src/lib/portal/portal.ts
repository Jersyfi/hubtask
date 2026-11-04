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

/** A portal the signed-in account uses as a requester (`portals` on `GET /accounts/me`). */
export interface PortalPlace { readonly collection_id: string; readonly name: string }

/**
 * Where a requester who holds nothing but portals starts: their one portal, else nowhere - with
 * several, the start lists them. Somebody with access anywhere starts on their day as before.
 */
export function portalStart(hasNoAccess: boolean, portals: readonly PortalPlace[]): string | undefined {
  return hasNoAccess && portals.length === 1 ? `/portal/${portals[0]?.collection_id}` : undefined;
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
