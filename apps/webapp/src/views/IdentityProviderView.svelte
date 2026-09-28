<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's identity providers, configured (H-04, SI-10).
  //
  // **Plural, and two levels deep.** The listing carries this workspace's own rows and the ones its
  // installation offers every workspace on it. An inherited row is drawn with **no controls at all**
  // rather than controls that answer a refusal: it is not this workspace's to change, and a disabled
  // switch beside it would be a promise that pressing harder helps.
  //
  // **The client secret goes one way, and this screen says so rather than pretending.** It is sealed
  // on the way in and is a member of no answer (E-02), so editing an existing provider shows an empty
  // secret field with a sentence explaining that leaving it empty is not "no secret" — it is "the one
  // already sealed". A row of dots standing in for a value nobody can read would be a screen
  // inventing a fact.
  //
  // **The modes a preset permits come from the server.** Which of the three a provider may use is a
  // rule about the provider (a public issuer may only sign in people who were invited first), and the
  // presets answer it — so the select offers what would be accepted rather than three of which two
  // are refused. A client that decided this itself would be a second copy of the rule.
  //
  // **Removal costs something, and it is stated before the button.** The accounts provisioned through
  // the provider keep their rows and their live sessions; what they lose is the way back in. For an
  // account that never had a password, that is the whole way back in.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Input, PageHeader, Select, Spinner, Stack, Switch, Textarea } from '@hubtask/design-system/components';
  import type { IdentityProvider, IdentityProviderConfiguration } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import ProviderMark from '../lib/signin/ProviderMark.svelte';
  import { identityProvider } from '../lib/data/identityprovider.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  /** Which row the form is editing. `undefined` is closed; the empty string is a new one. */
  let editing = $state<string | undefined>(undefined);
  let issuer = $state('');
  let clientId = $state('');
  let clientSecret = $state('');
  let displayName = $state('');
  let kind = $state('GENERIC');
  let provisioning = $state('DOMAINS');
  let domains = $state('');
  let enabled = $state(true);
  /** The order the buttons are drawn in. Kept rather than shown: a workspace with two providers
      orders them by adding them, and a control for it would be a control for a list of two. */
  let position = $state(0);
  let confirmingRemoval = $state<string | undefined>(undefined);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let saved = $state(false);

  $effect(() => {
    // `untrack` for the reason every store here records: the read writes the store and writing it
    // reads it, so an effect that tracked that read would re-run on its own first answer.
    const stop = untrack(() => identityProvider.open());
    return stop;
  });

  // Not named `state`: a variable of that name turns every `$state` in this file into a store
  // subscription, which is a compiler rule rather than a style preference.
  const reading = $derived(identityProvider.state);
  const own = $derived(identityProvider.own);
  const inherited = $derived(identityProvider.inherited);

  /**
   * A read that failed with `404` is an installation that does not serve the collection — the
   * screen the workspace had before SI-10, which is nothing to configure here. Anything else is a
   * refusal worth showing: a caller without the permission, a server that could not answer.
   */
  const unavailable = $derived(reading.status === 'failed' && reading.error.status === 404);
  const unreadable = $derived(
    reading.status === 'failed' && !unavailable ? renderProblem(reading.error, messages) : undefined,
  );

  /** The preset of whichever kind the form is set to, where the presets have been read. */
  const preset = $derived(identityProvider.presetOf(kind as IdentityProvider['kind']));

  const kindOptions = $derived(
    identityProvider.presets.map((one) => ({
      value: one.kind,
      label: t(`app.identity_provider.kind.${one.kind.toLowerCase()}`),
    })),
  );

  /**
   * The modes this preset permits, in the order they tighten. Empty until the presets are read,
   * which is the third value of every capability question here: not yet known is not "all of them".
   */
  const provisioningOptions = $derived(
    (preset?.provisioning ?? []).map((mode) => ({
      value: mode,
      label: t(`app.identity_provider.provisioning.${mode.toLowerCase()}`),
    })),
  );

  /** Opens the form on a new provider. */
  function add(): void {
    editing = '';
    issuer = '';
    clientId = '';
    clientSecret = '';
    displayName = '';
    kind = 'GENERIC';
    provisioning = 'DOMAINS';
    domains = '';
    enabled = true;
    position = identityProvider.own.length;
    failure = undefined;
    saved = false;
  }

  /** Opens it on one that exists. The secret stays empty, because nothing can read it back. */
  function edit(provider: IdentityProvider): void {
    editing = provider.id;
    issuer = provider.issuer;
    clientId = provider.client_id;
    clientSecret = '';
    displayName = provider.display_name;
    kind = provider.kind;
    provisioning = provider.provisioning;
    domains = (provider.allowed_email_domains ?? []).join('\n');
    enabled = provider.enabled;
    position = provider.position;
    failure = undefined;
    saved = false;
  }

  function close(): void {
    editing = undefined;
    clientSecret = '';
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    isWorking = true;
    failure = undefined;
    saved = false;
    const body: IdentityProviderConfiguration = {
      issuer: issuer.trim(),
      client_id: clientId.trim(),
      display_name: displayName.trim(),
      kind: kind as IdentityProvider['kind'],
      provisioning: provisioning as IdentityProvider['provisioning'],
      position,
      enabled,
      allowed_email_domains: readDomains(domains),
    };
    // Sent only when it was typed. An empty string and an absent field are two different requests:
    // absent keeps the sealed one, and that is the contract's promise rather than this screen's rule.
    if (clientSecret !== '') body.client_secret = clientSecret;
    try {
      if (editing === '' || editing === undefined) {
        await identityProvider.add(body);
      } else {
        await identityProvider.configure(editing, body);
      }
      // Out of the component's state at once. A secret kept for a second submission is a secret
      // sitting in a form for as long as the tab is open.
      clientSecret = '';
      saved = true;
      editing = undefined;
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      isWorking = false;
    }
  }

  async function remove(id: string): Promise<void> {
    isWorking = true;
    failure = undefined;
    saved = false;
    try {
      await identityProvider.remove(id);
      confirmingRemoval = undefined;
      if (editing === id) close();
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      isWorking = false;
    }
  }

  /** One domain per line, or separated by commas — whichever somebody pastes. */
  function readDomains(written: string): string[] {
    return written
      .split(/[\n,]/)
      .map((one) => one.trim())
      .filter((one) => one !== '');
  }

  function problemOf(cause: unknown): ReturnType<typeof renderProblem> {
    return cause instanceof TransportError
      ? renderProblem(cause, messages)
      : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
  }
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.identity_provider.title')));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.identity_provider.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'identity-provider', label: t('app.identity_provider.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  <Stack gap="300">
    <p class="quiet">{t('app.identity_provider.intro')}</p>

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="quiet">
        <Spinner label={t('app.identity_provider.reading')} />
        <span>{t('app.identity_provider.reading')}</span>
      </p>
    {:else if unreadable}
      <Banner tone="danger" title={unreadable.message}>
        {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
      </Banner>
    {:else if unavailable}
      <Banner tone="info">{t('app.identity_provider.not_served')}</Banner>
    {:else}
      {#if failure}
        <!-- The server's own sentence: an issuer that could not be reached, a mark on an issuer it
             does not belong to, a mode the provider may not use, a caller who may read and not
             write. -->
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {:else if saved}
        <Banner tone="success">{t('app.identity_provider.saved')}</Banner>
      {/if}

      {#if inherited.length > 0}
        <Stack gap="150">
          <h2 class="section">{t('app.identity_provider.inherited_title')}</h2>
          <p class="quiet">{t('app.identity_provider.inherited_intro')}</p>
          <ul class="providers">
            {#each inherited as provider (provider.id)}
              <li class="provider">
                <ProviderMark kind={provider.kind} name={provider.display_name} />
                <div class="about">
                  <span class="name">{provider.display_name}</span>
                  <span class="detail">{provider.issuer}</span>
                </div>
                <Badge tone={provider.enabled ? 'neutral' : 'warning'}>
                  {provider.enabled
                    ? t('app.identity_provider.badge_inherited')
                    : t('app.identity_provider.badge_off')}
                </Badge>
              </li>
            {/each}
          </ul>
        </Stack>
      {/if}

      <Stack gap="150">
        <h2 class="section">{t('app.identity_provider.own_title')}</h2>
        {#if own.length === 0}
          <Banner tone="info">{t('app.identity_provider.none')}</Banner>
        {:else}
          <ul class="providers">
            {#each own as provider (provider.id)}
              <li class="provider">
                <ProviderMark kind={provider.kind} name={provider.display_name} />
                <div class="about">
                  <span class="name">{provider.display_name}</span>
                  <span class="detail">{provider.issuer}</span>
                  <span class="detail">
                    {t(`app.identity_provider.provisioning.${provider.provisioning.toLowerCase()}`)}
                  </span>
                </div>
                {#if !provider.enabled}
                  <Badge tone="warning">{t('app.identity_provider.badge_off')}</Badge>
                {/if}
                <div class="row">
                  <Button tone="subtle" onclick={() => edit(provider)}>
                    {t('app.identity_provider.edit')}
                  </Button>
                  <Button tone="subtle" onclick={() => (confirmingRemoval = provider.id)}>
                    {t('app.identity_provider.remove')}
                  </Button>
                </div>
              </li>
              {#if confirmingRemoval === provider.id}
                <li class="confirm">
                  <!-- Said before the button rather than in the dialog that follows it: somebody
                       deciding whether to press it is the person who needs to know what it costs. -->
                  <Banner tone="warning" title={t('app.identity_provider.remove_confirm')}>
                    {t('app.identity_provider.remove_cost')}
                  </Banner>
                  <div class="row">
                    <Button
                      tone="danger"
                      isBusy={isWorking}
                      busyLabel={t('app.identity_provider.removing')}
                      onclick={() => void remove(provider.id)}
                    >
                      {t('app.identity_provider.remove_now')}
                    </Button>
                    <Button tone="subtle" onclick={() => (confirmingRemoval = undefined)}>
                      {t('app.identity_provider.keep')}
                    </Button>
                  </div>
                </li>
              {/if}
            {/each}
          </ul>
        {/if}

        {#if editing === undefined}
          <div>
            <Button tone="primary" onclick={add}>{t('app.identity_provider.add')}</Button>
          </div>
        {/if}
      </Stack>

      {#if editing !== undefined}
        <form class="panel" onsubmit={save}>
          <Stack gap="200">
            <h2 class="section">
              {editing === ''
                ? t('app.identity_provider.add_title')
                : t('app.identity_provider.edit_title')}
            </h2>
            <Select
              label={t('app.identity_provider.kind_label')}
              hint={t('app.identity_provider.kind_hint')}
              options={kindOptions}
              bind:value={kind}
            />
            {#if preset}
              <!-- The instructions are the server's message code rendered with this installation's
                   own callback: the one value every registration form at every provider asks for. -->
              <Banner tone="info" title={t('app.identity_provider.registration')}>
                {messages.t(preset.instructions, { redirect_uri: preset.redirect_uri })}
                {#if preset.particular}
                  {' '}{messages.t(preset.particular, {})}
                {/if}
              </Banner>
            {/if}
            <Input
              label={t('app.identity_provider.issuer_label')}
              hint={t('app.identity_provider.issuer_hint')}
              bind:value={issuer}
              type="url"
              autocomplete="off"
              spellcheck={false}
              isRequired
            />
            <Input
              label={t('app.identity_provider.name_label')}
              hint={t('app.identity_provider.name_hint')}
              bind:value={displayName}
              autocomplete="off"
            />
            <Input
              label={t('app.identity_provider.client_id_label')}
              hint={t('app.identity_provider.client_id_hint')}
              bind:value={clientId}
              autocomplete="off"
              spellcheck={false}
              isRequired
            />
            <Input
              label={t('app.identity_provider.client_secret_label')}
              hint={editing === ''
                ? t('app.identity_provider.client_secret_hint')
                : t('app.identity_provider.client_secret_kept')}
              bind:value={clientSecret}
              type="password"
              autocomplete="off"
              spellcheck={false}
              isRequired={editing === ''}
            />
            <Select
              label={t('app.identity_provider.provisioning_label')}
              hint={t('app.identity_provider.provisioning_hint')}
              options={provisioningOptions}
              bind:value={provisioning}
            />
            {#if provisioning === 'DOMAINS'}
              <Textarea
                label={t('app.identity_provider.domains_label')}
                hint={t('app.identity_provider.domains_hint')}
                bind:value={domains}
                rows={3}
                spellcheck={false}
              />
            {/if}
            <Switch
              label={t('app.identity_provider.enabled_label')}
              hint={t('app.identity_provider.enabled_hint')}
              bind:checked={enabled}
            />
            <div class="row">
              <Button
                type="submit"
                tone="primary"
                isBusy={isWorking}
                busyLabel={t('app.identity_provider.saving')}
              >
                {t('app.identity_provider.save')}
              </Button>
              <Button tone="subtle" onclick={close}>{t('app.identity_provider.cancel')}</Button>
            </div>
          </Stack>
        </form>
      {/if}
    {/if}
  </Stack>
</Stack>

<style>
  /* The screen takes the region it is given, and what needs a measure carries one: prose has the
     one `app.css` gives every paragraph, fields have their own, and a list has none
     (ADR-0065 decision 2). */

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .providers { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }

  /* A row is the mark, what it is, and what may be done to it. The middle column takes the space,
     so the controls sit on one axis however long the issuers are. */
  .provider {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-150);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .about { display: grid; gap: var(--sp-025); flex: 1 1 20ch; min-inline-size: 0; }
  .name { font-weight: var(--fw-medium); }
  .detail { color: var(--text-secondary); font-size: var(--fs-100); overflow-wrap: anywhere; }

  .confirm { display: grid; gap: var(--sp-100); }

  /* A form is neither prose nor a table, and it is the third case of ADR-0065 decision 2: an
     input as wide as the region is a target nobody aims at, so the fields carry a measure of
     their own while the lists beside them take the width. */
  form { margin: 0; max-inline-size: 52ch; }

  /* The surface a form stands on, as the other screens of the section draw one: a standalone
     element in the sense of design-system.md rule 1, on the frame's canvas. */
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
