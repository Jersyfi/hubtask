<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A portfolio roadmap's public link (UC-GOL-03 check 15), for somebody who manages the
  // workspace's members: made where the workspace allows public roadmaps, shown once and copied,
  // replaced or taken away. Whoever holds the link reads the roadmap as its maker reads it.

  import { untrack } from 'svelte';
  import { Button, Inline, Input, Stack } from '@hubtask/design-system/components';
  import type { PortfolioPublicLinkState, PublicLinksPolicy } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import { roadmapLinkAddress } from '../data/roadmaplink.ts';
  import { publicRoadmapsPolicyPath, roadmapLinkPath, roadmapLinks } from '../data/roadmaplink.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    portfolioId: string;
  }

  const { portfolioId }: Props = $props();

  const administers = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const held = untrack(() => resource<PortfolioPublicLinkState>(roadmapLinkPath(portfolioId)));
  const policy = resource<PublicLinksPolicy>(publicRoadmapsPolicyPath);
  const link = $derived(held.state.status === 'ready' ? held.state.data.link : null);
  const allowed = $derived(policy.state.status === 'ready' && policy.state.data.allowed);
  const nameOf = (id: string | null | undefined) => (id ? accounts.nameOf(id) : undefined) ?? t('app.people.unnamed');

  let madeAddress = $state<string | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);

  async function make() {
    refusal = undefined;
    try {
      const made = await roadmapLinks.make(portfolioId);
      madeAddress = roadmapLinkAddress(globalThis.location.origin, made.token);
      void navigator.clipboard?.writeText(madeAddress).catch(() => undefined);
      toast.show({ text: t('app.roadmap.link_made'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }

  async function remove() {
    refusal = undefined;
    madeAddress = undefined;
    try {
      await roadmapLinks.remove(portfolioId);
      toast.show({ text: t('app.roadmap.link_removed'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }
</script>

{#if administers && (link || allowed)}
  <section aria-labelledby="roadmap-link-heading" class="panel" data-roadmap-link>
    <Stack gap="100">
      <h2 class="heading" id="roadmap-link-heading">{t('app.roadmap.link')}</h2>
      {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
      {#if link}
        <p class="quiet">
          {allowed ? t('app.roadmap.link_open', { name: nameOf(link.created_by) }) : t('app.roadmap.link_paused')}
        </p>
      {:else}
        <p class="quiet">{t('app.roadmap.link_none')}</p>
      {/if}
      {#if madeAddress}
        <Input label={t('app.roadmap.link_address')} value={madeAddress} readonly />
        <p class="quiet">{t('app.roadmap.link_once')}</p>
      {/if}
      <Inline gap="100" isWrapping>
        {#if allowed}
          <Button size="sm" tone="secondary" icon="link" onclick={make}>
            {link ? t('app.roadmap.link_renew') : t('app.roadmap.link_make')}
          </Button>
        {/if}
        {#if link}
          <Button size="sm" tone="subtle" onclick={remove}>{t('app.roadmap.link_remove')}</Button>
        {/if}
      </Inline>
    </Stack>
  </section>
{/if}

<style>
  .panel { border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); padding: var(--sp-150); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
