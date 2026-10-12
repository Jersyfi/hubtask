<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The moment an invited account becomes a person.
  //
  // **The token arrives in the fragment, and that is the server's decision rather than this
  // screen's.** `DeliverNotification.go` links `<base>/redeem#token=…`: a fragment is never sent
  // to any server, never reaches an access log, and never travels in a `Referer` to whatever the
  // invitation mail was rendered by. This screen owes it the same care in the other direction —
  // read once, and replaced in the history entry before the first request leaves, so that a
  // reader who presses Back, or bookmarks the page, is not carrying a live credential around.
  //
  // **The rules are the workspace's, and they are shown rather than guessed at.** This screen used
  // to say "at least twelve characters" from a constant, which is right until a workspace asks for
  // fifteen - and then it is a screen that lies and a password that is refused after it was typed.
  // `PasswordField` reads the rules this workspace actually applies and renders one line each.
  //
  // **One field, not two.** The eye is what replaces "repeat it" (3.3.8): a password that can be
  // read is one nobody has to type twice.
  //
  // **No password where the workspace has none**. A workspace that switched the password off
  // refuses one here, so this screen does not ask for it: it says the invitation is accepted by
  // signing in through the workspace's provider - which makes the account active. Nobody invited is
  // left in front of a field that cannot work.
  //
  // **The provider is chosen here, and the invitation goes with it** (ADR-0078 §1). A
  // provider's word alone activates no invited account: the second proof is this link, which the
  // server binds to the provider flow without spending it. So the buttons are on this card rather
  // than on the sign-in card - a person sent there would arrive without the invitation and be
  // refused by any provider that is not authoritative for their address.

  // **The terms of use are agreed here, where there are any** (UC-ID-19 check 1). The box is not a
  // condition of the buttons being drawn: pressing one without it says so at the box, which is
  // what P-05 asks of a control whose refusal the card can explain. The link and version agreed
  // to travel with the password and with the provider, so what was shown is what is stored.

  import { Banner, Button, Checkbox, Stack } from '@hubtask/design-system/components';

  import PasswordField from '../lib/signin/PasswordField.svelte';
  import ProviderMark from '../lib/signin/ProviderMark.svelte';
  import SignInCard from '../lib/signin/SignInCard.svelte';
  import { takeFragmentToken } from '../lib/signin/fragmentToken.ts';
  import { invitation } from '../lib/data/invitation.svelte.ts';
  import { oidc } from '../lib/data/oidc.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { session, type TermsInForce } from '../lib/session.svelte.ts';

  interface Props {
    /**
     * Where to go once there is a session. The frame's router, as the OIDC callback takes it.
     *
     * The screen has to send the person on itself: once there is a session, `/redeem` is no
     * longer a screen, and a person left on the address would be signed in and looking at
     * "nothing here". An invited person has no path to return to, so the start is where they go.
     */
    onnavigate?: (path: string) => void;
  }

  let { onnavigate }: Props = $props();

  let password = $state('');
  const isBusy = $derived(session.status === 'verifying');

  /**
   * The token, taken from the fragment once and then removed from the address.
   *
   * `$state` rather than a `$derived` over `location`: the value has to survive the very
   * `replaceState` that removes it, and a derivation over the address would answer nothing the
   * moment the address stopped carrying it.
   */
  const token = $state(takeFragmentToken());

  $effect(() => signInRules.read());
  // The workspace and the address, read once with the token in the body (UC-ID-07 check 1).
  $effect(() => {
    if (token !== '') void invitation.inspect(token);
  });
  /** The card's title: the workspace by name once it is known, the plain one until then. */
  const named = $derived(invitation.summary?.workspace_name);

  /** The terms of use in force here, or none - no box and nothing to agree to (UC-ID-19 check 5). */
  const terms = $derived.by((): TermsInForce | undefined => {
    const legal = signInRules.legal;
    if (!legal.terms_url) return undefined;
    return {
      url: legal.terms_url,
      version: legal.terms_version,
      set_by: legal.terms_set_by ?? 'INSTALLATION',
    };
  });
  let agreed = $state(false);
  /** Set when a button was pressed without the box ticked; cleared by ticking it. */
  let agreementMissing = $state(false);
  $effect(() => {
    if (agreed) agreementMissing = false;
  });

  /** Whether the card may go on: no terms, or the box ticked - otherwise it says so at the box. */
  function mayContinue(): boolean {
    if (terms === undefined || agreed) return true;
    agreementMissing = true;
    return false;
  }

  /**
   * Hands the browser to one of the workspace's providers with this invitation bound to the flow.
   *
   * An invitation that cannot be redeemed is refused before the browser leaves, and the refusal is
   * shown on this card.
   */
  async function useProvider(providerId: string): Promise<void> {
    if (!mayContinue()) return;
    const url = await oidc.begin(undefined, providerId, token, undefined, terms);
    // Leaving this origin entirely. `assign` rather than `replace`: Back returns to this card, which
    // is where somebody who changed their mind at the provider wants to be - with the token already
    // out of the address, so the history entry carries no credential.
    if (url) location.assign(url);
  }

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!mayContinue()) return;
    const signedIn = await session.redeem(token, password, terms);
    // Out of the component's state whatever happened.
    password = '';
    if (signedIn) onnavigate?.('/');
  }
</script>

{#snippet notice()}
  {#if oidc.failure}
    <!-- The server's own code: an invitation that cannot be redeemed, or a provider that is not
         there. -->
    <Banner tone="danger">{t(oidc.failure)}</Banner>
  {:else if session.problem}
    <Banner tone="danger" title={session.problem.message}>
      {#if session.problem.reference}{session.problem.reference}{/if}
    </Banner>
  {:else if invitation.failure}
    <!-- The invitation would not redeem: the redemption's one sentence, before anything is typed. -->
    <Banner tone="danger">{t(invitation.failure)}</Banner>
  {/if}
{/snippet}

{#snippet invited()}
  <!-- Whose invitation this is: the address it went to, which a provider has to confirm too. -->
  {#if invitation.summary}
    <p class="invited">
      <span class="who">{t('app.redeem.invited_as')}</span>
      <span class="address">{invitation.summary.email}</span>
    </p>
  {/if}
{/snippet}

{#if token === ''}
  <SignInCard title={t('app.redeem.title')} {notice}>
    <!-- Reached without a link, or with one that has already been used and the fragment lost on
         the way. Either way there is nothing to redeem, and saying which would tell a probe
         whether the token was real. -->
    <Banner tone="warning">{t('app.redeem.no_token')}</Banner>
  </SignInCard>
{:else if signInRules.wasRead && !signInRules.hasPassword}
  <SignInCard
    title={named ? t('app.redeem.invited_title', { workspace: named }) : t('app.redeem.provider_title')}
    lead={t('app.redeem.provider_intro')}
    {notice}
  >
    {@render invited()}
    {#if signInRules.providers.length > 0}
      {@render agreement()}
      {@render providers('primary')}
    {:else}
      <!-- No provider the rules name: the sign-in card says what there is, which is all this
           card could say too. -->
      <Button tone="primary" isFull onclick={() => onnavigate?.('/')}>{t('app.redeem.to_sign_in')}</Button>
    {/if}
  </SignInCard>
{:else}
  <SignInCard
    title={named ? t('app.redeem.invited_title', { workspace: named }) : t('app.redeem.title')}
    lead={t(named ? 'app.redeem.invited_intro' : 'app.redeem.intro')}
    {notice}
  >
    <form onsubmit={submit}>
      <Stack gap="200">
        {@render invited()}
        <PasswordField
          bind:value={password}
          label={t('app.redeem.password_label')}
          context={{ email: invitation.summary?.email, workspaceHost: signInRules.workspaceHost }}
          proof={{ kind: 'invitation', token }}
          hint={t('app.redeem.password_hint')}
        />
        {@render agreement()}
        <Button type="submit" tone="primary" isFull {isBusy} busyLabel={t('app.redeem.working')}>
          {t('app.redeem.submit')}
        </Button>
      </Stack>
    </form>
    {#if signInRules.providers.length > 0}
      <!-- UC-ID-07 check 5: where the workspace offers a provider, the invitation is accepted
           through it as well as with a password - the card with the buttons does it. -->
      <div class="instead">
        <p class="quiet">{t('app.redeem.or_provider')}</p>
        {@render providers('secondary')}
      </div>
    {/if}
  </SignInCard>
{/if}

{#snippet agreement()}
  <!-- Only where terms are set. The link opens beside the card, as the footer's does, and the box
       names the version agreed to, which is what the account keeps. -->
  {#if terms}
    <div class="agreement">
      <a class="terms" href={terms.url} target="_blank" rel="noopener noreferrer">
        {terms.version
          ? t('app.terms.read_versioned', { version: terms.version })
          : t('app.terms.read')}
      </a>
      <Checkbox
        bind:checked={agreed}
        label={terms.version
          ? t('app.terms.agree_label_versioned', { version: terms.version })
          : t('app.terms.agree_label')}
        aria-invalid={agreementMissing}
        aria-describedby={agreementMissing ? 'terms-missing' : undefined}
      />
      <!-- Mounted empty and filled when a button was pressed without the box: a region created
           with its sentence is one nothing was listening to. -->
      <p id="terms-missing" class="missing" role="alert">
        {agreementMissing ? t('auth.terms_not_agreed') : ''}
      </p>
    </div>
  {/if}
{/snippet}

{#snippet providers(tone: 'primary' | 'secondary')}
  <!-- One button per provider the rules name, each carrying this invitation (ADR-0078 §1). -->
  <Stack gap="100">
    {#each signInRules.providers as provider (provider.id)}
      <Button
        {tone}
        isFull
        isBusy={oidc.isHandingOverTo(provider.id)}
        busyLabel={t('app.sign_in.provider_working')}
        onclick={() => void useProvider(provider.id)}
      >
        {#snippet lead()}
          <ProviderMark kind={provider.kind} name={provider.display_name} />
        {/snippet}
        {t('app.sign_in.provider_named', { name: provider.display_name })}
      </Button>
    {/each}
  </Stack>
{/snippet}

<style>
  form { margin: 0; }
  .instead { display: grid; gap: var(--sp-100); margin-block-start: var(--sp-200); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .invited {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-050) var(--sp-150);
    margin: 0;
    padding: var(--sp-100) var(--sp-150);
    background: var(--bg-surface-sunken);
    border-radius: var(--r-md);
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }
  .invited .address { color: var(--text-primary); font-weight: var(--fw-medium); }
  .agreement { display: grid; gap: var(--sp-100); }
  .terms { justify-self: start; }
  .missing { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .missing:empty { display: none; }
</style>
