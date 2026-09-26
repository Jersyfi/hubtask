<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Every screen a signed-out visitor can reach, drawn as one card.
  //
  // **There is no shell here.** ADR-0061 draws the bar for a page that has navigation; signed out
  // there is nowhere to go, so a bar with a wordmark and nothing else is a row of furniture that
  // pushes the form down - on a phone, by exactly the height that decides whether the button is
  // under the keyboard. The wordmark moves onto the card, where it is also the thing that says
  // which product this is.
  //
  // **The host is named, and it is not a field.** Which workspace somebody is signing in to is
  // resolved from the address (multi-tenancy.md §3) and never from a body. Naming it turns the
  // address bar into something a reader can check, which is the cheapest anti-phishing measure a
  // sign-in screen has; making it a field would be the opposite.
  //
  // **The two washes are the only brand moment.** `ambient.primary` and `ambient.signature` have
  // been in `tokens.json` since v0.1 for exactly this - a tint laid over the canvas, the quietest
  // way the signature colour may appear - and no product screen had ever drawn them. Behind a
  // board they would be noise; behind one card on an empty canvas they are the one thing that
  // says Hubtask before anything is read.
  //
  // **The legal links are the operator's**, resolved for this workspace and rendered as whatever
  // is there - a link that is missing is a line that is missing, because the operator of a private
  // installation owes nobody an imprint.

  import type { Snippet } from 'svelte';

  import { Stack } from '@hubtask/design-system/components';

  import { signInRules } from '../data/signinrules.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The heading of this step. */
    title: string;
    /** Which step of how many, where a screen has more than one. */
    step?: { readonly index: number; readonly total: number };
    /** A sentence under the heading. */
    lead?: string;
    /** The banner slot: one refusal, one notice, never two. */
    notice?: Snippet;
    children: Snippet;
  }

  const { title, step, lead, notice, children }: Props = $props();

  const host = $derived(signInRules.workspaceHost ?? location.hostname);
  const legal = $derived(signInRules.legal);
</script>

<div class="canvas">
  <main id="main" class="main" tabindex="-1">
    <section class="card">
      <div class="brand">
        <!-- A name rather than a message: the product is called Hubtask in every language. -->
        <span class="wordmark">Hubtask</span>
        <span class="host">{t('app.sign_in.to_workspace', { host })}</span>
      </div>

      <Stack gap="250">
        {@render notice?.()}

        <Stack gap="050">
          {#if step}
            <p class="step">{t('app.sign_in.step', { step: String(step.index), total: String(step.total) })}</p>
          {/if}
          <h1>{title}</h1>
          {#if lead}
            <p class="lead">{lead}</p>
          {/if}
        </Stack>

        {@render children()}
      </Stack>
    </section>
  </main>

  <footer class="foot">
    {#if legal.imprint_url}
      <a href={legal.imprint_url} target="_blank" rel="noopener">{t('app.legal.imprint')}</a>
    {/if}
    {#if legal.privacy_url}
      <a href={legal.privacy_url} target="_blank" rel="noopener">{t('app.legal.privacy')}</a>
    {/if}
    {#if legal.terms_url}
      <a href={legal.terms_url} target="_blank" rel="noopener">{t('app.legal.terms')}</a>
    {/if}
    <a href={legal.accessibility_url ?? 'https://hubtask.eu/accessibility/'} target="_blank" rel="noopener">
      {t('app.about.accessibility')}
    </a>
  </footer>
</div>

<style>
  .canvas {
    display: flex;
    flex-direction: column;
    min-height: 100vh;
    /* The geometry is layout and stays here; the colours are the two ambient tokens and stay in
       tokens.json, which is what its own description asks for. */
    background:
      radial-gradient(ellipse 70% 60% at 12% -10%, var(--ambient-signature), transparent 70%),
      radial-gradient(ellipse 80% 70% at 100% 100%, var(--ambient-primary), transparent 70%),
      var(--bg-canvas);
  }

  .main {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--sp-400) var(--sp-200);
  }

  .main:focus { outline: none; }

  .card {
    /* The project has no global `box-sizing`, and this one has both a width and padding: without
       it the card is its own padding wider than the room it was given, and at 375 px it hangs off
       both edges of the screen. */
    box-sizing: border-box;
    /* Rule 4: a column that grows with its text. In characters, so a language that needs forty
       per cent more room takes it. */
    inline-size: min(100%, 52ch);
    display: grid;
    gap: var(--sp-250);
    padding: var(--sp-300);
    background: var(--bg-surface);
    /* Rule 1: raised is a standalone element. Not glass - only overlays blur. */
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-raised);
  }

  .brand {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--sp-100);
  }

  .wordmark {
    font-family: var(--font-display);
    font-size: var(--fs-200);
    font-weight: var(--fw-bold);
    color: var(--text-primary);
  }

  .host {
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    color: var(--text-subtle);
  }

  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-500);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .step {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    text-transform: uppercase;
    color: var(--text-subtle);
  }

  .lead {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-100);
  }

  .foot {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: var(--sp-100) var(--sp-250);
    padding: var(--sp-200);
    font-size: var(--fs-075);
  }

  .foot a { color: var(--text-subtle); }
</style>
