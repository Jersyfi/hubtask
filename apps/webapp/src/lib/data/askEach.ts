// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Asking AI about several jumble entries at once (UC-JUM-07 check 12): each through its own ask,
 * a few at a time, each followed on its own card.
 *
 * **No bulk request.** Every answer is already per entry - its job, its refusal, its card - and an
 * agent does the same with the same call. So this is the loop and nothing more.
 *
 * **A refusal that holds for all stops the rest.** AI switched off, its budget spent, the asker
 * keeping their own content out: asking the next entry would be refused the same way, so it is not
 * asked, and the refusal is said once. A refusal about one entry - settled meanwhile, nothing in
 * it - leaves the others to be asked.
 */

/** The refusals that hold for every entry, not only the one that met them. */
const SHARED: ReadonlySet<string> = new Set(['ai.unavailable', 'ai.kept_out', 'ai.processing_restricted', 'ai.source_not_allowed']);

/** How many asks are in flight at once: a selection is answered quickly without a burst. */
export const IN_FLIGHT = 4;

export interface AskedEach {
  /** How many asks were accepted. */
  readonly asked: number;
  /** The refusal that stopped the rest, where one did. */
  readonly stoppedBy?: unknown;
  /** The entries refused on their own account. */
  readonly failed: readonly { readonly id: string; readonly error: unknown }[];
}

function sharedRefusal(error: unknown): boolean {
  const code = (error as { detailCode?: unknown } | undefined)?.detailCode;
  return typeof code === 'string' && SHARED.has(code);
}

/** Asks about each entry in order, `limit` at a time, until a shared refusal stops the rest. */
export async function askEach(
  ids: readonly string[],
  ask: (id: string) => Promise<unknown>,
  limit: number = IN_FLIGHT,
): Promise<AskedEach> {
  let next = 0;
  let asked = 0;
  let stoppedBy: unknown;
  const failed: { id: string; error: unknown }[] = [];

  async function lane(): Promise<void> {
    while (stoppedBy === undefined && next < ids.length) {
      const id = ids[next] ?? '';
      next += 1;
      try {
        await ask(id);
        asked += 1;
      } catch (error) {
        if (sharedRefusal(error)) stoppedBy ??= error;
        else failed.push({ id, error });
      }
    }
  }

  await Promise.all(Array.from({ length: Math.min(limit, ids.length) }, lane));
  return stoppedBy === undefined ? { asked, failed } : { asked, stoppedBy, failed };
}
