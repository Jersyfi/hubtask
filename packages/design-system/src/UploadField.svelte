<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Choosing a file, and watching it go.
  //
  // **It moves no bytes.** The upload is three steps (arc42 §8.4) — stage, put the bytes where the
  // ticket says, confirm — and all three are the caller's. This hands over a `File` and renders
  // the progress it is told about. A component that uploaded would be a component with a request
  // in it, in the package that must contain none.
  //
  // **The native input stays in the accessibility tree.** It is transparent and on top, never
  // `display: none` — hiding it takes the keyboard and the screen reader with it, and
  // `test/conventions.test.js` fails on the other spelling. The drop target is an *addition*: a
  // person who cannot drag can always press.
  //
  // **The size limit is handed in and only announced.** What an installation accepts is its own
  // answer, and a component that refused a file locally would refuse one the server would have
  // taken. So the declared size is shown against the limit, and a file over it is reported rather
  // than swallowed — the reader learns before the upload rather than after it.

  import Button from './Button.svelte';
  import type { Disableable } from './control.ts';

  interface Props extends Disableable {
    /** What the field is called. Never hidden: a file input with no name announces as "button". */
    label: string;
    /** Standing help under the label — what may be uploaded, in the caller's words. */
    hint?: string;
    /** What the button that opens the file dialog says. */
    chooseLabel: string;
    /** What the drop target says. An addition to the button, never the only way in. */
    dropLabel: string;
    /** What the control that abandons an upload in flight says. */
    cancelLabel: string;
    /** The file the caller currently holds, already chosen. */
    file?: { readonly name: string; readonly size: number } | null;
    /**
     * The declared size against the limit, as one resolved sentence — "2.4 MB of 25 MB". Formed by
     * the caller, because a file size is a number with a unit and a unit is a locale
     * (`i18n-l10n.md` §2).
     */
    sizeLabel?: string;
    /** Said when the declared size is over the limit. Present means over; absent means within. */
    overLimitLabel?: string;
    /** How far the bytes have got, 0 to 100. Absent means nothing is in flight. */
    progress?: number;
    /** The progress as a sentence, for a reader who is not watching a bar — "40% uploaded". */
    progressLabel?: string;
    /** The types the dialog offers first. A hint to the platform, never a check (see above). */
    accept?: string;
    onChoose?: (file: File) => void;
    onCancel?: () => void;
  }

  const {
    label,
    hint,
    chooseLabel,
    dropLabel,
    cancelLabel,
    file = null,
    sizeLabel,
    overLimitLabel,
    progress,
    progressLabel,
    accept,
    disabledReason,
    onChoose,
    onCancel,
  }: Props = $props();

  let input = $state<HTMLInputElement | null>(null);
  let isOver = $state(false);

  const unavailable = $derived(disabledReason !== undefined);
  const reasonId = $derived(unavailable ? `reason-${Math.random().toString(36).slice(2, 9)}` : undefined);
  const hintId = $derived(hint ? `hint-${Math.random().toString(36).slice(2, 9)}` : undefined);
  const isUploading = $derived(progress !== undefined);

  function take(files: FileList | null | undefined) {
    const chosen = files?.[0];
    if (!chosen || unavailable) return;
    onChoose?.(chosen);
  }
</script>

<div class="field">
  <label class="label" for={input?.id}>{label}</label>
  {#if hint}<p class="hint" id={hintId}>{hint}</p>{/if}

  <!-- The drop target wraps the button and the input rather than replacing them. `dragover` has
       to be cancelled or the browser opens the file instead, which is the one line every drop
       target gets wrong. -->
  <div
    class="target"
    data-over={isOver ? '' : undefined}
    ondragover={(event) => {
      event.preventDefault();
      isOver = true;
    }}
    ondragleave={() => {
      isOver = false;
    }}
    ondrop={(event) => {
      event.preventDefault();
      isOver = false;
      take(event.dataTransfer?.files);
    }}
    role="presentation"
  >
    <input
      class="native"
      type="file"
      {accept}
      bind:this={input}
      disabled={unavailable}
      aria-describedby={[hintId, reasonId].filter(Boolean).join(' ') || undefined}
      onchange={(event) => take((event.currentTarget as HTMLInputElement).files)}
    />
    <span class="painted">
      <Button tone="secondary" disabledReason={disabledReason} onclick={() => input?.click()}>
        {chooseLabel}
      </Button>
      <span class="drop">{dropLabel}</span>
    </span>
  </div>

  {#if file}
    <p class="chosen">
      <span class="name">{file.name}</span>
      {#if sizeLabel}<span class="size">{sizeLabel}</span>{/if}
    </p>
    {#if overLimitLabel}
      <!-- Rule 3: a sentence, not a red border. And it is a report rather than a refusal — what
           the installation accepts is the installation's answer. -->
      <p class="over" role="alert">{overLimitLabel}</p>
    {/if}
  {/if}

  {#if isUploading}
    <p class="progress">
      <progress max="100" value={progress}></progress>
      <!-- Polite: the reader started this and is not waiting on it the way they wait on a
           failure, and an assertive region would interrupt them at every percent. -->
      <span aria-live="polite">{progressLabel}</span>
    </p>
    <Button tone="subtle" onclick={() => onCancel?.()}>{cancelLabel}</Button>
  {/if}

  {#if unavailable}<p class="reason" id={reasonId}>{disabledReason}</p>{/if}
</div>

<style>
  .field { display: flex; flex-direction: column; gap: var(--sp-050); min-width: 0; }

  .label { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .target {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    border: var(--bw-thick) dashed var(--border-subtle);
    border-radius: var(--r-md);
    padding: var(--sp-150);
    transition: background-color var(--motion-state-duration) var(--motion-state-easing),
      border-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .target[data-over] { background: var(--bg-surface-raised); border-color: var(--accent-primary); }

  /* Transparent and on top, never hidden: the input is the control and everything beside it is a
     picture of it. It covers the whole target so that a click anywhere opens the dialog. */
  .native {
    position: absolute;
    inset: 0;
    inline-size: 100%;
    block-size: 100%;
    opacity: 0;
    cursor: pointer;
  }

  .native:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  /* The painting takes no click meant for the input underneath - except the button, which is a
     real control with its own keyboard behaviour. */
  .painted { display: flex; align-items: center; gap: var(--sp-100); pointer-events: none; }

  .painted :global(button) { pointer-events: auto; }

  .drop { color: var(--text-subtle); font-size: var(--fs-075); }

  .chosen { display: flex; flex-wrap: wrap; gap: var(--sp-100); margin: 0; min-width: 0; }

  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .size,
  .hint,
  .reason { color: var(--text-subtle); font-size: var(--fs-075); }

  .hint,
  .reason { margin: 0; }

  .over { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

  .progress { display: flex; align-items: center; gap: var(--sp-100); margin: 0; }

  @media (prefers-reduced-motion: reduce) {
    .target { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .target { transition-duration: var(--dur-instant); }
</style>
