// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The governed agents' paths and what the *Agents* screen decides before it draws a control
 * (UC-AGT-01 … UC-AGT-05, UC-AGT-07). Pure, so the tests read it without a component.
 *
 * **Which press is offered mirrors the domain's transitions** (`core/domain/model/agent`), so a
 * control is drawn only where the server would take it (P-05); the server still decides (rule 2).
 */

import type { Agent, AgentMeteringList, AgentRequest, AgentWrite } from '@hubtask/sync-engine';

export const agentsPath = '/agents';
export const agentModelsPath = '/agent-models';
export const agentHaltPath = '/agents:halt';
/** The workspace's agent policy: whether MCP takes registered agents only. */
export const agentPolicyPath = '/agents:policy';
export const agentPath = (agentId: string) => `/agents/${agentId}`;
export const agentActionsPath = (agentId: string) => `/agents/${agentId}/actions`;
export const agentJobsPath = (agentId: string) => `/agents/${agentId}/jobs`;
export const agentCasesPath = (agentId: string) => `/agents/${agentId}/cases`;
export const agentEvaluationsPath = (agentId: string) => `/agents/${agentId}/evaluations`;
export const agentModelPath = (modelId: string) => `/agent-models/${modelId}`;
export const provenancePath = (itemId: string) => `/items/${itemId}/provenance`;

export function meteringPath(agentId?: string, days?: number): string {
  const query = new URLSearchParams();
  if (agentId) query.set('agent_id', agentId);
  if (days) query.set('days', String(days));
  const text = query.toString();
  return text ? `/agents:metering?${text}` : '/agents:metering';
}

export function requestsPath(status: AgentRequest['status'] = 'PENDING', agentId?: string): string {
  const query = new URLSearchParams({ status });
  if (agentId) query.set('agent_id', agentId);
  return `/agent-requests?${query.toString()}`;
}

/** A press on an agent's lifecycle. */
export type LifecyclePress = 'approve' | 'activate' | 'suspend' | 'retire';

/** Who is looking: whether they administer agents, and their own account. */
export interface Viewer {
  readonly administers: boolean;
  readonly accountId: string | undefined;
}

/**
 * The lifecycle presses an agent offers this viewer, in the order the screen draws them.
 *
 * An administrator approves an agent that is a draft or whose model moved since its approval -
 * never one acting for themselves (`agents.owner_approves_not`) -, puts an approved or stopped one
 * live once its approval covers its model, and retires it. Its owner or an administrator stops a
 * live or approved one. A retired agent offers nothing.
 */
export function lifecyclePresses(agent: Pick<Agent, 'status' | 'needs_approval' | 'owner_id'>, viewer: Viewer): LifecyclePress[] {
  if (agent.status === 'RETIRED') return [];
  const isOwner = viewer.accountId !== undefined && agent.owner_id === viewer.accountId;
  const out: LifecyclePress[] = [];
  if (viewer.administers && !isOwner && (agent.status === 'DRAFT' || agent.needs_approval)) out.push('approve');
  if (viewer.administers && (agent.status === 'APPROVED' || agent.status === 'SUSPENDED') && !agent.needs_approval) out.push('activate');
  if ((viewer.administers || isOwner) && (agent.status === 'ACTIVE' || agent.status === 'APPROVED')) out.push('suspend');
  if (viewer.administers) out.push('retire');
  return out;
}

/** What a shipped agent's job works on; an agent built elsewhere runs none here. */
export function jobTarget(kind: Agent['kind']): 'ENTRY' | 'HUB' | undefined {
  if (kind === 'EXTERNAL') return undefined;
  return kind === 'DUPLICATE_REVIEW' ? 'HUB' : 'ENTRY';
}

/** Whether *Run on…* is offered: a live shipped agent, to its owner or an administrator (UC-AGT-07 check 7). */
export function mayStartJob(agent: Pick<Agent, 'status' | 'kind' | 'owner_id'>, viewer: Viewer): boolean {
  if (agent.status !== 'ACTIVE' || jobTarget(agent.kind) === undefined) return false;
  return viewer.administers || (viewer.accountId !== undefined && agent.owner_id === viewer.accountId);
}

/** What a shipped agent's own run of its set needs: nothing, a hub, or answers somebody sends (UC-AGT-04 check 7). */
export function selfEvaluation(kind: Agent['kind']): 'ready' | 'hub' | undefined {
  if (kind === 'KNOWLEDGE_DRAFT') return 'ready';
  if (kind === 'CI_SUGGESTION' || kind === 'TICKET_TRIAGE') return 'hub';
  return undefined;
}

/** Whether the viewer decides an agent's held acts: its owner or an administrator (UC-AGT-02 check 5). */
export function mayDecide(agent: Pick<Agent, 'owner_id'> | undefined, viewer: Viewer): boolean {
  if (viewer.administers) return true;
  return agent !== undefined && viewer.accountId !== undefined && agent.owner_id === viewer.accountId;
}

/** One line of a held act's preview: a field the act would write, and what it would write. */
export interface PreviewLine {
  readonly key: string;
  readonly value: string;
}

const LONGEST = 160;

function said(value: unknown): string {
  if (value === null || value === undefined) return '—';
  if (typeof value === 'string') return value.length > LONGEST ? `${value.slice(0, LONGEST)}…` : value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  const text = JSON.stringify(value);
  return text.length > LONGEST ? `${text.slice(0, LONGEST)}…` : text;
}

/**
 * What approving a held act would do, as the lines a person reads before deciding (UC-AGT-02
 * check 9): each field of its input, nested ones by their path, in the order the input holds them.
 * A long value is cut, a list is said whole as written.
 */
export function previewOf(input: Record<string, unknown>, prefix = ''): PreviewLine[] {
  const out: PreviewLine[] = [];
  for (const [key, value] of Object.entries(input)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
      const inner = previewOf(value as Record<string, unknown>, path);
      if (inner.length > 0) out.push(...inner);
      else out.push({ key: path, value: '{}' });
    } else {
      out.push({ key: path, value: said(value) });
    }
  }
  return out;
}

/** A held act said in words: the sentence, the text the agent wrote, where it would land. */
export interface HeldActWords {
  readonly code: string;
  readonly params: Readonly<Record<string, string>>;
  readonly text?: string;
  readonly href?: string;
}

const textOf = (value: unknown): string | undefined => (typeof value === 'string' && value !== '' ? value : undefined);

/**
 * The acts the shipped roles propose (`Runner.go`: a comment on an entry, a page, a CI duplicate
 * resolved) in the decider's words (user-experience.md §7.8). Any other use case - an external
 * agent may call any its scopes allow - and any input not of the shipped shape answer nothing, so
 * the card keeps the name and the keys rather than guess at words.
 */
export function heldActWordsOf(useCase: string, input: Record<string, unknown>): HeldActWords | undefined {
  if (useCase === 'AddComment') {
    const item = textOf(input['item_id']);
    const body = textOf(input['body']);
    if (!item || !body) return undefined;
    return { code: input['internal'] === true ? 'app.agents.held.add_internal_comment' : 'app.agents.held.add_comment', params: {}, text: body, href: `/items/${item}` };
  }
  if (useCase === 'CreatePage') {
    const title = textOf(input['title']);
    if (!title) return undefined;
    const body = textOf(input['body']);
    return { code: 'app.agents.held.create_page', params: { title }, ...(body ? { text: body } : {}) };
  }
  if (useCase === 'ResolveCiDuplicate') {
    if (!textOf(input['duplicate_id']) || !textOf(input['survivor_id'])) return undefined;
    return { code: 'app.agents.held.resolve_ci_duplicate', params: {} };
  }
  return undefined;
}

/** One agent's sums over the days the metering answered. */
export interface MeteringTotal {
  readonly agentId: string;
  readonly allowed: number;
  readonly held: number;
  readonly refused: number;
  readonly tokens: number;
  readonly days: number;
}

/** The metering summed per agent, the agent that spent most tokens first (UC-AGT-02 check 8). */
export function meteringTotals(rows: AgentMeteringList['data']): MeteringTotal[] {
  const by = new Map<string, { allowed: number; held: number; refused: number; tokens: number; days: number }>();
  for (const row of rows) {
    const sum = by.get(row.agent_id) ?? { allowed: 0, held: 0, refused: 0, tokens: 0, days: 0 };
    sum.allowed += row.allowed;
    sum.held += row.held;
    sum.refused += row.refused;
    sum.tokens += row.tokens;
    sum.days += 1;
    by.set(row.agent_id, sum);
  }
  return [...by.entries()]
    .map(([agentId, sum]) => ({ agentId, ...sum }))
    .sort((a, b) => b.tokens - a.tokens || a.agentId.localeCompare(b.agentId));
}

/** A scope list typed by a person: words split on commas and white space, each once. */
export function scopesFrom(text: string): string[] {
  return [...new Set(text.split(/[\s,]+/).map((each) => each.trim()).filter(Boolean))];
}

/**
 * The scopes *Add an agent* offers to tick: the installation's, less the operator's (an agent of a
 * workspace never holds them), and any the agent already holds beyond them, so none is dropped.
 */
export function scopeChoices(declared: readonly string[], held: readonly string[]): string[] {
  return [...new Set([...declared.filter((scope) => !scope.startsWith('admin:')), ...held])];
}

/** The consent screen's words for a scope (`app.consent.scope.*`), one catalogue for both. */
export function scopeWordCode(scope: string): string {
  return `app.consent.scope.${scope.replace(/[:.]/g, '_')}`;
}

/** The typed list after ticking: what was typed beyond the offered scopes stays, then the ticked. */
export function scopesTicked(text: string, offered: readonly string[], ticked: readonly string[]): string {
  const kept = scopesFrom(text).filter((scope) => !offered.includes(scope));
  return [...new Set([...kept, ...offered.filter((scope) => ticked.includes(scope))])].join(' ');
}

/**
 * An expected answer typed as JSON, or why it is not one. Only an object is an answer the server
 * compares key by key (UC-AGT-04 check 2).
 */
export function expectedFrom(text: string): { value: Record<string, unknown> } | { problem: 'not_json' | 'not_object' } {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return { problem: 'not_json' };
  }
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) return { problem: 'not_object' };
  return { value: parsed as Record<string, unknown> };
}

/**
 * Which of an entry's values came from AI (UC-AGT-05 check 5): the field names of every accepted
 * suggestion, each once, in the order they were first accepted. Never a value - the mark holds none.
 */
export function aiFields(marks: readonly { fields: readonly string[]; accepted_at: string }[]): string[] {
  const ordered = [...marks].sort((a, b) => a.accepted_at.localeCompare(b.accepted_at));
  return [...new Set(ordered.flatMap((each) => each.fields))];
}

/** How long a model's provider keeps what it is sent, as a message code (UC-AGT-03 check 7). */
export function retentionOf(days: number | null): { code: string; params: Record<string, string> } {
  if (days === null) return { code: 'app.agents.retention_unknown', params: {} };
  if (days === 0) return { code: 'app.agents.retention_none', params: {} };
  return { code: 'app.agents.retention_days', params: { days: String(days) } };
}

/**
 * The domain's `DefaultBudget`: what an agent may spend unless it states otherwise. A zero allows
 * nothing (`Admission`), so a new agent never starts from zeros.
 */
export const DEFAULT_BUDGET = { tokens_per_day: 200000, actions_per_hour: 120, concurrent_runs: 2 } as const;

/** The roles the product ships, in the order *Add an agent* offers them (user-experience.md §7.8). */
export const SHIPPED_ROLES = ['TICKET_TRIAGE', 'KNOWLEDGE_DRAFT', 'DUPLICATE_REVIEW', 'CI_SUGGESTION'] as const;

/** What the register or change dialog holds, as typed. */
export interface AgentDraft {
  name: string;
  purpose: string;
  kind: AgentWrite['kind'];
  owner: string;
  risk: 'MINIMAL' | 'LIMITED';
  scopes: string;
  model: string;
  reversible: 'ALLOW' | 'NOTIFY' | 'APPROVAL' | 'NEVER';
  irreversible: 'APPROVAL' | 'NEVER';
  tokens: string;
  actions: string;
  runs: string;
  accuracy: string;
}

type Stated = Pick<Agent, 'name' | 'purpose' | 'kind' | 'owner_id' | 'risk_class' | 'scopes' | 'model_id' | 'policy' | 'budget' | 'min_accuracy'>;

/**
 * The dialog's start: the agent as it is, or a new one of a role - its kind, the reader as the
 * person it acts for, every act waiting for a person and the default budget. The name and purpose
 * of a role are words, filled by the caller.
 */
export function draftOf(of: Stated | undefined, viewerId: string, role: AgentWrite['kind'] = 'TICKET_TRIAGE'): AgentDraft {
  return {
    name: of?.name ?? '',
    purpose: of?.purpose ?? '',
    kind: of?.kind ?? role,
    owner: of?.owner_id ?? viewerId,
    risk: of?.risk_class ?? 'MINIMAL',
    scopes: (of?.scopes ?? []).join(' '),
    model: of?.model_id ?? '',
    reversible: of?.policy.reversible ?? 'APPROVAL',
    irreversible: of?.policy.irreversible ?? 'APPROVAL',
    tokens: String(of?.budget.tokens_per_day ?? DEFAULT_BUDGET.tokens_per_day),
    actions: String(of?.budget.actions_per_hour ?? DEFAULT_BUDGET.actions_per_hour),
    runs: String(of?.budget.concurrent_runs ?? DEFAULT_BUDGET.concurrent_runs),
    accuracy: String(of?.min_accuracy ?? 0),
  };
}

/** How many acts wait on each agent, by its id. */
export function heldCounts(requests: readonly Pick<AgentRequest, 'agent_id'>[]): Map<string, number> {
  const out = new Map<string, number>();
  for (const each of requests) out.set(each.agent_id, (out.get(each.agent_id) ?? 0) + 1);
  return out;
}

const STATE_ORDER: Readonly<Record<string, number>> = { ACTIVE: 0, APPROVED: 1, SUSPENDED: 2, DRAFT: 3, RETIRED: 4 };

/** The agents as their cards stand: live ones first, a retired one last, else as the server answered. */
export function byState<A extends Pick<Agent, 'status'>>(list: readonly A[]): A[] {
  return list
    .map((agent, index) => ({ agent, index }))
    .sort((a, b) => (STATE_ORDER[a.agent.status] ?? 9) - (STATE_ORDER[b.agent.status] ?? 9) || a.index - b.index)
    .map(({ agent }) => agent);
}
