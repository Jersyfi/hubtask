<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A person's own personal access tokens (`security.md` §5).
  //
  // **Profile configuration rather than administration**, which is ADR-0032's split rather than
  // convenience: these are this person's credentials, nobody else can list them, and an
  // administrator who could would learn exactly which of somebody's automations to attack.
  //
  // **Three rules this screen must not soften.** The expiry has no default and the form cannot be
  // submitted without one — every default that could be offered is either so short that people
  // work around it or so long that it is the eternal credential the rule exists to prevent. The
  // scopes are ticked one at a time, never pre-ticked as everything. And an admin scope demands a
  // fresh proof, which arrives as the step-up prompt the wrapper raises — this screen does not
  // decide when, because which scopes are privileged is the server's rule.
  //
  // **The credential appears once, in `OneTimeSecret`, and this screen keeps no copy.** It is held
  // in one `$state` for as long as the panel is open and dropped when it closes; nothing writes it
  // to storage, to the address, or to a log.
  //
  // **The two ways a token stops working are told apart**, because "it ran out" and "somebody
  // pulled it" are different answers to the same question and the contract keeps them in separate
  // columns for exactly that reason.

  import { untrack } from 'svelte';

  import {
    Badge,
    Banner,
    Button,
    CheckboxGroup,
    Input,
    OneTimeSecret,
    Radio,
    Spinner,
    Stack,
    Table,
  } from '@hubtask/design-system/components';

  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { tokens, type MintedToken } from '../lib/data/tokens.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { deviceZone, endOfDayIn, todayIn } from '../lib/i18n/zone.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { check, focusFirst, isComplete, without, type Missing } from '../lib/forms/missing.ts';
  import { sorting } from '../lib/sorting.svelte.ts';

  interface Props {
    /** A service account's tokens, where the caller may mint for one. Absent means their own. */
    accountId?: string;
    /**
     * Whether the reader may mint for that service account: a token for it is held to the role
     * ceiling (identity.md §13), so a role it holds above the reader's own is a certain refusal
     * (P-05). One's own tokens are always minted.
     */
    mintable?: boolean;
  }

  const { accountId, mintable = true }: Props = $props();

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
  // and its whole: ticking both says nothing more than the whole. A service account's panel
  // preselects the narrow scope - it is how a purchase platform's token is made - and a person's
  // own panel preselects nothing, so an ordinary token stays one (the owner's word on the preview).
  const installationDefault = $derived(accountId ? PROVISIONING_SCOPE : NO_INSTALLATION);

  let name = $state('');
  let expiresAt = $state('');
  let chosen = $state<string[]>([]);
  let installation = $state(untrack(() => installationDefault));
  let minted = $state<MintedToken | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  /**
   * The token the reader is asked about. Withdrawing is final - whatever used it stops, and a
   * withdrawn token cannot be restored, only a new one made - so it asks first (ux-inventory.md
   * §E0).
   */
  let withdrawing = $state<{ id: string; name: string } | undefined>(undefined);

  $effect(() => untrack(() => tokens.open(accountId)));

  // The credential dies with the screen. Not a courtesy: it is the one value here that is a
  // credential, and a panel that left it behind would leave it in memory for the tab's life.
  $effect(() => () => (minted = undefined));

  const reading = $derived(tokens.stateOf(accountId));
  // A service account's panel speaks about the service account: "You have no tokens" said to the
  // administrator looking at it was about the wrong person. So is the CalDAV note, which tells
  // somebody their own address is the user name.
  const readingLabel = $derived(accountId ? t('app.tokens.reading_service') : t('app.tokens.reading'));
  const held = $derived(tokens.of(accountId));

  /**
   * Every scope this installation declares, from the manifest and never a list written here - less
   * the operator's scopes for somebody the operator register does not name. Minting one of them
   * asks that register as well as a fresh proof (identity.md §19), and `instance.reachable` is the
   * register's answer for this reader; offering it there would be a proof given into a certain
   * refusal (P-05). The register decides whose mint it is - the person creating the token - so a
   * service account's panel follows the same answer.
   */
  const declared = $derived(manifest.value?.token_scopes ?? []);
  const scopes = $derived(declared.filter((scope) => !scope.startsWith(OPERATOR_SCOPE_PREFIX)));

  /**
   * The installation's scopes as one choice, drawn only for somebody the register names and only with
   * the scopes this installation declares - the reasoning above, for the same two.
   */
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

  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  // Scopes are not sortable: the cell is a list, and sorting a list of lists by its first entry
  // would be an order nobody asked for that looks like one somebody did.
  const columns = $derived([
    { id: 'name', label: t('app.tokens.name'), isSortable: true },
    { id: 'scopes', label: t('app.tokens.scopes') },
    { id: 'expiry', label: t('app.tokens.expires'), isSortable: true },
    { id: 'used', label: t('app.tokens.last_used'), isSortable: true },
    { id: 'state', label: t('app.tokens.state'), isSortable: true },
    { id: 'actions', label: t('app.tokens.actions'), isLabelHidden: true },
  ]);

  // Sorted here, because the whole list is here: `GET /auth/tokens` answers it in one response
  // for a personal account, and nothing on this screen is holding a cursor.
  const order = sorting({
    rows: () => held,
    read: (token, columnId) => {
      if (columnId === 'expiry') return token.expires_at;
      if (columnId === 'used') return token.last_used_at;
      // The state is a word the reader sees, so it sorts as that word rather than as a timestamp:
      // "every revoked one together" is the question somebody sorting this column is asking.
      if (columnId === 'state') return stateOf(token);
      return token.name;
    },
  });

  /** Which of the three words the state column draws. Read twice: once to sort, once to render. */
  function stateOf(token: { revoked_at?: string | null; expires_at: string }): string {
    if (token.revoked_at) return t('app.tokens.revoked');
    if (new Date(token.expires_at).getTime() <= Date.now()) return t('app.tokens.expired');
    return t('app.tokens.live');
  }

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : undefined;

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
  let mintForm = $state<HTMLElement | null>(null);

  async function mint(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    missing = check([
      { kind: 'field', key: 'name', code: 'app.tokens.name_needed', isFilled: name.trim() !== '' },
      { kind: 'field', key: 'expires', code: 'app.tokens.expiry_needed', isFilled: Boolean(expiresAt) },
      { kind: 'at-least-one', key: 'scopes', code: 'app.tokens.pick_a_scope', chosen: requested.length },
    ]);
    if (!isComplete(missing)) {
      queueMicrotask(() => focusFirst(mintForm));
      return;
    }

    failure = undefined;
    isWorking = true;
    try {
      // The date control answers a day; the contract wants an instant: the end of that day where
      // the person is, so a token chosen for "the 31st" works through their 31st and is listed so.
      minted = await tokens.mint({
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
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Stack gap="300">
  {#if minted}
    <!-- The only time this value exists anywhere but the server's hash. The acknowledgement is
         required, because a token scrolled past is a token nobody copied. -->
    <OneTimeSecret
      value={minted.token}
      label={t('app.tokens.minted_title')}
      hint={t('app.tokens.minted_hint')}
      revealLabel={t('app.tokens.reveal')}
      hideLabel={t('app.tokens.hide')}
      copyLabel={t('app.tokens.copy')}
      copiedLabel={t('app.tokens.copied')}
      acknowledgementLabel={t('app.tokens.kept')}
      notAcknowledgedReason={t('app.tokens.keep_first')}
      dismissLabel={t('app.tokens.done')}
      onDismiss={() => (minted = undefined)}
    />
    <!-- The one other thing a token is for, said here because no other screen says it: what a calendar
         client asks for as the password. -->
    {#if !accountId}<p class="quiet">{t('app.tokens.caldav_note')}</p>{/if}
  {/if}

  {#if failure}
    <Banner tone="danger" title={failure.message}>
      {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
    </Banner>
  {/if}

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting"><Spinner label={readingLabel} /> <span>{readingLabel}</span></p>
  {:else if refusal}
    <Banner tone="danger" title={refusal.message}>
      {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
    </Banner>
  {:else if held.length === 0}
    <p class="quiet">{accountId ? t('app.tokens.none_service') : t('app.tokens.none')}</p>
  {:else}
    <Table
      label={t('app.tokens.title')}
      isLabelHidden
      {columns}
      sort={order.sort}
      onSort={(next) => order.by(next)}
    >
      {#each order.rows as token (token.id)}
        <tr>
          <th scope="row" class="name">{token.name}</th>
          <td class="scopes">{token.scopes.join(', ')}</td>
          <td>{when(token.expires_at)}</td>
          <td>{when(token.last_used_at) ?? t('app.tokens.never_used')}</td>
          <td>
            {#if token.revoked_at}
              <!-- Somebody pulled it. A different fact from "it ran out", and the contract keeps
                   them in separate columns so that this screen can say which. -->
              <Badge tone="danger">{t('app.tokens.revoked')}</Badge>
            {:else if new Date(token.expires_at).getTime() <= Date.now()}
              <Badge tone="warning">{t('app.tokens.expired')}</Badge>
            {:else}
              <Badge tone="success">{t('app.tokens.live')}</Badge>
            {/if}
          </td>
          <td>
            {#if !token.revoked_at}
              <!-- `danger`: withdrawing a token stops whatever was using it, at once once the
                   dialog is confirmed. Every row action in these settings that takes access away now
                   carries the tone, because a reader should be able to tell the destructive
                   control from the harmless one without reading it. -->
              <Button
                size="sm"
                tone="danger"
                isBusy={isWorking}
                busyLabel={t('app.tokens.working')}
                onclick={() => (withdrawing = { id: token.id, name: token.name })}
              >
                {t('app.tokens.revoke')}
              </Button>
            {/if}
          </td>
        </tr>
      {/each}
    </Table>

  {/if}

  {#if !mintable}
    <p class="quiet">{t('app.tokens.mint_above_own')}</p>
  {:else}
  <Stack gap="150">
    <h2 class="section">{t('app.tokens.mint_title')}</h2>
    <!-- `novalidate`: what is missing is said by the form itself, in its own words, when it is
         pressed - not by the browser's bubble (design-system.md §4, case 3). -->
    <form onsubmit={mint} bind:this={mintForm} novalidate>
      <Stack gap="200">
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
            options={scopes.map((scope) => ({ value: scope, label: scope }))}
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

        <div>
          <Button
            type="submit"
            tone="primary"
            isBusy={isWorking}
            busyLabel={t('app.tokens.minting')}
          >
            {t('app.tokens.mint')}
          </Button>
        </div>
      </Stack>
    </form>
  </Stack>
  {/if}
</Stack>

<FinalActDialog
  isOpen={withdrawing !== undefined}
  title={t('app.tokens.revoke_confirm_title', { name: withdrawing?.name ?? '' })}
  body={t('app.tokens.revoke_confirm_body', { name: withdrawing?.name ?? '' })}
  confirmLabel={t('app.tokens.revoke')}
  busyLabel={t('app.tokens.working')}
  onConfirm={async () => {
    if (!withdrawing) return;
    try {
      await tokens.revoke(withdrawing.id);
      announcer.say(t('app.tokens.revoked_announced'));
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    }
  }}
  onClose={() => (withdrawing = undefined)}
/>

<style>
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .label { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); }

  .scopes { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }

  form { margin: 0; }
</style>
