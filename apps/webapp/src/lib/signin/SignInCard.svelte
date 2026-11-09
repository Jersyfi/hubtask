<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Every screen a signed-out visitor can reach, drawn as one card: a standalone page.
  //
  // **There is no shell here.** ADR-0061 draws the bar for a page that has navigation; signed out
  // there is nowhere to go, so a bar with a wordmark and nothing else is a row of furniture that
  // pushes the form down - on a phone, by exactly the height that decides whether the button is
  // under the keyboard. The mark moves onto the card, where it is also the thing that says
  // which product this is.
  //
  // **The workspace is named, its host beside it, and neither is a field.** Which workspace somebody
  // is signing in to is resolved from the address (multi-tenancy.md §3) and never from a body. The
  // host under the name turns the address bar into something a reader can check, which is the
  // cheapest anti-phishing measure a sign-in screen has; making it a field would be the opposite.
  //
  // The card, its canvas and the two ambient washes are the standalone page kind's (page-system
  // §7.9); this adds the mark, the workspace and the step.
  //
  // **The legal links are the operator's**, resolved for this workspace and rendered as whatever
  // is there - a link that is missing is a line that is missing, because the operator of a private
  // installation owes nobody an imprint.

  import type { Snippet } from 'svelte';

  import { Logo, Stack, StandalonePage } from '@hubtask/design-system/components';

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
    /**
     * The installation's own address rather than a workspace's: the host is named alone, since
     * nobody signs in to it (UC-ID-25 check 3).
     */
    isInstallation?: boolean;
    /**
     * The line beside the mark where it is not this page's host: an installed app names the
     * workspace it signs in to, and nothing before one is chosen (`null`).
     */
    hostLine?: string | null;
    children: Snippet;
  }

  const { title, step, lead, notice, isInstallation = false, hostLine, children }: Props = $props();

  const host = $derived(signInRules.workspaceHost ?? location.hostname);
  const workspace = $derived(signInRules.workspaceName);
  const legal = $derived(signInRules.legal);
</script>

<StandalonePage>
  <div class="brand">
    <!-- A name rather than a message: the product is called Hubtask in every language. -->
    <Logo size="md" wordmark="Hubtask" />
    {#if hostLine !== null}
      {#if hostLine === undefined && !isInstallation && workspace}
        <span class="workspace" data-sign-in-workspace>
          <span class="name">{t('app.sign_in.to_workspace_named', { name: workspace })}</span>
          <span class="host">{host}</span>
        </span>
      {:else}
        <span class="host">{hostLine ?? (isInstallation ? location.hostname : t('app.sign_in.to_workspace', { host }))}</span>
      {/if}
    {/if}
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
  {#snippet foot()}
    {#if legal.imprint_url}
      <a href={legal.imprint_url} target="_blank" rel="noopener">{t('app.legal.imprint')}</a>
    {/if}
    {#if legal.privacy_url}
      <a href={legal.privacy_url} target="_blank" rel="noopener">{t('app.legal.privacy')}</a>
    {/if}
    {#if legal.terms_url}
      <a href={legal.terms_url} target="_blank" rel="noopener">{t('app.legal.terms')}</a>
    {/if}
    <!-- The operator's, like the other three, and absent when unset (UC-ID-18 check 5). It used
         to fall back to hubtask.eu and say so in its label, which made the project answer for an
         installation it does not run. -->
    {#if legal.accessibility_url}
      <a href={legal.accessibility_url} target="_blank" rel="noopener">{t('app.legal.accessibility')}</a>
    {/if}
  {/snippet}
</StandalonePage>

<style>
  .brand {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-100);
  }

  .workspace { display: grid; justify-items: end; gap: var(--sp-025); text-align: end; }
  .name { font-size: var(--fs-100); font-weight: var(--fw-medium); color: var(--text-primary); overflow-wrap: anywhere; }

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
</style>
