<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Setting up a second factor, from two places: a person doing it from their profile, and
  // a person the workspace's rule routed into it instead of into a session.
  //
  // **One walk for both places, in the order UC-ID-03 tells it**: the secret, one code to confirm
  // it, then the ten recovery codes once, and *Continue*. Two places that differed - a plain input
  // and a plain list in one, the one-time panel in the other - would give a person two answers to
  // one question.
  //
  // **The QR code is drawn by the design system, and it is an aid rather than the setup.**
  // [ADR-0053](../../../../docs/adr/ADR-0053-totp-qr-code.md) chose an encoder over a dependency;
  // the image stands beside the typed setup and does not replace it: the base32 secret in
  // groups of four, which every authenticator accepts typed in, and the `otpauth://` URI as a
  // link, which is the best answer of all on the device that holds the authenticator. A URI the
  // encoder cannot draw - past its thirteen versions, which takes a very long issuer and address
  // together - is not an error here: the image is left out and the two ways in remain.
  //
  // **The confirmation is the sign-in's own code field** (`CodeField`), so the field a person
  // learns here is the field they meet at every sign-in after it.
  //
  // **The codes come after the confirmation, in the one-time panel.** No call answers them again,
  // so they are held here from the moment the setup answered them until *Continue*, and dropped
  // with the panel. *Continue* stays unavailable until the reader ticks that they stored them - ten
  // codes scrolled past are ten codes nobody wrote down, and they are the way back when the phone
  // is gone. During a sign-in *Continue* is also what opens the session: the pair the confirmation
  // answered waits for it, so nobody lands in the product with their codes still unread.
  //
  // **Setting up arms nothing until the code confirms it.** A reader who closes the tab before that
  // is exactly where they started.

  import { canCopy } from '@hubtask/design-system/components';
  import { Banner, Button, CodeField, OneTimeSecret, QrCode, Stack, encodeQr } from '@hubtask/design-system/components';

  import { mfa } from '../data/mfa.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The credential a sign-in that demands a factor answered. Absent for a signed-in caller. */
    pendingToken?: string;
    /** Called at *Continue*, with the pair where the confirmation *was* the sign-in. */
    onarmed?: (pair?: { access: string; refresh: string }) => void;
  }

  let { pendingToken, onarmed }: Props = $props();

  let code = $state('');
  /** The codes and the pair between the confirmation and *Continue*. */
  let armed = $state<{ codes: readonly string[]; pair?: { access: string; refresh: string } } | undefined>(undefined);

  const enrollment = $derived(mfa.enrollment);

  // The secret in groups of four. Not decoration: thirty-two unbroken characters typed into a
  // phone is where a mistake happens, and the confirmation would then say "wrong code" about a
  // secret that was mistyped rather than a code that was.
  const grouped = $derived((enrollment?.secret ?? '').replace(/(.{4})/g, '$1 ').trim());

  // The image, where the encoder can draw one. Undefined past its tables is the one refusal it
  // has, and the screen answers it by showing the secret without a picture rather than nothing.
  const matrix = $derived.by(() => {
    if (!enrollment) return undefined;
    try {
      return encodeQr(enrollment.otpauth_uri);
    } catch {
      return undefined;
    }
  });

  $effect(() => () => mfa.forget());

  async function confirm(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    // Taken before the confirmation: the store drops the whole setup once it is confirmed, and the
    // codes are what the reader still has to see.
    const codes = enrollment?.recovery_codes ?? [];
    const answer = await mfa.confirm(code.trim(), pendingToken);
    code = '';
    if (answer?.armed) armed = { codes, pair: answer.tokens };
  }

  function proceed(): void {
    const pair = armed?.pair;
    armed = undefined;
    onarmed?.(pair);
  }
</script>

<Stack gap="200">
  {#if mfa.failure}
    <!-- The server's own code: a setup refused while one is already armed says so, and a wrong code
         is the sign-in's own sentence. -->
    <Banner tone="danger">{t(mfa.failure)}</Banner>
  {/if}

  {#if armed}
    <OneTimeSecret
      value={armed.codes.join('\n')}
      label={t('app.recovery.title')}
      hint={t('app.mfa.recovery_hint')}
      revealLabel={t('app.recovery.reveal')}
      hideLabel={t('app.recovery.hide')}
      copyLabel={canCopy(navigator.clipboard) ? t('app.recovery.copy') : undefined}
      copiedLabel={t('app.recovery.copied')}
      acknowledgementLabel={t('app.recovery.acknowledge')}
      notAcknowledgedReason={t('app.recovery.not_acknowledged')}
      dismissLabel={t('app.mfa.continue')}
      onDismiss={proceed}
    />
  {:else if !enrollment}
    <p class="quiet">{t('app.mfa.intro')}</p>
    <div>
      <Button
        tone="primary"
        isBusy={mfa.isWorking}
        busyLabel={t('app.mfa.starting')}
        onclick={() => void mfa.start(pendingToken)}
      >
        {t('app.mfa.start')}
      </Button>
    </div>
  {:else}
    <Stack gap="150">
      <h3 class="section">{t('app.mfa.secret_title')}</h3>
      <p class="quiet">{t('app.mfa.secret_hint')}</p>
      {#if matrix}
        <QrCode {matrix} label={t('app.mfa.qr_label')} />
      {/if}
      <!-- Selectable text rather than a field: it is read and copied, never edited. -->
      <p class="secret">{grouped}</p>
      <p>
        <!-- The best answer on a phone, where a QR on the same screen cannot be photographed by
             the camera above it. It does nothing on a desktop with no handler, which is why it is
             an addition rather than the only way in. -->
        <a href={enrollment.otpauth_uri}>{t('app.mfa.open_in_app')}</a>
      </p>
    </Stack>

    <form onsubmit={confirm}>
      <Stack gap="200">
        <CodeField
          label={t('app.mfa.code_label')}
          hint={t('app.mfa.code_hint')}
          bind:value={code}
          isRequired
        />
        <div>
          <Button
            type="submit"
            tone="primary"
            isBusy={mfa.isWorking}
            busyLabel={t('app.mfa.confirming')}
          >
            {t('app.mfa.confirm')}
          </Button>
        </div>
      </Stack>
    </form>
  {/if}
</Stack>

<style>
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  /* Monospace: a secret is read a character at a time, and l/1 and O/0 are what goes wrong when
     it is not. The grouping does the rest — there is no tracking token, and inventing one for a
     single paragraph would be a value added to `tokens.json` for one caller. */
  .secret {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-300);
    overflow-wrap: anywhere;
  }

  form { margin: 0; }
</style>
