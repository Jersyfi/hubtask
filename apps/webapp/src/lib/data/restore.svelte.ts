// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Putting an archive back, and the report that has to be read before it happens for real.
 *
 * **`dry_run` is true in the contract and never flipped silently.** A restore is asked for as a
 * dry run first, its report is read, and the request without `dry_run` is **composed from the
 * first** rather than assembled again — what was reviewed is what runs, which is the whole point
 * of `backup-restore.md` §8.3 step 2 and the reason `Asked` is kept as a value rather than as a
 * set of form fields.
 *
 * **The proof goes in the body.** Every other privileged operation in this contract carries its
 * step-up grant in a header; a destructive restore carries it in `step_up_token`. So this module
 * takes the token as an argument rather than letting the engine attach one, and the caller pairs
 * it with `stepUp.around` the same way every other screen does.
 *
 * **Two modes are not here.** `INSTANCE` and `NEW_TENANT` cross or create a tenant, which is the
 * installation operator's business rather than one workspace's (F4's decision 6). They are absent
 * from `Mode` rather than filtered later, so nothing in this client can compose one by accident.
 */

import { engine } from './engine.ts';
import { jobs } from './jobs.svelte.ts';
import type { JobRef } from './jobs.ts';
import type { Asked, Run } from './restore.ts';

export { isDestructive, MODES } from './restore.ts';
export type { Asked, Mode, Report, Run } from './restore.ts';

const RESTORES = '/restores';

class Restores {
  #runs = $state<Record<string, Run>>({});

  /** One restore, by id. What carries the report. */
  of(restoreId: string): Run | undefined {
    return this.#runs[restoreId];
  }

  /**
   * Asks for a restore, and hands the job to the watcher.
   *
   * `dry` decides the one field that separates a rehearsal from the thing itself, and it is named
   * at every call site rather than defaulted here: a default that meant "for real" would be one
   * missing argument away from a workspace being replaced.
   */
  async start(asked: Asked, dry: boolean, stepUpToken?: string): Promise<JobRef> {
    const accepted = await engine.mutate<JobRef>(
      'POST',
      RESTORES,
      {
        target_id: asked.target_id,
        archive_id: asked.archive_id,
        mode: asked.mode,
        dry_run: dry,
        ...(asked.target_tenant_id ? { target_tenant_id: asked.target_tenant_id } : {}),
        ...(asked.conflict_rule ? { conflict_rule: asked.conflict_rule } : {}),
        ...(asked.selection ? { selection: asked.selection } : {}),
        ...(asked.create_safety_backup === undefined
          ? {}
          : { create_safety_backup: asked.create_safety_backup }),
        ...(asked.confirmation ? { confirmation: asked.confirmation } : {}),
        // The one operation in this contract whose proof travels in the body (§8.3 step 3).
        ...(stepUpToken ? { step_up_token: stepUpToken } : {}),
      },
      { idempotencyKey: crypto.randomUUID() },
    );
    jobs.watch(accepted);
    return accepted;
  }

  /**
   * The restore a `JobRef` points at, taken from `result_url`.
   *
   * The last segment of the path the server named, and nothing is assumed beyond that: the
   * contract says `result_url` is where the result can be fetched, and for a restore that is
   * `/restores/{id}`.
   */
  idOf(accepted: JobRef): string | undefined {
    const url = accepted.result_url;
    if (!url) return undefined;
    const segment = url.split('?')[0]?.split('/').pop();
    return segment === '' ? undefined : segment;
  }

  /** Reads one restore's row — where the report and the safety copy's identifier arrive. */
  async read(restoreId: string): Promise<void> {
    const answer = await engine.refresh<Run>({ path: `${RESTORES}/${restoreId}` });
    if (answer.status === 'ready') this.#runs = { ...this.#runs, [restoreId]: answer.data };
  }
}

export const restores = new Restores();
