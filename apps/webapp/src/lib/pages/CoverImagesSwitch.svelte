<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Which image library a page's cover may take pictures from (UC-DOC-16 check 5), for somebody who
  // manages the workspace's members; none by default. The library is named only here, as the
  // choice itself. Locked by the installation or the plan, it is shown and not changed.

  import { Select, SettingRow } from '@hubtask/design-system/components';
  import type { CoverImagesPolicy } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { coverImagesPolicyPath, pages } from '../data/pages.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const administers = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const policy = resource<CoverImagesPolicy>(coverImagesPolicyPath);
  const held = $derived(policy.state.status === 'ready' ? policy.state.data : undefined);

  async function set(library: string) {
    try {
      await pages.setCoverImages(library as CoverImagesPolicy['library']);
      toast.show({ text: library === 'OFF' ? t('app.admin.cover_images_unset') : t('app.admin.cover_images_set'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }
</script>

{#if administers && held}
  <div data-cover-images>
    <SettingRow label={t('app.admin.cover_images')} help={t('app.admin.cover_images_hint')} fieldWidth="sm">
      {#snippet control(nameId)}
        <Select
          label={t('app.admin.cover_images')}
          isLabelHidden
          aria-labelledby={nameId}
          disabledReason={held.locked_by ? t('app.admin.cover_images_locked') : undefined}
          bind:value={() => held.library ?? 'OFF', (library) => void set(library)}
          options={[
            { value: 'OFF', label: t('app.admin.cover_images_none') },
            { value: 'OPENVERSE', label: 'Openverse' },
          ]}
        />
      {/snippet}
    </SettingRow>
  </div>
{/if}
