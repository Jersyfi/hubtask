// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The level above the workspaces, as the dashboard reads and writes it (ADR-0070 §5).
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
  IdentityProvider,
  IdentityProviderConfiguration,
  ProviderWithdrawal,
  IdentityProviderPreset,
  EncryptionStatus,
  InstanceJournalEntry,
  InstanceOverview,
  InstanceSettings,
  Operator,
  PasswordOpeningRequest,
  ProvisionedTenant,
  TenantProvision,
  TenantQuotas,
  ResourceState,
  SessionElevation,
} from '@hubtask/sync-engine';

import { platform } from '../platform/index.ts';
import { engine } from './engine.ts';
import { identityProviderPresetPath } from './identityprovider.svelte.ts';
import { stepUp } from './stepup.svelte.ts';

const ELEVATE = '/auth/sessions:elevate';
const OVERVIEW = '/admin/overview';
const TENANTS = '/admin/tenants';
const SETTINGS = '/admin/settings';
const OPERATORS = '/admin/operators';
const JOURNAL = '/admin/journal';
const PROVIDERS = '/admin/identity-providers';
const ENCRYPTION = '/admin/encryption';

/** Everything a write at this level can make stale. One list, so no caller forgets half of it. */
const EVERYTHING = [OVERVIEW, TENANTS, SETTINGS, OPERATORS, JOURNAL, PROVIDERS, ENCRYPTION];

/** How often the remaining time is recomputed. A second: it is a clock somebody is reading. */
const TICK_MS = 1_000;

class Instance {
  // Read from the seam rather than started empty: the elevation belongs to the session, and a
  // reload that forgot it would send the reader back to the door while the hour still stands at the
  // server - where passing it again would start a *new* hour (ADR-0070 §4).
  #until = $state<string | undefined>(platform.elevatedUntil());
  #now = $state(Date.now());
  #ticking: ReturnType<typeof setInterval> | undefined;

  #overview = $state<ResourceState<InstanceOverview>>({ status: 'idle' });
  #workspaces = $state<ResourceState<readonly AdminTenant[]>>({ status: 'idle' });
  #settings = $state<ResourceState<InstanceSettings>>({ status: 'idle' });
  #operators = $state<ResourceState<readonly Operator[]>>({ status: 'idle' });
  #journal = $state<ResourceState<{ readonly data: readonly InstanceJournalEntry[] }>>({ status: 'idle' });
  #providers = $state<ResourceState<readonly IdentityProvider[]>>({ status: 'idle' });
  #encryption = $state<ResourceState<EncryptionStatus>>({ status: 'idle' });
  // The presets are neither the installation's nor a workspace's: they are what Hubtask knows about
  // three issuers, and the same list answers at both levels. Read through the path the workspace's
  // own store already names, so one route has one constant.
  #presets = $state<ResourceState<readonly IdentityProviderPreset[]>>({ status: 'idle' });

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

  get providers(): readonly IdentityProvider[] {
    return this.#providers.status === 'ready' ? this.#providers.data : [];
  }

  get providersState(): ResourceState<readonly IdentityProvider[]> {
    return this.#providers;
  }

  get encryption(): ResourceState<EncryptionStatus> {
    return this.#encryption;
  }

  /**
   * Raises this session for an hour.
   *
   * Through the step-up wrapper rather than with a token this module asked for: the call is refused
   * without a proof, and the wrapper is the one place that knows what to do with that refusal.
   */
  /**
   * Forgets the elevation, because the session it belonged to is gone.
   *
   * This store is a module singleton and its clock was not: signing out and back in inside one page
   * load left `#until` where it was, so the area drew itself with a live countdown over a session
   * the server refuses every call from — "Du arbeitest an der Installation. Noch 17 Minuten" beside
   * "Das verwendete Token darf das nicht". Worse, it did that for **whoever signed in next**,
   * which is a control plane drawn for somebody who was never offered it.
   *
   * An elevation belongs to one session (ADR-0070 §4: "hängt an dieser einen Sitzung, endet mit
   * ihr"), so every path that ends or replaces a session ends this too.
   */
  forget(): void {
    this.#until = undefined;
    this.#overview = { status: 'idle' };
    this.#workspaces = { status: 'idle' };
    this.#settings = { status: 'idle' };
    this.#operators = { status: 'idle' };
    this.#journal = { status: 'idle' };
    this.#providers = { status: 'idle' };
    this.#encryption = { status: 'idle' };
  }

  async elevate(): Promise<void> {
    const raised = await stepUp.around((stepUpToken) =>
      engine.mutate<SessionElevation>(
        'POST',
        ELEVATE,
        undefined,
        { stepUpToken, invalidates: EVERYTHING },
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

  /**
   * The installation's providers and the presets behind them, together.
   *
   * One subscription for both because the form cannot be drawn without the second: a kind whose
   * preset has not arrived is a kind whose permitted modes are unknown, and offering all three
   * would be guessing in the permissive direction.
   */
  openProviders(): () => void {
    const stopProviders = engine.subscribe<readonly IdentityProvider[]>({ path: PROVIDERS }, (next) => {
      this.#providers = next;
    });
    const stopPresets = engine.subscribe<readonly IdentityProviderPreset[]>(
      { path: identityProviderPresetPath },
      (next) => {
        this.#presets = next;
      },
    );
    return () => {
      stopProviders();
      stopPresets();
    };
  }

  /**
   * The keyring's census.
   *
   * **Read only, and that is the concept's own exception** (§5.7): "Der Schlüsselring bleibt in der
   * Umgebung und in `/admin/encryption`; ein Dashboard zeigt seinen Zustand und dreht ihn nicht."
   * A rotation is an operator at a terminal with the new key in their hand, not a button.
   */
  /** What each kind takes to register, and which admission modes it permits. */
  get presets(): readonly IdentityProviderPreset[] {
    return this.#presets.status === 'ready' ? this.#presets.data : [];
  }

  openEncryption(): () => void {
    return engine.subscribe<EncryptionStatus>({ path: ENCRYPTION }, (next) => {
      this.#encryption = next;
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
  /**
   * Registers an operator, named the way a person can name one: the workspace, and the address.
   *
   * Not the account id. `account` is behind row level security, so the control plane cannot list
   * accounts across workspaces and therefore cannot offer one to pick — which left the screen
   * asking somebody to type a UUID they would have had to get out of the database by hand. The
   * contract keeps the identifier form for a script that already has one.
   */
  async addOperator(workspace: string, email: string): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('POST', OPERATORS, { workspace, email }, {
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

  /** Suspends a workspace or resumes one. Both are audited and journalled. */
  async shift(tenantId: string, action: 'suspend' | 'resume'): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('POST', `${TENANTS}/${tenantId}:${action}`, undefined, {
        stepUpToken,
        invalidates: EVERYTHING,
      }),
    );
  }

  /**
   * Provisions a workspace, and answers the owner's way in.
   *
   * The redemption token is returned rather than held: it exists in this one answer and nowhere
   * else, the screen shows it through `OneTimeSecret`, and it dies with that screen. A store that
   * kept it would be a store holding a credential for as long as the tab is open.
   */
  async provision(request: TenantProvision): Promise<ProvisionedTenant> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<ProvisionedTenant>('POST', TENANTS, request, {
        idempotencyKey: crypto.randomUUID(),
        stepUpToken,
        invalidates: EVERYTHING,
      }),
    );
  }

  /**
   * Asks for a workspace to be deleted, which starts the grace period.
   *
   * The display name is typed by the person asking and is sent as the confirmation — the server
   * compares it, which is the whole point: a deletion nobody typed the name of is a deletion
   * somebody clicked past.
   */
  async requestDeletion(tenantId: string, confirmation: string): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('POST', `${TENANTS}/${tenantId}:delete`, { confirmation }, {
        stepUpToken,
        invalidates: EVERYTHING,
      }),
    );
  }

  /**
   * Opens the password for one workspace for a while (ADR-0078 §3): for a provider that is switched
   * on but broken. Behind a step-up, like every act that widens a way in; the hours, the requester
   * and the reason are refused before the proof is asked for, so a typo costs no proof.
   */
  async openPassword(tenantId: string, request: PasswordOpeningRequest): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('POST', `${TENANTS}/${tenantId}:open-password`, request, {
        stepUpToken,
        invalidates: EVERYTHING,
      }),
    );
  }

  /** Ends an opening before its time. No step-up: it narrows the way in. */
  async closePassword(tenantId: string): Promise<void> {
    await engine.mutate('POST', `${TENANTS}/${tenantId}:close-password`, undefined, {
      invalidates: EVERYTHING,
    });
  }

  /** Exports a workspace to one of its backup targets. Answered `202`: the work is a job. */
  async exportWorkspace(tenantId: string, targetId: string): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('POST', `${TENANTS}/${tenantId}:export`, { target_id: targetId }, {
        stepUpToken,
        invalidates: [JOURNAL, OVERVIEW],
      }),
    );
  }

  /** Sets a workspace's quotas. An absent number is the installation's default, not zero. */
  async setQuotas(tenantId: string, quotas: TenantQuotas): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('PATCH', `${TENANTS}/${tenantId}/quotas`, quotas, {
        stepUpToken,
        invalidates: EVERYTHING,
      }),
    );
  }

  /**
   * Writes the installation's own values.
   *
   * Refused while a file enforces them, which the answer says with `is_enforced_from_file` — the
   * screen reads that and offers no controls rather than offering a refusal (ADR-0070 §5: one API,
   * three doors, and one source per mode).
   */
  async writeSettings(settings: InstanceSettings): Promise<InstanceSettings> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<InstanceSettings>('PUT', SETTINGS, settings, {
        stepUpToken,
        invalidates: EVERYTHING,
      }),
    );
  }

  /** Adds a provider the installation offers every workspace, or replaces one. */
  async configureProvider(
    body: IdentityProviderConfiguration,
    id?: string,
  ): Promise<IdentityProvider> {
    return stepUp.around((stepUpToken) =>
      id === undefined
        ? engine.mutate<IdentityProvider>('POST', PROVIDERS, body, {
            stepUpToken,
            invalidates: EVERYTHING,
          })
        : engine.mutate<IdentityProvider>('PUT', `${PROVIDERS}/${id}`, body, {
            stepUpToken,
            invalidates: EVERYTHING,
          }),
    );
  }

  /**
   * Announces the end of an offer, or ends it now (ADR-0076 §2-3).
   *
   * A body with no date is the server's fourteen days. A date that has come is *Withdraw now*, and
   * the body must carry the count as the operator just read it - the server compares it inside the
   * same transaction, so a workspace that switched the provider on since makes it wrong, not unseen.
   */
  async withdrawProvider(id: string, body: ProviderWithdrawal): Promise<IdentityProvider> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<IdentityProvider>('POST', `${PROVIDERS}/${id}:withdraw`, body, {
        stepUpToken,
        invalidates: EVERYTHING,
      }),
    );
  }

  /** Keeps offering it - before the date, or after it, which restores the workspaces' sign-in. */
  async cancelWithdrawal(id: string): Promise<IdentityProvider> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<IdentityProvider>('POST', `${PROVIDERS}/${id}:cancel-withdrawal`, {}, {
        stepUpToken,
        invalidates: EVERYTHING,
      }),
    );
  }

  /**
   * Removes one from every workspace at once: the row, not only the offer. A workspace left with no
   * way in falls back to the password for the accounts that hold one (ADR-0076 §4).
   */
  async removeProvider(id: string): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('DELETE', `${PROVIDERS}/${id}`, undefined, {
        stepUpToken,
        invalidates: EVERYTHING,
      }),
    );
  }
}

export const instance = new Instance();
export {
  ENCRYPTION as instanceEncryptionPath,
  PROVIDERS as instanceProvidersPath,
  ELEVATE as instanceElevatePath,
  JOURNAL as instanceJournalPath,
  OPERATORS as instanceOperatorsPath,
  OVERVIEW as instanceOverviewPath,
  SETTINGS as instanceSettingsPath,
  TENANTS as instanceWorkspacesPath,
};
