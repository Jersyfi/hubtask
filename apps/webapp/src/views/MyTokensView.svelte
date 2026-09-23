<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The page around one's own tokens. `TokensView` is the panel, because the service accounts
  // screen shows the same list and the same form for an account that is not a person — and one
  // panel used twice is what keeps the mint's three rules in one place.

  import { PageHeader, Stack } from '@hubtask/design-system/components';

  import TokensView from './TokensView.svelte';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.tokens.title')));
</script>

<div class="screen">
  <Stack gap="300">
    <PageHeader title={t('app.tokens.title')} isTitleInBar={viewport.isCompact} />
    <p class="quiet">{t('app.tokens.intro')}</p>
    <TokensView />
  </Stack>
</div>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
