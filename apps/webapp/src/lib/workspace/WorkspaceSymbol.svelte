<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's square mark (UC-INS-22 check 6, the approved draft's scenes 2 and 3): its
  // symbol, or two letters where it has none - a mark and not a colour, so no two workspaces look
  // alike by colour. The box is fixed whatever the picture does: kept empty while the picture's
  // address is asked for, and the letters again when it fails to load, never a broken image.
  // Decorative: wherever it stands, a name beside it or around it names the workspace.

  import { initialsOf, type PictureSource } from './branding.ts';

  interface Props {
    name: string;
    /** The symbol's address; `null` while it is asked for; absent where there is none. */
    source?: PictureSource;
    /** `sm` beside a name in the header, `lg` alone or in a list, `nav` in a navigation row's mark. */
    size?: 'sm' | 'lg' | 'nav';
    /** Told when the picture fails to load, so a caller can fall back past it. */
    onfail?: () => void;
  }

  const { name, source, size = 'sm', onfail }: Props = $props();

  let failed = $state<string | undefined>(undefined);
  const drawn = $derived(typeof source === 'string' && source !== failed ? source : undefined);
</script>

<span class="symbol" data-size={size} aria-hidden="true">
  {#if drawn}
    <img src={drawn} alt="" onerror={() => { failed = drawn; onfail?.(); }} />
  {:else if source !== null}
    {initialsOf(name)}
  {/if}
</span>

<style>
  .symbol {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-300);
    block-size: var(--sp-300);
    border-radius: var(--r-sm);
    overflow: hidden;
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    line-height: 1;
  }

  .symbol[data-size='lg'] {
    inline-size: var(--sp-400);
    block-size: var(--sp-400);
    border-radius: var(--r-md);
    font-size: var(--fs-075);
  }

  .symbol[data-size='nav'] {
    inline-size: var(--layout-nav-mark);
    block-size: var(--layout-nav-mark);
    font-size: var(--fs-050);
  }

  .symbol img { inline-size: 100%; block-size: 100%; object-fit: contain; }
</style>
