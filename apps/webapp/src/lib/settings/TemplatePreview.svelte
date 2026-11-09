<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A template card's *Preview* (user-experience.md §5.2): what taking it creates, folded so a
  // gallery stays calm. The form that takes a template draws `TemplateCreates` open instead.
  import { Disclosure } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { createsOf } from '../workspace/hubGallery.ts';
  import TemplateCreates from './TemplateCreates.svelte';

  interface Props {
    template: Parameters<typeof createsOf>[0];
  }

  const { template }: Props = $props();
</script>

{#if createsOf(template).length > 0}
  <Disclosure label={t('app.templates.preview')} tone="quiet" description={t('app.templates.preview_hint')}>
    <TemplateCreates {template} />
  </Disclosure>
{/if}
