<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A person, in the space of a line.
  //
  // Two decisions worth stating, because both are usually made wrongly by default.
  //
  // **The name is required and it is the accessible name.** An avatar is a picture of a person and
  // means nothing without one; `role="img"` with `aria-label` puts the name where a screen reader
  // reads it once, rather than reading a file name or nothing at all.
  //
  // **The initials are code points, not words.** Splitting a name on spaces and taking capitals is
  // a Latin-script assumption that produces one letter for a Chinese name and nonsense for an
  // Arabic one. The first character of the first word and of the last is the least-wrong rule that
  // works in every script; where it is still poor, the picture is the answer and the name is
  // always there underneath.

  import type { ControlSize } from './control.ts';

  interface Props {
    /** Who this is. Required: an avatar with no name is a decoration with no meaning. */
    name: string;
    /** Their picture. Falls back to the initials when it is absent or fails to load. */
    src?: string;
    size?: ControlSize;
  }

  const { name, src, size = 'md' }: Props = $props();

  let failed = $state(false);

  /** First and last character of the name, by code point rather than by UTF-16 unit. */
  const initials = $derived.by(() => {
    const words = name.trim().split(/\s+/).filter(Boolean);
    if (words.length === 0) return '';
    const first = [...(words[0] ?? '')][0] ?? '';
    const last = words.length > 1 ? ([...(words[words.length - 1] ?? '')][0] ?? '') : '';
    return `${first}${last}`;
  });
</script>

<span class="avatar" data-size={size} role="img" aria-label={name}>
  {#if src && !failed}
    <!-- Empty alt: the wrapper already carries the name, and a picture announced twice is worse
         than one announced once. -->
    <img class="picture" {src} alt="" onerror={() => (failed = true)} />
  {:else}
    <span class="initials" aria-hidden="true">{initials}</span>
  {/if}
</span>

<style>
  .avatar {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    overflow: hidden;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-weight: var(--fw-medium);
    /* Deliberately not uppercased: a name is written the way its owner writes it, and `text-
       transform` on a script with no case does nothing while breaking one that has case rules of
       its own. */
    line-height: 1;
    user-select: none;
  }

  .avatar[data-size='md'] { width: var(--sp-400); height: var(--sp-400); font-size: var(--fs-075); }
  .avatar[data-size='sm'] { width: var(--sp-300); height: var(--sp-300); font-size: var(--fs-050); }

  .picture { width: 100%; height: 100%; object-fit: cover; }
</style>
