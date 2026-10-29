// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The configuration document (F-6) as the administration's *Changes* reads it: the versions, the
 * changes proposed for a person (D-16), the document in force, and a rollback planned and applied
 * through the same plan and apply every other door uses. An apply changes every block it names, so
 * it invalidates everything the configuration feeds rather than a list of its own.
 */

import type {
  ConfigApplyResult, ConfigExport, ConfigPlan, ConfigProposalList, ConfigVersionList, ResourceState,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const VERSIONS = '/config/versions';
const PROPOSALS = '/config/proposals';
const DOCUMENT = '/config';
// What an apply can change: every block of the document, and what the capabilities answer of it.
const TOUCHED = ['/config', '/tenant/modules', '/entry-types', '/link-types', '/workflows', '/custom-fields', '/roles', '/meta/capabilities'];

class Config {
  /** Reads the versions, newest first. **From `untrack`**. */
  versions(onNext: (state: ResourceState<ConfigVersionList>) => void): () => void {
    return engine.subscribe<ConfigVersionList>({ path: VERSIONS }, onNext);
  }

  /** Reads the proposals waiting for a person. **From `untrack`**. */
  waiting(onNext: (state: ResourceState<ConfigProposalList>) => void): () => void {
    return engine.subscribe<ConfigProposalList>({ path: `${PROPOSALS}?state=WAITING` }, onNext);
  }

  /** Reads the document in force. **From `untrack`**. */
  document(onNext: (state: ResourceState<ConfigExport>) => void): () => void {
    return engine.subscribe<ConfigExport>({ path: DOCUMENT }, onNext);
  }

  /** Plans going back to a version; writes nothing. */
  async planRollback(version: number): Promise<ConfigPlan> {
    return engine.mutate<ConfigPlan>('POST', '/config:plan', { to_version: version });
  }

  /** Applies a rollback by the plan's hash. */
  async rollback(version: number, planHash: string, acknowledge: boolean): Promise<ConfigApplyResult> {
    return engine.mutate<ConfigApplyResult>('POST', '/config:apply', { to_version: version, plan_hash: planHash, acknowledge }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: TOUCHED,
    });
  }

  /** Applies or declines a proposal. */
  async decide(id: string, decision: 'APPLY' | 'DECLINE', acknowledge: boolean): Promise<ConfigApplyResult> {
    return engine.mutate<ConfigApplyResult>('POST', `${PROPOSALS}/${encodeURIComponent(id)}:decide`, { decision, acknowledge }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: TOUCHED,
    });
  }
}

export const config = new Config();

/** Whether a plan or a proposal must have its flags acknowledged before it applies. */
export const needsAcknowledge = (flags: { hides_data: boolean; changes_permissions: boolean }): boolean =>
  flags.hides_data || flags.changes_permissions;
