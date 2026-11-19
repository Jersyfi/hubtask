// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The work surface's demo content for the stories: the story TAMA-18, the ticket SA-12 and the
// queue of the concept's mockups (docs/backlog/work-surface/mockups), in English. A stub of the
// query language stands in for the parser Phase 2 brings; it reads `field: a, b`, `-field: a` and a
// parenthesised group, and nothing else.

import type { FieldCatalogue, QueryLanguage, QueryToken } from './query.ts';
import type { StatusRef, WorkflowStep } from './steps.ts';
import type { Deadline } from './DeadlineList.svelte';

export const STORY_STATUSES: readonly StatusRef[] = [
  { id: 'backlog', label: 'Backlog', category: 'todo' },
  { id: 'ready', label: 'Ready', category: 'todo' },
  { id: 'progress', label: 'In progress', category: 'progress' },
  { id: 'review', label: 'In review', category: 'progress' },
  { id: 'done', label: 'Done', category: 'done' },
  { id: 'dropped', label: 'Dropped', category: 'done' },
];

const status = (id: string): StatusRef => STORY_STATUSES.find((item) => item.id === id)!;

export const IN_PROGRESS = status('progress');

export const STORY_STEPS: readonly WorkflowStep[] = [
  { id: 'request-review', label: 'Request review', to: status('review'), asks: 'asks: who reviews? · needs story points ✓', isNext: true },
  { id: 'put-back', label: 'Put back', to: status('ready'), asks: 'back into planning' },
  { id: 'drop', label: 'Drop', to: status('dropped'), blockedReason: 'only the product owner · Clara Neumann' },
];

export const FREE_STATUSES: readonly StatusRef[] = [
  { id: 'todo', label: 'To do', category: 'todo' },
  { id: 'doing', label: 'In progress', category: 'progress' },
  { id: 'done', label: 'Done', category: 'done' },
];

export interface Ticket {
  readonly key: string;
  readonly title: string;
  readonly type: string;
  readonly requester: string;
  readonly status: 'progress' | 'new' | 'waiting' | 'solved';
  readonly priority: string;
  readonly sla: { readonly share: number; readonly words: string; readonly tone: 'neutral' | 'warning' | 'danger' | 'success' };
  readonly assignee?: string;
}

export const TICKET_GROUPS: Record<Ticket['status'], { label: string; tone: 'info' | 'neutral' | 'warning' | 'success' }> = {
  progress: { label: 'In progress', tone: 'info' },
  new: { label: 'New', tone: 'neutral' },
  waiting: { label: 'Waiting for reply', tone: 'warning' },
  solved: { label: 'Solved', tone: 'success' },
};

export const TICKETS: readonly Ticket[] = [
  { key: 'SA-7', title: 'Management cannot reach the board portal', type: 'Incident', requester: 'Clara Neumann', status: 'progress', priority: 'P1 Critical', sla: { share: 92, words: '3 min left', tone: 'danger' }, assignee: 'Jana Krüger' },
  { key: 'SA-4', title: 'VPN gateway Frankfurt answers only now and then (30 % packet loss)', type: 'Incident', requester: 'Thomas Becker', status: 'progress', priority: 'P2 High', sla: { share: 64, words: '1 h 40 min', tone: 'warning' }, assignee: 'Mehmet Yılmaz' },
  { key: 'SA-8', title: 'Since the update the customer portal no longer opens in Firefox', type: 'Incident', requester: 'Hannah Albers', status: 'progress', priority: 'P3 Medium', sla: { share: 30, words: '1 day 2 h', tone: 'neutral' }, assignee: 'Mehmet Yılmaz' },
  { key: 'SA-1', title: 'Mail to customers arrives an hour late', type: 'Incident', requester: 'Jonas Weber', status: 'new', priority: 'P2 High', sla: { share: 40, words: '3 h 10 min', tone: 'neutral' } },
  { key: 'SA-2', title: 'The printer on the second floor is offline', type: 'Incident', requester: 'Priya Natarajan', status: 'new', priority: 'P3 Medium', sla: { share: 20, words: '6 h', tone: 'neutral' } },
  { key: 'SA-9', title: 'New access for trainees from 1 December (3 people)', type: 'Service request', requester: 'Amelia Hughes', status: 'new', priority: 'P4 Low', sla: { share: 10, words: '4 days', tone: 'neutral' } },
  { key: 'SA-5', title: "Shared mailbox 'invoices@' stopped receiving mail from the ERP", type: 'Incident', requester: 'Lukas Brandt', status: 'waiting', priority: 'P2 High', sla: { share: 0, words: 'paused', tone: 'neutral' } },
  { key: 'SA-10', title: 'Delivery notes are printed without the barcode since this morning', type: 'Incident', requester: "Grace O'Neill", status: 'solved', priority: 'P3 Medium', sla: { share: 100, words: 'met', tone: 'success' } },
];

export const QUERY_CATALOGUE: FieldCatalogue = {
  fields: [
    { key: 'type', label: 'Type', detail: 'kind · every object', example: 'type: incident' },
    { key: 'desk.sla', label: 'SLA', detail: 'state · Service desk', example: 'desk.sla: at-risk' },
    { key: 'who', label: 'Assignee', detail: 'person · every object', example: 'who: me, none' },
    { key: 'updated', label: 'Updated', detail: 'date · every object', example: 'updated: -7d' },
    { key: 'priority', label: 'Priority', detail: 'choice · Service desk', example: 'priority: P1, P2' },
    { key: 'cf.prioritised', label: 'Field "Prioritised by"', detail: 'person · custom field', example: 'cf.prioritised: me' },
  ],
  values: (field) =>
    field === 'priority'
      ? [
          { value: 'P1', label: 'P1 Critical', countLabel: '2 open' },
          { value: 'P2', label: 'P2 High', countLabel: '5 open' },
          { value: 'P3', label: 'P3 Medium', countLabel: '9 open' },
        ]
      : field === 'type'
        ? [
            { value: 'incident', label: 'Incident', countLabel: '7 open' },
            { value: 'request', label: 'Service request', countLabel: '3 open' },
          ]
        : [],
};

const label = (field: string) => QUERY_CATALOGUE.fields.find((item) => item.key === field)?.label ?? field;
let next = 0;

/** The stub language: enough for the stories, nothing the real parser should copy. */
export const STUB_LANGUAGE: QueryLanguage = {
  parse(text) {
    const tokens: QueryToken[] = [];
    let rest = text;
    const pattern = /^\s*(?:(\([^)]*\))|(-?)([\w.]+):\s*((?:[^\s,]+,\s*)*[^\s,]+))\s+/;
    for (let match = pattern.exec(rest); match; match = pattern.exec(rest)) {
      next += 1;
      if (match[1]) tokens.push({ kind: 'text', id: `t${next}`, text: match[1] });
      else {
        const field = match[3] ?? '';
        const values = (match[4] ?? '').split(',').map((value) => value.trim()).filter(Boolean);
        const known = QUERY_CATALOGUE.fields.some((item) => item.key === field);
        tokens.push({
          kind: 'chip',
          id: `t${next}`,
          field,
          fieldLabel: label(field),
          isNegated: match[2] === '-',
          values: values.map((value) => ({ value, label: QUERY_CATALOGUE.values?.(field, '').find((item) => item.value === value)?.label ?? value })),
          problem: known ? undefined : { message: 'is not a field here.', fix: 'priority: P1', fixLabel: 'Use priority' },
        });
      }
      rest = rest.slice(match[0].length);
    }
    return { tokens, rest };
  },
  format(token) {
    if (token.kind === 'text') return token.text;
    return `${token.isNegated ? '-' : ''}${token.field}: ${token.values.map((value) => value.value).join(', ')}`;
  },
};

/** The filter of the scene *qline*: four chips and a group, already parsed. */
export function initialQuery(): QueryToken[] {
  return STUB_LANGUAGE.parse('type: incident, request desk.sla: at-risk who: me, none updated: -7d (label: vip OR desk.org: "Stadtwerke Nord") ').tokens.map((token) => token);
}

/** The deadlines of the ticket SA-12, as the concept's ticket rail draws them (ET-6). */
export const TICKET_DEADLINES: readonly Deadline[] = [
  { id: 'next-reply', name: 'Next reply', context: 'Hannah · SLA', words: '1 h 20 min', share: 45, due: 'until 10:30 · new after every reply' },
  { id: 'res-1st', name: 'Resolution', context: 'Team 1st level · OLA', words: 'breached 40 min', share: 100, tone: 'danger', due: 'was due 12 Nov, 12:00' },
  { id: 'res-sla', name: 'Resolution', context: 'Hannah · SLA', words: '3 h 10 min', share: 30, due: 'until 12 Nov, 16:00' },
  { id: 'res-platform', name: 'Resolution', context: 'Team Platform · OLA', words: '1 h 05 min', share: 80, tone: 'warning', due: 'until 12 Nov, 16:00' },
];
