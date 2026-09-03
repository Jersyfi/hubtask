<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The screen that says what it is rather than pretending.
  //
  // There is no login route in `api/openapi.yaml` and no session endpoint: one security scheme, a
  // bearer. So this asks for the token `hubctl` already uses, says where to get one, and says in
  // one sentence that signing in with an account arrives with the OIDC connection in `0.6.0` -
  // which is the milestone that replaces this file.
  //
  // The token is treated as a secret: a password field with autocomplete off, never in a URL
  // (`preventDefault` before anything else, so no native GET can carry it), never in a log, and
  // never written into a message. The only thing that reads it back is the engine, per request,
  // through the platform seam.

  import { Banner, Button, Input, Stack } from '@hubtask/design-system/components';

  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { session } from '../lib/session.svelte.ts';

  let token = $state('');
  let missing = $state(false);

  const isBusy = $derived(session.status === 'verifying');

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const value = token.trim();
    missing = value === '';
    if (missing) return;

    if (await session.signIn(value)) {
      // Out of the field and out of the component's state the moment it is no longer needed.
      token = '';
    }
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.sign_in.title')}</h1>

    <Banner tone="info">{t('app.sign_in.temporary')}</Banner>

    {#if session.problem}
      <!-- The server's sentence for it - `errors.unauthenticated` for a token it refused - rather
           than a status code shown raw. -->
      <Banner tone="danger" title={session.problem.message}>
        {#if session.problem.reference}{session.problem.reference}{/if}
      </Banner>
    {/if}

    <form onsubmit={submit}>
      <Stack gap="200">
        <Input
          label={t('app.sign_in.token_label')}
          hint={t('app.sign_in.token_hint')}
          error={missing ? t('app.sign_in.token_required') : undefined}
          bind:value={token}
          type="password"
          autocomplete="off"
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
