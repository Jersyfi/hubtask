// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A collection's policies, both ways across the wire (issue 773).
 *
 * **The document is replaced whole.** `PUT /containers/{id}/policies` takes the whole document
 * and a key that is not sent falls back to its default - MANUAL for the completion policy, no
 * automatic assignment - so the form holds every key the contract names and writes every one it
 * has a value for. A form that sent the one field somebody touched would silently reset the
 * other.
 *
 * **What is predicted is predicted in the server's words.** The three rules a policy has to meet
 * - at least one candidate, exactly one for FIXED, groups for RANDOM_GROUP_MEMBER and accounts
 * otherwise - are the schema's, and each is answered with the server's own message code, so
 * that one fact has one sentence whether the client saw the refusal coming or the server sent
 * it. The values themselves come from the manifest, never from a list written here.
 */

import type { Container } from '@hubtask/sync-engine';

// The shapes are the contract's, reached through the container the sync engine re-exports
// (ADR-0004): a second description of a policy here would be a second thing to keep in step.
export type ContainerPolicies = NonNullable<Container['policies']>;
type AutoAssign = NonNullable<ContainerPolicies['auto_assign']>;
export type CompletionPolicy = NonNullable<ContainerPolicies['completion_policy']>;
export type AutoAssignStrategy = AutoAssign['strategy'];
export type Candidate = AutoAssign['candidates'][number];
export type CandidateKind = Candidate['kind'];

/** The form's state: empty is "the default" for the policy and "not at all" for the strategy. */
export interface PoliciesDraft {
  readonly completion: string;
  readonly strategy: string;
  readonly candidates: readonly Candidate[];
  readonly enabled: boolean;
}

/** The form, filled from what the collection holds. */
export function draftOf(policies: ContainerPolicies | undefined): PoliciesDraft {
  return {
    completion: policies?.completion_policy ?? '',
    strategy: policies?.auto_assign?.strategy ?? '',
    candidates: policies?.auto_assign?.candidates ?? [],
    enabled: policies?.auto_assign?.enabled ?? true,
  };
}

/** The document the form writes: every key it has a value for, and `null` for no assignment. */
export function documentOf(draft: PoliciesDraft): ContainerPolicies {
  return {
    ...(draft.completion ? { completion_policy: draft.completion as CompletionPolicy } : {}),
    auto_assign: draft.strategy
      ? { strategy: draft.strategy as AutoAssignStrategy, candidates: [...draft.candidates], enabled: draft.enabled }
      : null,
  };
}

/** Which kind of candidate a strategy draws from: groups for RANDOM_GROUP_MEMBER, accounts otherwise. */
export function candidateKindOf(strategy: string): CandidateKind {
  return strategy === 'RANDOM_GROUP_MEMBER' ? 'GROUP' : 'ACCOUNT';
}

/** A prediction of the server's refusal, in its own code, or nothing where the draft would pass. */
export function candidateProblem(draft: PoliciesDraft): { code: string; params?: Record<string, string> } | undefined {
  if (!draft.strategy) return undefined;
  if (draft.candidates.length === 0) return { code: 'containers.auto_assign_candidates_required' };
  if (draft.strategy === 'FIXED' && draft.candidates.length !== 1) {
    return { code: 'containers.auto_assign_single_candidate_required', params: { count: String(draft.candidates.length) } };
  }
  const wanted = candidateKindOf(draft.strategy);
  const wrong = draft.candidates.find((candidate) => candidate.kind !== wanted);
  if (wrong) return { code: 'containers.auto_assign_candidate_kind_invalid', params: { strategy: draft.strategy, kind: wrong.kind } };
  return undefined;
}

/** The candidate moved one place up or down; the list is unchanged when it cannot go there. */
export function moved(candidates: readonly Candidate[], index: number, by: -1 | 1): Candidate[] {
  const next = [...candidates];
  const target = index + by;
  const moving = next[index];
  const other = next[target];
  if (moving === undefined || other === undefined) return next;
  next[index] = other;
  next[target] = moving;
  return next;
}
