<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The ways in this installation offers every workspace (ADR-0070 §2).
  //
  // **Offered, not on.** A row added here appears on every workspace's sign-in settings as
  // something they may take; it is a way into none of them until an owner switches it on. That is
  // the concept's own sentence — "für alle Arbeitsbereiche angeboten, nirgends an" — and the reason
  // this screen has no per-workspace control: the taking is theirs, and a screen that could switch
  // it for them would be the installation reading over their shoulder. The one exception is the
  // installation's lock on the ways to sign in (identity.md §18): locked with providers among them,
  // it turns every offer admitted as "Only people invited here" on everywhere, and each row says
  // which applies to it, from the server's `installation_lock`.
  //
  // **A provider whose addresses are not verified cannot be offered.** The blast radius is what
  // decides it: this row reaches every workspace, and one that can only be `DOMAINS` or `ANY`
  // creates accounts in each of them from an address nobody vouched for. The server refuses it;
  // the form - the one a workspace's own providers use too (`ProviderFields`) - offers only the
  // modes the preset permits at this level, so the refusal is rare rather than the first thing a
  // reader meets.
  //
  // **An offer ends announced** (ADR-0076). *Withdraw* shows how many workspaces use the provider -
  // a number, never which - and names a day, two weeks ahead unless the operator chooses another;
  // until then it keeps working and the workspaces say when it ends. *Withdraw now* is for a
  // compromised provider and asks for the number to be typed back. *Keep offering it* undoes either,
  // after the date too. *Remove* comes after the withdrawal (ADR-0077 §2): it deletes every
  // connection between a person and the provider, which offering it again does not restore, so
  // while the offer stands and a workspace uses it the button says why it waits.

  import { tick, untrack } from 'svelte';

  import { Badge, Banner, Button, DataTable, Dialog, EmptyState, FilterBar, Input, ListPage, Spinner, Stack, type DataColumn, type MenuItem } from '@hubtask/design-system/components';
  import type { IdentityProvider, IdentityProviderPreset } from '@hubtask/sync-engine';

  import { draftBody, draftOf, draftRefusal, emptyDraft } from '../lib/signin/issuer.ts';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import ProviderFields from '../lib/signin/ProviderFields.svelte';
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
  import { certificateState } from '../lib/signin/certificate.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { focusFirst } from '../lib/forms/missing.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

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

  let draft = $state(emptyDraft('INVITED_ONLY'));
  let position = $state('0');
  let isEnabled = $state(true);
  /** The withdrawal's day, and the count typed back for *Withdraw now*. */
  let withdrawOn = $state('');
  // Typed as the field's string, and read through `typed()`: a number field inside the component
  // hands back a number at run time once something is typed, which `.trim()` would throw on.
  let confirmCount = $state('');

  $effect(() => {
    if (!instance.isElevated) return;
    const stop = untrack(() => instance.openProviders());
    return stop;
  });

  const reading = $derived(instance.providersState);
  const providers = $derived(instance.providers);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );
  const presets = $derived(instance.presets);

  const columns = $derived<DataColumn[]>([
    { id: 'provider', label: t('app.instance.column_provider'), priority: 1 },
    { id: 'issuer', label: t('app.instance.column_issuer'), priority: 3 },
    { id: 'admission', label: t('app.instance.column_admission'), priority: 3 },
    { id: 'state', label: t('app.instance.column_state'), priority: 2, isStatus: true },
  ]);

  let search = $state('');
  const shown = $derived(providers.filter((provider) => matchesSearch(search, [provider.display_name, provider.issuer])));

  /** A provider's ⋯: change it, withdraw it or keep offering it, and remove it last. */
  function rowMenu(provider: IdentityProvider) {
    const items: MenuItem[] = [
      { id: 'change', label: `${t('app.instance.provider_change')}…` },
      withdrawalPhase(provider) === 'offered'
        ? { id: 'withdraw', label: `${t('app.instance.provider_withdraw')}…` }
        : { id: 'keep', label: t('app.instance.provider_keep_offering') },
      // Removal comes after the withdrawal (ADR-0077 §2): while the offer stands and a workspace
      // uses it, the item says why instead of acting.
      { id: 'remove', label: `${t('app.instance.provider_remove')}…`, isDestructive: true, hasSeparatorBefore: true, disabledReason: removalReason(provider) },
    ];
    return { label: t('app.page.more_for', { name: provider.display_name }), items };
  }

  function onRowMenu(provider: IdentityProvider, id: string): void {
    if (id === 'keep') void run(`keep:${provider.id}`, async () => { await instance.cancelWithdrawal(provider.id); });
    else open(id as 'change' | 'withdraw' | 'remove', provider);
  }

  /**
   * What a mode is called, in the use case's own words (UC-ID-11 check 1). Which list `DOMAINS`
   * reads - directories or domains (ADR-0071 §2) - is said by the list's own field, not the mode.
   */
  function admissionLabel(mode: string): string {
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
    presets.find((each) => each.kind === draft.kind),
  );

  const HOUR = 60 * 60 * 1000;

  /** The count as typed: nothing where nothing was, which the server refuses as not confirmed. */
  function typed(value: string | number): number | null {
    const text = String(value).trim();
    return text === '' || Number.isNaN(Number(text)) ? null : Number(text);
  }

  /** How many workspaces use it, as the operator's projection answers it. */
  const usedBy = (provider: IdentityProvider | undefined): number => provider?.offered_workspaces ?? 0;

  /**
   * What the installation's lock on the ways to sign in does to this row (UC-INS-11 check 4), as the
   * server says it: on everywhere, or each workspace's switch. Without the lock, an invite-only offer
   * says what a lock would do to it, so the operator learns the lever where the provider is.
   */
  function lockSays(provider: IdentityProvider): string | undefined {
    switch (provider.installation_lock) {
      case 'TURNS_ON': return t('app.instance.provider_lock_turns_on');
      case 'LEAVES_TO_WORKSPACE': return t('app.instance.provider_lock_leaves_to_workspace');
      case 'NONE':
        return provider.provisioning === 'INVITED_ONLY' && withdrawalPhase(provider) !== 'withdrawn'
          ? t('app.instance.provider_lock_would_turn_on')
          : undefined;
      default: return undefined;
    }
  }

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
      draft = emptyDraft('INVITED_ONLY');
      position = '0';
      isEnabled = true;
    }
    if (next === 'change' && provider) {
      draft = draftOf(provider);
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
          : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
    } finally {
      working = undefined;
    }
  }

  /** The body both the add and the change send: the one form's fields, its position and switch. */
  function body(): Parameters<typeof instance.configureProvider>[0] {
    return { ...draftBody(draft, preset), position: Number(position) || 0, enabled: isEnabled };
  }

  $effect(() => page.entitle(t('app.instance.providers')));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <ListPage
      head={{
        title: t('app.instance.providers'),
        breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.providers') }),
        primary: { label: t('app.instance.provider_add'), icon: 'plus', onclick: () => open('add') },
      }}
      refused={door ?? (unreadable ? failed : undefined)}
    >
      {#snippet intro()}<p class="quiet">{t('app.instance.providers_intro')}</p>{/snippet}
      {#snippet filters()}
        <FilterBar
          searchLabel={t('app.instance.providers_search')}
          clearSearchLabel={t('app.page.clear_search')}
          bind:search
          countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(providers.length) }) : undefined}
        />
      {/snippet}

      {#if failure && acting === undefined}<FailureNote problem={failure} />{/if}
      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else}
        <DataTable
          label={t('app.instance.providers')}
          {columns}
          rows={shown}
          key={(provider: IdentityProvider) => provider.id}
          onopen={(provider: IdentityProvider) => open('change', provider)}
          rowLabel={(provider: IdentityProvider) => provider.display_name}
          menu={rowMenu}
          onmenu={onRowMenu}
          actionsLabel={t('app.page.row_actions')}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          shownLabel={providers.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
        >
          {#snippet cell(provider: IdentityProvider, column: DataColumn)}
            {#if column.id === 'provider'}
              <span class="name">{provider.display_name}</span>
              <span class="slug">{t(`app.identity_provider.kind.${provider.kind.toLowerCase()}`)}</span>
            {:else if column.id === 'issuer'}
              <span class="mono">{provider.issuer}</span>
            {:else if column.id === 'admission'}
              <span>{admissionLabel(provider.provisioning)}</span>
              {#if admits(provider).length > 0}<span class="slug">{admits(provider).join(', ')}</span>{/if}
            {:else}
              <span class="badges">
                {#if withdrawalPhase(provider) === 'offered'}
                  <Badge tone="success">{t('app.instance.provider_offered')}</Badge>
                {:else if withdrawalPhase(provider) === 'withdrawing'}
                  <Badge tone="warning">{t('app.instance.provider_withdrawing', { date: when(provider.withdraw_at) })}</Badge>
                {:else}
                  <Badge tone="neutral">{t('app.instance.provider_withdrawn')}</Badge>
                {/if}
                {#if certificateState(provider.saml_certificate_expires_at, Date.now()) === 'ended'}
                  <Badge tone="danger">{t('app.identity_provider.certificate_ended', { date: when(provider.saml_certificate_expires_at) })}</Badge>
                {:else if certificateState(provider.saml_certificate_expires_at, Date.now()) === 'ending'}
                  <Badge tone="warning">{t('app.identity_provider.certificate_ending', { date: when(provider.saml_certificate_expires_at) })}</Badge>
                {/if}
                {#if working === `keep:${provider.id}`}<Spinner label={t('app.instance.working')} />{/if}
              </span>
              <span class="slug">{t('app.instance.provider_in_use', { count: String(usedBy(provider)) })}</span>
              {#if lockSays(provider)}<span class="slug" data-lock>{lockSays(provider)}</span>{/if}
            {/if}
          {/snippet}
          {#snippet empty()}
            {#if providers.length === 0}
              <EmptyState kind="unused" title={t('app.instance.providers_none')} />
            {:else}
              <EmptyState kind="filtered" title={t('app.instance.providers_none_found')}>
                {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
              </EmptyState>
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </ListPage>
  {/snippet}
</InstanceGate>

{#snippet failed()}
  {#if unreadable}<FailureNote problem={unreadable} />{/if}
{/snippet}

<Dialog
  size="l"
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
      onclick={() => {
        // Checked here rather than switching Save off: the field already says why the server would
        // refuse it; pressed, focus goes there and nothing is sent (UC-WRK-32 check 3).
        if (draftRefusal(draft, preset) !== undefined) {
          void tick().then(() => focusFirst(document));
          return;
        }
        void run('configure', async () => {
          await instance.configureProvider(body(), acting?.provider?.id);
        });
      }}
    >
      {t('app.instance.provider_save')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <p class="prose">{t('app.instance.provider_form_intro')}</p>
    {#if failure && failure.fields.size === 0}<FailureNote problem={failure} />{/if}

    <ProviderFields bind:draft {presets} level="installation" isNew={acting?.kind !== 'change'}>
      {#snippet more()}
        <Input label={t('app.instance.provider_position_label')} hint={t('app.instance.provider_position_hint')} bind:value={position} type="number" inputmode="numeric" autocomplete="off" />
      {/snippet}
    </ProviderFields>
  </Stack>
</Dialog>

<Dialog
  size="m"
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
      <FailureNote problem={failure} />
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
  size="s"
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
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
    <Banner tone="warning">{t('app.instance.provider_remove_cost')}</Banner>
  </Stack>
</Dialog>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .mono { font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }
  .name { display: block; font-weight: var(--fw-medium); }
  .slug { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .badges { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
</style>
