<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // "Do you allow this app?" — the screen `POST /oauth/authorize` has been waiting for.
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

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { consent, type AppSummary } from '../lib/data/consent.svelte.ts';
  import { completionUrl, declineUrl, missingParts, readRequest } from '../lib/data/oauth.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import SignInCard from '../lib/signin/SignInCard.svelte';

  /** The request, read once. `undefined` means there is no request in this address at all. */
  const request = $state(readRequest(location.search));
  /** What an app's request lacks, where an app sent one that cannot proceed. */
  const missing = missingParts(location.search);

  let app = $state<AppSummary | undefined>(undefined);
  let isNaming = $state(true);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  /**
   * The installed apps' own request (ADR-0096): the built-in client, named by its device. It asks
   * for a session's scopes, said in one sentence rather than thirty lines, and the page warns that
   * a program opening it by itself would get the access (the approved preview's two choices).
   */
  const isApp = $derived(
    request?.device !== undefined && manifest.value?.app_client_id !== undefined && request.clientId === manifest.value.app_client_id,
  );

  $effect(() => {
    if (!request) {
      isNaming = false;
      return;
    }
    if (request.device !== undefined) {
      // The built-in client has no registration to read; the manifest says whether it is this one.
      isNaming = !manifest.isRead;
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
  $effect(() => page.entitle(t('app.consent.title')));

  /** A sentence with one of its parameters set apart, which the markup marks up. */
  function around(text: string, part: string): { before: string; part: string; after: string } {
    const at = part === '' ? -1 : text.indexOf(part);
    return at < 0 ? { before: text, part: '', after: '' } : { before: text.slice(0, at), part, after: text.slice(at + part.length) };
  }

  /**
   * The app's sentence with the device's name set apart, as the preview draws it: the name is the
   * one thing the person checks against the computer in front of them (ADR-0096 decision 3).
   */
  const appAsks = $derived(
    isApp && request?.device
      ? around(t('app.consent.app_asks', { platform: t(`app.app_platform.${request.platform}`), device: request.device }), request.device)
      : undefined,
  );
  /** Where the app is signed out later, named as the profile's screen is. */
  const signOutLater = $derived(around(t('app.consent.app_withdraw_later', { screen: t('app.grants.title') }), t('app.grants.title')));
</script>

<!-- The workspace's own host beside the mark, as on its sign-in card: the address a person checks
     before allowing anything. -->
<SignInCard title={t('app.consent.title')} hostLine={location.hostname}>
  {#if !request && missing.length > 0}
    <!-- An app asked, but its request lacks what the server requires - most often a skill whose
         console has PKCE switched off. Said precisely, because "nobody asks" sends the
         administrator looking in the wrong place. -->
    <Banner tone="warning">
      <Stack gap="100">
        <span>{t('app.consent.incomplete')}</span>
        <ul class="missing">
          {#each missing as part (part)}<li>{t(`app.consent.missing_${part}`)}</li>{/each}
        </ul>
      </Stack>
    </Banner>
    <p class="small"><a href="/">{t('app.back_to_start')}</a></p>
  {:else if !request}
    <!-- Reached without an authorization request: a bookmark, a reload after the redirect,
         somebody typing the path. A different thing from a request the server refuses. -->
    <Banner tone="warning">{t('app.consent.no_request')}</Banner>
    <p class="small"><a href="/">{t('app.back_to_start')}</a></p>
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

    {#if appAsks}
      <p class="asks">{appAsks.before}<b>{appAsks.part}</b>{appAsks.after}</p>
      <Banner tone="info">{t('app.consent.app_warning')}</Banner>
      <Stack gap="050">
        <h2 class="label">{t('app.consent.what_it_gets')}</h2>
        <p class="prose">{t('app.consent.app_gets')}</p>
      </Stack>
    {:else}
      <p class="asks">
        {#if app}
          {t('app.consent.asks', { app: app.name })}
        {:else}
          <!-- The name could not be read — the app is not registered here, or this reader may not
               read it. Saying so is better than printing the identifier as if it were a name. -->
          {t('app.consent.unnamed_app')}
        {/if}
      </p>

      <Stack gap="050">
        <h2 class="label">{t('app.consent.what_it_gets')}</h2>
        <ul class="scopes prose">
          {#each request.scopes as scope (scope)}
            <li>{wordFor(scope)}</li>
          {/each}
        </ul>
        <!-- The sentence that stops this reading like a promotion. -->
        <p class="prose">{t('app.consent.never_more')}</p>
      </Stack>
    {/if}

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

    {#if isApp}
      <p class="quiet small">{signOutLater.before}<em>{signOutLater.part}</em>{signOutLater.after}</p>
    {:else}
      <p class="quiet small">{t('app.consent.withdraw_later')}</p>
    {/if}
  {/if}
</SignInCard>

<style>
  .asks { margin: 0; color: var(--text-primary); font-size: var(--fs-200); }

  .label { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-primary); }

  .prose { margin: 0; color: var(--text-secondary); }

  .quiet { margin: 0; color: var(--text-subtle); }

  .small { margin: 0; font-size: var(--fs-075); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .scopes { padding-inline-start: var(--sp-300); display: grid; gap: var(--sp-050); }

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .missing { margin: 0; padding-inline-start: var(--sp-300); }
</style>
