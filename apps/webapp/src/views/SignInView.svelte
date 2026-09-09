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
  // **A `202` is not a failure.** It means the password was right and a second factor is owed.
  // F4-04 builds the code screen; until then the message says what is owed rather than pretending
  // the sign-in failed.
  //
  // The password is treated as a secret: a password field, `preventDefault` before anything else
  // so no native GET can carry it in a URL, never logged, never written into a message.

  import { Banner, Button, Input, Stack } from '@hubtask/design-system/components';

  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { session } from '../lib/session.svelte.ts';

  let email = $state('');
  let password = $state('');
  let missingEmail = $state(false);
  let missingPassword = $state(false);

  const isBusy = $derived(session.status === 'verifying');
  const owed = $derived(session.secondFactorOwed);

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
        {t('app.sign_in.second_factor_pending')}
      </Banner>
    {:else if session.problem}
      <!-- The server's one sentence for a refused sign-in, rather than a status code shown raw. -->
      <Banner tone="danger" title={session.problem.message}>
        {#if session.problem.reference}{session.problem.reference}{/if}
      </Banner>
    {/if}

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
        <div>
          <Button type="submit" tone="primary" {isBusy} busyLabel={t('app.sign_in.working')}>
            {t('app.sign_in.submit')}
          </Button>
        </div>
      </Stack>
    </form>
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
</style>
