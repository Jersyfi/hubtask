<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The page around one's own tokens. `TokensView` is the panel, because the service accounts
  // screen shows the same list and the same form for an account that is not a person — and one
  // panel used twice is what keeps the mint's three rules in one place.

  import { Stack } from '@hubtask/design-system/components';

  import TokensView from './TokensView.svelte';
  import SettingsHead from '../lib/frame/SettingsHead.svelte';
  import { t } from '../lib/i18n/i18n.svelte.ts';
</script>

<Stack gap="300">
  <!-- The section's head: the title, the trail and the entitling are one component for all eight
       screens of Your settings (ADR-0065 decision 3). -->
  <SettingsHead row="tokens" />
  <p class="quiet">{t('app.tokens.intro')}</p>
  <TokensView />
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
