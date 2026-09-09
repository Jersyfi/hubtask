<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The sign-in H-01 made possible: an email and a password.
  //
  // **One refusal, and this screen adds nothing to it.** A wrong password and an address nobody
  // holds produce the same answer, byte for byte, because whether an account exists is exactly
  // what a guessing client is trying to learn (`security.md` T-02). So there is no "unknown
  // address" state here, no per-field error from the server, and no hint about which half was
  // wrong. The only field errors are this screen's own: a box left empty.
  //
  // **A `202` is not a failure.** It means the password was right and a second factor is owed, so
  // the form becomes the second step rather than repeating the first: a code from the
  // authenticator, or one of the ten recovery codes shown at enrolment. `ENROLL` is the third
  // answer the server can give and it is not a code at all — a tenant switch routing an
  // unenrolled administrator into enrolment instead of into a session — so that one becomes the
  // enrolment panel rather than a field.
  //
  // The password is treated as a secret: a password field, `preventDefault` before anything else
  // so no native GET can carry it in a URL, never logged, never written into a message.

  import { Banner, Button, Input, Stack } from '@hubtask/design-system/components';

  import TotpEnrollment from '../lib/frame/TotpEnrollment.svelte';
  import { oidc } from '../lib/data/oidc.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { session } from '../lib/session.svelte.ts';

  let email = $state('');
  let password = $state('');
  let code = $state('');
  let recoveryCode = $state('');
  let usingRecovery = $state(false);
  let missingEmail = $state(false);
  let missingPassword = $state(false);

  const isBusy = $derived(session.status === 'verifying');
  const owed = $derived(session.secondFactorOwed);
  /** The enforcement path: enrolment rather than a code. */
  const mustEnroll = $derived(owed?.methods.includes('ENROLL') ?? false);
  const hasRecovery = $derived(owed?.methods.includes('RECOVERY') ?? false);

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

  /**
   * Hands the browser to the workspace's provider (H-04).
   *
   * **The button is always here, and the server decides whether it works.** Whether a provider is
   * configured sits behind a permission a signed-out visitor does not hold, so this screen cannot
   * ask — and a second unauthenticated endpoint that existed only to hide a button would be a new
   * thing to attack for the sake of a nicer screen. The refusal is a clear code and it is rendered.
   *
   * The address goes with it as a `login_hint` where one has been typed, so somebody does not type
   * it twice. It decides nothing: which account signs in is the ID token's `sub` and only that.
   */
  async function useProvider(): Promise<void> {
    const url = await oidc.begin(email);
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
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.sign_in.title')}</h1>

    {#if owed}
      <!-- The password was right. What is missing is the second step, and the enrolment and code
           screens arrive with F4-04. -->
      <Banner tone="info" title={t('app.sign_in.second_factor')}>
        {mustEnroll ? t('app.sign_in.must_enrol') : t('app.sign_in.second_factor_hint')}
      </Banner>
    {:else if oidc.failure}
      <!-- No provider configured, one switched off, or discovery unreachable: the server's own
           code, and the password form below is untouched — which is exactly the degradation
           `observability-reliability.md` §7 promises. -->
      <Banner tone="danger">{t(oidc.failure)}</Banner>
    {:else if session.problem}
      <!-- The server's one sentence for a refused sign-in, rather than a status code shown raw. -->
      <Banner tone="danger" title={session.problem.message}>
        {#if session.problem.reference}{session.problem.reference}{/if}
      </Banner>
    {/if}

    {#if mustEnroll}
      <!-- Not a code: this workspace demands a second factor of its administrators and this one
           has none yet, so the server routed the sign-in into enrolment. The panel presents the
           pending credential the `202` carried, and confirming it *is* the sign-in. -->
      <TotpEnrollment
        pendingToken={session.pendingCredential}
        onarmed={(pair) => pair && session.hold(pair)}
      />
    {:else if owed}
      <form onsubmit={complete}>
        <Stack gap="200">
          {#if usingRecovery}
            <Input
              label={t('app.sign_in.recovery_label')}
              hint={t('app.sign_in.recovery_hint')}
              bind:value={recoveryCode}
              autocomplete="one-time-code"
              spellcheck={false}
              isRequired
            />
          {:else}
            <Input
              label={t('app.sign_in.code_label')}
              hint={t('app.sign_in.code_hint')}
              bind:value={code}
              autocomplete="one-time-code"
              inputmode="numeric"
              spellcheck={false}
              isRequired
            />
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
    {:else}
    <form onsubmit={submit}>
      <Stack gap="200">
        <Input
          label={t('app.sign_in.email_label')}
          error={missingEmail ? t('app.sign_in.email_required') : undefined}
          bind:value={email}
          type="email"
          autocomplete="username"
          spellcheck={false}
          isRequired
        />
        <Input
          label={t('app.sign_in.password_label')}
          error={missingPassword ? t('app.sign_in.password_required') : undefined}
          bind:value={password}
          type="password"
          autocomplete="current-password"
          spellcheck={false}
          isRequired
        />
        <div class="row">
          <Button type="submit" tone="primary" {isBusy} busyLabel={t('app.sign_in.working')}>
            {t('app.sign_in.submit')}
          </Button>
          <!-- Beside the password rather than instead of it. Local accounts keep signing in when
               a provider cannot be reached, and they only do that if this form is still here. -->
          <Button
            tone="subtle"
            isBusy={oidc.isWorking}
            busyLabel={t('app.sign_in.provider_working')}
            onclick={() => void useProvider()}
          >
            {t('app.sign_in.provider')}
          </Button>
        </div>
      </Stack>
    </form>
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

  form { margin: 0; }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
