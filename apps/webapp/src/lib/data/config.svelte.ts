// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The configuration document (F-6) as the administration's *Changes* reads it: the versions, the
 * changes proposed for a person (D-16), the document in force, and a rollback planned and applied
 * through the same plan and apply every other door uses. An apply changes every block it names, so
 * it invalidates everything the configuration feeds rather than a list of its own.
 */

import type {
  ConfigApplyResult, ConfigExport, ConfigPlan, ConfigProposalList, ConfigSandbox, ConfigTemplateList, ConfigTemplatePlan, ConfigVersion,
  ConfigVersionList, ResourceState,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const VERSIONS = '/config/versions';
const PROPOSALS = '/config/proposals';
const DOCUMENT = '/config';
const TEMPLATES = '/config/templates';
const SANDBOX = '/config/sandbox';
// What an apply can change: every block of the document, and what the capabilities answer of it.
const TOUCHED = ['/config', TEMPLATES, '/tenant', '/tenant/modules', '/entry-types', '/link-types', '/workflows', '/custom-fields', '/roles', '/meta/capabilities'];

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

  /** Reads one version with its document, to compare two. */
  async version(version: number): Promise<ResourceState<ConfigVersion>> {
    return engine.refresh<ConfigVersion>({ path: `${VERSIONS}/${version}` });
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

  /** Reads the templates the reader may take, the newest version of each. **From `untrack`**. */
  templates(onNext: (state: ResourceState<ConfigTemplateList>) => void): () => void {
    return engine.subscribe<ConfigTemplateList>({ path: TEMPLATES }, onNext);
  }

  /**
   * Plans taking a template, or upgrading to its newer version: `take` names the upgrade's changes
   * to take, omitted taking every one the workspace did not change itself. Writes nothing.
   */
  async planTemplate(key: string, take?: readonly string[]): Promise<ConfigTemplatePlan> {
    return engine.mutate<ConfigTemplatePlan>('POST', `${TEMPLATES}/${encodeURIComponent(key)}:plan`, take ? { take: [...take] } : {});
  }

  /** Applies a template's plan by its hash, recording the template's provenance. */
  async applyTemplate(plan: ConfigTemplatePlan, acknowledge: boolean): Promise<ConfigApplyResult> {
    return engine.mutate<ConfigApplyResult>('POST', '/config:apply', {
      document: plan.document, mode: plan.mode, plan_hash: plan.plan_hash ?? '', template: plan.template, acknowledge,
    }, { idempotencyKey: crypto.randomUUID(), invalidates: TOUCHED });
  }

  /** Reads the sandbox and the plan of promoting it. **From `untrack`**. */
  sandbox(onNext: (state: ResourceState<ConfigSandbox>) => void): () => void {
    return engine.subscribe<ConfigSandbox>({ path: SANDBOX }, onNext);
  }

  /** Copies the configuration in force into the sandbox afresh, or throws the sandbox away. */
  async resetSandbox(discard: boolean): Promise<ConfigSandbox> {
    return engine.mutate<ConfigSandbox>('POST', SANDBOX, discard ? { discard: true } : { reset: true }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [SANDBOX],
    });
  }

  /** Promotes the sandbox by its promotion's plan hash: what it changed since its copy. */
  async promote(planHash: string, acknowledge: boolean): Promise<ConfigApplyResult> {
    return engine.mutate<ConfigApplyResult>('POST', '/config:apply', { from_sandbox: true, plan_hash: planHash, acknowledge }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [...TOUCHED, SANDBOX],
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
