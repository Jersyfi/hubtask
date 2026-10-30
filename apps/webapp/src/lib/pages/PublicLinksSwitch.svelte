<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Whether the workspace lets a page be opened by a public link (UC-DOC-08 check 4), for somebody
  // who manages its members. Locked by the installation or the plan, it is shown and not switched.

  import { Switch } from '@hubtask/design-system/components';
  import type { PublicLinksPolicy } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { pages, publicLinksPolicyPath } from '../data/pages.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const administers = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const policy = resource<PublicLinksPolicy>(publicLinksPolicyPath);
  const held = $derived(policy.state.status === 'ready' ? policy.state.data : undefined);

  async function set(allowed: boolean) {
    try {
      await pages.setPublicLinks(allowed);
      toast.show({ text: allowed ? t('app.admin.public_links_on') : t('app.admin.public_links_off'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }
</script>

{#if administers && held}
  <div data-public-links>
    <Switch
      label={t('app.admin.public_links')}
      hint={t('app.admin.public_links_hint')}
      disabledReason={held.locked_by ? t('app.admin.public_links_locked') : undefined}
      bind:checked={() => held.allowed, (on) => void set(on)}
    />
  </div>
{/if}
