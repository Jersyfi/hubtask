// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where the archives go, when they are written, what one run did, and whether it opens.
 *
 * Two listings and one job each. `POST /backups` and `POST /backups/{id}:verify` answer a `JobRef`
 * rather than a result, so both hand the reference straight to `jobs.svelte.ts` — this module
 * starts work and never polls, because a polling loop written here would be the second copy of
 * the one that already exists.
 *
 * **A credential is written and never read.** `BackupTarget` carries `config` without them and
 * `BackupTargetCreate` takes `credentials` beside it; nothing in this module reads one back, and
 * nothing can, which is the same shape the OIDC client secret has.
 *
 * **What has been backed up is read at the target, not from the database**, and that is the
 * contract's shape rather than a choice here: there is no listing of runs, and
 * `/backup-targets/{id}/backups` reads the manifests where the archives lie — so it still answers
 * after a total loss, which is exactly the moment somebody needs it. A run's own row
 * (`GET /backups/{id}`) is what carries `verified_at`, and an archive's id **is** its run's id,
 * so one listing plus one read per row needs no mapping in between.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { jobs } from './jobs.svelte.ts';
import type { JobRef } from './jobs.ts';
import type { Retention } from './backup.ts';

const TARGETS = '/backup-targets';
const SCHEDULES = '/backup-schedules';
const RUNS = '/backups';

/** Where an archive may be written. The manifest's own vocabulary. */
export type TargetKind =
  | 'LOCAL' | 'S3' | 'SFTP' | 'FTPS' | 'FTP' | 'WEBDAV' | 'SMB' | 'AZURE_BLOB' | 'GCS' | 'RCLONE' | 'HTTP_PUT';

/** One target, as the contract answers it — never with the credentials it was created with. */
export interface Target {
  readonly id: string;
  readonly name: string;
  readonly kind: TargetKind;
  readonly scope?: string;
  readonly config?: Readonly<Record<string, unknown>>;
  readonly encryption_mode: string;
  readonly encryption_key_id?: string | null;
  readonly region_note?: string | null;
  readonly enabled?: boolean;
  readonly last_test_at?: string | null;
  readonly last_test_ok?: boolean | null;
  /** What the target says about itself, as codes: `backup.target_unencrypted` and its kind. */
  readonly warnings?: readonly string[];
}

/** What a write-read-delete against the target found. Unreachable is a result, not an error. */
export interface Probe {
  readonly ok: boolean;
  readonly latency_ms: number;
  readonly writable: boolean;
  readonly free_bytes?: number | null;
  readonly error_code?: string | null;
}

/** What lies at the target, read from its manifests rather than from the database. */
export interface Archive {
  readonly archive_id: string;
  readonly path: string;
  readonly created_at: string;
  readonly mode: string;
  readonly parent_archive_id?: string | null;
  readonly scope?: { readonly kind?: string; readonly id?: string | null; readonly label?: string | null };
  readonly size_bytes?: number;
  readonly item_count?: number;
  readonly media_count?: number;
  readonly encrypted?: boolean;
  readonly checksum_status?: string;
  readonly complete?: boolean;
  readonly expires_at?: string | null;
}

/** One schedule: a rule, a zone, a plan, and a switch. */
export interface Schedule {
  readonly id: string;
  readonly target_id: string;
  readonly scope?: { readonly kind?: string; readonly id?: string | null };
  readonly rrule: string;
  readonly timezone?: string;
  readonly mode?: string;
  readonly full_rrule?: string | null;
  readonly include_media?: boolean;
  readonly include_audit?: boolean;
  readonly retention?: Retention;
  readonly notify_on?: readonly string[];
  readonly enabled?: boolean;
  /** Absent for a schedule that is off, and for one whose rule is spent. Neither is an error. */
  readonly next_run_at?: string | null;
}

/** One run of a backup. Its id is also the archive's id in the manifest at the target. */
export interface Run {
  readonly id: string;
  readonly target_id: string;
  readonly schedule_id?: string | null;
  readonly parent_run_id?: string | null;
  readonly trigger: string;
  readonly mode: string;
  readonly status: string;
  readonly archive_path?: string | null;
  readonly size_bytes?: number | null;
  readonly item_count?: number | null;
  readonly media_count?: number | null;
  readonly snapshot_at?: string | null;
  readonly started_at: string;
  readonly finished_at?: string | null;
  readonly error_code?: string | null;
  readonly expires_at?: string | null;
  readonly verified_at?: string | null;
  readonly verify_ok?: boolean | null;
}

/** What creating a target takes. `credentials` goes in and never comes back. */
export interface TargetDraft {
  readonly name: string;
  readonly kind: TargetKind;
  readonly config: Readonly<Record<string, unknown>>;
  readonly credentials?: Readonly<Record<string, unknown>>;
  readonly encryption_mode?: string;
  readonly insecure_acknowledged?: boolean;
}

class Backup {
  #targets = $state<ResourceState<readonly Target[]>>({ status: 'idle' });
  #schedules = $state<ResourceState<readonly Schedule[]>>({ status: 'idle' });
  #probes = $state<Record<string, Probe>>({});
  #archives = $state<Record<string, ResourceState<readonly Archive[]>>>({});
  #runs = $state<Record<string, Run>>({});

  get targets(): ResourceState<readonly Target[]> {
    return this.#targets;
  }

  get all(): readonly Target[] {
    return this.#targets.status === 'ready' ? this.#targets.data : [];
  }

  get schedules(): ResourceState<readonly Schedule[]> {
    return this.#schedules;
  }

  /** What the last probe of one target found. Held per target, and never persisted. */
  probeOf(targetId: string): Probe | undefined {
    return this.#probes[targetId];
  }

  /** What lies at one target, once somebody has asked. */
  archivesAt(targetId: string): ResourceState<readonly Archive[]> {
    return this.#archives[targetId] ?? { status: 'idle' };
  }

  /** One run, by the id it shares with its archive. What carries `verified_at`. */
  runOf(runId: string): Run | undefined {
    return this.#runs[runId];
  }

  /** Starts the two listings. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    const stopTargets = engine.subscribe<readonly Target[]>({ path: TARGETS }, (next) => {
      this.#targets = next;
    });
    const stopSchedules = engine.subscribe<readonly Schedule[]>({ path: SCHEDULES }, (next) => {
      this.#schedules = next;
    });
    return () => {
      stopTargets();
      stopSchedules();
    };
  }

  async createTarget(draft: TargetDraft): Promise<Target> {
    return engine.mutate<Target>('POST', TARGETS, draft, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [TARGETS],
    });
  }

  /**
   * Removes a target. Nothing at the target is touched, and a target a schedule names is refused.
   *
   * The refusal is a `409` naming the schedules, and the screen renders it: disarming a nightly
   * backup silently is the kind of thing discovered by whoever needed the archive.
   */
  async deleteTarget(targetId: string): Promise<void> {
    await engine.mutate<void>('DELETE', `${TARGETS}/${targetId}`, undefined, {
      invalidates: [TARGETS],
    });
  }

  /** A write-read-delete probe, through the server's guarded client. Held for the screen. */
  async test(targetId: string): Promise<Probe> {
    const probe = await engine.mutate<Probe>('POST', `${TARGETS}/${targetId}:test`, {}, {
      invalidates: [TARGETS],
    });
    this.#probes = { ...this.#probes, [targetId]: probe };
    return probe;
  }

  /**
   * Reads the manifests at the target. Works after a total loss; the database is not consulted.
   *
   * `refresh` bypasses the server's cache, which is what a reader asks for after a run they
   * watched finish — the cached answer is from before it.
   */
  async readArchives(targetId: string, refresh = false): Promise<void> {
    const path = `${TARGETS}/${targetId}/backups${refresh ? '?refresh=true' : ''}`;
    this.#archives = { ...this.#archives, [targetId]: { status: 'loading' } };
    const answer = await engine.refresh<readonly Archive[]>({ path });
    this.#archives = { ...this.#archives, [targetId]: answer };
  }

  /** Reads one run's row — the half of an archive that says whether it has been opened since. */
  async readRun(runId: string): Promise<void> {
    const answer = await engine.refresh<Run>({ path: `${RUNS}/${runId}` });
    if (answer.status === 'ready') this.#runs = { ...this.#runs, [runId]: answer.data };
  }

  async createSchedule(draft: Omit<Schedule, 'id' | 'next_run_at'>): Promise<Schedule> {
    return engine.mutate<Schedule>('POST', SCHEDULES, draft, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [SCHEDULES],
    });
  }

  /**
   * Changes a schedule. Merge-patch: an absent key changes nothing.
   *
   * `enabled: false` is the operation that matters and the reason a delete is not the only
   * answer — it keeps what somebody worked out about hours, zones and generations.
   */
  async updateSchedule(scheduleId: string, change: Record<string, unknown>): Promise<Schedule> {
    return engine.mutate<Schedule>('PATCH', `${SCHEDULES}/${scheduleId}`, change, {
      invalidates: [SCHEDULES],
    });
  }

  async deleteSchedule(scheduleId: string): Promise<void> {
    await engine.mutate<void>('DELETE', `${SCHEDULES}/${scheduleId}`, undefined, {
      invalidates: [SCHEDULES],
    });
  }

  /**
   * Starts a backup and hands the job to the watcher.
   *
   * `FULL` by default is the contract's own choice and worth not overriding: a run somebody asks
   * for by hand is usually asked for because something is about to happen.
   */
  async start(request: {
    target_id: string;
    mode?: string;
    include_media?: boolean;
    include_audit?: boolean;
  }): Promise<JobRef> {
    const accepted = await engine.mutate<JobRef>('POST', RUNS, request, {
      idempotencyKey: crypto.randomUUID(),
    });
    jobs.watch(accepted);
    return accepted;
  }

  /**
   * Checks the checksums and the decryptability at the target, without restoring anything.
   *
   * The answer lands on the run as `verified_at` and `verify_ok`, so what the caller does when
   * the job ends is read the run again — "we have backups" and "we have backups that open" are
   * different claims and they are stored in different fields.
   */
  async verify(runId: string): Promise<JobRef> {
    const accepted = await engine.mutate<JobRef>('POST', `${RUNS}/${runId}:verify`, {}, {
      idempotencyKey: crypto.randomUUID(),
    });
    jobs.watch(accepted);
    return accepted;
  }

  /**
   * The run a `JobRef` points at, taken from `result_url`.
   *
   * The last segment of the path the server named, and nothing is assumed about the rest of it:
   * the contract says `result_url` is where the result can be fetched, and for both of these
   * operations that is `/backups/{id}`. A `JobRef` without one answers nothing rather than a
   * guess.
   */
  runIdOf(accepted: JobRef): string | undefined {
    const url = accepted.result_url;
    if (!url) return undefined;
    const segment = url.split('?')[0]?.split('/').pop();
    return segment === '' ? undefined : segment;
  }
}

export const backup = new Backup();
