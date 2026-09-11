// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a restore is, without the state that runs one.
 *
 * The pure half, for the reason `jobs.ts` is separate from its store: which modes this client can
 * compose is a rule about a set, and a rule about a set is worth a test that needs neither a
 * browser nor a compiler.
 *
 * **Two of the contract's six modes are not here.** `INSTANCE` and `NEW_TENANT` cross or create a
 * tenant, which is the installation operator's business rather than one workspace's (F4's decision
 * 6). They are absent from the type rather than filtered at the end, so nothing in this client can
 * compose one by accident.
 */

/** The four a workspace may ask for. */
export type Mode = 'INSPECT' | 'SELECTIVE' | 'MERGE' | 'REPLACE_TENANT';

/** Every mode this client offers, in the order a screen should put them: safest first. */
export const MODES: readonly Mode[] = ['INSPECT', 'SELECTIVE', 'MERGE', 'REPLACE_TENANT'];

/**
 * The modes that replace what is here, and therefore need all three doors.
 *
 * A function rather than a comparison at each call site: "which of these is the dangerous one" is
 * a question that must have exactly one answer, and three copies of `=== 'REPLACE_TENANT'` is
 * three places for the answer to drift.
 */
export function isDestructive(mode: Mode): boolean {
  return mode === 'REPLACE_TENANT';
}

/** What a restore did, or — on a dry run — what it would do. The same shape either way. */
export interface Report {
  readonly new?: number;
  readonly overwritten?: number;
  readonly skipped?: number;
  readonly duplicated?: number;
  readonly conflicts?: number;
  readonly deleted?: number;
  readonly withheld?: Readonly<Record<string, number>>;
  readonly media?: number;
  readonly entities?: Readonly<Record<string, number>>;
}

/** One restore, asked for or done. */
export interface Run {
  readonly id: string;
  readonly target_id: string;
  readonly source_archive: string;
  readonly tenant_id?: string | null;
  readonly mode: string;
  readonly conflict_rule?: string;
  readonly dry_run: boolean;
  readonly status: string;
  /** The copy taken before a destructive mode, and the way back from it. */
  readonly safety_backup_run_id?: string | null;
  readonly report?: Report;
  readonly started_at?: string | null;
  readonly finished_at?: string | null;
  readonly error_code?: string | null;
}

/**
 * Everything one restore was asked for, minus the two things that are per-attempt.
 *
 * Held as a value so that the second request is the first one with `dry_run: false` — a screen
 * that rebuilt it from its inputs could rebuild it from inputs that had changed while the report
 * was being read.
 */
export interface Asked {
  readonly target_id: string;
  readonly archive_id: string;
  readonly mode: Mode;
  /**
   * The workspace being restored into.
   *
   * Required for every mode this client offers, and it is always *this* workspace: the two modes
   * that mint or cross a tenant are the operator's and are not here, so there is nothing for a
   * reader to choose. It is sent rather than omitted because the server refuses a request that
   * does not name it (`backup.restore_tenant_required`).
   */
  readonly target_tenant_id?: string;
  readonly conflict_rule?: 'SKIP' | 'OVERWRITE' | 'DUPLICATE';
  readonly selection?: {
    readonly container_ids?: readonly string[];
    readonly item_ids?: readonly string[];
  };
  readonly create_safety_backup?: boolean;
  readonly confirmation?: string;
}
