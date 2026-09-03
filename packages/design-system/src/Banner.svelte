<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Something is true about this page, and it stays true until it is not.
  //
  // That is the whole difference from `Toast`, and it decides everything else: a banner is in the
  // flow, it does not time out, and it moves the content below it rather than covering it. The
  // frame needs it twice - ADR-0035 §2's maturity banner and §4's `HealthBanner` are both a banner
  // with content, which is why neither of them is a component of its own.
  //
  // Dismissal is `onDismiss`, not a boolean the component owns. Whether a banner comes back on the
  // next page is a decision about the *thing being announced* - the maturity banner is dismissed
  // for the session, a degradation comes back while it lasts - and a component that remembered
  // would be answering a question it cannot see.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import { STATUS_ICON, type StatusTone } from './control.ts';
  import type { IconName } from './icons/index.ts';

  interface Props {
    tone?: StatusTone;
    /** The one line a reader takes away. Resolved text (ADR-0011). */
    title?: string;
    /** The tone's own mark, replaced where a better one exists. */
    icon?: IconName;
    /**
     * The name of the control that dismisses it. Present means dismissible - the same shape as
     * `disabledReason`: the label and the behaviour cannot come apart, so there is no dismissible
     * banner whose close button is called nothing.
     */
    dismissLabel?: string;
    onDismiss?: () => void;
    /** What else can be done about it: a link, a retry, a settings button. */
    action?: Snippet;
    children?: Snippet;
  }

  const { tone = 'info', title, icon, dismissLabel, onDismiss, action, children }: Props = $props();

  // A degradation and a failure interrupt; a notice waits for a pause. Assertive on everything
  // would make the maturity banner talk over whatever the reader was doing on arrival.
  const live = $derived(tone === 'danger' || tone === 'warning' ? 'alert' : 'status');
</script>

<div class="banner" data-tone={tone} role={live}>
  <span class="mark"><Icon name={icon ?? STATUS_ICON[tone]} size="sm" /></span>
  <div class="body">
    {#if title}<p class="title">{title}</p>{/if}
    {#if children}<p class="text">{@render children()}</p>{/if}
    {#if action}<div class="action">{@render action()}</div>{/if}
  </div>
  {#if dismissLabel}
    <IconButton icon="x" label={dismissLabel} size="sm" onclick={() => onDismiss?.()} />
  {/if}
</div>

<style>
  .banner {
    display: flex;
    align-items: start;
    gap: var(--sp-150);
    padding: var(--sp-150) var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font-size: var(--fs-100);
    /* Rule 4: no fixed width. The banner is as wide as it is given and its text wraps inside it. */
    text-align: start;
  }

  /* Rule 3: the mark carries the tone as well as the colour does, so the row still reads in
     greyscale. The text stays the reading colour - a whole paragraph in red is harder to read and
     says no more than the icon beside it already does. The colour is on the wrapper rather than on
     the icon, so the dismiss button's own mark is not dragged into the tone with it. */
  .mark { display: inline-flex; margin-block-start: var(--sp-025); }
  .banner[data-tone='info'] .mark { color: var(--text-brand); }
  .banner[data-tone='success'] .mark { color: var(--text-success); }
  .banner[data-tone='warning'] .mark { color: var(--text-warning); }
  .banner[data-tone='danger'] .mark { color: var(--text-danger); }

  .body {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    flex: 1;
    min-width: 0;
  }

  .title {
    margin: 0;
    font-weight: var(--fw-medium);
    overflow-wrap: anywhere;
  }

  .text {
    margin: 0;
    max-width: 80ch;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .action { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
