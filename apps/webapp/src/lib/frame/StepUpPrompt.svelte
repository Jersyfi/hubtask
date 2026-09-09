<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The prompt a `403 auth.step_up_required` produces (H-03).
  //
  // **In the frame, not on a screen.** Any request may meet the refusal, and the screen that made
  // it is the one that retries — so the dialog is rendered once, here, and the store is what any
  // caller awaits. A control that knew in advance it needed a proof would be a control guessing at
  // the server's policy.
  //
  // Which methods are offered is the refusal's answer, never a guess: an account with an armed
  // factor is asked for a code, one without is asked for its password, and a refusal that named
  // nothing falls back to the password because that is the one every account has.

  import { Banner, Button, Dialog, Input, Stack } from '@hubtask/design-system/components';

  import { stepUp } from '../data/stepup.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  let password = $state('');
  let code = $state('');

  const pending = $derived(stepUp.pending);
  const wantsCode = $derived(pending?.methods.includes('TOTP') ?? false);

  // Cleared whenever a new prompt opens, so a value typed for the last privileged action is not
  // sitting in the field for the next one.
  $effect(() => {
    if (pending) {
      password = '';
      code = '';
    }
  });

  async function prove(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    await stepUp.prove(wantsCode ? { code: code.trim() } : { password });
    // Out of the component's state whatever happened: a proof kept for a retry is a credential
    // sitting in memory after the moment that needed it.
    password = '';
    code = '';
  }
</script>

{#if pending}
  <Dialog
    title={t('app.step_up.title')}
    isOpen
    dismissLabel={t('app.step_up.cancel')}
    onClose={() => stepUp.cancel()}
  >
    <Stack gap="200">
      <p class="quiet">{t('app.step_up.why')}</p>

      {#if stepUp.failure}
        <!-- The server's own code. Which of the two was wrong is not something this client says. -->
        <Banner tone="danger">{t(stepUp.failure)}</Banner>
      {/if}

      <form id="step-up" onsubmit={prove}>
        {#if wantsCode}
          <Input
            label={t('app.step_up.code_label')}
            hint={t('app.step_up.code_hint')}
            bind:value={code}
            autocomplete="one-time-code"
            inputmode="numeric"
            spellcheck={false}
            isRequired
          />
        {:else}
          <Input
            label={t('app.step_up.password_label')}
            bind:value={password}
            type="password"
            autocomplete="current-password"
            spellcheck={false}
            isRequired
          />
        {/if}
      </form>
    </Stack>

    {#snippet actions()}
      <Button tone="subtle" onclick={() => stepUp.cancel()}>{t('app.step_up.cancel')}</Button>
      <Button
        type="submit"
        form="step-up"
        tone="primary"
        isBusy={stepUp.isWorking}
        busyLabel={t('app.step_up.working')}
      >
        {t('app.step_up.submit')}
      </Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  form { margin: 0; }
</style>
