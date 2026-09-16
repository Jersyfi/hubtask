<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Code, shown as code: a `<pre>` that scrolls inside its own box, in the mono face, with the
  // language named beside it and, where the caller asks for one, a control that copies it.
  //
  // No highlighter. Colouring tokens needs a grammar per language, and every grammar in
  // circulation is a dependency - a supply chain decision (CLAUDE.md) for a brochure that ships
  // no script at all. Monospace, wrapped nowhere, scrolled where it overflows: a reader copies a
  // `curl` line by selecting it, and colour would not change what they get.
  //
  // The copy control is the caller's choice, as `OneTimeSecret`'s is: a page that renders without
  // JavaScript (the website, ADR-0030) passes no `copyLabel` and gets no button that could not
  // work. Where it is asked for, it is offered only where `navigator.clipboard` exists.

  import Button from './Button.svelte';
  import { canCopy } from './secret.ts';

  interface Props {
    code: string;
    /** What the block is, for the accessibility tree: "Request", "Example response". */
    label: string;
    /** The language, drawn as a small label. `bash`, `json`, `http`. Absent means unlabelled. */
    language?: string;
    /** Present means a copy control is offered where the browser can copy. */
    copyLabel?: string;
    /** Announced after a copy. Says that it was copied, never what was copied. */
    copiedLabel?: string;
  }

  const { code, label, language, copyLabel, copiedLabel }: Props = $props();

  let hasCopied = $state(false);
  const hasClipboard = canCopy(globalThis.navigator?.clipboard);
  const isCopyable = $derived(copyLabel !== undefined && hasClipboard);

  async function copy(): Promise<void> {
    try {
      await globalThis.navigator.clipboard.writeText(code);
      hasCopied = true;
    } catch {
      hasCopied = false;
    }
  }
</script>

<figure class="block">
  <figcaption class="head">
    <span class="label">{label}</span>
    {#if language}<span class="language">{language}</span>{/if}
    {#if isCopyable}
      <span class="copy">
        <Button tone="secondary" size="sm" icon="copy" onclick={() => void copy()}>{copyLabel}</Button>
      </span>
    {/if}
  </figcaption>
  <!-- Focusable for the same reason `Table`'s scroll region is: a wide line is otherwise out of
       reach without a pointer (WCAG 2.1.1). The region carries the figure's name. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <pre class="code" tabindex="0" role="region" aria-label={label}><code>{code}</code></pre>
  <span class="announce" role="status" aria-live="polite">{hasCopied && copiedLabel ? copiedLabel : ''}</span>
</figure>

<style>
  .block {
    margin: 0;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-primary);
    /* Rule 4: as wide as it is given; the line scrolls, the block does not grow. */
    max-inline-size: 100%;
  }

  .head {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-050) var(--sp-150);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }

  .label { font-weight: var(--fw-medium); }
  .language { font-family: var(--font-mono); color: var(--text-subtle); }
  .copy { margin-inline-start: auto; }

  .code {
    margin: 0;
    padding: var(--sp-150);
    overflow-x: auto;
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    line-height: var(--lh-normal);
    /* A `<pre>` keeps the author's line breaks; `text-align: start` keeps a code sample from
       being mirrored under `dir="rtl"`, because code runs one way whatever the page does. */
    direction: ltr;
    text-align: start;
    tab-size: 2;
  }

  .code:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: calc(var(--sp-025) * -1);
  }

  /* Announced, never drawn (the `Table` technique). */
  .announce {
    position: absolute;
    inline-size: var(--sp-025);
    block-size: var(--sp-025);
    margin: calc(var(--sp-025) * -1);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
