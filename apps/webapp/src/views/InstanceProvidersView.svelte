<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The ways in this installation offers every workspace (SI-12, ADR-0070 §2).
  //
  // **Offered, not on.** A row added here appears on every workspace's sign-in settings as
  // something they may take; it is a way into none of them until an owner switches it on. That is
  // the concept's own sentence — "für alle Arbeitsbereiche angeboten, nirgends an" — and the reason
  // this screen has no per-workspace control: the taking is theirs, and a screen that could switch
  // it for them would be the installation reading over their shoulder.
  //
  // **A provider whose addresses are not verified cannot be offered.** The blast radius is what
  // decides it: this row reaches every workspace, and one that can only be `DOMAINS` or `ANY`
  // creates accounts in each of them from an address nobody vouched for. The server refuses it;
  // the form does not offer the modes the preset does not permit, so the refusal is rare rather
  // than the first thing a reader meets.
  //
  // **The secret is written and never read.** Sealed on the way in (E-02), absent from every
  // answer. Leaving it empty on a change keeps the one that is sealed, which is why the field says
  // so rather than looking like a field somebody forgot to fill.
  //
  // **An offer ends announced** (ADR-0076). *Withdraw* shows how many workspaces use the provider -
  // a number, never which - and names a day, two weeks ahead unless the operator chooses another;
  // until then it keeps working and the workspaces say when it ends. *Withdraw now* is for a
  // compromised provider and asks for the number to be typed back. *Keep offering it* undoes either,
  // after the date too. *Remove* comes after the withdrawal (ADR-0077 §2): it deletes every
  // connection between a person and the provider, which offering it again does not restore, so
  // while the offer stands and a workspace uses it the button says why it waits.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Dialog, Input, PageHeader, Select, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import type { IdentityProvider, IdentityProviderPreset } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import {
    defaultWithdrawalDate,
    earliestWithdrawalDate,
    removalWait,
    withdrawalMoment,
    withdrawalPhase,
  } from '../lib/instance/withdrawal.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  /** Which dialog is open, and over which row. `undefined` is none. */
  let acting = $state<
    { kind: 'add' | 'change' | 'remove' | 'withdraw'; provider?: IdentityProvider } | undefined
  >(undefined);
  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  let kind = $state('GENERIC');
  let issuer = $state('');
  let clientId = $state('');
  let clientSecret = $state('');
  let displayName = $state('');
  let provisioning = $state('INVITED_ONLY');
  let domains = $state('');
  let directories = $state('');
  let position = $state('0');
  let isEnabled = $state(true);
  /** The withdrawal's day, and the count typed back for *Withdraw now*. */
  let withdrawOn = $state('');
  // Typed as the field's string, and read through `typed()`: a number field inside the component
  // hands back a number at run time once something is typed, which `.trim()` would throw on.
  let confirmCount = $state('');

  $effect(() => {
    const stop = untrack(() => instance.openProviders());
    return stop;
  });

  const reading = $derived(instance.providersState);
  const providers = $derived(instance.providers);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );
  const presets = $derived(instance.presets);

  const columns = $derived([
    { id: 'provider', label: t('app.instance.column_provider') },
    { id: 'issuer', label: t('app.instance.column_issuer') },
    { id: 'admission', label: t('app.instance.column_admission') },
    { id: 'state', label: t('app.instance.column_state') },
    { id: 'actions', label: t('app.instance.column_actions'), isLabelHidden: true },
  ]);

  /**
   * What a mode is called for a given preset.
   *
   * `DOMAINS` reads two different lists depending on the provider (ADR-0071 §2), so it cannot have
   * one sentence: a row that says "the domains below" while the server reads directories is a row
   * that describes somebody else's configuration.
   */
  function admissionLabel(kind: string, mode: string): string {
    const claim = presets.find((each) => each.kind === kind)?.directory_claim;
    if (mode === 'DOMAINS' && claim) return t(`app.identity_provider.provisioning.directories_${claim}`);
    return t(`app.identity_provider.provisioning.${mode.toLowerCase()}`);
  }

  /** The list this row is actually read against: the directories where the preset has them. */
  function admits(provider: IdentityProvider): readonly string[] {
    return presets.find((each) => each.kind === provider.kind)?.directory_claim
      ? (provider.allowed_directories ?? [])
      : provider.allowed_email_domains;
  }

  /** The preset the chosen kind carries, which decides what the form may offer. */
  const preset = $derived<IdentityProviderPreset | undefined>(
    presets.find((each) => each.kind === kind),
  );

  /**
   * The modes this preset permits when the installation offers it, as the server answers them.
   *
   * Narrower than a workspace's for a preset with no directory claim: offered to every workspace, a
   * self-hosted issuer may admit only the people each workspace invited (ADR-0071's addendum). The
   * server's list rather than a rule repeated here, so the two cannot disagree.
   */
  const modes = $derived(preset === undefined ? [] : preset.installation_provisioning);
  /** Whether the installation's level narrows this preset to the invited, which the form says. */
  const narrowedToInvited = $derived(
    preset !== undefined && modes.length === 1 && preset.provisioning.length > 1,
  );

  // A mode the chosen preset does not permit is one the server refuses at the save, so it is
  // corrected to the strictest the preset allows the moment the kind changes. A rule rather than a
  // change handler, because the value can also arrive from a row being opened for a change.
  $effect(() => {
    const strictest = modes[0];
    if (strictest === undefined) return;
    if (!modes.includes(provisioning as (typeof modes)[number])) provisioning = strictest;
  });

  const HOUR = 60 * 60 * 1000;

  /** The count as typed: nothing where nothing was, which the server refuses as not confirmed. */
  function typed(value: string | number): number | null {
    const text = String(value).trim();
    return text === '' || Number.isNaN(Number(text)) ? null : Number(text);
  }

  /** How many workspaces use it, as the operator's projection answers it. */
  const usedBy = (provider: IdentityProvider | undefined): number => provider?.offered_workspaces ?? 0;

  const when = (at: string | null | undefined): string => (at ? formatDateTime(at, messages.locale) : '');

  /** Why *Remove* waits, as a sentence, or nothing where it may act. */
  function removalReason(provider: IdentityProvider): string | undefined {
    switch (removalWait(provider)) {
      case 'withdraw_first':
        return t('app.instance.provider_remove_first');
      case 'after_withdrawal':
        return t('app.instance.provider_remove_after', { date: when(provider.withdraw_at) });
      default:
        return undefined;
    }
  }

  function open(next: 'add' | 'change' | 'remove' | 'withdraw', provider?: IdentityProvider): void {
    acting = { kind: next, provider };
    failure = undefined;
    if (next === 'withdraw') {
      withdrawOn = defaultWithdrawalDate(actor.zone);
      confirmCount = '';
    }
    if (next === 'add') {
      kind = 'GENERIC';
      issuer = '';
      clientId = '';
      clientSecret = '';
      displayName = '';
      provisioning = 'INVITED_ONLY';
      domains = '';
      directories = '';
      position = '0';
      isEnabled = true;
    }
    if (next === 'change' && provider) {
      kind = provider.kind;
      issuer = provider.issuer;
      clientId = provider.client_id;
      clientSecret = '';
      displayName = provider.display_name;
      provisioning = provider.provisioning;
      domains = provider.allowed_email_domains.join(', ');
      directories = (provider.allowed_directories ?? []).join(', ');
      position = String(provider.position);
      isEnabled = provider.enabled;
    }
  }

  async function run(id: string, call: () => Promise<void>): Promise<void> {
    working = id;
    failure = undefined;
    try {
      await call();
      acting = undefined;
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
    } finally {
      working = undefined;
    }
  }

  /** The body both the add and the change send, with the secret left out where it is empty. */
  function body(): Parameters<typeof instance.configureProvider>[0] {
    const split = (written: string) =>
      written
        .split(',')
        .map((each) => each.trim())
        .filter((each) => each !== '');
    return {
      issuer: issuer.trim(),
      client_id: clientId.trim(),
      // Absent keeps what is sealed. An empty string would be a secret of no characters, which is
      // a different thing and one the provider would refuse at the exchange rather than here.
      ...(clientSecret.trim() === '' ? {} : { client_secret: clientSecret }),
      ...(displayName.trim() === '' ? {} : { display_name: displayName.trim() }),
      kind: kind as IdentityProvider['kind'],
      provisioning: provisioning as IdentityProvider['provisioning'],
      allowed_email_domains: split(domains),
      allowed_directories: split(directories),
      position: Number(position) || 0,
      enabled: isEnabled,
    };
  }

  $effect(() => page.entitle(t('app.instance.providers')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.providers')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.providers_intro')}</p>

      {#if failure}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {/if}

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else}
        <div>
          <Button tone="primary" onclick={() => open('add')}>{t('app.instance.provider_add')}</Button>
        </div>

        {#if providers.length === 0}
          <Banner tone="info">{t('app.instance.providers_none')}</Banner>
        {:else}
          <Table label={t('app.instance.providers')} isLabelHidden columns={columns}>
            {#each providers as provider (provider.id)}
              <tr>
                <td>
                  <span class="name">{provider.display_name}</span>
                  <span class="slug">{t(`app.identity_provider.kind.${provider.kind.toLowerCase()}`)}</span>
                </td>
                <td class="mono">{provider.issuer}</td>
                <td>
                  <span>{admissionLabel(provider.kind, provider.provisioning)}</span>
                  {#if admits(provider).length > 0}
                    <span class="slug">{admits(provider).join(', ')}</span>
                  {/if}
                </td>
                <td>
                  {#if withdrawalPhase(provider) === 'offered'}
                    <Badge tone="success">{t('app.instance.provider_offered')}</Badge>
                  {:else if withdrawalPhase(provider) === 'withdrawing'}
                    <Badge tone="warning">{t('app.instance.provider_withdrawing', { date: when(provider.withdraw_at) })}</Badge>
                  {:else}
                    <Badge tone="neutral">{t('app.instance.provider_withdrawn')}</Badge>
                  {/if}
                  <span class="slug">{t('app.instance.provider_in_use', { count: String(usedBy(provider)) })}</span>
                </td>
                <td>
                  <div class="row">
                    <Button tone="subtle" onclick={() => open('change', provider)}>
                      {t('app.instance.provider_change')}
                    </Button>
                    {#if withdrawalPhase(provider) === 'offered'}
                      <Button tone="subtle" onclick={() => open('withdraw', provider)}>
                        {t('app.instance.provider_withdraw')}
                      </Button>
                    {:else}
                      <Button
                        tone="subtle"
                        isBusy={working === `keep:${provider.id}`}
                        busyLabel={t('app.instance.working')}
                        onclick={() =>
                          void run(`keep:${provider.id}`, async () => {
                            await instance.cancelWithdrawal(provider.id);
                          })}
                      >
                        {t('app.instance.provider_keep_offering')}
                      </Button>
                    {/if}
                    <!-- Removal comes after the withdrawal (ADR-0077 §2): while the offer stands and a
                         workspace uses it, the button says why instead of acting - withdraw it first,
                         or, announced already, the day it may go. -->
                    <Button
                      tone="danger"
                      disabledReason={removalReason(provider)}
                      onclick={() => open('remove', provider)}
                    >
                      {t('app.instance.provider_remove')}
                    </Button>
                  </div>
                </td>
              </tr>
            {/each}
          </Table>
        {/if}
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<Dialog
  title={acting?.kind === 'change' ? t('app.instance.provider_change_title') : t('app.instance.provider_add_title')}
  isOpen={acting?.kind === 'add' || acting?.kind === 'change'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.cancel')}</Button>
    <Button
      tone="primary"
      isBusy={working === 'configure'}
      busyLabel={t('app.instance.working')}
      onclick={() =>
        void run('configure', async () => {
          await instance.configureProvider(body(), acting?.provider?.id);
        })}
    >
      {t('app.instance.provider_save')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <p class="prose">{t('app.instance.provider_form_intro')}</p>

    <Select
      label={t('app.instance.provider_kind_label')}
      bind:value={kind}
      options={presets.map((each) => ({
        value: each.kind,
        label: t(`app.identity_provider.kind.${each.kind.toLowerCase()}`),
      }))}
    />

    {#if narrowedToInvited}
      <!-- Said where the choice is made: the admission field below offers one answer, and a field
           with one answer needs the sentence that says why. -->
      <Banner tone="info">{t('app.instance.provider_invited_only')}</Banner>
    {/if}
    {#if preset?.particular}
      <Banner tone="info">{t(preset.particular, { redirect_uri: preset.redirect_uri })}</Banner>
    {/if}
    {#if preset}
      <p class="quiet-line">{t(preset.instructions, { redirect_uri: preset.redirect_uri })}</p>
    {/if}

    <Input label={t('app.instance.provider_issuer_label')} hint={t('app.instance.provider_issuer_hint')} bind:value={issuer} autocomplete="off" spellcheck={false} isRequired />
    <Input label={t('app.instance.provider_client_label')} bind:value={clientId} autocomplete="off" spellcheck={false} isRequired />
    <Input
      label={t('app.instance.provider_secret_label')}
      hint={acting?.kind === 'change' ? t('app.instance.provider_secret_kept') : t('app.instance.provider_secret_hint')}
      bind:value={clientSecret}
      type="password"
      autocomplete="off"
      spellcheck={false}
    />
    <Input label={t('app.instance.provider_name_label')} hint={t('app.instance.provider_name_hint')} bind:value={displayName} autocomplete="off" />

    <Select
      label={t('app.instance.provider_admission_label')}
      hint={t('app.instance.provider_admission_hint')}
      bind:value={provisioning}
      options={modes.map((mode) => ({ value: mode, label: admissionLabel(kind, mode) }))}
    />

    {#if provisioning === 'DOMAINS'}
      <!-- The list the server will read, and only that one (ADR-0071 §2). -->
      {#if preset?.directory_claim}
        <Input
          label={t(`app.identity_provider.directories_label_${preset.directory_claim}`)}
          hint={t(`app.identity_provider.directories_hint_${preset.directory_claim}`)}
          bind:value={directories}
          autocomplete="off"
          spellcheck={false}
        />
      {:else}
        <Input
          label={t('app.instance.provider_domains_label')}
          hint={t('app.instance.provider_domains_hint')}
          bind:value={domains}
          autocomplete="off"
          spellcheck={false}
        />
      {/if}
    {/if}
    {#if preset?.supports_templated_issuer}
      <!-- The platform case, said where the issuer is typed: one registration at the provider
           serving many customers, and the directory list is what bounds it. -->
      <Banner tone="info" title={t('app.instance.provider_shared_title')}>
        {t('app.instance.provider_shared')}
      </Banner>
    {/if}

    <Input label={t('app.instance.provider_position_label')} hint={t('app.instance.provider_position_hint')} bind:value={position} type="number" inputmode="numeric" autocomplete="off" />
  </Stack>
</Dialog>

<Dialog
  title={t('app.instance.provider_withdraw_title')}
  isOpen={acting?.kind === 'withdraw'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.keep')}</Button>
    <Button
      tone="primary"
      isBusy={working === 'withdraw'}
      busyLabel={t('app.instance.working')}
      onclick={() =>
        void run('withdraw', async () => {
          const at = withdrawalMoment(withdrawOn, actor.zone);
          await instance.withdrawProvider(acting?.provider?.id ?? '', at ? { withdraw_at: at } : {});
        })}
    >
      {t('app.instance.provider_withdraw_announce')}
    </Button>
  {/snippet}
  <Stack gap="200">
    <!-- A refusal inside the dialog, where it is read: the page behind a modal is not. One that
         names the count lands at the count's field instead. -->
    {#if failure && !failure.fields.has('/confirm_count')}
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
      </Banner>
    {/if}
    <!-- The number first: it is what the withdrawal costs, and the reason to choose the day well. -->
    <Banner tone="info" title={t('app.instance.provider_in_use', { count: String(usedBy(acting?.provider)) })}>
      {t('app.instance.provider_withdraw_intro')}
    </Banner>
    <Input
      label={t('app.instance.provider_withdraw_date_label')}
      hint={t('app.instance.provider_withdraw_date_hint')}
      bind:value={withdrawOn}
      type="date"
      min={earliestWithdrawalDate(actor.zone)}
      isRequired
    />

    <Stack gap="100">
      <h3 class="section">{t('app.instance.provider_withdraw_now_title')}</h3>
      <p class="prose">{t('app.instance.provider_withdraw_now_intro')}</p>
      <Input
        label={t('app.instance.provider_withdraw_now_count_label')}
        error={failure?.fields.get('/confirm_count')}
        bind:value={confirmCount}
        type="number"
        inputmode="numeric"
        autocomplete="off"
      />
      <div>
        <!-- Not disabled while the number is wrong: the server compares it inside its own
             transaction, and its refusal says the number as it stands now. -->
        <Button
          tone="danger"
          isBusy={working === 'withdraw-now'}
          busyLabel={t('app.instance.working')}
          onclick={() =>
            void run('withdraw-now', async () => {
              await instance.withdrawProvider(acting?.provider?.id ?? '', {
                // An hour back rather than this browser's now: a clock running ahead of the
                // server's would otherwise turn "now" into an announcement for a minute away, which
                // asks for no count. The server records the moment it actually ended.
                withdraw_at: new Date(Date.now() - HOUR).toISOString(),
                confirm_count: typed(confirmCount),
              });
            })}
        >
          {t('app.instance.provider_withdraw_now')}
        </Button>
      </div>
    </Stack>
  </Stack>
</Dialog>

<Dialog
  title={t('app.instance.provider_remove_title')}
  isOpen={acting?.kind === 'remove'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.keep')}</Button>
    <Button
      tone="danger"
      isBusy={working === 'remove'}
      busyLabel={t('app.instance.working')}
      onclick={() => void run('remove', () => instance.removeProvider(acting?.provider?.id ?? ''))}
    >
      {t('app.instance.provider_remove')}
    </Button>
  {/snippet}
  <Banner tone="warning">{t('app.instance.provider_remove_cost')}</Banner>
</Dialog>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-line { margin: 0; max-inline-size: 60ch; color: var(--text-secondary); font-size: var(--fs-100); }
  .mono { font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }
  .name { display: block; font-weight: var(--fw-medium); }
  .slug { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
</style>
