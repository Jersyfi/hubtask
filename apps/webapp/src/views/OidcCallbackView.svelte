<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where the provider sends the browser back.
  //
  // **This address is the server's, not this screen's.** `cmd/server/main.go` derives
  // `<base>/auth/callback` as the redirect URI and registers it with the provider; nothing about
  // where the code comes back is taken from a request. So the path is fixed, and this is the
  // screen under it.
  //
  // **It is the sign-in card, not a page of the application** (UC-ID-08 check 5). The person is
  // not signed in yet, so the frame's navigation would be controls for a product nobody has
  // entered; the card is what the signed-out screens are, and it names the workspace.
  //
  // **The code and the state leave the address before anything is sent.** An authorization code is
  // a credential for one exchange, and one left in the address bar is one in the history entry, in
  // a bookmark, and in the `Referer` of whatever the reader clicks next. `replaceState` rather
  // than `pushState`, for the same reason `RedeemView` uses it: a Back that returned to the
  // address with the code in it would put the code back.
  //
  // **Nothing here is offered twice.** A single-use `state` means a reload is not a retry, and the
  // way out of every failure is the same one sentence and a way back to the sign-in card.
  //
  // **It is also where a step-up at the provider comes back** (ADR-0075 §2). The provider knows one
  // callback address, so a person who left the page to confirm an action with their provider
  // returns here too. The tab remembered that before it left; such a return finishes the step-up
  // rather than a sign-in, and sends the person back to where they were, where the frame says the
  // confirmation holds.

  import { Banner, Button, Spinner } from '@hubtask/design-system/components';

  import SignInCard from '../lib/signin/SignInCard.svelte';
  import { oidc } from '../lib/data/oidc.svelte.ts';
  import { readArrival } from '../lib/data/oidc.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { stepUp } from '../lib/data/stepup.svelte.ts';
  import { session } from '../lib/session.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  interface Props {
    /** Where to go once there is a session, or back to the card. The router, as every view takes it. */
    onnavigate?: (path: string) => void;
  }

  let { onnavigate }: Props = $props();

  /**
   * The address, read once and then cleaned.
   *
   * `$state` rather than a derivation over `location`: the value has to survive the very
   * `replaceState` that removes it.
   */
  const arrival = $state(takeArrival());

  /**
   * Whether this return finishes a step-up, read once - completing it takes the note away. Only for
   * a tab with a session: one that has none is signing in, whatever an old note says, and the note
   * goes.
   */
  const isStepUp = takeStepUp();

  function takeStepUp(): boolean {
    const returning = stepUp.isReturning();
    if (returning && !session.isSignedIn) stepUp.forget();
    return returning && session.isSignedIn;
  }
  /** Where the person was before they left for the provider, once known. */
  let returnTo = $state('/');
  const failure = $derived(isStepUp ? stepUp.failure : oidc.failure);

  function takeArrival() {
    const read = readArrival(location.search);
    if (location.search !== '') history.replaceState(null, '', location.pathname);
    return read;
  }

  // The workspace's name and legal links, which the card shows on every signed-out screen.
  $effect(() => signInRules.read());

  $effect(() => {
    if (isStepUp) {
      if (arrival.kind === 'handoff') {
        void stepUp.completeAtProvider(arrival.handoff).then((done) => {
          returnTo = done.returnTo;
          if (done.ok) onnavigate?.(done.returnTo);
        });
      } else {
        returnTo = stepUp.abandonAtProvider();
      }
      return;
    }
    if (arrival.kind === 'handoff') {
      void oidc.complete(arrival.handoff).then((ok) => ok && onnavigate?.('/'));
    } else {
      // The provider refused, or there is no flow in this address at all. Both end here, and
      // saying which would tell somebody standing at the callback whether a handle they do not
      // hold was ever real.
      oidc.refuse();
    }
    return () => oidc.forget();
  });
</script>

{#snippet notice()}
  {#if failure}
    <!-- The server's own code: a spent or unknown `state`, a provider that could not be reached, a
         workspace whose provider is switched off - or, for a step-up, a different account at the
         provider, or one that did not confirm a fresh sign-in. -->
    <Banner tone="danger">{t(failure)}</Banner>
  {/if}
{/snippet}

<!-- The heading says the state and the banner the reason: "Signing you in" above a refusal would be
     two sentences that contradict each other. -->
{#if isStepUp}
  <SignInCard title={t(failure ? 'app.callback.step_up_failed_title' : 'app.callback.step_up_title')} {notice}>
    {#if failure}
      <div>
        <Button tone="secondary" onclick={() => onnavigate?.(returnTo)}>{t('app.callback.step_up_back')}</Button>
      </div>
    {:else}
      <p class="quiet">
        <Spinner /> {t('app.callback.step_up_working')}
      </p>
    {/if}
  </SignInCard>
{:else}
  <SignInCard title={t(failure ? 'app.callback.failed_title' : 'app.callback.title')} {notice}>
    {#if failure}
      <div>
        <Button tone="secondary" onclick={() => onnavigate?.('/')}>{t('app.callback.back')}</Button>
      </div>
    {:else}
      <p class="quiet">
        <Spinner /> {t('app.callback.working')}
      </p>
    {/if}
  </SignInCard>
{/if}

<style>
  .quiet {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }
</style>
