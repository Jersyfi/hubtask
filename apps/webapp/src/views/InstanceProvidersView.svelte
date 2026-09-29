<!-- SPDX-License-Identifier: BUSL-1.1
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

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Dialog, Input, PageHeader, Select, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import type { IdentityProvider, IdentityProviderPreset } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  /** Which dialog is open, and over which row. `undefined` is none. */
  let acting = $state<{ kind: 'add' | 'change' | 'remove'; provider?: IdentityProvider } | undefined>(undefined);
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

  /** The preset the chosen kind carries, which decides what the form may offer. */
  const preset = $derived<IdentityProviderPreset | undefined>(
    presets.find((each) => each.kind === kind),
  );

  /**
   * The modes this preset permits, minus the one an installation's row may never hold.
   *
   * A preset whose addresses are not verified permits only `DOMAINS` and `ANY`, and neither may be
   * an installation's: that is the whole list gone, which is what makes the kind unofferable rather
   * than merely restricted (ADR-0070 §2).
   */
  const modes = $derived(
    preset === undefined
      ? []
      : preset.addresses_verified
        ? preset.provisioning
        : [],
  );

  // A mode the chosen preset does not permit is one the server refuses at the save, so it is
  // corrected to the strictest the preset allows the moment the kind changes. A rule rather than a
  // change handler, because the value can also arrive from a row being opened for a change.
  $effect(() => {
    const strictest = modes[0];
    if (strictest === undefined) return;
    if (!modes.includes(provisioning as (typeof modes)[number])) provisioning = strictest;
  });

  function open(next: 'add' | 'change' | 'remove', provider?: IdentityProvider): void {
    acting = { kind: next, provider };
    failure = undefined;
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
                  <span>{t(`app.identity_provider.provisioning.${provider.provisioning.toLowerCase()}`)}</span>
                  {#if provider.allowed_email_domains.length > 0}
                    <span class="slug">{provider.allowed_email_domains.join(', ')}</span>
                  {/if}
                </td>
                <td>
                  <Badge tone={provider.enabled ? 'success' : 'neutral'}>
                    {provider.enabled ? t('app.instance.provider_offered') : t('app.instance.provider_withdrawn')}
                  </Badge>
                </td>
                <td>
                  <div class="row">
                    <Button tone="subtle" onclick={() => open('change', provider)}>
                      {t('app.instance.provider_change')}
                    </Button>
                    <Button tone="danger" onclick={() => open('remove', provider)}>
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
      disabledReason={modes.length === 0 ? t('app.instance.provider_unverified_title') : undefined}
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

    {#if preset && !preset.addresses_verified}
      <!-- Not a refusal to render later: the reason is stated where the choice is made, because a
           save that fails after four fields is four fields of wasted typing. -->
      <Banner tone="warning" title={t('app.instance.provider_unverified_title')}>
        {t('app.instance.provider_unverified')}
      </Banner>
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
      options={modes.map((mode) => ({
        value: mode,
        label: t(`app.identity_provider.provisioning.${mode.toLowerCase()}`),
      }))}
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
</style>
