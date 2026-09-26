<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Signing in, as the small step machine the server already describes.
  //
  // **What the server answers is the step.** `201` is a session, `202` carries the methods still
  // owed, and a refusal is one sentence. The screen has no state of its own beyond which of those
  // came back last: a code, an enrolment, a new password the rules now demand, or the first form
  // again. A step the server does not name is a step this screen cannot invent.
  //
  // **One refusal, and this screen adds nothing to it.** A wrong password and an address nobody
  // holds produce the same answer, byte for byte (`security.md` T-02). So there is no "unknown
  // address" state, no per-field error from the server, and no hint about which half was wrong.
  // The only field errors are this screen's own: a box left empty.
  //
  // **Four tones, four causes.** A refusal is `danger`; a session that ended is `info`, because
  // nothing was wrong and the path is remembered; a server that cannot be reached is `warning`
  // with the form still usable; a recovery code that was spent is `success` on the other side.
  // One slot, never two banners.
  //
  // The password is treated as a secret: a password field, `preventDefault` before anything else
  // so no native GET can carry it in a URL, never logged, never written into a message, and out of
  // this component's state the moment an attempt is over.

  import { Banner, Button, CodeField, Input, Stack } from '@hubtask/design-system/components';

  import PasswordField from '../lib/signin/PasswordField.svelte';
  import ProviderMark from '../lib/signin/ProviderMark.svelte';
  import SignInCard from '../lib/signin/SignInCard.svelte';
  import TotpEnrollment from '../lib/frame/TotpEnrollment.svelte';
  import { oidc } from '../lib/data/oidc.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { password as passwordApi } from '../lib/data/password.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { session } from '../lib/session.svelte.ts';
  import { remaining } from '../lib/signin/expiry.ts';
  import { ordered, readLastProvider, rememberProvider } from '../lib/signin/lastMethod.ts';

  let email = $state('');
  let password = $state('');
  let newPassword = $state('');
  let code = $state('');
  let recoveryCode = $state('');
  let usingRecovery = $state(false);
  let missingEmail = $state(false);
  let missingPassword = $state(false);
  /** The forgotten-password detour: a mode of this screen rather than an address of its own. */
  let asking = $state(false);

  const isBusy = $derived(session.status === 'verifying');
  const owed = $derived(session.secondFactorOwed);
  /** The enforcement path: enrolment rather than a code. */
  const mustEnroll = $derived(owed?.methods.includes('ENROLL') ?? false);
  const mustChange = $derived(session.mustChangePassword);
  const hasRecovery = $derived(owed?.methods.includes('RECOVERY') ?? false);
  /**
   * The providers, with the one this browser used last at the top.
   *
   * Read once rather than on every render: the order may not change under somebody's finger while
   * they are reaching for a button.
   */
  const lastProvider = readLastProvider();
  const providers = $derived(ordered(signInRules.providers, lastProvider));

  // The rules and the legal links, read once. Public, and the workspace comes from the host.
  $effect(() => signInRules.read());

  /** A clock over the pending credential, so a code that is about to be useless says so. */
  let now = $state(Date.now());
  $effect(() => {
    if (!owed?.expiresAt) return;
    const tick = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(tick);
  });
  const expiry = $derived(owed?.expiresAt ? remaining(owed.expiresAt, now) : undefined);

  async function complete(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    await session.completeSecondFactor(
      usingRecovery ? { recoveryCode: recoveryCode.trim() } : { code: code.trim() },
    );
    // Out of the component's state whatever happened. A recovery code works exactly once, and one
    // left in a field is one somebody may try to use again.
    code = '';
    recoveryCode = '';
  }

  async function setNewPassword(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    await session.setPasswordAndSignIn(newPassword);
    newPassword = '';
  }

  /**
   * Hands the browser to one of the workspace's providers (H-04).
   *
   * The address goes with it as a `login_hint` where one has been typed, so somebody does not type
   * it twice. It decides nothing: which account signs in is the ID token's `sub` and only that.
   */
  async function useProvider(providerId?: string): Promise<void> {
    // Remembered before the hand-over, because nothing after it runs: the browser leaves.
    if (providerId) rememberProvider(providerId);
    const url = await oidc.begin(email, providerId);
    // Leaving this origin entirely, so nothing after this line runs. `assign` rather than
    // `replace`: Back should return to the sign-in screen, which is where somebody who changed
    // their mind at the provider wants to be.
    if (url) location.assign(url);
  }

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const address = email.trim();
    missingEmail = address === '';
    missingPassword = password === '';
    if (missingEmail || missingPassword) return;

    await session.signIn(address, password);
    // Out of the component's state whatever happened. A password kept for a retry is a password
    // sitting in memory for as long as the tab is open.
    password = '';
  }

  async function askForLink(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    await passwordApi.forget(email.trim());
  }
</script>

{#snippet notice()}
  <!-- One slot, and the second step does not use it: the heading and the sentence under it already
       say what is owed, and a banner repeating them is the same sentence twice on one card. What
       goes here is what the heading cannot say - a refusal, an outage, a session that ended. -->
  {#if oidc.failure}
    <!-- No provider configured, one switched off, or discovery unreachable: the server's own
         code, and the password form below is untouched — which is exactly the degradation
         `observability-reliability.md` §7 promises. -->
    <Banner tone="danger">{t(oidc.failure)}</Banner>
  {:else if passwordApi.wasSent}
    <Banner tone="success">{t('app.password.forgot_sent')}</Banner>
  {:else if session.problem}
    <!-- The server's one sentence for a refused sign-in, rather than a status code shown raw.
         `role="alert"` is `Banner`'s own for a danger tone: a refusal beside a form is heard. -->
    <Banner tone={session.problem.isServerFault ? 'warning' : 'danger'} title={session.problem.message}>
      {#if session.problem.reference}{session.problem.reference}{/if}
    </Banner>
  {:else if session.endedNotice}
    <!-- Not a refusal: nothing was wrong, the path is remembered, and the tone says so. -->
    <Banner tone="info" title={t('app.sign_in.session_ended')}>
      {t('app.sign_in.session_ended_body')}
    </Banner>
  {/if}
{/snippet}

{#if asking}
  <SignInCard title={t('app.password.forgot_title')} lead={t('app.password.forgot_body')} {notice}>
    <form onsubmit={askForLink}>
      <Stack gap="200">
        <Input
          label={t('app.sign_in.email_label')}
          bind:value={email}
          type="email"
          autocomplete="username"
          spellcheck={false}
          isRequired
        />
        <div class="row">
          <Button type="submit" tone="primary" isBusy={passwordApi.isWorking} busyLabel={t('app.password.forgot_sending')}>
            {t('app.password.forgot_submit')}
          </Button>
          <Button tone="subtle" onclick={() => ((asking = false), passwordApi.clear())}>
            {t('app.password.back_to_sign_in')}
          </Button>
        </div>
      </Stack>
    </form>
  </SignInCard>
{:else if mustEnroll}
  <!-- Not a code: this workspace demands a second factor of its administrators and this one has
       none yet, so the server routed the sign-in into enrolment. Confirming it *is* the sign-in. -->
  <SignInCard title={t('app.mfa.title')} step={{ index: 2, total: 2 }} {notice}>
    <TotpEnrollment
      pendingToken={session.pendingCredential}
      onarmed={(pair) => pair && session.hold(pair)}
    />
  </SignInCard>
{:else if mustChange}
  <SignInCard
    title={t('app.sign_in.must_change')}
    step={{ index: 2, total: 2 }}
    lead={t('app.sign_in.must_change_body')}
    {notice}
  >
    <form onsubmit={setNewPassword}>
      <Stack gap="200">
        {@render identity()}
        <PasswordField
          bind:value={newPassword}
          label={t('app.password.new_label')}
          context={{ email: session.signingInAs, workspaceHost: signInRules.workspaceHost }}
          proof={{ kind: 'pending', token: session.pendingCredential ?? '' }}
          hint={t('app.redeem.password_hint')}
        />
        <div class="row">
          <Button type="submit" tone="primary" {isBusy} busyLabel={t('app.sign_in.working')}>
            {t('app.sign_in.must_change_submit')}
          </Button>
        </div>
      </Stack>
    </form>
  </SignInCard>
{:else if owed}
  <SignInCard
    title={t('app.sign_in.second_factor')}
    step={{ index: 2, total: 2 }}
    lead={t('app.sign_in.second_factor_hint')}
    {notice}
  >
    <form onsubmit={complete}>
      <Stack gap="200">
        {@render identity()}
        {#if usingRecovery}
          <CodeField
            label={t('app.sign_in.recovery_label')}
            hint={t('app.sign_in.recovery_hint')}
            bind:value={recoveryCode}
            length={8}
            groupOf={4}
            isRequired
          />
        {:else}
          <CodeField
            label={t('app.sign_in.code_label')}
            hint={t('app.sign_in.code_hint')}
            bind:value={code}
            isRequired
          />
        {/if}
        {#if expiry}
          <p class="expiry">{t('app.sign_in.expires_in', { remaining: expiry })}</p>
        {/if}
        <div class="row">
          <Button type="submit" tone="primary" {isBusy} busyLabel={t('app.sign_in.working')}>
            {t('app.sign_in.submit')}
          </Button>
          {#if hasRecovery}
            <Button tone="subtle" onclick={() => (usingRecovery = !usingRecovery)}>
              {usingRecovery ? t('app.sign_in.use_code') : t('app.sign_in.use_recovery')}
            </Button>
          {/if}
        </div>
      </Stack>
    </form>
  </SignInCard>
{:else}
  <SignInCard title={t('app.sign_in.title')} {notice}>
    <Stack gap="250">
      {#if signInRules.hasPassword}
        <form onsubmit={submit}>
          <Stack gap="200">
            <Input
              label={t('app.sign_in.email_label')}
              error={missingEmail ? t('app.sign_in.email_required') : undefined}
              bind:value={email}
              type="email"
              autocomplete="username webauthn"
              spellcheck={false}
              isRequired
            />
            <Stack gap="050">
              <Input
                label={t('app.sign_in.password_label')}
                error={missingPassword ? t('app.sign_in.password_required') : undefined}
                bind:value={password}
                type="password"
                autocomplete="current-password"
                spellcheck={false}
                revealLabel={t('app.password.reveal')}
                hideLabel={t('app.password.hide')}
                isRequired
              />
              {#if signInRules.isServed}
                <p class="forgot">
                  <!-- Only where the installation serves the reset: a link to a route that answers
                       `404` is worse than no link, and the concept says so in its own words. -->
                  <button class="link" type="button" onclick={() => (asking = true)}>
                    {t('app.sign_in.forgot')}
                  </button>
                </p>
              {/if}
            </Stack>
            <Button type="submit" tone="primary" isFull {isBusy} busyLabel={t('app.sign_in.working')}>
              {t('app.sign_in.submit')}
            </Button>
          </Stack>
        </form>
      {/if}

      {#if providers.length > 0 || signInRules.rules === undefined}
        {#if signInRules.hasPassword}
          <p class="or"><span>{t('app.sign_in.or')}</span></p>
        {/if}
        <Stack gap="100">
          {#if providers.length === 0}
            <!-- Nothing has been read yet, or the installation answers no list: the button is
                 offered and the server decides, exactly as F4 settled it. -->
            <Button
              tone="secondary"
              isFull
              isBusy={oidc.isWorking}
              busyLabel={t('app.sign_in.provider_working')}
              onclick={() => void useProvider()}
            >
              {t('app.sign_in.provider')}
            </Button>
          {:else}
            {#each providers as provider (provider.id)}
              <Button
                tone="secondary"
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
          {/if}
        </Stack>
      {/if}
    </Stack>
  </SignInCard>
{/if}

{#snippet identity()}
  <!-- 3.3.7: what this flow already has is not asked for again. Shown, because a second step with
       no subject is a step somebody has to trust blindly. -->
  <p class="identity">
    <span class="who">{t('app.sign_in.signing_in_as')}</span>
    <span class="address">{session.signingInAs}</span>
    <button class="link" type="button" onclick={() => session.startOver()}>
      {t('app.sign_in.not_you')}
    </button>
  </p>
{/snippet}

<style>
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .identity {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--sp-050) var(--sp-150);
    margin: 0;
    padding: var(--sp-100) var(--sp-150);
    background: var(--bg-surface-sunken);
    border-radius: var(--r-md);
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }

  .identity .address { color: var(--text-primary); font-weight: var(--fw-medium); }
  .identity .link { margin-inline-start: auto; }

  .expiry {
    margin: 0;
    font-size: var(--fs-075);
    color: var(--text-subtle);
  }

  .forgot { margin: 0; text-align: end; }

  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--text-brand);
    font: inherit;
    font-size: var(--fs-075);
    text-decoration: underline;
    cursor: pointer;
  }

  .link:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  /* The separator between the password and the providers: a rule with a word in it, drawn rather
     than written, because "or" is a catalogue entry and the lines are layout. */
  .or {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .or::before,
  .or::after {
    content: '';
    flex: 1;
    height: var(--bw-hairline);
    background: var(--border-subtle);
  }
</style>
