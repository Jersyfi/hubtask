<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one case offline-sync.md §5 leaves to the person: a CONFLICT on `notes`. The server has
  // already decided - its version is in place, and the version this device wrote is a system
  // comment on the entry, which is what `preserved_comment_id` links to - and this dialog shows
  // both, side by side, with two ways out: keep the server's, which is dismissing; or write mine
  // again, which is an ordinary PATCH of the field from the current version and nothing else.
  // Never a merge of the two texts, and never an automatic retry: the person reads both and
  // decides, and what they decide is a write like any other (F6-06).
  //
  // Every word is resolved text. The two texts are the reader's own and are drawn as text, never
  // as markup - a note is Markdown the server does not render, and this dialog does not either.

  import Button from './Button.svelte';
  import Dialog from './Dialog.svelte';
  import Stack from './Stack.svelte';

  interface Props {
    isOpen?: boolean;
    /** "Two versions of the notes" - the dialog's title. */
    title: string;
    /** Which field, in the catalogue's words - "Notes". */
    fieldLabel: string;
    /** The server's version: what the entry says now. */
    theirs: string;
    theirsLabel: string;
    /** This device's version, which lost. */
    mine: string;
    mineLabel: string;
    /** Where the displaced version was kept, when the server said. */
    preservedHref?: string;
    preservedLabel?: string;
    /** "Keep the server's" - dismisses. */
    keepLabel: string;
    /** "Write mine again" - the PATCH the caller performs. */
    rewriteLabel: string;
    /** Why writing again is unavailable - the entry is archived, or gone. There is no boolean. */
    rewriteDisabledReason?: string;
    dismissLabel: string;
    onKeep: () => void;
    onRewrite: () => void;
  }

  let {
    isOpen = $bindable(false),
    title,
    fieldLabel,
    theirs,
    theirsLabel,
    mine,
    mineLabel,
    preservedHref,
    preservedLabel,
    keepLabel,
    rewriteLabel,
    rewriteDisabledReason,
    dismissLabel,
    onKeep,
    onRewrite,
  }: Props = $props();
</script>

<Dialog bind:isOpen {title} {dismissLabel} onClose={onKeep}>
  {#snippet actions()}
    <Button onclick={onKeep}>{keepLabel}</Button>
    <Button tone="primary" onclick={onRewrite} disabledReason={rewriteDisabledReason}>{rewriteLabel}</Button>
  {/snippet}
  <Stack gap="200">
    <p class="field">{fieldLabel}</p>
    <div class="versions">
      <section class="version" aria-label={theirsLabel}>
        <h3 class="label">{theirsLabel}</h3>
        <pre class="text">{theirs}</pre>
      </section>
      <section class="version" data-mine aria-label={mineLabel}>
        <h3 class="label">{mineLabel}</h3>
        <pre class="text">{mine}</pre>
      </section>
    </div>
    {#if preservedHref && preservedLabel}
      <a class="preserved" href={preservedHref}>{preservedLabel}</a>
    {/if}
  </Stack>
</Dialog>

<style>
  .field {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  /* Side by side where there is room, one under the other where there is not: two columns of
     prose narrower than a sentence would be harder to compare than a stack. The floor is a
     measure in characters - text's own unit, which is why it is not a token (design-system.md
     §3: tokens are colours, spaces, radii, durations). */
  .versions {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(28ch, 1fr));
    gap: var(--sp-200);
  }

  .version {
    display: grid;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    min-inline-size: 0;
  }

  /* Mine lost: rule 3 says the border carries it as well as the heading does. */
  .version[data-mine] { border-color: var(--border-strong); }

  .label {
    margin: 0;
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    color: var(--text-secondary);
  }

  .text {
    margin: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-family: var(--font-ui);
    font-size: var(--fs-100);
    color: var(--text-primary);
  }

  .preserved {
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }

  /* Rule 5: the ring, where the link takes focus. */
  .preserved:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }
</style>
