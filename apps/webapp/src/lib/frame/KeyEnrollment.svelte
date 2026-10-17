<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Setting up a passkey or security key during sign-in, beside the app (UC-ID-22 check 8): the
  // workspace demands a second factor of this person, who holds none, and the server says a key can
  // be made at this host. A name, the device, and - where the account held no recovery code - the
  // ten codes once, with *Continue* opening the session only after they were stored, as the app's
  // setup does (`TotpEnrollment.svelte`).

  import { canCopy } from '@hubtask/design-system/components';
  import { Banner, Button, Input, OneTimeSecret, Stack } from '@hubtask/design-system/components';

  import type { OwedChange } from '../data/mfa.svelte.ts';
  import { passkeys } from '../data/passkeys.svelte.ts';
  import { nameFits, suggestedName } from '../data/passkeys.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The ENROLL step's credential. */
    pendingToken?: string;
    /** Called once the key is kept and the codes, if any, are stored: the pair, or the step owed. */
    onarmed?: (pair?: { access: string; refresh: string }, owedChange?: OwedChange) => void;
  }

  let { pendingToken, onarmed }: Props = $props();

  let name = $state(suggestedName(t('app.passkeys.default_name'), []));
  let waiting = $state(false);
  let button = $state<HTMLElement | undefined>(undefined);
  /** The codes and what follows them, between the key being kept and *Continue*. */
  let kept = $state<
    { codes: readonly string[]; pair?: { access: string; refresh: string }; owedChange?: OwedChange } | undefined
  >(undefined);

  $effect(() => () => passkeys.forget());

  function finish(pair?: { access: string; refresh: string }, owedChange?: OwedChange): void {
    onarmed?.(pair, owedChange);
  }

  async function add(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!pendingToken || !nameFits(name)) return;
    waiting = true;
    const result = await passkeys.enroll(pendingToken, name, (options) => navigator.credentials.create(options));
    waiting = false;
    if (result.end === 'cancelled') {
      // The person closed the prompt: nothing said, the focus where they left it (P-11).
      button?.querySelector('button')?.focus();
      return;
    }
    const answer = result.answer;
    if (result.end !== 'added' || !answer) return;
    const pair = answer.tokens
      ? { access: answer.tokens.access_token, refresh: answer.tokens.refresh_token }
      : undefined;
    const owedChange = (answer.challenge ?? undefined) as OwedChange | undefined;
    if (answer.recovery_codes && answer.recovery_codes.length > 0) {
      kept = { codes: answer.recovery_codes, pair, owedChange };
      return;
    }
    finish(pair, owedChange);
  }
</script>

{#if kept}
  <Stack gap="200">
    <OneTimeSecret
      value={kept.codes.join('\n')}
      label={t('app.recovery.title')}
      hint={t('app.passkeys.recovery_hint')}
      revealLabel={t('app.recovery.reveal')}
      hideLabel={t('app.recovery.hide')}
      copyLabel={canCopy(navigator.clipboard) ? t('app.recovery.copy') : undefined}
      copiedLabel={t('app.recovery.copied')}
      acknowledgementLabel={t('app.recovery.acknowledge')}
      notAcknowledgedReason={t('app.recovery.not_acknowledged')}
      dismissLabel={t('app.mfa.key_done')}
      onDismiss={() => finish(kept?.pair, kept?.owedChange)}
    />
  </Stack>
{:else}
  <form onsubmit={add}>
    <Stack gap="150">
      <p class="quiet">{t('app.mfa.key_setup_hint')}</p>
      {#if passkeys.failure}
        <Banner tone="danger" title={passkeys.failure.message}>
          {#if passkeys.failure.reference}{passkeys.failure.reference}{/if}
        </Banner>
      {/if}
      <Input
        label={t('app.passkeys.name_label')}
        bind:value={name}
        maxlength={64}
        isRequired
        autocomplete="off"
      />
      <div class="key" bind:this={button}>
        <Button
          type="submit"
          tone="primary"
          isBusy={waiting || passkeys.isWorking}
          busyLabel={t('app.sign_in.passkey_waiting')}
          disabledReason={nameFits(name) ? undefined : t('app.passkeys.name_needed')}
        >
          {t('app.mfa.key_setup')}
        </Button>
        <p class="quiet" role="status">{waiting ? t('app.sign_in.passkey_waiting_said') : ''}</p>
      </div>
    </Stack>
  </form>
{/if}

<style>
  form { margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .key { display: grid; gap: var(--sp-050); }
</style>
