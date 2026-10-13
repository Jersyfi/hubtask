<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The frame's notice that one of the reader's private hubs was opened through the emergency access
  // (ADR-0073 §4): on every page, from the record written when it was opened, for thirty days.
  // It is how a person without a mailbox is told at all; the hub itself says it too.

  import { untrack } from 'svelte';

  import { Banner } from '@hubtask/design-system/components';

  import { accounts } from '../data/accounts.svelte.ts';
  import { emergencyNotices } from '../data/emergencynotices.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => untrack(() => emergencyNotices.open()));
  $effect(() => {
    accounts.resolve(emergencyNotices.notices.map((each) => each.opened_by));
  });

  const at = (iso: string) => formatDateTime(iso, messages.locale);
</script>

{#each emergencyNotices.notices as notice (notice.id)}
  <div class="notice" data-emergency-notice="frame">
    <Banner tone="warning">
      {t('app.private_hubs.notice_frame', {
        name: accounts.nameOf(notice.opened_by) ?? t('app.private_hubs.somebody'),
        when: at(notice.opened_at), until: at(notice.ends_at), reason: notice.reason,
      })}
      {#snippet action()}
        <a
          href={`/hubs/${notice.hub_id}`}
          onclick={(event) => {
            event.preventDefault();
            onnavigate(`/hubs/${notice.hub_id}`);
          }}
        >
          {t('app.private_hubs.notice_open_hub')}
        </a>
      {/snippet}
    </Banner>
  </div>
{/each}

<style>
  .notice { margin-block-end: var(--sp-200); }
</style>
