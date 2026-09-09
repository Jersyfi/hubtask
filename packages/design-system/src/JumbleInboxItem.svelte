<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One arrival in the jumble inbox, and what was decided about it.
  //
  // **The subject and the body arrived from outside, and they are rendered as text.** Never as
  // markup, and there is nothing to configure about that: this component contains no `{@html}` and
  // `test/conventions.test.js` refuses one anywhere in the package. An intake address is a public
  // address — anybody who learns it can put a string in front of a reader — so the string a
  // stranger sent is drawn as characters, and a subject that is a script tag looks like a script
  // tag.
  //
  // **The sender is data, never an identity.** A `From` header authenticates nothing; the intake
  // token does. So it is shown as one more fact about the arrival, beside the channel, rather than
  // as a person with an avatar.
  //
  // **`DISMISSED` is a state, not a deletion.** The entry stays readable and ages out by retention
  // rule, so the component *says* that rather than fading the row towards invisible — a greyed-out
  // row is a client inventing a deletion the server did not perform.
  //
  // **An AI suggestion is a slot, not a feature of this component.** AI is switchable off, and
  // then this disappears without residue (§4's rule for `AISuggestion`); a card that drew one
  // itself would be a card with an opinion about a subsystem it does not own.

  import type { Snippet } from 'svelte';

  import Badge from './Badge.svelte';
  import Stack from './Stack.svelte';

  /** The three states an arrival can be in, plus room for one this build has not met. */
  export type JumbleStatus = 'NEW' | 'PROCESSED' | 'DISMISSED' | (string & {});

  interface Props {
    /**
     * The subject as it arrived. Where an arrival has none — a webhook, a quick capture — the
     * caller passes its own wording for that, because "no subject" is a sentence and sentences are
     * resolved outside this package (ADR-0011).
     */
    subject: string;
    /** As much of the body as the caller chose to show. Text, like the subject. */
    excerpt?: string;
    /** How it arrived, in words. */
    channelLabel: string;
    /** Who the transport says it came from, and the word for that. Both optional: API arrivals have neither. */
    sender?: string;
    senderLabel?: string;
    /** When it arrived, as the caller formatted it, and the machine-readable instant beside it. */
    receivedLabel: string;
    receivedAt?: string;
    status: JumbleStatus;
    /** The status in words. Falls back to the server's own token for one this build has no wording for. */
    statusLabel?: string;
    /** Where the entry a conversion produced lives. The other half of the provenance pair. */
    targetHref?: string;
    targetLabel?: string;
    /** What a dismissal means, said rather than implied by a faded row. */
    dismissedNote?: string;
    /** Room for `AISuggestion`, where AI is on and has something to propose. */
    suggestion?: Snippet;
    /** What the caller offers to do about it. This component performs nothing. */
    actions?: Snippet;
  }

  const {
    subject,
    excerpt,
    channelLabel,
    sender,
    senderLabel,
    receivedLabel,
    receivedAt,
    status,
    statusLabel,
    targetHref,
    targetLabel,
    dismissedNote,
    suggestion,
    actions,
  }: Props = $props();

  // `NEW` is the one that wants attention, and it is the only one with a tone. A decision that was
  // made is not a success and a dismissal is not a failure: both are simply settled.
  const tone = $derived(status === 'NEW' ? 'info' : 'neutral');
</script>

<article class="entry" data-status={status}>
  <Stack gap="150">
    <div class="head">
      <h3 class="subject">{subject}</h3>
      <Badge {tone}>{statusLabel ?? status}</Badge>
    </div>

    <p class="meta">
      <span>{channelLabel}</span>
      {#if sender && senderLabel}<span>{senderLabel}: {sender}</span>{/if}
      {#if receivedAt}
        <!-- A real `<time datetime>`: the instant is the machine's and the wording is the
             reader's, which is what lets one arrival read in whichever zone each client is set to. -->
        <time datetime={receivedAt}>{receivedLabel}</time>
      {:else}
        <span>{receivedLabel}</span>
      {/if}
    </p>

    {#if excerpt}
      <p class="excerpt">{excerpt}</p>
    {/if}

    {#if suggestion}{@render suggestion()}{/if}

    {#if targetHref && targetLabel}
      <p class="target"><a href={targetHref}>{targetLabel}</a></p>
    {/if}

    {#if dismissedNote}
      <p class="note">{dismissedNote}</p>
    {/if}

    {#if actions}
      <div class="actions">{@render actions()}</div>
    {/if}
  </Stack>
</article>

<style>
  .entry {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    min-width: 0;
  }

  /* A settled entry is quieter, and that is the whole of it: the border thins out, the text does
     not. Fading a dismissed row towards invisible would be a client drawing a deletion the server
     did not perform. */
  .entry[data-status='DISMISSED'] { background: var(--bg-surface-sunken); }

  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .subject {
    margin: 0;
    flex: 1 1 auto;
    min-width: 0;
    font-family: var(--font-display);
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
    /* The subject came from outside and may be one unbroken string of four hundred characters. */
    overflow-wrap: anywhere;
  }

  .meta {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-100);
    color: var(--text-subtle);
    font-size: var(--fs-075);
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .excerpt {
    margin: 0;
    color: var(--text-secondary);
    /* Whitespace is preserved because a plain-text mail is laid out with it, and collapsing it
       turns a list somebody sent into one long line. */
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .note { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  .target a { color: var(--text-brand); }

  .target a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }

  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
