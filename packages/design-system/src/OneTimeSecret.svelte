<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A value shown for the only time.
  //
  // Five of them exist in this product — a minted token, a webhook signing secret, a TOTP secret
  // with its recovery codes, an inbound trigger address, a jumble intake address — and they share
  // one problem: there is no call that answers them a second time, so a reader who closes the panel
  // without copying has lost something they cannot ask for again.
  //
  // **Hidden until asked for.** A secret drawn the moment a panel opens is a secret in whatever
  // screenshot, screen share or shoulder happens to be pointed at it. The mask says nothing about
  // the value, its length included (`secret.ts`).
  //
  // **Copy is offered where it works and left out where it does not.** `navigator.clipboard` is
  // absent on an insecure origin and in some browsers, and that is not an error: the value is on
  // screen and can be selected. A control that failed at the press would be worse than one that
  // was never there. What is announced afterwards is that it was copied — never the value, which
  // would put the secret into a live region for a screen reader to read out loud.
  //
  // **The acknowledgement is the caller's decision.** Ten recovery codes scrolled past are ten
  // codes nobody wrote down, so a caller may require a tick before the value can be dismissed. The
  // dismissal is then switched off *with its reason* rather than hidden, which is this package's
  // rule everywhere: `disabledReason` is what disables.
  //
  // **Nothing here outlives the panel.** The value is a prop and belongs to the caller; this
  // component copies it into no storage, no history entry and no module-level variable, and drops
  // the fact that it was revealed when it goes. `test/secret.test.js` is what keeps that true.

  import Button from './Button.svelte';
  import Checkbox from './Checkbox.svelte';
  import Stack from './Stack.svelte';
  import { canCopy, mask, mayDismiss } from './secret.ts';

  interface Props {
    /** The value itself, from the one answer that carried it. */
    value: string;
    /** What this value is. Resolved text (ADR-0011). */
    label: string;
    /** Why it is shown once, and what to do about that. */
    hint?: string;
    revealLabel: string;
    hideLabel: string;
    /** Absent means this browser is not asked to copy — the caller decides, per `canCopy`. */
    copyLabel?: string;
    /** Announced after a copy. Says that it was copied, never what was copied. */
    copiedLabel?: string;
    /** Present means the reader must tick it before the value may be dismissed. */
    acknowledgementLabel?: string;
    /** Why the dismissal is switched off until they do. Required with an acknowledgement. */
    notAcknowledgedReason?: string;
    /** Absent means this panel is not dismissible from inside it. */
    dismissLabel?: string;
    onDismiss?: () => void;
  }

  const {
    value,
    label,
    hint,
    revealLabel,
    hideLabel,
    copyLabel,
    copiedLabel,
    acknowledgementLabel,
    notAcknowledgedReason,
    dismissLabel,
    onDismiss,
  }: Props = $props();

  let isRevealed = $state(false);
  let hasAcknowledged = $state(false);
  let hasCopied = $state(false);

  // The browser capability is asked once, at construction: whether a clipboard exists does not
  // change while a panel is open, and re-reading it on every keystroke would be treating a browser
  // capability as state. Whether the caller wants the control is a prop, so that half derives.
  const hasClipboard = canCopy(globalThis.navigator?.clipboard);
  const isCopyable = $derived(copyLabel !== undefined && hasClipboard);

  const shown = $derived(isRevealed ? value : mask(value));
  const dismissible = $derived(
    mayDismiss({ isRequired: acknowledgementLabel !== undefined, hasAcknowledged }),
  );

  // Dropped when the panel goes, so that a panel reopened for a different secret does not open
  // already showing it.
  $effect(() => () => {
    isRevealed = false;
    hasAcknowledged = false;
    hasCopied = false;
  });

  async function copy(): Promise<void> {
    try {
      await globalThis.navigator.clipboard.writeText(value);
      hasCopied = true;
    } catch {
      // Refused by the browser — a permission, a document that is not focused. The value is on
      // screen and can be selected, so there is nothing to report that the reader cannot see.
      hasCopied = false;
    }
  }
</script>

<Stack gap="150">
  <span class="label">{label}</span>
  {#if hint}<p class="hint">{hint}</p>{/if}

  <!-- The value in monospace and selectable: a secret is read a character at a time, and l/1 and
       O/0 are what goes wrong when it is not. Never an input — there is nothing here to edit, and
       a field invites a browser to offer to save it. -->
  <p class="value" data-state={isRevealed ? 'revealed' : 'hidden'}>{shown}</p>

  <div class="controls">
    <Button
      tone="secondary"
      icon={isRevealed ? 'eye-off' : 'eye'}
      onclick={() => (isRevealed = !isRevealed)}
    >
      {isRevealed ? hideLabel : revealLabel}
    </Button>

    {#if isCopyable}
      <Button tone="secondary" icon="copy" onclick={() => void copy()}>{copyLabel}</Button>
    {/if}
  </div>

  <!-- Polite and outside the controls: it says that a copy happened, and it is the one thing on
       this panel that must never contain the value. -->
  <span class="announcement" aria-live="polite">{hasCopied ? (copiedLabel ?? '') : ''}</span>

  {#if acknowledgementLabel}
    <Checkbox label={acknowledgementLabel} bind:checked={hasAcknowledged} />
  {/if}

  {#if dismissLabel}
    <div>
      <Button
        tone="primary"
        disabledReason={dismissible ? undefined : notAcknowledgedReason}
        onclick={() => onDismiss?.()}
      >
        {dismissLabel}
      </Button>
    </div>
  {/if}
</Stack>

<style>
  .label { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .hint { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  .value {
    margin: 0;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-primary);
    font-family: var(--font-mono);
    overflow-wrap: anywhere;
  }

  /* Hidden and revealed are the same box: a mask of a fixed length that then grew to the value's
     would move everything under it, which rule 6 forbids for a reason a reader feels. */
  .value[data-state='hidden'] { color: var(--text-subtle); }

  .controls { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .announcement { color: var(--text-success); font-size: var(--fs-075); min-block-size: var(--sp-200); }
</style>
