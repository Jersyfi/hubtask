<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The trust line right under a page's title (S-2): who answers for the page, "verified until …"
  // or "review due" as a lozenge, who changed it last, then the labels. A reader sees at once
  // whether the page can be trusted, before the first sentence.
  import type { Snippet } from 'svelte';
  import Avatar from './Avatar.svelte';
  import StateLozenge from './StateLozenge.svelte';

  interface Props {
    /** *Mehmet Yılmaz*; absent, the line says `ownerless` instead. */
    owner?: string;
    /** *answers for this page* - drawn after the owner's name. */
    ownerRole: string;
    /** *No one answers for this page* - drawn when no owner is named. */
    ownerless?: string;
    /** The verification: `verified` with its words, `due` when it ran out, absent when never verified. */
    verification?: { readonly state: 'verified' | 'due'; readonly label: string };
    /** *Changed 22 min. ago by Amelia*. */
    changed?: string;
    /** The labels and their *+ Label*: the caller's own chips. */
    labels?: Snippet;
    /** What a writer may do with the verification: the lozenge becomes a button. */
    onverification?: () => void;
  }

  const { owner, ownerRole, ownerless, verification, changed, labels, onverification }: Props = $props();
</script>

<div class="trust" data-trust-line>
  <p class="line">
    {#if owner}
      <span class="owner"><Avatar name={owner} size="sm" /><strong>{owner}</strong> {ownerRole}</span>
    {:else if ownerless}
      <span class="owner quiet">{ownerless}</span>
    {/if}
    {#if verification}
      {#if onverification}
        <button type="button" class="verify" data-verification={verification.state} onclick={onverification}>
          <StateLozenge label={verification.label} tone={verification.state === 'verified' ? 'success' : 'warning'} icon={verification.state === 'verified' ? 'shield' : 'clock'} />
        </button>
      {:else}
        <span data-verification={verification.state}>
          <StateLozenge label={verification.label} tone={verification.state === 'verified' ? 'success' : 'warning'} icon={verification.state === 'verified' ? 'shield' : 'clock'} />
        </span>
      {/if}
    {/if}
    {#if changed}<span class="quiet">{changed}</span>{/if}
  </p>
  {#if labels}<div class="labels">{@render labels()}</div>{/if}
</div>

<style>
  .trust {
    display: grid;
    gap: var(--sp-050);
    padding-block: var(--sp-050) var(--sp-100);
  }

  .line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-050) var(--sp-150);
    max-inline-size: none;
    margin: 0;
    font-size: var(--fs-090);
    color: var(--text-primary);
  }

  .owner { display: inline-flex; align-items: center; gap: var(--sp-050); }
  .owner strong { font-weight: var(--fw-semibold); }
  .quiet { color: var(--text-secondary); }

  .verify {
    display: inline-flex;
    padding: 0;
    border: none;
    border-radius: var(--r-sm);
    background: none;
    font: inherit;
    cursor: pointer;
  }

  .verify:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .labels { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }
</style>
