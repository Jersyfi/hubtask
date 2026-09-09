<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's identity provider, configured (H-04).
  //
  // **The client secret goes one way, and this screen says so rather than pretending.** It is
  // sealed on the way in and is a member of no answer (E-02), so an already-configured provider
  // shows an empty secret field with a sentence explaining that leaving it empty is not "no
  // secret" — it is "the one already sealed". A row of dots standing in for a value nobody can
  // read would be a screen inventing a fact.
  //
  // **A provider is set whole.** `PUT` is the contract's shape, and the safe one: discovery runs
  // against the issuer before anything is stored, so an unreachable provider is refused here
  // rather than by the first person who tries to sign in.
  //
  // **Removal costs something, and it is stated before the button.** The accounts provisioned
  // through the provider keep their rows and their live sessions; what they lose is the way back
  // in. For an account that never had a password, that is the whole way back in.
  //
  // The area's navigation arrives with F4-08. This route is tagged `administration` from the day
  // it exists, so the area's own test finds it already tagged rather than having to classify it.

  import { untrack } from 'svelte';

  import { Banner, Button, Input, Spinner, Stack, Switch, Textarea } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import { identityProvider } from '../lib/data/identityprovider.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  let issuer = $state('');
  let clientId = $state('');
  let clientSecret = $state('');
  let domains = $state('');
  let enabled = $state(true);
  let confirmingRemoval = $state(false);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let saved = $state(false);
  /** Whether the fields have been filled from the last read. Once only, so typing is not overwritten. */
  let filled = false;

  $effect(() => {
    // `untrack` for the reason every store here records: the read writes the store and writing it
    // reads it, so an effect that tracked that read would re-run on its own first answer.
    const stop = untrack(() => identityProvider.open());
    return stop;
  });

  // Not named `state`: a variable of that name turns every `$state` in this file into a store
  // subscription, which is a compiler rule rather than a style preference.
  const reading = $derived(identityProvider.state);
  const provider = $derived(identityProvider.provider);

  /**
   * A read that failed with `404` is a workspace that has never configured one — the form, not an
   * error. Anything else is a refusal worth showing: a caller without the permission, a server
   * that could not answer.
   */
  const unconfigured = $derived(reading.status === 'failed' && reading.error.status === 404);
  const unreadable = $derived(
    reading.status === 'failed' && !unconfigured ? renderProblem(reading.error, messages) : undefined,
  );

  $effect(() => {
    const current = provider;
    if (!current || filled) return;
    filled = true;
    issuer = current.issuer;
    clientId = current.client_id;
    domains = (current.allowed_email_domains ?? []).join('\n');
    enabled = current.enabled;
  });

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    isWorking = true;
    failure = undefined;
    saved = false;
    try {
      await identityProvider.configure({
        issuer: issuer.trim(),
        client_id: clientId.trim(),
        // Empty means "keep the sealed one", which is only an answer the server can give: a `PUT`
        // that omitted the secret on a workspace with none would be refused, and that refusal is
        // the right one to show rather than a rule this screen invents about which case it is in.
        client_secret: clientSecret,
        enabled,
        allowed_email_domains: readDomains(domains),
      });
      // Out of the component's state at once. A secret kept for a second submission is a secret
      // sitting in a form for as long as the tab is open.
      clientSecret = '';
      saved = true;
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      isWorking = false;
    }
  }

  async function remove(): Promise<void> {
    isWorking = true;
    failure = undefined;
    saved = false;
    try {
      await identityProvider.remove();
      confirmingRemoval = false;
      filled = false;
      issuer = '';
      clientId = '';
      clientSecret = '';
      domains = '';
      enabled = true;
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
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.identity_provider.title')}</h1>
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
    {:else}
      {#if unconfigured}
        <Banner tone="info">{t('app.identity_provider.none')}</Banner>
      {/if}

      {#if failure}
        <!-- The server's own sentence: an issuer that could not be reached, a domain that is not
             one, a caller who may read this and not write it. -->
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {:else if saved}
        <Banner tone="success">{t('app.identity_provider.saved')}</Banner>
      {/if}

      <form onsubmit={save}>
        <Stack gap="200">
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
            label={t('app.identity_provider.client_id_label')}
            hint={t('app.identity_provider.client_id_hint')}
            bind:value={clientId}
            autocomplete="off"
            spellcheck={false}
            isRequired
          />
          <Input
            label={t('app.identity_provider.client_secret_label')}
            hint={provider
              ? t('app.identity_provider.client_secret_kept')
              : t('app.identity_provider.client_secret_hint')}
            bind:value={clientSecret}
            type="password"
            autocomplete="off"
            spellcheck={false}
            isRequired={!provider}
          />
          <Textarea
            label={t('app.identity_provider.domains_label')}
            hint={t('app.identity_provider.domains_hint')}
            bind:value={domains}
            rows={3}
            spellcheck={false}
          />
          <Switch
            label={t('app.identity_provider.enabled_label')}
            hint={t('app.identity_provider.enabled_hint')}
            bind:checked={enabled}
          />
          <div>
            <Button
              type="submit"
              tone="primary"
              isBusy={isWorking}
              busyLabel={t('app.identity_provider.saving')}
            >
              {t('app.identity_provider.save')}
            </Button>
          </div>
        </Stack>
      </form>

      {#if provider}
        <Stack gap="150">
          <h2 class="section">{t('app.identity_provider.remove_title')}</h2>
          <!-- Said before the button rather than in the dialog that follows it: somebody deciding
               whether to press it is the person who needs to know what it costs. -->
          <p class="quiet">{t('app.identity_provider.remove_cost')}</p>
          {#if confirmingRemoval}
            <Banner tone="warning">{t('app.identity_provider.remove_confirm')}</Banner>
            <div class="row">
              <Button
                tone="danger"
                isBusy={isWorking}
                busyLabel={t('app.identity_provider.removing')}
                onclick={() => void remove()}
              >
                {t('app.identity_provider.remove_now')}
              </Button>
              <Button tone="subtle" onclick={() => (confirmingRemoval = false)}>
                {t('app.identity_provider.keep')}
              </Button>
            </div>
          {:else}
            <div>
              <Button tone="secondary" onclick={() => (confirmingRemoval = true)}>
                {t('app.identity_provider.remove')}
              </Button>
            </div>
          {/if}
        </Stack>
      {/if}
    {/if}
  </Stack>
</div>

<style>
  /* Rule 4: a column that grows with its text and stops before it becomes a line nobody can read. */
  .screen { max-width: 60ch; }

  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  form { margin: 0; }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
