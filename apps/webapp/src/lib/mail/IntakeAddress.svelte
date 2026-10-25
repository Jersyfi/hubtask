<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's mail intake address with its copy button (ADR-0094): what a person forwards a
  // mail to, `#einkauf` in the subject choosing the collection.

  import { IconButton } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    address: string;
    lead?: string;
  }

  let { address, lead }: Props = $props();

  async function copy() {
    try {
      await navigator.clipboard.writeText(address);
      announcer.say(t('app.mail.copied_announced'));
    } catch {
      announcer.say(t('app.mail.copy_failed', { address }));
    }
  }
</script>

{#if lead}<p class="lead">{lead}</p>{/if}
<div class="address" data-intake-address>
  <code>{address}</code>
  <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: address })} onclick={() => void copy()} />
</div>
<p class="hint">{t('app.mail.subject_hint')}</p>

<style>
  .lead, .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .address { display: flex; align-items: center; gap: var(--sp-100); flex-wrap: wrap; }
  code { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
</style>
