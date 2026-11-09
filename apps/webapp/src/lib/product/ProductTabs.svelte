<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product home's `ViewBar` on a tab drawn by a view of its own (the navigation concept §4): on
  // a phone the chip *Settings ▾* whose sheet lists the siblings.
  import { ViewBar } from '@hubtask/design-system/components';

  import { viewport } from '../frame/viewport.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { productName, productViews } from './frame.svelte.ts';

  interface Props {
    module: string;
    current: string;
    onnavigate?: (path: string) => void;
  }
  const { module, current, onnavigate }: Props = $props();

  const views = $derived(productViews(module));
</script>

<ViewBar label={productName(module)} {views} isCompact={viewport.isCompact} closeLabel={t('app.sheet_close')} {current}
  onnavigate={(id) => onnavigate?.(views.find((each) => each.id === id)?.href ?? '/')} />
