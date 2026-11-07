<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The form that makes a token (`security.md` §5): on a person's own *Access tokens* page, and in
  // a service account's *New token* dialog. One form, so the three rules below hold in both.
  //
  // **Three rules this form must not soften.** The expiry has no default and the form cannot be
  // submitted without one — every default that could be offered is either so short that people
  // work around it or so long that it is the eternal credential the rule exists to prevent. The
  // scopes are ticked one at a time, never pre-ticked as everything. And an admin scope demands a
  // fresh proof, which arrives as the step-up prompt the wrapper raises — this form does not
  // decide when, because which scopes are privileged is the server's rule.
  //
  // The credential it makes is handed to the caller (`onminted`), which shows it once.

  import { untrack } from 'svelte';

  import { Button, CheckboxGroup, Input, Radio, Stack } from '@hubtask/design-system/components';

  import FailureNote from '../frame/FailureNote.svelte';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { tokens, type MintedToken } from '../data/tokens.svelte.ts';
  import { deviceZone, endOfDayIn, todayIn } from '../i18n/zone.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { check, focusFirst, isComplete, without, type Missing } from '../forms/missing.ts';
  import { scopeWord } from './tokenwords.ts';

  interface Props {
    /** A service account's token, where the caller may mint for one. Absent means one's own. */
    accountId?: string;
    onminted: (minted: MintedToken) => void;
    /** In a dialog the dialog's primary submits it (`form.requestSubmit()`), so it draws no button. */
    isInDialog?: boolean;
    form?: HTMLFormElement | null;
    isWorking?: boolean;
    /** Whether anything is typed or ticked: what a dialog's discard guard asks. */
    hasChanges?: boolean;
  }

  let { accountId, onminted, isInDialog = false, form = $bindable(null), isWorking = $bindable(false), hasChanges = $bindable(false) }: Props = $props();

  /**
   * The scopes the operator register guards (identity.md §19): the control plane's, `admin:tenants`
   * and its subset for a purchase platform, both named by this prefix in the contract.
   */
  const OPERATOR_SCOPE_PREFIX = 'admin:';
  const PROVISIONING_SCOPE = 'admin:provisioning';
  const OPERATOR_SCOPE = 'admin:tenants';
  /** The installation choice's "nothing": a value no scope can have. */
  const NO_INSTALLATION = 'none';

  // The installation is one choice rather than two checkboxes, because the two scopes are a subset
  // and its whole: ticking both says nothing more than the whole. A service account's form
  // preselects the narrow scope - it is how a purchase platform's token is made - and a person's
  // own form preselects nothing, so an ordinary token stays one (the owner's word on the preview).
  const installationDefault = $derived(accountId ? PROVISIONING_SCOPE : NO_INSTALLATION);

  let name = $state('');
  let expiresAt = $state('');
  let chosen = $state<string[]>([]);
  let installation = $state(untrack(() => installationDefault));
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  $effect(() => {
    hasChanges = name.trim() !== '' || expiresAt !== '' || chosen.length > 0;
  });

  /**
   * Every scope this installation declares, from the manifest and never a list written here - less
   * the operator's scopes for somebody the operator register does not name. Minting one of them
   * asks that register as well as a fresh proof (identity.md §19), and `instance.reachable` is the
   * register's answer for this reader; offering it there would be a proof given into a certain
   * refusal (P-05). The register decides whose mint it is - the person creating the token - so a
   * service account's form follows the same answer.
   */
  const declared = $derived(manifest.value?.token_scopes ?? []);
  const scopes = $derived(declared.filter((scope) => !scope.startsWith(OPERATOR_SCOPE_PREFIX)));

  const installationOptions = $derived(
    manifest.isInstanceReachable
      ? [
          { value: PROVISIONING_SCOPE, label: t('app.tokens.installation_provisioning'), hint: t('app.tokens.installation_provisioning_hint') },
          { value: OPERATOR_SCOPE, label: t('app.tokens.installation_all'), hint: t('app.tokens.installation_all_hint') },
        ].filter((option) => declared.includes(option.value))
      : [],
  );

  /** The installation scope the token will carry, if the choice offered one and it was taken. */
  const installationScope = $derived(
    installationOptions.some((option) => option.value === installation) ? installation : undefined,
  );

  const requested = $derived(installationScope ? [...chosen, installationScope] : chosen);

  // The days the date control offers and the day it answers are the person's, on the clock the
  // list beside it is drawn on (`formatDateTime` reads an instant in the browser's zone). A day
  // taken in UTC was the next day east of Greenwich: "the 31st" was listed as the 1st.
  const zone = deviceZone();
  const today = todayIn(zone);

  /**
   * The furthest day the server accepts: it takes at most 365 days from now, and a token ends at
   * the end of its day, so the last day offered is the 364th after today - the 365th would end
   * past the limit and be refused after the person had chosen it.
   */
  function latestAllowed(): string {
    const [year = 0, month = 1, day = 1] = today.split('-').map(Number);
    return new Date(Date.UTC(year, month - 1, day + 364)).toISOString().slice(0, 10);
  }

  /**
   * What *Create token* found missing when it was pressed, by field: never switched off for it
   * (design-system.md §4, case 3). Each mark goes when its field changes.
   */
  let missing = $state<Missing>({});

  async function mint(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    missing = check([
      { kind: 'field', key: 'name', code: 'app.tokens.name_needed', isFilled: name.trim() !== '' },
      { kind: 'field', key: 'expires', code: 'app.tokens.expiry_needed', isFilled: Boolean(expiresAt) },
      { kind: 'at-least-one', key: 'scopes', code: 'app.tokens.pick_a_scope', chosen: requested.length },
    ]);
    if (!isComplete(missing)) {
      queueMicrotask(() => focusFirst(form));
      return;
    }

    failure = undefined;
    isWorking = true;
    try {
      // The date control answers a day; the contract wants an instant: the end of that day where
      // the person is, so a token chosen for "the 31st" works through their 31st and is listed so.
      const minted = await tokens.mint({
        name: name.trim(),
        scopes: requested,
        expiresAt: endOfDayIn(expiresAt, zone) ?? `${expiresAt}T23:59:59Z`,
        accountId,
      });
      announcer.say(t('app.tokens.minted_announced'));
      name = '';
      expiresAt = '';
      chosen = [];
      installation = installationDefault;
      onminted(minted);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<!-- `novalidate`: what is missing is said by the form itself, in its own words, when it is
     pressed - not by the browser's bubble (design-system.md §4, case 3). -->
<form onsubmit={mint} bind:this={form} novalidate>
  <Stack gap="200">
    {#if failure}<FailureNote problem={failure} />{/if}
    <Input
      label={t('app.tokens.name')}
      hint={t('app.tokens.name_hint')}
      bind:value={name}
      isRequired
      oninput={() => (missing = without(missing, 'name'))}
      error={missing.name ? t(missing.name) : undefined}
    />

    <!-- `max` as well as `required`: the server refuses more than a year, and being told
         before the round trip is the same number said earlier rather than a second policy. -->
    <Input
      label={t('app.tokens.expires')}
      hint={t('app.tokens.expires_hint')}
      bind:value={expiresAt}
      oninput={() => (missing = without(missing, 'expires'))}
      error={missing.expires ? t(missing.expires) : undefined}
      type="date"
      min={today}
      max={latestAllowed()}
      isRequired
    />

    {#if scopes.length === 0}
      <!-- The manifest has not arrived. Offering nothing is honest; offering a list written
           here would be wrong on somebody's installation. -->
      <Stack gap="050">
        <span class="label">{t('app.tokens.scopes')}</span>
        <p class="quiet small">{t('app.tokens.scopes_unknown')}</p>
      </Stack>
    {:else}
      <!-- Nothing said in advance when none is ticked: *Create token* says it if it is pressed so. -->
      <CheckboxGroup
        label={t('app.tokens.scopes')}
        hint={t('app.tokens.scopes_hint')}
        options={scopes.map((scope) => ({ value: scope, label: scopeWord(scope), hint: scopeWord(scope) === scope ? undefined : scope }))}
        selected={chosen}
        error={missing.scopes ? t(missing.scopes) : undefined}
        onChange={(next) => {
          chosen = [...next];
          missing = without(missing, 'scopes');
        }}
      />
    {/if}

    {#if installationOptions.length > 0}
      <!-- Absent, not disabled, for somebody the register does not name (P-05). -->
      <Radio
        label={t('app.tokens.installation')}
        options={[
          { value: NO_INSTALLATION, label: t('app.tokens.installation_none'), hint: t('app.tokens.installation_none_hint') },
          ...installationOptions,
        ]}
        bind:value={installation}
      />
    {/if}

    {#if !isInDialog}
      <div>
        <Button type="submit" tone="primary" isBusy={isWorking} busyLabel={t('app.tokens.minting')}>
          {t('app.tokens.mint')}
        </Button>
      </div>
    {/if}
  </Stack>
</form>

<style>
  .label { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  form { margin: 0; }
</style>
