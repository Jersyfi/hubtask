<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // "Do you allow this app?" — the screen `POST /oauth/authorize` has been waiting for (H-05).
  //
  // **The app is named, and that is the whole security argument for the read behind it.** A consent
  // screen that showed an identifier would be asking somebody to approve a UUID, which is exactly
  // what a phishing attempt wants. The name comes from the narrow read this task added.
  //
  // **The scopes are sentences, not identifiers.** Somebody deciding what to allow cannot decide
  // about `items:write`. A scope this build has no wording for shows its identifier rather than
  // nothing — hiding it would be asking for consent to something the person cannot see.
  //
  // **A scope never grants more than the person holds.** That is the server's rule and it is stated
  // here, because "allow this app to write entries" reads like a promotion otherwise.
  //
  // **Nothing is pre-empted.** A redirect URI the app did not register, a missing challenge, a
  // method that is not `S256` — all are the server's refusals, rendered in its own words. A client
  // that checked first would be a second implementation of a rule that moves.
  //
  // **The redirect at the end is a top-level navigation to a URI the server validated**, not a
  // fetch, so no policy directive is involved — and the address is built from the server's answer
  // rather than from anything this screen composed.

  import { Banner, Button, Spinner, Stack } from '@hubtask/design-system/components';

  import { consent, type AppSummary } from '../lib/data/consent.svelte.ts';
  import { completionUrl, declineUrl, readRequest } from '../lib/data/oauth.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  /** The request, read once. `undefined` means there is no request in this address at all. */
  const request = $state(readRequest(location.search));

  let app = $state<AppSummary | undefined>(undefined);
  let isNaming = $state(true);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  $effect(() => {
    if (!request) {
      isNaming = false;
      return;
    }
    void consent.appNamed(request.clientId).then((named) => {
      app = named;
      isNaming = false;
    });
  });

  /**
   * A scope this build has wording for reads as a sentence; one it does not shows its identifier.
   *
   * `messages.has` rather than comparing `t()`'s answer to the code: the renderer *humanises* an
   * unknown code — `app.consent.scope.items_read` would come back as "Items read", which is a
   * sentence this client invented about a permission it does not understand. Asking the catalogue
   * whether it knows the code is the only way to tell the two apart.
   */
  function wordFor(scope: string): string {
    const code = `app.consent.scope.${scope.replace(/[:.]/g, '_')}`;
    return messages.has(code) ? t(code) : scope;
  }

  function leave(url: string | undefined): void {
    // Leaving this origin entirely, so nothing after this line runs. `assign` rather than
    // `replace`: Back should return here, which is where somebody who changed their mind wants
    // to be.
    if (url) location.assign(url);
  }

  async function approve(): Promise<void> {
    if (!request) return;
    failure = undefined;
    isWorking = true;
    try {
      const code = await consent.approve(request);
      leave(completionUrl(request.redirectUri, code, request.state));
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  function decline(): void {
    if (!request) return;
    // No call: declining creates nothing to record. The app is told in its own vocabulary rather
    // than left waiting.
    leave(declineUrl(request.redirectUri, request.state));
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.consent.title')}</h1>

    {#if !request}
      <!-- Reached without an authorization request: a bookmark, a reload after the redirect,
           somebody typing the path. A different thing from a request the server refuses. -->
      <Banner tone="warning">{t('app.consent.no_request')}</Banner>
      <p><a href="/">{t('app.back_to_start')}</a></p>
    {:else if isNaming}
      <p class="waiting"><Spinner label={t('app.consent.reading')} /> <span>{t('app.consent.reading')}</span></p>
    {:else}
      {#if failure}
        <!-- The server's own words: a redirect URI the app did not register, a challenge that is
             not S256, a scope this installation does not declare. -->
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {/if}

      <p class="lead">
        {#if app}
          {t('app.consent.asks', { app: app.name })}
        {:else}
          <!-- The name could not be read — the app is not registered here, or this reader may not
               read it. Saying so is better than printing the identifier as if it were a name. -->
          {t('app.consent.unnamed_app')}
        {/if}
      </p>

      <Stack gap="150">
        <h2 class="section">{t('app.consent.what_it_gets')}</h2>
        <ul class="scopes">
          {#each request.scopes as scope (scope)}
            <li>{wordFor(scope)}</li>
          {/each}
        </ul>
        <!-- The sentence that stops this reading like a promotion. -->
        <p class="quiet">{t('app.consent.never_more')}</p>
      </Stack>

      <div class="row">
        <Button
          tone="primary"
          isBusy={isWorking}
          busyLabel={t('app.consent.allowing')}
          onclick={() => void approve()}
        >
          {t('app.consent.allow')}
        </Button>
        <Button tone="subtle" onclick={decline}>{t('app.consent.decline')}</Button>
      </div>

      <p class="quiet small">{t('app.consent.withdraw_later')}</p>
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

  .lead { margin: 0; color: var(--text-primary); font-size: var(--fs-200); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .scopes { margin: 0; padding-inline-start: var(--sp-300); display: grid; gap: var(--sp-050); }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
