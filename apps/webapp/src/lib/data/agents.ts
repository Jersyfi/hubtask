// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The governed agents' paths and what the *Agents* screen decides before it draws a control
 * (UC-AGT-01 … UC-AGT-05, UC-AGT-07). Pure, so the tests read it without a component.
 *
 * **Which press is offered mirrors the domain's transitions** (`core/domain/model/agent`), so a
 * control is drawn only where the server would take it (P-05); the server still decides (rule 2).
 */

import type { Agent, AgentMeteringList, AgentRequest } from '@hubtask/sync-engine';

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
