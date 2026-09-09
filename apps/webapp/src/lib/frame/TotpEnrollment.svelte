<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Enrolling a second factor (H-02), from two places: a person doing it from their profile, and
  // an administrator a tenant switch routed into it instead of into a session.
  //
  // **There is no QR code here, and that is a decision rather than an omission.**
  // [ADR-0053](../../../../docs/adr/ADR-0053-totp-qr-code.md) puts the question — a dependency, an
  // encoder, or neither — with the costs each carries, and it is the owner's. Meanwhile this shows
  // what the contract's `secret` field exists for: the base32 secret in groups of four, which
  // every authenticator accepts typed in, and the `otpauth://` URI as a link, which is the best
  // answer of all on the device that holds the authenticator. When the ADR is decided, an image
  // joins what is already here rather than replacing a screen.
  //
  // **The secret and the codes are shown once.** No call answers them again, so they are held in
  // memory by `mfa.svelte.ts` for as long as this panel lives and dropped when it leaves. The
  // reader has to confirm they have the recovery codes before the code field appears — not
  // ceremony: ten codes scrolled past are ten codes nobody wrote down, and they are the way back
  // when the phone is gone.
  //
  // **Enrolling arms nothing.** Sign-in is unchanged until a valid code confirms it, so a reader
  // who closes the tab halfway through is exactly where they started.

  import { Banner, Button, Checkbox, Input, Stack } from '@hubtask/design-system/components';

  import { mfa } from '../data/mfa.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The credential an enforcement sign-in answered. Absent for a signed-in caller. */
    pendingToken?: string;
    /** Called when the factor is armed, with the pair where the confirmation *was* the sign-in. */
    onarmed?: (pair?: { access: string; refresh: string }) => void;
  }

  let { pendingToken, onarmed }: Props = $props();

  let code = $state('');
  let keptCodes = $state(false);

  const enrollment = $derived(mfa.enrollment);

  // The secret in groups of four. Not decoration: thirty-two unbroken characters typed into a
  // phone is where a mistake happens, and the confirmation would then say "wrong code" about a
  // secret that was mistyped rather than a code that was.
  const grouped = $derived((enrollment?.secret ?? '').replace(/(.{4})/g, '$1 ').trim());

  $effect(() => () => mfa.forget());

  async function confirm(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const answer = await mfa.confirm(code.trim(), pendingToken);
    code = '';
    if (answer?.armed) onarmed?.(answer.tokens);
  }
</script>

<Stack gap="200">
  {#if mfa.failure}
    <!-- The server's own code: an enrolment refused while one is already armed says so, and a
         tenant that forbids disabling names its own switch. -->
    <Banner tone="danger">{t(mfa.failure)}</Banner>
  {/if}

  {#if !enrollment}
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
      <!-- Selectable text rather than a field: it is read and copied, never edited. -->
      <p class="secret">{grouped}</p>
      <p>
        <!-- The best answer on a phone, where a QR on the same screen cannot be photographed by
             the camera above it. It does nothing on a desktop with no handler, which is why it is
             an addition rather than the only way in. -->
        <a href={enrollment.otpauth_uri}>{t('app.mfa.open_in_app')}</a>
      </p>
    </Stack>

    <Stack gap="150">
      <h3 class="section">{t('app.mfa.recovery_title')}</h3>
      <p class="quiet">{t('app.mfa.recovery_hint')}</p>
      <ul class="codes">
        {#each enrollment.recovery_codes as recovery (recovery)}
          <li>{recovery}</li>
        {/each}
      </ul>
      <Checkbox
        label={t('app.mfa.kept_codes')}
        checked={keptCodes}
        onchange={(event: Event) => (keptCodes = (event.currentTarget as HTMLInputElement).checked)}
      />
    </Stack>

    {#if keptCodes}
      <form onsubmit={confirm}>
        <Stack gap="200">
          <Input
            label={t('app.mfa.code_label')}
            hint={t('app.mfa.code_hint')}
            bind:value={code}
            autocomplete="one-time-code"
            inputmode="numeric"
            spellcheck={false}
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

  .codes {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(12ch, 1fr));
    gap: var(--sp-100);
    font-family: var(--font-mono);
  }

  form { margin: 0; }
</style>
