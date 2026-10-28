<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The prompt a `403 auth.step_up_required` produces.
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
  // page: the person signs in there once more and comes back to where they were. A passkey or
  // security key is a press too: the device answers, and nothing is typed.

  import { Banner, Button, CodeField, Dialog, Input, Stack } from '@hubtask/design-system/components';

  import { canCreatePasskeys } from '../data/passkeys.ts';
  import { stepUp } from '../data/stepup.svelte.ts';
  import type { StepUpMethod } from '../data/stepup.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  /** The ways a field can answer, in the order one is asked for first. */
  type Typed = Exclude<StepUpMethod, 'PROVIDER' | 'PASSKEY'>;
  const ORDER: readonly Typed[] = ['TOTP', 'PASSWORD', 'RECOVERY'];

  let password = $state('');
  let code = $state('');
  let recoveryCode = $state('');
  /** The way the reader switched to; until they do, the first typed way the account holds. */
  let switched = $state<Typed | undefined>(undefined);

  const pending = $derived(stepUp.pending);
  const typed = $derived(ORDER.filter((method) => pending?.methods.includes(method)));
  // Derived rather than set in an effect: the field has to be in the dialog when it opens, or the
  // dialog's own first focus lands on *Not now* before an effect could draw it.
  const chosen = $derived(switched ?? typed[0]);
  const offersProvider = $derived(pending?.methods.includes('PROVIDER') ?? false);
  /** A key, where the account holds one and this browser can ask a device. */
  const offersKey = $derived((pending?.methods.includes('PASSKEY') ?? false) && canCreatePasskeys(globalThis as never));
  let keyWaiting = $state(false);
  let keyBox = $state<HTMLElement | undefined>(undefined);

  async function useKey(): Promise<void> {
    keyWaiting = true;
    const ended = await stepUp.proveWithKey((options) => navigator.credentials.get(options));
    keyWaiting = false;
    // Closed by the person: nothing said, the focus where it was (P-11).
    if (ended === 'cancelled') keyBox?.querySelector('button')?.focus();
  }
  /** The provider's name, from the refusal - or a plain word where an older server named none. */
  const provider = $derived(pending?.provider ?? t('app.step_up.your_provider'));
  /**
   * Nothing this account can prove itself with: no password, no factor, no provider switched on in
   * its workspace. No field is drawn - one would ask for something the account does not have
   * (UC-ID-05 check 5) - and the sentence says who can help.
   */
  const cannotProve = $derived(
    pending !== undefined && typed.length === 0 && !offersProvider && !offersKey,
  );

  // Cleared whenever a new prompt opens, so a value typed for the last privileged action is not
  // sitting in the field for the next one; and the first typed way the account holds is the one
  // asked for again.
  $effect(() => {
    if (pending) {
      password = '';
      code = '';
      recoveryCode = '';
      switched = undefined;
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
        <!-- The server's own sentence. Which of the two was wrong is not something this client says. -->
        <Banner tone="danger" title={stepUp.failure.message}>
          {#if stepUp.failure.reference}{stepUp.failure.reference}{/if}
        </Banner>
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
        {:else if !offersKey}
          <!-- The provider is the account's only way: say where the person is about to go. -->
          <p>{t('app.step_up.provider_leaving', { provider })}</p>
        {/if}

        {#if offersKey}
          <div class="key" bind:this={keyBox}>
            <Button
              tone={chosen ? 'subtle' : 'primary'}
              isBusy={keyWaiting}
              busyLabel={t('app.sign_in.key_waiting')}
              onclick={() => void useKey()}
            >
              {t('app.step_up.with_key')}
            </Button>
            <p class="quiet" role="status">{keyWaiting ? t('app.sign_in.key_waiting_said') : ''}</p>
          </div>
        {/if}

        {#if typed.length > 1 || (offersProvider && (chosen || offersKey))}
          <div class="others">
            {#each typed.filter((method) => method !== chosen) as other (other)}
              <Button tone="subtle" onclick={() => (switched = other)}>{t(switchLabel[other])}</Button>
            {/each}
            {#if offersProvider && (chosen || offersKey)}
              <Button tone="subtle" isBusy={stepUp.isWorking} onclick={() => stepUp.confirmAtProvider()}>
                {t('app.step_up.with_provider', { provider })}
              </Button>
            {/if}
          </div>
        {/if}
      {/if}
      {#if pending?.purpose === 'change_address'}
        <!-- Why no passkey is offered: it belongs to the address the change ends (multi-tenancy.md §3.1). -->
        <p class="note">{t('app.step_up.change_address_note')}</p>
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
      {:else if !cannotProve && !offersKey}
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
  .note { margin: 0; font-size: var(--fs-075); color: var(--text-subtle); }

  form { margin: 0; }

  .key { display: grid; gap: var(--sp-050); }

  .others {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-100);
  }
</style>
