<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A list that could not be read, which `voice-and-tone.md` §4.4 says is **not** an empty state:
  // "a failure rendered as 'no results' is a lie the reader acts on". Its own component, so the
  // rule is structural rather than something a review has to notice.
  //
  // Three things it owes, and they are §3's rather than this component's invention. The message
  // **names the fix** where there is one (§3.1) and stops where there is none (§3.2) - which is the
  // caller's text, because the code and its params are the server's. The retry is offered where
  // the failure is retryable, and `TransportError.isRetryable` is what answers that. And the
  // **reference** is shown where there is one: an internal error without its `request_id` is a
  // support thread that cannot be traced (ADR-0025).
  //
  // It is announced. A list that quietly turns into a paragraph of prose is a change a screen
  // reader is never told about, so this is a live region - `assertive`, because the reader is
  // waiting for content that is not coming.

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';

  interface Props {
    /** Fault and fix, in one sentence or two short ones (§5.2). Resolved text (ADR-0011). */
    title: string;
    description?: string;
    /**
     * The server's correlation id, where there was one. Shown so it can be copied - never
     * explained, and never the whole of what the reader is told.
     */
    reference?: string;
    /** What the reference is called. Required whenever one is shown. */
    referenceLabel?: string;
    /** The name of the retry control. Absent means the failure is not retryable (§3.2). */
    retryLabel?: string;
    onRetry?: () => void;
  }

  const { title, description, reference, referenceLabel, retryLabel, onRetry }: Props = $props();
</script>

<div class="error" role="alert">
  <span class="mark" aria-hidden="true"><Icon name="triangle-alert" /></span>
  <p class="title">{title}</p>
  {#if description}<p class="description">{description}</p>{/if}
  {#if retryLabel}
    <div class="action"><Button tone="secondary" onclick={() => onRetry?.()}>{retryLabel}</Button></div>
  {/if}
  {#if reference && referenceLabel}
    <!-- Selectable, and in a monospace face: it exists to be copied into a support request, and a
         proportional font is where a `1` and an `l` stop being distinguishable. -->
    <p class="reference">{referenceLabel} <code>{reference}</code></p>
  {/if}
</div>

<style>
  .error {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--sp-150);
    padding: var(--sp-600) var(--sp-300);
    text-align: center;
  }

  /* Rule 3: the mark carries the tone as well as the colour, so it reads in greyscale. */
  .mark { display: inline-flex; color: var(--text-danger); }

  .title {
    margin: 0;
    max-width: 48ch;
    color: var(--text-primary);
    font-size: var(--fs-200);
    font-weight: var(--fw-medium);
  }

  .description {
    margin: 0;
    max-width: 56ch;
    color: var(--text-secondary);
    font-size: var(--fs-100);
  }

  .action { margin-top: var(--sp-100); }

  .reference {
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .reference code {
    font-family: var(--font-mono);
    user-select: all;
  }
</style>
