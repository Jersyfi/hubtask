<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The third-party apps this workspace has registered (H-05).
  //
  // **The redirect URIs are matched byte for byte**, at authorization and again at exchange. So the
  // form says that, and this screen normalises nothing: a trailing slash quietly removed here is an
  // app that fails to match its own address, and the failure appears much later and somewhere else.
  //
  // **Confidential or public is the app's shape, not a preference.** An app that can keep a secret
  // on a server is confidential and gets one; a native or single-page app cannot keep one and
  // brings PKCE instead. Getting it wrong is how a secret ends up in a bundle somebody can read.
  //
  // **Removing an app withdraws every grant that pointed at it** — the people who allowed it stop
  // being able to, and the sessions it holds refuse on their next request. Said before the button.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Checkbox, Input, OneTimeSecret, Spinner, Stack, Textarea } from '@hubtask/design-system/components';

  import { apps, type RegisteredApp } from '../lib/data/apps.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  let name = $state('');
  let uris = $state('');
  let confidential = $state(false);
  let registered = $state<RegisteredApp | undefined>(undefined);
  let removing = $state<string | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let isWorking = $state(false);

  $effect(() => untrack(() => apps.open()));

  // The secret dies with the screen, like every other one-time value in this client.
  $effect(() => () => (registered = undefined));

  const reading = $derived(apps.state);
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  /** One per line, and nothing else touched: they are matched byte for byte. */
  const readUris = (written: string) =>
    written.split('\n').map((one) => one.trim()).filter((one) => one !== '');

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
    }
  }

  async function register(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const list = readUris(uris);
    if (!name.trim() || list.length === 0) return;
    await attempt(async () => {
      registered = await apps.register(name.trim(), list, confidential);
      name = '';
      uris = '';
      confidential = false;
    });
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.apps.title')}</h1>
    <p class="quiet">{t('app.apps.intro')}</p>

    {#if registered}
      {#if registered.client_secret}
        <OneTimeSecret
          value={registered.client_secret}
          label={t('app.apps.secret_title')}
          hint={t('app.apps.secret_hint')}
          revealLabel={t('app.apps.reveal')}
          hideLabel={t('app.apps.hide')}
          copyLabel={t('app.apps.copy')}
          copiedLabel={t('app.apps.copied')}
          acknowledgementLabel={t('app.apps.kept')}
          notAcknowledgedReason={t('app.apps.keep_first')}
          dismissLabel={t('app.apps.done')}
          onDismiss={() => (registered = undefined)}
        />
      {:else}
        <!-- A public client has no secret at all, and saying so is better than an empty panel
             somebody reads as a failure. -->
        <Banner tone="info">{t('app.apps.no_secret')}</Banner>
      {/if}
    {/if}

    {#if failure}<Banner tone="danger">{failure}</Banner>{/if}

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting"><Spinner label={t('app.apps.reading')} /> <span>{t('app.apps.reading')}</span></p>
    {:else if refusal}
      <Banner tone="danger" title={refusal.message}>
        {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
      </Banner>
    {:else}
      {#each apps.all as app (app.id)}
        <section class="app">
          <Stack gap="150">
            <div class="head">
              <h2 class="name">{app.name}</h2>
              <Badge tone={app.confidential ? 'info' : 'neutral'}>
                {app.confidential ? t('app.apps.confidential') : t('app.apps.public')}
              </Badge>
            </div>
            <ul class="uris">
              {#each app.redirect_uris as uri (uri)}<li>{uri}</li>{/each}
            </ul>

            {#if removing === app.id}
              <Banner tone="warning">{t('app.apps.remove_cost')}</Banner>
              <div class="row">
                <Button
                  tone="danger"
                  isBusy={isWorking}
                  busyLabel={t('app.apps.working')}
                  onclick={() =>
                    void attempt(async () => {
                      await apps.remove(app.id);
                      removing = undefined;
                    })}
                >
                  {t('app.apps.remove_now')}
                </Button>
                <Button tone="subtle" onclick={() => (removing = undefined)}>{t('app.apps.keep')}</Button>
              </div>
            {:else}
              <div>
                <Button size="sm" tone="secondary" onclick={() => (removing = app.id)}>
                  {t('app.apps.remove')}
                </Button>
              </div>
            {/if}
          </Stack>
        </section>
      {:else}
        <p class="quiet">{t('app.apps.none')}</p>
      {/each}
    {/if}

    <Stack gap="150">
      <h2 class="section">{t('app.apps.new_title')}</h2>
      <form onsubmit={register}>
        <Stack gap="200">
          <Input label={t('app.apps.name')} hint={t('app.apps.name_hint')} bind:value={name} isRequired />
          <Textarea
            label={t('app.apps.uris')}
            hint={t('app.apps.uris_hint')}
            bind:value={uris}
            rows={3}
            spellcheck={false}
          />
          <Checkbox
            label={t('app.apps.is_confidential')}
            hint={t('app.apps.is_confidential_hint')}
            checked={confidential}
            onchange={(event: Event) => (confidential = (event.currentTarget as HTMLInputElement).checked)}
          />
          <div>
            <Button type="submit" tone="primary" isBusy={isWorking} busyLabel={t('app.apps.registering')}>
              {t('app.apps.register')}
            </Button>
          </div>
        </Stack>
      </form>
    </Stack>
  </Stack>
</div>

<style>
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .section,
  .name { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .app {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .head .name { flex: 1 1 auto; min-width: 0; }

  .uris {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: var(--sp-050);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    overflow-wrap: anywhere;
  }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  form { margin: 0; }
</style>
