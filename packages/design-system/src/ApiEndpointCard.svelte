<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One operation of the contract, as a reader of the reference meets it: the method, the path,
  // the summary, and beneath them whatever the caller renders - the description, the parameter
  // tables, an example.
  //
  // The method is a `Badge` with a tone, and the tone follows what the method *does* rather than
  // a colour convention from somebody else's documentation: a read is neutral information, a
  // creation succeeds in making something, a change warns that state moves, a deletion is the
  // dangerous one. Rule 3 holds because the word is always beside the colour.
  //
  // The card is a `<section>` with a heading and an `id`, so a reference page is an outline a
  // screen reader can jump through and a link into the page lands on the operation.

  import type { Snippet } from 'svelte';

  import Badge from './Badge.svelte';
  import type { StatusTone } from './control.ts';

  export type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'HEAD' | 'OPTIONS';

  interface Props {
    method: Method;
    path: string;
    /** The one line, resolved: the specification's own summary. */
    summary: string;
    /** The anchor. The operation's identifier is the obvious choice. */
    id: string;
    /** Drawn as the small word beside the summary; absent means current. Resolved text. */
    deprecatedLabel?: string;
    /** The heading level the page is at, so the card's heading nests correctly. */
    level?: 2 | 3 | 4;
    children?: Snippet;
  }

  const { method, path, summary, id, deprecatedLabel, level = 3, children }: Props = $props();

  const TONE: Record<Method, StatusTone | 'neutral'> = {
    GET: 'neutral',
    HEAD: 'neutral',
    OPTIONS: 'neutral',
    POST: 'success',
    PUT: 'warning',
    PATCH: 'warning',
    DELETE: 'danger',
  };
</script>

<section class="card" {id} class:is-deprecated={deprecatedLabel !== undefined}>
  <svelte:element this={`h${level}`} class="heading">
    <a class="anchor" href={`#${id}`}>
      <span class="method"><Badge tone={TONE[method]}>{method}</Badge></span>
      <code class="path" dir="ltr">{path}</code>
    </a>
  </svelte:element>
  <p class="summary">
    {summary}
    {#if deprecatedLabel}<span class="deprecated">{deprecatedLabel}</span>{/if}
  </p>
  {#if children}
    <div class="body">{@render children()}</div>
  {/if}
</section>

<style>
  .card {
    display: flex;
    flex-direction: column;
    gap: var(--sp-150);
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    color: var(--text-primary);
    text-align: start;
    max-inline-size: 100%;
  }

  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); line-height: var(--lh-snug); }

  .anchor {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100);
    color: inherit;
    text-decoration: none;
    border-radius: var(--r-sm);
  }
  .anchor:hover .path { text-decoration: underline; }
  .anchor:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .method { font-family: var(--font-mono); }
  /* A path runs one way whatever the page does, which is why the element says so itself. */
  .path { font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }

  .summary { margin: 0; color: var(--text-secondary); max-width: 80ch; }
  .deprecated {
    margin-inline-start: var(--sp-100);
    font-size: var(--fs-050);
    text-transform: uppercase;
    color: var(--text-danger);
  }
  .is-deprecated .path { text-decoration: line-through; }

  .body { display: flex; flex-direction: column; gap: var(--sp-200); }
</style>
