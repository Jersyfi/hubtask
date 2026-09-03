<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Something happened, and the reader does not have to do anything about it.
  //
  // The rule F1-06 puts on it is the one toasts usually break: it is announced **without stealing
  // focus**. So there is no `autofocus`, no focus call, and no `role="alert"` on the ordinary case
  // - `role="status"` is announced at the next pause, which is what a completed save deserves. A
  // toast that took focus would throw a keyboard user out of the field they were typing in, which
  // is the exact moment a save confirmation arrives.
  //
  // It follows that a toast may not be the only place something is said, and that anything it
  // offers must be reachable another way: it disappears, and what disappears cannot be tabbed to
  // by somebody who was reading further up the page.
  //
  // Where the stack of them sits is the frame's decision (F1-10), not this component's. A toast
  // that positioned itself would be a second answer to where notifications appear, and two toasts
  // would then sit on top of each other.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import { STATUS_ICON, type StatusTone } from './control.ts';
  import type { IconName } from './icons/index.ts';

  interface Props {
    tone?: StatusTone;
    icon?: IconName;
    /** The name of the control that closes it. Present means dismissible, as on `Banner`. */
    dismissLabel?: string;
    onDismiss?: () => void;
    /** One thing to do about it - "Undo" is the reason this slot exists at all. */
    action?: Snippet;
    children?: Snippet;
  }

  const { tone = 'info', icon, dismissLabel, onDismiss, action, children }: Props = $props();
</script>

<div class="toast" data-tone={tone} role="status">
  <span class="mark"><Icon name={icon ?? STATUS_ICON[tone]} size="sm" /></span>
  <p class="text">{@render children?.()}</p>
  {#if action}<div class="action">{@render action()}</div>{/if}
  {#if dismissLabel}
    <IconButton icon="x" label={dismissLabel} size="sm" onclick={() => onDismiss?.()} />
  {/if}
</div>

<style>
  .toast {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
    /* Nothing may cover a toast, which is why the scale has nothing above it (tokens.json). The
       number is the token's; a z-index written here would be the failure §6 names. */
    z-index: var(--z-toast);
    position: relative;
    max-width: 60ch;
    padding: var(--sp-150) var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    box-shadow: var(--shadow-overlay);
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
    /* Rule 6: it arrives on opacity and transform, so nothing around it moves. */
    animation: arrive var(--dur-base) var(--ease-entrance) both;
  }

  @keyframes arrive {
    from { opacity: 0; transform: translateY(var(--sp-100)); }
    to { opacity: 1; transform: none; }
  }

  .mark { display: inline-flex; flex: none; }
  .toast[data-tone='info'] .mark { color: var(--text-brand); }
  .toast[data-tone='success'] .mark { color: var(--text-success); }
  .toast[data-tone='warning'] .mark { color: var(--text-warning); }
  .toast[data-tone='danger'] .mark { color: var(--text-danger); }

  .text { flex: 1; margin: 0; min-width: 0; overflow-wrap: anywhere; }

  .action { display: flex; flex: none; gap: var(--sp-100); }

  /* Rule 6's floor: the entrance reduces to the colour it already is - it appears, rather than
     sliding. The announcement is unaffected, because it never depended on the movement. */
  @media (prefers-reduced-motion: reduce) {
    .toast { animation: none; }
  }

  :global([data-motion='reduced']) .toast { animation: none; }
</style>
