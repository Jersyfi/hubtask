<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The question a move asks while a settings screen holds unsaved changes (user-experience.md
  // §5.4): stay and keep them, or leave without them. Asked once for the whole application, so
  // every screen that holds changes asks it in the same words.

  import { Button, Dialog } from '@hubtask/design-system/components';

  import { leaveGuard } from './leave.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  const count = $derived(leaveGuard.pending === undefined ? 0 : leaveGuard.unsaved);
</script>

<Dialog size="s" title={t('app.leave.title')} isOpen={leaveGuard.pending !== undefined} dismissLabel={t('app.leave.stay')} onClose={() => leaveGuard.stay()}>
  <p class="body" data-leave-question>{t('app.leave.body', { count: String(count) })}</p>
  {#snippet actions()}
    <Button tone="danger" onclick={() => leaveGuard.leave()}>{t('app.leave.leave')}</Button>
    <Button tone="primary" onclick={() => leaveGuard.stay()}>{t('app.leave.stay')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
