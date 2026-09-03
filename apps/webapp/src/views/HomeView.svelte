<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The first view, and honest about being nearly empty: hubs, collections and the item hierarchy
  // are F2's, and inventing something to show here would be inventing a screen that has to be
  // taken away again.
  import { Stack } from '@hubtask/design-system/components';

  import { t } from '../lib/i18n/i18n.svelte.ts';
</script>

<Stack gap="200">
  <p>{t('app.home.empty')}</p>
</Stack>
