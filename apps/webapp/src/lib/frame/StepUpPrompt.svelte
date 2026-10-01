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
  // **Exactly the account's ways, never a guess** (ADR-0075). The refusal names them: the
  // authenticator's code where a factor is armed, the password where the account holds one, a
  // recovery code while one is left, and the provider it is connected to. One is asked for at a
  // time — the code first, then the password, then a recovery code — and every other way the
  // account holds is offered beneath it as one press. The provider is the one way that leaves the
  // page: the person signs in there once more and comes back to where they were.

  import { Banner, Button, CodeField, Dialog, Input, Stack } from '@hubtask/design-system/components';

  import { stepUp } from '../data/stepup.svelte.ts';
  import type { StepUpMethod } from '../data/stepup.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  /** The ways a field can answer, in the order one is asked for first. */
  type Typed = Exclude<StepUpMethod, 'PROVIDER'>;
  const ORDER: readonly Typed[] = ['TOTP', 'PASSWORD', 'RECOVERY'];

  let password = $state('');
  let code = $state('');
  let recoveryCode = $state('');
  let chosen = $state<Typed | undefined>(undefined);

  const pending = $derived(stepUp.pending);
  const typed = $derived(ORDER.filter((method) => pending?.methods.includes(method)));
  const offersProvider = $derived(pending?.methods.includes('PROVIDER') ?? false);
  /** The provider's name, from the refusal - or a plain word where an older server named none. */
  const provider = $derived(pending?.provider ?? t('app.step_up.your_provider'));
  /**
   * Nothing this account can prove itself with: no password, no factor, no provider switched on in
   * its workspace. No field is drawn - one would ask for something the account does not have
   * (UC-ID-05 check 5) - and the sentence says who can help.
   */
  const cannotProve = $derived(pending !== undefined && pending.methods.length === 0);

  // Cleared whenever a new prompt opens, so a value typed for the last privileged action is not
  // sitting in the field for the next one; and the first typed way the account holds is the one
  // asked for.
  $effect(() => {
    if (pending) {
      password = '';
      code = '';
      recoveryCode = '';
      chosen = ORDER.find((method) => pending.methods.includes(method));
    }
  });

  async function prove(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (chosen === 'TOTP') await stepUp.prove({ code: code.trim() });
    else if (chosen === 'RECOVERY') await stepUp.prove({ recoveryCode: recoveryCode.trim() });
    else await stepUp.prove({ password });
    // Out of the component's state whatever happened: a proof kept for a retry is a credential
    // sitting in memory after the moment that needed it.
    password = '';
    code = '';
    recoveryCode = '';
  }

  /** The press that switches to another typed way. */
  const switchLabel: Record<Typed, string> = {
    TOTP: 'app.step_up.with_code',
    PASSWORD: 'app.step_up.with_password',
    RECOVERY: 'app.step_up.with_recovery',
  };
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

      {#if cannotProve}
        <p>{t('app.step_up.no_method')}</p>
      {:else}
        {#if chosen}
          <form id="step-up" onsubmit={prove}>
            {#if chosen === 'TOTP'}
              <!-- The same field the sign-in's second step draws, because it is the same six digits
                   from the same authenticator. A modal focuses its first control by itself, so
                   there is nothing to ask for here. -->
              <CodeField
                label={t('app.step_up.code_label')}
                hint={t('app.step_up.code_hint')}
                bind:value={code}
                isRequired
              />
            {:else if chosen === 'RECOVERY'}
              <Input
                label={t('app.step_up.recovery_label')}
                hint={t('app.step_up.recovery_hint')}
                bind:value={recoveryCode}
                autocomplete="off"
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
        {:else}
          <!-- The provider is the account's only way: say where the person is about to go. -->
          <p>{t('app.step_up.provider_leaving', { provider })}</p>
        {/if}

        {#if typed.length > 1 || (offersProvider && chosen)}
          <div class="others">
            {#each typed.filter((method) => method !== chosen) as other (other)}
              <Button tone="subtle" onclick={() => (chosen = other)}>{t(switchLabel[other])}</Button>
            {/each}
            {#if offersProvider && chosen}
              <Button tone="subtle" isBusy={stepUp.isWorking} onclick={() => stepUp.confirmAtProvider()}>
                {t('app.step_up.with_provider', { provider })}
              </Button>
            {/if}
          </div>
        {/if}
      {/if}
    </Stack>

    {#snippet actions()}
      <Button tone="subtle" onclick={() => stepUp.cancel()}>{t('app.step_up.cancel')}</Button>
      {#if !cannotProve && chosen}
        <Button
          type="submit"
          form="step-up"
          tone="primary"
          isBusy={stepUp.isWorking}
          busyLabel={t('app.step_up.working')}
        >
          {t('app.step_up.submit')}
        </Button>
      {:else if !cannotProve}
        <Button
          tone="primary"
          isBusy={stepUp.isWorking}
          busyLabel={t('app.step_up.working')}
          onclick={() => stepUp.confirmAtProvider()}
        >
          {t('app.step_up.with_provider', { provider })}
        </Button>
      {/if}
    {/snippet}
  </Dialog>
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  form { margin: 0; }

  .others {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-100);
  }
</style>
