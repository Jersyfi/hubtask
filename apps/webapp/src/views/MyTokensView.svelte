<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The page around one's own tokens. `TokensView` is the panel, because the service accounts
  // screen shows the same list and the same form for an account that is not a person — and one
  // panel used twice is what keeps the mint's three rules in one place.

  import { Stack } from '@hubtask/design-system/components';

  import TokensView from './TokensView.svelte';
  import { t } from '../lib/i18n/i18n.svelte.ts';
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.tokens.title')}</h1>
    <p class="quiet">{t('app.tokens.intro')}</p>
    <TokensView />
  </Stack>
</div>

<style>
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .quiet { margin: 0; color: var(--text-secondary); }
</style>
