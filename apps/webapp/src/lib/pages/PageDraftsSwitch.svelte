<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Whether a hub's pages wait in drafts until a writer publishes them (UC-DOC-15), among the hub's
  // settings (user-experience.md §7.1). Drawn once the space answers: a hub without the knowledge
  // module, or a reader the space refuses, sees no switch.

  import { Switch } from '@hubtask/design-system/components';
  import type { PageSpaceDrafts } from '@hubtask/sync-engine';

  import { pages, spaceDraftsPath } from '../data/pages.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { hubId }: { hubId: string } = $props();

  // svelte-ignore state_referenced_locally
  const space = resource<PageSpaceDrafts>(spaceDraftsPath(hubId));
  const held = $derived(space.state.status === 'ready' ? space.state.data : undefined);

  async function set(on: boolean) {
    try {
      await pages.setDrafts(hubId, on);
      toast.show({ text: t(on ? 'app.pages.drafts_on' : 'app.pages.drafts_off'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }
</script>

{#if held}
  <section class="drafts" aria-labelledby="page-drafts-head" data-page-drafts>
    <h2 class="head" id="page-drafts-head">{t('app.pages.settings_section')}</h2>
    <Switch
      label={t('app.pages.drafts_switch')}
      hint={t('app.pages.drafts_switch_hint')}
      bind:checked={() => held.drafts === true, (on) => void set(on)}
    />
  </section>
{/if}

<style>
  .drafts {
    display: grid;
    gap: var(--sp-100);
    margin-block-start: var(--sp-400);
    padding-block-start: var(--sp-300);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }
  .head { margin: 0; font-family: var(--font-display); font-size: var(--fs-200); font-weight: var(--fw-semibold); }
</style>
