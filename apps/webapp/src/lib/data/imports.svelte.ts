// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The imports this tab started, and their reports.
 *
 * `POST /imports` answers `202` with a `JobRef`; the job is followed by `lib/data/jobs.ts` the
 * way every job is, and the report is read once from `GET /imports/{id}` when the job is
 * terminal. Not a resource: a report arrives once and is not something a screen watches change.
 */

import { engine } from './engine.ts';
import { jobs } from './jobs.svelte.ts';
import type { JobRef } from './jobs.ts';
import type { ImportRequest, ImportRun } from './imports.ts';

export type { ImportKind, ImportRun, ImportRequest } from './imports.ts';

const IMPORTS = '/imports';

class Imports {
  #runs = $state<Record<string, ImportRun>>({});

  /** What is known about one import, or nothing if this tab never read it. */
  of(importId: string): ImportRun | undefined {
    return this.#runs[importId];
  }

  /**
   * Starts the import and watches its job. The key is minted here: a retry of the same request
   * is a second import, and P-08 makes that a no-op on the server anyway.
   */
  async start(request: ImportRequest): Promise<JobRef> {
    const accepted = await engine.mutate<JobRef>('POST', IMPORTS, request, {
      idempotencyKey: crypto.randomUUID(),
    });
    jobs.watch(accepted);
    return accepted;
  }

  /** Reads one import's row - where the report and the refused rows arrive. */
  async read(importId: string): Promise<void> {
    const answer = await engine.refresh<ImportRun>({ path: `${IMPORTS}/${importId}` });
    if (answer.status === 'ready') this.#runs = { ...this.#runs, [importId]: answer.data };
  }
}

export const imports = new Imports();
