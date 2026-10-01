<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Replacing the authenticator (SC-17): a new phone, a lost app.
  //
  // **Never a moment without a factor.** The new secret waits beside the one in force; the old
  // authenticator and its recovery codes keep working until a code from the new app confirms the
  // swap, which the server makes in one statement. So this is offered wherever a factor is on - also
  // where the workspace requires one, which is exactly where turning it off and setting it up again
  // is not offered.
  //
  // **The same walk as setting one up**, in the same order: a step-up first (the dialog asks for
  // whatever the account holds), then the secret with its QR, one code from the new app, and the ten
  // new recovery codes once, behind the acknowledgement - the old ten stopped at the swap.

  import { canCopy } from '@hubtask/design-system/components';
  import { Banner, Button, CodeField, OneTimeSecret, QrCode, Stack, encodeQr } from '@hubtask/design-system/components';

  import { mfa } from '../data/mfa.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Called once the new codes have been seen: the account is read again. */
    onreplaced?: () => void;
  }

  let { onreplaced }: Props = $props();

  let code = $state('');
  let swapped = $state<readonly string[] | undefined>(undefined);

  const replacement = $derived(mfa.replacement);
  const grouped = $derived((replacement?.secret ?? '').replace(/(.{4})/g, '$1 ').trim());
  const matrix = $derived.by(() => {
    if (!replacement) return undefined;
    try {
      return encodeQr(replacement.otpauth_uri);
    } catch {
      return undefined;
    }
  });

  $effect(() => () => mfa.forget());

  async function confirm(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const codes = await mfa.confirmReplacement(code.trim());
    code = '';
    if (codes) swapped = codes;
  }

  function done(): void {
    swapped = undefined;
    onreplaced?.();
  }
</script>

<Stack gap="200">
  {#if mfa.failure}
    <Banner tone="danger">{t(mfa.failure)}</Banner>
  {/if}

  {#if swapped}
    <OneTimeSecret
      value={swapped.join('\n')}
      label={t('app.recovery.title')}
      hint={t('app.mfa.replace_codes_hint')}
      revealLabel={t('app.recovery.reveal')}
      hideLabel={t('app.recovery.hide')}
      copyLabel={canCopy(navigator.clipboard) ? t('app.recovery.copy') : undefined}
      copiedLabel={t('app.recovery.copied')}
      acknowledgementLabel={t('app.recovery.acknowledge')}
      notAcknowledgedReason={t('app.recovery.not_acknowledged')}
      dismissLabel={t('app.mfa.continue')}
      onDismiss={done}
    />
  {:else if !replacement}
    <p class="quiet">{t('app.mfa.replace_intro')}</p>
    <div>
      <Button
        tone="secondary"
        isBusy={mfa.isWorking}
        busyLabel={t('app.mfa.starting')}
        onclick={() => void mfa.replace()}
      >
        {t('app.mfa.replace_start')}
      </Button>
    </div>
  {:else}
    <Stack gap="150">
      <p class="quiet">{t('app.mfa.replace_secret_hint')}</p>
      {#if matrix}
        <QrCode {matrix} label={t('app.mfa.qr_label')} />
      {/if}
      <p class="secret">{grouped}</p>
      <p><a href={replacement.otpauth_uri}>{t('app.mfa.open_in_app')}</a></p>
    </Stack>

    <form onsubmit={confirm}>
      <Stack gap="200">
        <CodeField
          label={t('app.mfa.replace_code_label')}
          hint={t('app.mfa.code_hint')}
          bind:value={code}
          isRequired
        />
        <div>
          <Button type="submit" tone="primary" isBusy={mfa.isWorking} busyLabel={t('app.mfa.confirming')}>
            {t('app.mfa.replace_confirm')}
          </Button>
        </div>
      </Stack>
    </form>
  {/if}
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .secret {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-300);
    overflow-wrap: anywhere;
  }

  form { margin: 0; }
</style>
