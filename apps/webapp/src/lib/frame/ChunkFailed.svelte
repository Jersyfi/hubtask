<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What a screen or the frame draws when its chunk could not be fetched, in one shape for every
  // lazy screen (user-experience.md §5.8): after an upgrade it is news with *Reload* as the primary
  // act, offline it says so, otherwise it offers to load again. A reload is the retry: a browser
  // may keep a failed module import.
  import { ErrorState } from '@hubtask/design-system/components';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { ChunkFailure } from './chunkfailure.ts';

  const { outcome }: { outcome: Exclude<ChunkFailure, 'reload'> } = $props();
</script>

<section class="chunk-failed">
  <ErrorState
    kind={outcome === 'updated' ? 'updated' : 'failed'}
    title={t(outcome === 'updated' ? 'app.view_updated' : outcome === 'offline' ? 'app.view_unavailable' : 'app.view_failed')}
    retryLabel={t('app.view_reload')}
    onRetry={() => location.reload()}
  />
  <p class="back"><a href="/">{t('app.back_to_start')}</a></p>
</section>

<style>
  .chunk-failed { padding-block: var(--sp-300); }
  .back { margin: 0; text-align: center; font-size: var(--fs-100); }
</style>
