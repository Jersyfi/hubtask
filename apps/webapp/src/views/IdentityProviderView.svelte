<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's identity providers, configured - the *Ways and providers* tab of the sign-in
  // screen, under the list of ways that switches them, so it draws no head of its own.
  //
  // **Plural, and two levels deep.** The listing carries this workspace's own rows and the ones its
  // installation offers every workspace on it. An inherited row is not this workspace's to change,
  // so it carries no field and no remove. Whether it is a way in here is the workspace's own call -
  // "Für alle Arbeitsbereiche angeboten, nirgends an: jeder Owner schaltet selbst" (ADR-0070 §2) -
  // and that call, like switching any provider on or off, is made in one place: the list
  // of ways to sign in on the sign-in screen (UC-ID-11 check 8). This screen configures; it says
  // whether a provider is on here, and switches nothing.
  //
  // **The client secret goes one way, and this screen says so rather than pretending.** It is sealed
  // on the way in and is a member of no answer, so editing an existing provider shows an empty
  // secret field with a sentence explaining that leaving it empty is not "no secret" — it is "the one
  // already sealed". A row of dots standing in for a value nobody can read would be a screen
  // inventing a fact.
  //
  // **The fields are the one provider form** (`ProviderFields`), the installation's too: the modes a
  // preset permits come from the server, each said in the use case's own words (UC-ID-11 check 1).
  //
  // **What newcomers get is asked only where somebody new arrives**: under the two modes that create
  // accounts (UC-ID-11 check 5). The roles come from the manifest and never include OWNER, which the
  // server refuses for newcomers; the groups are the workspace's own.
  //
  // **Removal costs something, and it is stated before the button.** The accounts provisioned through
  // the provider keep their rows and their live sessions; what they lose is the way back in. For an
  // account that never had a password, that is the whole way back in.

  import { tick, untrack } from 'svelte';

  import { Badge, Banner, Button, ReadOnlyNotice, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { IdentityProvider, IdentityProviderConfiguration } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import ProviderMark from '../lib/signin/ProviderMark.svelte';
  import ProviderFields from '../lib/signin/ProviderFields.svelte';
  import { draftBody, draftOf, draftRefusal, emptyDraft } from '../lib/signin/issuer.ts';
  import { identityProvider } from '../lib/data/identityprovider.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { includedHere, mayHere } from '../lib/data/capability.svelte.ts';
  import NotInPlan from '../lib/workspace/NotInPlan.svelte';
  import ValueSource from '../lib/workspace/ValueSource.svelte';
  import { focusFirst } from '../lib/forms/missing.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  /** Which row the form is editing. `undefined` is closed; the empty string is a new one. */
  let editing = $state<string | undefined>(undefined);
  let draft = $state(emptyDraft('DOMAINS'));
  /** What newcomers get: empty is nothing, `role:<ROLE>` or `group:<id>` one of the two. */
  let newcomers = $state('');
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

  $effect(() => {
    // The groups a newcomer may be put in; read once for the form, like the presets.
    const stop = untrack(() => groups.open());
    return stop;
  });

  // Not named `state`: a variable of that name turns every `$state` in this file into a store
  // subscription, which is a compiler rule rather than a style preference.
  const reading = $derived(identityProvider.state);
  const own = $derived(identityProvider.own);
  const inherited = $derived(identityProvider.inherited);

  /**
   * A read that failed with `404` is an installation that does not serve the collection — and
   * there is nothing to configure here. Anything else is a
   * refusal worth showing: a caller without the permission, a server that could not answer.
   */
  const unavailable = $derived(reading.status === 'failed' && reading.error.status === 404);
  const unreadable = $derived(
    reading.status === 'failed' && !unavailable ? renderProblem(reading.error, messages) : undefined,
  );

  /** The preset of whichever kind the form is set to, where the presets have been read. */
  const preset = $derived(identityProvider.presetOf(draft.kind as IdentityProvider['kind']));

  /** Whether the mode chosen creates accounts, and so has newcomers to give anything to. */
  const createsAccounts = $derived(draft.provisioning === 'DOMAINS' || draft.provisioning === 'ANY');

  /**
   * What a newcomer may be given: nothing, a role of the manifest other than OWNER (the server
   * refuses that for newcomers), or one of the workspace's groups.
   */
  const newcomerOptions = $derived([
    { value: '', label: t('app.identity_provider.newcomers_nothing') },
    ...(manifest.value?.roles ?? [])
      .map((description) => description.role)
      .filter((role): role is NonNullable<typeof role> => role !== undefined && role !== 'OWNER')
      .map((role) => ({
        value: `role:${role}`,
        label: t('app.identity_provider.newcomers_role', { role: t(`app.people.role.${role.toLowerCase()}`) }),
      })),
    ...groups.all.map((group) => ({
      value: `group:${group.id}`,
      label: t('app.identity_provider.newcomers_group', { group: group.name }),
    })),
  ]);

  /** Opens the form on a new provider. */
  function add(): void {
    editing = '';
    draft = emptyDraft('DOMAINS');
    newcomers = '';
    position = identityProvider.own.length;
    failure = undefined;
    saved = false;
  }

  /** Opens it on one that exists. The secret stays empty, because nothing can read it back. */
  function edit(provider: IdentityProvider): void {
    editing = provider.id;
    draft = draftOf(provider);
    newcomers = provider.newcomer_role
      ? `role:${provider.newcomer_role}`
      : provider.newcomer_group_id
        ? `group:${provider.newcomer_group_id}`
        : '';
    position = provider.position;
    failure = undefined;
    saved = false;
  }

  function close(): void {
    editing = undefined;
    draft.clientSecret = '';
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    // Checked here rather than switching Save off: the field already says why the server would
    // refuse it; pressed, focus goes there and nothing is sent (UC-WRK-32 check 3).
    if (draftRefusal(draft, preset) !== undefined) {
      // Kept before the tick: an event's current target is gone once it has been dispatched.
      const form = event.currentTarget as HTMLFormElement;
      void tick().then(() => focusFirst(form));
      return;
    }
    isWorking = true;
    failure = undefined;
    saved = false;
    // No `enabled`: the field is deprecated, and the list of ways to sign in is the one place a
    // way in is turned on or off (UC-ID-11 check 8, ADR-0076 §5). A new provider is created off -
    // configuring is not offering - and a replaced one keeps its switch.
    const body: IdentityProviderConfiguration = { ...draftBody(draft, preset), position };
    // Sent only where somebody new arrives: the server refuses a newcomer setting under a mode that
    // creates nobody, and the configuration is set whole, so leaving it out is "nothing".
    if (createsAccounts && newcomers.startsWith('role:')) {
      body.newcomer_role = newcomers.slice('role:'.length) as NonNullable<IdentityProviderConfiguration['newcomer_role']>;
    } else if (createsAccounts && newcomers.startsWith('group:')) {
      body.newcomer_group_id = newcomers.slice('group:'.length);
    }
    try {
      if (editing === '' || editing === undefined) {
        await identityProvider.add(body);
      } else {
        await identityProvider.configure(editing, body);
      }
      // Out of the component's state at once. A secret kept for a second submission is a secret
      // sitting in a form for as long as the tab is open.
      draft.clientSecret = '';
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

  function problemOf(cause: unknown): ReturnType<typeof renderProblem> {
    return cause instanceof TransportError
      ? renderProblem(cause, messages)
      : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
  }
  /** Whether the reader may configure the workspace's providers (`MANAGE_MEMBERS`). */
  const mayManage = $derived(mayHere('MANAGE_MEMBERS').status === 'permitted');
  /**
   * Where the plan leaves the workspace's own providers out they rest: they sign nobody in, and
   * none is added or changed here - removing one stays (multi-tenancy.md §4.2, P-05).
   */
  const resting = $derived(includedHere('OWN_SIGN_IN_PROVIDERS') === false);
</script>

<Stack gap="300">
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
      <ReadOnlyNotice icon="info" text={t('app.identity_provider.not_served')} />
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
                {#if provider.enabled}
                  <!-- Said, not switched: whether it is a way in here is set in the list of ways to
                       sign in, the one place for it (UC-ID-11 check 8). -->
                  <Badge tone={provider.offered_here ? 'success' : 'neutral'}>
                    {provider.offered_here ? t('app.identity_provider.badge_on_here') : t('app.identity_provider.badge_off_here')}
                  </Badge>
                {:else}
                  <!-- Switched off at the installation. Not a refusal this workspace can lift, and
                       not a switch: a control that cannot do anything is a control somebody presses
                       twice before asking. -->
                  <Badge tone="warning">{t('app.identity_provider.badge_off')}</Badge>
                {/if}
                <!-- The installation's provider: whose it is, and whom to ask (UC-WRK-32 check 5). -->
                <ValueSource source="INSTANCE" lock="INSTANCE" value={provider.display_name} />
              </li>
            {/each}
          </ul>
        </Stack>
      {/if}

      <Stack gap="150">
        <h2 class="section">{t('app.identity_provider.own_title')}</h2>
        {#if resting}
          <NotInPlan body={t('app.plan.providers_body', { count: own.length, name: own[0]?.display_name ?? '' })} />
        {/if}
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
                {#if resting}
                  <Badge tone="neutral">{t('app.plan.resting')}</Badge>
                {:else if !provider.enabled}
                  <Badge tone="warning">{t('app.identity_provider.badge_off')}</Badge>
                {/if}
                {#if mayManage}
                <div class="row">
                  {#if !resting}
                  <Button tone="subtle" onclick={() => edit(provider)}>
                    {t('app.identity_provider.edit')}
                  </Button>
                  {/if}
                  <Button tone="subtle" onclick={() => (confirmingRemoval = provider.id)}>
                    {t('app.identity_provider.remove')}
                  </Button>
                </div>
                {/if}
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

        {#if editing === undefined && !resting}
          <!-- The reason for every refused control on this screen: the rows' edit and remove are
               absent where the reader may not configure providers. -->
          <WorkspaceGate permission="MANAGE_MEMBERS">
            {#snippet actions()}
            <div>
              <Button tone="primary" onclick={add}>{t('app.identity_provider.add')}</Button>
            </div>
            {/snippet}
          </WorkspaceGate>
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
            <ProviderFields bind:draft presets={identityProvider.presets} level="workspace" isNew={editing === ''}>
              {#snippet more()}
                {#if createsAccounts}
                  <Select
                    label={t('app.identity_provider.newcomers_label')}
                    hint={t('app.identity_provider.newcomers_hint')}
                    options={newcomerOptions}
                    bind:value={newcomers}
                  />
                {/if}
              {/snippet}
            </ProviderFields>
            <!-- No switch here: a provider is turned on or off in the list of ways to sign in,
                 and nowhere else (UC-ID-11 check 8). A new one is saved off. -->
            <p class="quiet">{t('app.identity_provider.switched_in_sign_in')}</p>
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
    box-shadow: var(--shadow-raised);
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
    box-shadow: var(--shadow-raised);
  }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
