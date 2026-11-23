// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The page *Create hub* (user-experience.md §5.2, ADR-0111) as data: the starts it offers - *Empty
 * hub* first, then the presets as the *Simple* family, then the catalogue's templates by family -,
 * which of them a search and a family show, and the one request a press sends. Pure, so the rules
 * that decide what is offered and what is sent are tested without a browser.
 */

import type { IconName } from '@hubtask/design-system/components';

import type { Family } from './hubGallery.ts';

/** The start chosen when the page opens: the workspace's default preset, no template. */
export const EMPTY_HUB = 'empty';

/** The value a preset's tile is chosen by; a template's is `template:` and its key. */
export const presetStart = (value: string): string => `preset:${value}`;
export const templateStart = (key: string): string => `template:${key}`;

export type StartFamily = 'simple' | Family;

/** One tile of the gallery, its words resolved. */
export interface Start {
  readonly value: string;
  readonly family: StartFamily;
  readonly label: string;
  readonly summary?: string;
  /** The products it switches on, by name. */
  readonly chips: readonly string[];
  readonly icon?: IconName;
  /** A quiet line: where the workspace stands with it. */
  readonly note?: string;
}

/**
 * Whether a start is found by what was typed: its name, its summary, its products and its family's
 * name, without regard to case. An empty search finds every start.
 */
export function matches(start: Start, familyName: string, query: string, locale?: string): boolean {
  const asked = query.trim().toLocaleLowerCase(locale);
  if (asked === '') return true;
  return [start.label, start.summary ?? '', ...start.chips, familyName].join(' ').toLocaleLowerCase(locale).includes(asked);
}

export interface Shown {
  /** The families in their order, each with the starts the search and the family leave; an emptied one left out. */
  readonly groups: readonly { readonly family: StartFamily; readonly starts: readonly Start[] }[];
  /** How many starts are shown, *Empty hub* not counted: it is never filtered away. */
  readonly count: number;
}

/**
 * What the gallery draws for a family (`all`, or one) and a search. *Empty hub* is not among the
 * starts: it stands pinned before them and stays chosen and offered whatever is searched. A start
 * left out stays chosen: a filter narrows what is seen, it does not answer the question.
 */
export function shownStarts(
  starts: readonly Start[],
  family: 'all' | StartFamily,
  query: string,
  familyName: (family: StartFamily) => string,
  locale?: string,
): Shown {
  const order: StartFamily[] = [];
  for (const start of starts) if (!order.includes(start.family)) order.push(start.family);
  const groups = order
    .filter((each) => family === 'all' || each === family)
    .map((each) => ({
      family: each,
      starts: starts.filter((start) => start.family === each && matches(start, familyName(each), query, locale)),
    }))
    .filter((group) => group.starts.length > 0);
  return { groups, count: groups.reduce((sum, group) => sum + group.starts.length, 0) };
}

/** Each family with its number of starts, in the gallery's order: the filter's counts. */
export function familyCounts(starts: readonly Start[]): { readonly family: StartFamily; readonly count: number }[] {
  const counts = new Map<StartFamily, number>();
  for (const start of starts) counts.set(start.family, (counts.get(start.family) ?? 0) + 1);
  return [...counts].map(([family, count]) => ({ family, count }));
}

/** What the page holds when *Create hub* is pressed. */
export interface HubDraft {
  readonly start: string;
  readonly name: string;
  readonly description: string;
  readonly icon: string | null;
  readonly colorToken: string | null;
  /** The switch, where it is drawn. */
  readonly isPrivate: boolean;
  /** Whether the switch is drawn: the workspace offers private hubs and the reader may make a shared one. */
  readonly isPrivacyOffered: boolean;
  /** The reader may make only a private hub of their own: no switch, no catalogue. */
  readonly isPrivateOnly: boolean;
  /** The chosen template, where the chosen start is one and the reader is offered the catalogue. */
  readonly template?: { readonly key: string; readonly carriesSamples: boolean };
  readonly answers: Readonly<Record<string, string>>;
  readonly withSamples: boolean;
  readonly acknowledged: boolean;
}

export type HubRequest =
  | {
      readonly via: 'create';
      readonly body: {
        type: 'HUB';
        parent_id: null;
        name: string;
        description: string | null;
        icon: string | null;
        color_token: string | null;
        private?: true;
        preset?: 'LIST' | 'TASKS' | 'PROJECT' | 'FLOW';
        preset_id?: string;
      };
    }
  | {
      readonly via: 'template';
      readonly body: {
        name: string;
        description: string | null;
        icon: string | null;
        color_token: string | null;
        template: string;
        private?: true;
        answers?: Record<string, string>;
        samples?: true;
        acknowledge?: true;
      };
    };

const BUILT_IN = ['LIST', 'TASKS', 'PROJECT', 'FLOW'] as const;

/**
 * The one request a press sends. *Empty hub* and a preset: `POST /containers`, its body as the
 * dialog sent it. A template: `POST /containers:from-template`, private where the switch is on
 * (UC-ID-16 check 9) - a reader who may make only a private hub is offered no template, since the
 * template asks the right to shape the workspace (configuration.md §6), so their hub goes the first
 * way, private, whatever else is held.
 */
export function hubRequest(draft: HubDraft): HubRequest {
  const name = draft.name.trim();
  const description = draft.description.trim() === '' ? null : draft.description.trim();
  const isPrivate = draft.isPrivateOnly || (draft.isPrivacyOffered && draft.isPrivate);
  const look = { icon: draft.icon, color_token: draft.colorToken };
  if (draft.template && !draft.isPrivateOnly && draft.start === templateStart(draft.template.key)) {
    return {
      via: 'template',
      body: {
        name,
        description,
        ...look,
        template: draft.template.key,
        ...(isPrivate ? { private: true as const } : {}),
        ...(Object.keys(draft.answers).length > 0 ? { answers: { ...draft.answers } } : {}),
        ...(draft.template.carriesSamples && draft.withSamples ? { samples: true as const } : {}),
        ...(draft.acknowledged ? { acknowledge: true as const } : {}),
      },
    };
  }
  const preset = draft.start.startsWith('preset:') ? draft.start.slice('preset:'.length) : '';
  return {
    via: 'create',
    body: {
      type: 'HUB',
      parent_id: null,
      name,
      description,
      ...look,
      ...(isPrivate ? { private: true as const } : {}),
      ...(preset === '' ? {} : (BUILT_IN as readonly string[]).includes(preset) ? { preset: preset as (typeof BUILT_IN)[number] } : { preset_id: preset }),
    },
  };
}

/**
 * Where the person lands once the hub exists: the template's own landing where it has one, else
 * the screen of the product the page was opened from, else the hub.
 */
export function landingOf(made: { readonly templateLanding?: string; readonly productScreen?: string; readonly hubId: string }): string {
  return made.templateLanding ?? made.productScreen ?? `/hubs/${encodeURIComponent(made.hubId)}`;
}

/** The page's address, with what the opener chose before: a template, the product it was opened from. */
export function createHubPath(from: { readonly template?: string; readonly product?: string } = {}): string {
  const query = new URLSearchParams();
  if (from.template) query.set('template', from.template);
  if (from.product) query.set('product', from.product);
  const said = query.toString();
  return said === '' ? '/hubs/new' : `/hubs/new?${said}`;
}
