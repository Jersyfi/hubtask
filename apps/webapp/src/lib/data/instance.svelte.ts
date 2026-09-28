// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The level above the workspaces, as the dashboard reads and writes it (SI-17, ADR-0070 §5).
 *
 * **The elevation is the door, and it is visible.** `admin:tenants` is carried by no session until a
 * registered operator raises their own for an hour by passing a fresh step-up. So this module holds
 * one thing no other store does: when that hour ends. It ticks, because an hour nobody can see the
 * end of is an hour somebody is surprised by, and when it reaches zero the area stops drawing and
 * the reader is returned to the application.
 *
 * **It does not slide.** Activity extends a session's own horizon and never this; a second hour
 * needs a second proof. Nothing here refreshes the elevation behind the reader's back.
 *
 * **Counts, states and limits — never rows.** Every read here is the control plane's own, and none
 * of them can reach into a workspace: the boundary is a database policy rather than a role, and the
 * dashboard does not go around it.
 */

import type {
  AdminTenant,
  InstanceJournalEntry,
  InstanceOverview,
  InstanceSettings,
  Operator,
  ResourceState,
  SessionElevation,
} from '@hubtask/sync-engine';

import { platform } from '../platform/index.ts';
import { engine } from './engine.ts';
import { stepUp } from './stepup.svelte.ts';

const ELEVATE = '/auth/sessions:elevate';
const OVERVIEW = '/admin/overview';
const TENANTS = '/admin/tenants';
const SETTINGS = '/admin/settings';
const OPERATORS = '/admin/operators';
const JOURNAL = '/admin/journal';

/** How often the remaining time is recomputed. A second: it is a clock somebody is reading. */
const TICK_MS = 1_000;

class Instance {
  // Read from the seam rather than started empty: the elevation belongs to the session, and a
  // reload that forgot it would send the reader back to the door while the hour still stands at the
  // server - where passing it again would start a *new* hour (SI-17, ADR-0070 §4).
  #until = $state<string | undefined>(platform.elevatedUntil());
  #now = $state(Date.now());
  #ticking: ReturnType<typeof setInterval> | undefined;

  #overview = $state<ResourceState<InstanceOverview>>({ status: 'idle' });
  #workspaces = $state<ResourceState<readonly AdminTenant[]>>({ status: 'idle' });
  #settings = $state<ResourceState<InstanceSettings>>({ status: 'idle' });
  #operators = $state<ResourceState<readonly Operator[]>>({ status: 'idle' });
  #journal = $state<ResourceState<{ readonly data: readonly InstanceJournalEntry[] }>>({ status: 'idle' });

  /** Whether this session carries the control plane's scope right now. */
  get isElevated(): boolean {
    return this.remainingSeconds > 0;
  }

  /** What is left of the hour, in seconds, and zero where there is no elevation. */
  get remainingSeconds(): number {
    if (this.#until === undefined) return 0;
    const left = Math.floor((Date.parse(this.#until) - this.#now) / 1000);
    return left > 0 ? left : 0;
  }

  get overview(): ResourceState<InstanceOverview> {
    return this.#overview;
  }

  get workspaces(): readonly AdminTenant[] {
    return this.#workspaces.status === 'ready' ? this.#workspaces.data : [];
  }

  get workspacesState(): ResourceState<readonly AdminTenant[]> {
    return this.#workspaces;
  }

  get settings(): ResourceState<InstanceSettings> {
    return this.#settings;
  }

  get operators(): readonly Operator[] {
    return this.#operators.status === 'ready' ? this.#operators.data : [];
  }

  get operatorsState(): ResourceState<readonly Operator[]> {
    return this.#operators;
  }

  get journal(): readonly InstanceJournalEntry[] {
    return this.#journal.status === 'ready' ? this.#journal.data.data : [];
  }

  get journalState(): ResourceState<{ readonly data: readonly InstanceJournalEntry[] }> {
    return this.#journal;
  }

  /**
   * Raises this session for an hour.
   *
   * Through the step-up wrapper rather than with a token this module asked for: the call is refused
   * without a proof, and the wrapper is the one place that knows what to do with that refusal.
   */
  async elevate(): Promise<void> {
    const raised = await stepUp.around((stepUpToken) =>
      engine.mutate<SessionElevation>(
        'POST',
        ELEVATE,
        undefined,
        { stepUpToken, invalidates: [OVERVIEW, TENANTS, SETTINGS, OPERATORS, JOURNAL] },
      ),
    );
    this.#until = raised.elevated_until;
    this.#now = Date.now();
    platform.rememberElevation(raised.elevated_until);
  }

  /**
   * Starts the clock. **From `untrack`**, for the reason every other store records.
   *
   * The interval is the whole of it: the elevation itself is the server's, and this only reads how
   * much of it is left. Nothing here asks for another hour.
   */
  open(): () => void {
    this.#now = Date.now();
    this.#ticking ??= setInterval(() => {
      this.#now = Date.now();
    }, TICK_MS);
    return () => {
      if (this.#ticking === undefined) return;
      clearInterval(this.#ticking);
      this.#ticking = undefined;
    };
  }

  /** The dashboard's first screen: counts and states, never rows. */
  openOverview(): () => void {
    return engine.subscribe<InstanceOverview>({ path: OVERVIEW }, (next) => {
      this.#overview = next;
    });
  }

  openWorkspaces(): () => void {
    return engine.subscribe<readonly AdminTenant[]>({ path: TENANTS }, (next) => {
      this.#workspaces = next;
    });
  }

  openSettings(): () => void {
    return engine.subscribe<InstanceSettings>({ path: SETTINGS }, (next) => {
      this.#settings = next;
    });
  }

  openOperators(): () => void {
    return engine.subscribe<readonly Operator[]>({ path: OPERATORS }, (next) => {
      this.#operators = next;
    });
  }

  openJournal(): () => void {
    return engine.subscribe<{ readonly data: readonly InstanceJournalEntry[] }>(
      { path: JOURNAL },
      (next) => {
        this.#journal = next;
      },
    );
  }

  /** Adds somebody to the register. The workspace comes from the account, never from the caller. */
  async addOperator(accountId: string): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('POST', OPERATORS, { account_id: accountId }, {
        stepUpToken,
        invalidates: [OPERATORS, JOURNAL],
      }),
    );
  }

  /**
   * Takes somebody out of it.
   *
   * The last operator cannot remove themselves, and the database refuses that in the statement
   * rather than in a read-then-write: two operators removing each other at the same moment would
   * both have read "there are two".
   */
  async removeOperator(accountId: string): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('DELETE', `${OPERATORS}/${accountId}`, undefined, {
        stepUpToken,
        invalidates: [OPERATORS, JOURNAL],
      }),
    );
  }

  /** Suspends a workspace, resumes one, or asks for its deletion. Each is audited and journalled. */
  async shift(tenantId: string, action: 'suspend' | 'resume'): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate(
        'POST',
        `${TENANTS}/${tenantId}:${action}`,
        undefined,
        { stepUpToken, invalidates: [TENANTS, OVERVIEW, JOURNAL] },
      ),
    );
  }
}

export const instance = new Instance();
export {
  ELEVATE as instanceElevatePath,
  JOURNAL as instanceJournalPath,
  OPERATORS as instanceOperatorsPath,
  OVERVIEW as instanceOverviewPath,
  SETTINGS as instanceSettingsPath,
  TENANTS as instanceWorkspacesPath,
};
