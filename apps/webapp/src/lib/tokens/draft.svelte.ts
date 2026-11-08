// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A token being made (`security.md` §5): what *New token* on one's own page and a service account's
 * *New token* dialog both fill, so the three rules hold in both.
 *
 * **Three rules a token's making must not soften.** The expiry has no default and nothing is sent
 * without one - every default that could be offered is either so short that people work around it
 * or so long that it is the eternal credential the rule exists to prevent. The scopes are chosen,
 * never pre-chosen as everything. And an admin scope demands a fresh proof, which arrives as the
 * step-up prompt the wrapper raises - this draft does not decide when, because which scopes are
 * privileged is the server's rule.
 */

import { manifest } from '../data/capabilities.svelte.ts';
import { tokens, type MintedToken } from '../data/tokens.svelte.ts';
import { deviceZone, endOfDayIn, todayIn } from '../i18n/zone.ts';
import { announcer } from '../announce.svelte.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';
import { renderProblem } from '../problem.ts';
import { check, isComplete, type Missing } from '../forms/missing.ts';

/**
 * The scopes the operator register guards (identity.md §19): the control plane's, `admin:tenants`
 * and its subset for a purchase platform, both named by this prefix in the contract.
 */
const OPERATOR_SCOPE_PREFIX = 'admin:';
const PROVISIONING_SCOPE = 'admin:provisioning';
const OPERATOR_SCOPE = 'admin:tenants';
/** The installation choice's "nothing": a value no scope can have. */
export const NO_INSTALLATION = 'none';

export class TokenDraft {
  readonly accountId: string | undefined;
  name = $state('');
  expiresAt = $state('');
  chosen = $state<string[]>([]);
  installation = $state(NO_INSTALLATION);
  failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** What *Create token* found missing when it was pressed, by field; each mark goes with a change. */
  missing = $state<Missing>({});
  isWorking = $state(false);

  /** A service account's token, where the caller may mint for one; absent means one's own. */
  constructor(accountId?: string) {
    this.accountId = accountId;
    this.installation = this.#installationDefault();
  }

  // The installation is one choice rather than two boxes, because the two scopes are a subset and
  // its whole. A service account's draft preselects the narrow scope - it is how a purchase
  // platform's token is made - and a person's own preselects nothing, so an ordinary token stays one.
  #installationDefault(): string {
    return this.accountId ? PROVISIONING_SCOPE : NO_INSTALLATION;
  }

  get hasChanges(): boolean {
    return this.name.trim() !== '' || this.expiresAt !== '' || this.chosen.length > 0;
  }

  /**
   * Every scope this installation declares, from the manifest and never a list written here - less
   * the operator's scopes for somebody the operator register does not name: minting one asks that
   * register as well as a fresh proof (identity.md §19), and offering it there would be a proof
   * given into a certain refusal (P-05).
   */
  get declared(): readonly string[] {
    return manifest.value?.token_scopes ?? [];
  }

  get scopes(): string[] {
    return this.declared.filter((scope) => !scope.startsWith(OPERATOR_SCOPE_PREFIX));
  }

  get installationOptions(): { value: string; label: string; hint: string }[] {
    if (!manifest.isInstanceReachable) return [];
    return [
      { value: PROVISIONING_SCOPE, label: t('app.tokens.installation_provisioning'), hint: t('app.tokens.installation_provisioning_hint') },
      { value: OPERATOR_SCOPE, label: t('app.tokens.installation_all'), hint: t('app.tokens.installation_all_hint') },
    ].filter((option) => this.declared.includes(option.value));
  }

  /** The installation scope the token will carry, if the choice offered one and it was taken. */
  get installationScope(): string | undefined {
    return this.installationOptions.some((option) => option.value === this.installation) ? this.installation : undefined;
  }

  get requested(): string[] {
    return this.installationScope ? [...this.chosen, this.installationScope] : this.chosen;
  }

  // The days the date control offers and the day it answers are the person's, on the clock the
  // lists are drawn on. A day taken in UTC was the next day east of Greenwich.
  readonly zone = deviceZone();
  readonly today = todayIn(this.zone);

  /**
   * The furthest day the server accepts: at most 365 days from now, and a token ends at the end of
   * its day, so the last day offered is the 364th after today.
   */
  get latestAllowed(): string {
    const [year = 0, month = 1, day = 1] = this.today.split('-').map(Number);
    return new Date(Date.UTC(year, month - 1, day + 364)).toISOString().slice(0, 10);
  }

  reset(): void {
    this.name = '';
    this.expiresAt = '';
    this.chosen = [];
    this.installation = this.#installationDefault();
    this.missing = {};
    this.failure = undefined;
  }

  /** Makes the token, or marks what is missing and answers nothing. */
  async mint(): Promise<MintedToken | undefined> {
    this.missing = check([
      { kind: 'field', key: 'name', code: 'app.tokens.name_needed', isFilled: this.name.trim() !== '' },
      { kind: 'field', key: 'expires', code: 'app.tokens.expiry_needed', isFilled: Boolean(this.expiresAt) },
      { kind: 'at-least-one', key: 'scopes', code: 'app.tokens.pick_a_scope', chosen: this.requested.length },
    ]);
    if (!isComplete(this.missing)) return undefined;

    this.failure = undefined;
    this.isWorking = true;
    try {
      // The date control answers a day; the contract wants an instant: the end of that day where
      // the person is, so a token chosen for "the 31st" works through their 31st and is listed so.
      const minted = await tokens.mint({
        name: this.name.trim(),
        scopes: this.requested,
        expiresAt: endOfDayIn(this.expiresAt, this.zone) ?? `${this.expiresAt}T23:59:59Z`,
        accountId: this.accountId,
      });
      announcer.say(t('app.tokens.minted_announced'));
      this.reset();
      return minted;
    } catch (cause) {
      this.failure = renderProblem(cause as never, messages);
      return undefined;
    } finally {
      this.isWorking = false;
    }
  }
}
