<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A person's own personal access tokens (G-01, `security.md` §5).
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

  import { Badge, Banner, Button, Checkbox, Input, OneTimeSecret, Spinner, Stack, Table } from '@hubtask/design-system/components';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { tokens, type MintedToken } from '../lib/data/tokens.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    /** A service account's tokens, where the caller may mint for one. Absent means their own. */
    accountId?: string;
  }

  const { accountId }: Props = $props();

  let name = $state('');
  let expiresAt = $state('');
  let chosen = $state<string[]>([]);
  let minted = $state<MintedToken | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  $effect(() => untrack(() => tokens.open(accountId)));

  // The credential dies with the screen. Not a courtesy: it is the one value here that is a
  // credential, and a panel that left it behind would leave it in memory for the tab's life.
  $effect(() => () => (minted = undefined));

  const reading = $derived(tokens.stateOf(accountId));
  const held = $derived(tokens.of(accountId));

  /** Every scope this installation declares, from the manifest and never a list written here. */
  const scopes = $derived(manifest.value?.token_scopes ?? []);

  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const columns = [
    { id: 'name', label: t('app.tokens.name') },
    { id: 'scopes', label: t('app.tokens.scopes') },
    { id: 'expiry', label: t('app.tokens.expires') },
    { id: 'used', label: t('app.tokens.last_used') },
    { id: 'state', label: t('app.tokens.state') },
    { id: 'actions', label: t('app.tokens.actions'), isLabelHidden: true },
  ];

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : undefined;

  /** A year from now, to the day: the furthest the server will accept. */
  function latestAllowed(): string {
    const year = new Date();
    year.setUTCFullYear(year.getUTCFullYear() + 1);
    return year.toISOString().slice(0, 10);
  }

  const today = new Date().toISOString().slice(0, 10);

  function toggle(scope: string, on: boolean): void {
    chosen = on ? [...chosen, scope] : chosen.filter((each) => each !== scope);
  }

  async function mint(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!name.trim() || !expiresAt || chosen.length === 0) return;

    failure = undefined;
    isWorking = true;
    try {
      // The date control answers a day; the contract wants an instant. End of that day in UTC, so
      // a token chosen for "the 31st" works through the 31st wherever the reader is.
      minted = await tokens.mint({
        name: name.trim(),
        scopes: chosen,
        expiresAt: `${expiresAt}T23:59:59Z`,
        accountId,
      });
      name = '';
      expiresAt = '';
      chosen = [];
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
  {/if}

  {#if failure}
    <Banner tone="danger" title={failure.message}>
      {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
    </Banner>
  {/if}

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting"><Spinner label={t('app.tokens.reading')} /> <span>{t('app.tokens.reading')}</span></p>
  {:else if refusal}
    <Banner tone="danger" title={refusal.message}>
      {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
    </Banner>
  {:else if held.length === 0}
    <p class="quiet">{t('app.tokens.none')}</p>
  {:else}
    <Table label={t('app.tokens.title')} isLabelHidden {columns}>
      {#each held as token (token.id)}
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
              <Button
                size="sm"
                tone="subtle"
                isBusy={isWorking}
                busyLabel={t('app.tokens.working')}
                onclick={() =>
                  void tokens.revoke(token.id).catch((cause) => {
                    failure = renderProblem(cause as never, messages);
                  })}
              >
                {t('app.tokens.revoke')}
              </Button>
            {/if}
          </td>
        </tr>
      {/each}
    </Table>
  {/if}

  <Stack gap="150">
    <h2 class="section">{t('app.tokens.mint_title')}</h2>
    <form onsubmit={mint}>
      <Stack gap="200">
        <Input
          label={t('app.tokens.name')}
          hint={t('app.tokens.name_hint')}
          bind:value={name}
          isRequired
        />

        <!-- `max` as well as `required`: the server refuses more than a year, and being told
             before the round trip is the same number said earlier rather than a second policy. -->
        <Input
          label={t('app.tokens.expires')}
          hint={t('app.tokens.expires_hint')}
          bind:value={expiresAt}
          type="date"
          min={today}
          max={latestAllowed()}
          isRequired
        />

        <Stack gap="050">
          <span class="label">{t('app.tokens.scopes')}</span>
          <p class="quiet small">{t('app.tokens.scopes_hint')}</p>
          {#if scopes.length === 0}
            <!-- The manifest has not arrived. Offering nothing is honest; offering a list written
                 here would be wrong on somebody's installation. -->
            <p class="quiet small">{t('app.tokens.scopes_unknown')}</p>
          {:else}
            <ul class="scope-list">
              {#each scopes as scope (scope)}
                <li>
                  <Checkbox
                    label={scope}
                    checked={chosen.includes(scope)}
                    onchange={(event: Event) =>
                      toggle(scope, (event.currentTarget as HTMLInputElement).checked)}
                  />
                </li>
              {/each}
            </ul>
          {/if}
        </Stack>

        <div>
          <Button
            type="submit"
            tone="primary"
            isBusy={isWorking}
            busyLabel={t('app.tokens.minting')}
            disabledReason={chosen.length === 0 ? t('app.tokens.pick_a_scope') : undefined}
          >
            {t('app.tokens.mint')}
          </Button>
        </div>
      </Stack>
    </form>
  </Stack>
</Stack>

<style>
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .label { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); }

  .scopes { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }

  .scope-list { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }

  .scope-list :global(label) { font-family: var(--font-mono); }

  form { margin: 0; }
</style>
