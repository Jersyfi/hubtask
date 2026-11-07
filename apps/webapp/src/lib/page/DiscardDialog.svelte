<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Discard changes?* (page-system §5.2): the one dialog allowed over another, asked when a dialog
  // holding changes is closed without its action. The words are the same everywhere.
  import { Button, Dialog } from '@hubtask/design-system/components';

  import type { DiscardGuard } from './discard.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  const { guard }: { guard: DiscardGuard } = $props();
</script>

<Dialog
  size="s"
  title={t('app.page.discard_title')}
  isOpen={guard.isAsking}
  dismissLabel={t('app.page.discard_keep')}
  onClose={() => guard.keep()}
  primary={{ label: t('app.page.discard'), tone: 'danger', onclick: () => guard.discard() }}
>
  <p class="body" data-discard-question>{t('app.page.discard_body')}</p>
  {#snippet actions()}
    <Button tone="secondary" onclick={() => guard.keep()}>{t('app.page.discard_keep')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
